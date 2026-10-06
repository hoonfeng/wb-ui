// domctors.go — DOM 接口构造器族与 prototype 链的集中注册（第 19 次监督轮）。
//
// 目标：把「接口标识符是否存在 + instanceof / 原型链 / constructor.name 是否成立」
// 从**无界**清单（探针 globals 163 项 missing 中大部分永远不追）转为**有界**、
// 可复算的应做集合（见 WORKITEMS §19）。
//
// 设计（沿用第 17 轮 DocumentType 与 htmlelements.go 的 newCtor 模式）：
//
//  1. 每个接口一个全局构造器，其 .prototype 挂到父接口的 .prototype 上
//     （Document → Node、HTMLDocument → Document、PointerEvent → MouseEvent →
//     UIEvent → Event、SVGPathElement → SVGGeometryElement → SVGGraphicsElement →
//     SVGElement → Element …）；
//  2. 引擎包装实例时把实例原型指向对应接口 prototype（domAttachProto），于是
//     `el instanceof Element`、`Object.getPrototypeOf(x) === X.prototype`、
//     `x.constructor.name === "X"` 全部成立；
//  3. new X() 返回 goja 按 X.prototype 构造的 this（fn 内返回 this），因此
//     `new X() instanceof X === true`。既有 6 个事件构造器（Event/MouseEvent/
//     WheelEvent/KeyboardEvent/CustomEvent/ToggleEvent）此前返回自建对象
//     （原型为 Object.prototype）→ dom.go 已改为返回 this。
//
// ★ 兼容性取舍（有意为之，见 WORKITEMS §19-3）：
//
//	集合类接口（NodeList / HTMLCollection / DOMRectList）的 prototype.__proto__
//	指向 Array.prototype 而非 Object.prototype。原因：本引擎既有实现把
//	getElementsBy* / querySelectorAll / children / childNodes / getClientRects
//	暴露为**数组**，前端代码历史上直接调 .map()/.indexOf()/.slice()。指向
//	Array.prototype 使数组方法经原型链仍可达，同时 instanceof 成立。这是与
//	规范的有意偏差（规范里 NodeList.prototype.__proto__ === Object.prototype）。
package bindings

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// ─── 接口原型注册表 ───────────────────────────────────────────────

var (
	domIfaceMu  sync.RWMutex
	domIfacePro = map[string]*jsc.JSObject{} // 接口名 → prototype
	// domIfaceNames 是「已注册接口名」清单，供 resetAndAdoptDOMRegistry 在新
	// runtime 上重建注册表。只含字符串、不含 runtime 绑定，可安全跨 rt 共享。
	domIfaceNames []string
)

// svgTagIface 把 SVG 专属标签（小写）映射到接口名。查表后经 domIfaceProto 取
// **当前 runtime** 的 prototype —— 不缓存 prototype 对象本身，避免跨 rt 泄漏
// （jsc.JSObject 绑定创建它的 runtime，跨 rt 使用会被 goja 拒绝）。
var svgTagIface = map[string]string{
	"svg":           "SVGSVGElement",
	"path":          "SVGPathElement",
	"text":          "SVGTextElement",
	"tspan":         "SVGTextElement",
	"textpath":      "SVGTextElement",
	"image":         "SVGImageElement",
	"use":           "SVGUseElement",
	"foreignobject": "SVGForeignObjectElement",
}

// mathMLTags 是 MathML Core 的元素名（小写）。本引擎不建模命名空间，
// createElementNS 的 MathML 分支与解析器产生的这些标签按标签名分派。
var mathMLTags = []string{
	"math", "mrow", "mi", "mo", "mn", "ms", "mtext", "mspace",
	"maction", "merror", "mfrac", "mpadded", "mphantom", "mroot", "msqrt",
	"mstyle", "mtable", "mtd", "mtr", "munder", "mover", "munderover",
	"mmultiscripts", "mprescripts", "msub", "msup", "msubsup",
	"semantics", "annotation", "annotation-xml",
}

// domIfaceProto 返回接口名对应的 prototype（未注册返回 nil）。
func domIfaceProto(name string) *jsc.JSObject {
	domIfaceMu.RLock()
	defer domIfaceMu.RUnlock()
	return domIfacePro[name]
}

// domIfaceProtoOr 同 domIfaceProto，未注册时返回 fallback。
func domIfaceProtoOr(name string, fallback *jsc.JSObject) *jsc.JSObject {
	if p := domIfaceProto(name); p != nil {
		return p
	}
	return fallback
}

// resetAndAdoptDOMRegistry 在每次 RegisterDOMBindings 入口调用：清空注册表，并从
// **当前 runtime** 的全局构造器重填 prototype。
//
// 必要性（第 19 轮实测根因）：jsc.JSObject 绑定到创建它的 runtime，把 A runtime 的
// prototype 交给 B runtime 使用会被 goja 拒绝（"Illegal runtime transition of an
// Object"）。注册表是包级 map，若不清空重填，第二个 runtime 的 wrapDocument 会拿到
// 前一个 rt 的 prototype → 抛错 → document 没能挂上全局，表现为整个 bindings 测试
// 包 "document is not defined"（单独跑某个测试却通过，全量跑从第二个测试起全败）。
func resetAndAdoptDOMRegistry(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	domIfaceMu.Lock()
	defer domIfaceMu.Unlock()
	domIfacePro = map[string]*jsc.JSObject{}
	for _, name := range domIfaceNames {
		fv := g.GetStr(name)
		if !fv.IsObject() {
			continue
		}
		fo := fv.AsObject()
		if fo == nil {
			continue
		}
		pv := fo.GetStr("prototype")
		if !pv.IsObject() {
			continue
		}
		if p := pv.AsObject(); p != nil {
			domIfacePro[name] = p
		}
	}
}

// domAttachProto 把**已创建对象**的原型指向接口 prototype。
//
// goja 的 Object.prototype.__proto__ 是访问器属性，Set 即调用其 setter —— 与
// dom.go 中对 prototype 对象自身的链式设置（`elementProto.Set("__proto__", …)`）
// 走同一条路径，因此对实例同样有效。
func domAttachProto(obj *jsc.JSObject, name string) {
	if obj == nil {
		return
	}
	if p := domIfaceProto(name); p != nil {
		obj.Set("__proto__", jsc.ObjectValue(p))
	}
}

// svgElementProtoForTag 返回 SVG 专属标签对应的接口 prototype（未命中返回 nil）。
func svgElementProtoForTag(tag string) *jsc.JSObject {
	name, ok := svgTagIface[tag]
	if !ok {
		return nil
	}
	return domIfaceProto(name)
}

// mathMLElementProtoForTag 返回 MathML 标签对应的 MathMLElement.prototype
// （未命中返回 nil）。
func mathMLElementProtoForTag(tag string) *jsc.JSObject {
	for _, t := range mathMLTags {
		if t == tag {
			return domIfaceProto("MathMLElement")
		}
	}
	return nil
}

// arrayPrototypeOf 取 Array.prototype（集合类接口的父原型）。
func arrayPrototypeOf(rt *jsc.Interpreter) *jsc.JSObject {
	if rt == nil {
		return nil
	}
	fv := rt.GlobalObject().GetStr("Array")
	if !fv.IsObject() {
		return nil
	}
	p := fv.AsObject().GetStr("prototype")
	if !p.IsObject() {
		return nil
	}
	return p.AsObject()
}

// domCtorThis 返回构造器内应使用的目标对象：new 调用时是 goja 按
// `ctor.prototype` 构造的 this（因此 `new X() instanceof X` 成立）；普通调用
// （X()）时退回新建对象，保持宽容语义（浏览器多数 DOM 接口不可构造）。
func domCtorThis(in *jsc.Interpreter, this jsc.JSValue, proto *jsc.JSObject) *jsc.JSObject {
	if this.IsObject() {
		if o := this.AsObject(); o != nil {
			return o
		}
	}
	if proto == nil {
		proto = in.ObjectPrototype()
	}
	return jsc.NewObject(proto)
}

// domRegisterIface 注册接口构造器（默认实现 = 返回 this / 新建对象）。
func domRegisterIface(rt *jsc.Interpreter, g *jsc.JSObject, name string, parent *jsc.JSObject) *jsc.JSObject {
	return domRegisterIfaceFn(rt, g, name, parent,
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			return domCtorThis(in, this, nil)
		})
}

// domRegisterIfaceFn 注册接口构造器，构造行为由 fn 决定（fn 内应返回 this 对应的
// 对象，以保留 goja 按 prototype 建立的 this）。
func domRegisterIfaceFn(rt *jsc.Interpreter, g *jsc.JSObject, name string,
	parent *jsc.JSObject, fn func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject) *jsc.JSObject {
	if rt == nil || g == nil {
		return nil
	}
	ctor := rt.NewConstructor(name, fn)
	fnVal := jsc.FunctionValue(ctor)
	g.Set(name, fnVal)
	pv := fnVal.AsObject().GetStr("prototype")
	if !pv.IsObject() {
		return nil
	}
	proto := pv.AsObject()
	if proto == nil {
		return nil
	}
	if parent != nil {
		proto.Set("__proto__", jsc.ObjectValue(parent))
	}
	// 显式回填 constructor：库常读 x.constructor / x.constructor.name 做分支。
	proto.Set("constructor", fnVal)
	domIfacePro[name] = proto
	return proto
}

// domAdoptIface 把**既有**构造器（dom.go / htmlelements.go / worker.go /
// media*.go 注册的）纳入注册表，并（可选）修正其父原型。
//
// 只改父链与 constructor 回填，绝不覆盖构造器本身 —— 既有构造器带属性填充逻辑
// （如 MouseEvent 的 clientX/ctrlKey），重建会丢语义。
func domAdoptIface(g *jsc.JSObject, name string, parent *jsc.JSObject) *jsc.JSObject {
	fv := g.GetStr(name)
	if !fv.IsObject() {
		return nil
	}
	fo := fv.AsObject()
	if fo == nil {
		return nil
	}
	pv := fo.GetStr("prototype")
	if !pv.IsObject() {
		return nil
	}
	proto := pv.AsObject()
	if proto == nil {
		return nil
	}
	if parent != nil {
		proto.Set("__proto__", jsc.ObjectValue(parent))
	}
	if _, ok := proto.GetByKey("constructor"); !ok {
		proto.Set("constructor", fv)
	}
	domIfacePro[name] = proto
	return proto
}

// ─── 事件构造器（F 组）────────────────────────────────────────────

// eventCtorDictKeys 是事件构造器 init-dict 的白名单键：只复制浏览器
// *EventInit 字典里真实存在的字段，避免把任意键灌进事件对象（那会让拼写错误
// 静默生效，掩盖问题）。
var eventCtorDictKeys = []string{
	"bubbles", "cancelable", "composed",
	// MouseEvent / PointerEvent
	"clientX", "clientY", "screenX", "screenY", "button", "buttons", "detail",
	"ctrlKey", "shiftKey", "altKey", "metaKey", "relatedTarget",
	"pointerId", "pointerType", "isPrimary", "width", "height", "pressure",
	"tangentialPressure", "tiltX", "tiltY", "twist",
	// KeyboardEvent / InputEvent
	"key", "code", "keyCode", "location", "repeat", "isComposing", "inputType", "data",
	// WheelEvent
	"deltaX", "deltaY", "deltaZ", "deltaMode",
	// TouchEvent
	"changedTouches", "touches", "targetTouches",
	// ToggleEvent / 其它
	"oldState", "newState", "state", "reason", "persisted",
}

// newEventCtor 造一个事件接口构造器：填 Event 基础字段 + init-dict 白名单字段 +
// Event 方法，返回 this（因此 new PointerEvent('x') instanceof PointerEvent）。
func newEventCtor(className string) func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
	return func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		obj := domCtorThis(in, this, nil)
		obj.SetClassName(className)
		typ := ""
		if len(args) >= 1 && !args[0].IsUndefined() {
			typ = args[0].ToString()
		}
		obj.Set("type", jsc.StringValue(typ))
		obj.Set("bubbles", jsc.BooleanValue(false))
		obj.Set("cancelable", jsc.BooleanValue(false))
		obj.Set("composed", jsc.BooleanValue(false))
		obj.Set("defaultPrevented", jsc.BooleanValue(false))
		obj.Set("target", jsc.Null())
		obj.Set("currentTarget", jsc.Null())
		if len(args) >= 2 && args[1].IsObject() {
			if dict := args[1].AsObject(); dict != nil {
				for _, k := range eventCtorDictKeys {
					if v, ok := dict.GetByKey(k); ok {
						obj.Set(k, v)
					}
				}
			}
		}
		obj.Set("preventDefault", jsc.FunctionValue(jsc.NewNativeFunction("preventDefault",
			func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				if this.IsObject() {
					this.AsObject().Set("defaultPrevented", jsc.BooleanValue(true))
				}
				return jsc.Undefined()
			}, 0)))
		obj.Set("stopPropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopPropagation",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.Undefined()
			}, 0)))
		obj.Set("composedPath", jsc.FunctionValue(jsc.NewNativeFunction("composedPath",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.ObjectValue(jsc.NewArray(nil, nil))
			}, 0)))
		return obj
	}
}

// ─── 注册入口 ─────────────────────────────────────────────────────

// registerDOMInterfaces 注册 DOM 接口构造器族并接好 prototype 链。
//
// 必须在 RegisterDOMBindings 中所有既有构造器（Node/Element/HTMLElement/
// SVGElement/Text/Comment/DocumentFragment/DocumentType/Attr/NamedNodeMap/
// Event 族/Range/DOMParser/MessageEvent/HTML*Element）注册完成之后调用。
func registerDOMInterfaces(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	domIfaceMu.Lock()
	defer domIfaceMu.Unlock()

	// ★ 注意：jsc 的 rt.ObjectPrototype() 返回的是**新建空对象**
	// （goja_adapter.go:103 `&JSObject{obj: r.vm.NewObject()}`），并非 Object.prototype。
	// 把它当父原型会让 `Object.getPrototypeOf(X.prototype) !== Object.prototype`
	// （第 19 轮夹具实测 6 处偏差）。因此「父为 Object」的接口一律传 nil：
	// goja 新建 prototype 对象的默认 [[Prototype]] 就是 Object.prototype，正是规范要求。
	arrProto := arrayPrototypeOf(rt)

	// ── 既有构造器入表（不覆盖，只补父链）──
	nodeProto := domAdoptIface(g, "Node", nil)
	// ★ CharacterData（DOM §4.10）：Text / Comment / ProcessingInstruction 的父接口
	// （规范链：CDATASection → Text → CharacterData → Node；PI → CharacterData → Node）。
	// 此前未建模 → 这些接口的父链直接指向 Node，与浏览器不一致。
	charDataProto := domRegisterIface(rt, g, "CharacterData", nodeProto)
	elementProto := domAdoptIface(g, "Element", nodeProto)
	svgElementProto := domAdoptIface(g, "SVGElement", elementProto)
	domAdoptIface(g, "HTMLElement", elementProto)
	textProto := domAdoptIface(g, "Text", charDataProto)
	domAdoptIface(g, "Comment", charDataProto)
	docFragProto := domAdoptIface(g, "DocumentFragment", nodeProto)
	domAdoptIface(g, "DocumentType", nodeProto)
	domAdoptIface(g, "Attr", nodeProto)
	domAdoptIface(g, "NamedNodeMap", nil)
	domAdoptIface(g, "Range", nil)
	domAdoptIface(g, "DOMParser", nil)
	// ★ 必须保存返回值：本函数全程持有 domIfaceMu **写锁**，而 domIfaceProto 内部取
	//   读锁 —— sync.RWMutex 不可重入，注册期间调用 domIfaceProto 会**死锁**（第 20
	//   轮实测：探针/测试挂起在 WebView 初始化）。注册期间的父原型一律用局部变量传递。
	eventTargetProto := domAdoptIface(g, "EventTarget", nil)
	eventProto := domAdoptIface(g, "Event", nil)
	domAdoptIface(g, "CustomEvent", eventProto)
	domAdoptIface(g, "ToggleEvent", eventProto)
	if p := domAdoptIface(g, "MessageEvent", eventProto); p != nil {
		_ = p
	}

	// ── I 组：DOM 核心接口 ──
	domRegisterIface(rt, g, "NodeList", arrProto)
	domRegisterIface(rt, g, "HTMLCollection", arrProto)
	domRegisterIface(rt, g, "DOMTokenList", nil)
	rectRO := domRegisterIface(rt, g, "DOMRectReadOnly", nil)
	domRegisterIface(rt, g, "DOMRect", rectRO)
	domRegisterIface(rt, g, "DOMRectList", arrProto)
	domRegisterIface(rt, g, "NodeIterator", nil)
	domRegisterIface(rt, g, "TreeWalker", nil)
	domRegisterIface(rt, g, "Selection", nil)
	domRegisterIface(rt, g, "DOMImplementation", nil)
	docProto := domRegisterIface(rt, g, "Document", nodeProto)
	domRegisterIface(rt, g, "HTMLDocument", docProto)
	domRegisterIface(rt, g, "XMLDocument", docProto)
	domRegisterIface(rt, g, "ShadowRoot", docFragProto)
	domRegisterIface(rt, g, "CDATASection", textProto)
	domRegisterIface(rt, g, "ProcessingInstruction", charDataProto)

	// XMLSerializer：serializeToString 是本引擎真实可用的实例能力
	// （复用 dom.Element.GetOuterHTML 的同一序列化器）。
	domRegisterIfaceFn(rt, g, "XMLSerializer", nil,
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			obj := domCtorThis(in, this, nil)
			obj.SetClassName("XMLSerializer")
			obj.Set("serializeToString", jsc.FunctionValue(jsc.NewNativeFunction("serializeToString",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) == 0 {
						return jsc.StringValue("")
					}
					return jsc.StringValue(serializeNodeXML(unwrapNode(a[0])))
				}, 1)))
			return obj
		})

	// ── H 组：SVG / MathML 构造器（原型链按 SVG 2 §3.2 的接口继承）──
	graphics := domRegisterIface(rt, g, "SVGGraphicsElement", svgElementProto)
	if graphics == nil {
		graphics = svgElementProto
	}
	domRegisterIface(rt, g, "SVGSVGElement", graphics)
	geom := domRegisterIface(rt, g, "SVGGeometryElement", graphics)
	if geom == nil {
		geom = graphics
	}
	domRegisterIface(rt, g, "SVGPathElement", geom)
	domRegisterIface(rt, g, "SVGTextElement", graphics)
	domRegisterIface(rt, g, "SVGImageElement", graphics)
	domRegisterIface(rt, g, "SVGUseElement", graphics)
	domRegisterIface(rt, g, "SVGForeignObjectElement", graphics)
	domRegisterIface(rt, g, "MathMLElement", elementProto)
	// SVG / MathML 的「标签 → 接口」分派走本文件顶部的**静态映射**（svgTagIface /
	// mathMLTags），每次查表时经 domIfaceProto 取当前 runtime 的 prototype ——
	// 不再缓存 prototype 对象本身（跨 rt 复用会被 goja 拒绝）。

	// ── F 组：事件构造器（Event 子类；原型链按 DOM §2.2 / UI Events）──
	uiEv := domRegisterIface(rt, g, "UIEvent", eventProto)
	mouse := domAdoptIface(g, "MouseEvent", uiEv)
	if mouse == nil {
		mouse = uiEv
	}
	domAdoptIface(g, "KeyboardEvent", uiEv)
	domAdoptIface(g, "WheelEvent", mouse)
	// 新注册（带 init-dict 填充，可 new）。
	domRegisterIfaceFn(rt, g, "PointerEvent", mouse, newEventCtor("PointerEvent"))
	domRegisterIfaceFn(rt, g, "DragEvent", mouse, newEventCtor("DragEvent"))
	domRegisterIfaceFn(rt, g, "FocusEvent", uiEv, newEventCtor("FocusEvent"))
	domRegisterIfaceFn(rt, g, "TouchEvent", uiEv, newEventCtor("TouchEvent"))
	domRegisterIfaceFn(rt, g, "InputEvent", uiEv, newEventCtor("InputEvent"))
	domRegisterIfaceFn(rt, g, "CompositionEvent", uiEv, newEventCtor("CompositionEvent"))
	for _, n := range []string{
		"ClipboardEvent", "AnimationEvent", "TransitionEvent", "ErrorEvent",
		"PromiseRejectionEvent", "PopStateEvent", "HashChangeEvent",
		"BeforeUnloadEvent", "PageTransitionEvent", "StorageEvent", "SubmitEvent",
	} {
		domRegisterIfaceFn(rt, g, n, eventProto, newEventCtor(n))
	}

	// ── CSS OM 组（第 20 轮；WORKITEMS §20）───────────────────────────
	// 依据 CSSOM §1：这些接口的**实例**引擎早已在用，此前缺的只是全局构造器与
	// 实例原型（§17 类别②E 已判定为「应做（并入类别①）」，第 19 轮误置于不计
	// 判据的 globalsOptional —— 判据一致性修正见 §20-5）：
	//   el.style / getComputedStyle()  → CSSStyleDeclaration
	//   document.styleSheets           → StyleSheetList
	//   styleSheets[i]                 → CSSStyleSheet（→ StyleSheet）
	//   styleSheets[i].cssRules        → CSSRuleList
	//   cssRules[i]                    → CSSRule 家族（按规则类型分派，见 wrapCSSRule）
	//   matchMedia()                   → MediaQueryList（→ EventTarget）
	// 原型链按 **Edge 实测基线**（dev/output/tmp/protocheck 实测，非凭规范推断）：
	//   CSSStyleSheet → StyleSheet → Object
	//   CSSMediaRule → CSSConditionRule → CSSGroupingRule → CSSRule → Object
	//   CSSPageRule → CSSGroupingRule → CSSRule → Object
	//   CSSSupportsRule → CSSConditionRule → CSSGroupingRule → CSSRule → Object
	//   CSSStyleRule / CSSFontFaceRule / CSSKeyframesRule / CSSKeyframeRule /
	//   CSSImportRule / CSSNamespaceRule → CSSRule → Object
	//   MediaQueryList → EventTarget；MediaQueryListEvent → Event
	domRegisterIface(rt, g, "CSSStyleDeclaration", nil)
	domRegisterIface(rt, g, "StyleSheetList", nil)
	styleSheetProto := domRegisterIface(rt, g, "StyleSheet", nil)
	domRegisterIface(rt, g, "CSSStyleSheet", styleSheetProto)
	cssRuleProto := domRegisterIface(rt, g, "CSSRule", nil)
	domRegisterIface(rt, g, "CSSStyleRule", cssRuleProto)
	groupingProto := domRegisterIface(rt, g, "CSSGroupingRule", cssRuleProto)
	conditionProto := domRegisterIface(rt, g, "CSSConditionRule", groupingProto)
	domRegisterIface(rt, g, "CSSMediaRule", conditionProto)
	domRegisterIface(rt, g, "CSSSupportsRule", conditionProto)
	domRegisterIface(rt, g, "CSSFontFaceRule", cssRuleProto)
	domRegisterIface(rt, g, "CSSKeyframesRule", cssRuleProto)
	domRegisterIface(rt, g, "CSSKeyframeRule", cssRuleProto)
	domRegisterIface(rt, g, "CSSImportRule", cssRuleProto)
	domRegisterIface(rt, g, "CSSNamespaceRule", cssRuleProto)
	domRegisterIface(rt, g, "CSSPageRule", groupingProto)
	domRegisterIface(rt, g, "CSSRuleList", nil)
	domRegisterIface(rt, g, "MediaQueryList", eventTargetProto)
	domRegisterIfaceFn(rt, g, "MediaQueryListEvent", eventProto, newEventCtor("MediaQueryListEvent"))

	// ── canvas 2D 组（第 20 轮 B1；WORKITEMS §20-4）────────────────────
	// 引擎已有**完整** canvas 2D 实现（canvas2d.go：绘制走 Skia，xterm 的
	// cellWidth/cellHeight 测量实际依赖它）—— 这三项此前以「需要完整 2D 语义」
	// 列入 globalsExcluded，与探针 globals 的 typeof 口径不符（§20-4 修正）：
	//   getContext('2d') 返回对象              → CanvasRenderingContext2D
	//   createImageData / getImageData 返回对象 → ImageData
	//   Path2D：第 21 轮接上了实例方法与 ctx 的路径参数（canvas2d.go 的 path2D：
	//   moveTo/lineTo/rect/arc/closePath/bezierCurveTo/quadraticCurveTo/ellipse/
	//   arcTo/roundRect/addPath；ctx.fill/stroke/clip/isPointInPath 接受 Path2D）。
	//   ★ 遗留（WORKITEMS §21）：`new Path2D(svgPathData)` 的 SVG 字符串解析未实现
	//   → 传字符串得到空路径（不抛错）。
	domRegisterIface(rt, g, "CanvasRenderingContext2D", nil)
	domRegisterIface(rt, g, "ImageData", nil)
	domRegisterIfaceFn(rt, g, "Path2D", nil,
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := domCtorThis(in, this, domIfaceProto("Path2D"))
			p := &path2D{}
			// new Path2D(otherPath2D)：拷贝其命令（规范要求独立副本，不共享状态）。
			if len(args) >= 1 {
				if src := path2DFrom(args[0]); src != nil {
					p.cmds = append(p.cmds, src.cmds...)
					p.curX, p.curY, p.hasCur = src.curX, src.curY, src.hasCur
				}
				// 字符串（SVG path data）与其它入参：宽容 —— 不解析、不抛错（遗留见上）。
			}
			obj.SetClassName("Path2D")
			obj.SetInternal(p)
			installPath2DMethods(obj)
			return obj
		})

	// ★ 刷新「注册之前就已创建」的实例原型：主 document 与 Selection 单例在
	// RegisterDOMBindings 早期创建（早于本函数），当时注册表为空 → 原型落空
	// （第 19 轮夹具实测 `document instanceof Document` 曾为 false）。
	if p := domIfacePro["HTMLDocument"]; p != nil {
		if dv := g.GetStr("document"); dv.IsObject() {
			if o := dv.AsObject(); o != nil {
				o.Set("__proto__", jsc.ObjectValue(p))
			}
		}
	}
	if p := domIfacePro["Selection"]; p != nil && sstate.selObj != nil {
		sstate.selObj.Set("__proto__", jsc.ObjectValue(p))
	}

	// 记录接口名清单：下一个 runtime 进入 RegisterDOMBindings 时按此清单从它自己的
	// 全局构造器重填 prototype（resetAndAdoptDOMRegistry）。
	domIfaceNames = domIfaceNames[:0]
	for name := range domIfacePro {
		domIfaceNames = append(domIfaceNames, name)
	}
}

// ─── NodeIterator（document.createNodeIterator 的返回类型）────────────

// wrapNodeIterator 把 dom.NodeIterator 包装成 JS 对象（DOM §4.4 NodeIterator）。
// 此前只有 createTreeWalker，createNodeIterator 缺失 → NodeIterator 无实例。
func wrapNodeIterator(rt *jsc.Interpreter, it *dom.NodeIterator) *jsc.JSObject {
	obj := jsc.NewObject(domIfaceProtoOr("NodeIterator", rt.ObjectPrototype()))
	obj.SetClassName("NodeIterator")
	obj.SetInternal(it)
	obj.Set("root", nodeToJS(rt, it.Root()))
	obj.Set("whatToShow", jsc.NumberValue(float64(it.WhatToShow())))
	obj.SetAccessor("referenceNode", nodeAccFn(rt, func() dom.Node { return it.ReferenceNode() }), nil)
	obj.SetAccessor("pointerBeforeReferenceNode", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(it.PointerBeforeReferenceNode())
	}), nil)
	obj.Set("nextNode", jsc.FunctionValue(jsc.NewNativeFunction("nextNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := it.NextNode(); n != nil {
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	obj.Set("previousNode", jsc.FunctionValue(jsc.NewNativeFunction("previousNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := it.PreviousNode(); n != nil {
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	// detach() 按规范是 no-op（DOM §4.4）。
	obj.Set("detach", jsc.FunctionValue(jsc.NewNativeFunction("detach",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			it.Detach()
			return jsc.Undefined()
		}, 0)))
	return obj
}

// wrapDOMRectList 把矩形数组包装成 DOMRectList 语义对象（getClientRects 的返回
// 类型）。用**真数组** + DOMRectList.prototype：数组方法（map/indexOf/slice）经
// 原型链仍可达（见文件头兼容性取舍），同时 `rects instanceof DOMRectList` 成立。
func wrapDOMRectList(in *jsc.Interpreter, rects []jsc.JSValue) *jsc.JSObject {
	arr := jsc.NewArray(in.ObjectPrototype(), rects)
	arr.Set("length", jsc.NumberValue(float64(len(rects))))
	arr.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.Null()
			}
			i := int(a[0].ToNumber())
			if i < 0 || i >= len(rects) {
				return jsc.Null()
			}
			return rects[i]
		}, 1)))
	domAttachProto(arr, "DOMRectList")
	return arr
}

// serializeNodeXML 序列化节点为 XML/HTML 文本（XMLSerializer.serializeToString）。
// 复用 dom.Element.GetOuterHTML 的同一序列化器，保证与 innerHTML/outerHTML 一致。
func serializeNodeXML(n dom.Node) string {
	switch v := n.(type) {
	case nil:
		return ""
	case *dom.Element:
		return v.GetOuterHTML()
	case *dom.Text:
		return escapeXMLText(v.TextContent())
	case *dom.Comment:
		return "<!--" + v.TextContent() + "-->"
	case *dom.CDATASection:
		return "<![CDATA[" + v.Data() + "]]>"
	case *dom.ProcessingInstruction:
		return "<?" + v.Target() + " " + v.Data() + "?>"
	case *dom.DocumentType:
		s := "<!DOCTYPE " + v.Name()
		if pub := v.PublicID(); pub != "" {
			s += " PUBLIC \"" + pub + "\""
		}
		if sys := v.SystemID(); sys != "" {
			s += " \"" + sys + "\""
		}
		return s + ">"
	case *dom.Document:
		var b strings.Builder
		for _, c := range v.ChildNodes() {
			b.WriteString(serializeNodeXML(c))
		}
		return b.String()
	case *dom.DocumentFragment:
		var b strings.Builder
		for _, c := range v.ChildNodes() {
			b.WriteString(serializeNodeXML(c))
		}
		return b.String()
	}
	return ""
}

// escapeXMLText 转义文本节点的 XML 特殊字符（& < >）。
func escapeXMLText(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// ─── 补充全局（core 组达标所需的最小 JS/Window 面）──────────────────

// registerWindowIdentity 补齐 Window 的身份与几何属性（core 组）。
//
// 依据：HTML §7.2 Window 接口。top/parent/frames 在无 iframe 场景下恒为
// window 自身；length 是子窗口数（本引擎无嵌套浏览上下文 → 0）；name/origin/
// isSecureContext/scrollX/scrollY/pageXOffset/pageYOffset/outerWidth/outerHeight
// 是标准属性。缺这些会让库的特性检测（如 `window.parent !== window` 判 iframe、
// `window.scrollY` 读滚动位置）走错分支。
func registerWindowIdentity(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	win := jsc.ObjectValue(g)
	g.Set("window", win)
	g.Set("self", win)
	g.Set("top", win)
	g.Set("parent", win)
	g.Set("frames", win)
	g.Set("length", jsc.NumberValue(0))
	if _, ok := g.GetByKey("name"); !ok {
		g.Set("name", jsc.StringValue(""))
	}
	origin := ""
	if lv := g.GetStr("location"); lv.IsObject() {
		if lo := lv.AsObject(); lo != nil {
			if ov, ok := lo.GetByKey("origin"); ok && !ov.IsUndefined() {
				origin = ov.ToString()
			}
		}
	}
	if _, ok := g.GetByKey("origin"); !ok {
		g.Set("origin", jsc.StringValue(origin))
	}
	if _, ok := g.GetByKey("isSecureContext"); !ok {
		g.Set("isSecureContext", jsc.BooleanValue(false))
	}
	// 滚动量：本引擎无嵌套滚动容器 → 0（与 HTML §7.2 初始值一致）。
	for _, n := range []string{"scrollX", "scrollY", "pageXOffset", "pageYOffset"} {
		if _, ok := g.GetByKey(n); !ok {
			g.Set(n, jsc.NumberValue(0))
		}
	}
	// 外框尺寸 ≈ 内部尺寸（无窗口装饰信息；与 innerWidth/innerHeight 一致是最
	// 保守的选择：Web 应用按 outerWidth 估滚动条宽度时得到 0，不会误判）。
	inner := func(name string) float64 {
		if v, ok := g.GetByKey(name); ok && !v.IsUndefined() {
			return v.ToNumber()
		}
		return 0
	}
	if _, ok := g.GetByKey("outerWidth"); !ok {
		g.Set("outerWidth", jsc.NumberValue(inner("innerWidth")))
	}
	if _, ok := g.GetByKey("outerHeight"); !ok {
		g.Set("outerHeight", jsc.NumberValue(inner("innerHeight")))
	}
}

// registerBase64Globals 注册 atob/btoa（HTML §8.2 Base64 实用函数，core 组）。
//
// 与浏览器一致的宽容度：btoa 只接受 Latin-1（码点 ≤ 0xFF），超范围按字节截断
// （浏览器抛 InvalidCharacterError；本引擎不抛异常以保持既有脚本不中断，
// 见 WORKITEMS §19-5 遗留）；atob 忽略空白并按标准/URL-safe 两种字母表解码。
func registerBase64Globals(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	g.Set("btoa", jsc.FunctionValue(jsc.NewNativeFunction("btoa",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.StringValue("")
			}
			s := a[0].ToString()
			buf := make([]byte, 0, len(s))
			for _, r := range s {
				if r > 0xFF {
					buf = append(buf, byte(r&0xFF))
					continue
				}
				buf = append(buf, byte(r))
			}
			return jsc.StringValue(base64.StdEncoding.EncodeToString(buf))
		}, 1)))
	g.Set("atob", jsc.FunctionValue(jsc.NewNativeFunction("atob",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.StringValue("")
			}
			enc := a[0].ToString()
			// 去掉空白（HTML 规范允许）。
			enc = strings.Map(func(r rune) rune {
				switch r {
				case ' ', '\t', '\n', '\r', '\f':
					return -1
				}
				return r
			}, enc)
			dec, err := base64.StdEncoding.DecodeString(enc)
			if err != nil {
				dec, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(enc, "="))
			}
			if err != nil {
				dec, err = base64.URLEncoding.DecodeString(enc)
			}
			if err != nil {
				return jsc.StringValue("")
			}
			// 解码结果为 Latin-1 码元序列（每个字节一个码点）。
			var b strings.Builder
			for _, by := range dec {
				b.WriteRune(rune(by))
			}
			return jsc.StringValue(b.String())
		}, 1)))
}

// registerExtraElementCtors 注册 Audio / Option（HTML §4.8.11 / §4.10.10）：
// 二者是「返回元素」的构造器函数，浏览器里 `new Audio()` 得到 <audio>、
// `new Option(text, value)` 得到 <option>。前端库（音视频、表单动态选项）直接
// 依赖这一形态。
func registerExtraElementCtors(rt *jsc.Interpreter, g *jsc.JSObject, doc *dom.Document) {
	if rt == nil || g == nil || doc == nil {
		return
	}
	if _, ok := g.GetByKey("Audio"); !ok {
		audioCtor := rt.NewConstructor("Audio", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			el := doc.CreateElement("audio")
			if len(args) >= 1 && args[0].ToString() != "" {
				src := args[0].ToString()
				el.SetAttribute("src", src)
			}
			return wrapElement(in, el)
		})
		g.Set("Audio", jsc.FunctionValue(audioCtor))
	}
	if _, ok := g.GetByKey("Option"); !ok {
		optCtor := rt.NewConstructor("Option", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			el := doc.CreateElement("option")
			if len(args) >= 1 {
				_ = el.AppendChild(doc.CreateTextNode(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].ToString() != "" {
				el.SetAttribute("value", args[1].ToString())
			}
			return wrapElement(in, el)
		})
		g.Set("Option", jsc.FunctionValue(optCtor))
	}
}

// domIndexKey 是集合索引的字符串形式（供 makeDOMRectList 等复用）。
func domIndexKey(i int) string { return strconv.Itoa(i) }

// ─── CSS OM：规则对象（cssRules[i] / item(i) 的返回类型）─────────────

// cssRuleIfaceName 把引擎 css 包的规则类型映射到 CSSOM 接口名（CSSOM §1.4；
// 与 Chromium 的规则类一一对应）。CSS Nesting 的 @layer / @container / @scope /
// @starting-style 与 @counter-style 在 Chromium 里归 CSSGroupingRule（或各自
// 子类，本引擎未单独建模）→ 统一映射到 CSSGroupingRule。
func cssRuleIfaceName(t css.RuleType) string {
	switch t {
	case css.RuleStyle:
		return "CSSStyleRule"
	case css.RuleMedia:
		return "CSSMediaRule"
	case css.RuleSupports:
		return "CSSSupportsRule"
	case css.RuleFontFace:
		return "CSSFontFaceRule"
	case css.RuleKeyframes:
		return "CSSKeyframesRule"
	case css.RuleKeyframe:
		return "CSSKeyframeRule"
	case css.RuleImport:
		return "CSSImportRule"
	case css.RuleNamespace:
		return "CSSNamespaceRule"
	case css.RulePage:
		return "CSSPageRule"
	case css.RuleLayerBlock, css.RuleLayerStatement, css.RuleContainer,
		css.RuleScope, css.RuleStartingStyle, css.RuleCounterStyle:
		return "CSSGroupingRule"
	}
	return "CSSRule"
}

// cssRuleTypeCode 返回 CSSOM 的 CSSRule.type 编号（Chromium 口径：STYLE_RULE=1、
// IMPORT_RULE=3、MEDIA_RULE=4、FONT_FACE_RULE=5、PAGE_RULE=6、KEYFRAMES_RULE=7、
// KEYFRAME_RULE=8、NAMESPACE_RULE=10、SUPPORTS_RULE=12）。未编号的规则类型
// （@layer 等）返回 0 —— 与 Chromium 对无编号规则的表现一致。
func cssRuleTypeCode(t css.RuleType) int {
	switch t {
	case css.RuleStyle:
		return 1
	case css.RuleImport:
		return 3
	case css.RuleMedia:
		return 4
	case css.RuleFontFace:
		return 5
	case css.RulePage:
		return 6
	case css.RuleKeyframes:
		return 7
	case css.RuleKeyframe:
		return 8
	case css.RuleNamespace:
		return 10
	case css.RuleSupports:
		return 12
	}
	return 0
}

// wrapCSSRule 把引擎的样式规则包装成 CSSRule 家族对象（`styleSheet.cssRules[i]`
// 与 `cssRules.item(i)` 的返回类型）。原型按规则类型分派，使
// `rule instanceof CSSStyleRule`、`rule.constructor.name === "CSSStyleRule"` 成立。
//
// ★ 第 21 轮：字段面（第 20 轮只暴露了 `type` 与对象身份）。按 Edge 实测基线：
//
//	CSSStyleRule.selectorText = "#d" / ".b, #c"
//	CSSStyleRule.cssText      = "#d { color: rgb(1, 2, 3); font-size: 15px; }"
//	CSSStyleRule.style        = CSSStyleDeclaration（getPropertyValue / cssText /
//	                            length / item 可用，属性读写 kebab 与 camelCase 双键）
//	CSSMediaRule / CSSSupportsRule：conditionText + cssRules（嵌套）+ 多行 cssText
//	                            （Edge 实测 "@media (min-width: 1px) {\n  .b { … }\n}"）
//
// ★ 第 22 轮（以 Edge --dump-dom 基线为准补齐 —— 证据 dev/output/wbui-audit/r22base.edge.txt）：
//
//	parentRule / parentStyleSheet  所有规则可用（顶层规则的 parentRule 为 null）
//	CSSImportRule    href / media（MediaList：mediaText / length / item）/ cssText
//	CSSFontFaceRule  style / cssText
//	CSSKeyframesRule name / cssRules（子项 CSSKeyframeRule：style / cssText）/ cssText
//	CSSPageRule      selectorText / style / cssText
//	CSSMediaRule     media（MediaList）
//
// @namespace 仍只有 type —— 如实记账（WORKITEMS §22；不在本轮范围，未取基线不宣称）。
func wrapCSSRule(in *jsc.Interpreter, r css.Rule, parent *jsc.JSObject, parentSheet *jsc.JSObject) *jsc.JSObject {
	name := "CSSRule"
	t := css.RuleUnknown
	if r != nil {
		t = r.Type()
		name = cssRuleIfaceName(t)
	}
	obj := jsc.NewObject(domIfaceProtoOr(name, in.ObjectPrototype()))
	obj.SetClassName(name)
	obj.SetInternal(r)
	obj.SetAccessor("type", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(cssRuleTypeCode(t)))
	}), nil)
	// ★ 第 22 轮：parentRule / parentStyleSheet。Edge 实测：顶层规则的 parentRule
	//   为 null，嵌套规则为**父规则的包装对象**；parentStyleSheet 是该规则所属的
	//   CSSStyleSheet 包装对象（与 `document.styleSheets[i]` 同一身份）。
	obj.SetAccessor("parentRule", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if parent == nil {
			return jsc.Null()
		}
		return jsc.ObjectValue(parent)
	}), nil)
	obj.SetAccessor("parentStyleSheet", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if parentSheet == nil {
			return jsc.Null()
		}
		return jsc.ObjectValue(parentSheet)
	}), nil)
	switch v := r.(type) {
	case *css.StyleRule:
		sel := selectorTextOf(v.Selectors)
		obj.SetAccessor("selectorText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(sel)
		}), nil)
		setStyleAccessor(obj, v.Declarations)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssRuleTextOf(v))
		}), nil)
		// CSSGroupingRule（CSS Nesting）：子规则列表（无嵌套时长度为 0）。
		obj.SetAccessor("cssRules", getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(wrapCSSRuleList(in, v.NestedRules, obj, parentSheet))
		}), nil)
	case *css.MediaRule:
		// conditionText 按浏览器序列化规则规范化（见 normalizeConditionText）。
		cond := normalizeConditionText(v.Condition)
		obj.SetAccessor("conditionText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cond)
		}), nil)
		// ★ 第 22 轮：media（MediaList）—— Edge 实测 `cssRules[i].media` 是对象，
		//   其 mediaText 即规范化后的条件文本。
		mediaObj := mediaListObj(in, cond)
		obj.SetAccessor("media", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(mediaObj)
		}), nil)
		obj.SetAccessor("cssRules", getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(wrapCSSRuleList(in, v.Rules, obj, parentSheet))
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssRuleTextOf(v))
		}), nil)
	case *css.SupportsRule:
		// 与 @media 同一规范化。★ 第 22 轮补 Edge 基线覆盖（WORKITEMS §22）：
		// `@supports (display: grid) and (not (display: inline-grid))` 的
		// conditionText 两侧一致（r22base.edge.txt）。
		cond := normalizeConditionText(v.Condition)
		obj.SetAccessor("conditionText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cond)
		}), nil)
		obj.SetAccessor("cssRules", getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(wrapCSSRuleList(in, v.Rules, obj, parentSheet))
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssRuleTextOf(v))
		}), nil)
	case *css.ImportRule:
		obj.SetAccessor("href", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(v.Href)
		}), nil)
		mediaObj := mediaListObj(in, strings.TrimSpace(v.Media))
		obj.SetAccessor("media", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(mediaObj)
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssImportTextOf(v))
		}), nil)
	case *css.FontFaceRule:
		setStyleAccessor(obj, v.Declarations)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue("@font-face { " + cssDeclsText(v.Declarations) + " }")
		}), nil)
	case *css.KeyframesRule:
		kfName := v.Name
		obj.SetAccessor("name", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(kfName)
		}), nil)
		obj.SetAccessor("cssRules", getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(wrapKeyframeList(in, v.Keyframes, obj, parentSheet))
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssKeyframesTextOf(v))
		}), nil)
	case *css.PageRule:
		sel := v.Selector
		obj.SetAccessor("selectorText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(sel)
		}), nil)
		setStyleAccessor(obj, v.Declarations)
		// Edge 实测（r22base：r5_nested_len=0）：CSSPageRule 继承 CSSGroupingRule，
		// cssRules 存在但恒为空列表。
		obj.SetAccessor("cssRules", getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(newRuleListObj(in, 0, nil))
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(pageRuleTextOf(v))
		}), nil)
	case *css.NamespaceRule:
		// ★ 第 23 轮（Edge 基线 dev/output/wbui-audit/r23base.edge.txt）：
		//   CSSNamespaceRule 暴露 prefix / namespaceURI / cssText。
		//   ★ href **不实现**：Edge 实测 `typeof rule.href === "undefined"` ——
		//   该接口本就没有 href 属性（监督者列出的 href 经基线核对不存在），
		//   故不在对象上定义它，读出来即 undefined，与 Edge 一致。
		nsPrefix, nsURI := v.Prefix, v.URI
		obj.SetAccessor("prefix", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(nsPrefix)
		}), nil)
		obj.SetAccessor("namespaceURI", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(nsURI)
		}), nil)
		obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(cssNamespaceTextOf(nsPrefix, nsURI))
		}), nil)
	}
	return obj
}

// normalizeConditionText 按浏览器的 CSSOM 序列化规则规范化条件文本
// （@media / @supports 的 conditionText）：
//
//	"( min-width :  1px )" → "(min-width: 1px)"
//
// 规则：折叠连续空白、去掉左括号后 / 右括号前 / 冒号前的空白、冒号后恰一个空格、
// 逗号后恰一个空格。
//
// 依据（Edge 实测，dev/output/tmp/r21edge2.txt）：`@media ( min-width :  1px )`
// 的 conditionText 为 "(min-width: 1px)"；而引擎 css 包保存的是**声明原文**
// （"( min-width :  1px )"）→ 不规范化会与浏览器不一致。
// 只在**展示层**（本 getter）规范化，不动 css 包的解析结果（条件匹配用的是
// 解析后的 MediaQuery 结构，不读这段文本）。
func normalizeConditionText(s string) string {
	out := strings.Join(strings.Fields(s), " ")
	if out == "" {
		return out
	}
	out = strings.ReplaceAll(out, "( ", "(")
	out = strings.ReplaceAll(out, " )", ")")
	out = strings.ReplaceAll(out, " :", ":")
	out = strings.ReplaceAll(out, ":", ": ")
	out = strings.ReplaceAll(out, ":  ", ": ")
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, ",", ", ")
	out = strings.ReplaceAll(out, ",  ", ", ")
	return strings.TrimSpace(out)
}

// selectorTextOf 返回选择器列表的 CSSOM 文本（无选择器时 ""）。
func selectorTextOf(l *css.SelectorList) string {
	if l == nil {
		return ""
	}
	return l.String()
}

// cssDeclsText 序列化声明列表为 CSSOM 文本：分号 + 空格分隔、末尾带分号
// （Edge 实测 "color: rgb(1, 2, 3); font-size: 15px;"；空列表为 ""）。
// ★ 第 22 轮：走 Declaration.CSSText（CSSOM 口径 —— url 带引号、font-family
// 去引号），而不是引擎内部的 Declaration.String。
func cssDeclsText(decls []css.Declaration) string {
	if len(decls) == 0 {
		return ""
	}
	parts := make([]string, 0, len(decls))
	for _, d := range decls {
		parts = append(parts, d.CSSText())
	}
	return strings.Join(parts, "; ") + ";"
}

// cssRuleTextOf 序列化单条规则为 CSSOM 文本（规则自身与分组规则的子规则共用）。
func cssRuleTextOf(r css.Rule) string {
	switch v := r.(type) {
	case *css.StyleRule:
		return selectorTextOf(v.Selectors) + " { " + cssDeclsText(v.Declarations) + " }"
	case *css.MediaRule:
		return conditionRuleTextOf("@media "+normalizeConditionText(v.Condition), v.Rules)
	case *css.SupportsRule:
		return conditionRuleTextOf("@supports "+normalizeConditionText(v.Condition), v.Rules)
	case *css.ImportRule:
		return cssImportTextOf(v)
	case *css.FontFaceRule:
		return "@font-face { " + cssDeclsText(v.Declarations) + " }"
	case *css.KeyframesRule:
		return cssKeyframesTextOf(v)
	case *css.PageRule:
		return pageRuleTextOf(v)
	}
	return ""
}

// cssImportTextOf 序列化 @import。
// ★ Edge 实测（r22base.edge.txt）：href 一律包在 url("…") 里，media 非空时以空格
// 接上，行尾分号 —— `@import url("probe-import.css") screen;`
func cssImportTextOf(v *css.ImportRule) string {
	s := `@import url("` + v.Href + `")`
	if m := strings.TrimSpace(v.Media); m != "" {
		s += " " + m
	}
	return s + ";"
}

// pageRuleTextOf 序列化 @page（Edge 实测 `@page :first { margin: 1cm; }`；
// 选择器为空时退化为 `@page { … }`）。
func pageRuleTextOf(v *css.PageRule) string {
	head := "@page"
	if sel := strings.TrimSpace(v.Selector); sel != "" {
		head += " " + sel
	}
	return head + " { " + cssDeclsText(v.Declarations) + " }"
}

// cssKeyframesTextOf 序列化 @keyframes。
// ★ Edge 实测格式（r22base.edge.txt）：首行 `@keyframes spin { ` **带一个尾随
// 空格**，每条关键帧缩进 2 空格、各占一行，收尾 `}` 独占一行。
func cssKeyframesTextOf(v *css.KeyframesRule) string {
	var b strings.Builder
	b.WriteString("@keyframes " + v.Name + " { \n")
	for i := range v.Keyframes {
		b.WriteString("  " + cssKeyframeTextOf(&v.Keyframes[i]) + "\n")
	}
	b.WriteString("}")
	return b.String()
}

// cssKeyframeTextOf 序列化单条关键帧（Edge 实测 `0% { opacity: 0; }`；
// 一条规则带多个键时以 ", " 连接）。
// ★ Edge 会把关键字键规范化：`from` → `0%`、`to` → `100%`（实测 r3_0_cssText /
// r3_1_cssText 用的是原文 from/to，而 Edge 输出 0%/100%）。引擎 AST 保留原文
// （Keys 供动画匹配用），故只在**序列化层**做换算。
func cssKeyframeTextOf(k *css.KeyframeRule) string {
	keys := make([]string, 0, len(k.Keys))
	for _, key := range k.Keys {
		keys = append(keys, normalizeKeyframeKey(key))
	}
	return strings.Join(keys, ", ") + " { " + cssDeclsText(k.Declarations) + " }"
}

// normalizeKeyframeKey 规范化单个关键帧键：`from` → `0%`、`to` → `100%`。
// 引擎 AST 保留原文（Keys 供动画匹配用），换算只发生在**序列化层**。
func normalizeKeyframeKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "from":
		return "0%"
	case "to":
		return "100%"
	}
	return strings.TrimSpace(key)
}

// cssKeyframeKeyTextOf 返回 CSSKeyframeRule.keyText（★ 第 23 轮，Edge 基线
// dev/output/wbui-audit/r23b.edge.txt）：
//
//	@keyframes { from { … }          → keyText = "0%"
//	@keyframes { from, 50% { … }     → keyText = "0%, 50%"
func cssKeyframeKeyTextOf(k *css.KeyframeRule) string {
	keys := make([]string, 0, len(k.Keys))
	for _, key := range k.Keys {
		keys = append(keys, normalizeKeyframeKey(key))
	}
	return strings.Join(keys, ", ")
}

// cssNamespaceTextOf 序列化 @namespace 规则（★ 第 23 轮，Edge 基线
// dev/output/wbui-audit/r23base.edge.txt）：
//
//	@namespace url("http://www.w3.org/1999/xhtml");    （无前缀 → prefix = ""）
//	@namespace svg url("http://www.w3.org/2000/svg");  （有前缀）
//
// url 恒带**双引号**、末尾带分号。
func cssNamespaceTextOf(prefix, uri string) string {
	if prefix != "" {
		return "@namespace " + prefix + " url(\"" + uri + "\");"
	}
	return "@namespace url(\"" + uri + "\");"
}

// conditionRuleTextOf 构造分组规则（@media / @supports）的多行 cssext 文本
// （Edge 实测：子规则缩进 2 空格、花括号独占行）。
func conditionRuleTextOf(head string, rules []css.Rule) string {
	var b strings.Builder
	b.WriteString(head)
	b.WriteString(" {\n")
	for _, r := range rules {
		b.WriteString("  ")
		b.WriteString(cssRuleTextOf(r))
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String()
}

// styleDeclObj 把规则的声明列表暴露成 CSSStyleDeclaration 实例
// （`styleSheet.cssRules[i].style`，即 CSS-in-JS / 主题探测最常用的路径）。
//
// 与浏览器一致的契约：原型 + constructor.name + instanceof、属性访问
// （kebab-case 与 camelCase 两种键都能读到值）、getPropertyValue、
// getPropertyPriority、item(i)、length、cssText。
//
// ★ 如实记账（WORKITEMS §21 遗留）：这是**只读快照** —— 通过它写入
// （setProperty / 属性赋值）不会回改样式表与渲染（浏览器会），故未提供写方法。
func styleDeclObj(in *jsc.Interpreter, decls []css.Declaration) jsc.JSValue {
	obj := jsc.NewObject(domIfaceProtoOr("CSSStyleDeclaration", in.ObjectPrototype()))
	obj.SetClassName("CSSStyleDeclaration")
	for i := range decls {
		key := strings.ToLower(strings.TrimSpace(decls[i].Name))
		if key == "" {
			continue
		}
		// ★ 第 22 轮：CSSOM 口径取值（url 带引号 / font-family 去引号）。
		val := strings.TrimSpace(decls[i].CSSTextValue())
		obj.Set(key, jsc.StringValue(val))
		if camel := kebabToCamel(key); camel != key {
			obj.Set(camel, jsc.StringValue(val))
		}
	}
	obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(cssDeclsText(decls))
	}), nil)
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(decls)))
	}), nil)
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.StringValue("")
			}
			i := int(a[0].ToNumber())
			if i < 0 || i >= len(decls) {
				return jsc.StringValue("")
			}
			return jsc.StringValue(strings.ToLower(strings.TrimSpace(decls[i].Name)))
		}, 1)))
	// declByName 按 CSSOM 语义查找声明：属性名大小写不敏感、camelCase 先换算。
	declByName := func(name string) (int, bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return -1, false
		}
		want := strings.ToLower(camelToKebab(name))
		for i := range decls {
			if strings.ToLower(strings.TrimSpace(decls[i].Name)) == want {
				return i, true
			}
		}
		return -1, false
	}
	obj.Set("getPropertyValue", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyValue",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.StringValue("")
			}
			if i, ok := declByName(a[0].ToString()); ok {
				return jsc.StringValue(strings.TrimSpace(decls[i].CSSTextValue()))
			}
			return jsc.StringValue("")
		}, 1)))
	obj.Set("getPropertyPriority", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyPriority",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.StringValue("")
			}
			if i, ok := declByName(a[0].ToString()); ok && decls[i].Important {
				return jsc.StringValue("important")
			}
			return jsc.StringValue("")
		}, 1)))
	return jsc.ObjectValue(obj)
}

// wrapCSSRuleList 把规则数组包装成 CSSRuleList（CSSOM §1.4）：length + item(i) +
// 索引属性；索引与 item(i) 返回**同一身份**的规则对象（浏览器里 CSSRuleList 是
// legacy platform object，`list[0] === list.item(0)`）。
//
// ★ 第 22 轮：parent / parentSheet 透传给每条规则（规则对象上的 parentRule /
// parentStyleSheet；嵌套规则的 parent 即本列表所属的**规则包装对象**）。
func wrapCSSRuleList(in *jsc.Interpreter, rules []css.Rule, parent *jsc.JSObject, parentSheet *jsc.JSObject) *jsc.JSObject {
	return newRuleListObj(in, len(rules), func(i int) *jsc.JSObject {
		return wrapCSSRule(in, rules[i], parent, parentSheet)
	})
}

// wrapKeyframeList 把 @keyframes 的关键帧数组包装成 CSSRuleList（子项是
// CSSKeyframeRule —— 它不在 css.Rule 接口里，故单独走 wrapKeyframeRule）。
func wrapKeyframeList(in *jsc.Interpreter, kfs []css.KeyframeRule, parent *jsc.JSObject, parentSheet *jsc.JSObject) *jsc.JSObject {
	return newRuleListObj(in, len(kfs), func(i int) *jsc.JSObject {
		return wrapKeyframeRule(in, &kfs[i], parent, parentSheet)
	})
}

// wrapKeyframeRule 包装单条 CSSKeyframeRule（type = 8 —— Edge 实测 r3_0_type=8）。
// 字段面：type / parentRule / parentStyleSheet / style（CSSStyleDeclaration）/
// cssText（`0% { opacity: 0; }`）。keyText 未实现（未取基线，不宣称）。
func wrapKeyframeRule(in *jsc.Interpreter, k *css.KeyframeRule, parent *jsc.JSObject, parentSheet *jsc.JSObject) *jsc.JSObject {
	obj := jsc.NewObject(domIfaceProtoOr("CSSKeyframeRule", in.ObjectPrototype()))
	obj.SetClassName("CSSKeyframeRule")
	obj.SetInternal(k)
	obj.SetAccessor("type", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(cssRuleTypeCode(css.RuleKeyframe)))
	}), nil)
	obj.SetAccessor("parentRule", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if parent == nil {
			return jsc.Null()
		}
		return jsc.ObjectValue(parent)
	}), nil)
	obj.SetAccessor("parentStyleSheet", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if parentSheet == nil {
			return jsc.Null()
		}
		return jsc.ObjectValue(parentSheet)
	}), nil)
	setStyleAccessor(obj, k.Declarations)
	// ★ 第 23 轮：CSSKeyframeRule.keyText（Edge 基线 r23b.edge.txt：单键
	//   `0%` / `100%`；一条规则带多个键时以 ", " 连接 —— `from, 50%` → `"0%, 50%"`）。
	obj.SetAccessor("keyText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(cssKeyframeKeyTextOf(k))
	}), nil)
	obj.SetAccessor("cssText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(cssKeyframeTextOf(k))
	}), nil)
	return obj
}

// newRuleListObj 构造 CSSRuleList 风格对象：length + item(i) + 索引属性，且索引与
// item(i) 返回**同一身份**（legacy platform object 语义）。at(i) 由调用方提供 ——
// 普通规则（wrapCSSRule）与关键帧（wrapKeyframeRule）两条路径共用。
func newRuleListObj(in *jsc.Interpreter, n int, at func(int) *jsc.JSObject) *jsc.JSObject {
	ro := jsc.NewObject(domIfaceProtoOr("CSSRuleList", in.ObjectPrototype()))
	ro.SetClassName("CSSRuleList")
	objs := make([]*jsc.JSObject, n)
	for i := 0; i < n; i++ {
		objs[i] = at(i)
		ro.Set(strconv.Itoa(i), jsc.ObjectValue(objs[i]))
	}
	ro.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(objs)))
	}), nil)
	ro.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.Null()
			}
			i := int(a[0].ToNumber())
			if i < 0 || i >= len(objs) {
				return jsc.Null()
			}
			return jsc.ObjectValue(objs[i])
		}, 1)))
	return ro
}

// setStyleAccessor 在规则包装对象上安装 `style`（CSSStyleDeclaration）。
// ★ 身份稳定（Edge 实测 `rule.style === rule.style` 为 true）：缓存挂在**规则包装
// 对象的闭包**（per-runtime）—— CSSStyleRule / CSSFontFaceRule / CSSPageRule /
// CSSKeyframeRule 四条路径共用同一实现。
func setStyleAccessor(obj *jsc.JSObject, decls []css.Declaration) {
	var styleObj jsc.JSValue
	haveStyle := false
	obj.SetAccessor("style", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if haveStyle {
			return styleObj
		}
		styleObj = styleDeclObj(in, decls)
		haveStyle = true
		return styleObj
	}), nil)
}

// mediaListObj 构造 MediaList。Edge 实测：CSSMediaRule.media 与 CSSImportRule.media
// 都是**对象**（不是字符串），其 mediaText 为逗号分隔的条件文本，另有 length /
// item(i)。为遵守第 22 轮「不新增构造器」约束，这里只造对象 + className，
// 不注册 MediaList 原型（夹具据此只断言 typeof 与 mediaText）。
func mediaListObj(in *jsc.Interpreter, text string) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("MediaList")
	text = strings.TrimSpace(text)
	obj.SetAccessor("mediaText", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(text)
	}), nil)
	var items []string
	if text != "" {
		for _, part := range strings.Split(text, ",") {
			if p := strings.TrimSpace(part); p != "" {
				items = append(items, p)
			}
		}
	}
	for i := range items {
		obj.Set(strconv.Itoa(i), jsc.StringValue(items[i]))
	}
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(items)))
	}), nil)
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.Null()
			}
			i := int(a[0].ToNumber())
			if i < 0 || i >= len(items) {
				return jsc.Null()
			}
			return jsc.StringValue(items[i])
		}, 1)))
	return obj
}
