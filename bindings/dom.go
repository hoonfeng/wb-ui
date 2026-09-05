// Package bindings implements Go <-> JS <-> DOM bridges for wb-ui.
// Completeness: 70% — adds full style/classList/traversal/event for SPA support.
package bindings

import (
	"crypto/rand"
	"fmt"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/goja"
	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/layout"
)

// OnStyleNodeAdded is an optional callback invoked when a <style> element is
// dynamically added to the DOM (via appendChild/insertBefore). The bindings
// set this from webkit.WebView so the frame can re-extract and apply the new
// styles. When nil, dynamic <style> injection is silently ignored.
var OnStyleNodeAdded func(node dom.Node)

// ViewportWidth / ViewportHeight 为 window.innerWidth/innerHeight 提供值
// （webkit.WebView.Resize 同步）。CM6 的 visiblePixelRange 用它们计算可见
// 像素视口，undefined 会产生 NaN → viewport 永不更新（滚动不重渲染行号）。
// ★ 多 WebView 场景：ViewportSizeForInterpreter（webkit 注入）按解释器
// 分派，优先返回所属 WebView 的实际视口——挂件 Resize 不再覆盖配置窗口
// 的 innerWidth。
var (
	ViewportWidth  float64
	ViewportHeight float64

	// ViewportSizeForInterpreter 按 JS 解释器返回其所属 WebView 的视口尺寸。
	ViewportSizeForInterpreter func(in *jsc.Interpreter) (w, h float64, ok bool)
)

// OnInlineStyleChanged is an optional callback invoked when an element's
// style attribute is changed via the JS style proxy (el.style.xxx = ...).
// The embedder should re-resolve styles and rebuild the render tree.
var OnInlineStyleChanged func(node dom.Node)

// OnClassChanged is an optional callback invoked when an element's class
// attribute changes (el.className = ... / classList.add/remove/toggle).
// ★ 祖先类变化影响后代选择器匹配（如 cm-focused 加在 cm-editor 上决定
// 后代 .cm-cursor 的 display）——必须清 resolver 样式缓存 + 重建渲染树，
// 否则后代 ResolveElement 命中旧缓存（光标 display:none 不可见）。
var OnClassChanged func(el *dom.Element)

// OnImageSrcChanged is an optional callback invoked when an <img>/<video>
// element's src attribute changes (el.src = ...). The embedder clears the
// render box's decoded-image cache and marks the render tree dirty so the
// next paint decodes the new source.
var OnImageSrcChanged func(el *dom.Element)

// FocusBridge is an optional callback invoked when JS calls el.focus() /
// el.blur() on an element. The embedder (app.Host) uses it to route JS
// focus to the engine's focused-element tracking (imeFocusedEl + caret
// rendering) — without it, JS focus() only flips the DOM flag and the
// engine never learns about the focused control, so typing goes nowhere
// and libraries (xterm.js) never receive the focus event that activates
// their input + cursor rendering.

// GetElementComputedFont returns the computed font description of an
// element (family, size px, weight, style). Used by Range.getClientRects
// to measure text widths for CodeMirror 6's charWidth/lineHeight probing.
// The embedder (app.Host / webkit.WebView) wires it to the style resolver.
var GetElementComputedFont func(el *dom.Element) (family string, size float64, weight int, style string)

// GetTextBasePos returns the layout base position (left, top in page
// coords) of a text node's first render segment. Range.getClientRects
// measures text-node substrings relative to the node's own inline origin —
// for a bare text node whose parent is a block container (e.g. CM6
// highlights `(` and `)  ` as raw text nodes directly under .cm-line),
// the parent box's left is the line start, NOT the node's x within the
// line. Using the render segment's x (which already includes all sibling
// content before it on the same line) fixes posAtCoords scanning for
// space-heavy lines ("a   b", indent + comment).
var GetTextBasePos func(t dom.Node) (left, top float64, ok bool)
var FocusBridge func(el *dom.Element, focused bool)

// SelectionBridge is an optional callback invoked when JS reads/writes an
// editable element's selection (selectionStart/End, setSelectionRange).
// The embedder maps it to the engine's form-control selection state
// (rendering.FocusedFormControlSel), which is also what the caret painter
// uses — so JS and engine selection stay in sync.
var SelectionBridge func(el *dom.Element) (start, end int)

// SetSelectionBridge is the write counterpart of SelectionBridge: JS calls
// el.setSelectionRange(start, end) — the embedder updates the engine's
// form-control selection (caret position) accordingly.
var SetSelectionBridge func(el *dom.Element, start, end int)

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
	// GetElementBoxRectFast 返回元素布局盒（布局缓存直读，不触发
	// rebuild/layout）。渲染树 dirty 时可能返回旧几何或 0——供
	// computedStyleFor 的 height/width 兜底用：输入测量场景（CM6
	// measure 的 getClientRects）渲染树频繁 dirty，若每次强制全量
	// rebuild（~22ms）会造成测量-布局风暴（事件响应慢主因）。调用方
	// 只在「布局已稳定」时依赖该值。
	GetElementBoxRectFast func(el *dom.Element) (left, top, width, height float64)
)

// ── ResizeObserver 真实现（浏览器标准）───────────────
// observe 注册元素后，宿主每帧调用 ResizeObserverCheck 检测布局尺寸
// 变化并触发回调（FitAddon 等响应式库依赖它）。stub 版本只回调一次
// contentRect=0 → xterm 收不到容器尺寸变化 → 保持 80x24 超出容器。
type roEntry struct {
	el          *dom.Element
	cb          jsc.JSValue
	lastW       float64
	lastH       float64
	initialized bool
	// debounce（浏览器体验等价）：尺寸连续快速变化（拖拽风暴）时延迟
	// 触发回调，稳定 66ms 后 fire 一次。引擎重建/布局慢（终端 fit →
	// xterm.resize → rows 重建可达 200ms+），拖拽期间每帧触发回调 →
	// 每帧 fit/resize → 每帧大布局 → 拖拽卡帧（「拖快跟不上」）。
	// 拖拽中终端内容无需实时重排，停止后 fit 一次即可（对齐浏览器
	// 最终状态，时序允许短暂延迟）。
	pendingW     float64
	pendingH     float64
	pendingSince time.Time
	firePending  bool
}

var (
	roMu            sync.Mutex
	resizeObservers []*roEntry
)

// roElementSize reads the element's laid-out size (0,0 when not laid out yet).
func roElementSize(el *dom.Element) (float64, float64) {
	if GetElementBoxRect == nil || el == nil {
		return 0, 0
	}
	_, _, w, h := GetElementBoxRect(el)
	return w, h
}

// fireROCallback invokes the observer callback with a ResizeObserverEntry
// ({target, contentRect:{x,y,width,height}}).
func fireROCallback(interp *jsc.Interpreter, cb jsc.JSValue, el *dom.Element, w, h float64) {
	if interp == nil {
		return
	}
	entry := jsc.NewObject(interp.ObjectPrototype())
	entry.Set("target", jsc.ObjectValue(wrapElement(interp, el)))
	rect := jsc.NewObject(interp.ObjectPrototype())
	rect.Set("x", jsc.NumberValue(0))
	rect.Set("y", jsc.NumberValue(0))
	rect.Set("width", jsc.NumberValue(w))
	rect.Set("height", jsc.NumberValue(h))
	rect.Set("top", jsc.NumberValue(0))
	rect.Set("left", jsc.NumberValue(0))
	rect.Set("right", jsc.NumberValue(w))
	rect.Set("bottom", jsc.NumberValue(h))
	entry.Set("contentRect", jsc.ObjectValue(rect))
	_, _ = interp.Call(cb, jsc.Undefined(), []jsc.JSValue{
		jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.ObjectValue(entry)})),
	})
}

// ResizeObserverCheck 由宿主每帧调用（主循环，布局后）：对比各被观察
// 元素的布局尺寸，变化时触发回调。与浏览器合成器驱动的 ResizeObserver
// 语义一致（尺寸变化在下一帧通知）。
func ResizeObserverCheck(interp *jsc.Interpreter) {
	if interp == nil {
		return
	}
	roMu.Lock()
	if len(resizeObservers) == 0 {
		roMu.Unlock()
		return
	}
	entries := append([]*roEntry(nil), resizeObservers...)
	roMu.Unlock()
	for _, e := range entries {
		w, h := roElementSize(e.el)
		if !e.initialized {
			e.initialized = true
			e.lastW, e.lastH = w, h
			if os.Getenv("WB_RO_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[ro] observe init el=%s size=%.0fx%.0f\n", elNameForRO(e.el), w, h)
			}
			// ★ 浏览器标准：observe 后异步触发一次初始回调（ResizeObserver
			// 规范：注册后首个已布局帧回调一次，contentRect 为当前尺寸）。
			// CodeMirror 6 创建后靠这次初始回调 requestMeasure → rAF →
			// TextWidth.measure（dummy 测 lineHeight/charWidth）；wb-ui 之前
			// 首次只记录尺寸不回调，CM6 的 HeightOracle 停留默认
			// lineHeight=14 → 行号栏按 14px/行步进而内容行 18.2px 逐行错位。
			// 仅尺寸>0 时回调（尺寸 0 的容器如未布局的 xterm 保持原行为，
			// 避免首次 0 尺寸回调干扰 fit 初始化）。
			if w > 0 && h > 0 {
				fireROCallback(interp, e.cb, e.el, w, h)
			}
			continue
		}
		if w != e.lastW || h != e.lastH {
			e.lastW, e.lastH = w, h
			if os.Getenv("WB_RO_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[ro] RESIZE el=%s %.0fx%.0f → %.0fx%.0f\n", elNameForRO(e.el), e.lastW, e.lastH, w, h)
			}
			fireROCallback(interp, e.cb, e.el, w, h)
		}
	}
}

// elNameForRO returns a short debug name for a ResizeObserver target.
func elNameForRO(el *dom.Element) string {
	if el == nil {
		return "<nil>"
	}
	cls := el.ClassName()
	if cls == "" {
		return el.LocalName()
	}
	return el.LocalName() + "." + cls
}

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

// IFrameSrcChanged 是 iframe src 属性变化（el.src = x 或
// setAttribute("src", x)）时的回调，由 webkit 注入以重载子文档
// （浏览器 iframe navigation 语义）。nil 时静默跳过（测试环境）。
var IFrameSrcChanged func(el *dom.Element, src string)

// ElementFromPoint 实现 document.elementFromPoint（webkit 分派器注入，
// 按解释器归属路由到对应 WebView 的渲染树命中）。视口坐标 → 命中的
// 最顶层元素（层叠感知：z-index/遮罩/弹窗按绘制顺序，后被绘制者在上）。
var ElementFromPoint func(in *jsc.Interpreter, x, y float64) *dom.Element

func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	// ★ 保存 interpreter：InsertTextAtSelection 插入后重建 selection
	// range 需要（makeSelRange 用 rt.ObjectPrototype）。
	sstate.rt = rt
	// ★ document 切换（LoadHTML 加载新文档）时清理跨文档的全局缓存与
	// 监听器 side-table：nodeWrapperCache 持有旧文档所有节点的 Go 强
	// 引用、registeredListeners/windowEventListeners 持有旧页面注册的
	// JS 回调、dom.observerRegistry 持有旧节点上的观察者——不清理则每次
	// 导航累积（内存探针实测：每次 LoadHTML +28MB、+38 万对象）。
	if registeredDocument != document {
		clearNodeCache()
		registeredListeners = map[listenerKey][]*jsListener{}
		windowEventListeners = map[string][]jsc.JSValue{}
		dom.ResetObserverRegistry()
	}
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
	// sstate 为包级（见文件尾部附近定义），供 InsertTextAtSelection 使用。
	selObj := jsc.NewObject(rt.ObjectPrototype())
	selObj.Set("anchorNode", jsc.Null())
	selObj.Set("anchorOffset", jsc.NumberValue(0))
	selObj.Set("focusNode", jsc.Null())
	selObj.Set("focusOffset", jsc.NumberValue(0))
	selObj.Set("isCollapsed", jsc.BooleanValue(true))
	selObj.Set("type", jsc.StringValue("None"))
	// ★ 保存 Selection 单例：updateRangeForInsert 插入后需同步
	// anchorNode/focusNode 等字段（CM6 的 DOMObserver 直接读这些字段，
	// 而非 getRangeAt）——不同步则读到旧光标位置，IME 输入后光标不后移。
	sstate.selObj = selObj

	// ★ 幂等分支已后移到 selObj 完整初始化之后（见下）——此前在
	// selObj 方法（collapse/setBaseAndExtent/…）初始化之前 return，
	// 幂等路径新建的 selObj 只有字段没有方法 → document.getSelection()
	// 返回无 collapse 的对象 → CM6 点击后 updateSelection 调
	// rawSel.collapse 抛 "Object has no member 'collapse'" → DOM
	// selection 不同步 → 真实键盘输入失败（「编辑器不能编辑」根因）。

	docObj := wrapDocument(rt, document)
	rt.GlobalObject().Set("document", jsc.ObjectValue(docObj))

	// window / self / globalThis → 全局对象
	g.Set("window", jsc.ObjectValue(g))
	g.Set("self", jsc.ObjectValue(g))
	g.Set("globalThis", jsc.ObjectValue(g))

	// ★ window.innerWidth/innerHeight（浏览器标准）：CodeMirror 6 的
	// visiblePixelRange 用 Math.min(win.innerHeight, rect.bottom) 计算可见
	// 像素视口——undefined 参与 Math.min 产出 NaN → viewport 永不更新 →
	// 滚动后行号 gutter 不重渲染（用户「滚动时行号不绘制」的根因）。
	// 值由 webkit.WebView.Resize 同步（bindings.ViewportWidth/Height），
	// 多 WebView 优先按解释器分派（ViewportSizeForInterpreter）。
	g.SetAccessor("innerWidth", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if ViewportSizeForInterpreter != nil {
			if w, _, ok := ViewportSizeForInterpreter(in); ok {
				return jsc.NumberValue(w)
			}
		}
		return jsc.NumberValue(ViewportWidth)
	}), nil)
	g.SetAccessor("innerHeight", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if ViewportSizeForInterpreter != nil {
			if _, h, ok := ViewportSizeForInterpreter(in); ok {
				return jsc.NumberValue(h)
			}
		}
		return jsc.NumberValue(ViewportHeight)
	}), nil)

	// ★ Window 构造器（浏览器标准：window 的构造函数，window instanceof
	// Window === true）。CodeMirror 6 的 isScrolledToBottom 用
	// `elt2 instanceof Window` 判断 scroll parent 是否为 window——缺 Window
	// 时抛 ReferenceError，measure 在 dummy 测量前中断，HeightOracle 停留
	// 默认 lineHeight=14 → 行号栏按 14px/行步进而内容 ~18.2px 逐行错位。
	// 注：此处只保证 Window 标识符存在（instanceof 走原生原型判断，window
	// 不在其链上返回 false → CM6 走元素 scrollTop 分支，不抛异常即够）。
	winCtor := rt.NewConstructor("Window", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		return nil // 浏览器语义：new Window() 抛 TypeError
	})
	g.Set("Window", jsc.FunctionValue(winCtor))

	// ★ __wbMeasureText：canvas 2D measureText 的原生实现（Skia 精确
	// advance）。此前 canvas measureText 用「DOM span 实测」：插入 span →
	// 强制布局 → getBoundingClientRect/offsetWidth → 移除，布局失败时回退
	// len×fs×0.6 估算（空格 7.8px vs 浏览器真实 7.1475px，差 9% ——
	// 「文本宽度没有使用标准 Skia」根因），且每次调用都触发 DOM 变更 +
	// 布局，是 CM6/xterm 测量风暴的慢热源。原生路径宽度 = Skia advance
	// （W=7.1475 与浏览器一致），高度保持与浏览器 canvas TextMetrics
	// 对齐（fBA+fBD 取整，0.8/0.2 拆分，同此前 DOM 路径行为）。
	g.Set("__wbMeasureText", jsc.FunctionValue(jsc.NewNativeFunction("__wbMeasureText",
		func(in *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) < 2 {
				return jsc.Null()
			}
			fontSpec := a[0].ToString()
			text := a[1].ToString()
			fam, size, weight, stl := parseCanvasFontSpec(fontSpec)
			w := 0.0
			if layout.MeasureTextFunc != nil {
				w = layout.MeasureTextFunc(fam, size, weight, stl, text)
			}
			ascent, descent := size*0.8, size*0.2
			if layout.FontMetricsFunc != nil {
				fa, fd, _ := layout.FontMetricsFunc(fam, size, weight, stl)
				if fa > 0 && fd > 0 {
					ascent, descent = fa, fd
				}
			}
			h := math.Round(ascent + descent)
			fba, fbd := h*0.8, h*0.2
			o := jsc.NewObject(in.ObjectPrototype())
			o.Set("width", jsc.NumberValue(w))
			o.Set("fontBoundingBoxAscent", jsc.NumberValue(fba))
			o.Set("fontBoundingBoxDescent", jsc.NumberValue(fbd))
			o.Set("actualBoundingBoxAscent", jsc.NumberValue(fba))
			o.Set("actualBoundingBoxDescent", jsc.NumberValue(fbd))
			o.Set("height", jsc.NumberValue(h))
			return jsc.ObjectValue(o)
		}, 2)))

	// ★ NodeFilter 全局常量（浏览器标准）：前端 createTreeWalker 的
	// SHOW_TEXT/FILTER_ACCEPT 等常量 + acceptNode 结果。缺 NodeFilter 时
	// createTreeWalker(SHOW_TEXT) 抛 ReferenceError（# 注释字符定位测量）。
	nf := jsc.NewObject(rt.ObjectPrototype())
	nf.Set("FILTER_ACCEPT", jsc.NumberValue(float64(dom.FilterAccept)))
	nf.Set("FILTER_REJECT", jsc.NumberValue(float64(dom.FilterReject)))
	nf.Set("FILTER_SKIP", jsc.NumberValue(float64(dom.FilterSkip)))
	nf.Set("SHOW_ALL", jsc.NumberValue(float64(dom.ShowAll)))
	nf.Set("SHOW_ELEMENT", jsc.NumberValue(float64(dom.ShowElement)))
	nf.Set("SHOW_ATTRIBUTE", jsc.NumberValue(float64(dom.ShowAttribute)))
	nf.Set("SHOW_TEXT", jsc.NumberValue(float64(dom.ShowText)))
	nf.Set("SHOW_CDATA_SECTION", jsc.NumberValue(float64(dom.ShowCDATASection)))
	nf.Set("SHOW_ENTITY_REFERENCE", jsc.NumberValue(float64(dom.ShowEntityReference)))
	nf.Set("SHOW_ENTITY", jsc.NumberValue(float64(dom.ShowEntity)))
	nf.Set("SHOW_PROCESSING_INSTRUCTION", jsc.NumberValue(float64(dom.ShowProcessingInstruction)))
	nf.Set("SHOW_COMMENT", jsc.NumberValue(float64(dom.ShowComment)))
	nf.Set("SHOW_DOCUMENT", jsc.NumberValue(float64(dom.ShowDocument)))
	nf.Set("SHOW_DOCUMENT_TYPE", jsc.NumberValue(float64(dom.ShowDocumentType)))
	nf.Set("SHOW_DOCUMENT_FRAGMENT", jsc.NumberValue(float64(dom.ShowDocumentFragment)))
	nf.Set("SHOW_NOTATION", jsc.NumberValue(float64(dom.ShowNotation)))
	g.Set("NodeFilter", jsc.ObjectValue(nf))

	// ★ devicePixelRatio（浏览器标准）：xterm 的 dpr = window.devicePixelRatio
	//   （无 fallback）用于 cellHeight = ceil(charSize.height × dpr) 计算。
	//   此前未定义 → undefined → cell.height = NaN → style.height="NaNpx"
	//   → 行高异常、终端内容画到视口外（空白）。CSS 像素渲染 → 1。
	g.Set("devicePixelRatio", jsc.NumberValue(1))

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

	// Image 构造器（new Image() → <img> 元素；canvas 2D drawImage 的
	// 图片源、live2d 纹理加载依赖）。
	imgCtor := rt.NewConstructor("Image", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		el := document.CreateElement("img")
		return wrapElement(in, el)
	})
	g.Set("Image", jsc.FunctionValue(imgCtor))

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
		name := args[0].ToString()
		el.SetAttribute(name, args[1].ToString())
		// ★ computed style 缓存失效（class/style 等属性影响样式匹配）
		InvalidateComputedStyle(el)
		// ★ class 属性变化（CM6/Vue 用 setAttribute('class') 加 cm-focused）
		// 影响后代选择器匹配——触发 OnClassChanged（清 resolver 缓存 +
		// 重建渲染树）。
		if name == "class" {
			if OnClassChanged != nil {
				OnClassChanged(el)
			}
		}
		// ★ iframe 的 src 是「导航属性」：JS 改 src 应重载子文档
		// （浏览器 iframe navigation 语义）。webkit 注入 IFrameSrcChanged
		// 回调处理重载；未注入时静默（如测试环境）。
		if name == "src" && el.LocalName() == "iframe" && IFrameSrcChanged != nil {
			IFrameSrcChanged(el, el.GetAttribute("src"))
		}
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
		name := args[0].ToString()
		el.RemoveAttribute(name)
		InvalidateComputedStyle(el)
		if name == "class" {
			if OnClassChanged != nil {
				OnClassChanged(el)
			}
		}
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
			for _, prop := range []string{"color", "backgroundColor", "background", "fontFamily", "fontSize", "lineHeight", "fontWeight", "borderColor", "width", "height", "display", "position", "opacity", "visibility", "marginTop", "marginBottom", "paddingTop", "paddingBottom", "textAlign", "whiteSpace", "overflow", "overflowX", "overflowY", "overflowWrap", "wordBreak", "textOverflow", "cursor", "zIndex", "verticalAlign", "maxHeight", "minHeight", "maxWidth", "minWidth", "borderRadius", "boxShadow", "userSelect", "pointerEvents", "top", "left", "right", "bottom", "transform", "flexDirection", "alignItems", "justifyContent", "fontStyle", "fontVariant", "letterSpacing", "textDecoration", "borderTop", "borderBottom", "borderLeft", "borderRight", "borderStyle", "borderWidth", "borderTopWidth", "borderRightWidth", "borderBottomWidth", "borderLeftWidth", "padding", "margin", "gap", "rowGap", "columnGap", "gridTemplateColumns", "gridTemplateRows", "boxSizing", "float", "clear", "listStyle", "backgroundImage", "backgroundRepeat", "backgroundPosition", "backgroundSize", "outline", "content", "clipPath"} {
				key := prop
				if k := camelToKebab(prop); k != prop {
					key = k
				}
			if v, ok := computed[key]; ok {
				cs.Set(prop, jsc.StringValue(v))
			}
		}
		// background 简写展开：浏览器 getComputedStyle 的 backgroundColor
		// 恒有值（简写会展开到各子属性；无背景时返回透明 rgba(0,0,0,0)）。
		if _, ok := computed["background-color"]; !ok {
			if v, ok2 := computed["background"]; ok2 {
				cs.Set("backgroundColor", jsc.StringValue(v))
			} else {
				cs.Set("backgroundColor", jsc.StringValue("rgba(0, 0, 0, 0)"))
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
			// composedPath（浏览器标准）：JS 构造的事件派发后 target 在
			// dispatch 时设置；构造期无 target 返回空数组。CM6 构造
			// synthetic 事件或测试库可能调用。
			ev.Set("composedPath", jsc.FunctionValue(jsc.NewNativeFunction("composedPath",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.ObjectValue(jsc.NewArray(nil, nil))
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
						"screenX", "screenY", "button", "buttons", "detail",
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

	// KeyboardEvent 构造函数（浏览器标准：终端 xterm 等库构造 wheel 事件派发，
	// 也用于测试/无障碍滚动）。字段含 deltaX/deltaY/deltaZ/deltaMode。
	g.Set("WheelEvent", jsc.FunctionValue(rt.NewConstructor("WheelEvent",
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
			ev.Set("deltaX", jsc.NumberValue(0))
			ev.Set("deltaY", jsc.NumberValue(0))
			ev.Set("deltaZ", jsc.NumberValue(0))
			ev.Set("deltaMode", jsc.NumberValue(0))
			ev.Set("defaultPrevented", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					for _, k := range []string{"bubbles", "cancelable", "clientX", "clientY",
						"screenX", "screenY", "button", "buttons",
						"ctrlKey", "shiftKey", "altKey", "metaKey",
						"deltaX", "deltaY", "deltaZ", "deltaMode"} {
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
					"key", "code", "keyCode", "which", "charCode",
					"ctrlKey", "shiftKey", "altKey", "metaKey",
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
			// ★ ignore(fn)：浏览器语义为「fn 执行期间暂停收集变更，结束后恢复」。
			// CodeMirror 6 的 TextWidth.measure 用 observer.ignore 包裹 dummy
			// 测量（插入/移除测量元素），缺 ignore 时测量抛异常 → HeightOracle
			// 停留默认 lineHeight=14 → 行号栏按 14px/行步进而内容 18.2px 错位。
			// wb-ui 的观察回调经 FlushMutationObservers 在微任务批量投递，
			// 同步执行 fn 期间产生的记录只在 fn 返回后的微任务才回调，等效暂停。
			obj.Set("ignore", jsc.FunctionValue(jsc.NewNativeFunction("ignore",
				func(in *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 || !a[0].IsCallable() {
						return jsc.Undefined()
					}
					v, _ := in.Call(a[0], jsc.Undefined(), nil)
					return v
				}, 1)))
			// ★ forceFlush()：CodeMirror 6 的 View.measure 在开头调用
			// observer.forceFlush()（CM6 对 MutationObserver 的私有扩展，立即
			// 处理积压变更）。wb-ui 缺它时 measure 抛异常 → measureScheduled
			// 卡在 0 → 后续 requestMeasure 的 `measureScheduled < 0` 判断永不
			// 成立 → rAF 不再注册 → CM6 的 HeightOracle 永远停在默认 14。
			// wb-ui 的观察记录由 FlushMutationObservers 在微任务批量投递，
			// forceFlush 直接取走积压记录投递回调即可。
			obj.Set("forceFlush", jsc.FunctionValue(jsc.NewNativeFunction("forceFlush",
				func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					records := mo.TakeRecords()
					if len(records) > 0 {
						jsRecords := make([]jsc.JSValue, len(records))
						for i, r := range records {
							jsRecords[i] = jsc.ObjectValue(mutationRecordToJS(in, r))
						}
						moObj := jsc.NewObject(in.ObjectPrototype())
						moObj.SetInternal(mo)
						in.Call(cb, jsc.Undefined(), []jsc.JSValue{
							jsc.ObjectValue(jsc.NewArray(nil, jsRecords)),
							jsc.ObjectValue(moObj),
						})
					}
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

	// ResizeObserver 构造函数 — 真实现（浏览器标准）：observe 注册元素后，
	// 宿主每帧调用 ResizeObserverCheck 检测元素布局尺寸变化并触发回调。
	// 此前是 stub（observe 只回调一次 contentRect=0）→ FitAddon 等依赖
	// ResizeObserver 的库收不到尺寸变化 → xterm 保持初始 80x24 超出容器
	// （底部内容/光标被裁剪不可见）。
	g.Set("ResizeObserver", jsc.FunctionValue(rt.NewConstructor("ResizeObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 {
						return jsc.Undefined()
					}
					targetObj := a[0].AsObject()
					if targetObj == nil {
						return jsc.Undefined()
					}
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil {
						return jsc.Undefined()
					}
					roMu.Lock()
					// 幂等：同一元素重复 observe 只保留一个。
					replaced := false
					for _, e := range resizeObservers {
						if e.el == target {
							e.cb = cb
							replaced = true
							break
						}
					}
					if !replaced {
						w, h := roElementSize(target)
						resizeObservers = append(resizeObservers, &roEntry{el: target, cb: cb, lastW: w, lastH: h})
					}
					roMu.Unlock()
					// 浏览器行为：observe 后异步回调一次初始尺寸。
					el := in.EnsureEventLoop()
					el.QueueMicrotask(jsc.FunctionValue(jsc.NewNativeFunction("ro-cb-init",
						func(interp2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
							w, h := roElementSize(target)
							fireROCallback(interp2, cb, target, w, h)
							return jsc.Undefined()
						}, 0)))
					return jsc.Undefined()
				}, 1)))
			obj.Set("unobserve", jsc.FunctionValue(jsc.NewNativeFunction("unobserve",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 {
						return jsc.Undefined()
					}
					targetObj := a[0].AsObject()
					if targetObj == nil {
						return jsc.Undefined()
					}
					target, _ := targetObj.Internal().(*dom.Element)
					roMu.Lock()
					for i, e := range resizeObservers {
						if e.el == target {
							resizeObservers = append(resizeObservers[:i], resizeObservers[i+1:]...)
							break
						}
					}
					roMu.Unlock()
					return jsc.Undefined()
				}, 1)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					roMu.Lock()
					resizeObservers = nil
					roMu.Unlock()
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
				func(in *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 2 {
						o := this.AsObject()
						o.Set("startContainer", a[0])
						o.Set("startOffset", jsc.NumberValue(float64(int(a[1].ToNumber()))))
						o.Set("collapsed", jsc.BooleanValue(false))
						updateCommonAncestor(o, in)
					}
					return jsc.Undefined()
				}, 2)))
			r.Set("setEnd", jsc.FunctionValue(jsc.NewNativeFunction("setEnd",
				func(in *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) >= 2 {
						o := this.AsObject()
						o.Set("endContainer", a[0])
						o.Set("endOffset", jsc.NumberValue(float64(int(a[1].ToNumber()))))
						o.Set("collapsed", jsc.BooleanValue(false))
						updateCommonAncestor(o, in)
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
				func(in *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
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
						updateCommonAncestor(o, in)
					}
					return jsc.Undefined()
				}, 1)))
			r.Set("selectNodeContents", jsc.FunctionValue(jsc.NewNativeFunction("selectNodeContents",
				func(in *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
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
						updateCommonAncestor(o, in)
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
		func(rt *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) >= 1 {
				selObj.Set("anchorNode", a[0])
				selObj.Set("focusNode", a[0])
				offset := int64(0)
				if len(a) >= 2 {
					offset = int64(a[1].ToNumber())
				}
				selObj.Set("anchorOffset", jsc.NumberValue(float64(offset)))
				selObj.Set("focusOffset", jsc.NumberValue(float64(offset)))
				// ★ 同步 sstate.ranges：CM6 点击/光标移动用 collapse 写 DOM
				// selection，InsertTextAtSelection（contenteditable 输入）
				// 依赖 ranges[0]——collapse 不填充则真实输入永远 false
				// （「编辑器不可编辑」根因：probe 用 addRange 绕过，真实
				// 点击走 collapse）。
				sstate.ranges = []*jsc.JSObject{makeSelRange(rt, a[0], offset, a[0], offset)}
			}
			selObj.Set("isCollapsed", jsc.BooleanValue(true))
			selObj.Set("rangeCount", jsc.NumberValue(float64(len(sstate.ranges))))
			return jsc.Undefined()
		}, 2)))
	selObj.Set("setBaseAndExtent", jsc.FunctionValue(jsc.NewNativeFunction("setBaseAndExtent",
		func(rt *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) >= 4 {
				ao := int64(a[1].ToNumber())
				fo := int64(a[3].ToNumber())
				selObj.Set("anchorNode", a[0])
				selObj.Set("anchorOffset", jsc.NumberValue(float64(ao)))
				selObj.Set("focusNode", a[2])
				selObj.Set("focusOffset", jsc.NumberValue(float64(fo)))
				sstate.ranges = []*jsc.JSObject{makeSelRange(rt, a[0], ao, a[2], fo)}
				selObj.Set("isCollapsed", jsc.BooleanValue(a[0].SameAs(a[2]) && ao == fo))
				selObj.Set("rangeCount", jsc.NumberValue(1))
			}
			return jsc.Undefined()
		}, 4)))
	selObj.Set("extend", jsc.FunctionValue(jsc.NewNativeFunction("extend",
		func(rt *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) < 1 {
				return jsc.Undefined()
			}
			off := int64(0)
			if len(a) >= 2 {
				off = int64(a[1].ToNumber())
			}
			selObj.Set("focusNode", a[0])
			selObj.Set("focusOffset", jsc.NumberValue(float64(off)))
			if len(sstate.ranges) == 0 {
				sstate.ranges = []*jsc.JSObject{makeSelRange(rt, selObj.GetStr("anchorNode"), int64(selObj.GetStr("anchorOffset").ToNumber()), a[0], off)}
			} else {
				sstate.ranges[0].Set("endContainer", a[0])
				sstate.ranges[0].Set("endOffset", jsc.NumberValue(float64(off)))
			}
			selObj.Set("isCollapsed", jsc.BooleanValue(false))
			selObj.Set("rangeCount", jsc.NumberValue(float64(len(sstate.ranges))))
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

	// ★ 幂等分支：后续 RegisterDOMBindings 调用（每次 EvalJS 前）只刷新
	// document 引用（新建 docObj 挂完整初始化的 selObj），不重建 prototype。
	// 必须放在 selObj 全部方法初始化之后（否则幂等路径的 selObj 无
	// collapse 等方法 → CM6 updateSelection 抛 "Object has no member
	// 'collapse'" → DOM selection 不同步 → 真实键盘输入失败）。
	if _, ok := g.GetByKey(domBindingsMarker); ok {
		idoc := wrapDocument(rt, document)
		idoc.Set("getSelection", jsc.FunctionValue(jsc.NewNativeFunction("getSelection",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.ObjectValue(selObj)
			}, 0)))
		g.Set("document", jsc.ObjectValue(idoc))
		// ★ 幂等刷新也需重新应用 canvas 2D 补丁（新 document 对象的
		// createElement 未包装，xterm 测量仍会失败）。
		applyCanvas2DPatch(rt)
		return
	}

	// ★ canvas 2D 测量（xterm.js 的 cellWidth/cellHeight 计算依赖）：
	// createElement('canvas') 附加 getContext('2d') + measureText。
	// wb-ui 无原生 canvas 2D——measureText 用 DOM span 实测字符尺寸，
	// 返回 TextMetrics 结构。此前 canvas 无 getContext → xterm 测量
	// 得到 NaN → style.height="NaNpx" → 行高异常（310px）→ 终端内容
	// 画到视口外（y≈918），用户看到终端空白。
	applyCanvas2DPatch(rt)
	// 完整注册完成——打上幂等标记（后续调用仅刷新 document）。
	rt.GlobalObject().Set(domBindingsMarker, jsc.BooleanValue(true))
}

// applyCanvas2DPatch 包装 document.createElement：canvas 元素附加
// getContext('2d') + measureText（xterm 的 cellWidth/cellHeight 测量依赖）。
// 幂等：window.document 每次幂等刷新（新 docObj）都需重新包装。
func applyCanvas2DPatch(rt *jsc.Interpreter) {
	if v, err := rt.RunJS(`(function(){
  var doc = window.document;
  if (!doc || doc.__canvasPatched) return 'skip';
  doc.__canvasPatched = true;
  var orig = doc.createElement.bind(doc);
  doc.createElement = function(tag){
    var el = orig(tag);
    if (el && String(tag).toLowerCase() === 'canvas') {
      if (!el.getContext) {
        el.getContext = function(type){
          if (type !== '2d') return null;
          if (el.__ctx) return el.__ctx;
          var ctx = {
            font: '10px sans-serif',
            measureText: function(text){
              var fontSpec = this.font || '10px sans-serif';
              // ★ 原生 Skia 测量（Go 侧 __wbMeasureText）：宽度 = 精确
              // advance（W=7.1475 与浏览器一致），无 DOM 变更/强制布局。
              // 此前的 DOM span 实测路径布局失败时回退 len*fs*0.6 估算
              // （空格宽 7.8 vs 真实 7.1475，差 9%）且每次调用触发
              // 布局风暴——CM6/xterm 测量慢与宽度不准的根因。
              if (typeof window.__wbMeasureText === 'function') {
                try {
                  var r = window.__wbMeasureText(fontSpec, String(text));
                  if (r && r.width > 0) {
                    var asc = r.fontBoundingBoxAscent || r.height * 0.8;
                    var desc = r.fontBoundingBoxDescent || r.height * 0.2;
                    return { width: r.width,
                             actualBoundingBoxAscent: asc, actualBoundingBoxDescent: desc,
                             fontBoundingBoxAscent: asc, fontBoundingBoxDescent: desc,
                             height: r.height };
                  }
                } catch(e) {}
              }
              var s = doc.createElement('span');
              // ★ 不能用 font 简写（wb-ui 可能不解析 font: 简写 → span 落
              // 默认 16px → 测得 8x19.2）。拆成 style.fontSize/fontFamily
              // 单独设置（xterm 自己的 measure element 就是 style.fontSize
              // 路径，实测有效）。
              var fontSpec = this.font || '10px sans-serif';
              var m = /([+-]?[\d.]+)px\s*([^;]*)/.exec(fontSpec);
              // ★ span 必须 position:static（不能 absolute）——wb-ui 引擎
              // 对 absolute 元素的子内容不布局，getBoundingClientRect 宽=0
              // → 走 fallback len*fs*0.6（空格 13*0.6=7.8，浏览器真实
              // 7.1475，差 9%）→「空格间距」偏大。static inline-block +
              // white-space:pre 在 absolute holder 内可测出真实宽度
              // （7.1475）。holder 保持 absolute 是为了隐藏+脱离文档流。
              s.style.cssText = 'position:static;visibility:hidden;white-space:pre;display:inline-block;';
              // ★ line-height:normal 必须——xterm 的 .xterm-char-measure-element
              // 有它（xterm.css），行高=字体度量 15.22；不设则继承 1.2×fs=15.6。
              // 配合 wb-ui 的 line-height:normal→字体度量解析，两处一致。
              s.style.lineHeight = 'normal';
              if (m) { s.style.fontSize = m[1] + 'px'; s.style.fontFamily = m[2].trim(); }
              else { s.style.font = fontSpec; }
              s.textContent = String(text);
              var w = 0, h = 0;
              if (doc.body) {
                // ★ span 高度布局：xterm 自己的 .xterm-char-measure-element
                // 是 absolute + inline-block + line-height:normal 挂
                // .xterm-helpers（absolute 容器）能测出真实行高；直接挂
                // body 时 height 布局为 0。用 absolute holder（模拟 xterm
                // 结构），holder 也是 div 高度 0 但 span absolute 脱离流。
                var holder = doc.createElement('div');
                holder.style.cssText = 'position:absolute;left:0;top:0;visibility:hidden;';
                holder.appendChild(s);
                doc.body.appendChild(holder);
                try { var r = s.getBoundingClientRect(); w = r.width || 0; h = r.height || 0; } catch(e) {}
                // ★ h 优先用 offsetHeight：浏览器 canvas measureText 的
                // fontBoundingBoxAscent+Descent 是字体真实度量
                // （Consolas 13px = 15.0，不含 lineGap）；rect.height 含
                // lineGap（15.22）→ xterm ceil(15.22)=16 vs Edge ceil(15)=15。
                // offsetHeight 四舍五入 15.22→15，正好对齐 Edge。
                var oh = 0;
                try { oh = s.offsetHeight || 0; } catch(e) {}
                if (oh > 0) { h = oh; }
                if (w <= 0) { w = s.offsetWidth || 0; }
                doc.body.removeChild(holder);
              }
              // fallback：wb-ui 对未布局 span 的测量可能为 0——按 font-size 估算
              var fs = parseFloat(this.font) || 13;
              if (w <= 0) { w = String(text).length * fs * 0.6; }
              if (h <= 0) { h = fs * 1.2; }
              // ★ 高度用 span 实测值（getBoundingClientRect().height 返回
              // 真实行高 15.22；xterm 用它做 ceil(15.22)=16，Edge 里
              // fontBoundingBoxAscent+Descent=15）。fba/fbd 用实测 h 的
              // 0.8/0.2 比例——但注意 h 已是真实字体行高（非估算）。
              var ascent = h * 0.8, descent = h * 0.2;
              return { width: w, actualBoundingBoxAscent: ascent, actualBoundingBoxDescent: descent,
                       fontBoundingBoxAscent: ascent, fontBoundingBoxDescent: descent, height: h };
            }
          };
          el.__ctx = ctx;
          return ctx;
        };
        el.toDataURL = function(){ return ''; };
      }
    }
    return el;
  };
  // ★ OffscreenCanvas 全局：浏览器存在，xterm 的 CharSizeService 优先
  // 用它做 canvas 测量（measureText('W') 返回浮点精确宽 7.147px），
  // 缺失时 xterm fallback DOM offsetWidth（229/32=7.15625）→ cell 宽
  // 差 0.09px/char → 80 列 screen 宽 573 vs 浏览器 572。提供同实现。
  if (typeof OffscreenCanvas === 'undefined') {
    window.OffscreenCanvas = function(w, h) {
      var c = doc.createElement('canvas');
      c.width = w || 300; c.height = h || 150;
      return c;
    };
    window.OffscreenCanvas.prototype = {};
  }
  return 'patched';
})()`); err != nil {
		fmt.Fprintf(os.Stderr, "[bindings] canvas patch RunJS error: %v\n", err)
	} else if os.Getenv("WB_TERM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[bindings] canvas patch: %s\n", v.ToString())
	}
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
	// elementFromPoint（CSSOM-View 标准 API）：层叠感知命中（遮罩/弹窗
	// 按 z 序，顶层的先命中）。webkit 分派器按解释器归属路由。
	obj.Set("elementFromPoint", jsc.FunctionValue(jsc.NewNativeFunction("elementFromPoint",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if ElementFromPoint == nil || len(args) < 2 {
				return jsc.Null()
			}
			el := ElementFromPoint(in, args[0].ToNumber(), args[1].ToNumber())
			if el == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapElement(in, el))
		}, 2)))
	// elementsFromPoint：从顶层到最深的命中元素列表（简化：顶部元素
	// + 其祖先链按 DOM 级联；空/未命中为空数组）。
	obj.Set("elementsFromPoint", jsc.FunctionValue(jsc.NewNativeFunction("elementsFromPoint",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			arr := jsc.NewArray(in.ObjectPrototype(), nil)
			if ElementFromPoint == nil || len(args) < 2 {
				return jsc.ObjectValue(arr)
			}
			el := ElementFromPoint(in, args[0].ToNumber(), args[1].ToNumber())
			if el == nil {
				return jsc.ObjectValue(arr)
			}
			var items []jsc.JSValue
			for e := el; e != nil; e = e.ParentElement() {
				items = append(items, jsc.ObjectValue(wrapElement(in, e)))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 2)))
	obj.Set("createElement", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createElementNS", funcVal(fn2(func(in *jsc.Interpreter, ns, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createTextNode", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapText(in, doc.CreateTextNode(arg)))
	})))
	obj.Set("createRange", jsc.FunctionValue(jsc.NewNativeFunction("createRange",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapRange(in, nil, 0, nil, 0))
		}, 0)))
	obj.Set("createTreeWalker", jsc.FunctionValue(jsc.NewNativeFunction("createTreeWalker",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			root := unwrapNode(args[0])
			if root == nil {
				return jsc.Null()
			}
			what := uint32(dom.ShowAll)
			if len(args) > 1 {
				what = uint32(args[1].ToNumber())
			}
			var filter dom.NodeFilter
			if len(args) > 2 && !args[2].IsNull() && !args[2].IsUndefined() {
				fo := args[2].AsObject()
				if fo != nil {
					if af, ok := fo.GetByKey("acceptNode"); ok && !af.IsNull() && !af.IsUndefined() {
						filter = dom.NodeFilterFunc(func(n dom.Node) dom.NodeFilterResult {
							res, _ := in.Call(af, jsc.Undefined(), []jsc.JSValue{nodeToJS(in, n)})
							return dom.NodeFilterResult(uint16(res.ToNumber()))
						})
					}
				}
			}
			return jsc.ObjectValue(wrapTreeWalker(in, dom.NewTreeWalker(root, what, filter)))
		}, 3)))
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
	// ★ 浏览器标准：document.defaultView === window。CodeMirror 6 的
	// view.win 取 ownerDocument.defaultView 并调用 win.requestAnimationFrame
	// 驱动 measure（HeightOracle 行高探测）；若 defaultView 缺失/非 window，
	// win 落到无 rAF 的对象 → requestMeasure 抛异常 → 行号栏按默认 14px 步进。
	obj.SetAccessor("defaultView", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return in.GlobalObject().GetOrZero("window")
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
	// （决定光标/选区渲染）。wb-ui 由 Element.SetFocused 记录焦点状态，
	// Document 维护 focused 元素缓存（O(1)，不遍历全文档）。
	obj.Set("hasFocus", jsc.FunctionValue(jsc.NewNativeFunction("hasFocus",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(doc.FocusedElement() != nil)
		}, 0)))
	// document.activeElement — CM6 的 hasFocus 检查
	// `document.hasFocus() && document.activeElement == contentDOM`；此前
	// 缺失 → undefined == contentDOM 恒 false → CM6 updateSelection 视为
	// 未聚焦 → 点击后 DOM selection 不同步 → 真实键盘输入失败（「编辑器
	// 不能编辑」根因之一）。浏览器语义：无焦点时返回 body。
	obj.SetAccessor("activeElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if fe := doc.FocusedElement(); fe != nil {
			return jsc.ObjectValue(wrapElement(in, fe))
		}
		if body := doc.Body(); body != nil {
			return jsc.ObjectValue(wrapElement(in, body))
		}
		return jsc.Null()
	}), nil)

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

// ClearPageBindingsFor 清除与指定解释器/文档相关的全局 DOM 绑定缓存。
// WebView.Destroy 调用（挂件重建/窗口关闭时）：这些包级注册表持有
// 旧文档节点（nodeWrapperCache/observerRegistry 的 key）与 JS 回调
// （registeredListeners/windowEventListeners 的 jsListener/JSValue 持
// 解释器引用）——不清理则旧 WebView 的 DOM 树与 JS 堆永不被 GC。
// 多 WebView 场景按解释器/文档精确过滤（全表清空会破坏其他 WebView
// 仍在使用的监听器）。
func ClearPageBindingsFor(interp *jsc.Interpreter, doc *dom.Document) {
	clearNodeCache()
	if interp != nil {
		for k, list := range registeredListeners {
			kept := list[:0]
			for _, l := range list {
				if l.interp != interp {
					kept = append(kept, l)
				}
			}
			if len(kept) == 0 {
				delete(registeredListeners, k)
			} else {
				registeredListeners[k] = kept
			}
		}
		for k, fns := range windowEventListeners {
			kept := fns[:0]
			for _, fn := range fns {
				if fn.Interp() != interp {
					kept = append(kept, fn)
				}
			}
			if len(kept) == 0 {
				delete(windowEventListeners, k)
			} else {
				windowEventListeners[k] = kept
			}
		}
	}
	dom.ClearObserverRegistryFor(doc)
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
	if tgt, ok := r.Target.(*dom.Element); ok && tgt != nil {
		obj.Set("target", jsc.ObjectValue(wrapElement(in, tgt)))
	} else {
		obj.Set("target", jsc.Null())
	}
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

// ── Selection 单例（包级）──────────────────────────────────
// sstate 在 RegisterDOMBindings 中填充（range 数据），供包内
// InsertTextAtSelection（contenteditable 光标插入）等使用。包级而非
// 函数内局部：contenteditable 输入发生在 host 事件循环（非注册期）。
type selState struct {
	ranges []*jsc.JSObject
	// rt 保存 RegisterDOMBindings 的 interpreter，供
	// InsertTextAtSelection（contenteditable 光标插入）在插入后重建
	// selection range（新文本节点）时使用。
	rt *jsc.Interpreter
	// selObj 保存 window.getSelection() 返回的 Selection 单例，供
	// updateRangeForInsert 在插入后同步 anchorNode/focusNode 等字段——
	// CM6 的 DOMObserver.readSelectionChange 直接读这些字段（而非
	// getRangeAt），不同步则读到旧光标位置 → IME/字符输入后光标不后移
	// （「光标停在插入文字前」根因）。
	selObj *jsc.JSObject
}

var sstate = &selState{}

// makeSelRange 构造 Selection 同步用 range 对象（结构同 Range 构造：
// startContainer/startOffset/endContainer/endOffset），供 collapse /
// setBaseAndExtent / extend 填充 sstate.ranges——InsertTextAtSelection
//（contenteditable 光标插入）只读 sstate.ranges[0]。
func makeSelRange(rt *jsc.Interpreter, anchor jsc.JSValue, anchorOff int64, focus jsc.JSValue, focusOff int64) *jsc.JSObject {
	r := jsc.NewObject(rt.ObjectPrototype())
	r.Set("startContainer", anchor)
	r.Set("startOffset", jsc.NumberValue(float64(anchorOff)))
	r.Set("endContainer", focus)
	r.Set("endOffset", jsc.NumberValue(float64(focusOff)))
	r.Set("collapsed", jsc.BooleanValue(anchor.SameAs(focus) && anchorOff == focusOff))
	updateCommonAncestor(r, rt)
	return r
}

// commonAncestorOf 返回 a/b 的最近公共祖先节点（沿 ParentNode 链找首个
// 同时是两者祖先的节点）。Range.commonAncestorContainer 的标准语义。
func commonAncestorOf(a, b dom.Node) dom.Node {
	if a == nil || b == nil {
		return nil
	}
	set := map[dom.Node]bool{}
	for n := a; n != nil; n = n.ParentNode() {
		set[n] = true
	}
	for n := b; n != nil; n = n.ParentNode() {
		if set[n] {
			return n
		}
	}
	return nil
}

// updateCommonAncestor 用 range 对象的 start/endContainer 计算公共祖先并
// 写回 commonAncestorContainer（CM6 DOMObserver 依赖它判断变更范围）。
func updateCommonAncestor(o *jsc.JSObject, in *jsc.Interpreter) {
	sc := o.GetStr("startContainer")
	ec := o.GetStr("endContainer")
	if sc.IsNull() || sc.IsUndefined() || ec.IsNull() || ec.IsUndefined() {
		o.Set("commonAncestorContainer", jsc.Null())
		return
	}
	sn := unwrapNode(sc)
	en := unwrapNode(ec)
	anc := commonAncestorOf(sn, en)
	if anc == nil {
		o.Set("commonAncestorContainer", jsc.Null())
		return
	}
	o.Set("commonAncestorContainer", nodeToJS(in, anc))
}

// compareDocPosition 实现 Node.compareDocumentPosition 的位掩码语义
// （WHATWG DOM 标准）：
//
//	DISCONNECTED=0x01  PRECEDING=0x02  FOLLOWING=0x04
//	CONTAINS=0x08      CONTAINED_BY=0x10  IMPLEMENTATION_SPECIFIC=0x20
func compareDocPosition(a, b dom.Node) int {
	if a == nil || b == nil {
		return 0x01 | 0x20
	}
	if a == b {
		return 0
	}
	// 收集祖先链（自身在最前，根在最后）
	var ancA, ancB []dom.Node
	for n := a; n != nil; n = n.ParentNode() {
		ancA = append(ancA, n)
	}
	for n := b; n != nil; n = n.ParentNode() {
		ancB = append(ancB, n)
	}
	// 从根向下找最近公共祖先（ancA[ia] == ancB[ib]）
	ia, ib := len(ancA)-1, len(ancB)-1
	lcaIdx := -1 // ancA 中 LCA 的索引
	for ia >= 0 && ib >= 0 && ancA[ia] == ancB[ib] {
		lcaIdx = ia
		ia--
		ib--
	}
	if lcaIdx < 0 {
		return 0x01 | 0x20 // 不同文档树：DISCONNECTED
	}
	if ia < 0 {
		// a 是 b 的祖先（a 的链遍历完仍全部匹配）
		return 0x08 | 0x02 // CONTAINS + PRECEDING
	}
	if ib < 0 {
		return 0x10 | 0x04 // CONTAINED_BY + FOLLOWING
	}
	// LCA 下的两个分支节点 ancA[ia] 与 ancB[ib]：按子节点顺序比较
	lca := ancA[lcaIdx]
	posA, posB := -1, -1
	idx := 0
	for c := lca.FirstChild(); c != nil; c = c.NextSibling() {
		if c == ancA[ia] {
			posA = idx
		}
		if c == ancB[ib] {
			posB = idx
		}
		if posA >= 0 && posB >= 0 {
			break
		}
		idx++
	}
	if posA < posB {
		return 0x02 // PRECEDING
	}
	return 0x04 // FOLLOWING
}

// InsertTextAtSelection 在 DOM Selection 的当前 range 处插入文本（光标处插入）。
// contenteditable（CodeMirror 6 输入区）依赖此路径：wb-ui 宿主层对
// input/textarea 走 value/textContent 直接替换，但对 contenteditable 用
// SetTextContent(全文) 会抹掉 CM6 的结构化 DOM（.cm-line + 语法高亮 span），
// 且 CM6 的 input 处理发现文本未变（全文替换文本相同）不会重建结构 → 布局
// 永久破坏。浏览器语义：在光标处插入文本节点 → 派发 input 事件 → CM6 的
// readDOMChange 对比 DOM/state 差异后重建正确结构。返回 false 表示无有效
// selection（调用方应回退：跳过 DOM 修改，仅派发 input 事件）。
func InsertTextAtSelection(text string) bool {
	if len(sstate.ranges) == 0 {
		return false
	}
	r := sstate.ranges[0]
	if r == nil {
		return false
	}
	sc := r.GetStr("startContainer")
	if sc.IsNull() || sc.IsUndefined() {
		return false
	}
	node := unwrapNode(sc)
	if node == nil {
		return false
	}
	off := int(r.GetStr("startOffset").ToNumber())
	if off < 0 {
		off = 0
	}
	insLen := len([]rune(text))
	if t, ok := node.(*dom.Text); ok {
		rs := []rune(t.NodeValue())
		if off > len(rs) {
			off = len(rs)
		}
		tail, err := t.SplitText(off)
		if err != nil {
			return false
		}
		doc := t.OwnerDocument()
		if doc == nil {
			return false
		}
		ins := dom.NewText(doc, text)
		if p := t.ParentNode(); p != nil {
			if err := p.InsertBefore(ins, tail); err != nil {
				return false
			}
			// ★ 插入后把选区光标移到插入文本之后（浏览器语义「文本往后
			// 排」）：CM6 每次输入后 DOM 重建（readDOMChange 重写
			// .cm-line），重建前的 sstate.ranges 指向旧文本节点/旧 offset
			// → 下一次 InsertTextAtSelection 用旧位置插入 → 文本插到上次
			// 输入之前（「文本插入到光标前」根因：probe 实测 IME 提交
			// "拼"后普通字符 X 插到"拼"前面，funcX拼 vs func拼X）。这里
			// 同步把 ranges[0] 更新为新插入文本节点 + 文本后的 offset。
			sstate.updateRangeForInsert(ins, insLen)
			return true
		}
		return false
	}
	if el, ok := node.(*dom.Element); ok {
		doc := el.OwnerDocument()
		if doc == nil {
			return false
		}
		ins := dom.NewText(doc, text)
		var ref dom.Node
		i := 0
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if i == off {
				ref = c
				break
			}
			i++
		}
		if err := el.InsertBefore(ins, ref); err != nil {
			return false
		}
		sstate.updateRangeForInsert(ins, insLen)
		return true
	}
	return false
}

// updateRangeForInsert 在文本插入后把 sstate.ranges[0] 更新为
// 「父元素 + 插入文本之后的子节点索引」。★ 不用新文本节点本身：
// CM6 每次输入后 readDOMChange 异步重建 .cm-line（旧文本节点被替换/
// 分离），指向文本节点的 range 在重建后 startContainer.ParentNode()==nil
// → 下一次 InsertTextAtSelection 插入失败（probe 实测 X 完全没插进去）。
// 父元素（.cm-line）在重建后仍存在，元素级 offset 表示「第 N 个子节点
// 之前」，重建后由 CM6 的 collapse 覆盖为精确位置（浏览器语义）。
// 该 range 仅供下一次输入前短暂使用（同一帧内 readDOMChange 尚未运行）。
func (s *selState) updateRangeForInsert(ins *dom.Text, insLen int) {
	if s == nil || s.rt == nil || len(s.ranges) == 0 {
		return
	}
	parent := ins.ParentNode()
	if parent == nil {
		return
	}
	// 子节点索引：ins 之后的索引 = ins 所在 index + 1（文本节点在
	// DOM 中子节点粒度，元素 offset 以子节点计——splitText 后 ins 前
	// 是原节点前半，索引计算需遍历）。
	idx := 0
	found := false
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if c == ins {
			found = true
			idx++
			break
		}
		idx++
	}
	if !found {
		return
	}
	jsv := nodeToJS(s.rt, parent)
	if jsv.IsNull() || jsv.IsUndefined() {
		return
	}
	s.ranges = []*jsc.JSObject{makeSelRange(s.rt, jsv, int64(idx), jsv, int64(idx))}
	// ★ 同步 window.getSelection() 的 anchor/focus 字段：CM6 的
	// DOMObserver.readSelectionChange 直接读 selObj.anchorNode/anchorOffset
	// /focusNode/focusOffset（而非 getRangeAt），不同步则读到旧光标位置
	// → IME/普通字符输入后光标不后移（显示在插入文字前）。
	if s.selObj != nil {
		s.selObj.Set("anchorNode", jsv)
		s.selObj.Set("anchorOffset", jsc.NumberValue(float64(idx)))
		s.selObj.Set("focusNode", jsv)
		s.selObj.Set("focusOffset", jsc.NumberValue(float64(idx)))
		s.selObj.Set("isCollapsed", jsc.BooleanValue(true))
		s.selObj.Set("rangeCount", jsc.NumberValue(1))
	}
}

// nodeToJS 将 dom.Node 转换为对应的 JS 对象。
func nodeToJS(in *jsc.Interpreter, n dom.Node) jsc.JSValue {
	if isNilNode(n) {
		return jsc.Null()
	}
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

// wrapElement 创建元素包装器。★ 惰性属性：包装器是 goja DynamicObject，
// 属性在首次访问时经 installElementProperty（bindings/lazyelement.go）
// 物化——此前每元素立即安装 ~90 个自有属性（~60µs/元素）是 CM6 文件
// 打开重绘 / Vue 文件树 / xterm 行重建 DOM 构建慢的主因。
func wrapElement(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	// Return cached wrapper if available
	if cached, ok := nodeWrapperCache[el]; ok {
		return cached
	}
	proto := rt.ObjectPrototype()
	if domElementProto != nil {
		proto = domElementProto
	}
	props := &lazyElemProps{
		el:      el,
		interp:  rt,
		cached:  map[string]jsc.JSValue{},
		expando: map[string]jsc.JSValue{},
	}
	obj := jsc.NewLazyObject(rt, proto, props)
	obj.SetInternal(el)
	// Cache before returning
	nodeWrapperCache[el] = obj
	return obj
}
// ─── classList ──────────────────────────────────────────

func makeClassList(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	cls := jsc.NewObject(rt.ObjectPrototype())
	get := func() []string { return strings.Fields(el.GetClassName()) }
	set := func(c []string) {
		el.SetClassName(strings.Join(c, " "))
		InvalidateComputedStyle(el)
		if OnClassChanged != nil {
			OnClassChanged(el)
		}
	}

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
		// ★ 浏览器标准：cssText getter 返回序列化形式——每个声明以分号结尾
		// （Chrome/Firefox 均返回如 "height: 0px; visibility: hidden;"）。
		// JS 端 `style.cssText += "..."`（CM6 gutter spacer 用）依赖这个分号；
		// 原实现原样返回 style 属性（"height:0px" 无分号）导致拼接出
		// "height:0pxvisibility..." 非法声明，spacer 的隐藏/零高样式全失效
		// （行号栏顶部多渲染一个 "99" 测量元素、行号整体下移一行、行号高亮错位）。
		return vm.ToValue(serializeCSSText(s.el.GetAttribute("style")))
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
		InvalidateComputedStyle(s.el)
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
		InvalidateComputedStyle(s.el)
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
	InvalidateComputedStyle(s.el)
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

// serializeCSSText 按浏览器 CSSStyleDeclaration.cssText 序列化 style 属性
// 字符串：每个声明以分号结尾，声明间用空格分隔（浏览器序列化示例：
// "height: 0px; visibility: hidden;"）。这保证 JS 端 `style.cssText += "..."`
// 追加拼接安全（CM6 gutter spacer 依赖）；原实现原样返回 style 属性
// （无分号）会让追加拼出非法声明（"height:0pxvisibility..."），
// 导致声明的 visibility/height 全部失效。
func serializeCSSText(style string) string {
	if strings.TrimSpace(style) == "" {
		return ""
	}
	parts := strings.Split(style, ";")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part+";")
	}
	return strings.Join(out, " ")
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
		if isStyleElement(n) {
			BumpStyleVersion()
			if OnStyleNodeAdded != nil {
				OnStyleNodeAdded(n)
			}
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

// wrapShadowRoot 包装 dom.ShadowRoot 为 JS 对象（最小实现：树导航 + host/mode）。
func wrapShadowRoot(rt *jsc.Interpreter, sr *dom.ShadowRoot) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("ShadowRoot")
	obj.SetInternal(sr)

	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(sr.NodeType()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(sr.NodeName()), nil)
	obj.SetAccessor("mode", strAcc(sr.Mode()), nil)
	obj.SetAccessor("host", nodeAccFn(rt, func() dom.Node { return sr.Host() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return sr.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return sr.LastChild() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, sr.ChildNodes())
	}), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(sr.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { sr.SetTextContent(v.ToString()) })

	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil {
			return jsc.Null()
		}
		sr.AppendChild(n)
		if OnNodeInserted != nil {
			OnNodeInserted(n)
		}
		return a
	})))
	obj.Set("removeChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil {
			return jsc.Null()
		}
		sr.RemoveChild(n)
		if OnNodeRemoved != nil {
			OnNodeRemoved(n)
		}
		return a
	})))

	return obj
}

// ─── Text / Comment ────────────────────────────────────

// wrapText 创建 Text 节点包装器。★ 惰性属性（同 wrapElement 模式）：
// 属性在首次访问时经 installTextProperty（bindings/lazytext.go）物化。
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
	props := &lazyTextProps{
		t:       t,
		interp:  rt,
		cached:  map[string]jsc.JSValue{},
		expando: map[string]jsc.JSValue{},
	}
	obj := jsc.NewLazyObject(rt, proto, props)
	obj.SetInternal(t)
	nodeWrapperCache[t] = obj
	return obj
}
// ─── TreeWalker（document.createTreeWalker / NodeFilter 常量）───
// CodeMirror 6 与前端文本测量用 createTreeWalker 遍历文本节点（SHOW_TEXT）。

// wrapTreeWalker 创建一个 JS TreeWalker 对象，包装 dom.TreeWalker。
func wrapTreeWalker(rt *jsc.Interpreter, w *dom.TreeWalker) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("TreeWalker")
	obj.SetInternal(w)

	obj.Set("root", nodeToJS(rt, w.Root()))
	obj.Set("whatToShow", jsc.NumberValue(float64(w.WhatToShow())))
	obj.Set("currentNode", nodeToJS(rt, w.CurrentNode()))
	syncCurrent := func(in *jsc.Interpreter) {
		obj.Set("currentNode", nodeToJS(in, w.CurrentNode()))
	}
	obj.Set("nextNode", jsc.FunctionValue(jsc.NewNativeFunction("nextNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := w.NextNode(); n != nil {
				syncCurrent(in)
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	obj.Set("previousNode", jsc.FunctionValue(jsc.NewNativeFunction("previousNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := w.PreviousNode(); n != nil {
				syncCurrent(in)
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	obj.Set("parentNode", jsc.FunctionValue(jsc.NewNativeFunction("parentNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := w.ParentNode(); n != nil {
				syncCurrent(in)
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	obj.Set("firstChild", jsc.FunctionValue(jsc.NewNativeFunction("firstChild",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := w.FirstChild(); n != nil {
				syncCurrent(in)
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	obj.Set("lastChild", jsc.FunctionValue(jsc.NewNativeFunction("lastChild",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if n := w.LastChild(); n != nil {
				syncCurrent(in)
				return nodeToJS(in, n)
			}
			return jsc.Null()
		}, 0)))
	return obj
}

// ─── Range（CodeMirror 6 文本测量依赖：textRange → getClientRects）───

// rangeState 保存 Range 对象的边界。
type rangeState struct {
	startNode dom.Node
	startOff  int
	endNode   dom.Node
	endOff    int
}

// wrapRange 创建一个 JS Range 对象。CM6 的 TextWidth.measure 用
// document.createRange() + setEnd/setStart + getClientRects 探测字符宽度
// 与行高；缺 createRange 时测量抛异常，HeightOracle 停留在默认
// lineHeight=14，行号栏按 14px/行步进而内容按真实行高 18.2px，逐行错位。
func wrapRange(rt *jsc.Interpreter, sn dom.Node, so int, en dom.Node, eo int) *jsc.JSObject {
	st := &rangeState{startNode: sn, startOff: so, endNode: en, endOff: eo}
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Range")
	obj.SetInternal(st)

	nodeVal := func(arg jsc.JSValue) dom.Node {
		return unwrapNode(arg)
	}

	obj.Set("setStart", jsc.FunctionValue(jsc.NewNativeFunction("setStart",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.startNode, st.startOff = n, int(args[1].ToNumber())
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("setEnd", jsc.FunctionValue(jsc.NewNativeFunction("setEnd",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.endNode, st.endOff = n, int(args[1].ToNumber())
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("setStartBefore", jsc.FunctionValue(jsc.NewNativeFunction("setStartBefore",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.startNode, st.startOff = n, 0
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("setEndBefore", jsc.FunctionValue(jsc.NewNativeFunction("setEndBefore",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.endNode, st.endOff = n, 0
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("setStartAfter", jsc.FunctionValue(jsc.NewNativeFunction("setStartAfter",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.startNode, st.startOff = n, 1
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("setEndAfter", jsc.FunctionValue(jsc.NewNativeFunction("setEndAfter",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.endNode, st.endOff = n, 1
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("collapse", jsc.FunctionValue(jsc.NewNativeFunction("collapse",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st.endNode, st.endOff = st.startNode, st.startOff
			return jsc.Undefined()
		}, 0)))
	obj.Set("selectNode", jsc.FunctionValue(jsc.NewNativeFunction("selectNode",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.startNode, st.endNode = n, n
				st.startOff, st.endOff = 0, 1
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("selectNodeContents", jsc.FunctionValue(jsc.NewNativeFunction("selectNodeContents",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if n := nodeVal(args[0]); n != nil {
				st.startNode, st.endNode = n, n
				st.startOff, st.endOff = 0, nodeLen(n)
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("deleteContents", jsc.FunctionValue(jsc.NewNativeFunction("deleteContents",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined() // no-op：wb-ui 不依赖 Range 修改 DOM
		}, 0)))
	obj.Set("cloneRange", jsc.FunctionValue(jsc.NewNativeFunction("cloneRange",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapRange(in, st.startNode, st.startOff, st.endNode, st.endOff))
		}, 0)))
	obj.Set("getClientRects", jsc.FunctionValue(jsc.NewNativeFunction("getClientRects",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			var rects []jsc.JSValue
			if l, t, w, h, ok := rangeRect(st); ok {
				r := jsc.NewObject(in.ObjectPrototype())
				r.Set("x", jsc.NumberValue(l))
				r.Set("y", jsc.NumberValue(t))
				r.Set("left", jsc.NumberValue(l))
				r.Set("top", jsc.NumberValue(t))
				r.Set("width", jsc.NumberValue(w))
				r.Set("height", jsc.NumberValue(h))
				r.Set("right", jsc.NumberValue(l+w))
				r.Set("bottom", jsc.NumberValue(t+h))
				rects = append(rects, jsc.ObjectValue(r))
			}
			arr := jsc.NewArray(in.ObjectPrototype(), rects)
			arr.Set("length", jsc.NumberValue(float64(len(rects))))
			return jsc.ObjectValue(arr)
		}, 0)))
	obj.Set("getBoundingClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			r := jsc.NewObject(in.ObjectPrototype())
			l, t, w, h, _ := rangeRect(st)
			r.Set("x", jsc.NumberValue(l))
			r.Set("y", jsc.NumberValue(t))
			r.Set("left", jsc.NumberValue(l))
			r.Set("top", jsc.NumberValue(t))
			r.Set("width", jsc.NumberValue(w))
			r.Set("height", jsc.NumberValue(h))
			r.Set("right", jsc.NumberValue(l+w))
			r.Set("bottom", jsc.NumberValue(t+h))
			return jsc.ObjectValue(r)
		}, 0)))
	obj.SetAccessor("startContainer", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return nodeJS(rt, st.startNode)
	}), nil)
	obj.SetAccessor("endContainer", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return nodeJS(rt, st.endNode)
	}), nil)
	// ★ commonAncestorContainer（浏览器标准）：CM6 DOMObserver 用它判断
	// 变更范围；此前缺失 → 读 undefined → readDOMChange 逻辑异常。
	obj.SetAccessor("commonAncestorContainer", getter(func(in *jsc.Interpreter) jsc.JSValue {
		anc := commonAncestorOf(st.startNode, st.endNode)
		if anc == nil {
			return jsc.Null()
		}
		return nodeJS(in, anc)
	}), nil)
	obj.SetAccessor("startOffset", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(st.startOff))
	}), nil)
	obj.SetAccessor("endOffset", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(st.endOff))
	}), nil)
	obj.SetAccessor("collapsed", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(st.startNode == st.endNode && st.startOff == st.endOff)
	}), nil)
	return obj
}

func nodeLen(n dom.Node) int {
	switch v := n.(type) {
	case *dom.Text:
		return v.Length()
	case *dom.Element:
		c := 0
		for ch := v.FirstChild(); ch != nil; ch = ch.NextSibling() {
			c++
		}
		return c
	}
	return 0
}

// rangeRect 计算 Range 的边界矩形。支持 Text 节点区间（CM6 探测场景）：
// 宽度 = 区间文本的实际渲染宽度（前缀偏移 + 子串测量），高度 = 行高。
// 对非 Text 节点回退到父元素/起始节点的 box rect。
func rangeRect(st *rangeState) (left, top, width, height float64, ok bool) {
	t0 := time.Now()
	defer func() { domStat("rangeRect", time.Since(t0)) }()
	t, isText := st.startNode.(*dom.Text)
	if !isText || st.endNode != st.startNode {
		// 非文本区间：用起始节点所在元素的 box。
		n := st.startNode
		if n == nil {
			n = st.endNode
		}
		if el, eok := n.(*dom.Element); eok && GetElementBoxRect != nil {
			l, t, w, h := GetElementBoxRect(el)
			return l, t, w, h, true
		}
		return 0, 0, 0, 0, false
	}
	data := t.Data()
	from, to := st.startOff, st.endOff
	if from < 0 {
		from = 0
	}
	if to > len(data) {
		to = len(data)
	}
	if to < from {
		from, to = to, from
	}
	sub := data[from:to]
	prefix := data[:from]
	// ★ 位置（left/top）不在这里获取：GetElementBoxRect 内部 forceLayout，
	// CM6 的 TextWidth.measure 在 rAF 中反复调用 getClientRects，若每次都
	// 强制全量布局会造成测量-布局风暴（dummy 插入/移除反复重建渲染树）。
	// CM6 只读 rects[0].width（→ charWidth=width/27）与 height（→
	// textHeight），位置用 0 即可。字体从父元素 computed style 取。
	parent, _ := t.ParentNode().(*dom.Element)
	fam, size, weight, stl := "", 14.0, 400, "normal"
	var elLeft, elTop float64
	if parent != nil {
		if GetElementComputedFont != nil {
			fam, size, weight, stl = GetElementComputedFont(parent)
		}
		// ★ 位置（left/top）：浏览器 getClientRects 返回绝对屏幕坐标。
		// 前端字符定位测量（# 注释对齐、TreeWalker 字符 x 偏移）读
		// rects[0].left 需要真实坐标。★ 用 GetElementBoxRectFast（布局
		// 缓存直读，不强制 rebuild）：CM6 的 TextWidth.measure 在 rAF 里
		// 对每字符调 getClientRects，此时渲染树可能 dirty（DOM 刚变更），
		// 若每次 GetElementBoxRect 全量 forceLayout（rebuild ~22ms）→
		// 测量-布局风暴（每次输入 300ms 的 rangeRect 部分）。文本测量
		// 只读 width/height（位置对 charWidth/textHeight 无贡献），
		// dirty 时的旧几何不影响测量正确性。
		boxFn := GetElementBoxRectFast
		if boxFn == nil {
			boxFn = GetElementBoxRect
		}
		// ★ 裸文本节点优先用自身的 render segment 位置：CM6 把行内
		// `(` / `)  ` 等标点与空格渲染为 cm-line 的裸文本子节点（父元素
		// 是 block 容器）。父 box 的 left = 行首，不含该节点前面兄弟内容
		// 的宽度 → 子区间 rect 恒错（posAtCoords 对「空格多的行」错乱：
		// span 首字符与裸文本标点的 x 全落在行首）。文本节点自身的
		// RenderText segment.X 已含行内全部前缀宽度（绝对坐标）。
		textBaseUsed := false
		if GetTextBasePos != nil {
			if bx, by, ok := GetTextBasePos(t); ok {
				elLeft, elTop = bx, by
				textBaseUsed = true
			}
		}
		if !textBaseUsed {
			if boxFn != nil {
				elLeft, elTop, _, _ = boxFn(parent)
			}
		}
		// ★ 内容从 padding 内侧开始：浏览器 Range 的 left = 父元素 border
		// box 左 + border-left + padding-left（+ 前缀文本宽）。CM6 的
		// .cm-line 有 padding: 0 2px 0 6px（行首 6px 缩进）——漏加则
		// 字符 x 偏移少 6px（# 注释与浏览器错位）。border 默认 0 忽略。
		// ★ textBaseUsed 时 elLeft 已是文本节点首字符的绝对 x（segment.X
		// 已含行内全部前缀 + padding），再加 padding 会双计 6px。
		if !textBaseUsed {
			if cs := computedStyleFor(parent); cs != nil {
				if v, ok := cs["padding-left"]; ok {
					if pv, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil {
						elLeft += pv
					}
				}
				if v, ok := cs["padding-top"]; ok {
					if pv, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil {
						elTop += pv
					}
				}
			}
		}
	}
	prefixW := measureTextWidth(fam, size, weight, stl, prefix)
	width = measureTextWidth(fam, size, weight, stl, sub)
	left += prefixW + elLeft
	top += elTop
	// ★ 高度：浏览器 Range.getClientRects 的高度 = CSS line-height
	// （行框高，含 half-leading），不是字体行距（ascent+descent+lineGap）。
	// CM6 用它作为 textHeight → 光标高度 = textHeight——字体行距会让
	// 光标只有 15.2px（行高 18.2 的 84%），光标顶贴行顶、底空 3.8px →
	// 「光标与 activeLine 背景不居中/平齐」。优先从父元素 computed
	// style 读 line-height（"18.2px" 或无单位倍数 "1.4"）。
	height = measureLineHeight(fam, size, weight, stl)
	// line-height 走继承链：computedStyleFor 只收集元素自身匹配的声明，
	// cm-line 的 line-height 通常声明在 .cm-content/.cm-editor 等祖先。
	// 浏览器 Range.getClientRects 的高度 = 最终 line-height（含继承）。
	// ★ 结果按 parent 缓存：CM6 measure 在 rAF 内对同一父元素的多个字符
	// 反复调 getClientRects，line-height 在输入期间不变，逐层
	// computedStyleFor（~17 层 × 29 次 rangeRect = 495 次/输入）纯浪费。
	if parent != nil {
		if h, found, ok := lineHeightCacheGet(parent); ok {
			if found {
				height = h
			}
		} else {
			found := false
			for p := dom.Node(parent); p != nil; p = p.ParentNode() {
				if pel, ok := p.(*dom.Element); ok {
					if cs := computedStyleFor(pel); cs != nil {
						if v, ok := cs["line-height"]; ok && v != "" && v != "normal" {
							if strings.HasSuffix(v, "px") {
								if pv, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil && pv > 0 {
									height = pv
									found = true
								}
							} else if lh, err := strconv.ParseFloat(v, 64); err == nil && lh > 0 {
								height = lh * size // 无单位倍数：line-height:1.4 → 1.4×font-size
								found = true
							}
							break
						}
					}
				}
			}
			lineHeightCachePut(parent, height, found)
		}
	}
	if width == 0 && height == 0 {
		return 0, 0, 0, 0, false
	}
	return left, top, width, height, true
}

func measureTextWidth(fam string, size float64, weight int, stl, text string) float64 {
	if layout.MeasureTextFunc != nil {
		return layout.MeasureTextFunc(fam, size, weight, stl, text)
	}
	return float64(len([]rune(text))) * size * 0.6
}

func measureLineHeight(fam string, size float64, weight int, stl string) float64 {
	if layout.FontMetricsFunc != nil {
		a, d, g := layout.FontMetricsFunc(fam, size, weight, stl)
		if h := a + d + g; h > 0 {
			return h
		}
	}
	return size * 1.2
}

// nodeJS 返回 node 的 JS 包装（Text → wrapText，Element → wrapElement）。
func nodeJS(rt *jsc.Interpreter, n dom.Node) jsc.JSValue {
	switch v := n.(type) {
	case *dom.Element:
		return jsc.ObjectValue(wrapElement(rt, v))
	case *dom.Text:
		return jsc.ObjectValue(wrapText(rt, v))
	case *dom.Comment:
		return jsc.ObjectValue(wrapComment(rt, v))
	case nil:
		return jsc.Null()
	}
	return jsc.Null()
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
		if isNilNode(n) {
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

// isNilNode reports whether a dom.Node interface value is nil, including
// typed-nil cases (a *dom.Element nil pointer stored in the interface, which
// fails the plain `n == nil` check).
func isNilNode(n dom.Node) bool {
	if n == nil {
		return true
	}
	switch v := n.(type) {
	case *dom.Element:
		return v == nil
	case *dom.Text:
		return v == nil
	case *dom.Comment:
		return v == nil
	case *dom.DocumentFragment:
		return v == nil
	}
	return false
}

// isNilEl reports whether a *dom.Element pointer is nil.
func isNilEl(el *dom.Element) bool { return el == nil }

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
		if isNilEl(els[i]) {
			return jsc.Null()
		}
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

func arrNode(in *jsc.Interpreter, nodes []dom.Node) jsc.JSValue {
	return arrayValue(in, len(nodes), func(i int) jsc.JSValue {
		if isNilNode(nodes[i]) {
			return jsc.Null()
		}
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

// selectorListHasPseudoElement reports whether any complex selector in the
// list contains a pseudo-element simple selector (::before / ::after /
// ::selection / ::placeholder ...). Such rules style the pseudo-element, not
// the element itself, and must not contribute to getComputedStyle(el).
func selectorListHasPseudoElement(l *css.SelectorList) bool {
	if l == nil {
		return false
	}
	for _, cs := range l.Selectors {
		for _, comp := range cs.Compounds {
			for _, s := range comp.Selectors {
				if s.Match == css.MatchPseudoElement {
					return true
				}
			}
		}
	}
	return false
}

func computedStyleFor(el dom.Node) map[string]string {
	t0 := time.Now()
	defer func() { domStat("computedStyleFor", time.Since(t0)) }()
	if el == nil {
		return map[string]string{}
	}
	if cached, ok := cssCacheGet(el); ok {
		return cached
	}
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
				// ★ 伪元素规则（::selection / ::before / ::after 等）不参与元素
				// 本身的 computed style：::selection 的 background 只影响选中
				// 文本高亮，误级联会让 getComputedStyle(el).backgroundColor
				// 返回 ::selection 的背景（如 var(--accent)）。::before/::after
				// 由 ResolvePseudoElement 单独处理。
				if selectorListHasPseudoElement(rl.Selectors) {
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
	// ★ 简写展开：padding/margin 简写 → 各方向子属性。浏览器
	// getComputedStyle 对简写返回展开后的子属性（getPropertyValue('padding-top')
	// 有值）；级联 map 只存简写键时，FitAddon.proposeDimensions 读
	// padding-top → parseInt("") = NaN → 不减 padding → 行数多算
	// （.term-xterm-wrap 151px 容器 9 行 vs 浏览器 8 行）→ 终端内容底部
	// 间隙变小（6px vs 浏览器 19px）。布局层 padding 已生效，只补
	// computed style 的简写展开即可对齐 fit 计算。
	expandBoxShorthand(out, "padding", []string{"padding-top", "padding-right", "padding-bottom", "padding-left"})
	expandBoxShorthand(out, "margin", []string{"margin-top", "margin-right", "margin-bottom", "margin-left"})
	// 解析 var(--xxx) 引用（自定义属性继承链：:root → body → ... → el）。
	// 浏览器语义：自定义属性随级联继承，子元素 var() 引用解析为最近祖先的
	// 定义值。wb-ui 级联 map 本身不含继承值，此处补收集 + 替换。
	resolveVarInComputed(out, el)
	// ★ 浏览器语义：getComputedStyle 的 width/height 返回「实际布局尺寸」
	// （即使无显式 CSS 声明，flex/grid 拉伸的元素也有计算值）。引擎的级联
	// map 只含声明值——FitAddon.proposeDimensions 用
	// getComputedStyle(parent).height 算容器可用高度，声明缺失时 parseInt("")
	// = NaN → fit return → xterm 保持 80x24 超出容器（光标被裁剪）。此处
	// 声明缺失时从渲染树读布局几何兜底（只对无声明元素产生开销）。
	if GetElementBoxRectFast != nil {
		if e, ok := el.(*dom.Element); ok {
			if _, has := out["height"]; !has {
				_, _, _, h := GetElementBoxRectFast(e)
				if h > 0 {
					out["height"] = fmt.Sprintf("%.1fpx", h)
				}
			}
			if _, has := out["width"]; !has {
				_, _, w, _ := GetElementBoxRectFast(e)
				if w > 0 {
					out["width"] = fmt.Sprintf("%.1fpx", w)
				}
			}
		}
	}
	// ★ 浏览器标准：computed style 对每个属性恒有值（未声明 = initial）。
	// padding-*/margin-* 四边未声明时补 0px——FitAddon.proposeDimensions 用
	// getComputedStyle(el).getPropertyValue('padding-top') → parseInt 计算
	// 可用宽高，空字符串 parseInt = NaN → cols=NaN → fit return → 终端恒
	// 80 列（可输出宽度错误）。xterm.css 的 .xterm 无 padding 声明，浏览器
	// 返回 "0px" 而非 ""。
	for _, k := range []string{"padding-top", "padding-right", "padding-bottom", "padding-left",
		"margin-top", "margin-right", "margin-bottom", "margin-left"} {
		if _, ok := out[k]; !ok {
			out[k] = "0px"
		}
	}
	cssCachePut(el, out)
	return out
}

// expandBoxShorthand 把四边简写（padding/margin 等）展开为子属性：
// 1 值 → 四边；2 值 → 上下/左右；3 值 → 上/左右/下；4 值 → 上右下左。
// 仅当简写键存在且子属性键尚未被更具体的规则覆盖时展开。
func expandBoxShorthand(out map[string]string, shorthand string, subs []string) {
	v, ok := out[shorthand]
	if !ok || len(subs) != 4 {
		return
	}
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	var top, right, bottom, left string
	switch len(parts) {
	case 1:
		top, right, bottom, left = parts[0], parts[0], parts[0], parts[0]
	case 2:
		top, bottom = parts[0], parts[0]
		right, left = parts[1], parts[1]
	case 3:
		top = parts[0]
		right, left = parts[1], parts[1]
		bottom = parts[2]
	case 4:
		top, right, bottom, left = parts[0], parts[1], parts[2], parts[3]
	default:
		return
	}
	vals := []string{top, right, bottom, left}
	for i, sub := range subs {
		if _, has := out[sub]; !has {
			out[sub] = vals[i]
		}
	}
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
