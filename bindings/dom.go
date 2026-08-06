// Package bindings implements Go <-> JS <-> DOM bridges for wb-ui.
// Completeness: 70% — adds full style/classList/traversal/event for SPA support.
package bindings

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"wb-ui.com/goja"
	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/jsc"
)

// OnStyleNodeAdded is an optional callback invoked when a <style> element is
// dynamically added to the DOM (via appendChild/insertBefore). The bindings
// set this from webkit.WebView so the frame can re-extract and apply the new
// styles. When nil, dynamic <style> injection is silently ignored.
var OnStyleNodeAdded func(node dom.Node)

// OnInlineStyleChanged is an optional callback invoked when an element's
// style attribute is changed via the JS style proxy (el.style.xxx = ...).
// The embedder should re-resolve styles and rebuild the render tree.
var OnInlineStyleChanged func(node dom.Node)

// OnNodeInserted is an optional callback invoked when a DOM node is added to
// the document (via appendChild / insertBefore / replaceChild). The embedder
// should rebuild the render tree so the new node appears in layout/paint.
var OnNodeInserted func(node dom.Node)

// OnNodeRemoved is an optional callback invoked when a DOM node is removed
// from the document (via removeChild / replaceChild). The embedder should
// rebuild the render tree.
var OnNodeRemoved func(node dom.Node)

// ── 渲染树桥（由 webkit.WebView 注入）──
// Element 的滚动/尺寸 CSSOM 属性（scrollTop/scrollHeight/clientHeight/
// offsetHeight 等）需要真实布局几何。bindings 不直接依赖 rendering 包
// （避免耦合），改为回调注入：webkit.WebView 在注册 DOM bindings 时设置
// 这些函数，wrapElement 的 accessor 通过它们读取/写入渲染树。
var (
	// GetElementScrollOffset 返回元素当前滚动偏移 (x, y)；无渲染盒返回 0,0。
	GetElementScrollOffset func(el *dom.Element) (x, y float64)
	// SetElementScrollOffset 写入元素滚动偏移。非滚动容器或越界由实现方
	// 忽略/钳制（浏览器语义：非 overflow 容器 scrollTop 赋值无效）。
	SetElementScrollOffset func(el *dom.Element, x, y float64)
	// GetElementScrollMetrics 返回 (viewW, viewH, totalW, totalH, scrollable)：
	// view* 为 padding-box 尺寸（clientWidth/clientHeight），
	// total* 为内容包围盒尺寸（scrollWidth/scrollHeight）。
	GetElementScrollMetrics func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool)
	// GetElementBoxRect 返回元素布局盒 (left, top, width, height)
	// （offsetLeft/offsetTop/offsetWidth/offsetHeight 用）。
	GetElementBoxRect func(el *dom.Element) (left, top, width, height float64)
)

// MediaQueryContextProvider 提供 matchMedia 评估所需的设备/视口上下文
// （视口尺寸、devicePixelRatio、颜色方案等）；由宿主（webkit.WebView）
// 在注册时注入真实值；nil 时 matchMedia 用默认值（1280×800、light）。
var MediaQueryContextProvider func() *css.MediaQueryContext

// windowEventListeners 存储 window 上的事件监听器（window.dispatchEvent
// 真分发用）。key 为事件类型字符串（如 "resize"、"message"、自定义事件）。
var windowEventListeners = map[string][]jsc.JSValue{}

// DOM prototype objects — set by RegisterDOMBindings, used by wrappers.
var (
	domElementProto *jsc.JSObject // Element.prototype
	domTextProto    *jsc.JSObject // Text.prototype
	domCommentProto *jsc.JSObject // Comment.prototype
	domDocFragProto *jsc.JSObject // DocumentFragment.prototype
)

// domBindingsMarker 是挂在 interpreter 全局对象上的隐藏标记，用于幂等判断：
// 同一 interpreter 的 RegisterDOMBindings 只完整注册一次（构造函数与
// prototype 链），后续调用仅刷新 document 引用。以 interpreter 为粒度，
// 避免多实例测试（rt1/rt2 各自完整注册）互相影响。
const domBindingsMarker = "\x00__wbui_dom_bindings_registered"

func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	// ★ 幂等注册（按 interpreter）：构造函数与 prototype 链只在首次
	// 调用时构建。后续调用（如 EvalJS 每次执行前）仅刷新 document 引用——
	// 若每次都重建 Element.prototype，JS 侧对 prototype 的 hook/修改
	// （Vue scoped data-v 探针、monkey-patch 等）会被新 prototype 静默
	// 覆盖，导致探针失效（与标准浏览器行为相悖）。
	g := rt.GlobalObject()
	registeredDocument = document

	// ── Selection 单例（提前创建）──────────────────────────────
	// ★ CodeMirror 6 等库依赖 document.getSelection()。幂等分支每次
	// EvalJS/RunJS 前都会新建 document 对象；若 getSelection 只在首次
	// 注册时挂到 docObj，刷新后的 document 将丢失该方法，CM6 初始化
	// 直接抛 "Object has no member 'getSelection'"。因此 Selection
	// 单例必须在幂等分支之前创建，且两处 docObj 都需挂 getSelection。
	type selState struct {
		ranges []*jsc.JSObject
	}
	var sstate = &selState{}
	selObj := jsc.NewObject(rt.ObjectPrototype())
	selObj.Set("rangeCount", jsc.NumberValue(0))
	selObj.Set("anchorNode", jsc.Null())
	selObj.Set("anchorOffset", jsc.NumberValue(0))
	selObj.Set("focusNode", jsc.Null())
	selObj.Set("focusOffset", jsc.NumberValue(0))
	selObj.Set("isCollapsed", jsc.BooleanValue(true))
	selObj.Set("type", jsc.StringValue("None"))

	if _, ok := g.GetByKey(domBindingsMarker); ok {
		docObj := wrapDocument(rt, document)
		docObj.Set("getSelection", jsc.FunctionValue(jsc.NewNativeFunction("getSelection",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.ObjectValue(selObj)
			}, 0)))
		g.Set("document", jsc.ObjectValue(docObj))
		return
	}

	docObj := wrapDocument(rt, document)
	rt.GlobalObject().Set("document", jsc.ObjectValue(docObj))

	// window / self / globalThis → 全局对象
	g.Set("window", jsc.ObjectValue(g))
	g.Set("self", jsc.ObjectValue(g))
	g.Set("globalThis", jsc.ObjectValue(g))

	// ── DOM Constructors (Go 原生) ─────────────────────────
	// Each constructor's .prototype is extracted and used by the
	// corresponding wrapper function so that `el instanceof Element`
	// and `txt instanceof Text` work correctly (real prototype chain).

	// Prototype objects — used by wrapElement / wrapText / etc.
	var nodeProto, elementProto, htmlElementProto, svgElementProto *jsc.JSObject
	var textProto, commentProto, docFragProto, attrProto *jsc.JSObject

	emptyCtor := func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		return this.AsObject()
	}

	nodeCtor := rt.NewConstructor("Node", emptyCtor)
	g.Set("Node", jsc.FunctionValue(nodeCtor))

	eltCtor := rt.NewConstructor("Element", emptyCtor)
	g.Set("Element", jsc.FunctionValue(eltCtor))

	htmlCtor := rt.NewConstructor("HTMLElement", emptyCtor)
	g.Set("HTMLElement", jsc.FunctionValue(htmlCtor))

	svgCtor := rt.NewConstructor("SVGElement", emptyCtor)
	g.Set("SVGElement", jsc.FunctionValue(svgCtor))

	textCtor := rt.NewConstructor("Text", emptyCtor)
	g.Set("Text", jsc.FunctionValue(textCtor))

	commentCtor := rt.NewConstructor("Comment", emptyCtor)
	g.Set("Comment", jsc.FunctionValue(commentCtor))

	fragCtor := rt.NewConstructor("DocumentFragment", emptyCtor)
	g.Set("DocumentFragment", jsc.FunctionValue(fragCtor))

	attrCtor := rt.NewConstructor("Attr", emptyCtor)
	g.Set("Attr", jsc.FunctionValue(attrCtor))

	// Extract .prototype objects
	nodeProto = jsc.FunctionValue(nodeCtor).AsObject().GetStr("prototype").AsObject()
	elementProto = jsc.FunctionValue(eltCtor).AsObject().GetStr("prototype").AsObject()
	htmlElementProto = jsc.FunctionValue(htmlCtor).AsObject().GetStr("prototype").AsObject()
	svgElementProto = jsc.FunctionValue(svgCtor).AsObject().GetStr("prototype").AsObject()
	textProto = jsc.FunctionValue(textCtor).AsObject().GetStr("prototype").AsObject()
	commentProto = jsc.FunctionValue(commentCtor).AsObject().GetStr("prototype").AsObject()
	docFragProto = jsc.FunctionValue(fragCtor).AsObject().GetStr("prototype").AsObject()
	attrProto = jsc.FunctionValue(attrCtor).AsObject().GetStr("prototype").AsObject()

	// Build prototype chain via __proto__ (goja supports __proto__).
	elementProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	htmlElementProto.Set("__proto__", jsc.ObjectValue(elementProto))
	svgElementProto.Set("__proto__", jsc.ObjectValue(elementProto))
	textProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	commentProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	docFragProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	attrProto.Set("__proto__", jsc.ObjectValue(nodeProto))

	// Store for wrapper functions (package-level).
	domElementProto = elementProto
	domTextProto = textProto
	domCommentProto = commentProto
	domDocFragProto = docFragProto

	// ── Element.prototype: attribute 方法（标准 DOM 设计）──
	// 属性方法定义在 prototype 上而非每个包装实例上：
	//   1. 符合 DOM 规范（Element.prototype.setAttribute 等，HTML/SVG/Element
	//      所有元素经原型链共享，且 Element.prototype.setAttribute 可访问）
	//   2. 实例包装更轻（每个元素少 5 个自有属性）
	//   3. 可被框架/探针在 Element.prototype 上 hook（如 Vue scoped 样式
	//      data-v 属性写入追踪、测试工具 monkey-patch）
	// native 函数经 this 取回包装对象的内部 *dom.Element。
	protoAttr := func(name string, argc int, fn func(el *dom.Element, args []jsc.JSValue) jsc.JSValue) {
		elementProto.Set(name, jsc.FunctionValue(jsc.NewNativeFunction(name,
			func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if !this.IsObject() {
					return jsc.Undefined()
				}
				el, ok := this.AsObject().Internal().(*dom.Element)
				if !ok || el == nil {
					return jsc.Undefined()
				}
				return fn(el, args)
			}, argc)))
	}
	protoAttr("getAttribute", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Null()
		}
		return jsc.StringValue(el.GetAttribute(args[0].ToString()))
	})
	protoAttr("setAttribute", 2, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		el.SetAttribute(args[0].ToString(), args[1].ToString())
		return jsc.Undefined()
	})
	protoAttr("hasAttribute", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.BooleanValue(false)
		}
		return jsc.BooleanValue(el.HasAttribute(args[0].ToString()))
	})
	protoAttr("removeAttribute", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Undefined()
		}
		el.RemoveAttribute(args[0].ToString())
		return jsc.Undefined()
	})
	protoAttr("toggleAttribute", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Undefined()
		}
		name := args[0].ToString()
		if el.HasAttribute(name) {
			el.RemoveAttribute(name)
			return jsc.BooleanValue(false)
		}
		el.SetAttribute(name, "")
		return jsc.BooleanValue(true)
	})

	// location 桩
	loc := jsc.NewObject(rt.ObjectPrototype())
	loc.Set("href", jsc.StringValue("about:blank"))
	loc.Set("origin", jsc.StringValue(""))
	loc.Set("hostname", jsc.StringValue(""))
	loc.Set("pathname", jsc.StringValue("/"))
	loc.Set("search", jsc.StringValue(""))
	loc.Set("hash", jsc.StringValue(""))
	loc.Set("protocol", jsc.StringValue("file:"))
	loc.Set("assign", jsc.FunctionValue(jsc.NewNativeFunction("assign",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	loc.Set("replace", jsc.FunctionValue(jsc.NewNativeFunction("replace",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	loc.Set("reload", jsc.FunctionValue(jsc.NewNativeFunction("reload",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	g.Set("location", jsc.ObjectValue(loc))

	// ─── Navigation State ───
	type navEntry struct {
		state map[string]interface{}
		title string
		url   string
	}
	navState := struct {
		entries      []navEntry
		index        int
		popListeners []struct {
			fn      jsc.JSValue
			capture bool
		}
	}{
		entries: []navEntry{{url: "/"}},
	}
	updateLocation := func(url string) {
		loc.Set("href", jsc.StringValue(url))
		if idx := strings.Index(url, "?"); idx >= 0 {
			loc.Set("pathname", jsc.StringValue(url[:idx]))
			loc.Set("search", jsc.StringValue(url[idx:]))
		} else if idx := strings.Index(url, "#"); idx >= 0 {
			loc.Set("pathname", jsc.StringValue(url[:idx]))
			loc.Set("hash", jsc.StringValue(url[idx:]))
		} else {
			loc.Set("pathname", jsc.StringValue(url))
			loc.Set("search", jsc.StringValue(""))
			loc.Set("hash", jsc.StringValue(""))
		}
	}

	// ─── window.history (real implementation) ───
	hist := jsc.NewObject(rt.ObjectPrototype())
	updateHistState := func() {
		if navState.index >= 0 && navState.index < len(navState.entries) {
			e := navState.entries[navState.index]
			if e.state != nil {
				hist.Set("state", jsc.StringValue(fmt.Sprintf("%v", e.state)))
			} else {
				hist.Set("state", jsc.Null())
			}
		}
		hist.Set("length", jsc.NumberValue(float64(len(navState.entries))))
	}
	dispatchPopstate := func() {
		if len(navState.popListeners) == 0 {
			return
		}
		stateVal := jsc.Null()
		if navState.index >= 0 && navState.index < len(navState.entries) && navState.entries[navState.index].state != nil {
			stateVal = jsc.StringValue(fmt.Sprintf("%v", navState.entries[navState.index].state))
		}
		for _, l := range navState.popListeners {
			ev := jsc.NewObject(rt.ObjectPrototype())
			ev.Set("type", jsc.StringValue("popstate"))
			ev.Set("state", stateVal)
			rt.Call(l.fn, jsc.Undefined(), []jsc.JSValue{jsc.ObjectValue(ev)})
		}
	}

	hist.Set("pushState", jsc.FunctionValue(jsc.NewNativeFunction("pushState",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			var state map[string]interface{}
			var urlStr string
			if len(args) >= 1 && args[0].IsObject() {
				state = make(map[string]interface{})
				if obj := args[0].AsObject(); obj != nil {
					for _, k := range obj.Keys() {
						if v, ok := obj.GetByKey(k); ok {
							state[k] = v.ToString()
						}
					}
				}
			}
			if len(args) >= 3 {
				urlStr = args[2].ToString()
			}
			navState.entries = navState.entries[:navState.index+1]
			navState.entries = append(navState.entries, navEntry{state: state, url: urlStr})
			navState.index = len(navState.entries) - 1
			updateHistState()
			if urlStr != "" {
				updateLocation(urlStr)
			}
			return jsc.Undefined()
		}, 3)))
	hist.Set("replaceState", jsc.FunctionValue(jsc.NewNativeFunction("replaceState",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if navState.index < 0 || navState.index >= len(navState.entries) {
				return jsc.Undefined()
			}
			if len(args) >= 1 && args[0].IsObject() {
				state := make(map[string]interface{})
				if obj := args[0].AsObject(); obj != nil {
					for _, k := range obj.Keys() {
						if v, ok := obj.GetByKey(k); ok {
							state[k] = v.ToString()
						}
					}
				}
				navState.entries[navState.index].state = state
			}
			if len(args) >= 3 {
				urlStr := args[2].ToString()
				navState.entries[navState.index].url = urlStr
				updateLocation(urlStr)
			}
			updateHistState()
			return jsc.Undefined()
		}, 3)))
	hist.Set("go", jsc.FunctionValue(jsc.NewNativeFunction("go",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			delta := 0
			if len(args) >= 1 && args[0].IsNumber() {
				delta = int(args[0].ToNumber())
			}
			newIdx := navState.index + delta
			if newIdx < 0 || newIdx >= len(navState.entries) {
				return jsc.Undefined()
			}
			navState.index = newIdx
			updateHistState()
			updateLocation(navState.entries[navState.index].url)
			dispatchPopstate()
			return jsc.Undefined()
		}, 1)))
	hist.Set("back", jsc.FunctionValue(jsc.NewNativeFunction("back",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if navState.index <= 0 {
				return jsc.Undefined()
			}
			navState.index--
			updateHistState()
			updateLocation(navState.entries[navState.index].url)
			dispatchPopstate()
			return jsc.Undefined()
		}, 0)))
	hist.Set("forward", jsc.FunctionValue(jsc.NewNativeFunction("forward",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if navState.index >= len(navState.entries)-1 {
				return jsc.Undefined()
			}
			navState.index++
			updateHistState()
			updateLocation(navState.entries[navState.index].url)
			dispatchPopstate()
			return jsc.Undefined()
		}, 0)))
	updateHistState()
	g.Set("history", jsc.ObjectValue(hist))

	// window.navigator 桩


	// window.navigator 桩
	nav := jsc.NewObject(rt.ObjectPrototype())
	nav.Set("userAgent", jsc.StringValue("wb-ui"))
	nav.Set("platform", jsc.StringValue("Go"))
	nav.Set("language", jsc.StringValue("zh-CN"))
	nav.Set("languages", jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.StringValue("zh-CN"), jsc.StringValue("en")})))
	g.Set("navigator", jsc.ObjectValue(nav))

	// window.screen 桩
	screen := jsc.NewObject(rt.ObjectPrototype())
	screen.Set("width", jsc.NumberValue(1280))
	screen.Set("height", jsc.NumberValue(800))
	g.Set("screen", jsc.ObjectValue(screen))

// window.console 由 SetupGlobal 设置

	// localStorage / sessionStorage（内存存储，对标浏览器）
	// 可通过 SetLocalStoragePersist 开启文件持久化（desktop 端重启不丢状态）。
	store := make(map[string]string)
	if localPersist != nil {
		for k, v := range localPersist.Load() {
			store[k] = v
		}
	}
	g.Set("localStorage", jsc.ObjectValue(makeStorage(rt, store, false)))
	g.Set("sessionStorage", jsc.ObjectValue(makeStorage(rt, store, true)))

	// performance.now — 返回毫秒级高精度时间戳
	g.Set("performance", jsc.ObjectValue(makePerformance(rt)))

	// getComputedStyle — 返回元素的级联计算样式对象。
	// 实现：遍历 document 内 <style> 样式表，用 css 包 SelectorChecker 匹配
	// 元素，按 specificity + 源顺序级联，再按标准优先级叠加 inline style 与
	// !important，最后把命中声明回写到对象（常用属性直接复制 + 统一
	// getPropertyValue 查询，kebab-case 键）。CSS 变量 --x 也参与级联，
	// TerminalPanel 等依赖 getPropertyValue('--x') 的 xterm 主题可拿到真实值。
	// ★ 绝不能返回 Null：依赖 getComputedStyle(el).getPropertyValue('--x')
	// 的代码会抛 "Value is not an object"，组件挂载中断 → Vue subTree.el 未设置
	// → 后续 patch 级联失败 → v-if 关闭不移除 DOM。
	g.Set("getComputedStyle", jsc.FunctionValue(jsc.NewNativeFunction("getComputedStyle",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			cs := jsc.NewObject(in.ObjectPrototype())
			var computed map[string]string
			if len(args) >= 1 {
				if n := unwrapNode(args[0]); n != nil {
					computed = computedStyleFor(n)
				}
			}
			if computed == nil {
				computed = map[string]string{}
			}
			// 常用属性直接回写到对象属性（camelCase，与浏览器一致）
			for _, prop := range []string{"color", "backgroundColor", "background", "fontFamily", "fontSize", "lineHeight", "fontWeight", "borderColor", "width", "height", "display", "position", "opacity", "visibility", "marginTop", "marginBottom", "paddingTop", "paddingBottom", "textAlign", "whiteSpace"} {
				key := prop
				if k := camelToKebab(prop); k != prop {
					key = k
				}
				if v, ok := computed[key]; ok {
					cs.Set(prop, jsc.StringValue(v))
				}
			}
			cs.Set("getPropertyValue", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyValue",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) == 0 {
						return jsc.StringValue("")
					}
					prop := strings.ToLower(strings.TrimSpace(a[0].ToString()))
					if v, ok := computed[prop]; ok {
						return jsc.StringValue(v)
					}
					if camel := kebabToCamel(prop); camel != prop {
						if v, ok := computed[camel]; ok {
							return jsc.StringValue(v)
						}
					}
					return jsc.StringValue("")
				}, 1)))
			cs.Set("getPropertyPriority", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyPriority",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.StringValue("")
				}, 1)))
			cs.Set("cssText", jsc.StringValue(""))
			return jsc.ObjectValue(cs)
		}, 1)))

	// Event 基类构造函数
	g.Set("Event", jsc.FunctionValue(rt.NewConstructor("Event",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.Set("type", jsc.StringValue(""))
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("composed", jsc.BooleanValue(false))
			ev.Set("target", jsc.Null())
			ev.Set("currentTarget", jsc.Null())
			ev.Set("defaultPrevented", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					if v, ok := o.GetByKey("bubbles"); ok {
						ev.Set("bubbles", v)
					}
					if v, ok := o.GetByKey("cancelable"); ok {
						ev.Set("cancelable", v)
					}
					if v, ok := o.GetByKey("composed"); ok {
						ev.Set("composed", v)
					}
				}
			}
			ev.Set("preventDefault", jsc.FunctionValue(jsc.NewNativeFunction("preventDefault",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					this.AsObject().Set("defaultPrevented", jsc.BooleanValue(true))
					return jsc.Undefined()
				}, 0)))
			ev.Set("stopPropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopPropagation",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 0)))
			return ev
		})))

	// MouseEvent 构造函数
	g.Set("MouseEvent", jsc.FunctionValue(rt.NewConstructor("MouseEvent",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.Set("type", jsc.StringValue(""))
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("target", jsc.Null())
			ev.Set("clientX", jsc.NumberValue(0))
			ev.Set("clientY", jsc.NumberValue(0))
			ev.Set("screenX", jsc.NumberValue(0))
			ev.Set("screenY", jsc.NumberValue(0))
			ev.Set("button", jsc.NumberValue(0))
			ev.Set("buttons", jsc.NumberValue(0))
			ev.Set("ctrlKey", jsc.BooleanValue(false))
			ev.Set("shiftKey", jsc.BooleanValue(false))
			ev.Set("altKey", jsc.BooleanValue(false))
			ev.Set("metaKey", jsc.BooleanValue(false))
			ev.Set("defaultPrevented", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					for _, k := range []string{"bubbles", "cancelable", "clientX", "clientY",
						"screenX", "screenY", "button", "buttons",
						"ctrlKey", "shiftKey", "altKey", "metaKey"} {
						if v, ok := o.GetByKey(k); ok {
							ev.Set(k, v)
						}
					}
				}
			}
			ev.Set("preventDefault", jsc.FunctionValue(jsc.NewNativeFunction("preventDefault",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					this.AsObject().Set("defaultPrevented", jsc.BooleanValue(true))
					return jsc.Undefined()
				}, 0)))
			ev.Set("stopPropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopPropagation",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 0)))
			return ev
		})))

	// KeyboardEvent 构造函数
	g.Set("KeyboardEvent", jsc.FunctionValue(rt.NewConstructor("KeyboardEvent",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.Set("type", jsc.StringValue(""))
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("target", jsc.Null())
			ev.Set("key", jsc.StringValue(""))
			ev.Set("code", jsc.StringValue(""))
			ev.Set("ctrlKey", jsc.BooleanValue(false))
			ev.Set("shiftKey", jsc.BooleanValue(false))
			ev.Set("altKey", jsc.BooleanValue(false))
			ev.Set("metaKey", jsc.BooleanValue(false))
			ev.Set("repeat", jsc.BooleanValue(false))
			ev.Set("isComposing", jsc.BooleanValue(false))
			ev.Set("defaultPrevented", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					for _, k := range []string{"bubbles", "cancelable",
						"key", "code", "ctrlKey", "shiftKey", "altKey", "metaKey",
						"repeat", "isComposing"} {
						if v, ok := o.GetByKey(k); ok {
							ev.Set(k, v)
						}
					}
				}
			}
			ev.Set("preventDefault", jsc.FunctionValue(jsc.NewNativeFunction("preventDefault",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					this.AsObject().Set("defaultPrevented", jsc.BooleanValue(true))
					return jsc.Undefined()
				}, 0)))
			ev.Set("stopPropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopPropagation",
				func(interp *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 0)))
			return ev
		})))

	// CustomEvent 构造函数
	g.Set("CustomEvent", jsc.FunctionValue(rt.NewConstructor("CustomEvent",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.Set("type", jsc.StringValue(""))
			ev.Set("detail", jsc.Null())
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("composed", jsc.BooleanValue(false))
			if len(args) >= 1 { ev.Set("type", jsc.StringValue(args[0].ToString())) }
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					if v, ok := o.GetByKey("detail"); ok { ev.Set("detail", v) }
					if v, ok := o.GetByKey("bubbles"); ok { ev.Set("bubbles", v) }
					if v, ok := o.GetByKey("cancelable"); ok { ev.Set("cancelable", v) }
				}
			}
			return ev
		})))

	// EventTarget 基类（可实例化的非 DOM 事件目标）。
	// 浏览器标准 API：addEventListener / removeEventListener / dispatchEvent。
	// 组件库或自定义事件源（如 WebSocket stub、状态总线）可能直接使用它。
	g.Set("EventTarget", jsc.FunctionValue(rt.NewConstructor("EventTarget",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := jsc.NewObject(in.ObjectPrototype())
			listeners := map[string][]jsc.JSValue{} // type → JS callbacks
			obj.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 || !a[1].IsCallable() {
						return jsc.Undefined()
					}
					t := a[0].ToString()
					for _, fn := range listeners[t] {
						if fn.SameAs(a[1]) {
							return jsc.Undefined() // duplicate
						}
					}
					listeners[t] = append(listeners[t], a[1])
					return jsc.Undefined()
				}, 2)))
			obj.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 {
						return jsc.Undefined()
					}
					t := a[0].ToString()
					cur := listeners[t]
					out := cur[:0]
					for _, fn := range cur {
						if len(a) >= 2 && a[1].IsCallable() && fn.SameAs(a[1]) {
							continue
						}
						out = append(out, fn)
					}
					listeners[t] = out
					return jsc.Undefined()
				}, 2)))
			obj.Set("dispatchEvent", jsc.FunctionValue(jsc.NewNativeFunction("dispatchEvent",
				func(interp *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 || !a[0].IsObject() {
						return jsc.BooleanValue(false)
					}
					ev := a[0]
					evObj := ev.AsObject()
					if evObj == nil {
						return jsc.BooleanValue(false)
					}
					// 设置 target/currentTarget（若未定义）
					if v, ok := evObj.GetByKey("target"); !ok || v.IsUndefined() || v.IsNull() {
						evObj.Set("target", this)
					}
					evObj.Set("currentTarget", this)
					t := ""
					if v, ok := evObj.GetByKey("type"); ok && v.IsString() {
						t = v.ToString()
					}
					// 复制一份，避免回调中增删影响遍历
					var cbs []jsc.JSValue
					cbs = append(cbs, listeners[t]...)
					for _, fn := range cbs {
						interp.Call(fn, this, []jsc.JSValue{ev})
					}
					return jsc.BooleanValue(true)
				}, 1)))
			return obj
		})))

	// WebSocket 构造器（通用 stub）。
	// wb-ui 引擎不内置真实 WebSocket 传输；此 stub 提供完整的浏览器语法
	// （readyState 常量 / onopen / onmessage / onerror / onclose / send / close /
	// addEventListener），供无真实网络环境的应用（桌面端）安全使用：
	// 不建立连接、不崩溃、事件由宿主通过 dispatchMessage/dispatchStatus 注入。
	// 有真实传输需求的宿主可在注入层覆盖 window.WebSocket。
	wsCtor := rt.NewConstructor("WebSocket",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := jsc.NewObject(in.ObjectPrototype())
			url := ""
			if len(args) >= 1 {
				url = args[0].ToString()
			}
			obj.Set("url", jsc.StringValue(url))
			obj.Set("readyState", jsc.NumberValue(0))
			obj.Set("bufferedAmount", jsc.NumberValue(0))
			obj.Set("extensions", jsc.StringValue(""))
			obj.Set("protocol", jsc.StringValue(""))
			obj.Set("binaryType", jsc.StringValue("blob"))
			// 暴露最新实例：宿主可通过 globalThis.__desktopWS.dispatchMessage 推事件
			g.Set("__desktopWS", jsc.ObjectValue(obj))

			listeners := map[string][]jsc.JSValue{}
			handle := func(evType string, ev jsc.JSValue) {
				// on<type> 属性回调
				onProp := "on" + evType
				if v, ok := obj.GetByKey(onProp); ok && v.IsCallable() {
					in.Call(v, jsc.ObjectValue(obj), []jsc.JSValue{ev})
				}
				// addEventListener 注册的回调
				for _, fn := range listeners[evType] {
					in.Call(fn, jsc.ObjectValue(obj), []jsc.JSValue{ev})
				}
			}

			obj.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 || !a[1].IsCallable() {
						return jsc.Undefined()
					}
					listeners[a[0].ToString()] = append(listeners[a[0].ToString()], a[1])
					return jsc.Undefined()
				}, 2)))
			obj.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 {
						return jsc.Undefined()
					}
					cur := listeners[a[0].ToString()]
					out := cur[:0]
					for _, fn := range cur {
						if fn.SameAs(a[1]) {
							continue
						}
						out = append(out, fn)
					}
					listeners[a[0].ToString()] = out
					return jsc.Undefined()
				}, 2)))
			obj.Set("send", jsc.FunctionValue(jsc.NewNativeFunction("send",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined() // 桌面模式忽略 send（心跳等）
				}, 1)))
			obj.Set("close", jsc.FunctionValue(jsc.NewNativeFunction("close",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					if obj.GetStr("readyState").ToNumber() == 3 {
						return jsc.Undefined()
					}
					obj.Set("readyState", jsc.NumberValue(3))
					ev := jsc.NewObject(in.ObjectPrototype())
					ev.Set("type", jsc.StringValue("close"))
					ev.Set("code", jsc.NumberValue(1000))
					ev.Set("reason", jsc.StringValue(""))
					handle("close", jsc.ObjectValue(ev))
					return jsc.Undefined()
				}, 0)))
			// 宿主扩展：dispatchMessage / dispatchStatus 推送事件
			obj.Set("dispatchMessage", jsc.FunctionValue(jsc.NewNativeFunction("dispatchMessage",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					data := ""
					if len(a) >= 1 {
						data = a[0].ToString()
					}
					ev := jsc.NewObject(in.ObjectPrototype())
					ev.Set("type", jsc.StringValue("message"))
					ev.Set("data", jsc.StringValue(data))
					handle("message", jsc.ObjectValue(ev))
					return jsc.Undefined()
				}, 1)))
			obj.Set("dispatchStatus", jsc.FunctionValue(jsc.NewNativeFunction("dispatchStatus",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					data := ""
					if len(a) >= 1 {
						data = a[0].ToString()
					}
					ev := jsc.NewObject(in.ObjectPrototype())
					ev.Set("type", jsc.StringValue("message"))
					ev.Set("data", jsc.StringValue(data))
					handle("message", jsc.ObjectValue(ev))
					return jsc.Undefined()
				}, 1)))
			// 异步触发 onopen（模拟连接建立；等前端设置 onopen 后再回调）
			if el := in.EnsureEventLoop(); el != nil {
				openCb := in.NewNativeFunction("wsOpen", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					obj.Set("readyState", jsc.NumberValue(1))
					ev := jsc.NewObject(in.ObjectPrototype())
					ev.Set("type", jsc.StringValue("open"))
					handle("open", jsc.ObjectValue(ev))
					return jsc.Undefined()
				}, 0)
				_ = el.SetTimeout(jsc.FunctionValue(openCb), 0)
			} else {
				obj.Set("readyState", jsc.NumberValue(1))
				ev := jsc.NewObject(in.ObjectPrototype())
				ev.Set("type", jsc.StringValue("open"))
				handle("open", jsc.ObjectValue(ev))
			}
			return obj
		})
	// 静态常量挂在构造器上
	wsCtorObj := jsc.FunctionValue(wsCtor).AsObject()
	wsCtorObj.Set("CONNECTING", jsc.NumberValue(0))
	wsCtorObj.Set("OPEN", jsc.NumberValue(1))
	wsCtorObj.Set("CLOSING", jsc.NumberValue(2))
	wsCtorObj.Set("CLOSED", jsc.NumberValue(3))
	g.Set("WebSocket", jsc.FunctionValue(wsCtor))

	// DOMParser
	domParserDoc := document // capture for closures
	g.Set("DOMParser", jsc.FunctionValue(rt.NewConstructor("DOMParser",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("parseFromString", jsc.FunctionValue(jsc.NewNativeFunction("parseFromString",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 { return jsc.Null() }
					html := a[0].ToString()
					div := domParserDoc.CreateElement("div")
					div.SetInnerHTML(html)
					mockDoc := jsc.NewObject(interp.ObjectPrototype())
					wrappedDiv := wrapElement(interp, div)
					mockDoc.Set("documentElement", jsc.ObjectValue(wrappedDiv))
					mockDoc.Set("body", jsc.ObjectValue(wrappedDiv))
					mockDoc.Set("querySelector", wrappedDiv.GetStr("querySelector"))
					mockDoc.Set("querySelectorAll", wrappedDiv.GetStr("querySelectorAll"))
					return jsc.ObjectValue(mockDoc)
				}, 2)))
			return obj
		})))

	// URL / URLSearchParams
	g.Set("URL", jsc.FunctionValue(rt.NewConstructor("URL",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := jsc.NewObject(in.ObjectPrototype())
			href := ""
			if len(args) >= 1 {
				href = args[0].ToString()
				// 第二参数 base：相对 URL 拼接（如 new URL('/api/fs/list', location.origin)）
				if len(args) >= 2 && args[1].IsString() && args[1].ToString() != "" && !strings.Contains(href, "://") {
					base := args[1].ToString()
					if strings.HasSuffix(base, "/") {
						base = strings.TrimSuffix(base, "/")
					}
					href = base + href
				}
			}
			obj.Set("href", jsc.StringValue(href))
			obj.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.StringValue(href)
				}, 0)))
			// 简单 URL 解析
			queryPart := ""
			if href != "" {
				if colonIdx := strings.Index(href, "://"); colonIdx > 0 {
					obj.Set("protocol", jsc.StringValue(href[:colonIdx+1]))
					rest := href[colonIdx+3:]
					// 分离 path 与 query
					qIdx := strings.IndexByte(rest, '?')
					if qIdx >= 0 {
						queryPart = rest[qIdx+1:]
						rest = rest[:qIdx]
					}
					if pathIdx := strings.IndexByte(rest, '/'); pathIdx > 0 {
						obj.Set("hostname", jsc.StringValue(rest[:pathIdx]))
						obj.Set("pathname", jsc.StringValue(rest[pathIdx:]))
					} else {
						obj.Set("hostname", jsc.StringValue(rest))
						obj.Set("pathname", jsc.StringValue("/"))
					}
				}
				// pathname 也支持纯相对 URL（如 /api/fs/list）
				if colonIdx := strings.Index(href, "://"); colonIdx < 0 {
					qIdx := strings.IndexByte(href, '?')
					if qIdx >= 0 {
						queryPart = href[qIdx+1:]
						obj.Set("pathname", jsc.StringValue(href[:qIdx]))
					} else {
						obj.Set("pathname", jsc.StringValue(href))
					}
				}
				obj.Set("search", jsc.StringValue(("?" + queryPart)))
				obj.Set("origin", jsc.StringValue(obj.GetStr("protocol").ToString() + "//" + obj.GetStr("hostname").ToString()))
			}
			// searchParams：URLSearchParams 实例
			sp := makeURLSearchParams(in, queryPart)
			obj.Set("searchParams", jsc.ObjectValue(sp))
			obj.Set("host", jsc.StringValue(obj.GetStr("hostname").ToString()))
			// ★ URL.toString 必须包含 searchParams 的当前序列化。api.js 用
			// new URL('/api/x', origin).searchParams.set(k,v) 再 toString()
			// 构造带 query 的请求 URL——若不拼回，/api/fs/list?path=... 的
			// path 参数丢失，fs/list 回退到默认工作区，所有根目录列出同一目录。
			obj.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					qs := ""
					if fn := sp.GetStr("toString"); fn.IsCallable() {
						if r, err := in.Call(fn, jsc.ObjectValue(sp), nil); err == nil {
							qs = r.ToString()
						}
					}
					if qs != "" {
						if strings.Contains(href, "?") {
							return jsc.StringValue(href + "&" + qs)
						}
						return jsc.StringValue(href + "?" + qs)
					}
					return jsc.StringValue(href)
				}, 0)))
			return obj
		})))

	// URLSearchParams 全局构造器
	g.Set("URLSearchParams", jsc.FunctionValue(rt.NewConstructor("URLSearchParams",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			init := ""
			if len(args) >= 1 {
				init = args[0].ToString()
			}
			return makeURLSearchParams(in, init)
		})))

	// requestIdleCallback / cancelIdleCallback（GUI 模式下立即执行）
	g.Set("requestIdleCallback", jsc.FunctionValue(jsc.NewNativeFunction("requestIdleCallback",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			el := in.EnsureEventLoop()
			id := el.SetTimeout(args[0], 0) // 通过宏任务延迟执行
			return jsc.NumberValue(float64(id))
		}, 1)))
	g.Set("cancelIdleCallback", jsc.FunctionValue(jsc.NewNativeFunction("cancelIdleCallback",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearTimeout(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))

	// crypto.randomUUID / crypto.getRandomValues（crypto/rand 真随机）
	cryptoObj := jsc.NewObject(rt.ObjectPrototype())
	cryptoObj.Set("randomUUID", jsc.FunctionValue(jsc.NewNativeFunction("randomUUID",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			// 用 crypto/rand 生成 version 4 UUID（不可预测，无碰撞）
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				// 理论不可达（crypto/rand 只返回 nil err）；回退时间戳保底
				for i := range b {
					b[i] = byte(time.Now().UnixNano() >> (i * 4))
				}
			}
			b[6] = (b[6] & 0x0f) | 0x40 // version 4
			b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
			return jsc.StringValue(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
		}, 0)))
	cryptoObj.Set("getRandomValues", jsc.FunctionValue(jsc.NewNativeFunction("getRandomValues",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			// 用 crypto/rand 填充传入的 TypedArray（Vite/加密库依赖）
			if len(args) == 0 {
				return jsc.Undefined()
			}
			obj := args[0].AsObject()
			if obj == nil {
				return jsc.Undefined()
			}
			length := 0
			if lv, ok := obj.GetByKey("length"); ok {
				length = int(lv.ToNumber())
			}
			if length <= 0 || length > 65536 {
				return jsc.Undefined()
			}
			buf := make([]byte, length)
			if _, err := rand.Read(buf); err != nil {
				return jsc.Undefined()
			}
			for i, v := range buf {
				obj.Set(strconv.Itoa(i), jsc.NumberValue(float64(v)))
			}
			return args[0]
		}, 1)))
	g.Set("crypto", jsc.ObjectValue(cryptoObj))

	// CSS.escape / CSS.supports
	cssObj := jsc.NewObject(rt.ObjectPrototype())
	cssObj.Set("escape", jsc.FunctionValue(jsc.NewNativeFunction("escape",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.StringValue("")
			}
			return jsc.StringValue(cssEscapeIdent(args[0].ToString()))
		}, 1)))
	cssObj.Set("supports", jsc.FunctionValue(jsc.NewNativeFunction("supports",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			s := strings.TrimSpace(args[0].ToString())
			if s == "" {
				return jsc.BooleanValue(false)
			}
			// (property: value) 声明形式：校验 property 为已知 CSS 属性
			if strings.HasPrefix(s, "(") && strings.Contains(s, ":") {
				inner := strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
				parts := strings.SplitN(inner, ":", 2)
				prop := strings.ToLower(strings.TrimSpace(parts[0]))
				if isKnownCSSProperty(prop) {
					return jsc.BooleanValue(true)
				}
				// 未知属性按现代浏览器行为返回 false
				return jsc.BooleanValue(false)
			}
			// selector 形式：CSS.supports('selector') 返回选择器是否被引擎支持。
			// 用 css 包真实解析，并检测未知伪类/伪元素（浏览器对未知伪类返回
			// false；对不支持的选择器语法同样 false），替代此前的保守 true。
			sel := css.NewParser(s).ParseSelectorList()
			if sel == nil || len(sel.Selectors) == 0 {
				return jsc.BooleanValue(false)
			}
			if selectorHasUnknownPseudo(sel) {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(true)
		}, 1)))
	g.Set("CSS", jsc.ObjectValue(cssObj))

	// window.matchMedia — 基于 css 包真实解析与匹配
	g.Set("matchMedia", jsc.FunctionValue(jsc.NewNativeFunction("matchMedia",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			query := ""
			if len(args) > 0 {
				query = args[0].ToString()
			}
			mq := jsc.NewObject(in.ObjectPrototype())
			mq.Set("media", jsc.StringValue(query))
			ctx := css.MediaQueryContext{
				Width: 1280, Height: 800, DeviceWidth: 1280, DeviceHeight: 800,
				DevicePixelRatio: 1, Orientation: "landscape",
				PrefersColorScheme: "light", Hover: "hover", AnyHover: "hover",
				Pointer: "fine", AnyPointer: "fine",
			}
			if MediaQueryContextProvider != nil {
				if c := MediaQueryContextProvider(); c != nil {
					ctx = *c
				}
			}
			matched := false
			trimmed := strings.TrimSpace(query)
			if trimmed == "" || trimmed == "all" {
				matched = true
			} else if qs, err := css.ParseMediaQueryList(query); err == nil && len(qs) > 0 {
				matched = css.MatchesAny(qs, ctx)
			}
			mq.Set("matches", jsc.BooleanValue(matched))
			// 存储监听器，支持 change 事件（简化：不主动重估，宿主可重建）
			mql := struct{ listeners []jsc.JSValue }{}
			mq.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 2 && a[1].IsCallable() {
						mql.listeners = append(mql.listeners, a[1])
					}
					return jsc.Undefined()
				}, 2)))
			mq.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 {
						return jsc.Undefined()
					}
					out := mql.listeners[:0]
					for _, fn := range mql.listeners {
						if !fn.SameAs(a[1]) {
							out = append(out, fn)
						}
					}
					mql.listeners = out
					return jsc.Undefined()
				}, 2)))
			mq.Set("addListener", jsc.FunctionValue(jsc.NewNativeFunction("addListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 1 && a[0].IsCallable() {
						mql.listeners = append(mql.listeners, a[0])
					}
					return jsc.Undefined()
				}, 1)))
			mq.Set("removeListener", jsc.FunctionValue(jsc.NewNativeFunction("removeListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 {
						return jsc.Undefined()
					}
					out := mql.listeners[:0]
					for _, fn := range mql.listeners {
						if !fn.SameAs(a[0]) {
							out = append(out, fn)
						}
					}
					mql.listeners = out
					return jsc.Undefined()
				}, 1)))
			return jsc.ObjectValue(mq)
		}, 1)))

	// setTimeout / setInterval / clearTimeout / clearInterval（事件循环驱动）
	g.Set("setTimeout", jsc.FunctionValue(jsc.NewNativeFunction("setTimeout",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			el := in.EnsureEventLoop()
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			delayMs := int64(0)
			if len(args) >= 2 && args[1].IsNumber() {
				delayMs = int64(args[1].ToNumber())
			}
			id := el.SetTimeout(args[0], delayMs)
			return jsc.NumberValue(float64(id))
		}, 2)))
	g.Set("setInterval", jsc.FunctionValue(jsc.NewNativeFunction("setInterval",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			el := in.EnsureEventLoop()
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			intervalMs := int64(0)
			if len(args) >= 2 && args[1].IsNumber() {
				intervalMs = int64(args[1].ToNumber())
			}
			if intervalMs < 4 {
				intervalMs = 4 // 浏览器最小间隔 4ms
			}
			id := el.SetInterval(args[0], intervalMs)
			return jsc.NumberValue(float64(id))
		}, 2)))
	g.Set("clearTimeout", jsc.FunctionValue(jsc.NewNativeFunction("clearTimeout",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearTimeout(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))
	g.Set("clearInterval", jsc.FunctionValue(jsc.NewNativeFunction("clearInterval",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearInterval(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))

	// requestAnimationFrame / cancelAnimationFrame（事件循环驱动）
	g.Set("requestAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("requestAnimationFrame",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			el := in.EnsureEventLoop()
			id := el.RequestAnimationFrame(args[0])
			return jsc.NumberValue(float64(id))
		}, 1)))
	g.Set("cancelAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("cancelAnimationFrame",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.CancelAnimationFrame(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))

	// queueMicrotask（事件循环驱动）
	g.Set("queueMicrotask", jsc.FunctionValue(jsc.NewNativeFunction("queueMicrotask",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.Undefined()
			}
			el := in.EnsureEventLoop()
			el.QueueMicrotask(args[0])
			return jsc.Undefined()
		}, 1)))

	// window.addEventListener / removeEventListener — 通用存储（dispatchEvent 真分发）
	g.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 || !args[1].IsCallable() {
				return jsc.Undefined()
			}
			eventType := args[0].ToString()
			// 记录事件类型（含 capture 标志，真分发按序调用；popstate/hashchange
			// 仍由 navState 在 URL 变化时触发）
			windowEventListeners[eventType] = append(windowEventListeners[eventType], args[1])
			if eventType == "popstate" || eventType == "hashchange" {
				navState.popListeners = append(navState.popListeners, struct {
					fn      jsc.JSValue
					capture bool
				}{fn: args[1], capture: len(args) >= 3 && args[2].ToBoolean()})
			}
			return jsc.Undefined()
		}, 2)))
	g.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 || !args[1].IsCallable() {
				return jsc.Undefined()
			}
			eventType := args[0].ToString()
			// 从通用表移除
			if cur, ok := windowEventListeners[eventType]; ok {
				out := cur[:0]
				for _, fn := range cur {
					if !fn.SameAs(args[1]) {
						out = append(out, fn)
					}
				}
				windowEventListeners[eventType] = out
			}
			// 从 navState 移除（兼容 popstate/hashchange 触发）
			if eventType == "popstate" || eventType == "hashchange" {
				targetID := args[1].AsFunction().String()
				for i := len(navState.popListeners) - 1; i >= 0; i-- {
					if navState.popListeners[i].fn.AsFunction().String() == targetID {
						navState.popListeners = append(navState.popListeners[:i], navState.popListeners[i+1:]...)
					}
				}
			}
			return jsc.Undefined()
		}, 2)))
	// window.dispatchEvent — 真实分发到 window 上的监听器（target/currentTarget=window）
	g.Set("dispatchEvent", jsc.FunctionValue(jsc.NewNativeFunction("dispatchEvent",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 || !args[0].IsObject() {
				return jsc.BooleanValue(false)
			}
			ev := args[0].AsObject()
			evType := ""
			if v, ok := ev.GetByKey("type"); ok {
				evType = v.ToString()
			}
			if evType == "" {
				return jsc.BooleanValue(false)
			}
			evv := args[0]
			if listeners, ok := windowEventListeners[evType]; ok {
				for _, fn := range listeners {
					if fn.IsCallable() {
						in.Call(fn, jsc.ObjectValue(g), []jsc.JSValue{evv})
					}
				}
			}
			return jsc.BooleanValue(true)
		}, 1)))

	// MutationObserver 构造函数
	g.Set("MutationObserver", jsc.FunctionValue(rt.NewConstructor("MutationObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			mo := dom.NewMutationObserver(func(records []*dom.MutationRecord, observer *dom.MutationObserver) {
				// 将 Go 记录转换为 JS 对象并调用 JS 回调
				jsRecords := make([]jsc.JSValue, len(records))
				for i, r := range records {
					jsRecords[i] = jsc.ObjectValue(mutationRecordToJS(in, r))
				}
				moObj := jsc.NewObject(in.ObjectPrototype())
				moObj.SetInternal(observer)
				// 在事件循环的微任务中投递（已在 dom.FlushMutationObservers 中调用）
				in.Call(cb, jsc.Undefined(), []jsc.JSValue{
					jsc.ObjectValue(jsc.NewArray(nil, jsRecords)),
					jsc.ObjectValue(moObj),
				})
			})
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.SetInternal(mo)
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					opts := &dom.MutationObserverOptions{}
					if len(a) >= 2 && a[1].IsObject() {
						optObj := a[1].AsObject()
						if optObj != nil {
							if v, ok := optObj.GetByKey("childList"); ok { opts.ChildList = v.ToBoolean() }
							if v, ok := optObj.GetByKey("attributes"); ok { opts.Attributes = v.ToBoolean() }
							if v, ok := optObj.GetByKey("characterData"); ok { opts.CharacterData = v.ToBoolean() }
							if v, ok := optObj.GetByKey("subtree"); ok { opts.Subtree = v.ToBoolean() }
							if v, ok := optObj.GetByKey("attributeOldValue"); ok { opts.AttributeOldValue = v.ToBoolean() }
							if v, ok := optObj.GetByKey("characterDataOldValue"); ok { opts.CharacterDataOldValue = v.ToBoolean() }
							if filter, ok := optObj.GetByKey("attributeFilter"); ok && filter.IsObject() {
								arr := filter.AsObject()
								if arr != nil {
									for _, k := range arr.Keys() {
										if v, ok := arr.GetByKey(k); ok {
											opts.AttributeFilter = append(opts.AttributeFilter, v.ToString())
										}
									}
								}
							}
						}
					}
					mo.Observe(target, opts)
					return jsc.Undefined()
				}, 2)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					mo.Disconnect()
					return jsc.Undefined()
				}, 0)))
			obj.Set("takeRecords", jsc.FunctionValue(jsc.NewNativeFunction("takeRecords",
				func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					records := mo.TakeRecords()
					jsRecords := make([]jsc.JSValue, len(records))
					for i, r := range records {
						jsRecords[i] = jsc.ObjectValue(mutationRecordToJS(in, r))
					}
					return jsc.ObjectValue(jsc.NewArray(nil, jsRecords))
				}, 0)))
			return obj
		})))

	// IntersectionObserver 构造函数（GUI 模式下所有元素视为 100% 可见）
	g.Set("IntersectionObserver", jsc.FunctionValue(rt.NewConstructor("IntersectionObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			var observed []*dom.Element
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					observed = append(observed, target)
					// 立即通过微任务通知 100% 可见（对标浏览器首次 observe 行为）
					el := in.EnsureEventLoop()
					el.QueueMicrotask(jsc.FunctionValue(jsc.NewNativeFunction("io-cb",
						func(interp2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
							entry := jsc.NewObject(interp2.ObjectPrototype())
							entry.Set("isIntersecting", jsc.BooleanValue(true))
							entry.Set("intersectionRatio", jsc.NumberValue(1.0))
							entry.Set("target", jsc.ObjectValue(wrapElement(interp2, target)))
							entry.Set("boundingClientRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							entry.Set("intersectionRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							entry.Set("rootBounds", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							_, _ = interp2.Call(cb, jsc.Undefined(), []jsc.JSValue{
								jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.ObjectValue(entry)})),
							})
							return jsc.Undefined()
						}, 0)))
					return jsc.Undefined()
				}, 1)))
			obj.Set("unobserve", jsc.FunctionValue(jsc.NewNativeFunction("unobserve",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					for i, el := range observed {
						if el == target {
							observed = append(observed[:i], observed[i+1:]...)
							break
						}
					}
					return jsc.Undefined()
				}, 1)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					observed = nil
					return jsc.Undefined()
				}, 0)))
			obj.Set("takeRecords", jsc.FunctionValue(jsc.NewNativeFunction("takeRecords",
				func(in2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.ObjectValue(jsc.NewArray(nil, nil))
				}, 0)))
			return obj
		})))

	// ResizeObserver 构造函数
	g.Set("ResizeObserver", jsc.FunctionValue(rt.NewConstructor("ResizeObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			var observed []*dom.Element
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					observed = append(observed, target)
					// 通过微任务通知初始尺寸
					el := in.EnsureEventLoop()
					el.QueueMicrotask(jsc.FunctionValue(jsc.NewNativeFunction("ro-cb",
						func(interp2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
							entry := jsc.NewObject(interp2.ObjectPrototype())
							entry.Set("target", jsc.ObjectValue(wrapElement(interp2, target)))
							entry.Set("contentRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							_, _ = interp2.Call(cb, jsc.Undefined(), []jsc.JSValue{
								jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.ObjectValue(entry)})),
							})
							return jsc.Undefined()
						}, 0)))
					return jsc.Undefined()
				}, 1)))
			obj.Set("unobserve", jsc.FunctionValue(jsc.NewNativeFunction("unobserve",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					for i, el := range observed {
						if el == target {
							observed = append(observed[:i], observed[i+1:]...)
							break
						}
					}
					return jsc.Undefined()
				}, 1)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					observed = nil
					return jsc.Undefined()
				}, 0)))
			return obj
		})))

	// ── Selection + Range API ───────────────────────────────
	// Real Go implementation — not a stub. Supports:
	//   window.getSelection() → Selection with Ranges
	//   new Range() → Range referencing actual wb-ui DOM nodes
	//   range.setStart(node, offset) / range.setEnd(node, offset)
	//   selection.addRange(range) / getRangeAt / removeAllRanges
	// Mirrors WHATWG Selection API.
	// （sstate/selObj 在函数开头创建，见幂等分支注释）

	// Range constructor
	g.Set("Range", jsc.FunctionValue(rt.NewConstructor("Range",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			r := jsc.NewObject(in.ObjectPrototype())
			r.Set("startContainer", jsc.Null())
			r.Set("startOffset", jsc.NumberValue(0))
			r.Set("endContainer", jsc.Null())
			r.Set("endOffset", jsc.NumberValue(0))
			r.Set("collapsed", jsc.BooleanValue(true))
			r.Set("commonAncestorContainer", jsc.Null())

			r.Set("setStart", jsc.FunctionValue(jsc.NewNativeFunction("setStart",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 2 {
						o := this.AsObject()
						o.Set("startContainer", a[0])
o.Set("startOffset", jsc.NumberValue(float64(int(a[1].ToNumber()))))
						o.Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 2)))
			r.Set("setEnd", jsc.FunctionValue(jsc.NewNativeFunction("setEnd",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 2 {
						o := this.AsObject()
						o.Set("endContainer", a[0])
o.Set("endOffset", jsc.NumberValue(float64(int(a[1].ToNumber()))))
						o.Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 2)))
			r.Set("setStartBefore", jsc.FunctionValue(jsc.NewNativeFunction("setStartBefore",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 1 {
						this.AsObject().Set("startContainer", jsc.Null())
						this.AsObject().Set("startOffset", jsc.NumberValue(0))
						this.AsObject().Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 1)))
			r.Set("setEndBefore", jsc.FunctionValue(jsc.NewNativeFunction("setEndBefore",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 1 {
						this.AsObject().Set("endContainer", jsc.Null())
						this.AsObject().Set("endOffset", jsc.NumberValue(0))
						this.AsObject().Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 1)))
			r.Set("cloneRange", jsc.FunctionValue(jsc.NewNativeFunction("cloneRange",
				func(interp *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					o := this.AsObject()
					c := jsc.NewObject(interp.ObjectPrototype())
					for _, k := range []string{
						"startContainer", "startOffset", "endContainer", "endOffset"} {
						if v, ok := o.GetByKey(k); ok { c.Set(k, v) }
					}
					c.Set("collapsed", o.GetStr("collapsed"))
					return jsc.ObjectValue(c)
				}, 0)))
			r.Set("selectNode", jsc.FunctionValue(jsc.NewNativeFunction("selectNode",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 1 {
						o := this.AsObject()
						o.Set("startContainer", a[0])
						o.Set("startOffset", jsc.NumberValue(0))
						o.Set("endContainer", a[0])
						ec := int64(0)
						if cn := a[0].AsObject().GetStr("childNodes"); !cn.IsUndefined() {
ec = int64(cn.AsObject().GetStr("length").ToNumber())
						}
						o.Set("endOffset", jsc.NumberValue(float64(ec)))
						o.Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 1)))
			r.Set("selectNodeContents", jsc.FunctionValue(jsc.NewNativeFunction("selectNodeContents",
				func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 1 {
						o := this.AsObject()
						o.Set("startContainer", a[0])
						o.Set("startOffset", jsc.NumberValue(0))
						o.Set("endContainer", a[0])
						ec := int64(0)
						if cn := a[0].AsObject().GetStr("childNodes"); !cn.IsUndefined() {
ec = int64(cn.AsObject().GetStr("length").ToNumber())
						}
						o.Set("endOffset", jsc.NumberValue(float64(ec)))
						o.Set("collapsed", jsc.BooleanValue(false))
					}
					return jsc.Undefined()
				}, 1)))
			r.Set("deleteContents", jsc.FunctionValue(jsc.NewNativeFunction("deleteContents",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 0)))
			r.Set("extractContents", jsc.FunctionValue(jsc.NewNativeFunction("extractContents",
				func(interp *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.ObjectValue(jsc.NewObject(interp.ObjectPrototype()))
				}, 0)))
			r.Set("compareBoundaryPoints", jsc.FunctionValue(jsc.NewNativeFunction("compareBoundaryPoints",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.NumberValue(0)
				}, 2)))
			r.Set("detach", jsc.FunctionValue(jsc.NewNativeFunction("detach",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 0)))
			return r
		})))

	// Selection singleton 方法（selObj 在函数开头创建）

	selObj.Set("getRangeAt", jsc.FunctionValue(jsc.NewNativeFunction("getRangeAt",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 { return jsc.Null() }
idx := int(a[0].ToNumber())
			if idx >= 0 && idx < len(sstate.ranges) {
				return jsc.ObjectValue(sstate.ranges[idx])
			}
			return jsc.Null()
		}, 1)))
	selObj.Set("addRange", jsc.FunctionValue(jsc.NewNativeFunction("addRange",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 { return jsc.Undefined() }
			sel := this.AsObject()
			r := a[0].AsObject()
			sstate.ranges = append(sstate.ranges, r)
			if sc, ok := r.GetByKey("startContainer"); ok { sel.Set("anchorNode", sc) }
			if so, ok := r.GetByKey("startOffset"); ok { sel.Set("anchorOffset", so) }
			if ec, ok := r.GetByKey("endContainer"); ok { sel.Set("focusNode", ec) }
			if eo, ok := r.GetByKey("endOffset"); ok { sel.Set("focusOffset", eo) }
			sel.Set("rangeCount", jsc.NumberValue(float64(len(sstate.ranges))))
			sel.Set("isCollapsed", jsc.BooleanValue(false))
			sel.Set("type", jsc.StringValue("Range"))
			return jsc.Undefined()
		}, 1)))
	selObj.Set("removeRange", jsc.FunctionValue(jsc.NewNativeFunction("removeRange",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 { return jsc.Undefined() }
			target := a[0].AsObject()
			for i, r := range sstate.ranges {
				if r == target {
					sstate.ranges = append(sstate.ranges[:i], sstate.ranges[i+1:]...)
					break
				}
			}
			if len(sstate.ranges) == 0 {
				selObj.Set("anchorNode", jsc.Null())
				selObj.Set("focusNode", jsc.Null())
				selObj.Set("isCollapsed", jsc.BooleanValue(true))
				selObj.Set("type", jsc.StringValue("None"))
			}
			selObj.Set("rangeCount", jsc.NumberValue(float64(len(sstate.ranges))))
			return jsc.Undefined()
		}, 1)))
	selObj.Set("removeAllRanges", jsc.FunctionValue(jsc.NewNativeFunction("removeAllRanges",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			sstate.ranges = nil
			selObj.Set("rangeCount", jsc.NumberValue(0))
			selObj.Set("anchorNode", jsc.Null())
			selObj.Set("anchorOffset", jsc.NumberValue(0))
			selObj.Set("focusNode", jsc.Null())
			selObj.Set("focusOffset", jsc.NumberValue(0))
			selObj.Set("isCollapsed", jsc.BooleanValue(true))
			selObj.Set("type", jsc.StringValue("None"))
			return jsc.Undefined()
		}, 0)))
	selObj.Set("collapse", jsc.FunctionValue(jsc.NewNativeFunction("collapse",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) >= 1 {
				selObj.Set("anchorNode", a[0])
				offset := int64(0)
if len(a) >= 2 { offset = int64(a[1].ToNumber()) }
				selObj.Set("anchorOffset", jsc.NumberValue(float64(offset)))
				selObj.Set("focusNode", a[0])
				selObj.Set("focusOffset", jsc.NumberValue(float64(offset)))
			}
			selObj.Set("isCollapsed", jsc.BooleanValue(true))
			return jsc.Undefined()
		}, 2)))
	selObj.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.StringValue("")
		}, 0)))
	selObj.Set("containsNode", jsc.FunctionValue(jsc.NewNativeFunction("containsNode",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(false)
		}, 1)))

	g.Set("getSelection", jsc.FunctionValue(jsc.NewNativeFunction("getSelection",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(selObj)
		}, 0)))
	docObj.Set("getSelection", jsc.FunctionValue(jsc.NewNativeFunction("getSelection",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(selObj)
		}, 0)))

	// 完整注册完成——打上幂等标记（后续调用仅刷新 document）。
	rt.GlobalObject().Set(domBindingsMarker, jsc.BooleanValue(true))
}

type ElementWrapper struct {
	JS  *jsc.JSObject
	DOM *dom.Element
}

// ─── Document ──────────────────────────────────────────

func wrapDocument(rt *jsc.Interpreter, doc *dom.Document) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Document")
obj.SetInternal(doc)

	obj.Set("getElementById", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		if el := doc.GetElementById(arg); el != nil {
			return jsc.ObjectValue(wrapElement(in, el))
		}
		return jsc.Null()
	})))
	obj.Set("createElement", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createElementNS", funcVal(fn2(func(in *jsc.Interpreter, ns, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createTextNode", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapText(in, doc.CreateTextNode(arg)))
	})))
	obj.Set("createComment", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapComment(in, doc.CreateComment(arg)))
	})))
	obj.Set("createDocumentFragment", funcVal(fn0(func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(wrapDocFrag(in, doc.CreateDocumentFragment()))
	})))
	obj.Set("createEvent", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return eventToJS(in, doc.CreateEvent(arg))
	})))
	obj.Set("querySelector", funcVal(fn1(func(in *jsc.Interpreter, sel string) jsc.JSValue {
		// 使用完整 CSS 选择器引擎
		if found := DocumentQuerySelector(doc, sel); found != nil {
			return jsc.ObjectValue(wrapElement(in, found))
		}
		return jsc.Null()
	})))
	obj.Set("querySelectorAll", funcVal(fn1(func(in *jsc.Interpreter, sel string) jsc.JSValue {
		return arrElem(in, DocumentQuerySelectorAll(doc, sel))
	})))
	obj.Set("getElementsByTagName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByTagName(arg)
		return arrJS(in, els)
	})))
	obj.Set("getElementsByClassName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByClassName(arg)
		return arrJS(in, els)
	})))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(doc)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(doc)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(doc)))

	// Accessors
	obj.SetAccessor("body", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if b := doc.Body(); b != nil { return jsc.ObjectValue(wrapElement(in, b)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("head", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if h := doc.Head(); h != nil { return jsc.ObjectValue(wrapElement(in, h)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("documentElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if de := doc.DocumentElement(); de != nil { return jsc.ObjectValue(wrapElement(in, de)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("title",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(doc.Title()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { doc.SetTitle(v.ToString()) })
	obj.SetAccessor("URL", strAcc(doc.URL()), nil)
	obj.SetAccessor("cookie", strAcc(""), nil)
	obj.SetAccessor("compatMode", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if doc.Quirks() {
			return jsc.StringValue("BackCompat")
		}
		return jsc.StringValue("CSS1Compat")
	}), nil)

	// document.hasFocus() — CodeMirror 6 等库用它判断编辑器是否获得焦点
	// （决定光标/选区渲染）。wb-ui 由 Element.SetFocused 记录焦点状态。
	obj.Set("hasFocus", jsc.FunctionValue(jsc.NewNativeFunction("hasFocus",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			els := DocumentQuerySelectorAll(doc, "*")
			for _, el := range els {
				if el.IsFocused() {
					return jsc.BooleanValue(true)
				}
			}
			return jsc.BooleanValue(false)
		}, 0)))

	return obj
}

// ─── Node wrapper cache ────────────────────────────────
// Ensures the same Go dom.Node always maps to the same JS wrapper, so
// JS-side properties (__vue_app__, _vnode) set on one wrapper are visible
// through all DOM access methods (querySelector, getElementById, etc.),
// and reference-equality checks (===) work as in the browser.
//
// Reference equality is REQUIRED by Vue 3's renderer: removeFragment()
// terminates its traversal with `cur !== anchor` — if each nextSibling
// call returned a fresh wrapper, cur would never equal anchor and the
// loop would walk past the end of the fragment until cur is undefined,
// crashing with "Cannot read property 'nextSibling' of undefined".
var nodeWrapperCache = make(map[dom.Node]*jsc.JSObject)

// makeURLSearchParams 构造一个 URLSearchParams 对象，从 query 字符串（不带 ?）解析。
// 支持 set/get/append/delete/has/toString/forEach/entries——companion 前端
// api.js 的 apiURL() 依赖 u.searchParams.set(k, v)。
func makeURLSearchParams(in *jsc.Interpreter, query string) *jsc.JSObject {
	params := make(map[string][]string)
	order := []string{} // 保序：记录 key 首次出现顺序（URLSearchParams 迭代顺序）
	addParam := func(k, v string) {
		if _, ok := params[k]; !ok {
			order = append(order, k)
		}
		params[k] = append(params[k], v)
	}
	if query != "" {
		for _, pair := range strings.Split(query, "&") {
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			k := kv[0]
			v := ""
			if len(kv) > 1 {
				v = kv[1]
			}
			if decoded, err := url.QueryUnescape(k); err == nil {
				k = decoded
			}
			if decoded, err := url.QueryUnescape(v); err == nil {
				v = decoded
			}
			addParam(k, v)
		}
	}
	removeKey := func(k string) {
		delete(params, k)
		for i, ok := range order {
			if ok == k {
				order = append(order[:i], order[i+1:]...)
				break
			}
		}
	}
	sp := jsc.NewObject(in.ObjectPrototype())
	sp.SetClassName("URLSearchParams")
	getAll := func(key string) []string {
		if vs, ok := params[key]; ok {
			return vs
		}
		return nil
	}
	sp.Set("get", jsc.FunctionValue(jsc.NewNativeFunction("get", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Null()
		}
		vs := getAll(args[0].ToString())
		if len(vs) == 0 {
			return jsc.Null()
		}
		return jsc.StringValue(vs[0])
	}, 1)))
	sp.Set("getAll", jsc.FunctionValue(jsc.NewNativeFunction("getAll", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		vs := getAll(args[0].ToString())
		arr := jsc.NewObject(in.ObjectPrototype())
		arr.SetClassName("Array")
		if vs != nil {
			for i, v := range vs {
				arr.Set(strconv.Itoa(i), jsc.StringValue(v))
			}
		}
		arr.Set("length", jsc.NumberValue(float64(len(vs))))
		return jsc.ObjectValue(arr)
	}, 1)))
	sp.Set("has", jsc.FunctionValue(jsc.NewNativeFunction("has", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.BooleanValue(false)
		}
		_, ok := params[args[0].ToString()]
		return jsc.BooleanValue(ok)
	}, 1)))
	sp.Set("set", jsc.FunctionValue(jsc.NewNativeFunction("set", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		k := args[0].ToString()
		if _, ok := params[k]; ok {
			removeKey(k)
		}
		addParam(k, args[1].ToString())
		return jsc.Undefined()
	}, 2)))
	sp.Set("append", jsc.FunctionValue(jsc.NewNativeFunction("append", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		addParam(args[0].ToString(), args[1].ToString())
		return jsc.Undefined()
	}, 2)))
	sp.Set("delete", jsc.FunctionValue(jsc.NewNativeFunction("delete", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) > 0 {
			removeKey(args[0].ToString())
		}
		return jsc.Undefined()
	}, 1)))
	sp.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		var parts []string
		for _, k := range order {
			for _, v := range params[k] {
				parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		return jsc.StringValue(strings.Join(parts, "&"))
	}, 0)))
	// entries/keys/values 返回真迭代器（含 next()），并实现 Symbol.iterator
	// 支持 Array.from / for...of / 展开。
	makeIter := func(items []jsc.JSValue) *jsc.JSObject {
		it := jsc.NewObject(in.ObjectPrototype())
		idx := 0
		it.Set("next", jsc.FunctionValue(jsc.NewNativeFunction("next",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				res := jsc.NewObject(in.ObjectPrototype())
				if idx >= len(items) {
					res.Set("value", jsc.Undefined())
					res.Set("done", jsc.BooleanValue(true))
					return jsc.ObjectValue(res)
				}
				res.Set("value", items[idx])
				res.Set("done", jsc.BooleanValue(false))
				idx++
				return jsc.ObjectValue(res)
			}, 0)))
		// 迭代器自身实现 Symbol.iterator（返回自身），支持 Array.from/for...of
		it.SetIterator(func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(it)
		})
		return it
	}
	allEntries := func() []jsc.JSValue {
		var items []jsc.JSValue
		for _, k := range order {
			for _, v := range params[k] {
				items = append(items, jsc.ObjectValue(jsc.NewArrayForInterp(in, []jsc.JSValue{jsc.StringValue(k), jsc.StringValue(v)})))
			}
		}
		return items
	}
	sp.Set("entries", jsc.FunctionValue(jsc.NewNativeFunction("entries", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return jsc.ObjectValue(makeIter(allEntries()))
	}, 0)))
	sp.Set("keys", jsc.FunctionValue(jsc.NewNativeFunction("keys", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		var items []jsc.JSValue
		for _, k := range order {
			for range params[k] {
				items = append(items, jsc.StringValue(k))
			}
		}
		return jsc.ObjectValue(makeIter(items))
	}, 0)))
	sp.Set("values", jsc.FunctionValue(jsc.NewNativeFunction("values", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		var items []jsc.JSValue
		for _, k := range order {
			for _, v := range params[k] {
				items = append(items, jsc.StringValue(v))
			}
		}
		return jsc.ObjectValue(makeIter(items))
	}, 0)))
	// Symbol.iterator：直接复用 entries 迭代器
	sp.SetIterator(func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return jsc.ObjectValue(makeIter(allEntries()))
	})
			sp.Set("forEach", jsc.FunctionValue(jsc.NewNativeFunction("forEach", func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 || !args[0].IsCallable() {
			return jsc.Undefined()
		}
		for k, vs := range params {
			for _, v := range vs {
				in.Call(args[0], jsc.StringValue(v), []jsc.JSValue{jsc.StringValue(k), jsc.ObjectValue(sp)})
			}
		}
		return jsc.Undefined()
	}, 1)))
	return sp
}

func clearNodeCache() {
	nodeWrapperCache = make(map[dom.Node]*jsc.JSObject)
}

// isStyleElement reports whether n is an HTML <style> element.
func isStyleElement(n dom.Node) bool {
	if n == nil { return false }
	el, ok := n.(*dom.Element)
	return ok && strings.EqualFold(el.TagName(), "style")
}

// mutationRecordToJS 将 Go MutationRecord 转换为 JS 对象。
func mutationRecordToJS(in *jsc.Interpreter, r *dom.MutationRecord) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.Set("type", jsc.StringValue(string(r.Type)))
	obj.Set("target", jsc.ObjectValue(wrapElement(in, r.Target.(*dom.Element))))
	if r.AddedNodes != nil {
		jsAdded := make([]jsc.JSValue, len(r.AddedNodes))
		for i, n := range r.AddedNodes {
			jsAdded[i] = nodeToJS(in, n)
		}
		obj.Set("addedNodes", jsc.ObjectValue(jsc.NewArray(nil, jsAdded)))
	}
	if r.RemovedNodes != nil {
		jsRemoved := make([]jsc.JSValue, len(r.RemovedNodes))
		for i, n := range r.RemovedNodes {
			jsRemoved[i] = nodeToJS(in, n)
		}
		obj.Set("removedNodes", jsc.ObjectValue(jsc.NewArray(nil, jsRemoved)))
	}
	if r.PreviousSibling != nil {
		obj.Set("previousSibling", nodeToJS(in, r.PreviousSibling))
	}
	if r.NextSibling != nil {
		obj.Set("nextSibling", nodeToJS(in, r.NextSibling))
	}
	if r.AttributeName != "" {
		obj.Set("attributeName", jsc.StringValue(r.AttributeName))
		obj.Set("attributeNamespace", jsc.Null())
	}
	if r.OldValue != "" {
		obj.Set("oldValue", jsc.StringValue(r.OldValue))
	}
	return obj
}

// nodeToJS 将 dom.Node 转换为对应的 JS 对象。
func nodeToJS(in *jsc.Interpreter, n dom.Node) jsc.JSValue {
	switch v := n.(type) {
	case *dom.Element:
		return jsc.ObjectValue(wrapElement(in, v))
	case *dom.Text:
		return jsc.ObjectValue(wrapText(in, v))
	case *dom.Comment:
		return jsc.ObjectValue(wrapComment(in, v))
	default:
		return jsc.Null()
	}
}

// makeDOMRect 创建一个 DOMRect 对象。
func makeDOMRect(in *jsc.Interpreter, x, y, w, h float64) *jsc.JSObject {
	r := jsc.NewObject(in.ObjectPrototype())
	r.Set("x", jsc.NumberValue(x))
	r.Set("y", jsc.NumberValue(y))
	r.Set("width", jsc.NumberValue(w))
	r.Set("height", jsc.NumberValue(h))
	r.Set("top", jsc.NumberValue(y))
	r.Set("right", jsc.NumberValue(x+w))
	r.Set("bottom", jsc.NumberValue(y+h))
	r.Set("left", jsc.NumberValue(x))
	return r
}

// ─── localStorage 文件持久化（可选） ─────────────────────────

// LocalStoragePersist 接口抽象 localStorage 的持久化后端。
// 设置后，localStorage.setItem/removeItem/clear 会同步落盘；
// sessionStorage 保持纯内存（对标浏览器会话语义）。
type LocalStoragePersist interface {
	// Load 返回启动时已有的全部键值。
	Load() map[string]string
	// Save 持久化单个键值（value=空串表示删除）。
	Save(key, value string)
}

// localPersist 是当前生效的持久化后端；nil 表示纯内存模式。
var localPersist LocalStoragePersist

// SetLocalStoragePersist 启用/关闭 localStorage 文件持久化。
// 应在 RegisterDOMBindings 之前调用（desktop 入口在 LoadHTML 前设置）。
func SetLocalStoragePersist(p LocalStoragePersist) {
	localPersist = p
}

// makeStorage 创建一个 localStorage/sessionStorage 对象。
func makeStorage(rt *jsc.Interpreter, store map[string]string, session bool) *jsc.JSObject {
	s := jsc.NewObject(rt.ObjectPrototype())
	persist := func(key, value string) {
		if session || localPersist == nil {
			return
		}
		localPersist.Save(key, value)
	}
	s.Set("setItem", jsc.FunctionValue(jsc.NewNativeFunction("setItem",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 2 {
				store[args[0].ToString()] = args[1].ToString()
				persist(args[0].ToString(), args[1].ToString())
			}
			return jsc.Undefined()
		}, 2)))
	s.Set("getItem", jsc.FunctionValue(jsc.NewNativeFunction("getItem",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				if v, ok := store[args[0].ToString()]; ok {
					return jsc.StringValue(v)
				}
			}
			return jsc.Null()
		}, 1)))
	s.Set("removeItem", jsc.FunctionValue(jsc.NewNativeFunction("removeItem",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				delete(store, args[0].ToString())
				persist(args[0].ToString(), "")
			}
			return jsc.Undefined()
		}, 1)))
	s.Set("clear", jsc.FunctionValue(jsc.NewNativeFunction("clear",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			for k := range store {
				delete(store, k)
				persist(k, "")
			}
			return jsc.Undefined()
		}, 0)))
	s.Set("key", jsc.FunctionValue(jsc.NewNativeFunction("key",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				idx := int(args[0].ToNumber())
				i := 0
				for k := range store {
					if i == idx {
						return jsc.StringValue(k)
					}
					i++
				}
			}
			return jsc.Null()
		}, 1)))
	s.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(store)))
	}), nil)
	return s
}

// makePerformance 创建一个 performance 对象（now 以解释器创建时刻为时间原点）。
func makePerformance(rt *jsc.Interpreter) *jsc.JSObject {
	p := jsc.NewObject(rt.ObjectPrototype())
	origin := time.Now().UnixMilli()
	p.Set("now", jsc.FunctionValue(jsc.NewNativeFunction("now",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.NumberValue(float64(time.Now().UnixMilli() - origin))
		}, 0)))
	// timeOrigin 与浏览器一致（页面加载时刻）
	p.Set("timeOrigin", jsc.NumberValue(float64(origin)))
	return p
}

// ─── Element ───────────────────────────────────────────

func wrapElement(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	// Return cached wrapper if available
	if cached, ok := nodeWrapperCache[el]; ok {
		return cached
	}
	proto := rt.ObjectPrototype()
	if domElementProto != nil {
		proto = domElementProto
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("Element")
obj.SetInternal(el)
	// Cache before returning
	nodeWrapperCache[el] = obj

	// Attributes — 定义在 Element.prototype（见 RegisterDOMBindings），
	// 实例不再重复绑定，避免遮蔽 prototype 上的标准方法。

	// Node tree
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.AppendChild(n)
		if OnNodeInserted != nil { OnNodeInserted(n) }
		if OnStyleNodeAdded != nil && isStyleElement(n) { OnStyleNodeAdded(n) }
		return a
	})))
	obj.Set("removeChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.RemoveChild(n)
		if OnNodeRemoved != nil { OnNodeRemoved(n) }
		return a
	})))
	obj.Set("insertBefore", funcVal(fn2Node(func(in *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil {
			return jsc.Null()
		}
		// DocumentFragment: insert all children individually.
		if frag, ok := nc.(*dom.DocumentFragment); ok {
			for c := frag.FirstChild(); c != nil; c = frag.FirstChild() {
				frag.RemoveChild(c)
				if err := el.InsertBefore(c, rc); err != nil {
					// 容错：refChild 不在本节点下时回退为追加，避免 Vue vnode/DOM 不一致
					_ = el.AppendChild(c)
				}
				if OnNodeInserted != nil {
					OnNodeInserted(c)
				}
			}
			return a0
		}
		if err := el.InsertBefore(nc, rc); err != nil {
			// 浏览器对 anchor 不在父下的情况抛 NotFoundError；goja 环境 Vue 的
			// vnode/DOM 可能短暂不一致（anchor detached），静默失败会让元素
			// 永远不进 DOM 但 OnNodeInserted 照常触发 → vnode 认为已插入 →
			// 后续 v-if 关闭/卸载时 unmount 找不到正确 parent，DOM 不移除。
			// 回退追加保证元素真实进入 DOM，Vue 状态一致。
			_ = el.AppendChild(nc)
		}
		if OnNodeInserted != nil {
			OnNodeInserted(nc)
		}
		return a0
	})))
	obj.Set("replaceChild", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, oc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil || oc == nil { return jsc.Null() }
		el.ReplaceChild(nc, oc)
		if OnNodeRemoved != nil { OnNodeRemoved(oc) }
		if OnNodeInserted != nil { OnNodeInserted(nc) }
		return a1
	})))
	obj.Set("contains", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, _ jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.BooleanValue(false) }
		return jsc.BooleanValue(el.Contains(n))
	})))
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			deep := len(args) > 0 && args[0].ToBoolean()
			switch v := el.CloneNode(deep).(type) {
			case *dom.Element:
				return jsc.ObjectValue(wrapElement(in, v))
			case *dom.Text:
				return jsc.ObjectValue(wrapText(in, v))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("hasChildNodes", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.HasChildNodes())
	})))
	obj.Set("isConnected", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.IsConnected())
	})))

	// CSS 选择器匹配（对标浏览器）
	obj.Set("matches", jsc.FunctionValue(jsc.NewNativeFunction("matches",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			return jsc.BooleanValue(ElementMatches(el, args[0].ToString()))
		}, 1)))
	obj.Set("closest", jsc.FunctionValue(jsc.NewNativeFunction("closest",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			if found := ElementClosest(el, args[0].ToString()); found != nil {
				return jsc.ObjectValue(wrapElement(in, found))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("querySelector", jsc.FunctionValue(jsc.NewNativeFunction("querySelector",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			if found := ElementQuerySelector(el, args[0].ToString()); found != nil {
				return jsc.ObjectValue(wrapElement(in, found))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("querySelectorAll", jsc.FunctionValue(jsc.NewNativeFunction("querySelectorAll",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return arrElem(in, nil) }
			return arrElem(in, ElementQuerySelectorAll(el, args[0].ToString()))
		}, 1)))
	// insertAdjacentHTML
	obj.Set("insertAdjacentHTML", jsc.FunctionValue(jsc.NewNativeFunction("insertAdjacentHTML",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 { return jsc.Undefined() }
			el.InsertAdjacentHTML(args[0].ToString(), args[1].ToString())
			if OnStyleNodeAdded != nil {
				// Check for newly added <style> elements
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if isStyleElement(c) { OnStyleNodeAdded(c) }
				}
			}
			return jsc.Undefined()
		}, 2)))
	// dataset — DOMStringMap 代理 data-* 属性
	obj.Set("dataset", jsc.ObjectValue(makeDataset(rt, el)))

	// classList
	obj.Set("classList", jsc.ObjectValue(makeClassList(rt, el)))

	// style — a live object that reads/writes the style attribute
	obj.Set("style", jsc.ObjectValue(makeStyleObject(rt, el)))

	// Events
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(el)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(el)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(el)))

	// Tree traversal — dynamic getters so they reflect live DOM tree
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return el.ParentNode() }), nil)
	obj.SetAccessor("parentElement", nodeAccFn(rt, func() dom.Node { return el.ParentElement() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return el.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return el.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return el.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return el.LastChild() }), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		n := 0
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*dom.Element); ok { n++ }
		}
		return jsc.NumberValue(float64(n))
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		var els []*dom.Element
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok { els = append(els, e) }
		}
		return arrElem(in, els)
	}), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, el.ChildNodes())
	}), nil)
	// ownerDocument — needed by Vue 3 when checking element's document
	obj.SetAccessor("ownerDocument", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return in.GlobalObject().GetOrZero("document")
	}), nil)

	// ── 滚动 / 尺寸 CSSOM 属性（真实几何，经渲染树桥）──
	// 前端（Vue scrollToBottom 等）依赖 el.scrollTop = el.scrollHeight /
	// el.clientHeight / offsetHeight 等；桥未注入（非 webkit 宿主）时安全回退 0。
	obj.SetAccessor("scrollTop",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			if GetElementScrollOffset == nil {
				return jsc.NumberValue(0)
			}
			_, y := GetElementScrollOffset(el)
			return jsc.NumberValue(y)
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			if SetElementScrollOffset == nil {
				return
			}
			x := 0.0
			if GetElementScrollOffset != nil {
				x, _ = GetElementScrollOffset(el)
			}
			SetElementScrollOffset(el, x, v.ToNumber())
		})
	obj.SetAccessor("scrollLeft",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			if GetElementScrollOffset == nil {
				return jsc.NumberValue(0)
			}
			x, _ := GetElementScrollOffset(el)
			return jsc.NumberValue(x)
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			if SetElementScrollOffset == nil {
				return
			}
			y := 0.0
			if GetElementScrollOffset != nil {
				_, y = GetElementScrollOffset(el)
			}
			SetElementScrollOffset(el, v.ToNumber(), y)
		})
	obj.SetAccessor("scrollHeight", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementScrollMetrics == nil {
			return jsc.NumberValue(0)
		}
		_, _, _, th, _ := GetElementScrollMetrics(el)
		return jsc.NumberValue(th)
	}), nil)
	obj.SetAccessor("scrollWidth", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementScrollMetrics == nil {
			return jsc.NumberValue(0)
		}
		_, _, tw, _, _ := GetElementScrollMetrics(el)
		return jsc.NumberValue(tw)
	}), nil)
	obj.SetAccessor("clientHeight", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementScrollMetrics == nil {
			return jsc.NumberValue(0)
		}
		_, vh, _, _, _ := GetElementScrollMetrics(el)
		return jsc.NumberValue(vh)
	}), nil)
	obj.SetAccessor("clientWidth", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementScrollMetrics == nil {
			return jsc.NumberValue(0)
		}
		vw, _, _, _, _ := GetElementScrollMetrics(el)
		return jsc.NumberValue(vw)
	}), nil)
	obj.SetAccessor("offsetHeight", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementBoxRect == nil {
			return jsc.NumberValue(0)
		}
		_, _, _, h := GetElementBoxRect(el)
		return jsc.NumberValue(h)
	}), nil)
	obj.SetAccessor("offsetWidth", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementBoxRect == nil {
			return jsc.NumberValue(0)
		}
		_, _, w, _ := GetElementBoxRect(el)
		return jsc.NumberValue(w)
	}), nil)
	obj.SetAccessor("offsetTop", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementBoxRect == nil {
			return jsc.NumberValue(0)
		}
		_, top, _, _ := GetElementBoxRect(el)
		return jsc.NumberValue(top)
	}), nil)
	obj.SetAccessor("offsetLeft", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if GetElementBoxRect == nil {
			return jsc.NumberValue(0)
		}
		left, _, _, _ := GetElementBoxRect(el)
		return jsc.NumberValue(left)
	}), nil)

	// Position / dimension (Vue needs these)
	obj.Set("getBoundingClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			r := jsc.NewObject(in.ObjectPrototype())
			left, top, w, h := 0.0, 0.0, 0.0, 0.0
			if GetElementBoxRect != nil {
				left, top, w, h = GetElementBoxRect(el)
			}
			r.Set("x", jsc.NumberValue(left))
			r.Set("y", jsc.NumberValue(top))
			r.Set("width", jsc.NumberValue(w))
			r.Set("height", jsc.NumberValue(h))
			r.Set("top", jsc.NumberValue(top))
			r.Set("right", jsc.NumberValue(left+w))
			r.Set("bottom", jsc.NumberValue(top+h))
			r.Set("left", jsc.NumberValue(left))
			return jsc.ObjectValue(r)
		}, 0)))
	obj.Set("scrollIntoView", jsc.FunctionValue(jsc.NewNativeFunction("scrollIntoView",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	// element.remove() — self-removal from DOM
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := el.ParentNode(); p != nil { p.RemoveChild(el) }
			return jsc.Undefined()
		}, 0)))
	// focus / blur stubs
	obj.Set("focus", jsc.FunctionValue(jsc.NewNativeFunction("focus",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			el.SetFocused(true)
			return jsc.Undefined()
		}, 0)))
	obj.Set("blur", jsc.FunctionValue(jsc.NewNativeFunction("blur",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			el.SetFocused(false)
			return jsc.Undefined()
		}, 0)))
	// form control: value / checked / disabled / type
	tag := strings.ToLower(el.LocalName())
	if tag == "input" || tag == "select" || tag == "textarea" || tag == "button" || tag == "option" {
		obj.SetAccessor("value",
			getter(func(_ *jsc.Interpreter) jsc.JSValue {
				return jsc.StringValue(el.GetAttribute("value"))
			}),
			func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
				el.SetAttribute("value", v.ToString())
			})
		if tag == "input" {
			obj.SetAccessor("checked",
				getter(func(_ *jsc.Interpreter) jsc.JSValue {
					return jsc.BooleanValue(el.HasAttribute("checked"))
				}),
				func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
					if v.ToBoolean() {
						el.SetAttribute("checked", "checked")
					} else {
						el.RemoveAttribute("checked")
					}
				})
			obj.SetAccessor("type",
				getter(func(_ *jsc.Interpreter) jsc.JSValue {
					return jsc.StringValue(el.GetAttribute("type"))
				}),
				nil)
		}
		if tag == "input" || tag == "select" || tag == "textarea" || tag == "button" {
			obj.SetAccessor("disabled",
				getter(func(_ *jsc.Interpreter) jsc.JSValue {
					return jsc.BooleanValue(el.HasAttribute("disabled"))
				}),
				func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
					if v.ToBoolean() {
						el.SetAttribute("disabled", "disabled")
					} else {
						el.RemoveAttribute("disabled")
					}
				})
		}
	}

	// ── <select> specific ──
	// ── <select> specific ──
	if tag == "select" {
		obj.SetAccessor("multiple",
			getter(func(_ *jsc.Interpreter) jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("multiple"))
			}),
			nil)
		obj.SetAccessor("selectedIndex",
			getter(func(_ *jsc.Interpreter) jsc.JSValue {
				idx := 0
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						if opt.HasAttribute("selected") {
							return jsc.NumberValue(float64(idx))
						}
						idx++
					}
				}
				return jsc.NumberValue(-1)
			}),
			func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
				selIdx := int(v.ToNumber())
				idx := 0
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						if idx == selIdx {
							opt.SetAttribute("selected", "selected")
						} else {
							opt.RemoveAttribute("selected")
						}
						idx++
					}
				}
			})
		// options: returns an HTMLOptionsCollection-like object (NodeList of option elements)
		obj.SetAccessor("options", getter(func(in *jsc.Interpreter) jsc.JSValue {
			var opts []jsc.JSValue
			for c := el.FirstChild(); c != nil; c = c.NextSibling() {
				if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
					opts = append(opts, jsc.ObjectValue(wrapElement(in, opt)))
				}
			}
			arr := jsc.NewArray(in.ObjectPrototype(), opts)
			arr.Set("length", jsc.NumberValue(float64(len(opts))))
			return jsc.ObjectValue(arr)
		}), nil)
	}

	// ── <option> specific ──
	if tag == "option" {
		obj.SetAccessor("selected",
			getter(func(_ *jsc.Interpreter) jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("selected"))
			}),
			func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("selected", "selected")
				} else {
					el.RemoveAttribute("selected")
				}
			})
	}

	// Accessors for string properties

	// Accessors for string properties
	obj.SetAccessor("tagName", strAcc(el.TagName()), nil)
	obj.SetAccessor("nodeName", strAcc(el.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(el.NodeType()))
	}), nil)
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null()
	}), nil)
	obj.SetAccessor("id",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetId()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetId(v.ToString()) })
	obj.SetAccessor("className",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetClassName()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	// attributes — NamedNodeMap 风格数组：length + 索引（{name,value}）。
	// CodeMirror 6 的 setAttrs 依赖 dom.attributes.length / attributes[i].name
	// 做属性同步，缺失会导致 "Cannot read property 'length' of undefined"。
	obj.SetAccessor("attributes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		names := el.AttributeNames()
		return arrayValue(in, len(names), func(i int) jsc.JSValue {
			attr := jsc.NewObject(in.ObjectPrototype())
			attr.Set("name", jsc.StringValue(names[i]))
			attr.Set("value", jsc.StringValue(el.GetAttribute(names[i])))
			return jsc.ObjectValue(attr)
		})
	}), nil)
	obj.SetAccessor("innerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetInnerHTML()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetInnerHTML(v.ToString()) })
	obj.SetAccessor("outerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetOuterHTML()) }), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetTextContent(v.ToString()) })

	// <template> elements need .content returning a DocumentFragment
	// (Vue 3 + createStaticVNode depends on this).
	if strings.EqualFold(el.LocalName(), "template") {
		obj.SetAccessor("content",
			getter(func(in *jsc.Interpreter) jsc.JSValue {
				doc := el.OwnerDocument()
				if doc == nil {
					// Fallback: use a detached fragment if no owner document
					return jsc.ObjectValue(wrapDocFrag(in, dom.NewDocumentFragment(nil)))
				}
				frag := doc.CreateDocumentFragment()
				// Move all child nodes into the fragment
				for c := el.FirstChild(); c != nil; c = el.FirstChild() {
					frag.AppendChild(c)
				}
				return jsc.ObjectValue(wrapDocFrag(in, frag))
			}), nil)
	}

	return obj
}

// ─── classList ──────────────────────────────────────────

func makeClassList(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	cls := jsc.NewObject(rt.ObjectPrototype())
	get := func() []string { return strings.Fields(el.GetClassName()) }
	set := func(c []string) { el.SetClassName(strings.Join(c, " ")) }

	cls.Set("add", jsc.FunctionValue(jsc.NewNativeFunction("add",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { m[a.ToString()] = true }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { delete(m, a.ToString()) }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			for _, c := range get() { if c == n { return jsc.BooleanValue(true) } }
			return jsc.BooleanValue(false)
		}, 1)))
	cls.Set("toggle", jsc.FunctionValue(jsc.NewNativeFunction("toggle",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			force := len(args) >= 2
			cs := get()
			for i, c := range cs {
				if c == n {
					if force && args[1].ToBoolean() {
						return jsc.BooleanValue(true) // already there
					}
					cs = append(cs[:i], cs[i+1:]...)
					set(cs)
					return jsc.BooleanValue(false)
				}
			}
			if force && !args[1].ToBoolean() {
				return jsc.BooleanValue(false) // force-remove but not there
			}
			cs = append(cs, n)
			set(cs)
			return jsc.BooleanValue(true)
		}, 2)))
	cls.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			idx := int(args[0].ToNumber())
			cs := get()
			if idx < 0 || idx >= len(cs) {
				return jsc.Null()
			}
			return jsc.StringValue(cs[idx])
		}, 1)))
	cls.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(get())))
	}), nil)
	cls.SetAccessor("value",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetClassName()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	return cls
}

// ─── dataset (DOMStringMap) ────────────────────────────

func makeDataset(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	ds := jsc.NewObject(rt.ObjectPrototype())
	// DOMStringMap uses a proxy-like pattern: reading ds.key translates to
	// el.getAttribute("data-key"), writing translates to setAttribute.
	// Go's JSObject doesn't support full Proxy, so we provide direct accessor
	// methods and also attempt to pre-populate known data-* attrs.
	ds.Set("get", jsc.FunctionValue(jsc.NewNativeFunction("_get",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.StringValue("") }
			key := "data-" + camelToKebab(args[0].ToString())
			return jsc.StringValue(el.GetAttribute(key))
		}, 1)))
	ds.Set("set", jsc.FunctionValue(jsc.NewNativeFunction("_set",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 { return jsc.Undefined() }
			key := "data-" + camelToKebab(args[0].ToString())
			el.SetAttribute(key, args[1].ToString())
			return jsc.Undefined()
		}, 2)))
	// Pre-populate with existing data-* attributes
	for _, name := range el.AttributeNames() {
		if strings.HasPrefix(name, "data-") {
			camel := kebabToCamel(name[5:])
			val := el.GetAttribute(name)
			ds.Set(camel, jsc.StringValue(val))
		}
	}
	return ds
}

// camelToKebab converts "someProp" → "some-prop"
func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + 32) // lowercase
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// kebabToCamel converts "some-prop" → "someProp"
func kebabToCamel(s string) string {
	var b strings.Builder
	upper := false
	for _, r := range s {
		if r == '-' {
			upper = true
			continue
		}
		if upper {
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r - 32)
			} else {
				b.WriteRune(r)
			}
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ─── style object ──────────────────────────────────────
// A live CSSStyleDeclaration that reads/writes the element's style attribute.

// styleProxy implements goja.DynamicObject to intercept property-level style assignments
// (e.g. style.width = '280px') used by Vue 3's patchStyle, while still supporting
// the standard cssText / setProperty / removeProperty methods.
type styleProxy struct {
	el *dom.Element
	vm *goja.Runtime
}

func (s *styleProxy) Get(key string) goja.Value {
	vm := s.vm
	switch key {
	case "cssText":
		return vm.ToValue(s.el.GetAttribute("style"))
	case "setProperty":
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			props := parseStyle(s.el.GetAttribute("style"))
			props[call.Arguments[0].String()] = call.Arguments[1].String()
			s.el.SetAttribute("style", joinStyle(props))
			return goja.Undefined()
		})
	case "removeProperty":
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return vm.ToValue("")
			}
			props := parseStyle(s.el.GetAttribute("style"))
			old := props[call.Arguments[0].String()]
			delete(props, call.Arguments[0].String())
			s.el.SetAttribute("style", joinStyle(props))
			return vm.ToValue(old)
		})
	default:
		// CSS property: return the value from the style attribute
		props := parseStyle(s.el.GetAttribute("style"))
		if v, ok := props[camelToKebab(key)]; ok {
			return vm.ToValue(v)
		}
		return vm.ToValue("")
	}
}

func (s *styleProxy) Set(key string, val goja.Value) bool {
	switch key {
	case "cssText":
		s.el.SetAttribute("style", val.String())
		if os.Getenv("WB_STYLE_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[styleProxy] cssText=%q\n", val.String())
		}
		if OnInlineStyleChanged != nil {
			OnInlineStyleChanged(s.el)
		}
		return true
	case "setProperty", "removeProperty":
		return false // let goja handle as a regular property (function assignment)
	default:
		// CSS property write: parse existing style, update, write back
		props := parseStyle(s.el.GetAttribute("style"))
		strVal := val.String()
		ckey := camelToKebab(key)
		if strVal == "" || strVal == "undefined" || strVal == "null" {
			delete(props, ckey)
		} else {
			props[ckey] = strVal
		}
		s.el.SetAttribute("style", joinStyle(props))
		if os.Getenv("WB_STYLE_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[styleProxy] Set(%q, %q) tag=%s id=%s → %q\n",
				key, strVal, s.el.TagName(), s.el.GetAttribute("id"), s.el.GetAttribute("style"))
		}
		if OnInlineStyleChanged != nil {
			OnInlineStyleChanged(s.el)
		}
		return true
	}
}

func (s *styleProxy) Has(key string) bool {
	switch key {
	case "cssText", "setProperty", "removeProperty":
		return true
	}
	props := parseStyle(s.el.GetAttribute("style"))
	_, ok := props[key]
	return ok
}

func (s *styleProxy) Keys() []string {
	props := parseStyle(s.el.GetAttribute("style"))
	keys := []string{"cssText", "setProperty", "removeProperty"}
	for k := range props {
		keys = append(keys, k)
	}
	return keys
}

func (s *styleProxy) Delete(key string) bool {
	props := parseStyle(s.el.GetAttribute("style"))
	delete(props, key)
	s.el.SetAttribute("style", joinStyle(props))
	return true
}

func makeStyleObject(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	pr := &styleProxy{el: el, vm: rt.VM()}
	gojaObj := rt.VM().NewDynamicObject(pr)
	return jsc.WrapObject(gojaObj, rt)
}

// parseStyle parses "color:red;font-size:16px" → map
func parseStyle(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func joinStyle(m map[string]string) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ";")
}

// ─── DocumentFragment ──────────────────────────────────

func wrapDocFrag(rt *jsc.Interpreter, frag *dom.DocumentFragment) *jsc.JSObject {
	proto := rt.ObjectPrototype()
	if domDocFragProto != nil {
		proto = domDocFragProto
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("DocumentFragment")
	obj.SetInternal(frag)

	// DOM tree navigation — needed by Vue 3 insertStaticContent
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(frag.NodeType()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(frag.NodeName()), nil)

	// Tree traversal — dynamic live getters
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return frag.ParentNode() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return frag.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return frag.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return frag.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return frag.LastChild() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, frag.ChildNodes())
	}), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		n := 0
		for c := frag.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*dom.Element); ok { n++ }
		}
		return jsc.NumberValue(float64(n))
	}), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(frag.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { frag.SetTextContent(v.ToString()) })

	// appendChild
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		frag.AppendChild(n)
		if OnNodeInserted != nil { OnNodeInserted(n) }
		if OnStyleNodeAdded != nil && isStyleElement(n) {
			OnStyleNodeAdded(n)
		}
		return a
	})))
	// removeChild — Vue 3 insertStaticContent uses this
	obj.Set("removeChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		frag.RemoveChild(n)
		if OnNodeRemoved != nil { OnNodeRemoved(n) }
		return a
	})))
	// insertBefore — Vue 3 insertStaticContent uses this to insert template content
	obj.Set("insertBefore", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil { return jsc.Null() }
		frag.InsertBefore(nc, rc)
		if OnNodeInserted != nil { OnNodeInserted(nc) }
		return a0
	})))
	// hasChildNodes
	obj.Set("hasChildNodes", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(frag.HasChildNodes())
	})))
	// cloneNode
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			deep := len(args) > 0 && args[0].ToBoolean()
			cloned := frag.CloneNode(deep)
			if df, ok := cloned.(*dom.DocumentFragment); ok {
				return jsc.ObjectValue(wrapDocFrag(in, df))
			}
			return jsc.Null()
		}, 1)))

	return obj
}

// ─── Text / Comment ────────────────────────────────────

func wrapText(rt *jsc.Interpreter, t *dom.Text) *jsc.JSObject {
	// Return cached wrapper if available (Vue removeFragment relies on
	// reference equality of Text/Comment wrappers to terminate traversal).
	if cached, ok := nodeWrapperCache[t]; ok {
		return cached
	}
	proto := rt.ObjectPrototype()
	if domTextProto != nil {
		proto = domTextProto
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("Text")
	obj.SetInternal(t)
	nodeWrapperCache[t] = obj
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := t.ParentNode(); p != nil { p.RemoveChild(t) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(t.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(t.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("nodeValue",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(t.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.Length()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(t.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.NodeType()))
	}), nil)
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return t.ParentNode() }), nil)
	// 树遍历属性（Vue 3 渲染器需要：removeFragment/patch 依赖 nextSibling/previousSibling）
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return t.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return t.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return t.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return t.LastChild() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, t.ChildNodes())
	}), nil)
	return obj
}

func wrapComment(rt *jsc.Interpreter, c *dom.Comment) *jsc.JSObject {
	// Return cached wrapper if available (Vue removeFragment relies on
	// reference equality of Text/Comment wrappers to terminate traversal).
	if cached, ok := nodeWrapperCache[c]; ok {
		return cached
	}
	proto := rt.ObjectPrototype()
	if domCommentProto != nil {
		proto = domCommentProto
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("Comment")
	obj.SetInternal(c)
	nodeWrapperCache[c] = obj
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := c.ParentNode(); p != nil { p.RemoveChild(c) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(c.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(c.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("nodeValue",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(c.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(c.NodeType()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(c.NodeName()), nil)

	// 树遍历属性（Vue 3 渲染器需要）
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return c.ParentNode() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return c.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return c.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return c.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return c.LastChild() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, c.ChildNodes())
	}), nil)
	return obj
}

// ─── Helpers ───────────────────────────────────────────

func unwrapNode(v jsc.JSValue) dom.Node {
	if !v.IsObject() { return nil }
	if n, ok := v.AsObject().Internal().(dom.Node); ok { return n }
	return nil
}

// Accessor helpers
type getterFn = func(*jsc.Interpreter, jsc.JSValue) jsc.JSValue

func getter(fn func(*jsc.Interpreter) jsc.JSValue) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return fn(in) }
}

func strAcc(s string) getterFn {
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.StringValue(s) }
}

func nodeAcc(rt *jsc.Interpreter, n dom.Node) getterFn {
	if n == nil {
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
	}
	switch v := n.(type) {
	case *dom.Element:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapElement(rt, v))
		}
	case *dom.Text:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapText(rt, v))
		}
	}
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
}

// nodeAccFn returns an accessor getter that calls fn() each time it is read,
// so it stays in sync with the live DOM tree.
func nodeAccFn(rt *jsc.Interpreter, fn func() dom.Node) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
		n := fn()
		if n == nil {
			return jsc.Null()
		}
		switch v := n.(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		case *dom.Comment:
			return jsc.ObjectValue(wrapComment(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		}
		return jsc.Null()
	}
}

func funcVal(fn *jsc.JSFunction) jsc.JSValue { return jsc.FunctionValue(fn) }

// fn0/fn1/fn2 helpers
func fn0(fn func(in *jsc.Interpreter) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return fn(in)
	}, 0)
}

func fn1(fn func(in *jsc.Interpreter, arg string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, args[0].ToString())
	}, 1)
}

func fn2(fn func(in *jsc.Interpreter, a, b string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Undefined() }
		return fn(in, args[0].ToString(), args[1].ToString())
	}, 2)
}

func fn1Node(fn func(in *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), args[0])
	}, 1)
}

func fn2Node(fn func(in *jsc.Interpreter, n1, n2 dom.Node, a0, a1 jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), unwrapNode(args[1]), args[0], args[1])
	}, 2)
}

// arr helpers
func arrElem(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

func arrNode(in *jsc.Interpreter, nodes []dom.Node) jsc.JSValue {
	return arrayValue(in, len(nodes), func(i int) jsc.JSValue {
		switch v := nodes[i].(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		case *dom.Comment:
			return jsc.ObjectValue(wrapComment(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		}
		return jsc.Null()
	})
}
func arrayValue(in *jsc.Interpreter, n int, fn func(int) jsc.JSValue) jsc.JSValue {
	arr := make([]jsc.JSValue, n)
	for i := 0; i < n; i++ { arr[i] = fn(i) }
	return jsc.ObjectValue(jsc.NewArrayForInterp(in, arr))
}
func arrJS(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

// cssEscapeIdent 按 CSSOM 规范的 CSS.escape 转义 CSS 标识符。
func cssEscapeIdent(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	escapeByte := func(c byte) {
		b.WriteByte('\\')
		switch {
		case c < 0x20 || c == 0x7F:
			// 控制字符 → 十六进制转义 + 空格
			b.WriteString(strconv.FormatInt(int64(c), 16))
			b.WriteByte(' ')
		case c >= 0x30 && c <= 0x39 || c >= 0x41 && c <= 0x5A || c >= 0x61 && c <= 0x7A || c == '_' || c == '-' || c >= 0x80:
			// 标识符字符 → 反斜杠前缀原样（规范行为）
			b.WriteByte(c)
		default:
			// 其余可打印 ASCII（空格等）→ 反斜杠 + 原字符（如 'a b' → 'a\ b'）
			b.WriteByte(c)
		}
	}
	first := s[0]
	switch {
	case first >= '0' && first <= '9':
		// 首字符数字 → 十六进制转义（CSS 标识符不能以数字开头）
		b.WriteByte('\\')
		b.WriteString(strconv.FormatInt(int64(first), 16))
		b.WriteByte(' ')
	case first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z' || first == '_' || first == '-' || first >= 0x80:
		b.WriteByte(first)
	default:
		escapeByte(first)
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '-' && i == len(s)-1 {
			// 尾随连字符必须转义
			b.WriteString("\\-")
			continue
		}
		if c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '-' || c >= 0x80 {
			b.WriteByte(c)
			continue
		}
		escapeByte(c)
	}
	return b.String()
}

// registeredDocument 由 RegisterDOMBindings 维护，getComputedStyle 通过它
// 遍历 document 内的 <style> 样式表做级联计算。
var registeredDocument *dom.Document

// styleSheetCache 缓存样式表解析结果：key 为 document + 全部 <style> 文本
// 的 FNV 指纹。style 文本不变（含运行时动态注入）即复用解析结果，避免每次
// getComputedStyle 全量重解析（Vue 应用多个 style 标签时解析开销可观）。
// class/inline style 变化不失效：选择器匹配与内联叠加在每次计算时实时进行。
var styleSheetCache struct {
	doc    *dom.Document
	finger uint64
	rules  []css.Rule
}

// collectStyleTexts 遍历 document 收集所有 <style> 元素的文本，并返回
// 基于 FNV-1a 的指纹（含文本顺序信息）。
func collectStyleTexts(doc dom.Node) ([]string, uint64) {
	var texts []string
	finger := uint64(14695981039346656037) // FNV offset basis
	mix := func(b []byte) {
		for _, c := range b {
			finger ^= uint64(c)
			finger *= 1099511628211
		}
	}
	var visit func(n dom.Node)
	visit = func(n dom.Node) {
		if n == nil {
			return
		}
		if n.NodeType() == dom.NodeElement {
			if e, ok := n.(*dom.Element); ok && strings.EqualFold(e.TagName(), "style") {
				var sb strings.Builder
				for _, c := range e.ChildNodes() {
					if c.NodeType() == dom.NodeText {
						sb.WriteString(c.NodeValue())
					}
				}
				if sb.Len() > 0 {
					t := sb.String()
					texts = append(texts, t)
					mix([]byte(t))
				}
			}
		}
		for _, c := range n.ChildNodes() {
			visit(c)
		}
	}
	visit(doc)
	return texts, finger
}

// computedStyleFor 计算元素的级联样式（bindings 层近似实现）：
// 遍历 document 中 <style> 元素文本 → css 包解析（指纹缓存复用）→
// SelectorChecker 匹配元素 → 按 specificity + 源顺序级联 → 再按标准优先级
// 叠加 inline style 与 !important。返回 kebab-case 属性名 → 原始值字符串 的
// 映射（CSS 变量 --x 同样参与级联）。
// computedStyleDepth 防止 resolveVarInComputed 收集祖先自定义属性时
// computedStyleFor 无限递归（祖先的 out 也含 var() → 再解析 → 再收集…）。
var computedStyleDepth int

func computedStyleFor(el dom.Node) map[string]string {
	if computedStyleDepth > 8 {
		return map[string]string{}
	}
	computedStyleDepth++
	defer func() { computedStyleDepth-- }()
	out := map[string]string{}
	doc := el.OwnerDocument()
	if doc == nil {
		doc = registeredDocument
	}
	if doc == nil {
		return out
	}
	type matchedDecl struct {
		spec  css.Specificity
		order int
		decl  css.Declaration
	}
	var decls []matchedDecl
	checker := css.NewSelectorChecker()
	orderCounter := 0
	var applyRules func(rules []css.Rule)
	applyRules = func(rules []css.Rule) {
		for _, r := range rules {
			switch rl := r.(type) {
			case *css.StyleRule:
				if rl.Selectors == nil || len(rl.Selectors.Selectors) == 0 {
					continue
				}
				if !checker.MatchList(rl.Selectors, el) {
					continue
				}
				spec := css.SpecificityOfList(rl.Selectors)
				for _, d := range rl.Declarations {
					if d.Name == "" {
						continue
					}
					orderCounter++
					decls = append(decls, matchedDecl{spec: spec, order: orderCounter, decl: d})
				}
			case *css.MediaRule:
				if mediaMatches(rl) {
					applyRules(rl.Rules)
				}
			}
		}
	}
	// 收集 document 中所有 <style> 元素文本并解析（指纹缓存复用）
	texts, finger := collectStyleTexts(doc)
	var rules []css.Rule
	if styleSheetCache.doc == doc && styleSheetCache.finger == finger {
		rules = styleSheetCache.rules
	} else {
		for _, t := range texts {
			rules = append(rules, css.NewParser(t).ParseStyleSheet()...)
		}
		styleSheetCache.doc = doc
		styleSheetCache.finger = finger
		styleSheetCache.rules = rules
	}
	applyRules(rules)
	// 按 specificity 升序 + 源顺序升序排序：后应用者优先
	sort.Slice(decls, func(i, j int) bool {
		if c := decls[i].spec.Compare(decls[j].spec); c != 0 {
			return c < 0
		}
		return decls[i].order < decls[j].order
	})
	// 第一遍：普通声明（!important 稍后覆盖）
	for _, md := range decls {
		if md.decl.Important {
			continue
		}
		out[strings.ToLower(md.decl.Name)] = strings.TrimSpace(md.decl.ValueString())
	}
	// inline style：特异性高于 author 普通声明，但低于 author !important
	if e, ok := el.(*dom.Element); ok {
		for k, v := range parseStyle(e.GetAttribute("style")) {
			out[k] = v
		}
	}
	// 第二遍：!important 声明
	for _, md := range decls {
		if md.decl.Important {
			out[strings.ToLower(md.decl.Name)] = strings.TrimSpace(md.decl.ValueString())
		}
	}
	// 解析 var(--xxx) 引用（自定义属性继承链：:root → body → ... → el）。
	// 浏览器语义：自定义属性随级联继承，子元素 var() 引用解析为最近祖先的
	// 定义值。wb-ui 级联 map 本身不含继承值，此处补收集 + 替换。
	resolveVarInComputed(out, el)
	return out
}

// resolveVarInComputed 解析 computedStyle map 中所有 var(--name[,fallback])
// 引用。自定义属性来源：documentElement(:root) 声明（本项目的 CSS 变量均
// 定义于 :root；跳过祖先链全遍历——每层全量级联计算成本极高，且实际变量
// 都集中在 :root。若未来出现 body 级覆盖变量，再引入按元素缓存）。
// 仅当 out 存在 var( 时才执行，避免每次 getComputedStyle 全量开销。
func resolveVarInComputed(out map[string]string, el dom.Node) {
	hasVar := false
	for _, v := range out {
		if strings.Contains(v, "var(") {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return
	}
	doc := el.OwnerDocument()
	if doc == nil {
		doc = registeredDocument
	}
	if doc == nil {
		return
	}
	vars := map[string]string{}
	if de := doc.DocumentElement(); de != nil {
		st := computedStyleFor(de)
		for k, v := range st {
			if strings.HasPrefix(k, "--") {
				vars[k] = v
			}
		}
	}
	if len(vars) == 0 {
		return
	}
	for k, v := range out {
		if strings.Contains(v, "var(") {
			out[k] = substituteVars(v, vars, 0)
		}
	}
}

// substituteVars 把字符串中 var(--name[, fallback]) 替换为 vars[name]。
// fallback 缺失且变量未定义时保留原 var() 文本（与浏览器行为一致）。
// depth 限制嵌套（变量值本身含 var() 时的递归深度）。
func substituteVars(s string, vars map[string]string, depth int) string {
	if depth > 4 || !strings.Contains(s, "var(") {
		return s
	}
	var buf strings.Builder
	i := 0
	for i < len(s) {
		vi := strings.Index(s[i:], "var(")
		if vi < 0 {
			buf.WriteString(s[i:])
			break
		}
		vi += i
		buf.WriteString(s[i:vi])
		// 找匹配的右括号（支持嵌套 var( 的 fallback）
		depthParen := 0
		j := vi + 4
		for j < len(s) {
			if s[j] == '(' {
				depthParen++
			} else if s[j] == ')' {
				if depthParen == 0 {
					break
				}
				depthParen--
			}
			j++
		}
		if j >= len(s) {
			buf.WriteString(s[vi:])
			break
		}
		inner := s[vi+4 : j]
		name := strings.TrimSpace(inner)
		fallback := ""
		if comma := strings.Index(inner, ","); comma >= 0 {
			name = strings.TrimSpace(inner[:comma])
			fallback = strings.TrimSpace(inner[comma+1:])
		}
		if v, ok := vars[name]; ok {
			buf.WriteString(substituteVars(v, vars, depth+1))
		} else if fallback != "" {
			buf.WriteString(substituteVars(fallback, vars, depth+1))
		} else {
			buf.WriteString(s[vi : j+1])
		}
		i = j + 1
	}
	return buf.String()
}

// mediaMatches 判断 MediaRule 条件是否匹配当前媒体上下文
// （视口尺寸/颜色方案来自 MediaQueryContextProvider，与 matchMedia 一致）。
func mediaMatches(rule *css.MediaRule) bool {
	if rule == nil {
		return false
	}
	if len(rule.Parsed) == 0 {
		return strings.TrimSpace(rule.Condition) == ""
	}
	ctx := css.MediaQueryContext{
		Width: 1280, Height: 800, DeviceWidth: 1280, DeviceHeight: 800,
		DevicePixelRatio: 1, Orientation: "landscape",
		PrefersColorScheme: "light", Hover: "hover", AnyHover: "hover",
		Pointer: "fine", AnyPointer: "fine",
	}
	if MediaQueryContextProvider != nil {
		if c := MediaQueryContextProvider(); c != nil {
			ctx = *c
		}
	}
	return css.MatchesAny(rule.Parsed, ctx)
}

// selectorHasUnknownPseudo 检测选择器列表是否含有引擎未知的伪类/伪元素
// （CSS.supports 对未知伪类返回 false）。注意 PseudoClassUnknown/PseudoElementUnknown
// 与“无伪类”共用 0 值，必须结合 Match 类型判断：只有 MatchPseudoClass /
// MatchPseudoElement 时才检查对应枚举。
func selectorHasUnknownPseudo(sel *css.SelectorList) bool {
	if sel == nil {
		return false
	}
	for _, cs := range sel.Selectors {
		for _, comp := range cs.Compounds {
			for _, s := range comp.Selectors {
				switch s.Match {
				case css.MatchPseudoClass:
					if s.PseudoClass == css.PseudoClassUnknown {
						return true
					}
				case css.MatchPseudoElement:
					if s.PseudoElem == css.PseudoElementUnknown {
						return true
					}
				}
			}
		}
	}
	return false
}

// knownCSSProps 是 CSS.supports('(prop: value)') 判断用的已知属性表
// （覆盖 Vue/常见库会探测的属性；非穷尽，未列出时 supports 返回 false
// 与浏览器行为一致——浏览器对未知属性也返回 false）。
var knownCSSProps = map[string]bool{
	"align-content": true, "align-items": true, "align-self": true,
	"animation": true, "appearance": true, "aspect-ratio": true,
	"backdrop-filter": true, "background": true, "background-clip": true,
	"background-color": true, "background-image": true, "background-size": true,
	"border": true, "border-bottom": true, "border-color": true,
	"border-radius": true, "border-top": true, "border-width": true,
	"bottom": true, "box-shadow": true, "box-sizing": true,
	"caption-side": true, "caret-color": true, "clip-path": true,
	"color": true, "column-gap": true, "columns": true,
	"content": true, "cursor": true, "display": true,
	"filter": true, "flex": true, "flex-basis": true, "flex-direction": true,
	"flex-flow": true, "flex-grow": true, "flex-shrink": true, "flex-wrap": true,
	"float": true, "font": true, "font-family": true, "font-size": true,
	"font-style": true, "font-weight": true, "gap": true,
	"grid": true, "grid-area": true, "grid-auto-columns": true,
	"grid-auto-rows": true, "grid-column": true, "grid-row": true,
	"grid-template": true, "grid-template-areas": true, "grid-template-columns": true,
	"grid-template-rows": true, "height": true, "inset": true,
	"inset-block": true, "inset-inline": true, "justify-content": true,
	"justify-items": true, "justify-self": true, "left": true,
	"letter-spacing": true, "line-height": true, "list-style": true,
	"margin": true, "margin-bottom": true, "margin-left": true,
	"margin-right": true, "margin-top": true, "max-height": true,
	"max-width": true, "min-height": true, "min-width": true,
	"object-fit": true, "object-position": true, "opacity": true,
	"order": true, "outline": true, "overflow": true, "overflow-x": true,
	"overflow-y": true, "padding": true, "padding-bottom": true,
	"padding-left": true, "padding-right": true, "padding-top": true,
	"place-content": true, "place-items": true, "place-self": true,
	"pointer-events": true, "position": true, "resize": true,
	"right": true, "row-gap": true, "scroll-behavior": true,
	"text-align": true, "text-decoration": true, "text-overflow": true,
	"text-shadow": true, "text-transform": true, "top": true,
	"transform": true, "transform-origin": true, "transition": true,
	"user-select": true, "vertical-align": true, "visibility": true,
	"white-space": true, "width": true, "word-break": true,
	"word-wrap": true, "z-index": true,
}

// isKnownCSSProperty 判断属性名是否在已知 CSS 属性表中。
func isKnownCSSProperty(prop string) bool {
	return knownCSSProps[strings.ToLower(prop)]
}

// Silence unused import warning
var _ = fmt.Sprintf
