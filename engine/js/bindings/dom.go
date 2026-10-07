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

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/js/goja"
	"wb-ui/engine/js/jsc"
	"wb-ui/engine/layout"
)

// OnStyleNodeAdded is an optional callback invoked when a <style> element is
// dynamically added to the DOM (via appendChild/insertBefore). The bindings
// set this from webkit.WebView so the frame can re-extract and apply the new
// styles. When nil, dynamic <style> injection is silently ignored.
var OnStyleNodeAdded func(node dom.Node)

// DocumentReadyState 由宿主（webkit.WebView）注入：按文档返回其所属 frame 的
// readyState（"loading"/"interactive"/"complete"）。未注入时 document.readyState
// 回退 "complete"（脚本能跑就说明文档已可用，比 undefined 安全——库的
// "!== 'loading'" 门禁在 undefined 下恒假）。
var DocumentReadyState func(doc *dom.Document) string

// DocumentStyleSheets 由宿主（webkit.WebView）注入：返回该文档已装配的全部
// 样式表（<style> 提取的 + 运行时注入的 <link rel=stylesheet>）。未注入时
// document.styleSheets 为空列表。
var DocumentStyleSheets func(doc *dom.Document) []*css.CSSStyleSheet

// ViewportWidth / ViewportHeight 为 window.innerWidth/innerHeight 提供值
// （webkit.WebView.Resize 同步）。CM6 的 visiblePixelRange 用它们计算可见
// 像素视口，undefined 会产生 NaN → viewport 永不更新（滚动不重渲染行号）。
// ★ 多 WebView 场景：ViewportSizeForInterpreter（webkit 注入）按解释器
// 分派，优先返回所属 WebView 的实际视口——挂件 Resize 不再覆盖配置窗口
// 的 innerWidth。
var (
	ViewportWidth  float64
	ViewportHeight float64

	// DevicePixelRatio 为 window.devicePixelRatio 提供值（设备像素比，
	// 默认 1）。宿主（webkit.WebView.SetDeviceScaleFactor，或 CDP 的
	// Emulation.setDeviceMetricsOverride{deviceScaleFactor}）设置后页面
	// **立即**读到新值——设备像素比是运行时可变的环境量，不能像早期实现
	// 那样硬编码成常量。
	DevicePixelRatio float64

	// ViewportSizeForInterpreter 按 JS 解释器返回其所属 WebView 的视口尺寸。
	ViewportSizeForInterpreter func(in *jsc.Interpreter) (w, h float64, ok bool)

	// DevicePixelRatioForInterpreter 按 JS 解释器返回其所属 WebView 的设备
	// 像素比（多 WebView 场景：每个 WebView 各自仿真自己的 DSF，互不覆盖）。
	DevicePixelRatioForInterpreter func(in *jsc.Interpreter) (d float64, ok bool)
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

// OnFullscreenChanged is an optional callback invoked when the document's
// fullscreen element changes (HTML §4.11.6). Style recalculation does not need
// it: the implementation also fires OnClassChanged, which is the existing
// "computed style invalid + render tree dirty" path. Embedders use this hook
// when they want to do something host-specific (e.g. toggle a real window).
var OnFullscreenChanged func(el *dom.Element)

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
	// GetElementComputedSnapshot 返回元素在**渲染树**上的 resolved computed
	// style 关键属性快照（含继承值、font-size 已绝对化为 px、颜色归一化为
	// rgb(...)）。getComputedStyle 用它补齐「继承值 / 计算值」——
	// computedStyleFor 的级联 map 只含元素**自身命中**的声明（见 D2 说明）。
	// 元素不在渲染树（display:none 子树 / 尚未布局）时返回 nil。
	GetElementComputedSnapshot func(el *dom.Element) map[string]string
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
	// ★ 不要改用 GetElementBoxRectFast（2026-09-27 实测回归）：虽然宿主
	// 是在 EnsureLayout 之后调用本函数、看似"布局已完成，直读缓存足够"，
	// 但 getElementBoxRect 开头的 forceLayout()（RebuildRenderTreeIfNeeded
	// 绕过 rebuild cooldown 的强制重建）正是让渲染树在帧末回到 clean 的
	// 关键。改走 Fast（跳过强制重建）后脏状态会跨帧累积，实测 IDE 页面
	// layout 从 ~80ms 恶化到 ~350ms（total 200ms → 450ms，两次独立测量
	// 一致；RO 本身尺寸稳定、无回调风暴）。
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

// MediaQueryContextForInterpreter 是同一份上下文，但**按解释器归属**解析。
//
// 为什么需要它：Provider 是包级单例，多 WebView（多文档、多解释器）场景下
// 它只能指向其中一个 —— 实测两个 WebView 同存时，被测 WebView 调
// SetDeviceScaleFactor(2) 后 matchMedia("(min-resolution: 2dppx)") 仍按**另一个**
// WebView 的像素比求值（命中结果与 getComputedStyle 自相矛盾）。因此
// matchMedia 优先用它，Provider 仅作「拿不到归属」时的兜底。
var MediaQueryContextForInterpreter func(in *jsc.Interpreter) *css.MediaQueryContext

// MediaQueryContextForElement 是同一份上下文，但**按元素归属**解析
// （getComputedStyle 的 @media 判定用，见 computedStyleFor → mediaMatches）。
//
// 为什么需要它：computedStyleFor 在 bindings 内自己走一遍级联（不走 resolver），
// 其 @media 判定原先只能用包级 Provider —— 比 matchMedia 的 ForInterpreter
// 少一层归属信息。多 WebView 同进程时（主窗口 + 挂件窗口，或同包测试并存多个
// WebView），Provider 只能指向其中一个：实测单独跑一个 DSF 用例通过，与另一个
// 建 WebView 的用例并跑就失败（读到另一个 WebView 的像素比 → DSF=2 时
// @media (min-resolution: 2dppx) 不匹配，getComputedStyle 停在基础规则，
// 与 matchMedia 已命中自相矛盾）。按元素归属后每个页面只用自己的视口与像素比。
var MediaQueryContextForElement func(n dom.Node) *css.MediaQueryContext

// windowEventListeners 存储 window 上的事件监听器（window.dispatchEvent
// 真分发用）。key 为事件类型字符串（如 "resize"、"message"、自定义事件）。
var windowEventListeners = map[string][]jsc.JSValue{}

// DOM prototype objects — set by RegisterDOMBindings, used by wrappers.
var (
	domElementProto *jsc.JSObject // Element.prototype
	domTextProto    *jsc.JSObject // Text.prototype
	domCommentProto *jsc.JSObject // Comment.prototype
	domDocFragProto *jsc.JSObject // DocumentFragment.prototype
	domAttrProto    *jsc.JSObject // Attr.prototype（NamedNodeMap 的条目）

	// domNamedNodeMapProto 是 NamedNodeMap.prototype：el.attributes 返回的
	// 集合以此为原型，`el.attributes instanceof NamedNodeMap` 经原型链成立
	// （React 19 的水合契约据此判断 attributes 是否为真实集合对象）。
	domNamedNodeMapProto *jsc.JSObject
)

// CurrentScriptElement 是 document.currentScript 的取值来源：page.Frame 在
// 执行某个 <script> 期间把它设为该元素，执行结束清空（其余时刻为 null，
// 同 HTML §4.12.1 currentScript 语义）。React 19 用它定位正在执行的宿主
// 脚本；缺失时读到 undefined，`head.appendChild(undefined)` 抛错并使整个
// 水合脚本中断。
var CurrentScriptElement *dom.Element

// FireResourceEvent 触发元素上的资源事件（load/error）：先调 on<type> 属性
// 处理器，再走 dispatchEvent（addEventListener 注册的监听器）。
//
// 宿主在异步资源到位后调用——运行时动态 <script src> 的 onload 就走这里
// （浏览器语义：经典脚本取回并执行后派发 load，取回/执行失败派发 error）。
func FireResourceEvent(rt *jsc.Interpreter, el *dom.Element, typ string) {
	if rt == nil || el == nil {
		return
	}
	wrapped := wrapElement(rt, el)
	if wrapped == nil {
		return
	}
	// ★ 事件对象必须在**当前解释器**的 runtime 里构造：
	//   jsc.NewObject(nil) 会新建独立 goja runtime，跨 runtime 使用即抛
	//   "Illegal runtime transition of an Object"。
	ev := rt.ObjectPrototype()
	ev.SetClassName("Event")
	ev.Set("type", jsc.StringValue(typ))
	ev.Set("target", jsc.ObjectValue(wrapped))
	evVal := jsc.ObjectValue(ev)
	if h, ok := wrapped.GetByKey("on" + typ); ok && h.IsCallable() {
		_, _ = rt.Call(h, jsc.ObjectValue(wrapped), []jsc.JSValue{evVal})
	}
	if d, ok := wrapped.GetByKey("dispatchEvent"); ok && d.IsCallable() {
		_, _ = rt.Call(d, jsc.ObjectValue(wrapped), []jsc.JSValue{evVal})
	}
}

// namedNodeMapCache 缓存每个元素的 attributes 集合对象：DOM 规定
// `el.attributes === el.attributes`（同一 NamedNodeMap 实例），因此不能每次
// 访问都新建对象。集合内容保持 live —— 每次属性读/写后由 refreshNamedNodeMap
// 同步 length 与数字索引。生命周期与 nodeWrapperCache 一致（clearNodeCache
// 时一并清空，避免持有旧文档节点的强引用）。
// namedNodeMapEntry 保存一个元素的 attributes 集合对象，并记住它的
// interpreter（刷新 Attr 条目时以其 ObjectPrototype 兜底，避免 nil 原型）。
type namedNodeMapEntry struct {
	obj    *jsc.JSObject
	interp *jsc.Interpreter
}

var (
	namedNodeMapMu    sync.Mutex
	namedNodeMapCache = map[*dom.Element]*namedNodeMapEntry{}

	// namedNodeMapLenKey 是缓存对象上记录"上次刷新时属性数"的隐藏键，用于
	// 属性减少时清除残留索引（attributes[2] 不应读到已删除的属性）。
	namedNodeMapLenKey = "\x00__wbui_nnm_len"
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

// ScriptSrcChanged 在 <script> 的 src 属性变化且元素已连接文档时调用
// （webkit 注入实现）：按 HTML「prepare a script」语义取回并执行脚本——
// gou-ide 的区域包 client 半正是靠运行时装载编译 bundle 注册槽位。
var ScriptSrcChanged func(el *dom.Element, src string)

// StylesheetHrefChanged 在 <link rel=stylesheet> 的 href 变化（或插入文档）
// 且元素已连接时调用（webkit 注入实现）：运行时动态样式表按浏览器语义
// 取回并接入级联——插件包 CSS 全靠这条路径（client.js 里 createElement('link')
// + rel/href + appendChild）。
var StylesheetHrefChanged func(el *dom.Element, href string)

// ElementFromPoint 实现 document.elementFromPoint（webkit 分派器注入，
// 按解释器归属路由到对应 WebView 的渲染树命中）。视口坐标 → 命中的
// 最顶层元素（层叠感知：z-index/遮罩/弹窗按绘制顺序，后被绘制者在上）。
var ElementFromPoint func(in *jsc.Interpreter, x, y float64) *dom.Element

// ── getComputedStyle 白名单回写表（包级预计算一次）────────────────────────
// normalizeLineHeightComputed 把级联里的 line-height 原始值归一为浏览器
// getComputedStyle 的语义：数值 / 百分比 / em / rem 相对**本元素 computed
// font-size** 解析为绝对长度（px）；`normal` 与已是绝对长度的值原样返回。
//
// 例：`line-height:1.5` + `font-size:16px` → "24px"；`150%` → "24px"；
// `2em` → "32px"；`normal` → "normal"；`20px` → "20px"。
func normalizeLineHeightComputed(lh, fontSize string) string {
	s := strings.TrimSpace(lh)
	if s == "" || s == "normal" {
		return lh
	}
	// 已是绝对长度 / 视口单位：原样返回。
	for _, u := range []string{"px", "pt", "pc", "cm", "mm", "in", "q", "vw", "vh", "vmin", "vmax"} {
		if strings.HasSuffix(s, u) {
			return lh
		}
	}
	fs := 16.0
	if f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(fontSize), "px"), 64); err == nil && f > 0 {
		fs = f
	}
	num := func(str string) (float64, bool) {
		f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
		return f, err == nil
	}
	// rem 必须在 em 之前判："2rem" 也以 "em" 结尾。
	switch {
	case strings.HasSuffix(s, "rem"):
		if f, ok := num(strings.TrimSuffix(s, "rem")); ok {
			return strconv.FormatFloat(f*fs, 'f', -1, 64) + "px"
		}
	case strings.HasSuffix(s, "em"):
		if f, ok := num(strings.TrimSuffix(s, "em")); ok {
			return strconv.FormatFloat(f*fs, 'f', -1, 64) + "px"
		}
	case strings.HasSuffix(s, "%"):
		if f, ok := num(strings.TrimSuffix(s, "%")); ok {
			return strconv.FormatFloat(f/100*fs, 'f', -1, 64) + "px"
		}
	default:
		if f, ok := num(s); ok {
			return strconv.FormatFloat(f*fs, 'f', -1, 64) + "px"
		}
	}
	return lh
}

// computedStylePropWhitelistCSV 是 getComputedStyle 回写到 JS 对象属性上的
// 属性清单，**逐字保留原先函数内字面量的成员与顺序**（含 backgroundRepeat /
// backgroundPosition / backgroundSize 三项重复——保留重复以保证与优化前
// 行为逐字一致：重复项只是重复写同一个键/属性，无副作用）。
// ★ H5：末尾追加 scrollbarGutter —— scrollbar-gutter 的 computed 值。
//
//	注意 H4 已把 scrollbar-gutter 纳入**已知 CSS 属性表**，但 getComputedStyle
//	的输出白名单是**独立**的一张表，两者都要有（H6 踩过同一坑：初始值表只对
//	白名单属性生效）。
//
// ★ 为什么提到包级：该回写循环对**每次** getComputedStyle 调用都执行，原先
//
//	写法在函数内构造 []string 切片、并对每一项跑一次 camelToKebab（字符串
//	扫描 + 可能分配）。真实编辑器 DOM 上 getComputedStyle 1e3 实测 47~62ms
//	（≈47µs/次），而浏览器同操作约 1µs —— 其中一笔固定开销就是每次调用的
//	~120 次转换与 ~120 元素切片分配。预计算后每次调用只剩 map 查找 + cs.Set。
const computedStylePropWhitelistCSV = "color,backgroundColor,background,fontFamily,fontSize,lineHeight,fontWeight,borderColor,width,height,display,position,opacity,visibility,marginTop,marginRight,marginBottom,marginLeft,paddingTop,paddingRight,paddingBottom,paddingLeft,textAlign,whiteSpace,overflow,overflowX,overflowY,overflowWrap,wordBreak,textOverflow,cursor,zIndex,verticalAlign,maxHeight,minHeight,maxWidth,minWidth,borderRadius,boxShadow,userSelect,pointerEvents,top,left,right,bottom,transform,flexDirection,alignItems,justifyContent,fontStyle,fontVariant,letterSpacing,textDecoration,borderTop,borderBottom,borderLeft,borderRight,borderStyle,borderWidth,borderTopStyle,borderRightStyle,borderBottomStyle,borderLeftStyle,borderTopColor,borderRightColor,borderBottomColor,borderLeftColor,alignSelf,flexWrap,backgroundSize,backgroundRepeat,backgroundPosition,backgroundClip,flex,flexGrow,flexShrink,flexBasis,order,objectFit,mixBlendMode,filter,transition,animation,willChange,tableLayout,borderCollapse,direction,writingMode,textTransform,wordSpacing,textIndent,aspectRatio,gridGap,gridColumn,gridRow,borderTopWidth,borderRightWidth,borderBottomWidth,borderLeftWidth,padding,margin,gap,rowGap,columnGap,gridTemplateColumns,gridTemplateRows,boxSizing,float,clear,listStyle,backgroundImage,backgroundRepeat,backgroundPosition,backgroundSize,outline,content,clipPath,listStyleType,borderSpacing,lineBreak,scrollbarGutter,transformOrigin,textWrap"

// computedStylePropEntry 是白名单的一项：prop 是回写到 JS 对象的 camelCase
// 属性名（getComputedStyle(el).fontSize），key 是级联 map 里的 kebab-case 键
// （getPropertyValue('font-size')）；无连字符时两者相同。
type computedStylePropEntry struct {
	prop string
	key  string
}

// computedStylePropEntries 由上面的 CSV 在包加载时展开一次（顺序与 CSV 一致）。
var computedStylePropEntries = func() []computedStylePropEntry {
	props := strings.Split(computedStylePropWhitelistCSV, ",")
	out := make([]computedStylePropEntry, 0, len(props))
	for _, p := range props {
		key := p
		if k := camelToKebab(p); k != p {
			key = k
		}
		out = append(out, computedStylePropEntry{prop: p, key: key})
	}
	return out
}()

// borderWidthLonghandProps 把 border/border-width 简写展开出的四条 longhand
// 回写到对象属性。原先在 getComputedStyle 内用 map 字面量构造（每次调用都
// 分配一个 4 项 map，且迭代顺序随机）；提到包级改为有序切片，语义等价
// （四项互不影响）但零分配、顺序确定。
var borderWidthLonghandProps = []struct{ longhand, prop string }{
	{"border-top-width", "borderTopWidth"},
	{"border-right-width", "borderRightWidth"},
	{"border-bottom-width", "borderBottomWidth"},
	{"border-left-width", "borderLeftWidth"},
}

func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	// ★ 第 19 轮：接口原型注册表按 runtime 隔离 —— 见 domctors.go 的
	// resetAndAdoptDOMRegistry（跨 rt 复用 prototype 会被 goja 拒绝 "Illegal
	// runtime transition of an Object"，导致 document 挂不上全局）。
	resetAndAdoptDOMRegistry(rt, rt.GlobalObject())
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
		registeredListeners = map[listenerKey]*jsListener{}
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

	// ── API 级性能插桩接线（WB_PERF_DISPATCH=1 时有效；默认关闭零开销）──
	// 把 jsc 包的 API 计数/计时设施接到 dom 包的 hook 上：dom 层在进入 listener
	// 回调时置位开关、退出时关闭，本轮 scroll 派发结束时输出分解表。jsc 侧的
	// wrapper 只在「名单内的 API 名 + 开关开启」时才真正安装（见 perfapi.go），
	// 因此默认运行时路径只是原函数直调。
	dom.PerfAPIScopeReset = jsc.PerfAPIReset
	dom.PerfAPIScopeEnter = jsc.PerfAPIEnter
	dom.PerfAPIScopeExit = jsc.PerfAPIExit
	dom.PerfAPIScopeDump = func(lt time.Duration) string { return jsc.PerfAPIDump(12, lt) }

	// ── Selection 单例（提前创建）──────────────────────────────
	// ★ CodeMirror 6 等库依赖 document.getSelection()。幂等分支每次
	// EvalJS/RunJS 前都会新建 document 对象；若 getSelection 只在首次
	// 注册时挂到 docObj，刷新后的 document 将丢失该方法，CM6 初始化
	// 直接抛 "Object has no member 'getSelection'"。因此 Selection
	// 单例必须在幂等分支之前创建，且两处 docObj 都需挂 getSelection。
	// sstate 为包级（见文件尾部附近定义），供 InsertTextAtSelection 使用。
	// ★ 第 19 轮：Selection 接口原型（`document.getSelection() instanceof Selection`）。
	selObj := jsc.NewObject(domIfaceProtoOr("Selection", rt.ObjectPrototype()))
	selObj.SetClassName("Selection")
	selObj.Set("anchorNode", jsc.Null())
	selObj.Set("anchorOffset", jsc.NumberValue(0))
	selObj.Set("focusNode", jsc.Null())
	selObj.Set("focusOffset", jsc.NumberValue(0))
	selObj.Set("isCollapsed", jsc.BooleanValue(true))
	selObj.Set("type", jsc.StringValue("None"))
	// ★ 第 19 轮：rangeCount 初值（规范：无 Range 时为 0；此前只在 addRange/
	// removeAllRanges 时才设置 → 初始读取得到 undefined，与浏览器不一致）。
	selObj.Set("rangeCount", jsc.NumberValue(0))
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

	// ★ window.visualViewport（CSSOM View §4.2 VisualViewport）：视觉视口
	// 对象。前端库用它做视口探测 / 虚拟键盘补偿，一致性夹具直接读
	// visualViewport.width/height——**缺该标识符时属性读取抛
	// ReferenceError，整段脚本中断**（脚本后续语句全部不执行，页面停在
	// 初始状态，表现为"元素没变色"，而不是只有读视口那一行失效）。
	// 无缩放（scale=1）且视觉视口未滚动时，宽高 === innerWidth/innerHeight、
	// offset/page 偏移为 0（CSSOM View §4.2.1），因此与 innerWidth 共用同一
	// 分派（ViewportSizeForInterpreter 优先，多 WebView 各读自己尺寸）。
	// 事件方法按 EventTarget 语义注册为 no-op：wb-ui 尚未合成视觉视口的
	// scroll/resize/zoom 事件，但注册动作本身不应抛异常。
	vv := jsc.NewObject(rt.ObjectPrototype())
	vv.SetAccessor("width", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if ViewportSizeForInterpreter != nil {
			if w, _, ok := ViewportSizeForInterpreter(in); ok {
				return jsc.NumberValue(w)
			}
		}
		return jsc.NumberValue(ViewportWidth)
	}), nil)
	vv.SetAccessor("height", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if ViewportSizeForInterpreter != nil {
			if _, h, ok := ViewportSizeForInterpreter(in); ok {
				return jsc.NumberValue(h)
			}
		}
		return jsc.NumberValue(ViewportHeight)
	}), nil)
	vv.Set("scale", jsc.NumberValue(1))
	for _, name := range []string{"offsetLeft", "offsetTop", "pageLeft", "pageTop"} {
		vv.Set(name, jsc.NumberValue(0))
	}
	vvNoop := func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return jsc.Undefined()
	}
	vv.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener", vvNoop, 2)))
	vv.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener", vvNoop, 2)))
	vv.Set("dispatchEvent", jsc.FunctionValue(jsc.NewNativeFunction("dispatchEvent",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(true) // 无监听者，取消未发生
		}, 1)))
	g.Set("visualViewport", jsc.ObjectValue(vv))

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
	//   → 行高异常、终端内容画到视口外（空白）。CSS 像素渲染 → 默认 1。
	// ★ B3（CDP Emulation 的 deviceScaleFactor）：改成**动态 accessor**——
	//   设备像素比是运行时可变的环境量（CDP 会中途改它），硬编码 1 会让
	//   Emulation 仿真形同虚设（页面永远读到 1）。无 DSF 的宿主读到默认 1，
	//   既有行为不变；多 WebView 按解释器分派（DevicePixelRatioForInterpreter）。
	g.SetAccessor("devicePixelRatio", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if DevicePixelRatioForInterpreter != nil {
			if d, ok := DevicePixelRatioForInterpreter(in); ok {
				return jsc.NumberValue(d)
			}
		}
		d := DevicePixelRatio
		if d <= 0 {
			d = 1
		}
		return jsc.NumberValue(d)
	}), nil)

	// ── DOM Constructors (Go 原生) ─────────────────────────
	// Each constructor's .prototype is extracted and used by the
	// corresponding wrapper function so that `el instanceof Element`
	// and `txt instanceof Text` work correctly (real prototype chain).

	// Prototype objects — used by wrapElement / wrapText / etc.
	var nodeProto, elementProto, htmlElementProto, svgElementProto *jsc.JSObject
	var textProto, commentProto, docFragProto, attrProto, docTypeProto *jsc.JSObject

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

	// DocumentType 构造器（DOM §4.9）：doctype 节点的接口对象，使
	// `document.firstChild instanceof DocumentType` 与其原型链成立。与其它 DOM
	// 构造器一致：不保证可 new（emptyCtor 直接返回 this）。
	docTypeCtor := rt.NewConstructor("DocumentType", emptyCtor)
	g.Set("DocumentType", jsc.FunctionValue(docTypeCtor))

	fragCtor := rt.NewConstructor("DocumentFragment", emptyCtor)
	g.Set("DocumentFragment", jsc.FunctionValue(fragCtor))

	attrCtor := rt.NewConstructor("Attr", emptyCtor)
	g.Set("Attr", jsc.FunctionValue(attrCtor))

	// NamedNodeMap 构造器：只用于原型链与 instanceof（DOM §4.9.3；调用方
	// 不应 new 它——浏览器同样不保证可构造，此处退化为返回 this）。
	nnmCtor := rt.NewConstructor("NamedNodeMap", emptyCtor)
	g.Set("NamedNodeMap", jsc.FunctionValue(nnmCtor))

	// Image 构造器（new Image() → <img> 元素；canvas 2D drawImage 的
	// 图片源、live2d 纹理加载依赖）。
	imgCtor := rt.NewConstructor("Image", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		el := document.CreateElement("img")
		return wrapElement(in, el)
	})
	g.Set("Image", jsc.FunctionValue(imgCtor))

	// ── 媒体文本轨道（<track>/TextTrack 家族，HTML §4.8.11）──
	// HTMLTrackElement.track / HTMLMediaElement.textTracks 的载体类型：
	// TextTrack、TextTrackCueList、TextTrackList、VTTCue。
	registerMediaTypes(rt, g)

	// ── 媒体元素（HTMLMediaElement/HTMLVideoElement/HTMLAudioElement）──
	// 属性与方法在 engine/js/bindings/media_element.go（原型链在下方接到 HTMLElement）。
	registerMediaElementTypes(rt, g)

	// Extract .prototype objects
	nodeProto = jsc.FunctionValue(nodeCtor).AsObject().GetStr("prototype").AsObject()
	elementProto = jsc.FunctionValue(eltCtor).AsObject().GetStr("prototype").AsObject()
	htmlElementProto = jsc.FunctionValue(htmlCtor).AsObject().GetStr("prototype").AsObject()
	svgElementProto = jsc.FunctionValue(svgCtor).AsObject().GetStr("prototype").AsObject()
	textProto = jsc.FunctionValue(textCtor).AsObject().GetStr("prototype").AsObject()
	commentProto = jsc.FunctionValue(commentCtor).AsObject().GetStr("prototype").AsObject()
	docTypeProto = jsc.FunctionValue(docTypeCtor).AsObject().GetStr("prototype").AsObject()
	docFragProto = jsc.FunctionValue(fragCtor).AsObject().GetStr("prototype").AsObject()
	attrProto = jsc.FunctionValue(attrCtor).AsObject().GetStr("prototype").AsObject()

	// Build prototype chain via __proto__ (goja supports __proto__).
	elementProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	htmlElementProto.Set("__proto__", jsc.ObjectValue(elementProto))
	svgElementProto.Set("__proto__", jsc.ObjectValue(elementProto))
	textProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	commentProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	docTypeProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	docFragProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	attrProto.Set("__proto__", jsc.ObjectValue(nodeProto))
	// <video>/<audio> 包装器使用 HTMLVideoElement/HTMLAudioElement 原型
	// （→ HTMLMediaElement → HTMLElement），因此 `video instanceof HTMLMediaElement`
	// 成立、且媒体方法仍在原型链上可达。
	if domMediaProto != nil {
		domMediaProto.Set("__proto__", jsc.ObjectValue(htmlElementProto))
	}

	// Store for wrapper functions (package-level).
	domElementProto = elementProto
	domTextProto = textProto
	domCommentProto = commentProto
	domDocumentTypeProto = docTypeProto
	domDocFragProto = docFragProto
	domAttrProto = attrProto
	domNamedNodeMapProto = jsc.FunctionValue(nnmCtor).AsObject().GetStr("prototype").AsObject()

	// ── HTML*Element 构造器体系（HTML 规范 §4）──
	// 建立全部具体元素接口构造器 + tag → prototype 分派表（htmlelements.go）。
	// 必须在此处调用：依赖上面接好的
	// HTMLElement.prototype.__proto__ = Element.prototype。
	registerHTMLElementTypes(rt, g, htmlElementProto, svgElementProto)

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
		invalidateNamedNodeMap(el)
		// ★ computed style 缓存失效（class/style 与 IDL 状态属性影响样式
		// 匹配）：按属性分流——状态属性（checked/disabled/open…）的影响会
		// 波及兄弟/后继组合器，需父级范围；其余按 el 子树。
		invalidateAttrChange(el, name)
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
		invalidateNamedNodeMap(el)
		invalidateAttrChange(el, name)
		return jsc.Undefined()
	})
	protoAttr("toggleAttribute", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Undefined()
		}
		name := args[0].ToString()
		if el.HasAttribute(name) {
			el.RemoveAttribute(name)
			invalidateNamedNodeMap(el)
			invalidateAttrChange(el, name)
			return jsc.BooleanValue(false)
		}
		el.SetAttribute(name, "")
		invalidateNamedNodeMap(el)
		invalidateAttrChange(el, name)
		return jsc.BooleanValue(true)
	})

	// ── Element.prototype.hasAttributes / removeAttributeNode ──
	// removeAttributeNode 是 React 19 清空属性的路径：它持有一次
	// el.attributes 引用，循环里反复 removeAttributeNode(map[0])，依赖集合
	// 的 live 语义（length 与索引随删除推进），缺失该方法时直接抛
	// TypeError。
	protoAttr("hasAttributes", 0, func(el *dom.Element, _ []jsc.JSValue) jsc.JSValue {
		return jsc.BooleanValue(el.HasAttributes())
	})
	protoAttr("removeAttributeNode", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 || !args[0].IsObject() {
			return jsc.Null()
		}
		ao := args[0].AsObject()
		if ao == nil {
			return jsc.Null()
		}
		name := ""
		if v, ok := ao.GetByKey("name"); ok {
			name = v.ToString()
		}
		// 浏览器在属性不存在时抛 NotFoundError；此处返回 null（调用方按
		// "移除失败"处理即可，不中断脚本）。
		if name == "" || !el.RemoveAttribute(name) {
			return jsc.Null()
		}
		invalidateNamedNodeMap(el)
		invalidateAttrChange(el, name)
		return args[0]
	})

	// ── Element.prototype.scrollTo / scrollBy（CSSOM View）──
	// 支持对象参数（{left, top, behavior}）与位置参数（scrollTo(x, y)）。
	// 现代框架的 ref 回调用 scrollTo({left, top, behavior: "auto"}) 恢复
	// 滚动位置，缺失方法时抛 TypeError 中断整个水合脚本。behavior 为滚动
	// 动画提示（"smooth"），wb-ui 立即到位（无合成器动画）——位置语义一致。
	scrollWithElement := func(el *dom.Element, args []jsc.JSValue, relative bool) {
		dx, dy := 0.0, 0.0
		if len(args) > 0 && args[0].IsObject() {
			if o := args[0].AsObject(); o != nil {
				if v, ok := o.GetByKey("left"); ok {
					dx = v.ToNumber()
				}
				if v, ok := o.GetByKey("top"); ok {
					dy = v.ToNumber()
				}
			}
		} else {
			if len(args) > 0 {
				dx = args[0].ToNumber()
			}
			if len(args) > 1 {
				dy = args[1].ToNumber()
			}
		}
		if math.IsNaN(dx) {
			dx = 0
		}
		if math.IsNaN(dy) {
			dy = 0
		}
		if SetElementScrollOffset == nil {
			return
		}
		cx, cy := 0.0, 0.0
		if GetElementScrollOffset != nil {
			cx, cy = GetElementScrollOffset(el)
		}
		if relative {
			dx, dy = cx+dx, cy+dy
		}
		// 越界/非滚动容器由实现方钳制（与 scrollTop 赋值同一语义）。
		SetElementScrollOffset(el, dx, dy)
	}
	protoAttr("scrollTo", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		scrollWithElement(el, args, false)
		return jsc.Undefined()
	})
	protoAttr("scrollBy", 1, func(el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		scrollWithElement(el, args, true)
		return jsc.Undefined()
	})

	// ★ 第 18 次监督轮：类别① 第一批标准 DOM 方法（ParentNode/ChildNode、
	// getElementsByTagName/getElementsByClassName（live 集合）、insertAdjacent*、
	// isEqualNode/isSameNode/webkitMatchesSelector、命名空间属性族
	// setAttributeNS/getAttributeNS/hasAttributeNS/removeAttributeNS/getAttributeNames）。
	// 全部实现见 elemdomapi.go；装在 Element.prototype 上，与上面的 protoAttr
	// 属性方法同族（浏览器里它们同样位于 Element.prototype）。
	installElementProtoDOMMethods(elementProto)

	// location：与**文档 URL** 联动。
	//
	// 此前是写死 "about:blank"/"file:" 的静态桩——LoadURL 导航后页面脚本
	// 读 location 拿到的仍是 about:blank，按 location 分支的前端路由会走
	// 错分支；而引擎解析相对引用读的是 doc.URL，两者必须同源（同一个
	// 文档 URL，一次 SetDocumentURL 全部生效）。
	loc := jsc.NewObject(rt.ObjectPrototype())
	locHref := func() string {
		if u := document.URL(); u != "" {
			return u
		}
		return "about:blank"
	}
	locURL := func() *url.URL {
		u, err := url.Parse(locHref())
		if err != nil || u == nil {
			return &url.URL{Path: locHref()}
		}
		return u
	}
	loc.SetAccessor("href", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(locHref())
	}), func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
		// `location.href = url` 等价于 location.assign(url)（浏览器语义）。
		// 此前只有 getter——赋值被静默丢弃，靠 href 赋值做跳转的代码全部失效。
		requestNavigation(in, document, v.ToString(), NavPush)
	})
	loc.SetAccessor("protocol", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if s := locURL().Scheme; s != "" {
			return jsc.StringValue(s + ":")
		}
		return jsc.StringValue("")
	}), nil)
	loc.SetAccessor("host", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(locURL().Host)
	}), nil)
	loc.SetAccessor("hostname", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(locURL().Hostname())
	}), nil)
	loc.SetAccessor("port", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(locURL().Port())
	}), nil)
	loc.SetAccessor("pathname", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		p := locURL().EscapedPath()
		if p == "" {
			// about:blank / 无 URL：保持历史上的 "/"。
			p = "/"
		}
		return jsc.StringValue(p)
	}), nil)
	loc.SetAccessor("search", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if q := locURL().RawQuery; q != "" {
			return jsc.StringValue("?" + q)
		}
		return jsc.StringValue("")
	}), nil)
	loc.SetAccessor("hash", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if f := locURL().Fragment; f != "" {
			return jsc.StringValue("#" + f)
		}
		return jsc.StringValue("")
	}), func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
		// `location.hash = "#x"` / `= "x"`：**同文档**导航——不重新加载文档，
		// 只改 URL 的 fragment + 滚动到锚点 + 派发 hashchange（浏览器语义；
		// 同文档导航同样产生历史条目）。此前 hash 只有 getter，赋值被静默
		// 丢弃，靠 hash 做锚点跳转/单页路由的页面全部失效。
		requestFragmentNavigation(in, document, v.ToString())
	})
	loc.SetAccessor("origin", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		u := locURL()
		switch u.Scheme {
		case "http", "https", "file":
			return jsc.StringValue(u.Scheme + "://" + u.Host)
		}
		return jsc.StringValue("")
	}), nil)
	loc.Set("assign", jsc.FunctionValue(jsc.NewNativeFunction("assign",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				requestNavigation(in, document, args[0].ToString(), NavPush)
			}
			return jsc.Undefined()
		}, 1)))
	loc.Set("replace", jsc.FunctionValue(jsc.NewNativeFunction("replace",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				requestNavigation(in, document, args[0].ToString(), NavReplace)
			}
			return jsc.Undefined()
		}, 1)))
	loc.Set("reload", jsc.FunctionValue(jsc.NewNativeFunction("reload",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			requestReload(in)
			return jsc.Undefined()
		}, 0)))
	g.Set("location", jsc.ObjectValue(loc))

	// ─── Navigation State ───
	// 历史栈是**按解释器注册**的包级对象（不是本函数的局部变量）：宿主的
	// 文档导航（LoadHTML/LoadURL 装配、location.assign 触发的导航）完成后要
	// 回写它（见 navigation.go 的 NoteDocumentNavigation）——history.length
	// 因此反映真实文档数、back/forward 能遍历到上一个文档。
	navState := navStateFor(rt)
	updateLocation := func(url string) {
		// pushState/replaceState 改的是**文档 URL**（同源路径相对当前文档
		// 解析），location 的 accessor 自动反映新值——不再直接写 location
		// 的字段（写字段会与 accessor 打架，且 document.URL 不跟着变）。
		document.SetURL(dom.ResolveURL(document.URL(), url))
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
		} else {
			hist.Set("state", jsc.Null())
		}
		hist.Set("length", jsc.NumberValue(float64(len(navState.entries))))
	}
	// 装配注册：宿主导航完成后（NoteDocumentNavigation）需要回写 length/state。
	navState.hist = hist
	navState.refresh = updateHistState
	dispatchPopstate := func() {
		stateVal := jsc.Null()
		if navState.index >= 0 && navState.index < len(navState.entries) && navState.entries[navState.index].state != nil {
			stateVal = jsc.StringValue(fmt.Sprintf("%v", navState.entries[navState.index].state))
		}
		for _, l := range navState.popListeners {
			if l.eventType != "" && l.eventType != "popstate" {
				continue // hashchange 监听器：由 dispatchHashChange 派发
			}
			ev := jsc.NewObject(rt.ObjectPrototype())
			ev.Set("type", jsc.StringValue("popstate"))
			ev.Set("state", stateVal)
			rt.Call(l.fn, jsc.Undefined(), []jsc.JSValue{jsc.ObjectValue(ev)})
		}
	}
	// dispatchHashChange 派发同文档 fragment 导航的 hashchange（HTML 规范：
	// 只在 fragment 真的变化时派发，事件带 oldURL/newURL）。宿主在完成 URL
	// 更新与锚点滚动后调用（见 bindings.DispatchHashChange）。
	dispatchHashChange := func(oldURL, newURL string) {
		for _, l := range navState.popListeners {
			if l.eventType != "hashchange" {
				continue
			}
			ev := jsc.NewObject(rt.ObjectPrototype())
			ev.Set("type", jsc.StringValue("hashchange"))
			ev.Set("oldURL", jsc.StringValue(oldURL))
			ev.Set("newURL", jsc.StringValue(newURL))
			rt.Call(l.fn, jsc.Undefined(), []jsc.JSValue{jsc.ObjectValue(ev)})
		}
	}
	navState.dispatchHash = dispatchHashChange

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
			// 空栈（文档还没经过宿主装配登记，例如纯 bindings 用法）：先补一条
			// 当前文档条目，否则下面的切片会越界。
			if len(navState.entries) == 0 {
				navState.entries = append(navState.entries, navEntry{url: document.URL()})
				navState.index = 0
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
	// goToIndex 是 back/forward/go 的共同实现。目标是**宿主导航条目**且与当前
	// 文档不同 → 请求宿主真的换文档（跨文档遍历；浏览器里这种遍历完成后不派发
	// popstate，popstate 只用于同文档历史遍历——指针由宿主装配完成后的
	// NoteDocumentNavigation(NavTraverse) 移动）。同文档条目（pushState 写的）
	// 仍按原行为移动指针 + 派发 popstate。
	goToIndex := func(target int) {
		if target < 0 || target >= len(navState.entries) {
			return
		}
		e := navState.entries[target]
		if e.hostNavigated && e.url != "" && e.url != document.URL() {
			if !sameDocumentURL(e.url, document.URL()) {
				// 跨文档遍历：请求宿主换文档；条目指针由宿主装配完成后的
				// NoteDocumentNavigation(NavTraverse) 移动。
				if NavigationRequest != nil {
					NavigationRequest(rt, e.url, NavTraverse)
				}
				return
			}
			// 同文档遍历（条目与当前文档只差 fragment）：不换文档。按浏览器
			// 顺序——先移动指针并派发 popstate，再让宿主更新 URL、滚动到锚点、
			// 派发 hashchange（宿主用「当前 URL vs 目标 URL」判断 fragment 是否
			// 真的变化，所以这两步必须在 updateLocation 之前）。
			navState.index = target
			updateHistState()
			dispatchPopstate()
			if FragmentNavigation != nil {
				FragmentNavigation(rt, fragmentOf(e.url), NavTraverse)
			}
			updateLocation(e.url)
			return
		}
		navState.index = target
		updateHistState()
		if e.url != "" {
			updateLocation(e.url)
		}
		dispatchPopstate()
	}
	hist.Set("go", jsc.FunctionValue(jsc.NewNativeFunction("go",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			delta := 0
			if len(args) >= 1 && args[0].IsNumber() {
				delta = int(args[0].ToNumber())
			}
			goToIndex(navState.index + delta)
			return jsc.Undefined()
		}, 1)))
	hist.Set("back", jsc.FunctionValue(jsc.NewNativeFunction("back",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			goToIndex(navState.index - 1)
			return jsc.Undefined()
		}, 0)))
	hist.Set("forward", jsc.FunctionValue(jsc.NewNativeFunction("forward",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			goToIndex(navState.index + 1)
			return jsc.Undefined()
		}, 0)))
	updateHistState()
	g.Set("history", jsc.ObjectValue(hist))

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
			// ★ 第 20 轮：返回对象是 CSSStyleDeclaration 实例
			//（此前原型为 Object.prototype → `cs instanceof CSSStyleDeclaration`
			// 为 false；CSSOM §1.2 getComputedStyle 的返回类型）。
			cs := jsc.NewObject(domIfaceProtoOr("CSSStyleDeclaration", in.ObjectPrototype()))
			var computed map[string]string
			// ★ H6：transform-origin 需要在回写阶段实时算（依赖 border-box 几何，
			//   不能进 computedStyleFor 的 per-element 缓存）—— 此处记住元素。
			var targetEl *dom.Element
			if len(args) >= 1 {
				if n := unwrapNode(args[0]); n != nil {
					if e, ok2 := n.(*dom.Element); ok2 {
						targetEl = e
					}
					computed = computedStyleFor(n)
					computed = withDisplayFallback(computed, n)
					// ★ H7：getComputedStyle 的 transform 必须按 CSSOM 归一为
					//   matrix(...)（浏览器行为）。级联里存的是**原始值**
					//   （"translate(20px, 10px)"），直接回写 → 与 Edge 不一致
					//   （g2_transform.html 实测：Edge 得 matrix(1, 0, 0, 1, 20, 10)）。
					//   百分比参照元素 **border-box** 尺寸、em/ex/ch 参照 font-size
					//   —— 只有值里真的含这些单位时才去取（GetElementBoxRect 内部
					//   forceLayout，常态避免开销）。未知/3D 语法归一失败时保留原值。
					if tv, ok := computed["transform"]; ok && tv != "" {
						refW, refH, fs := 0.0, 0.0, 16.0
						if tfNeedsGeometry(tv) {
							if e, eok := n.(*dom.Element); eok {
								if GetElementBoxRect != nil {
									_, _, refW, refH = GetElementBoxRect(e)
								}
								if GetElementComputedFont != nil {
									if _, size, _, _ := GetElementComputedFont(e); size > 0 {
										fs = size
									}
								}
							}
						}
						if norm, nok := css.TransformToMatrix(tv, refW, refH, fs); nok {
							computed["transform"] = norm
						}
					}
					// ★ line-height 的 computed 值归一为**绝对长度**（Edge 实测）：
					//   `line-height:1.5` + `font-size:16px` → getComputedStyle 返回
					//   "24px"；百分比同理（150% → "24px"）；`normal` 与显式长度
					//   （"20px"）保持原样。级联里存的是**原始声明值**（"1.5"），
					//   直接回写 → 与 Edge 不一致（h2_baseline_matrix 的
					//   l_input_lh15 / l_button_lh15 / l_select_lh15 /
					//   l_textarea_lh15 四例：Edge "24px" / wbui "1.5"）。
					//   与 transform 一样只在**回写阶段**归一，不动 computedStyleFor
					//   的 map（布局侧读的是 Length 结构，不走这张表）。
					if lh, ok := computed["line-height"]; ok && lh != "" {
						computed["line-height"] = normalizeLineHeightComputed(lh, computed["font-size"])
					}
					// ★ display 回退：computedStyleFor 的级联只收录**声明过**的属性，
					//   未声明 display 的元素（如 `<span>`、插件注入的 div）不会出现在
					//   表里 → `getComputedStyle(el).display` 返回 undefined，污染一切
					//   依赖它的 JS（实测某个被隐藏的 span 的 disp=undefined）。
					//   此处按 UA 语义补默认值：带 hidden 属性 → "none"；否则按标签的
					//   UA 默认 display 映射（block / inline-block / inline）。
					if _, ok := computed["display"]; !ok {
						if e, ok2 := n.(*dom.Element); ok2 {
							computed["display"] = uaDefaultDisplayFor(e)
						}
					}
				}
			}
			if computed == nil {
				computed = map[string]string{}
			}
			// 常用属性直接回写到对象属性（camelCase，与浏览器一致）
			// ★ 四向 padding/margin 缺一不可：此前白名单只有 paddingTop/Bottom 与
			//   marginTop/Bottom，**缺 paddingRight/Left、marginRight/Left** →
			//   `getComputedStyle(el).paddingLeft/paddingRight` 恒为 undefined
			//   （实测 gou-ide `.chat-input`：pt=8px pb=8px 而 pr/pl=undefined），
			//   一切依赖四向 computed 的 JS 自适应计算拿到 undefined 后走错分支。
			// ★ 白名单与 kebab 键均为包级预计算（computedStylePropEntries），
			//   此处不再每次调用构造切片/跑 camelToKebab。表成员与顺序逐字
			//   等同于原先的字面量（见 computedStylePropWhitelistCSV 注释）。
			for _, entry := range computedStylePropEntries {
				prop, key := entry.prop, entry.key
				if v, ok := computed[key]; ok {
					// ★ 第 23 轮：CSSOM 口径（url token 带双引号 / font-family 去引号）
					//   —— 与 getPropertyValue 路径同口径（Edge 基线
					//   dev/output/wbui-audit/r23base.edge.txt）。
					cs.Set(prop, jsc.StringValue(css.CSSTextValueOf(prop, v)))
				} else if init, ok2 := uaInitialComputedValues[prop]; ok2 {
					// ★ 浏览器保证 computed style 对**每个属性恒有值**（未声明 = CSS 初始值）：
					//   引擎级联 map 只含声明值 → 未声明属性读到 undefined →
					//   parseFloat(cs.fontSize)=NaN 一类分支走错。此处仅补 JS 对象层，
					//   不改 computedStyleFor 的 map（避免影响布局与继承链计算）。
					cs.Set(prop, jsc.StringValue(init))
				}
			}
			// ★ H6：transform-origin 的 computed 值是**绝对 px**（浏览器语义：未声明
			//   时等价于 50% 50%，按 **border-box** 尺寸解析）。Edge 实测 g2_transform：
			//   100×40 的元素 → "50px 20px"；transform-origin:0 0 → "0px 0px"；
			//   40×40 → "20px 20px"；h6_misc_props：left top → "0px 0px"、
			//   25% 75% → "25px 75px"、right bottom → "100px 100px"。
			//   几何随布局变化 → 在**回写阶段**实时算（不进 computedStyleFor 缓存）。
			if targetEl != nil {
				var bw, bh float64
				if GetElementBoxRectFast != nil {
					_, _, bw, bh = GetElementBoxRectFast(targetEl)
				}
				// ★ GetElementBoxRectFast 直读布局缓存 —— 页面内**同步** <script>
				//   执行 getComputedStyle 时布局尚未跑，它返回 0 → transform-origin
				//   落空（实测 h6_misc_props 探针：页面内读全 undefined，而 webshot
				//   的 -js 在渲染后读则为 "50px 50px"）。浏览器语义是「getComputedStyle
				//   前必有布局」，故此处用 GetElementBoxRect（forceLayout）兜底；
				//   布局一旦跑过，Fast 即有值，不会每次都重建。
				if (bw <= 0 || bh <= 0) && GetElementBoxRect != nil {
					_, _, bw, bh = GetElementBoxRect(targetEl)
				}
				if bw > 0 && bh > 0 {
					cs.Set("transformOrigin", jsc.StringValue(
						resolveTransformOrigin(computed["transform-origin"], bw, bh)))
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
			// ★ border / border-width 简写展开：浏览器 getComputedStyle 的
			//   borderTopWidth 等 longhand 由 `border: 1px solid #333`（或
			//   `border-width: 1px 2px`）展开而来。此前未展开 → 读数是 undefined
			//   （实测最小复现：声明了 `border:1px solid #333` 而 bt=undefined），
			//   依赖边框宽度做布局/自适应计算的 JS 全部失效。
			bwTok := func(s string) string {
				for _, f := range strings.Fields(s) {
					// 宽度 token：数字开头（1px / 2px / 0.5em），排除样式与颜色关键字
					if len(f) > 0 && (f[0] == '0' || f[0] == '1' || f[0] == '2' || f[0] == '3' ||
						f[0] == '4' || f[0] == '5' || f[0] == '6' || f[0] == '7' ||
						f[0] == '8' || f[0] == '9' || f[0] == '.') {
						return f
					}
				}
				return ""
			}
			for _, longhand := range []string{"border-top-width", "border-right-width", "border-bottom-width", "border-left-width"} {
				if _, ok := computed[longhand]; ok {
					continue
				}
				w := ""
				if v, ok2 := computed["border-width"]; ok2 {
					w = bwTok(v)
				} else if v, ok2 := computed["border"]; ok2 {
					w = bwTok(v)
				}
				if w != "" {
					computed[longhand] = w
				}
			}
			// ★ 顺序：白名单回写循环在上方**已经执行完**，因此展开得到的 longhand
			//   必须在这里补写回对象（否则 computed 里有了、对象属性仍 undefined ——
			//   实测 `border:1px solid #333` 声明下 bt=undefined 正是此因）。
			// ★ 包级有序表（原先每次调用构造 map 字面量：分配 + 顺序随机）。
			for _, lw := range borderWidthLonghandProps {
				if v, ok := computed[lw.longhand]; ok {
					cs.Set(lw.prop, jsc.StringValue(v))
				}
			}
			cs.Set("getPropertyValue", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyValue",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) == 0 {
						return jsc.StringValue("")
					}
					prop := strings.ToLower(strings.TrimSpace(a[0].ToString()))
					if v, ok := computed[prop]; ok {
						return jsc.StringValue(css.CSSTextValueOf(prop, v))
					}
					if camel := kebabToCamel(prop); camel != prop {
						if v, ok := computed[camel]; ok {
							return jsc.StringValue(css.CSSTextValueOf(camel, v))
						}
					}
					// ★ 浏览器 getPropertyValue 对**每个属性恒有值**（未声明 = CSS 初始值）：
					//   引擎级联 map 只含声明值 → 未声明属性返回 ""，与浏览器不同，
					//   下游 parseFloat("")=NaN 与 parseFloat("16px") 行为差异明显。
					//   与属性访问路径共用 uaInitialComputedValues（camelCase 键）。
					if camel := kebabToCamel(prop); camel != prop {
						if init, ok := uaInitialComputedValues[camel]; ok {
							return jsc.StringValue(init)
						}
					} else if init, ok := uaInitialComputedValues[prop]; ok {
						return jsc.StringValue(init)
					}
					return jsc.StringValue("")
				}, 1)))
			cs.Set("getPropertyPriority", jsc.FunctionValue(jsc.NewNativeFunction("getPropertyPriority",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.StringValue("")
				}, 1)))
			// ★ 第 21 轮：CSSStyleDeclaration 的 item(i)/length（此前只有
			//   getPropertyValue/getPropertyPriority/cssText，item/length 缺失 →
			//   `getComputedStyle(el).length` 为 undefined、item 调用抛 TypeError）。
			//   契约与 Edge 一致：item(i) 返回**属性名**（string）、越界返回 ""
			//   （Edge 实测 item(0) 为 string、越界为 ""）；length 为属性条数
			//   （浏览器对每个 CSS 属性恒有值 → 数量级几百，本引擎按级联 map
			//   的条数报告 —— 夹具只断言 > 0，不比较具体数值）。
			cs.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) == 0 {
						return jsc.StringValue("")
					}
					i := int(a[0].ToNumber())
					if i < 0 || i >= len(computed) {
						return jsc.StringValue("")
					}
					for k := range computed {
						if i == 0 {
							return jsc.StringValue(k)
						}
						i--
					}
					return jsc.StringValue("")
				}, 1)))
			cs.Set("length", jsc.NumberValue(float64(len(computed))))
			cs.Set("cssText", jsc.StringValue(""))
			return jsc.ObjectValue(cs)
		}, 1)))

	// Event 基类构造函数
	g.Set("Event", jsc.FunctionValue(rt.NewConstructor("Event",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			// ★ 第 19 轮：返回 this（goja 按 Event.prototype 构造），使
			// `new Event('x') instanceof Event` 与 ev.constructor.name 成立。
			ev := domCtorThis(in, thisVal, domIfaceProto("Event"))
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
			// ★ 第 19 轮：返回 this（原型链 MouseEvent → UIEvent → Event）。
			ev := domCtorThis(in, thisVal, domIfaceProto("MouseEvent"))
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
			// ★ 第 19 轮：返回 this（原型链 WheelEvent → MouseEvent → UIEvent → Event）。
			ev := domCtorThis(in, thisVal, domIfaceProto("WheelEvent"))
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
			// ★ 第 19 轮：返回 this（原型链 KeyboardEvent → UIEvent → Event）。
			ev := domCtorThis(in, thisVal, domIfaceProto("KeyboardEvent"))
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
			// ★ 第 19 轮：返回 this（原型链 CustomEvent → Event）。
			ev := domCtorThis(in, thisVal, domIfaceProto("CustomEvent"))
			ev.Set("type", jsc.StringValue(""))
			ev.Set("detail", jsc.Null())
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("composed", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					if v, ok := o.GetByKey("detail"); ok {
						ev.Set("detail", v)
					}
					if v, ok := o.GetByKey("bubbles"); ok {
						ev.Set("bubbles", v)
					}
					if v, ok := o.GetByKey("cancelable"); ok {
						ev.Set("cancelable", v)
					}
				}
			}
			return ev
		})))

	// ToggleEvent 构造函数（HTML §4.11.4 / §6.12）：`new ToggleEvent(type,
	// {oldState, newState, source})`。<details> / <dialog> / popover 的
	// toggle 与 beforetoggle 都用它；页面除了用它自己派发 toggle，还普遍用
	// `typeof ToggleEvent !== "undefined"` 做支持检测（缺构造器会被判成不支持）。
	// 属性初值按 IDL：oldState/newState 是空串、source 是 null。
	g.Set("ToggleEvent", jsc.FunctionValue(rt.NewConstructor("ToggleEvent",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			// ★ 第 19 轮：返回 this（原型链 ToggleEvent → Event）。
			ev := domCtorThis(in, thisVal, domIfaceProto("ToggleEvent"))
			ev.SetClassName("ToggleEvent")
			ev.Set("type", jsc.StringValue(""))
			ev.Set("oldState", jsc.StringValue(""))
			ev.Set("newState", jsc.StringValue(""))
			ev.Set("source", jsc.Null())
			ev.Set("bubbles", jsc.BooleanValue(false))
			ev.Set("cancelable", jsc.BooleanValue(false))
			ev.Set("composed", jsc.BooleanValue(false))
			ev.Set("defaultPrevented", jsc.BooleanValue(false))
			if len(args) >= 1 {
				ev.Set("type", jsc.StringValue(args[0].ToString()))
			}
			if len(args) >= 2 && args[1].IsObject() {
				if o := args[1].AsObject(); o != nil {
					for _, k := range []string{"oldState", "newState", "source",
						"bubbles", "cancelable", "composed"} {
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

	// EventTarget 基类（可实例化的非 DOM 事件目标）。
	// 浏览器标准 API：addEventListener / removeEventListener / dispatchEvent。
	// 组件库或自定义事件源（如 WebSocket stub、状态总线）可能直接使用它。
	var etTargetProto *jsc.JSObject
	g.Set("EventTarget", jsc.FunctionValue(rt.NewConstructor("EventTarget",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			obj := jsc.NewObject(etTargetProto)
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

	// ★ DOM 规范：Node 继承 EventTarget（Node.prototype.__proto__ =
	// EventTarget.prototype）。此前缺这一环，导致
	// `document.body instanceof EventTarget` 为 false（浏览器为 true），
	// 事件系统类库（检测目标是否事件目标）会走错分支。
	// 同时把 etTargetProto 回填给上面的构造器闭包，使
	// `new EventTarget() instanceof EventTarget` 成立。
	if etv, ok := g.GetByKey("EventTarget"); ok && etv.IsObject() {
		if p := etv.AsObject().GetStr("prototype").AsObject(); p != nil {
			etTargetProto = p
			if nodeProto != nil {
				nodeProto.Set("__proto__", jsc.ObjectValue(p))
			}
		}
	}

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

	// Worker / MessageEvent：并发脚本执行（独立 goja 运行时 + 消息泵）。
	installWorker(rt, g)

	// DOMParser
	domParserDoc := document // capture for closures
	g.Set("DOMParser", jsc.FunctionValue(rt.NewConstructor("DOMParser",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			// ★ 第 19 轮：返回 this（`new DOMParser() instanceof DOMParser`）。
			obj := domCtorThis(in, thisVal, domIfaceProto("DOMParser"))
			obj.Set("parseFromString", jsc.FunctionValue(jsc.NewNativeFunction("parseFromString",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 2 {
						return jsc.Null()
					}
					html := a[0].ToString()
					div := domParserDoc.CreateElement("div")
					div.SetInnerHTML(html)
					// ★ 第 19 轮：产物接口按 MIME 决定 —— text/html → HTMLDocument，
					// 其余（text/xml、application/xml、image/svg+xml…）→ XMLDocument
					// （DOM §4.5），因此 `p.parseFromString(s,"text/xml") instanceof
					// XMLDocument` 成立。
					iface := "HTMLDocument"
					if mime := a[1].ToString(); strings.Contains(mime, "xml") {
						iface = "XMLDocument"
					}
					mockDoc := jsc.NewObject(domIfaceProtoOr(iface, interp.ObjectPrototype()))
					mockDoc.SetClassName(iface)
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
				obj.Set("origin", jsc.StringValue(obj.GetStr("protocol").ToString()+"//"+obj.GetStr("hostname").ToString()))
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
			// 两参数形式 CSS.supports(property, value)：property 需为已知 CSS
			// 属性且 value 符合该属性语法。此前忽略第二个参数、把属性名当选择器
			// 解析（"flex-flow" 是合法 tag 名 → 恒 true），于是合法与非法值都
			// 被判为支持（flex-flow 夹具的 "CSS supports accepts only the
			// shorthand grammar" 需要拒绝 "row column"/"none"）。
			if len(args) >= 2 {
				prop := strings.ToLower(strings.TrimSpace(args[0].ToString()))
				val := strings.TrimSpace(args[1].ToString())
				return jsc.BooleanValue(cssSupportsDeclaration(prop, val))
			}
			// 单参数形式 CSS.supports(conditionText)：参数是 @supports 的**条件
			// 文本** —— 裸声明 / 括号声明 / not-and-or 组合。
			// ★ Edge 实测（dev/fixtures/webshot/h4_supports_bounds.html）：单参数
			// 并**不是选择器语义** —— 'div > p'、'a:hover'、'display' 在 Edge 一律
			// false。此前此处把参数当选择器解析，'display' 是合法 tag 名 → 恒 true，
			// 与 Edge 相反；'a:hover' 也因 hover 是已知伪类而误判 true。故改为按
			// 条件文本求值（含值语法校验），不再回退到选择器解析。
			return jsc.BooleanValue(cssSupportsCondition(args[0].ToString()))
		}, 1)))
	g.Set("CSS", jsc.ObjectValue(cssObj))

	// window.matchMedia — 基于 css 包真实解析与匹配
	g.Set("matchMedia", jsc.FunctionValue(jsc.NewNativeFunction("matchMedia",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			query := ""
			if len(args) > 0 {
				query = args[0].ToString()
			}
			// ★ 第 20 轮：返回 MediaQueryList 实例（原型 → EventTarget；
			// 此前原型为 Object.prototype → `mq instanceof MediaQueryList` 为 false）。
			mq := jsc.NewObject(domIfaceProtoOr("MediaQueryList", in.ObjectPrototype()))
			mq.Set("media", jsc.StringValue(query))
			ctx := css.MediaQueryContext{
				Width: 1280, Height: 800, DeviceWidth: 1280, DeviceHeight: 800,
				DevicePixelRatio: 1, Orientation: "landscape",
				PrefersColorScheme: "light", Hover: "hover", AnyHover: "hover",
				Pointer: "fine", AnyPointer: "fine",
			}
			// ★ 优先按解释器归属解析（多 WebView 不串台），无归属信息再回退
			// 包级 Provider（单 WebView 宿主的既有接线不变）。
			if MediaQueryContextForInterpreter != nil {
				if c := MediaQueryContextForInterpreter(in); c != nil {
					ctx = *c
				}
			} else if MediaQueryContextProvider != nil {
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

	// setImmediate / clearImmediate（事件循环驱动）。
	// 语义：不参与宏任务的到期排序，在每轮到期定时器之前执行（IE10+/Node）。
	// 存在性很重要——React Scheduler 的宿主回调按
	// isInputPending → setImmediate → MessageChannel → setTimeout 依次回退，
	// 缺前者就落到下一档（调度粒度随之变粗）。
	g.Set("setImmediate", jsc.FunctionValue(jsc.NewNativeFunction("setImmediate",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			el := in.EnsureEventLoop()
			return jsc.NumberValue(float64(el.SetImmediate(args[0])))
		}, 1)))
	g.Set("clearImmediate", jsc.FunctionValue(jsc.NewNativeFunction("clearImmediate",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearImmediate(int(args[0].ToNumber()))
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
				navState.popListeners = append(navState.popListeners, navPopListener{
					fn:        args[1],
					capture:   len(args) >= 3 && args[2].ToBoolean(),
					eventType: eventType,
				})
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
					opts := &dom.MutationObserverOptions{}
					if len(a) >= 2 && a[1].IsObject() {
						optObj := a[1].AsObject()
						if optObj != nil {
							if v, ok := optObj.GetByKey("childList"); ok {
								opts.ChildList = v.ToBoolean()
							}
							if v, ok := optObj.GetByKey("attributes"); ok {
								opts.Attributes = v.ToBoolean()
							}
							if v, ok := optObj.GetByKey("characterData"); ok {
								opts.CharacterData = v.ToBoolean()
							}
							if v, ok := optObj.GetByKey("subtree"); ok {
								opts.Subtree = v.ToBoolean()
							}
							if v, ok := optObj.GetByKey("attributeOldValue"); ok {
								opts.AttributeOldValue = v.ToBoolean()
							}
							if v, ok := optObj.GetByKey("characterDataOldValue"); ok {
								opts.CharacterDataOldValue = v.ToBoolean()
							}
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
					if len(a) < 1 {
						return jsc.Undefined()
					}
					targetObj := a[0].AsObject()
					if targetObj == nil {
						return jsc.Undefined()
					}
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
			// ★ 第 19 轮：返回 this（goja 按 Range.prototype 构造）→
			// `new Range() instanceof Range` 成立。
			r := domCtorThis(in, thisVal, domIfaceProto("Range"))
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
						if v, ok := o.GetByKey(k); ok {
							c.Set(k, v)
						}
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
			if len(a) == 0 {
				return jsc.Null()
			}
			idx := int(a[0].ToNumber())
			if idx >= 0 && idx < len(sstate.ranges) {
				return jsc.ObjectValue(sstate.ranges[idx])
			}
			return jsc.Null()
		}, 1)))
	selObj.Set("addRange", jsc.FunctionValue(jsc.NewNativeFunction("addRange",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.Undefined()
			}
			sel := this.AsObject()
			r := a[0].AsObject()
			sstate.ranges = append(sstate.ranges, r)
			if sc, ok := r.GetByKey("startContainer"); ok {
				sel.Set("anchorNode", sc)
			}
			if so, ok := r.GetByKey("startOffset"); ok {
				sel.Set("anchorOffset", so)
			}
			if ec, ok := r.GetByKey("endContainer"); ok {
				sel.Set("focusNode", ec)
			}
			if eo, ok := r.GetByKey("endOffset"); ok {
				sel.Set("focusOffset", eo)
			}
			sel.Set("rangeCount", jsc.NumberValue(float64(len(sstate.ranges))))
			sel.Set("isCollapsed", jsc.BooleanValue(false))
			sel.Set("type", jsc.StringValue("Range"))
			return jsc.Undefined()
		}, 1)))
	selObj.Set("removeRange", jsc.FunctionValue(jsc.NewNativeFunction("removeRange",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) == 0 {
				return jsc.Undefined()
			}
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
	// ★ 第 19 次监督轮：DOM 接口构造器族 + prototype 链（domctors.go）、
	// Window 身份属性、atob/btoa、Audio/Option。必须在全部既有构造器
	// （Node/Element/HTMLElement/SVGElement/Event 族/Range/DOMParser/
	// MessageEvent/HTML*Element）注册完成之后调用 —— domAdoptIface 复用它们的
	// prototype，domRegisterIface 注册本引擎此前缺失的接口（NodeList/
	// Document/HTMLDocument/ShadowRoot/Selection/DOMRect 族/SVG 接口/事件子类…）。
	registerDOMInterfaces(rt, g)
	registerWindowIdentity(rt, g)
	registerBase64Globals(rt, g)
	registerExtraElementCtors(rt, g, document)
	// ★ WebAudio 最小子集（TC-M-603）：AudioContext / decodeAudioData。与 Audio/Option
	// 同一时机挂载——特性检测（`typeof AudioContext !== 'undefined'`）在 DOM 构造器族
	// 齐备后即可用；宿主解码器由 app.InstallWebAudio 注入（未注入时 decodeAudioData
	// 如实 reject，不编造 buffer）。
	installWebAudio(rt, g)
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
	return wrapDocumentAs(rt, doc, "HTMLDocument")
}

// wrapDocumentAs 同 wrapDocument，但显式指定接口名（HTMLDocument / XMLDocument）。
//
// ★ 第 19 轮：主文档与 DOMImplementation.createHTMLDocument 的产物是
// HTMLDocument，DOMParser.parseFromString(…, "application/xml") 与 createDocument
// 的产物是 XMLDocument（DOM §4.5）。二者 prototype 链均为 → Document.prototype
// → Node.prototype，所以 `document instanceof Document` 与
// `document instanceof HTMLDocument` 同时成立。
//
// ★ 第 17 次监督轮：document 包装对象进 nodeWrapperCache —— 同一 *dom.Document
// 必须只对应一个 JS 对象。否则 `doctype.parentNode === document`、
// `documentElement.parentNode === document`、`el.ownerDocument === document`
// 这些**身份**判断全部失败（库与夹具都用 === 判同一文档），且
// DOMImplementation.createHTMLDocument 造出的第二个文档与它自己的节点也会
// 各自拿到不同包装。
func wrapDocumentAs(rt *jsc.Interpreter, doc *dom.Document, ifaceName string) *jsc.JSObject {
	// ★ 第 17 次监督轮：document 包装对象进 nodeWrapperCache —— 同一 *dom.Document
	// 必须只对应一个 JS 对象。否则 `doctype.parentNode === document`、
	// `documentElement.parentNode === document`、`el.ownerDocument === document`
	// 这些**身份**判断全部失败（库与夹具都用 === 判同一文档），且
	// DOMImplementation.createHTMLDocument 造出的第二个文档与它自己的节点也会
	// 各自拿到不同包装。
	if cached, ok := nodeWrapperCache[doc]; ok {
		return cached
	}
	obj := jsc.NewObject(domIfaceProtoOr(ifaceName, rt.ObjectPrototype()))
	obj.SetClassName(ifaceName)
	obj.SetInternal(doc)

	obj.Set("getElementById", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		if el := doc.GetElementById(arg); el != nil {
			return jsc.ObjectValue(wrapElement(in, el))
		}
		return jsc.Null()
	})))
	// document.currentScript：正在执行的 <script> 元素（HTML §4.12.1），
	// 其余时刻为 null。page.Frame 在每个脚本执行前后设置/清空
	// bindings.CurrentScriptElement。
	obj.SetAccessor("currentScript", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if CurrentScriptElement == nil {
			return jsc.Null()
		}
		return jsc.ObjectValue(wrapElement(in, CurrentScriptElement))
	}), nil)
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
		// ★ 第 16 次监督轮：DOM 层 Element 不建模命名空间（只保留本地标签名，
		//   见 engine/dom/element.go 文件头），绑定层在这里记录显式的非 HTML
		//   命名空间，供 Element.namespaceURI 的值语义使用（elemattr.go）。
		el := doc.CreateElement(arg)
		recordElementNamespace(el, strings.TrimSpace(ns))
		return jsc.ObjectValue(wrapElement(in, el))
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
	// createNodeIterator（DOM §4.4）：此前只有 createTreeWalker，NodeIterator
	// 接口因此没有实例。dom 层 engine/dom/nodeiterator.go 已完整移植，这里接线。
	obj.Set("createNodeIterator", jsc.FunctionValue(jsc.NewNativeFunction("createNodeIterator",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
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
				if fo := args[2].AsObject(); fo != nil {
					if af, ok := fo.GetByKey("acceptNode"); ok && !af.IsNull() && !af.IsUndefined() {
						filter = dom.NodeFilterFunc(func(n dom.Node) dom.NodeFilterResult {
							res, _ := in.Call(af, jsc.Undefined(), []jsc.JSValue{nodeToJS(in, n)})
							return dom.NodeFilterResult(uint16(res.ToNumber()))
						})
					}
				}
			}
			return jsc.ObjectValue(wrapNodeIterator(in, dom.NewNodeIterator(root, what, filter)))
		}, 3)))
	// caretRangeFromPoint / caretPositionFromPoint（CSSOM-View §7.3）：用同一套
	// 层叠命中（ElementFromPoint）定位光标所在的元素，再给出该处的空 Range /
	// CaretPosition。前端用它做「点击处插入光标」「拖拽落点」。
	obj.Set("caretRangeFromPoint", jsc.FunctionValue(jsc.NewNativeFunction("caretRangeFromPoint",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if ElementFromPoint == nil || len(args) < 2 {
				return jsc.Null()
			}
			el := ElementFromPoint(in, args[0].ToNumber(), args[1].ToNumber())
			if el == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapRange(in, el, 0, el, 0))
		}, 2)))
	obj.Set("caretPositionFromPoint", jsc.FunctionValue(jsc.NewNativeFunction("caretPositionFromPoint",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if ElementFromPoint == nil || len(args) < 2 {
				return jsc.Null()
			}
			el := ElementFromPoint(in, args[0].ToNumber(), args[1].ToNumber())
			if el == nil {
				return jsc.Null()
			}
			cp := jsc.NewObject(in.ObjectPrototype())
			cp.SetClassName("CaretPosition")
			cp.Set("offsetNode", nodeToJS(in, el))
			cp.Set("offset", jsc.NumberValue(0))
			cp.Set("getClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getClientRect",
				func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					l, t, w, h := 0.0, 0.0, 0.0, 0.0
					if GetElementBoxRect != nil {
						l, t, w, h = GetElementBoxRect(el)
					}
					return jsc.ObjectValue(makeDOMRect(in, l, t, w, h))
				}, 0)))
			return jsc.ObjectValue(cp)
		}, 2)))
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
		// ★ 第 19 轮：querySelectorAll 返回 **NodeList**（规范类型）；
		// 数组语义保留（NodeList.prototype → Array.prototype，见 domctors.go）。
		return arrElemAs(in, DocumentQuerySelectorAll(doc, sel), "NodeList")
	})))
	obj.Set("getElementsByTagName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByTagName(arg)
		// ★ 第 19 轮：getElementsBy* 返回 **HTMLCollection**（规范类型）。
		return arrElemAs(in, els, "HTMLCollection")
	})))
	obj.Set("getElementsByClassName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByClassName(arg)
		return arrElemAs(in, els, "HTMLCollection")
	})))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(doc)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(doc)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(doc)))

	// Accessors
	obj.SetAccessor("body", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if b := doc.Body(); b != nil {
			return jsc.ObjectValue(wrapElement(in, b))
		}
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
		if h := doc.Head(); h != nil {
			return jsc.ObjectValue(wrapElement(in, h))
		}
		return jsc.Null()
	}), nil)
	obj.SetAccessor("documentElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if de := doc.DocumentElement(); de != nil {
			return jsc.ObjectValue(wrapElement(in, de))
		}
		return jsc.Null()
	}), nil)
	obj.SetAccessor("title",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(doc.Title()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { doc.SetTitle(v.ToString()) })
	// document.URL 必须**动态读**：LoadURL 导航后 location.href 与
	// document.URL 要立刻反映新文档的 URL（相对引用解析也读它）。
	// 此前传的是注册时的值快照（doc.URL() 在装配时求值）→ 恒为空串。
	obj.SetAccessor("URL", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(doc.URL())
	}), nil)
	// document.baseURI：**所有相对引用的解析基准**（浏览器语义）——存在
	// `<base href>` 时是按其解析后的绝对 URL，否则等于 document.URL。
	// 动态读：脚本随时可以插入/修改/删除 `<base>`，浏览器里立即生效。
	obj.SetAccessor("baseURI", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(doc.BaseURL())
	}), nil)
	// 全屏 API（HTML §4.11.6）：状态由 Element.requestFullscreen / 本方法维护，
	// CSS 的 :fullscreen 与 fullscreenchange 事件消费它。是否把宿主窗口真的切到
	// 全屏由宿主决定（见 bindings.OnFullscreenChanged）。
	obj.SetAccessor("fullscreenElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if fe := doc.FullscreenElement(); fe != nil {
			return jsc.ObjectValue(wrapElement(in, fe))
		}
		return jsc.Null()
	}), nil)
	obj.SetAccessor("fullscreenEnabled", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(true)
	}), nil)
	obj.Set("exitFullscreen", jsc.FunctionValue(jsc.NewNativeFunction("exitFullscreen",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return exitFullscreenFor(in, doc)
		}, 0)))
	obj.SetAccessor("cookie", strAcc(""), nil)
	obj.SetAccessor("compatMode", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if doc.Quirks() {
			return jsc.StringValue("BackCompat")
		}
		return jsc.StringValue("CSS1Compat")
	}), nil)

	// document.readyState（HTML §3.1.4）：jQuery ready()、Vue mount 时机探测、
	// 以及大量 `if (document.readyState !== 'loading')` 门禁都直接读它。值由
	// 宿主按 frame 的加载阶段提供（见 DocumentReadyState）。
	obj.SetAccessor("readyState", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if DocumentReadyState != nil {
			return jsc.StringValue(DocumentReadyState(doc))
		}
		return jsc.StringValue("complete")
	}), nil)
	// document.scripts（HTML §3.1.4）：文档内全部 <script> 的实时集合。库用它
	// 做「已加载脚本扫描」（懒加载 / 去重注入）。沿用 getElementsByTagName 的
	// 数组语义 —— HTMLCollection 构造器本身属 P1（覆盖矩阵 §二 B）。
	obj.SetAccessor("scripts", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrElemAs(in, doc.GetElementsByTagName("script"), "HTMLCollection")
	}), nil)
	// document.styleSheets（CSSOM §document.styleSheets）：StyleSheetList。包含
	// <style> 提取的与运行时注入的样式表（后者正是插件 CSS 的通道）。
	// ★ 第 21 轮：列表与其中样式表包装对象**身份稳定** —— CSSOM 要求
	//   `document.styleSheets === document.styleSheets`、
	//   `styleSheets[0] === styleSheets[0]`、`styleSheets[0] === styleSheets.item(0)`
	//   成立（库用引用比较做去重 / 判断样式是否已装配）。此前每次访问都重新包装
	//   → 三个断言全为 false。缓存挂在闭包里（per-runtime —— jsc.JSObject 绑定
	//   创建它的 runtime）；样式表条数变化时重建。
	var sheetList jsc.JSValue
	sheetListLen := -1
	haveSheetList := false
	obj.SetAccessor("styleSheets", getter(func(in *jsc.Interpreter) jsc.JSValue {
		var sheets []*css.CSSStyleSheet
		if DocumentStyleSheets != nil {
			sheets = DocumentStyleSheets(doc)
		}
		if haveSheetList && sheetListLen == len(sheets) {
			return sheetList
		}
		sheetList = wrapStyleSheetList(in, sheets)
		sheetListLen = len(sheets)
		haveSheetList = true
		return sheetList
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

	// ★ 第 17 次监督轮：Document 的 Node/ParentNode/Document 接口族（nodeType/
	// nodeName/ownerDocument/firstChild/lastChild/childNodes/children/
	// firstElementChild/lastElementChild/childElementCount/characterSet/charset/
	// inputEncoding/contentType/documentURI/referrer/implementation/dir/domain/
	// location/forms/images/links/embeds/plugins/anchors/applets/all/
	// adoptedStyleSheets/fonts/hidden/visibilityState/pointerLockElement/
	// pictureInPictureElement/designMode/scrollingElement/timeline/doctype）。
	// 逐项语义与实现见 dociface.go。
	installDocumentIfaceProps(rt, obj, doc)

	// ★ 第 18 次监督轮：Document 侧的类别① 方法（ParentNode 的 append/prepend/
	// replaceChildren、importNode/adoptNode、getElementsByName，以及
	// createAttribute/createProcessingInstruction/createCDATASection）。实现见
	// elemdomapi.go 的 installDocumentDOMMethods。
	installDocumentDOMMethods(rt, obj, doc)

	nodeWrapperCache[doc] = obj
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
	// ★ C-P4-3：style 句柄缓存同样持有元素与 goja 对象，随文档切换一起清
	// （否则旧文档的元素与其 style 对象泄漏，内存探针会看到每次导航累积）。
	styleObjectCache = make(map[*dom.Element]styleObjectEntry)
	namedNodeMapMu.Lock()
	namedNodeMapCache = map[*dom.Element]*namedNodeMapEntry{}
	namedNodeMapMu.Unlock()
}

// namedNodeMapFor 返回元素 attributes 的 NamedNodeMap 包装对象（同元素同
// 实例，满足 `el.attributes === el.attributes`），并在返回前同步为当前属性。
//
// NamedNodeMap 是 live 集合（DOM §4.9.3）：length 与数字索引随属性增删变化。
// React 19 用 `while (map.length) el.removeAttributeNode(map[0])` 清空属性，
// 循环条件与取出的条目都必须反映每次删除后的状态。
func namedNodeMapFor(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	namedNodeMapMu.Lock()
	entry, ok := namedNodeMapCache[el]
	namedNodeMapMu.Unlock()
	if !ok || entry == nil || entry.obj == nil {
		proto := rt.ObjectPrototype()
		if domNamedNodeMapProto != nil {
			proto = domNamedNodeMapProto
		}
		entry = &namedNodeMapEntry{obj: jsc.NewObject(proto), interp: rt}
		namedNodeMapMu.Lock()
		namedNodeMapCache[el] = entry
		namedNodeMapMu.Unlock()
	}
	refreshNamedNodeMap(entry, el)
	return entry.obj
}

// invalidateNamedNodeMap 在元素属性增删后把缓存的 NamedNodeMap 同步到新状态
// （仅当该元素已暴露过 attributes 时才需要——否则下次访问自然会同步）。
func invalidateNamedNodeMap(el *dom.Element) {
	if el == nil {
		return
	}
	namedNodeMapMu.Lock()
	entry, ok := namedNodeMapCache[el]
	namedNodeMapMu.Unlock()
	if ok && entry != nil {
		refreshNamedNodeMap(entry, el)
	}
}

// refreshNamedNodeMap 把元素的属性列表同步到 NamedNodeMap 包装对象：length、
// 每个数字索引对应的 Attr 对象，并清除属性减少后残留的索引。
func refreshNamedNodeMap(entry *namedNodeMapEntry, el *dom.Element) {
	if entry == nil || entry.obj == nil || el == nil {
		return
	}
	obj := entry.obj
	prev := 0
	if v, ok := obj.GetByKey(namedNodeMapLenKey); ok {
		prev = int(v.ToNumber())
	}
	proto := domAttrProto
	if proto == nil && entry.interp != nil {
		proto = entry.interp.ObjectPrototype()
	}
	names := el.AttributeNames()
	for i, name := range names {
		attr := jsc.NewObject(proto)
		attr.Set("name", jsc.StringValue(name))
		attr.Set("value", jsc.StringValue(el.GetAttribute(name)))
		obj.Set(strconv.Itoa(i), jsc.ObjectValue(attr))
	}
	for i := len(names); i < prev; i++ {
		obj.Set(strconv.Itoa(i), jsc.Undefined())
	}
	obj.Set(namedNodeMapLenKey, jsc.NumberValue(float64(len(names))))
	obj.Set("length", jsc.NumberValue(float64(len(names))))
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
		for k, l := range registeredListeners {
			if l.interp == interp {
				delete(registeredListeners, k)
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
	// 媒体文本轨道缓存（<track> → TextTrack / <video> → TextTrackList）与
	// per-interpreter 原型集合：同样持有旧文档节点与解释器引用。
	clearMediaCachesFor(interp, doc)
	dom.ClearObserverRegistryFor(doc)
}

// isStyleElement reports whether n is an HTML <style> element.
func isStyleElement(n dom.Node) bool {
	if n == nil {
		return false
	}
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
// （contenteditable 光标插入）只读 sstate.ranges[0]。
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
// SelectionInsideElement 报告当前 DOM Selection 的起点是否位于 root 子树内。
//
// 用途：contenteditable 的字符插入必须先校验「selection 属于该元素」——
// sstate.ranges 是**全局单例**，会残留其它元素（终端 textarea、上一次 CM6
// 光标、已卸载节点）的旧 selection；此时 InsertTextAtSelection 会把字符插到
// **别处**并返回 true，目标输入框仍为空（实测：ev:input 触发 5 次而
// .chat-input 的 inputDetail len=0/htmlLen=0/kids=0 —— 用户「点击输入框打字
// 无反应」的另一条成因）。
func SelectionInsideElement(root *dom.Element) bool {
	if root == nil || len(sstate.ranges) == 0 {
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
	// ★ 陈旧 range 严格化：起点所在节点已从文档分离（上一个页面 / 已卸载元素）时，
	//   该 selection 对当前元素无效（浏览器语义：分离节点不参与选择命中）。
	//   sstate 是包级单例、跨 WebView 复用，会残留旧页面的 range —— 不校验会把
	//   退格/插入误判成「有有效选区」而走错路径（实测：同包全量测试时退格走了旧的
	//   setFocusedElementValue 路径（对 contenteditable 直接 return，保护 CM6 结构
	//   的正确设计）→ 退格完全无效、textContent 保持 "hello"；单独跑因无残留而正确）。
	if !node.IsConnected() {
		return false
	}
	for n := node; n != nil; n = n.ParentNode() {
		if n == dom.Node(root) {
			return true
		}
	}
	return false
}

// DeleteCharFromContentEditable 在 contenteditable 根的**内容末尾**删除一个字符
// （forward=false 退格：删末尾字符；forward=true：caret 在末尾、其后无字符，不动）。
// 用于「页面从不维护 DOM Selection」的手写 contenteditable：
// sstate.ranges 恒为空 → 选区路径无效；而 setFocusedElementValue 对 contenteditable
// 直接 return（保护 CodeMirror 6 的结构化 DOM，正确）→ 退格完全无效
// （实测：键入 "hello" 后退格 3 次 textContent 仍为 "hello"）。与字符输入的
// AppendTextToContentEditable 对称，作为无 Selection 时的兜底删除。
// 返回 true 表示已改写 DOM。
func DeleteCharFromContentEditable(root *dom.Element, forward bool) bool {
	if root == nil || forward {
		// 向后的 Delete：无 caret 时位置视为内容末尾，其后无字符可删。
		return false
	}
	var target *dom.Text
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		for _, c := range n.ChildNodes() {
			if t, ok := c.(*dom.Text); ok && len([]rune(t.NodeValue())) > 0 {
				target = t
			}
			walk(c)
		}
	}
	walk(root)
	if target == nil {
		return false
	}
	rs := []rune(target.NodeValue())
	if len(rs) == 0 {
		return false
	}
	if err := target.SetNodeValue(string(rs[:len(rs)-1])); err != nil {
		return false
	}
	return true
}

// AppendTextToContentEditable 在 contenteditable 根的**内容末尾**追加文本，用于
// 「无有效 DOM Selection」时的兜底插入（浏览器语义：contenteditable 聚焦后输入的
// 字符必须进入文档；无 caret 时追加到内容末尾）。
//
// 背景：InsertTextAtSelection 依赖 sstate.ranges（DOM Selection 状态），而
// sstate.ranges 只在页面调用 selection.collapse()/addRange()/setBaseAndExtent()
// 时才被填充。CodeMirror 6 每次点击都会写 collapse，所以 CM6 输入区正常；但
// **手写 contenteditable**（gou-ide 对话输入框 .chat-input：只监听 @input/@keydown、
// 从不碰 selection API）在真实点击聚焦后 ranges 恒为空 → InsertTextAtSelection
// 返回 false → 字符被**静默丢弃**（实测证据：ev:input 触发 5 次而 DOM 文本长度为 0，
// 用户表现为「点击输入框后打字无反应」）。
// 返回 true 并同步 sstate.ranges 到插入点之后，保证连续输入逐字追加。
func AppendTextToContentEditable(root *dom.Element, text string) bool {
	if root == nil || text == "" {
		return false
	}
	// 末节点是文本节点时直接续写（保持节点数最小，页面序列化最简）。
	if last := root.LastChild(); last != nil {
		if t, ok := last.(*dom.Text); ok {
			if err := t.SetNodeValue(t.NodeValue() + text); err == nil {
				sstate.updateRangeForInsert(t, len([]rune(text)))
				return true
			}
		}
	}
	doc := root.OwnerDocument()
	if doc == nil {
		return false
	}
	ins := dom.NewText(doc, text)
	if err := root.AppendChild(ins); err != nil {
		return false
	}
	sstate.updateRangeForInsert(ins, len([]rune(text)))
	return true
}

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
	case *dom.DocumentType:
		// ★ 第 17 次监督轮：doctype 节点（document.firstChild / document.doctype
		// 的取值，以及 doctype 兄弟指针的回读都经过这里）。
		return jsc.ObjectValue(wrapDocumentType(in, v))
	case *dom.Document:
		// ★ 第 17 次监督轮：节点的父节点可能是 Document（doctype.parentNode、
		// html.parentNode、attribute 之外的 ownerDocument 链）——必须返回
		// **document 包装对象本身**才能满足 `=== document`（此前落到 default
		// → 返回 null，夹具实测 doctype.parentNode.is.document=false）。
		return jsc.ObjectValue(wrapDocument(in, v))
	case *dom.Attr:
		// ★ 第 18 次监督轮：Attr 节点（createAttribute 的返回值、
		// getAttributeNode 一类的节点取值）。
		return jsc.ObjectValue(wrapAttr(in, v))
	case *dom.ProcessingInstruction:
		// ★ 第 18 次监督轮：PI 节点（createProcessingInstruction 的返回值；
		// 它也出现在 childNodes 里）。
		return jsc.ObjectValue(wrapProcessingInstruction(in, v))
	case *dom.CDATASection:
		// ★ 第 18 次监督轮：CDATA 节点（XML 文档的 createCDATASection 结果）。
		return jsc.ObjectValue(wrapCDATASection(in, v))
	default:
		return jsc.Null()
	}
}

// makeDOMRect 创建一个 DOMRect 对象。
func makeDOMRect(in *jsc.Interpreter, x, y, w, h float64) *jsc.JSObject {
	// ★ 第 19 轮：DOMRect 接口原型（getBoundingClientRect / IntersectionObserver
	// entry 的 boundingClientRect 等据此满足 instanceof DOMRect / DOMRectReadOnly）。
	r := jsc.NewObject(domIfaceProtoOr("DOMRect", in.ObjectPrototype()))
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
// 属性在首次访问时经 installElementProperty（engine/js/bindings/lazyelement.go）
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
	// <video>/<audio> 走媒体元素原型（HTMLMediaElement 家族）。
	if mp := mediaElementPrototypeFor(el); mp != nil {
		proto = mp
	} else if hp := htmlElementPrototypeFor(el); hp != nil {
		// 其余元素按 HTML 接口分派（HTMLDivElement/HTMLIFrameElement/...），
		// 使 `div instanceof HTMLDivElement`、`div.constructor.name` 成立。
		// 原型链：XxxElement.prototype → HTMLElement.prototype → Element.prototype
		// → Node.prototype（htmlelements.go 建立）。
		proto = hp
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
	// ★ 第 19 轮：classList 是 DOMTokenList（`el.classList instanceof DOMTokenList`）。
	cls := jsc.NewObject(domIfaceProtoOr("DOMTokenList", rt.ObjectPrototype()))
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
			if len(args) == 0 {
				return jsc.Undefined()
			}
			m := make(map[string]bool)
			for _, c := range get() {
				m[c] = true
			}
			for _, a := range args {
				m[a.ToString()] = true
			}
			var r []string
			for c := range m {
				r = append(r, c)
			}
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Undefined()
			}
			m := make(map[string]bool)
			for _, c := range get() {
				m[c] = true
			}
			for _, a := range args {
				delete(m, a.ToString())
			}
			var r []string
			for c := range m {
				r = append(r, c)
			}
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			n := args[0].ToString()
			for _, c := range get() {
				if c == n {
					return jsc.BooleanValue(true)
				}
			}
			return jsc.BooleanValue(false)
		}, 1)))
	cls.Set("toggle", jsc.FunctionValue(jsc.NewNativeFunction("toggle",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
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
			if len(args) == 0 {
				return jsc.Null()
			}
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
			if len(args) == 0 {
				return jsc.StringValue("")
			}
			key := "data-" + camelToKebab(args[0].ToString())
			return jsc.StringValue(el.GetAttribute(key))
		}, 1)))
	ds.Set("set", jsc.FunctionValue(jsc.NewNativeFunction("_set",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 {
				return jsc.Undefined()
			}
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

	// ★ C-P4-1 解析缓存：decls 是读路径（getPropertyValue / length / item /
	// cssText / Keys）的中枢，此前每次调用都重新解析 style 属性文本（实测
	// getPropertyValue 约 1.5µs/次）。以「上次解析的文本」为键：任何写路径都要
	// 回写属性文本 → 文本一变缓存自动失效，不需要额外的失效钩子。
	cachedText  string
	cachedDecls []styleDecl
	cachedOK    bool
}

// ── 第 21 轮：CSSStyleDeclaration 的保序声明模型 ──────────────────────
//
// style 属性以**文本**存于元素上，但 CSSStyleDeclaration 的
// length / item(i) / [i] / cssText 都要求**按声明顺序**枚举；map 无序，
// 故这里用切片保序。同名声明覆盖时保留**首次出现的位置**（Chromium 的
// CSSStyleDeclaration 亦然：`color:red; color:blue` → 只有一条 color，
// 位置在首次出现处）。
type styleDecl struct {
	Name  string
	Value string
}

// parseStyleDecls 解析 style 属性文本为保序声明列表。
func parseStyleDecls(s string) []styleDecl {
	var out []styleDecl
	idx := make(map[string]int, 8)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		val := strings.TrimSpace(kv[1])
		if i, ok := idx[name]; ok {
			out[i].Value = val
			continue
		}
		idx[name] = len(out)
		out = append(out, styleDecl{Name: name, Value: val})
	}
	return out
}

// joinStyleDecls 把保序声明列表写回 style 属性文本（"name:value;name:value"，
// 与既有 joinStyle 的输出格式一致 —— 规范化（冒号后空格）只发生在 cssText
// getter 一侧（serializeCSSText），属性文本本身保持紧凑形式）。
func joinStyleDecls(decls []styleDecl) string {
	parts := make([]string, 0, len(decls))
	for _, d := range decls {
		parts = append(parts, d.Name+":"+d.Value)
	}
	return strings.Join(parts, ";")
}

// styleIndexKey 判断 `style[0]` 这类索引键并返回下标。
func styleIndexKey(key string) (int, bool) {
	if key == "" {
		return 0, false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(key)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// matchStyleDecl 按 CSSOM 语义查找声明（getPropertyValue / setProperty /
// removeProperty 路径）：属性名**大小写不敏感**（Edge 实测
// getPropertyValue("Width") 命中 "width"），camelCase 先换算为 kebab-case
// 再查；自定义属性（--x）名大小写敏感（规范）。
func matchStyleDecl(decls []styleDecl, name string) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1, false
	}
	if strings.HasPrefix(name, "--") {
		for i, d := range decls {
			if d.Name == name {
				return i, true
			}
		}
		return -1, false
	}
	want := strings.ToLower(camelToKebab(name))
	for i, d := range decls {
		if strings.ToLower(d.Name) == want {
			return i, true
		}
	}
	return -1, false
}

// matchStyleDeclExact 按原样名字精确匹配（属性访问 `style.width` 的**既有**
// 语义，大小写敏感 —— 第 21 轮不改这条路径的行为）。
func matchStyleDeclExact(decls []styleDecl, name string) (int, bool) {
	for i, d := range decls {
		if d.Name == name {
			return i, true
		}
	}
	return -1, false
}

// setStyleDecl 写入声明：已存在则**原位替换**（保持声明顺序），否则追加。
func setStyleDecl(decls []styleDecl, name, value string) []styleDecl {
	if i, ok := matchStyleDecl(decls, name); ok {
		decls[i].Value = value
		return decls
	}
	return append(decls, styleDecl{Name: name, Value: value})
}

// removeStyleDecl 删除声明（大小写不敏感），返回新列表。
func removeStyleDecl(decls []styleDecl, name string) []styleDecl {
	i, ok := matchStyleDecl(decls, name)
	if !ok {
		return decls
	}
	return append(decls[:i:i], decls[i+1:]...)
}

// splitImportant 把声明值拆成「值 + 是否 !important」（CSSOM：getPropertyValue
// 不含优先级，getPropertyPriority 返回 "important"/""）。
func splitImportant(v string) (string, bool) {
	t := strings.TrimRight(v, " \t")
	if i := strings.LastIndex(t, "!"); i >= 0 {
		if strings.EqualFold(strings.TrimSpace(t[i+1:]), "important") {
			return strings.TrimSpace(t[:i]), true
		}
	}
	return v, false
}

// decls 返回当前元素的保序声明列表（**带解析缓存**）。返回值与缓存共享底层
// 数组，只读使用；写路径请用 declsForWrite。
func (s *styleProxy) decls() []styleDecl {
	text := s.el.GetAttribute("style")
	if s.cachedOK && s.cachedText == text {
		return s.cachedDecls
	}
	decls := parseStyleDecls(text)
	s.cachedText, s.cachedDecls, s.cachedOK = text, decls, true
	return decls
}

// declsForWrite 返回**可安全修改**的声明副本（写路径专用）：setStyleDecl 命中
// 同名声明时会原地改 Value，若直接改 decls() 的共享数组就把解析缓存写脏了
// （属性文本没变、缓存内容已变 → 后续读拿到错值）。
func (s *styleProxy) declsForWrite() []styleDecl {
	src := s.decls()
	out := make([]styleDecl, len(src))
	copy(out, src)
	return out
}

func (s *styleProxy) Get(key string) goja.Value {
	vm := s.vm
	// ★ 第 21 轮：CSSStyleDeclaration 是 indexed + named properties 对象 ——
	//   `style[0]` 返回第 0 条声明的**属性名**，与 `style.item(0)` 同值
	//   （Edge 实测 `style[0] === style.item(0)` 为 true）。索引键必须在 default
	//   的「CSS 属性取值」分支**之前**处理，否则 "0" 会被当属性名查询而返回 ""。
	if idx, ok := styleIndexKey(key); ok {
		decls := s.decls()
		if idx < len(decls) {
			return vm.ToValue(decls[idx].Name)
		}
		return vm.ToValue("")
	}
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
			decls := s.declsForWrite()
			name := call.Arguments[0].String()
			v := call.Arguments[1].String()
			// ★ 第 21 轮：第三参数 priority 按 CSSOM 接受 "important"（大小写
			//   不敏感、允许前后空白；其它值忽略）。Edge 实测：
			//   setProperty("color","red","important") 之后
			//   getPropertyPriority("color") === "important"、
			//   cssText === "color: red !important;"。
			if len(call.Arguments) >= 3 &&
				strings.EqualFold(strings.TrimSpace(call.Arguments[2].String()), "important") {
				v = strings.TrimSpace(v) + " !important"
			}
			// ★ CSSOM 规范：setProperty(name, "") 等价于 removeProperty(name)。
			//   此前无条件写入 → `setProperty('transform','')` 之后级联里存着
			//   空值声明，getComputedStyle().transform 返回 "" 而非初始值
			//   "none"（Edge 实测：h7_transform_norm 的空值 case → Edge none / wbui ""）。
			if strings.TrimSpace(call.Arguments[1].String()) == "" {
				// ★ C-P4-1 无变更快速路径：没有该声明时「移除」是空操作。
				if _, ok := matchStyleDecl(decls, name); !ok {
					return goja.Undefined()
				}
				decls = removeStyleDecl(decls, name)
			} else {
				// ★ C-P4-1 同值快速路径：声明已存在且值一致 → 属性文本不会变，
				// 跳过 SetAttribute / 计算样式失效 / 宿主回调。Vue/React 的
				// patchStyle 在 diff 未变时反复写同值，此前每次都触发一次全文档
				// 样式失效。
				if i, ok := matchStyleDecl(decls, name); ok && decls[i].Value == v {
					return goja.Undefined()
				}
				decls = setStyleDecl(decls, name, v)
			}
			s.el.SetAttribute("style", joinStyleDecls(decls))
			// 与 styleProxy.Set 路径对齐：不失效缓存会让后续 getComputedStyle 读到旧值。
			InvalidateComputedStyle(s.el)
			if OnInlineStyleChanged != nil {
				OnInlineStyleChanged(s.el)
			}
			return goja.Undefined()
		})
	case "getPropertyValue":
		// CSSOM §1.2：返回声明值（不含 !important）；未声明返回 ""。属性名
		// 大小写不敏感（Edge 实测 getPropertyValue("Width") 命中 "width"）。
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return vm.ToValue("")
			}
			decls := s.decls()
			if i, ok := matchStyleDecl(decls, call.Arguments[0].String()); ok {
				v, _ := splitImportant(decls[i].Value)
				// ★ 第 23 轮：CSSOM 口径（url 带引号 / font-family 去引号）——
				//   Edge 基线 dev/output/wbui-audit/r23base.edge.txt。
				return vm.ToValue(css.CSSTextValueOf(decls[i].Name, v))
			}
			return vm.ToValue("")
		})
	case "getPropertyPriority":
		// CSSOM §1.2：返回 "important" 或 ""。
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return vm.ToValue("")
			}
			decls := s.decls()
			if i, ok := matchStyleDecl(decls, call.Arguments[0].String()); ok {
				if _, imp := splitImportant(decls[i].Value); imp {
					return vm.ToValue("important")
				}
			}
			return vm.ToValue("")
		})
	case "item":
		// CSSOM §1.2：返回第 i 条声明的**属性名**；越界返回 ""（Edge 实测：
		// item(-1) / item(99) 均返回 ""，不是 null）。
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return vm.ToValue("")
			}
			decls := s.decls()
			i := int(call.Arguments[0].ToInteger())
			if i < 0 || i >= len(decls) {
				return vm.ToValue("")
			}
			return vm.ToValue(decls[i].Name)
		})
	case "length":
		// CSSOM §1.2：声明条数（Edge 实测：内联 style 的 length 即声明数，
		// 含自定义属性）。
		return vm.ToValue(len(s.decls()))
	case "removeProperty":
		return vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return vm.ToValue("")
			}
			decls := s.declsForWrite()
			name := call.Arguments[0].String()
			i, ok := matchStyleDecl(decls, name)
			if !ok {
				// ★ C-P4-1 无变更快速路径：没有该声明 → 不写属性、不失效。
				return vm.ToValue("")
			}
			old, _ := splitImportant(decls[i].Value)
			s.el.SetAttribute("style", joinStyleDecls(removeStyleDecl(decls, name)))
			return vm.ToValue(old)
		})
	default:
		// CSS property: return the value from the style attribute
		//（属性访问路径保持**既有**语义：精确名匹配 + camelCase 换算；值原样
		//  返回（含 !important），与浏览器 `style.width` 一致。）
		decls := s.decls()
		if i, ok := matchStyleDeclExact(decls, camelToKebab(key)); ok {
			// ★ 第 23 轮：属性访问路径与 getPropertyValue 同口径（值文本仍保留
			//   !important，只有 url 引号 / font-family 引号被规范化）。
			return vm.ToValue(css.CSSTextValueOf(decls[i].Name, decls[i].Value))
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
		decls := s.declsForWrite()
		strVal := val.String()
		ckey := camelToKebab(key)
		// ★ 空白串（"  "）也须视为「移除属性」：CSSOM 规定属性值首尾空白不计入，
		//   全空白值等价于空值 → 属性被移除。此前只判 `strVal == ""`，于是
		//   `el.style.transform = '  '` 会写入 `style="transform:  "`，级联里
		//   留下一个空值声明，getComputedStyle().transform 返回 ""（Edge 为 none）。
		//   这是 h7_transform_norm 唯一残留差异的来源。
		if strings.TrimSpace(strVal) == "" || strVal == "undefined" || strVal == "null" {
			// ★ C-P4-1 无变更快速路径：没有该声明时「移除」是空操作。
			if _, ok := matchStyleDecl(decls, ckey); !ok {
				return true
			}
			decls = removeStyleDecl(decls, ckey)
		} else {
			// ★ C-P4-1 同值快速路径（见 setProperty 处说明）。
			if i, ok := matchStyleDecl(decls, ckey); ok && decls[i].Value == strVal {
				return true
			}
			decls = setStyleDecl(decls, ckey, strVal)
		}
		s.el.SetAttribute("style", joinStyleDecls(decls))
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
	case "cssText", "setProperty", "removeProperty",
		"getPropertyValue", "getPropertyPriority", "item", "length":
		return true
	}
	decls := s.decls()
	if idx, ok := styleIndexKey(key); ok {
		return idx < len(decls)
	}
	_, ok := matchStyleDeclExact(decls, key)
	return ok
}

func (s *styleProxy) Keys() []string {
	decls := s.decls()
	keys := make([]string, 0, len(decls)+7)
	keys = append(keys, "cssText", "setProperty", "removeProperty")
	keys = append(keys, "length", "item", "getPropertyValue", "getPropertyPriority")
	for _, d := range decls {
		keys = append(keys, d.Name)
	}
	return keys
}

func (s *styleProxy) Delete(key string) bool {
	decls := s.declsForWrite()
	if _, ok := matchStyleDecl(decls, key); !ok {
		return true // ★ C-P4-1 无变更快速路径：没有该声明 → 不写属性、不失效
	}
	s.el.SetAttribute("style", joinStyleDecls(removeStyleDecl(decls, key)))
	InvalidateComputedStyle(s.el)
	return true
}

// styleObjectEntry 缓存一条 el.style 句柄（带所属解释器，避免跨 rt 复用 goja 对象
// ——goja 对跨 runtime 的对象访问会报 "Illegal runtime transition"）。
type styleObjectEntry struct {
	rt  *jsc.Interpreter
	obj *jsc.JSObject
}

// styleObjectCache 按元素缓存 `el.style` 的 JS 对象（C-P4-3 句柄缓存）：
// 浏览器里 CSSStyleDeclaration 是**稳定实例**（`el.style === el.style` 为 true），
// 而宿主侧此前每次取句柄都新建 dynamic object + 装配原型（实测 ~159ns/次）；
// Vue 的 patchStyle、CM6 的 gutter spacer 等热路径反复取句柄，开销可观。
// 生命周期与 nodeWrapperCache 一致（clearNodeCache 里清空）。
var styleObjectCache = make(map[*dom.Element]styleObjectEntry)

// styleObjectFor 取（必要时创建）元素的 style 包装对象。
func styleObjectFor(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	if el == nil {
		return makeStyleObject(rt, el)
	}
	if e, ok := styleObjectCache[el]; ok && e.rt == rt {
		return e.obj
	}
	obj := makeStyleObject(rt, el)
	styleObjectCache[el] = styleObjectEntry{rt: rt, obj: obj}
	return obj
}

func makeStyleObject(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	pr := &styleProxy{el: el, vm: rt.VM()}
	gojaObj := rt.VM().NewDynamicObject(pr)
	obj := jsc.WrapObject(gojaObj, rt)
	// ★ 第 20 轮：`el.style` 是 CSSStyleDeclaration 实例。
	// ★ 必须用 jsc.SetObjectPrototype（goja 的 SetPrototype），**不能**走
	//   obj.Set("__proto__", …)：styleProxy 是 dynamic object，其 Set handler 会把
	//   "__proto__" 当成 CSS 属性写进元素的 style 属性（属性名变成 "__proto__"）。
	//   原型上只含 constructor（本引擎不为中心化的 style 对象建模整套
	//   CSSStyleDeclaration 方法面，见 WORKITEMS §20-6）——故 Get 拦截不受影响：
	//   `style.getPropertyValue` 之类仍返回 ""（既有行为，本轮未改变）。
	if p := domIfaceProto("CSSStyleDeclaration"); p != nil {
		jsc.SetObjectPrototype(obj, p)
	}
	return obj
}

// parseStyle parses "color:red;font-size:16px" → map
func parseStyle(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
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
	decls := parseStyleDecls(style)
	if len(decls) == 0 {
		return ""
	}
	// ★ 第 21 轮：按 Edge 实测基线规范化 —— 每条声明 **"name: value;"**（冒号后
	//   恰一个空格），声明之间单个空格分隔。此前直接把属性原文按 "; " 切块加尾
	//   分号（"height:20px" 原样保留），而浏览器 cssText 恒为规范化形式
	//   （Edge 实测：style="width: 10px; height:20px" → cssText
	//   "width: 10px; height: 20px;"）。尾分号保持不变 —— `style.cssText += "..."`
	//   的追加拼接安全性依赖它。
	out := make([]string, 0, len(decls))
	for _, d := range decls {
		// ★ 第 23 轮：cssText 也走 CSSOM 值口径（Edge 实测
		//   `background-image: url("foo.png"); background-repeat: repeat-x;`）。
		out = append(out, d.Name+": "+css.CSSTextValueOf(d.Name, d.Value)+";")
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
	// Element 级遍历（DOM §4.4）：DocumentFragment 同族 API——只认 Element，
	// 跳过 Text/Comment，无则 null（<template>.content 与 Vue insertStaticContent
	// 的 `while (frag.firstElementChild)` 类搬运循环依赖它，缺了会静默终止）。
	obj.SetAccessor("firstElementChild", nodeAccFn(rt, func() dom.Node { return firstElementChildOf(frag) }), nil)
	obj.SetAccessor("lastElementChild", nodeAccFn(rt, func() dom.Node { return lastElementChildOf(frag) }), nil)
	obj.SetAccessor("nextElementSibling", nodeAccFn(rt, func() dom.Node { return nextElementSiblingOf(frag) }), nil)
	obj.SetAccessor("previousElementSibling", nodeAccFn(rt, func() dom.Node { return previousElementSiblingOf(frag) }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, frag.ChildNodes())
	}), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		n := 0
		for c := frag.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*dom.Element); ok {
				n++
			}
		}
		return jsc.NumberValue(float64(n))
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrElemAs(in, elementChildrenOf(frag), "HTMLCollection")
	}), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(frag.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { frag.SetTextContent(v.ToString()) })

	// appendChild
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil {
			return jsc.Null()
		}
		frag.AppendChild(n)
		if OnNodeInserted != nil {
			OnNodeInserted(n)
		}
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
		if n == nil {
			return jsc.Null()
		}
		frag.RemoveChild(n)
		if OnNodeRemoved != nil {
			OnNodeRemoved(n)
		}
		return a
	})))
	// insertBefore — Vue 3 insertStaticContent uses this to insert template content
	obj.Set("insertBefore", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil {
			return jsc.Null()
		}
		frag.InsertBefore(nc, rc)
		if OnNodeInserted != nil {
			OnNodeInserted(nc)
		}
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
	// ★ 第 19 轮：ShadowRoot 接口原型（ShadowRoot → DocumentFragment → Node）。
	obj := jsc.NewObject(domIfaceProtoOr("ShadowRoot", rt.ObjectPrototype()))
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
	// Element 级遍历（DOM §4.4）：ShadowRoot 继承 DocumentFragment，同一族 API
	// （只认 Element，跳过 Text/Comment）。
	obj.SetAccessor("firstElementChild", nodeAccFn(rt, func() dom.Node { return firstElementChildOf(sr) }), nil)
	obj.SetAccessor("lastElementChild", nodeAccFn(rt, func() dom.Node { return lastElementChildOf(sr) }), nil)
	obj.SetAccessor("nextElementSibling", nodeAccFn(rt, func() dom.Node { return nextElementSiblingOf(sr) }), nil)
	obj.SetAccessor("previousElementSibling", nodeAccFn(rt, func() dom.Node { return previousElementSiblingOf(sr) }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, sr.ChildNodes())
	}), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(elementChildrenOf(sr))))
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrElemAs(in, elementChildrenOf(sr), "HTMLCollection")
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
// 属性在首次访问时经 installTextProperty（engine/js/bindings/lazytext.go）物化。
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
	// ★ 第 19 轮：TreeWalker 接口原型。
	obj := jsc.NewObject(domIfaceProtoOr("TreeWalker", rt.ObjectPrototype()))
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
	// ★ 第 19 轮：Range 接口原型。
	obj := jsc.NewObject(domIfaceProtoOr("Range", rt.ObjectPrototype()))
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
				// ★ 第 19 轮：矩形统一由 makeDOMRect 构造（DOMRect 接口原型），
				// 容器为 DOMRectList（规范返回类型，此前是裸数组）。
				rects = append(rects, jsc.ObjectValue(makeDOMRect(in, l, t, w, h)))
			}
			return jsc.ObjectValue(wrapDOMRectList(in, rects))
		}, 0)))
	obj.Set("getBoundingClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			l, t, w, h, _ := rangeRect(st)
			return jsc.ObjectValue(makeDOMRect(in, l, t, w, h))
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
			if p := c.ParentNode(); p != nil {
				p.RemoveChild(c)
			}
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
	if !v.IsObject() {
		return nil
	}
	if n, ok := v.AsObject().Internal().(dom.Node); ok {
		return n
	}
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
	case *dom.Document:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapDocument(rt, v))
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
		case *dom.DocumentType:
			return jsc.ObjectValue(wrapDocumentType(in, v))
		case *dom.Document:
			return jsc.ObjectValue(wrapDocument(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		case *dom.Attr:
			return jsc.ObjectValue(wrapAttr(in, v))
		case *dom.ProcessingInstruction:
			return jsc.ObjectValue(wrapProcessingInstruction(in, v))
		case *dom.CDATASection:
			return jsc.ObjectValue(wrapCDATASection(in, v))
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
	case *dom.DocumentType:
		return v == nil
	case *dom.Document:
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
		if len(args) == 0 {
			return jsc.Null()
		}
		return fn(in, args[0].ToString())
	}, 1)
}

func fn2(fn func(in *jsc.Interpreter, a, b string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		return fn(in, args[0].ToString(), args[1].ToString())
	}, 2)
}

func fn1Node(fn func(in *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Null()
		}
		return fn(in, unwrapNode(args[0]), args[0])
	}, 1)
}

func fn2Node(fn func(in *jsc.Interpreter, n1, n2 dom.Node, a0, a1 jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Null()
		}
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

// arrElemAs 与 arrElem 同，但把结果数组的原型指向集合接口
// （NodeList / HTMLCollection）—— `el.querySelectorAll("x") instanceof NodeList`、
// `el.children instanceof HTMLCollection` 因此成立，而数组方法（map/indexOf/
// slice）经 Array.prototype 链仍可达（见 domctors.go 文件头的兼容性取舍）。
func arrElemAs(in *jsc.Interpreter, els []*dom.Element, iface string) jsc.JSValue {
	v := arrElem(in, els)
	if o := v.AsObject(); o != nil {
		domAttachProto(o, iface)
	}
	return v
}

func arrNode(in *jsc.Interpreter, nodes []dom.Node) jsc.JSValue {
	// ★ 第 19 轮：childNodes 返回 **NodeList**（规范类型；数组语义保留）。
	v := arrayValue(in, len(nodes), func(i int) jsc.JSValue {
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
		case *dom.DocumentType:
			return jsc.ObjectValue(wrapDocumentType(in, v))
		case *dom.Document:
			return jsc.ObjectValue(wrapDocument(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		case *dom.Attr:
			return jsc.ObjectValue(wrapAttr(in, v))
		case *dom.ProcessingInstruction:
			return jsc.ObjectValue(wrapProcessingInstruction(in, v))
		case *dom.CDATASection:
			return jsc.ObjectValue(wrapCDATASection(in, v))
		}
		return jsc.Null()
	})
	if o := v.AsObject(); o != nil {
		domAttachProto(o, "NodeList")
	}
	return v
}
func arrayValue(in *jsc.Interpreter, n int, fn func(int) jsc.JSValue) jsc.JSValue {
	arr := make([]jsc.JSValue, n)
	for i := 0; i < n; i++ {
		arr[i] = fn(i)
	}
	return jsc.ObjectValue(jsc.NewArrayForInterp(in, arr))
}
func arrJS(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

// wrapStyleSheetList 把样式表列表暴露成 StyleSheetList 语义（CSSOM §document.
// styleSheets）：length + item(i) + 索引属性 + 可遍历。库常做
// `Array.from(document.styleSheets)` 或按 length 判断样式是否已装配。
func wrapStyleSheetList(in *jsc.Interpreter, sheets []*css.CSSStyleSheet) jsc.JSValue {
	// ★ 第 20 轮：CSSOM §1.5 StyleSheetList 原型
	//（`document.styleSheets instanceof StyleSheetList`）。
	obj := jsc.NewObject(domIfaceProtoOr("StyleSheetList", in.ObjectPrototype()))
	obj.SetClassName("StyleSheetList")
	// ★ 预建包装对象数组：索引访问与 item(i) 必须返回**同一身份**的对象
	//   （浏览器里 StyleSheetList 是 reflector，`list[0] === list.item(0)`）。
	//   每次现造会让两者引用不等，破坏库用来做去重/比较的身份判断。
	objs := make([]*jsc.JSObject, len(sheets))
	for i, s := range sheets {
		objs[i] = wrapStyleSheet(in, s)
		obj.Set(strconv.Itoa(i), jsc.ObjectValue(objs[i]))
	}
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(objs)))
	}), nil)
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			i := int(args[0].ToNumber())
			if i < 0 || i >= len(objs) {
				return jsc.Null()
			}
			return jsc.ObjectValue(objs[i])
		}, 1)))
	return jsc.ObjectValue(obj)
}

// wrapStyleSheet 把 CSSStyleSheet 暴露成 JS 对象（CSSOM §CSSStyleSheet）：
// href/title/type/disabled/ownerNode 与 cssRules/rules。★ cssRules 只提供
// length/item（CSSRule 对象模型未移植，见 css/stylesheet.go 的 Completeness
// 说明），而库最常用的恰好是 `sheet.cssRules.length` 这一探测。
func wrapStyleSheet(in *jsc.Interpreter, s *css.CSSStyleSheet) *jsc.JSObject {
	// ★ 第 20 轮：CSSOM §1.5 CSSStyleSheet 原型（→ StyleSheet）
	//（`document.styleSheets[0] instanceof CSSStyleSheet`）。
	obj := jsc.NewObject(domIfaceProtoOr("CSSStyleSheet", in.ObjectPrototype()))
	obj.SetClassName("CSSStyleSheet")
	obj.SetInternal(s)
	obj.SetAccessor("href", strAcc(s.Href()), nil)
	obj.SetAccessor("title", strAcc(s.Title()), nil)
	obj.SetAccessor("type", strAcc(s.Type()), nil)
	obj.SetAccessor("disabled",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.BooleanValue(s.Disabled()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { s.SetDisabled(v.ToBoolean()) })
	obj.SetAccessor("ownerNode", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if n := s.OwnerNode(); n != nil {
			if el, ok := n.(*dom.Element); ok {
				return jsc.ObjectValue(wrapElement(in, el))
			}
		}
		return jsc.Null()
	}), nil)
	// ★ 第 21 轮：规则列表**身份稳定** —— CSSOM 要求 `sheet.cssRules === sheet.cssRules`
	//   为 true（同一 CSSStyleSheet 包装对象上的连续访问返回同一 CSSRuleList；
	//   规则对象亦然：`cssRules[0] === cssRules[0]`）。此前每次访问新建 CSSRuleList
	//   并把全部规则重新包装 → 两个断言都是 false。缓存挂在**本包装对象的闭包**里
	//   （per-runtime —— jsc.JSObject 绑定创建它的 runtime，绝不跨 rt 复用）；
	//   规则条数变化时重建，避免读到过期集合。
	var ruleList *jsc.JSObject
	ruleListLen := -1
	ruleListAcc := getter(func(in *jsc.Interpreter) jsc.JSValue {
		rules := s.Rules()
		if ruleList != nil && ruleListLen == len(rules) {
			return jsc.ObjectValue(ruleList)
		}
		// ★ 第 20/21 轮：CSSRuleList + 规则对象（CSSOM §1.4）。列表构造统一走
		//   wrapCSSRuleList（索引访问与 item(i) 返回同一身份；嵌套规则的
		//   cssRules 复用同一实现）。
		// ★ 第 22 轮：parent = nil（顶层规则的 parentRule 为 null）、
		//   parentSheet = 本包装对象（Edge 实测 `rule.parentStyleSheet` 就是
		//   `document.styleSheets[i]` 那个对象 —— 身份一致）。
		ruleList = wrapCSSRuleList(in, rules, nil, obj)
		ruleListLen = len(rules)
		return jsc.ObjectValue(ruleList)
	})
	obj.SetAccessor("cssRules", ruleListAcc, nil)
	obj.SetAccessor("rules", ruleListAcc, nil)
	return obj
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
	// ★ WB_COMPUTED_DEBUG=1：诊断必须放在 cssCacheGet **之前** —— 缓存命中会提前
	//   return（真实页面该元素 computed 早已入缓存），此前把打印放缓存之后 →
	//   连续两轮拿不到 matchedDecls（输出为空）。此处至少明确区分「缓存命中」与
	//   「规则未匹配」两种情况，避免再用推断代替数据。
	debugComputed := os.Getenv("WB_COMPUTED_DEBUG") != ""
	if debugComputed {
		if e2, ok2 := el.(*dom.Element); ok2 && strings.Contains(e2.ClassName(), "chat-input") {
			if cached, ok3 := cssCacheGet(el); ok3 {
				fmt.Printf("[computed-dump] <%s class=%q> CACHE HIT: cached padding-top=%q min-height=%q (decls 不可见，需清缓存或首帧采集)\n",
					e2.LocalName(), e2.ClassName(), cached["padding-top"], cached["min-height"])
			} else {
				fmt.Printf("[computed-dump] <%s class=%q> CACHE MISS: 走完整级联，将打印 matchedDecls\n", e2.LocalName(), e2.ClassName())
			}
		}
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
				if mediaMatches(rl, el) {
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
	// ★ WB_COMPUTED_DEBUG=1：打印匹配到该元素的**全部声明**（名称/值/!important/
	//   源顺序），仅针对 chat-input 以减少噪音。用于定位「规则匹配却读不到」或
	//   「被更高优先级声明覆盖」的真因 —— 禁止用推断代替该数据。
	if os.Getenv("WB_COMPUTED_DEBUG") != "" {
		if e2, ok := el.(*dom.Element); ok && strings.Contains(e2.ClassName(), "chat-input") {
			fmt.Printf("[computed-dump] <%s class=%q> matched decls=%d\n", e2.LocalName(), e2.ClassName(), len(decls))
			for _, md := range decls {
				n := md.decl.Name
				if n == "padding" || strings.HasPrefix(n, "padding-") || n == "min-height" ||
					n == "font-size" || n == "box-sizing" || n == "height" {
					fmt.Printf("[computed-dump]   %-18s = %-30v imp=%-5v order=%d\n",
						n, md.decl.Value, md.decl.Important, md.order)
				}
			}
		}
	}
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
	// ★ overflow 简写展开：浏览器  → overflowX/overflowY 均为
	//   "auto"（getComputedStyle 恒展开简写）。此前只展 padding/margin →
	//   读 overflowX/overflowY 得 undefined，依赖它判断「是否可滚」的 JS 失效。
	expandOverflowShorthand(out)
	// ★ border 简写展开（style/color 部分；width 已有 bwTok 展开）：
	//   浏览器  → borderTopStyle="dashed"、
	//   borderTopColor="rgb(18, 52, 86)" 等长写恒有值。
	expandBorderStyleLonghand(out)
	// ★ font 简写展开（与 engine/style 的 applyDeclaration 共用
	//   css.ParseFontShorthand）：级联 map 里 `font: 24px sans-serif` 只落在
	//   "font" 键上，而 getComputedStyle(el).fontSize 查 "font-size" → 缺失 →
	//   回退初始值 16px；布局侧读 ComputedStyle.FontSize 却是 24px —— 同一元素
	//   「读到的字号」与「画出来的字号」脱节（E4/G5 探针命中）。
	expandFontShorthand(out)
	// ★ list-style 简写展开（与 engine/style 的 case "list-style" 对应）：
	//   `list-style: square` 只落在级联 map 的 "list-style" 键上 →
	//   getComputedStyle(el).listStyleType 落空、回退初始值 disc（G7 探针：
	//   square/decimal 的 computed 与 disc 无差别）。
	expandListStyleShorthand(out)
	// 解析 var(--xxx) 引用（自定义属性继承链：:root → body → ... → el）。
	// 浏览器语义：自定义属性随级联继承，子元素 var() 引用解析为最近祖先的
	// 定义值。wb-ui 级联 map 本身不含继承值，此处补收集 + 替换。
	resolveVarInComputed(out, el)
	// ★ 继承与计算值补全（D2）：级联 map 只含元素自身声明，inherited:yes 的
	//   属性（color/font-size/font-family/line-height…）未声明时必须继承父元素的
	//   **computed** 值；font-size 还必须是绝对长度（px）、color 建议归一化为
	//   rgb(...)，与布局/绘制同源。缺了这一步，`body{font-size:40px}` 的子元素
	//   会读回初始值 16px、`color` 读回 undefined（而布局实际用的是 40px 红字）。
	inheritComputedProps(out, el)
	applyComputedSnapshot(out, el)
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
	// ★ H6：padding/margin 的**简写** computed 值。浏览器
	//   getComputedStyle(el).padding 恒有值（四边拼接，如 "1px 2px"）；wbui 此前
	//   只有 expandBoxShorthand（简写 → 四边）这一向，四边齐备时未反向拼装 →
	//   `cs.padding` 读到 undefined。border-width 同理（四边齐备时拼装）。
	composeBoxShorthand(out, "padding", []string{"padding-top", "padding-right", "padding-bottom", "padding-left"})
	composeBoxShorthand(out, "margin", []string{"margin-top", "margin-right", "margin-bottom", "margin-left"})
	// ★ H6：border-*-width 四边 —— 级联 map 里通常只有 `border` / `border-width`
	//   简写（或个别 longhand），而浏览器 getComputedStyle 四边恒有值。
	//   规则（CSS 2.1 §8.5.1）：border-style 为 none/hidden（或未声明）时宽度
	//   的计算值是 0px；否则取简写里的宽度 token（缺省 medium = 3px）。
	//   补完四边后下面的 composeBoxShorthand 才能拼出 `borderWidth`
	//   （g1_formctl 的 t1/t5/t6/t7 与 h6_misc_props 的 #p 都依赖它）。
	for _, side := range []string{"top", "right", "bottom", "left"} {
		wKey, sKey := "border-"+side+"-width", "border-"+side+"-style"
		if _, ok := out[wKey]; ok {
			continue
		}
		st := out[sKey]
		if st == "" {
			st = cssBorderStyleToken(out["border"])
		}
		if st == "" {
			st = firstCSSKeyword(out["border-style"])
		}
		if st == "" || st == "none" || st == "hidden" {
			out[wKey] = "0px"
			continue
		}
		w := cssBorderWidthToken(out["border-width"])
		if w == "" {
			w = cssBorderWidthToken(out["border"])
		}
		if w == "" {
			w = cssBorderWidthToken(out["border-"+side])
		}
		if w == "" {
			w = "3px" // medium（border-width 初始值）
		}
		out[wKey] = w
	}
	composeBoxShorthand(out, "border-width", []string{"border-top-width", "border-right-width", "border-bottom-width", "border-left-width"})
	// ★ 注意：transform-origin 的 computed 值是**绝对 px**、依赖 border-box 几何，
	//   而几何随布局变化 —— 它**不能**进 per-element 缓存（cssCachePut），
	//   否则首次调用时若布局尚未就绪就缓存了 "undefined"，之后永远读不到。
	//   故该属性在 getComputedStyle 的回写阶段**实时**计算（见彼处注释）。
	cssCachePut(el, out)
	return out
}

// cssBorderWidthToken 从 border / border-width / border-<side> 简写里取出宽度
// token（数字开头者，如 1px / 2px / 0.5em），跳过样式（solid）与颜色关键字。
func cssBorderWidthToken(s string) string {
	for _, f := range strings.Fields(s) {
		if f == "" {
			continue
		}
		if (f[0] >= '0' && f[0] <= '9') || f[0] == '.' || f[0] == '-' || f[0] == '+' {
			// 排除负数以外的纯数字（border 宽度不允许负值，但宽松接受）
			return f
		}
	}
	return ""
}

// cssBorderStyleToken 从 border 简写里取出样式关键字（solid/dashed/…）。
func cssBorderStyleToken(s string) string {
	for _, f := range strings.Fields(s) {
		switch strings.ToLower(f) {
		case "none", "hidden", "dotted", "dashed", "solid", "double",
			"groove", "ridge", "inset", "outset":
			return strings.ToLower(f)
		}
	}
	return ""
}

// firstCSSKeyword 返回字符串里第一个空白分隔的 token（小写）。
func firstCSSKeyword(s string) string {
	for _, f := range strings.Fields(s) {
		return strings.ToLower(f)
	}
	return ""
}

// composeBoxShorthand 在四边 longhand 齐备而简写缺失时按 CSS 规则拼出简写
// （浏览器 getComputedStyle(el).padding 恒有值）。与 expandBoxShorthand 互逆。
func composeBoxShorthand(out map[string]string, shorthand string, subs []string) {
	if _, has := out[shorthand]; has {
		return
	}
	if len(subs) != 4 {
		return
	}
	var v [4]string
	for i, s := range subs {
		x, ok := out[s]
		if !ok || x == "" {
			return
		}
		v[i] = x
	}
	out[shorthand] = composeFourValueShorthand(v[0], v[1], v[2], v[3])
}

// composeFourValueShorthand 按 CSS 简写最小化规则拼装四值（top right bottom left）：
// 四边相同 → 1 值；上下/左右相同 → 2 值；左右相同 → 3 值；否则 4 值。
// 例：1px/2px/1px/2px → "1px 2px"；3px/3px/3px/4px → "3px 3px 3px 4px"。
func composeFourValueShorthand(t, r, b, l string) string {
	switch {
	case t == r && r == b && b == l:
		return t
	case t == b && r == l:
		return t + " " + r
	case r == l:
		return t + " " + r + " " + b
	}
	return t + " " + r + " " + b + " " + l
}

// resolveTransformOrigin 把 transform-origin 的计算值解析为浏览器序列化的
// 绝对 px 形式 "Xpx Ypx"。v 为空（未声明）时等价于 "50% 50%"。
// 支持关键字 left/center/right/top/bottom、<length>（px/无单位）与 <percentage>。
func resolveTransformOrigin(v string, w, h float64) string {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return fmt.Sprintf("%gpx %gpx", w/2, h/2)
	}
	x := resolveOriginComponent(fields[0], w, h, true)
	y := resolveOriginComponent("center", w, h, false)
	if len(fields) > 1 {
		y = resolveOriginComponent(fields[1], w, h, false)
	}
	return fmt.Sprintf("%gpx %gpx", x, y)
}

// resolveOriginComponent 解析 transform-origin 的单个分量；isX 决定
// 百分比/关键字的参照轴（x → 宽，y → 高）。
func resolveOriginComponent(s string, w, h float64, isX bool) float64 {
	ref := h
	if isX {
		ref = w
	}
	t := strings.ToLower(strings.TrimSpace(s))
	switch t {
	case "left", "top":
		return 0
	case "right", "bottom":
		return ref
	case "center":
		return ref / 2
	}
	if strings.HasSuffix(t, "%") {
		if pct, err := strconv.ParseFloat(strings.TrimSuffix(t, "%"), 64); err == nil {
			return ref * pct / 100
		}
		return ref / 2
	}
	if val, err := strconv.ParseFloat(strings.TrimSuffix(t, "px"), 64); err == nil {
		return val
	}
	return ref / 2
}

// inheritedComputedProps 是 CSS 中 inherited:yes、且 getComputedStyle 会被读到的
// 属性：子元素未声明时继承**父元素的 computed 值**（CSS Cascade §2.1）。
var inheritedComputedProps = []string{
	"color", "cursor", "direction", "font-family", "font-size", "font-style",
	"font-variant", "font-weight", "letter-spacing", "line-height",
	"list-style-image", "list-style-position", "list-style-type",
	"text-align", "text-indent", "text-transform", "visibility",
	"white-space", "word-spacing", "border-collapse", "border-spacing",
	"caption-side", "empty-cells", "quotes",
}

// domParentElement 返回 DOM 父元素（跨 shadow 边界时返回 shadow host，与
// style.parentElement 的继承链语义一致）。
func domParentElement(el *dom.Element) *dom.Element {
	if el == nil {
		return nil
	}
	p := el.ParentNode()
	switch v := p.(type) {
	case *dom.Element:
		return v
	case *dom.ShadowRoot:
		return v.Host()
	}
	return nil
}

// inheritComputedProps 沿父链补齐 inherited 属性。
//
// ★ 为什么需要（D2）：computedStyleFor 只把「匹配到本元素的声明」写进
// map，未声明属性既不继承父值、也不出现在 map 里。调用方（getComputedStyle）
// 的白名单回退随后把它们补成 **CSS 初始值** —— 于是
// `<body style="font-size:40px;color:#ff0000"><div>` 里的 div 读
// `getComputedStyle(div).fontSize` 得到 "16px"、`color` 得到 undefined，
// 而同一次渲染的布局用 40px 红字。所有依赖 computed style 做决策的库
// （CSS-in-JS、虚拟列表测量、组件库尺寸探测、xterm fit）都会拿到错误值。
//
// computedStyleFor 有 per-element 缓存，沿父链递归代价可控。
func inheritComputedProps(out map[string]string, n dom.Node) {
	el, ok := n.(*dom.Element)
	if !ok || el == nil {
		return
	}
	missing := missingInheritedProps(out)
	if len(missing) == 0 {
		return
	}
	parent := domParentElement(el)
	if parent == nil {
		return
	}
	pcs := computedStyleFor(parent)
	if len(pcs) == 0 {
		return
	}
	for _, p := range missing {
		if v, has := pcs[p]; has {
			out[p] = v
		}
	}
}

// missingInheritedProps 返回 out 里尚未出现、且属于 inherited 集合的属性名。
// 抽成独立函数是为了让三个 computed 路径（元素 / 伪元素）共用同一判断。
func missingInheritedProps(out map[string]string) []string {
	var missing []string
	for _, p := range inheritedComputedProps {
		if _, has := out[p]; !has {
			missing = append(missing, p)
		}
	}
	return missing
}

// applyComputedSnapshot 用渲染树上的 resolved computed style 覆盖关键属性
// （font-size / color / font-family …），使 JS 读值与布局、绘制三方同源。
//
// font-size 尤其重要：字符串继承只能传递「父声明的原样文本」，若声明是
// `2em` / `150%` / `1.2rem` 就无法在 JS 侧得到 px —— 渲染树快照给出的
// 是布局实际使用的 used font-size（px）。
func applyComputedSnapshot(out map[string]string, n dom.Node) {
	if GetElementComputedSnapshot == nil {
		return
	}
	el, ok := n.(*dom.Element)
	if !ok || el == nil {
		return
	}
	// ★ 按需取快照：getComputedStyle 是热路径（IDE 场景 1e3 次调用实测
	//   ~47ms），快照要构造 map。绝大多数元素已在级联 map 里带上 font-size
	//   （绝对）与 color，此时无需访问渲染树 —— 只有「font-size 缺失」或
	//   「拿到的不是绝对长度（继承到 em/%/rem 文本）」或「color 缺失」时，
	//   才需要渲染树的 used 值。
	need := false
	if v, has := out["font-size"]; !has || !strings.HasSuffix(strings.TrimSpace(v), "px") {
		need = true
	}
	// 浏览器把 color 的 computed 值归一化为 rgb()/rgba() 文本，而级联 map 里
	// 是声明原样（"red" / "#ff0000" / "var(--x)" 解析结果）。非 rgb 前缀时
	// 也走快照归一化，避免 JS 侧按浏览器格式解析颜色失败。
	if v, has := out["color"]; !has || !strings.HasPrefix(strings.TrimSpace(v), "rgb") {
		need = true
	}
	// ★ H2：表单控件（input/button/select/textarea）在 UA 样式表里有**自己的**
	//   font（Arial 13.3333px；textarea 是 monospace）与 padding/border，且
	//   **不继承**文档字体。computedStyleFor 只级联作者样式 → 控件的 font-family
	//   会落到 inheritComputedProps 给的继承值（如容器的 sans-serif/16px）、
	//   padding 落空（undefined）。渲染树 style 是布局实际使用的值（含 UA 表，
	//   与绘制同源），故控件**无条件**取快照。
	if !need && isUAFormControl(el) {
		need = true
	}
	if !need {
		return
	}
	snap := GetElementComputedSnapshot(el)
	// ★ 2026-10-07 覆盖语义修正：快照只做「补缺 + 归一化 + 控件的 UA 属性」，
	//   不再对每个键**全量覆盖**级联结果。
	//
	//   为什么必须改（实测根因）：快照取的是**渲染树**上的 resolved style，而渲染树
	//   可能尚未按最新文档状态重算。实测（`in.SetChecked(true)` / `el.checked = true`
	//   之后）：同一个 <input> 上，`#c:checked{min-height:2px}` **生效**，而
	//   `#c:checked{color:rgb(9,9,9);font-size:22px;font-family:Courier}` **全部不生效**
	//   —— 差别只在「该属性是否在快照键里」：min-height 不在，color/font-size/
	//   font-family 在，于是它们被视作陈旧的快照值覆盖。级联（out）才是按**当前**
	//   文档状态现算的（同一轮里 `:checked` 的匹配已被证明正确：`querySelector`
	//   命中、兄弟组合器生效、matchedDecls 含该规则），比快照新。
	uaControl := isUAFormControl(el)
	for k, v := range snap {
		if v == "" {
			continue
		}
		cur, has := out[k]
		switch {
		case !has: // 级联缺该属性（含控件 UA 样式表提供的）→ 补
		case snapshotNormalizes(k, cur): // 级联是声明原样文本 → 归一化成浏览器格式
		case uaControl && uaControlSnapshotProp(k): // 控件的 UA 字体 / padding / border
		default:
			continue // 级联已有、无需归一化、非 UA 属性 → 保留级联值
		}
		out[k] = v
	}
}

// snapshotNormalizes 报告「级联里该属性的值是否需要归一化」——即声明原样文本要换成
// 浏览器 getComputedStyle 的格式（颜色归一化为 rgb()/rgba()，长度归一化为绝对 px，
// line-height/letter-spacing 的数值倍率折算成 px 或 normal）。只有这些才允许用渲染树
// 快照覆盖级联值；其余属性级联值即为最终值（见 applyComputedSnapshot 的说明）。
func snapshotNormalizes(name, cascaded string) bool {
	v := strings.TrimSpace(cascaded)
	switch name {
	case "color":
		return !strings.HasPrefix(v, "rgb")
	case "font-size":
		return !strings.HasSuffix(v, "px")
	case "line-height", "letter-spacing":
		return v != "" && v != "normal" && !strings.HasSuffix(v, "px")
	}
	return false
}

// uaControlSnapshotProp 报告该属性是否是「表单控件由 **UA 样式表**提供」的那些 ——
// 作者级联里它们要么缺失、要么被 inheritComputedProps 误填成继承值（控件在 UA 样式表
// 里**不继承**文档字体），必须取渲染树（作者 + UA 合成的）值。
func uaControlSnapshotProp(name string) bool {
	switch name {
	case "font-family", "font-size", "font-style", "font-variant":
		return true
	}
	return strings.HasPrefix(name, "padding-") ||
		strings.HasPrefix(name, "border-") ||
		name == "padding" || name == "border"
}

// isUAFormControl 报告元素是否是带 UA 样式（font-family/font-size/padding/
// border）的表单控件 —— 即 engine/html5/defaultcss.go 里
// `input, button, select, textarea` 那组规则覆盖的元素。
func isUAFormControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch strings.ToLower(el.LocalName()) {
	case "input", "button", "select", "textarea":
		return true
	}
	return false
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
func mediaMatches(rule *css.MediaRule, el dom.Node) bool {
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
	// ★ 归属优先：@media 与 matchMedia 必须用同一份上下文。el 可为 nil
	//   （无归属）→ 退化到包级 Provider，再退化到默认上下文。
	resolved := false
	if MediaQueryContextForElement != nil && el != nil {
		if c := MediaQueryContextForElement(el); c != nil {
			ctx, resolved = *c, true
		}
	}
	if !resolved && MediaQueryContextProvider != nil {
		if c := MediaQueryContextProvider(); c != nil {
			ctx = *c
		}
	}
	return css.MatchesAny(rule.Parsed, ctx)
}

// ── CSS.supports() 的条件求值 ──────────────────────────────────────────────
//
// 单参数 CSS.supports(conditionText) 的参数是 @supports 的**条件文本**，不是
// 选择器。支持的条件形态（与 Edge 逐项实测对齐，基线见
// dev/fixtures/webshot/h4_supports_bounds.html）：
//
//	<declaration>                    display:grid          → true
//	( <declaration> )                ( display : grid )    → true
//	not <condition>                  not (display:grid)    → false（内层为真）
//	<condition> and <condition>      (a:1) and (b:2)      → 与
//	<condition> or  <condition>      (a:1) or  (b:2)      → 或
//	@container (min-width:1px){}     at-rule 不是条件       → false
//	display / div > p / a:hover      非条件也非声明         → false

// cssSupportsCondition 求值一段 @supports 条件文本。
func cssSupportsCondition(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// at-rule 不是条件表达式（Edge：'@container (min-width:1px){}' → false）。
	if strings.HasPrefix(s, "@") {
		return false
	}
	// not 的优先级高于 and/or，先处理。
	if rest, ok := cutKeywordPrefix(s, "not"); ok {
		return !cssSupportsCondition(rest)
	}
	if i := indexTopLevelKeyword(s, "or"); i >= 0 {
		return cssSupportsCondition(s[:i]) || cssSupportsCondition(s[i+len("or"):])
	}
	if i := indexTopLevelKeyword(s, "and"); i >= 0 {
		return cssSupportsCondition(s[:i]) && cssSupportsCondition(s[i+len("and"):])
	}
	// 整串被一对括号包裹 → 剥掉后再求值（'(display:grid)' → 'display:grid'）。
	if inner, ok := stripOuterParens(s); ok {
		return cssSupportsCondition(inner)
	}
	// 裸声明 <property>: <value>。
	if i := strings.Index(s, ":"); i > 0 {
		return cssSupportsDeclaration(strings.ToLower(strings.TrimSpace(s[:i])),
			strings.TrimSpace(s[i+1:]))
	}
	// 既不是声明也不是组合条件（'display'、'div > p'、'a:hover'）→ 不支持。
	return false
}

// cssSupportsDeclaration 判定一条 <property>: <value> 声明是否为 wbui 认识的
// 语法。对齐 Edge：未知属性 → false；已知属性但值语法非法 → false；自定义属性
// （--x: ...）→ true；厂商前缀剥掉后再判。
func cssSupportsDeclaration(prop, val string) bool {
	if prop == "" || val == "" {
		return false
	}
	// 自定义属性（--x: ...）语法上永远合法。
	if strings.HasPrefix(prop, "--") {
		return true
	}
	if p, ok := stripVendorPrefix(prop); ok {
		prop = p
	}
	if !isKnownCSSProperty(prop) {
		return false
	}
	return cssValueSupported(prop, val)
}

// cutKeywordPrefix 在 s 以关键字 kw 开头且其后紧跟空白或 '(' 时返回剩余部分；
// 否则 ok=false（避免把 'nothing' 当成 'not' 处理）。
func cutKeywordPrefix(s, kw string) (string, bool) {
	if !strings.HasPrefix(s, kw) {
		return "", false
	}
	rest := s[len(kw):]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t' && rest[0] != '(') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// indexTopLevelKeyword 在括号深度 0 处查找独立关键字 kw（前后为空白），返回
// 其起始下标；不存在时 -1。'(display:grid) and (color:red)' 里括号内的文本不
// 会干扰：两个条件各自成对，深度 0 处只剩 ' and '。
func indexTopLevelKeyword(s, kw string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
			continue
		case ')':
			depth--
			continue
		}
		if depth != 0 || i+len(kw) > len(s) || s[i:i+len(kw)] != kw {
			continue
		}
		if i > 0 && !isCSSSpaceByte(s[i-1]) {
			continue
		}
		if j := i + len(kw); j < len(s) && !isCSSSpaceByte(s[j]) {
			continue
		}
		return i
	}
	return -1
}

// stripOuterParens 在一对**匹配**的外层括号包裹整个 s 时返回内部文本；
// '(a) and (b)' 不会被误剥（首括号在中途就闭合了）。
func stripOuterParens(s string) (string, bool) {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return "", false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return "", false
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	return s[1 : len(s)-1], true
}

// stripVendorPrefix 剥掉厂商前缀；'-webkit-mask-image' → 'mask-image'。
func stripVendorPrefix(prop string) (string, bool) {
	for _, p := range []string{"-webkit-", "-moz-", "-ms-", "-o-"} {
		if strings.HasPrefix(prop, p) && len(prop) > len(p) {
			return prop[len(p):], true
		}
	}
	return prop, false
}

func isCSSSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

// cssValueSupported 判定 value 是否是 prop 的合法语法。分三层：
//  1. 单关键字枚举属性（display/position/…）→ 值必须命中该属性的关键字集合
//     （因此 'display:grid grid' 与 'display:bogusvalue' 都判 false，与 Edge 一致）；
//  2. 多关键字枚举属性（color-scheme/scrollbar-gutter/…）→ 每个空白分隔的 token
//     都须命中集合（'color-scheme:light dark'、'scrollbar-gutter:stable both-edges' 合法）；
//  3. 其余已知属性 → 只做通用语法检查（cssValueSyntaxOK），保持 CSS.supports
//     作为特性探测 API 的宽松语义，不做完整 CSS 校验。
//
// 全局关键字（inherit/initial/unset/revert）对任何属性都合法。
func cssValueSupported(prop, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "inherit", "initial", "unset", "revert", "revert-layer":
		return true
	}
	if kw, ok := cssSingleKeywordProps[prop]; ok {
		return kw[strings.ToLower(value)]
	}
	if kw, ok := cssMultiKeywordProps[prop]; ok {
		toks := strings.Fields(strings.ToLower(value))
		if len(toks) == 0 {
			return false
		}
		for _, t := range toks {
			if !kw[t] {
				return false
			}
		}
		return true
	}
	switch prop {
	case "flex-flow":
		_, ok := css.ParseFlexFlow(value)
		return ok
	case "flex-direction":
		switch strings.ToLower(value) {
		case "row", "row-reverse", "column", "column-reverse":
			return true
		}
		return false
	case "flex-wrap":
		switch strings.ToLower(value) {
		case "nowrap", "wrap", "wrap-reverse":
			return true
		}
		return false
	}
	return cssValueSyntaxOK(value)
}

// cssValueSyntaxOK 对没有专门语法的属性做通用检查：非空、引号/括号配平、
// 不含声明级非法字符（; { }）。宽松通过——CSS.supports 是特性探测而非完整
// CSS 校验器。
func cssValueSyntaxOK(v string) bool {
	depth := 0
	var inStr byte
	for i := 0; i < len(v); i++ {
		c := v[i]
		if inStr != 0 {
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = c
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		case ';', '{', '}':
			return false
		}
	}
	return depth == 0 && inStr == 0
}

// tfNeedsGeometry 报告 transform 值是否含需要元素几何/字体度量的单位：
// %（参照 border-box 尺寸）、em/rem/ex/ch（参照 font-size）。
// transform 的函数名（translate/translateX/scale/skew/rotate/matrix）都不含
// 这些子串，故用子串判断即可。
func tfNeedsGeometry(v string) bool {
	if strings.Contains(v, "%") {
		return true
	}
	return strings.Contains(v, "em") || strings.Contains(v, "ex") || strings.Contains(v, "ch")
}

// cssKeywordSet 把关键字列表折叠成集合。
func cssKeywordSet(kw ...string) map[string]bool {
	m := make(map[string]bool, len(kw))
	for _, k := range kw {
		m[k] = true
	}
	return m
}

// cssSingleKeywordProps 是「值只能是**单个**关键字」的属性表。仅收纯关键字
// 属性——值可为长度/数字/函数的属性（width、opacity、aspect-ratio、line-height、
// font-weight…）不在此表，交给通用检查，否则会把 'aspect-ratio:1' 误判为 false。
var cssSingleKeywordProps = map[string]map[string]bool{
	"display": cssKeywordSet("block", "inline", "inline-block", "flex", "inline-flex",
		"grid", "inline-grid", "flow-root", "list-item", "table", "inline-table",
		"table-row", "table-row-group", "table-cell", "table-caption", "contents", "none"),
	"position":         cssKeywordSet("static", "relative", "absolute", "fixed", "sticky"),
	"float":            cssKeywordSet("none", "left", "right", "inline-start", "inline-end"),
	"clear":            cssKeywordSet("none", "left", "right", "both", "inline-start", "inline-end"),
	"visibility":       cssKeywordSet("visible", "hidden", "collapse"),
	"box-sizing":       cssKeywordSet("content-box", "border-box"),
	"object-fit":       cssKeywordSet("fill", "contain", "cover", "none", "scale-down"),
	"table-layout":     cssKeywordSet("auto", "fixed"),
	"border-collapse":  cssKeywordSet("separate", "collapse"),
	"direction":        cssKeywordSet("ltr", "rtl"),
	"writing-mode":     cssKeywordSet("horizontal-tb", "vertical-rl", "vertical-lr", "sideways-rl", "sideways-lr"),
	"text-orientation": cssKeywordSet("mixed", "upright", "sideways"),
	"mix-blend-mode": cssKeywordSet("normal", "multiply", "screen", "overlay", "darken",
		"lighten", "color-dodge", "color-burn", "hard-light", "soft-light",
		"difference", "exclusion", "hue", "saturation", "color", "luminosity", "plus-lighter"),
	"isolation":       cssKeywordSet("auto", "isolate"),
	"text-wrap":       cssKeywordSet("wrap", "nowrap", "balance", "pretty", "stable"),
	"scroll-behavior": cssKeywordSet("auto", "smooth"),
	"pointer-events":  cssKeywordSet("auto", "none", "all"),
	"user-select":     cssKeywordSet("auto", "none", "text", "all", "contain"),
	"resize":          cssKeywordSet("none", "both", "horizontal", "vertical", "block", "inline"),
	"white-space":     cssKeywordSet("normal", "nowrap", "pre", "pre-wrap", "pre-line", "break-spaces"),
	"word-break":      cssKeywordSet("normal", "keep-all", "break-all", "break-word"),
	"line-break":      cssKeywordSet("auto", "loose", "normal", "strict", "anywhere"),
	"overflow-wrap":   cssKeywordSet("normal", "break-word", "anywhere"),
	"word-wrap":       cssKeywordSet("normal", "break-word", "anywhere"),
	"text-align":      cssKeywordSet("left", "right", "center", "justify", "start", "end", "match-parent"),
	"text-transform":  cssKeywordSet("none", "capitalize", "uppercase", "lowercase", "full-width"),
	"font-style":      cssKeywordSet("normal", "italic", "oblique"),
	"list-style-type": cssKeywordSet("disc", "circle", "square", "decimal", "decimal-leading-zero",
		"lower-roman", "upper-roman", "lower-alpha", "upper-alpha", "lower-latin",
		"upper-latin", "lower-greek", "none"),
	"list-style-position": cssKeywordSet("inside", "outside"),
	"overflow":            cssKeywordSet("visible", "hidden", "scroll", "auto", "clip"),
	"overflow-x":          cssKeywordSet("visible", "hidden", "scroll", "auto", "clip"),
	"overflow-y":          cssKeywordSet("visible", "hidden", "scroll", "auto", "clip"),
	"backface-visibility": cssKeywordSet("visible", "hidden"),
	"transform-style":     cssKeywordSet("flat", "preserve-3d"),
	"object-position": cssKeywordSet("center", "top", "bottom", "left", "right",
		"center center", "center top", "center bottom", "left center", "right center"),
}

// cssMultiKeywordProps 是「值可由多个关键字组成」的属性表；每个空白分隔的
// token 都须命中集合。'contain: layout paint'、'color-scheme: light dark'、
// 'scrollbar-gutter: stable both-edges' 都合法。
var cssMultiKeywordProps = map[string]map[string]bool{
	"contain": cssKeywordSet("none", "strict", "content", "size", "layout", "style",
		"paint", "inline-size", "block-size"),
	"color-scheme":          cssKeywordSet("normal", "light", "dark", "only"),
	"scrollbar-gutter":      cssKeywordSet("auto", "stable", "always", "both-edges"),
	"overscroll-behavior":   cssKeywordSet("auto", "contain", "none"),
	"overscroll-behavior-x": cssKeywordSet("auto", "contain", "none"),
	"overscroll-behavior-y": cssKeywordSet("auto", "contain", "none"),
	"touch-action": cssKeywordSet("auto", "none", "manipulation", "pan-x", "pan-y",
		"pan-left", "pan-right", "pan-up", "pan-down", "pinch-zoom"),
	"font-variant-numeric": cssKeywordSet("normal", "ordinal", "slashed-zero",
		"lining-nums", "oldstyle-nums", "proportional-nums", "tabular-nums",
		"diagonal-fractions", "stacked-fractions"),
	"font-variant": cssKeywordSet("normal", "none", "small-caps", "all-small-caps",
		"petite-caps", "all-petite-caps", "unicase", "titling-caps", "common-ligatures",
		"no-common-ligatures", "discretionary-ligatures", "no-discretionary-ligatures",
		"historical-ligatures", "no-historical-ligatures", "contextual",
		"no-contextual", "ordinal", "slashed-zero", "tabular-nums", "oldstyle-nums",
		"proportional-nums", "lining-nums", "small-caps-nums"),
	"grid-auto-flow": cssKeywordSet("row", "column", "dense"),
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

// modernCSSProps 是对 knownCSSProps 的补充：现代 CSS 属性（逻辑属性、视觉效果、
// 滚动与书写模式、字体变体等），也包括 wbui 已经消费但基础表漏收的属性
// （contain/will-change/mix-blend-mode/writing-mode/mask-image 在
// engine/rendering、engine/layout、engine/style 里都有真实实现）。
// 单独成表而非混入 knownCSSProps 的字母序字面量，便于增删；isKnownCSSProperty
// 合并查询两者。
var modernCSSProps = map[string]bool{
	"accent-color": true, "background-position": true, "background-repeat": true,
	"backface-visibility": true, "border-spacing": true, "color-scheme": true,
	"column-count": true, "column-fill": true, "column-width": true,
	"contain": true, "content-visibility": true, "font-stretch": true,
	"font-variant-numeric": true, "grid-auto-flow": true, "grid-column-start": true,
	"grid-column-end": true, "grid-row-start": true, "grid-row-end": true,
	"hyphens": true, "image-rendering": true, "isolation": true, "line-break": true,
	"mask-image": true, "mask-position": true, "mask-repeat": true, "mask-size": true,
	"mix-blend-mode": true, "orphans": true, "overscroll-behavior": true,
	"overscroll-behavior-x": true, "overscroll-behavior-y": true, "perspective": true,
	"perspective-origin": true, "scroll-margin": true, "scroll-padding": true,
	"scrollbar-gutter": true, "tab-size": true, "text-orientation": true,
	"text-underline-offset": true, "text-wrap": true, "touch-action": true,
	"transform-style": true, "widows": true, "will-change": true, "writing-mode": true,
}

// isKnownCSSProperty 判断属性名是否在已知 CSS 属性表中（基础表 + 现代属性补充表）。
func isKnownCSSProperty(prop string) bool {
	prop = strings.ToLower(prop)
	return knownCSSProps[prop] || modernCSSProps[prop]
}

// Silence unused import warning
var _ = fmt.Sprintf

// withDisplayFallback 给 getComputedStyle 的结果补 display 默认值：级联只收录
// **声明过**的属性，未声明 display 的元素（span 等）不在表里 → JS 读
// getComputedStyle(el).display 得 undefined（实测某个被 hidden 的
// span 读 disp=undefined，污染一切依赖 display 的逻辑）。按 UA 语义回退：
// hidden → "none"；块级 → "block"；可替换/表单控件 → "inline-block"；其它 → "inline"。
func withDisplayFallback(m map[string]string, n dom.Node) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	if _, ok := m["display"]; ok {
		return m
	}
	e, ok := n.(*dom.Element)
	if !ok || e == nil {
		return m
	}
	// ★ 与 uaDefaultDisplayFor 共用同一张 UA display 表（单一真相源）。
	// 此前本函数内嵌了一份与之重复的 switch，两张表各自演化：`li` 在这份里是
	// "block"、在 UA 样式表（defaultcss.go）与 applyDefaultDisplay（resolver.go）
	// 里是 "list-item" —— getComputedStyle(el).display 因此报 block，与布局实际
	// 使用的 list-item 脱节（G7 探针：Edge list-item vs wbui block）。
	m["display"] = uaDefaultDisplayFor(e)
	return m
}

// uaDefaultDisplayFor 返回元素的 UA 默认 display 值：hidden → "none"；块级 →
// "block"；可替换/表单控件 → "inline-block"；其它 → "inline"。供
// getComputedStyle 在级联未声明 display 时回退使用 —— 否则 JS 读到 undefined，
// 污染一切依赖 display 的逻辑（含 hidden 缺陷的验证）。
func uaDefaultDisplayFor(e *dom.Element) string {
	if e == nil {
		return "inline"
	}
	if e.HasAttribute("hidden") {
		return "none"
	}
	switch e.LocalName() {
	// ★ li 的 UA display 是 list-item，不是 block：与 UA 样式表
	//   defaultcss.go 的 `li{display:list-item}`、applyDefaultDisplay
	//   （resolver.go）保持一致（三处原先不一致 → computed 读到 block）。
	case "li":
		return "list-item"
	// ★ H6（第 5 次监督轮）：table 系列的 UA display 与 defaultcss.go 的
	//   `table { display: table }` / `tr { display: table-row }` 等规则保持一致
	//   —— 本表此前缺这些分支，`getComputedStyle(table).display` 回退成
	//   "inline"（Edge 实测 g7_listpseudo 的 tb = "table"）。
	case "table":
		return "table"
	case "caption":
		return "table-caption"
	case "thead":
		return "table-header-group"
	case "tbody":
		return "table-row-group"
	case "tfoot":
		return "table-footer-group"
	case "tr":
		return "table-row"
	case "td", "th":
		return "table-cell"
	case "col":
		return "table-column"
	case "colgroup":
		return "table-column-group"
	case "html", "body", "div", "p", "section", "article", "header", "footer",
		"main", "nav", "aside", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol",
		"form", "blockquote", "pre", "figure", "figcaption", "fieldset",
		"details", "summary", "dialog", "address", "hr":
		return "block"
	case "button", "input", "select", "textarea", "img", "svg", "canvas",
		"video", "audio", "iframe", "embed", "object", "progress", "meter":
		return "inline-block"
	}
	return "inline"
}

// uaInitialComputedValues 是 CSS 规范里各属性的**初始值**（initial value）表。
// 浏览器 getComputedStyle 对每个属性恒有值（未声明即初始值）；引擎的级联 map
// 只含声明值，未声明属性会读到 undefined —— 依赖它的 JS（parseFloat(fontSize)、
// 判断 min-width/max-width、读 pointer-events 做命中判断等）会走错分支。
// 仅在 JS 对象层回退，不影响 computedStyleFor 的 map 与布局/继承计算。
var uaInitialComputedValues = map[string]string{
	"fontSize":       "16px",
	"lineHeight":     "normal",
	"fontWeight":     "400",
	"fontStyle":      "normal",
	"fontFamily":     "",
	"pointerEvents":  "auto",
	"alignItems":     "normal",
	"justifyContent": "normal",
	"alignSelf":      "auto",
	"minWidth":       "auto",
	"maxWidth":       "none",
	"minHeight":      "auto",
	"maxHeight":      "none",
	"overflowX":      "visible",
	"overflowY":      "visible",
	// ★ H5：scrollbar-gutter 的 CSS 初始值是 auto（不为滚动条预留空间）。
	"scrollbarGutter": "auto",
	// ★ H6：text-wrap 的初始值是 wrap（Edge 实测 h6_misc_props 的默认元素 tw=wrap）。
	"textWrap":           "wrap",
	"zIndex":             "auto",
	"boxSizing":          "content-box",
	"textAlign":          "start",
	"whiteSpace":         "normal",
	"flexDirection":      "row",
	"flexWrap":           "nowrap",
	"borderRadius":       "0px",
	"opacity":            "1",
	"visibility":         "visible",
	"position":           "static",
	"transform":          "none",
	"cursor":             "auto",
	"verticalAlign":      "baseline",
	"borderStyle":        "none",
	"borderColor":        "currentcolor",
	"borderTopStyle":     "none",
	"borderRightStyle":   "none",
	"borderBottomStyle":  "none",
	"borderLeftStyle":    "none",
	"backgroundImage":    "none",
	"backgroundRepeat":   "repeat",
	"backgroundSize":     "auto",
	"backgroundPosition": "0% 0%",
	"backgroundClip":     "border-box",
	"listStyle":          "outside none disc",
	"textDecoration":     "none solid currentcolor",
	"outline":            "none",
	"float":              "none",
	"clear":              "none",
	"userSelect":         "auto",
	"wordBreak":          "normal",
	"textOverflow":       "clip",
	"overflowWrap":       "normal",
	"letterSpacing":      "normal",
	"top":                "auto",
	"left":               "auto",
	"right":              "auto",
	"bottom":             "auto",
	"width":              "auto",
	"height":             "auto",
	"gap":                "normal",
	"rowGap":             "normal",
	"columnGap":          "normal",
	// ★ 列表 / 表格 / 断行 / 合成相关（G7、G5、G2 探针实测 undefined）：
	//   浏览器对**每个**属性恒返回计算值（未声明 = CSS 初始值），缺失会让依赖
	//   这些值的 JS 走错分支。
	"listStyleType":     "disc",
	"listStylePosition": "outside",
	"listStyleImage":    "none",
	"borderCollapse":    "separate",
	"borderSpacing":     "0px",
	"lineBreak":         "auto",
	"willChange":        "auto",
	"clipPath":          "none",
	"filter":            "none",
}

// expandOverflowShorthand 把 overflow 简写展开为 overflow-x / overflow-y：
// HTML 语义（CSS Overflow 3）：单值  同时设定两轴；双值
//
//	分别为 x / y。已显式声明的长写不覆盖。
func expandOverflowShorthand(out map[string]string) {
	if _, ok := out["overflow-x"]; ok {
		if _, ok2 := out["overflow-y"]; ok2 {
			return
		}
	}
	v, ok := out["overflow"]
	if !ok {
		return
	}
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	x, y := parts[0], parts[0]
	if len(parts) >= 2 {
		y = parts[1]
	}
	if _, ok := out["overflow-x"]; !ok {
		out["overflow-x"] = x
	}
	if _, ok := out["overflow-y"]; !ok {
		out["overflow-y"] = y
	}
}

// expandBorderStyleLonghand 由 border / border-style 简写展开四边的 style 与
// color 长写（width 已由 bwTok 路径处理）。浏览器 getComputedStyle 下
//
//	的 borderTopStyle/borderTopColor 恒有值。
func expandBorderStyleLonghand(out map[string]string) {
	styleTok := func(s string) string {
		for _, f := range strings.Fields(s) {
			switch strings.ToLower(f) {
			case "none", "hidden", "dotted", "dashed", "solid", "double",
				"groove", "ridge", "inset", "outset":
				return strings.ToLower(f)
			}
		}
		return ""
	}
	colorTok := func(s string) string {
		for _, f := range strings.Fields(s) {
			if len(f) > 0 && (f[0] == '#' || strings.HasPrefix(f, "rgb") ||
				strings.HasPrefix(f, "hsl") || f == "transparent" || f == "currentcolor") {
				return f
			}
			// 颜色关键字（red / blue …）：排除已知的 style 与 width token
			if len(f) > 2 && styleTok(f) == "" && f[0] >= 'a' && f[0] <= 'z' {
				if _, err := strconv.ParseFloat(f, 64); err != nil && !strings.HasSuffix(f, "px") {
					return f
				}
			}
		}
		return ""
	}
	src := []string{}
	if v, ok := out["border"]; ok {
		src = append(src, v)
	}
	if v, ok := out["border-style"]; ok {
		src = append(src, v)
	}
	for _, s := range src {
		if st := styleTok(s); st != "" {
			for _, lh := range []string{"border-top-style", "border-right-style", "border-bottom-style", "border-left-style"} {
				if _, ok := out[lh]; !ok {
					out[lh] = st
				}
			}
			break
		}
	}
	for _, s := range src {
		if c := colorTok(s); c != "" {
			for _, lh := range []string{"border-top-color", "border-right-color", "border-bottom-color", "border-left-color"} {
				if _, ok := out[lh]; !ok {
					out[lh] = c
				}
			}
			break
		}
	}
}

// expandFontShorthand 把 font 简写展开为字体子属性长写。浏览器
// getComputedStyle 对 `font` 简写恒返回展开后的 font-style / font-variant /
// font-weight / font-size / line-height / font-family（getPropertyValue
// ('font-size') 有值）。级联 map 只存简写键 "font" 时，读 fontSize 会落空并回退
// 默认值 16px，而布局用的是简写里的字号 —— 同一元素「读到的字号」与「画出来的
// 字号」脱节。解析复用 css.ParseFontShorthand（与 engine/style 同一份实现）。
// 已显式声明的长写不覆盖（与 expandOverflowShorthand 一致）。
func expandFontShorthand(out map[string]string) {
	v, ok := out["font"]
	if !ok {
		return
	}
	st, variant, weight, size, lineHeight, family, ok2 := css.ParseFontShorthand(v)
	if !ok2 {
		return
	}
	set := func(k, val string) {
		if val == "" {
			return
		}
		if _, exists := out[k]; !exists {
			out[k] = val
		}
	}
	set("font-style", st)
	set("font-variant", variant)
	set("font-weight", weight)
	set("font-size", size)
	set("font-family", strings.Trim(family, `"'`))
	if lineHeight != "" {
		if strings.EqualFold(strings.TrimSpace(lineHeight), "normal") {
			set("line-height", "normal")
		} else {
			set("line-height", lineHeight)
		}
	}
}

// expandListStyleShorthand 把 list-style 简写展开为 list-style-type /
// list-style-position / list-style-image。浏览器 getComputedStyle 恒返回展开后的
// 长写；级联 map 只存简写键时读 listStyleType 会落空。关键字归类与
// engine/style 的 `case "list-style"` 同思路：类型关键字 → type，inside/outside
// → position，url()/渐变 → image。已显式声明的长写不覆盖。
func expandListStyleShorthand(out map[string]string) {
	v, ok := out["list-style"]
	if !ok {
		return
	}
	set := func(k, val string) {
		if val == "" {
			return
		}
		if _, exists := out[k]; !exists {
			out[k] = val
		}
	}
	for _, tok := range strings.Fields(v) {
		low := strings.ToLower(tok)
		switch low {
		case "disc", "circle", "square", "decimal", "decimal-leading-zero",
			"lower-alpha", "upper-alpha", "lower-roman", "upper-roman",
			"lower-greek", "lower-latin", "upper-latin", "armenian", "georgian",
			"none":
			set("list-style-type", low)
		case "inside", "outside":
			set("list-style-position", low)
		default:
			if strings.HasPrefix(low, "url(") || strings.HasPrefix(low, "linear-gradient(") ||
				strings.HasPrefix(low, "radial-gradient(") {
				set("list-style-image", tok)
			}
		}
	}
}
