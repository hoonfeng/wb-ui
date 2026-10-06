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
	domAdoptIface(g, "EventTarget", nil)
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
