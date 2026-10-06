// Translation of: Source/WebKit/WebView.h
//                  Source/WebKit/WebView.cpp
//                  Source/WebKit/UIProcess/win/WebView.h
// Completeness: 40%

package webkit

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"wb-ui/bridge"
	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/js/bindings"
	"wb-ui/engine/js/jsc"
	"wb-ui/engine/layout"
	"wb-ui/engine/page"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/popover"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

const (
	DefaultWebViewWidth  = 800
	DefaultWebViewHeight = 600
)

// ★ 多 WebView 绑定分派：bindings 包的几何/事件回调指针（GetElementBoxRect、
// OnStyleNodeAdded、ViewportWidth 等）是包级全局，单 WebView 时代由每个
// WebView 的 LoadHTML/injectRenderTreeBridge 直接赋值（后者覆盖前者）。
// 多 WebView（如直播挂件助手：配置窗口 + N 个挂件离屏窗口）时，后 LoadHTML
// 的 WebView（挂件重建）会把配置窗口的绑定覆盖掉 → 配置 JS 的
// getBoundingClientRect 用挂件的 RenderView 查找元素 → 查不到返回 0 →
// 画布合成位置全错（"部件跳到画布外上方"根因）。
// 修复：每个 WebView 的绑定闭包存进自己的 wvBridge 表；bindings 包级指针
// 改为分派器（按元素 OwnerDocument / 解释器 找到所属 WebView，调它的闭包）。
// 单 WebView 行为与之前完全一致（注册表只有一个），多 WebView 各查各的。
var (
	webviewsMu     sync.RWMutex
	webviews       = map[*WebView]bool{}
	webviewBridges = map[*WebView]*wvBridge{}
	webviewInterps = map[*jsc.Interpreter]*WebView{}
	bridgeDispatch sync.Once
)

// wvBridge 保存某个 WebView 自己的绑定闭包（injectRenderTreeBridge / LoadHTML
// 注入时存入，分派器调用时按元素归属取出）。
type wvBridge struct {
	getElementBoxRect       func(el *dom.Element) (float64, float64, float64, float64)
	getElementBoxRectFast   func(el *dom.Element) (float64, float64, float64, float64)
	getElementScrollMetrics func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool)
	getElementScrollOffset  func(el *dom.Element) (float64, float64)
	setElementScrollOffset  func(el *dom.Element, x, y float64)
	getTextBasePos          func(n dom.Node) (float64, float64, bool)
	getElementComputedFont  func(el *dom.Element) (string, float64, int, string)
	onStyleNodeAdded        func(n dom.Node)
	onInlineStyleChanged    func(n dom.Node)
	onClassChanged          func(el *dom.Element)
	onNodeInserted          func(n dom.Node)
	onNodeRemoved           func(n dom.Node)
	mediaQueryCtx           func() *css.MediaQueryContext
	iframeSrcChanged        func(el *dom.Element, src string)
}

func registerWebView(wv *WebView) {
	webviewsMu.Lock()
	webviews[wv] = true
	webviewsMu.Unlock()
}

func wvBridgeOf(wv *WebView) *wvBridge {
	webviewsMu.Lock()
	defer webviewsMu.Unlock()
	b := webviewBridges[wv]
	if b == nil {
		b = &wvBridge{}
		webviewBridges[wv] = b
	}
	return b
}

// webViewForNode 返回包含该节点的 WebView（按 OwnerDocument 匹配）。
func webViewForNode(n dom.Node) *WebView {
	if n == nil {
		return nil
	}
	doc := n.OwnerDocument()
	if doc == nil {
		return nil
	}
	webviewsMu.RLock()
	defer webviewsMu.RUnlock()
	for wv := range webviews {
		if wv.mainFrame != nil && wv.mainFrame.Document() == doc {
			return wv
		}
	}
	return nil
}

// webViewForDocument 返回当前装配着该 Document 的 WebView（按 mainFrame 的
// 文档标识匹配）。★ 不复用 webViewForNode：它按 n.OwnerDocument() 反查，而
// Document 自身的 ownerDocument 在规范里是 null（Node 语义），拿 Document
// 去查会落空。
func webViewForDocument(doc *dom.Document) *WebView {
	if doc == nil {
		return nil
	}
	webviewsMu.RLock()
	defer webviewsMu.RUnlock()
	for wv := range webviews {
		if wv.mainFrame != nil && wv.mainFrame.Document() == doc {
			return wv
		}
	}
	return nil
}

// webViewForInterpreter 返回拥有该 JS 解释器的 WebView。
func webViewForInterpreter(in *jsc.Interpreter) *WebView {
	if in == nil {
		return nil
	}
	webviewsMu.RLock()
	defer webviewsMu.RUnlock()
	return webviewInterps[in]
}

// installBridgeDispatch 安装分派器（只装一次）：bindings 包级指针指向
// 分派闭包，按元素/解释器归属路由到对应 WebView 的 wvBridge 闭包。
func installBridgeDispatch() {
	bridgeDispatch.Do(func() {
		// dom 层内容属性事件处理器执行器（HTML onclick="code" 语义）：
		// 内容属性的 code 视为元素创建时注册的监听器，DispatchEvent 派发
		// 路径（JS dispatchEvent / Go 直接派发 / 冒泡祖先）都应执行它——
		// 此前只有 Interaction/Host 点击管线手动执行，页面 JS 用
		// dispatchEvent(new MouseEvent('click')) 模拟点击时 onclick 落空。
		// 按元素归属定位 WebView 后复用 execInlineHandler（this=元素）。
		dom.InlineEventAttrRunner = func(el *dom.Element, eventType string) {
			attr := "on" + eventType
			if el.GetAttribute(attr) == "" {
				return
			}
			wv := webViewForNode(el)
			if wv == nil {
				return
			}
			execInlineHandler(wv, el, attr)
		}
		bindings.GetElementBoxRect = func(el *dom.Element) (float64, float64, float64, float64) {
			wv := webViewForNode(el)
			if wv == nil {
				return 0, 0, 0, 0
			}
			if b := wvBridgeOf(wv); b != nil && b.getElementBoxRect != nil {
				return b.getElementBoxRect(el)
			}
			return 0, 0, 0, 0
		}
		bindings.ElementFromPoint = func(in *jsc.Interpreter, x, y float64) *dom.Element {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return nil
			}
			rv := wv.RenderView()
			if rv == nil {
				return nil
			}
			wv.EnsureHitTestReady()
			rv = wv.RenderView()
			if rv == nil {
				return nil
			}
			return rendering.HitTest(rv, x, y, "")
		}
		// document.readyState 接线：值随所属 frame 的加载阶段推进
		// （SetDocument → "interactive"，装载收尾 → "complete"）。
		bindings.DocumentReadyState = func(doc *dom.Document) string {
			wv := webViewForDocument(doc)
			if wv == nil || wv.mainFrame == nil {
				return "complete"
			}
			fr := wv.mainFrame.Frame()
			if fr == nil {
				return "complete"
			}
			return fr.ReadyState()
		}
		// document.styleSheets 接线：<style> 提取的 + 运行时注入的 <link>。
		bindings.DocumentStyleSheets = func(doc *dom.Document) []*css.CSSStyleSheet {
			wv := webViewForDocument(doc)
			if wv == nil || wv.mainFrame == nil {
				return nil
			}
			fr := wv.mainFrame.Frame()
			if fr == nil {
				return nil
			}
			return fr.StyleSheets()
		}
		bindings.GetElementBoxRectFast = func(el *dom.Element) (float64, float64, float64, float64) {
			wv := webViewForNode(el)
			if wv == nil {
				return 0, 0, 0, 0
			}
			if b := wvBridgeOf(wv); b != nil && b.getElementBoxRectFast != nil {
				return b.getElementBoxRectFast(el)
			}
			return 0, 0, 0, 0
		}
		bindings.GetElementScrollMetrics = func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool) {
			wv := webViewForNode(el)
			if wv == nil {
				return 0, 0, 0, 0, false
			}
			if b := wvBridgeOf(wv); b != nil && b.getElementScrollMetrics != nil {
				return b.getElementScrollMetrics(el)
			}
			return 0, 0, 0, 0, false
		}
		bindings.GetElementScrollOffset = func(el *dom.Element) (float64, float64) {
			wv := webViewForNode(el)
			if wv == nil {
				return 0, 0
			}
			if b := wvBridgeOf(wv); b != nil && b.getElementScrollOffset != nil {
				return b.getElementScrollOffset(el)
			}
			return 0, 0
		}
		bindings.SetElementScrollOffset = func(el *dom.Element, x, y float64) {
			wv := webViewForNode(el)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.setElementScrollOffset != nil {
				b.setElementScrollOffset(el, x, y)
			}
		}
		bindings.GetTextBasePos = func(n dom.Node) (float64, float64, bool) {
			wv := webViewForNode(n)
			if wv == nil {
				return 0, 0, false
			}
			if b := wvBridgeOf(wv); b != nil && b.getTextBasePos != nil {
				return b.getTextBasePos(n)
			}
			return 0, 0, false
		}
		bindings.GetElementComputedFont = func(el *dom.Element) (string, float64, int, string) {
			wv := webViewForNode(el)
			if wv == nil {
				return "sans-serif", 14, 400, "normal"
			}
			if b := wvBridgeOf(wv); b != nil && b.getElementComputedFont != nil {
				return b.getElementComputedFont(el)
			}
			return "sans-serif", 14, 400, "normal"
		}
		bindings.OnStyleNodeAdded = func(n dom.Node) {
			wv := webViewForNode(n)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.onStyleNodeAdded != nil {
				b.onStyleNodeAdded(n)
			}
		}
		bindings.OnInlineStyleChanged = func(n dom.Node) {
			wv := webViewForNode(n)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.onInlineStyleChanged != nil {
				b.onInlineStyleChanged(n)
			}
		}
		bindings.OnClassChanged = func(el *dom.Element) {
			wv := webViewForNode(el)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.onClassChanged != nil {
				b.onClassChanged(el)
			}
		}
		bindings.OnNodeInserted = func(n dom.Node) {
			wv := webViewForNode(n)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.onNodeInserted != nil {
				b.onNodeInserted(n)
			}
			// ★ 运行时动态插入的 <script src>（createElement("script") + src +
			//   appendChild）：浏览器语义要求取回并执行、执行完派发 load（onload
			//   回调）。gou-ide 的区域包 client 半全靠这个模式装载编译 bundle 并
			//   注册槽位（见 /plugins-assets/ui-titlebar/client.js）——此前引擎只
			//   处理装配期的 <script>，动态脚本永不执行 → 槽位表恒为空 → 桌面端
			//   各区域停在「未装配」空态（实测 2026-09-25）。
			if el, ok := n.(*dom.Element); ok && el.IsConnected() {
				switch el.LocalName() {
				case "script":
					if src := el.GetAttribute("src"); src != "" {
						wv.handleDynamicScript(el, src)
					}
				case "link":
					if isStyleSheetLink(el) {
						if href := el.GetAttribute("href"); href != "" {
							wv.handleDynamicStylesheet(el, href)
						}
					}
				}
			}
		}
		// ★ <script src> 属性变化（已连接文档）：走同一条动态脚本装载路径。
		bindings.ScriptSrcChanged = func(el *dom.Element, src string) {
			if wv := webViewForNode(el); wv != nil {
				wv.handleDynamicScript(el, src)
			}
		}
		// ★ <link rel=stylesheet> 的 rel/href 变化（已连接文档）：同一条动态样式表路径。
		bindings.StylesheetHrefChanged = func(el *dom.Element, href string) {
			if wv := webViewForNode(el); wv != nil {
				wv.handleDynamicStylesheet(el, href)
			}
		}
		bindings.OnNodeRemoved = func(n dom.Node) {
			wv := webViewForNode(n)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.onNodeRemoved != nil {
				b.onNodeRemoved(n)
			}
		}
		bindings.MediaQueryContextProvider = func() *css.MediaQueryContext {
			// 无元素上下文：由最近一次设置 mediaQueryCtx 的 WebView 提供。
			// 多 WebView 场景 matchMedia 语义不精确（少见），保持单值兜底。
			webviewsMu.RLock()
			defer webviewsMu.RUnlock()
			for wv := range webviews {
				if b := webviewBridges[wv]; b != nil && b.mediaQueryCtx != nil {
					return b.mediaQueryCtx()
				}
			}
			return nil
		}
		// ★ 按解释器归属解析（matchMedia 优先用这条）：Provider 是包级单例，
		// 多 WebView 时只能指向其中之一 —— 实测两个 WebView 同存时被测 WebView
		// 的 DSF 变化不会反映到 matchMedia（读到另一个 WebView 的像素比），
		// 甚至与自身 getComputedStyle 结论矛盾。归属解析后每个页面只用自己的
		// 视口/像素比。
		bindings.MediaQueryContextForInterpreter = func(in *jsc.Interpreter) *css.MediaQueryContext {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return nil
			}
			if b := wvBridgeOf(wv); b != nil && b.mediaQueryCtx != nil {
				return b.mediaQueryCtx()
			}
			return nil
		}
		// ★ getComputedStyle 的 @media 判定（bindings.computedStyleFor →
		// mediaMatches）也要同一份上下文，但那里有元素、没有解释器：
		// 按元素归属解析，避免多 WebView 并存时读到另一个页面的像素比
		// （表现为 matchMedia 已命中而 computed style 停在基础规则）。
		bindings.MediaQueryContextForElement = func(n dom.Node) *css.MediaQueryContext {
			wv := webViewForNode(n)
			if wv == nil {
				return nil
			}
			if b := wvBridgeOf(wv); b != nil && b.mediaQueryCtx != nil {
				return b.mediaQueryCtx()
			}
			return nil
		}
		bindings.IFrameSrcChanged = func(el *dom.Element, src string) {
			wv := webViewForNode(el)
			if wv == nil {
				return
			}
			if b := wvBridgeOf(wv); b != nil && b.iframeSrcChanged != nil {
				b.iframeSrcChanged(el, src)
			}
		}
		// window.innerWidth/innerHeight 按解释器归属分派（挂件 Resize
		// 不再覆盖配置窗口的视口尺寸）。
		bindings.ViewportSizeForInterpreter = func(in *jsc.Interpreter) (float64, float64, bool) {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return 0, 0, false
			}
			return float64(wv.width), float64(wv.height), true
		}
		// window.devicePixelRatio 同样按解释器归属分派（每个 WebView 各自的
		// 设备像素比，Emulation 只影响被仿真的那个页面）。
		bindings.DevicePixelRatioForInterpreter = func(in *jsc.Interpreter) (float64, bool) {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return 0, false
			}
			return wv.DeviceScaleFactor(), true
		}
		// popover（HTML §6.12）的异步任务队列：toggle 事件必须异步派发
		// （规范 queue an element task），这里按元素找到所属 WebView 的 JS
		// 事件循环。bindings 侧无法提供它——那里没有「元素 → 解释器」的映射，
		// 而字体/样式等钩子只需要元素。无解释器（纯布局宿主/未初始化）时
		// 退化为同步派发，保证状态机仍然推进。
		popover.QueueTask = func(el *dom.Element, fn func()) {
			wv := webViewForNode(el)
			if wv == nil || wv.jsInterpreter == nil {
				fn()
				return
			}
			in := wv.jsInterpreter
			loop := in.EnsureEventLoop()
			if loop == nil {
				fn()
				return
			}
			cb := in.NewNativeFunction("popover_toggle_task",
				func(*jsc.Interpreter, jsc.JSValue, []jsc.JSValue) jsc.JSValue {
					fn()
					return jsc.Undefined()
				}, 0)
			_ = loop.SetTimeout(jsc.FunctionValue(cb), 0)
		}
	})
}

type WebView struct {
	page          *page.Page
	mainFrame     *WebFrame
	settings      *page.Settings
	jsInterpreter *jsc.Interpreter
	jsLogger      *jsc.BufferLogger
	width, height int
	// colorScheme 是 (prefers-color-scheme) 的值（"light"/"dark"），
	// 宿主经 SetPrefersColorScheme 设置后，matchMedia 与媒体查询上下文
	// 会动态反映；空串按 light 处理。
	colorScheme string
	// hoverCapability/pointerCapability 是 (hover)/(pointer) 媒体特性能力，
	// 默认 "hover"/"fine"（desktop 鼠标场景）；宿主可经
	// SetPointerCapabilities 调整为触屏（coarse/none）等场景。
	hoverCapability   string
	pointerCapability string
	// deviceScaleFactor 是设备像素比（CDP Emulation.setDeviceMetricsOverride
	// 的 deviceScaleFactor；浏览器默认 1）。由 SetDeviceScaleFactor 设置。
	//
	// ★ 语义边界（如实记录，见 docs/TECH_DEBT.md）：引擎按 **CSS 像素**光栅化，
	// 本值不改变布局尺寸，只影响 ① window.devicePixelRatio ② CSS 媒体查询
	// （min-resolution / -webkit-device-pixel-ratio / dppx…）
	// ③ Page.captureScreenshot 的输出分辨率（宿主按 DSF 放大）。
	deviceScaleFactor float64

	// currentURL 是主文档的加载 URL（LoadURL 设置，LoadHTML 直出内容时为
	// ""）。它是「文档基地址」的唯一来源：iframe 相对路径 src
	// （如 src="page.html"）、外部资源引用（<link href="a.css"> /
	// <script src="/js/x.js">）与页面脚本的 fetch("/api") 都依赖它做基准
	// 解析（WebKit completeURL 语义）。
	//
	// 读写加锁：LoadURL 在宿主线程写，fetch/XHR 在解释器线程读。
	currentURL string
	// baseHref 是主文档 `<base href>` 的原始值缓存。文档基准（document.baseURI）
	// = ResolveURL(currentURL, baseHref)（见 documentBaseURL）。为什么缓存而不是
	// 每次读 Document.BaseURL()：fetch/XHR 在**解释器线程**解析相对 URL，而
	// 遍历 DOM 树找 `<base>` 是宿主线程的活动（跨线程读同一棵树会有数据竞争）。
	// 宿主线程在装配完成与 DOM 变更回调里刷新（refreshBaseHref）。
	baseHref     string
	currentURLMu sync.RWMutex
	// subframeJS 为每个 iframe 子 Frame 维护独立的 JS 全局环境（浏览器
	// iframe 语义：子文档有自己的 window/document，与父文档互不干扰）。
	subframeJS map[*page.Frame]*jsc.Interpreter

	// domBindingsInjected 标记「当前 document 的 DOM bindings + 渲染树几何桥
	// 已注入」。LoadHTML 在页面脚本执行前注入一次并置位；EvalJS 仅在未注入
	// 时兜底注入，避免每次 EvalJS 重复执行 RegisterDOMBindings（幂等分支
	// wrapDocument + applyCanvas2DPatch 的 RunJS）与 injectRenderTreeBridge
	// （重赋值 10+ 个包级几何桥闭包）的固定开销。
	domBindingsInjected bool

	// canvas 是渲染输出复用缓冲：Render() 尺寸未变时复用（Skia Surface/
	// Paint/fontCache 随 Canvas 生命周期，不随每次 Render 重建——高帧率下
	// 反复 NewCanvas 是分配/释放风暴）；尺寸变化时释放重建。
	canvas *graphics.Canvas

	// destroyed 标记 WebView 已销毁（Destroy 后不可再使用）。
	destroyed bool

	// interact 引擎级鼠标交互管线（惰性创建）：裸 WebView 宿主（配置
	// 窗口）喂入 HandleMouseButton/MouseMove/Wheel 获得浏览器标准交互
	// （含 select 下拉弹层）。app.Host 场景仍走 Host 自身管线。
	interact *Interaction
	// dynScriptMu/dynScripts 是「运行时动态插入的 <script src>」待执行队列：
	// appendChild 发生在 JS 执行栈内，goja 不允许在 native 调用栈内重入执行
	// 脚本，故先入队、在帧边界（Render 开头）取回并执行。
	dynScriptMu sync.Mutex
	dynScripts  []dynScript

	// formFocus 表单交互服务（惰性创建）：焦点管理/点击定位光标/文本
	// 编辑/blur-Enter 提交 onchange。裸 WebView 宿主（configwin）此前
	// 自建 NewFormFocus — 统一走引擎入口。
	formFocus *FormFocus

	// mode 是运行模式（见 mode.go）：决定装配阶段接入哪些浏览器专属
	// 能力（外部网络/子框架/并发脚本/导航/外部资源）。零值 = ModeBrowser，
	// 即历史行为。
	mode Mode
	// modeLocked 在首次装配（LoadHTML 的注入阶段）后置位：此后只允许
	// 同值 SetMode（切换返回 ErrModeLocked）。
	modeLocked bool
	// resourceResolver 是宿主资源解析器（可选）：两种模式都先经它，
	// UI 库模式下是外部资源引用的唯一通道。
	resourceResolver ResourceResolver
	// resourceCache 是本 WebView 的资源内存缓存（惰性创建，见
	// resource_cache.go）：浏览器 memory cache 语义——同一 URL 只取一次。
	resourceCacheMu sync.Mutex
	resourceCache   *resourceCache

	// imageLoadedOff 是「图片异步加载完成」全局监听器的注销函数
	// （NewWebViewWithMode 注册、Destroy 注销）。不注销会让渲染层的全局
	// 监听器集合永久持有已销毁的 WebView。
	imageLoadedOff func()
	// videoFrameOff 是「播放帧已交付」全局监听器的注销函数（A2-③，与
	// imageLoadedOff 同一条规矩：NewWebViewWithMode 注册、Destroy 注销）。
	videoFrameOff func()
	// resourceLoadedMu 保护 onResourceLoaded（回调在取字节的 goroutine 里
	// 触发，宿主可能在任意线程设置）。
	resourceLoadedMu sync.Mutex
	// onResourceLoaded 是宿主可选的「异步资源加载完成」回调（见
	// SetOnResourceLoaded）：引擎内部已自动置脏重绘，本回调供宿主做日志/
	// 统计/自定义刷新。
	onResourceLoaded func(url string)

	// navMu 保护导航状态（页面脚本在解释器线程发起导航，宿主线程读写）。
	navMu sync.Mutex
	// pendingNavKind 是本次导航的种类（location.replace/reload/history 遍历
	// 与普通导航在历史栈上行为不同），由 loadHTMLFrom 装配完成后消费一次。
	pendingNavKind bindings.NavKind
	pendingNavSet  bool
	// reloadHandler 是 location.reload() 的宿主实现（见 SetReloadHandler）：
	// 没有来源 URL 的文档（LoadHTML 直出内容）引擎无处重新取内容。
	reloadHandler func() bool
	// onNavigationBlocked 是「导航被模式门禁拒绝」的回调（见
	// SetOnNavigationBlocked）。
	onNavigationBlocked func(url string)
	// assembling 标记「正在装配文档」：装配期间页面脚本发起的导航（location
	// 赋值等）排队到装配结束再执行（见 navigateTo / finishAssembly），
	// queuedNav 保存该请求。
	assembling bool
	queuedNav  *queuedNavigation
}

// FormFocus 返回引擎表单交互服务（首次调用创建；Destroy 后返回 nil）。
// 标准交互收敛点：点击聚焦（interact.MouseButton 内自动）、键盘编辑
// （CharInput/KeyInput）、blur/Enter 提交 onchange（Submit）。
func (wv *WebView) FormFocus() *FormFocus {
	if wv.destroyed {
		return nil
	}
	if wv.formFocus == nil {
		wv.formFocus = NewFormFocus(wv)
	}
	return wv.formFocus
}

// Interaction 返回引擎交互管线（首次调用时创建；Destroy 后返回 nil）。
func (wv *WebView) Interaction() *Interaction {
	if wv.destroyed {
		return nil
	}
	if wv.interact == nil {
		wv.interact = &Interaction{wv: wv}
	}
	return wv.interact
}

// HandleMouseButton 向引擎喂入鼠标按键事件（客户区 CSS 像素）：
// button=0 左键；action=0 按下 / 1 释放。裸 WebView 宿主的标准交互入口。
func (wv *WebView) HandleMouseButton(x, y float64, button, action int) {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.MouseButton(x, y, button, action)
	}
}

// HandleMouseMove 向引擎喂入鼠标移动事件。
func (wv *WebView) HandleMouseMove(x, y float64) {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.MouseMove(x, y)
	}
}

// HandleMouseMoveBatched 把鼠标移动并入当前事件批（不立即派发）。
//
// 配合 FlushMouseMoves 使用：宿主在事件批开始时改用本方法喂入移动、批末调用
// FlushMouseMoves，一批内多条移动即合并为一次 hover 追踪 + 一次 mousemove
// 派发（坐标取批内最后一次）。鼠标移动是事件批里最密集的事件，宿主每帧
// PollEvents 可能拿到多条；逐条派发会让页面在两次渲染之间收到大量坐标，
// 且每次都要命中测试 + 构造事件对象（T4 事件派发聚合）。
//
// 未调用 FlushMouseMoves 时移动不会派发，因此宿主必须成对使用；单条移动
// 路径（HandleMouseMove）行为不变。
func (wv *WebView) HandleMouseMoveBatched(x, y float64) {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.MouseMoveBatched(x, y)
	}
}

// FlushMouseMoves 派发当前批内累积的鼠标移动（无累积时零开销）。
func (wv *WebView) FlushMouseMoves() {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.FlushMoves()
	}
}

// CloseRequest 处理 close request（HTML 的 close watcher / 「关闭请求」语义）：
// 关闭最上层的 auto/hint popover（HTML §6.12：manual popover 不响应）。
//
// 宿主在用户按下 Esc、且页面没有 preventDefault 掉 keydown 时调用它；返回
// 是否消费了该请求（false = 当前没有可关闭的 popover，宿主可继续自己的 Esc
// 逻辑，例如退出全屏或关闭自己的窗口）。
//
// 已知边界：<dialog> 的 Esc 关闭（dialog 的 close watcher）未实现，本端口只
// 有 popover 参与 close request。
func (wv *WebView) CloseRequest() bool {
	if wv == nil || wv.destroyed {
		return false
	}
	doc := wv.Document()
	if doc == nil {
		return false
	}
	if !popover.CloseRequest(doc) {
		return false
	}
	// 关闭改变了 :popover-open 与 UA 的 display 规则 → 同步渲染树与布局，
	// 否则宿主在同一帧内还会画出旧内容（与点击路径的同步策略一致）。
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	return true
}

// HandleMouseLeave 鼠标离开窗口（清除 :hover 残留）。
func (wv *WebView) HandleMouseLeave() {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.MouseLeave()
	}
}

// HandleWheel 向引擎喂入滚轮事件（deltaY：Win32 滚轮增量，正=向上）。
// 滚动目标为最近光标位置（Interaction 内部维护）。
func (wv *WebView) HandleWheel(deltaY float64) {
	if it := wv.Interaction(); it != nil && !wv.destroyed {
		it.Wheel(deltaY)
	}
}

// ensureFonts initializes the global FontManager (if not already done) and
// wires layout text measurement to real glyph metrics. Text rendering requires
// a non-nil FontManager — without it inline text silently disappears (the
// render_test / companion paths never called InitFontManager). Desktop hosts
// (app.NewHost) may have already initialized with a bundled font directory;
// GetFontManager() guards against re-initialization (InitFontManager is a
// sync.Once).
func ensureFonts() {
	if graphics.GetFontManager() == nil {
		// 空目录：selectDefaults 通过 skia.NewTypeface 按 OS 字体名查找
		// （Microsoft YaHei / Consolas / SimSun），无需预加载字体文件。
		_ = graphics.InitFontManager("")
	}
	// ★ 自动加载系统字体（C:\Windows\Fonts 等）：symbol（Segoe UI Symbol）
	//   / emoji / CJK 等 fallback 字体依赖字体文件预加载后才能解析；
	//   LoadSystemFonts 有幂等保护，重复调用无副作用。这是 wb-ui 自动
	//   完成的初始化，调用方（desktop 主程序、render_test、probe）无需
	//   再手动 LoadSystemFonts——否则折叠三角 ▸ 等几何符号会因 symbolTF
	//   为 nil 渲染成 .notdef 方块。
	if mgr := graphics.GetFontManager(); mgr != nil {
		mgr.LoadSystemFonts()
	}
	if layout.MeasureTextFunc == nil {
		layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
			return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
		}
	}
	if layout.FontMetricsFunc == nil {
		layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
			f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
			return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
		}
	}
}

// NewWebView 创建一个 WebView，模式为 ModeBrowser（嵌入浏览器，历史默认）。
func NewWebView() *WebView {
	return NewWebViewWithMode(ModeBrowser)
}

// NewWebViewWithMode 按指定模式创建 WebView（模式语义见 mode.go / docs/MODES.md）：
//   - ModeBrowser：嵌入浏览器，完整浏览器语义（外部资源/子框架/网络/导航）
//   - ModeToolkit：UI 库，保留渲染/布局/DOM/CSS/事件/宿主桥，裁剪浏览器专属
//     的外部输入（无真实网络、无 XHR/Worker/WebSocket、不装配 iframe 子文档、
//     不允许 LoadURL；外部样式/脚本只经 SetResourceResolver 或 data: URL）
//
// 模式在首次 LoadHTML 装配时锁定（此后 SetMode 只接受同值）。
func NewWebViewWithMode(mode Mode) *WebView {
	ensureFonts()
	settings := page.NewSettings()
	p := page.NewPage(settings)
	wv := &WebView{
		page: p, settings: settings,
		width: DefaultWebViewWidth, height: DefaultWebViewHeight,
		mode: mode,
	}
	registerWebView(wv)
	wv.mainFrame = NewWebFrame(wv, p.MainFrame())
	if mf := p.MainFrame(); mf != nil {
		// ★ 模式接线（外部资源统一入口）：<link rel=stylesheet> 与
		//   <script src> 都走 wv.loadExternalResource —— 宿主
		//   ResourceResolver（两种模式一致）→ data: URL → 仅 Browser
		//   模式下才允许 http(s)/file（UI 库模式返回
		//   ErrExternalResourceBlocked，不做隐式外部访问）。
		mf.StyleSheetLoader = func(href string) (string, error) {
			return wv.loadExternalResource(href, PurposeStylesheet)
		}
		mf.ScriptLoader = func(src string) (string, error) {
			return wv.loadExternalResource(src, PurposeScript)
		}
		// ★ 图片资源接线（<img src>/background-image/mask-image/SVG <image>）：
		//   同一策略链——宿主 ResourceResolver 优先、相对引用按文档 URL
		//   解析、UI 库模式拒绝 http(s)/file（渲染层不再自己 httpGet）。
		mf.ImageLoader = &webViewImageLoader{wv: wv}
	}
	// ★ 异步图片加载完成 → 自动置脏重绘（浏览器语义：资源到位即
	//   invalidate）。按需渲染的宿主（app.Host 的空闲帧跳过 Paint）若不等
	//   这个通知，后台 goroutine 取回的图片**永远不会画出来**——此前
	//   rendering.SetBackgroundImageLoadedCallback 只有测试在用，主链路
	//   没有人接线。
	wv.imageLoadedOff = rendering.AddBackgroundImageLoadedListener(wv.onAsyncImageLoaded)
	// ★ 播放帧交付完成 → 自动置脏重绘（A2-③）：异步抽帧的字节在 worker
	// goroutine 上回来，按需渲染的宿主（app.Host 空闲帧跳过 Paint）不会自己
	// 发现帧缓存里多了一帧——没有这条通知，播放画面要等**下一次别的重绘理由**
	// 才更新，看起来就是卡的。
	wv.videoFrameOff = rendering.AddVideoFrameReadyListener(wv.onAsyncVideoFrame)
	// ★ iframe 子文档：渲染侧（paint/hit-test）经 IFrameLookup 取回
	// iframe 元素的子 Frame 渲染视图（避免 rendering→page 包循环依赖）。
	rendering.IFrameLookup = func(el *dom.Element) rendering.IFrameSubdocument {
		f := page.IFrameFrame(el)
		if f == nil {
			return nil
		}
		return f
	}
	// ★ iframe src 变化（JS 侧 el.src = x / setAttribute）→ 重载子文档。
	installBridgeDispatch()
	// ★ 页面脚本发起的导航（location.assign/replace/reload、history 遍历）→
	//   本 WebView 的 LoadURL（见 navigation.go）。
	installNavigationDispatch()
	wvBridgeOf(wv).iframeSrcChanged = wv.handleIFrameSrcChanged
	// ★ iframe 滚动容器查找：给定子文档 Document 反查承载它的子 Frame。
	rendering.IFrameContaining = func(doc *dom.Document) rendering.IFrameSubdocument {
		if doc == nil {
			return nil
		}
		var found *page.Frame
		page.ForEachIFrame(func(_ *dom.Element, f *page.Frame) {
			if found == nil && f != nil && f.Document() == doc {
				found = f
			}
		})
		if found == nil {
			return nil
		}
		return found
	}
	return wv
}

// handleIFrameSrcChanged 处理 JS 侧修改 iframe src：解析绝对 URL 后
// 卸载旧子 Frame 并重新加载（浏览器 iframe navigation 语义）。
func (wv *WebView) handleIFrameSrcChanged(el *dom.Element, src string) {
	if el == nil {
		return
	}
	abs := resolveIframeSrc(src, wv.documentURL())
	// 旧子 Frame 卸载：注销注册表 + 清理其 JS 环境。
	if old := page.IFrameFrame(el); old != nil {
		page.UnregisterIFrame(el)
		delete(wv.subframeJS, old)
	}
	// ★ 模式：UI 库模式不装配 iframe 子文档（卸载旧子文档后即返回，
	//   <iframe> 元素仍参与布局/绘制，只是没有子文档内容）。
	if !wv.mode.allowsSubframes() {
		page.Logf("IFrame", "src change: subframes disabled in %s mode", wv.mode)
		return
	}
	if abs == "" {
		page.Logf("IFrame", "src change: unresolvable %q, unloaded", src)
		return
	}
	wv.loadSubframe(el, abs)
}

func (wv *WebView) Page() *page.Page         { return wv.page }
func (wv *WebView) MainFrame() *WebFrame     { return wv.mainFrame }
func (wv *WebView) Settings() *page.Settings { return wv.settings }
func (wv *WebView) Width() int               { return wv.width }
func (wv *WebView) Height() int              { return wv.height }

// SetPrefersColorScheme 设置 (prefers-color-scheme) 的值（"light"/"dark"），
// 供 matchMedia 与 CSS 媒体查询使用。
func (wv *WebView) SetPrefersColorScheme(scheme string) {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "dark", "light":
		wv.colorScheme = strings.ToLower(strings.TrimSpace(scheme))
	default:
		wv.colorScheme = "light"
	}
}

// PrefersColorScheme 返回当前颜色方案（默认 "light"）。
func (wv *WebView) PrefersColorScheme() string {
	if wv.colorScheme == "" {
		return "light"
	}
	return wv.colorScheme
}

// SetPointerCapabilities 设置媒体查询 (hover)/(pointer) 能力值。
// hover 接受 "hover"/"none"，pointer 接受 "fine"/"coarse"/"none"，
// 非法值回退默认（"hover"/"fine"）。
func (wv *WebView) SetPointerCapabilities(hover, pointer string) {
	switch strings.ToLower(strings.TrimSpace(hover)) {
	case "hover", "none":
		wv.hoverCapability = strings.ToLower(strings.TrimSpace(hover))
	default:
		wv.hoverCapability = "hover"
	}
	switch strings.ToLower(strings.TrimSpace(pointer)) {
	case "fine", "coarse", "none":
		wv.pointerCapability = strings.ToLower(strings.TrimSpace(pointer))
	default:
		wv.pointerCapability = "fine"
	}
}

// HoverCapability 返回 (hover) 能力值（默认 "hover"）。
func (wv *WebView) HoverCapability() string {
	if wv.hoverCapability == "" {
		return "hover"
	}
	return wv.hoverCapability
}

// PointerCapability 返回 (pointer) 能力值（默认 "fine"）。
func (wv *WebView) PointerCapability() string {
	if wv.pointerCapability == "" {
		return "fine"
	}
	return wv.pointerCapability
}

// SetDeviceScaleFactor 设置设备像素比（CDP Emulation.setDeviceMetricsOverride 的
// deviceScaleFactor；浏览器默认 1）。非正数按 1 处理。
//
// ★ 语义边界（如实记录，见 docs/TECH_DEBT.md）：引擎按 **CSS 像素**光栅化，
// 本值不改变布局尺寸，只影响 ① window.devicePixelRatio ② CSS 媒体查询
// （min-resolution / -webkit-device-pixel-ratio / dppx…）
// ③ Page.captureScreenshot 的输出分辨率（宿主按 DSF 放大输出）。
func (wv *WebView) SetDeviceScaleFactor(dsf float64) {
	if dsf <= 0 {
		dsf = 1
	}
	if wv.DeviceScaleFactor() == dsf {
		return
	}
	wv.deviceScaleFactor = dsf
	bindings.DevicePixelRatio = dsf
	// 设备像素比是媒体特性的输入 → 全文档样式都要重新匹配。两处缓存都要失效：
	//  ① bindings 的 computed style 缓存（getComputedStyle 路径，全量——媒体查询
	//     影响所有元素，不能只清子树）；
	//  ② 样式 resolver 的元素级级联缓存 + 渲染树重建（与 class 变更同一条路径，
	//     见 onClassChanged）。
	// 缺 ① 会让页面读到旧样式（实测：DSF=2 后 matchMedia 已为 true，但
	// getComputedStyle 仍返回基础规则的 color）。
	bindings.BumpStyleVersion()
	fr := wv.mainFrame.Frame()
	if fr == nil {
		return
	}
	// ★ 媒体查询求值的输入也包含 DSF：resolver 的上下文必须一起刷新，否则
	// @media (min-resolution: 2dppx) 这类规则永不匹配（实测 computed color
	// 停在基础规则，与 matchMedia 已 true 自相矛盾）。
	wv.syncMediaQueryContext()
	if rsv := fr.Resolver(); rsv != nil {
		if doc := wv.mainFrame.Document(); doc != nil {
			if root := doc.DocumentElement(); root != nil {
				rsv.InvalidateSubtree(root)
			}
		}
	}
	fr.MarkRenderTreeDirty()
	fr.SetNeedsLayout(true)
}

// DeviceScaleFactor 返回当前设备像素比（默认 1）。
func (wv *WebView) DeviceScaleFactor() float64 {
	if wv.deviceScaleFactor <= 0 {
		return 1
	}
	return wv.deviceScaleFactor
}

// mediaQueryContext 返回本 WebView 当前的媒体查询求值上下文。
//
// 单一来源：matchMedia（bindings.MediaQueryContextForInterpreter → bridge）、
// js 层 @media 规则求值与样式解析（resolver）都取这一份，同一页面就不会在不同
// 路径下给出互相矛盾的结论（曾经 matchMedia 为 true 而 computed style 仍停在
// 基础规则）。
func (wv *WebView) mediaQueryContext() *css.MediaQueryContext {
	w, h := wv.width, wv.height
	if w <= 0 {
		w = DefaultWebViewWidth
	}
	if h <= 0 {
		h = DefaultWebViewHeight
	}
	orientation := "landscape"
	if h > w {
		orientation = "portrait"
	}
	return &css.MediaQueryContext{
		Width: w, Height: h, DeviceWidth: w, DeviceHeight: h,
		// ★ B3：设备像素比来自 Emulation（默认 1）——(min-resolution) /
		//   (-webkit-device-pixel-ratio) / dppx 据此匹配。
		DevicePixelRatio: wv.DeviceScaleFactor(), Orientation: orientation,
		PrefersColorScheme: wv.PrefersColorScheme(),
		Hover:              wv.HoverCapability(), AnyHover: wv.HoverCapability(),
		Pointer:            wv.PointerCapability(), AnyPointer: wv.PointerCapability(),
	}
}

// syncMediaQueryContext 把当前设备像素比同步给样式 resolver：@media 的
// resolution 系列（min-resolution / dppx / dpi / dpcm）与 matchMedia 因此用同一
// 份输入。只改 dpr，不动 resolver 的视口尺寸（frame.SetViewportSize 才是权威）。
func (wv *WebView) syncMediaQueryContext() {
	if wv.mainFrame == nil {
		return
	}
	fr := wv.mainFrame.Frame()
	if fr == nil {
		return
	}
	if rsv := fr.Resolver(); rsv != nil {
		rsv.SetDevicePixelRatio(wv.DeviceScaleFactor())
	}
}

// BeforePageScripts is an optional hook invoked after DOM bindings are
// registered but before any page <script> executes. The `window` global
// object exists at this point (it is created by RegisterDOMBindings), so
// embedders can inject page-environment JS (e.g. fetch interception for
// desktop mode) that must be visible to the application code.
var BeforePageScripts func(rt *jsc.Interpreter)

// LoadHTML 用给定内容装配主文档。内容没有来源 URL：文档基地址为空，
// 相对引用没有基准可解析（<link href="x.css"> 仍按既有规则走「相对当前
// 目录的文件读取」）。需要真实网页的相对 URL 语义时用 LoadURL。
func (wv *WebView) LoadHTML(src string) error {
	return wv.loadHTMLFrom(src, "")
}

// LoadHTMLWithBaseURL 同 LoadHTML，但给文档一个**来源 URL**：它成为
// document.URL / location.href，并作为相对引用的解析基准（document.baseURI）。
//
// 用途：宿主自己提供内容（内嵌页面、UI 库模式下的模板），但页面里写的是相对
// 引用，需要一个基准才解析得出来（LoadHTML 的 about:blank 没有基准）。浏览器
// 里等价于文档里写 `<base href>`——两者可以同时用：`<base href>` 的值会以这个
// URL 为基准再解析一次（document.baseURI 语义）。
//
// 与 LoadURL 的区别：**不取内容、不联网**，只给文档一个基准，因此 UI 库模式
// 也允许（外部资源仍然只经宿主 ResourceResolver 或 data: URL）。
func (wv *WebView) LoadHTMLWithBaseURL(src, baseURL string) error {
	return wv.loadHTMLFrom(src, baseURL)
}

// loadHTMLFrom 是 LoadHTML 的实现体。docURL 是文档的来源 URL（LoadURL
// 传真实 URL，LoadHTML 传 ""），它决定 location.href / document.URL 以及
// 所有相对引用（<link>/<script src>/fetch/XHR/iframe src）的解析基准。
func (wv *WebView) loadHTMLFrom(src, docURL string) error {
	if wv.destroyed || wv.mainFrame == nil {
		return ErrDestroyed
	}
	// ★ 装配期间标记 + 结束处理：页面脚本里发起的导航（location 赋值、
	// history 遍历）排队到装配结束后执行——浏览器里导航是异步的，不会在
	// 当前脚本执行中途替换文档/渲染树。
	wv.navMu.Lock()
	wv.assembling = true
	wv.navMu.Unlock()
	defer wv.finishAssembly()
	wv.setDocumentURL(docURL)
	// 模式在此刻生效并锁定（注入阶段按模式接线，见 mode.go）。
	wv.modeLocked = true
	if wv.mainFrame != nil {
		fn := func(code string) error {
			_, err := wv.EvalJS(code)
			return err
		}
		wv.mainFrame.ScriptEngine = fn
		if fr := wv.mainFrame.Frame(); fr != nil {
			fr.ScriptEngine = fn
		}
	}
	// Ensure JS runtime is initialized and inject bridge + fetch before
	// page scripts execute. This makes Go-registered API routes available
	// as fetch() intercepts in GUI mode.
	wv.ensureJSRuntime()
	// ★ 模式接线（1/4）：fetch 在两种模式都注册（UI 库也要用宿主桥路由
	//   取数据），但 UI 库模式关闭「无匹配路由 → 真实网络请求」的回退；
	//   XMLHttpRequest 是浏览器专属 API，UI 库模式不注册。
	page.RegisterFetchWithPolicy(wv.jsInterpreter, wv.mode.allowsNetwork())
	if wv.mode.allowsNetwork() {
		page.RegisterXMLHttpRequest(wv.jsInterpreter)
	}
	bridge.InjectAll(wv.jsInterpreter)

	// Inject the bridge SDK script as inline JS before any page scripts.
	// This SDK wraps fetch() to intercept registered API routes.
	if sdk := bridge.InjectSDK(); sdk != "" {
		if _, err := wv.jsInterpreter.RunJS(sdk); err != nil {
			fmt.Fprintf(os.Stderr, "[wb-ui] bridge SDK injection failed: %v\n", err)
		}
	}

	// ★ 文档 URL 必须在**文档装配之前**就位（见 page.Frame.SetPendingDocumentURL）：
	//   `<link href>`/`<script src>` 的加载发生在装配过程中，需要文档基准才能
	//   解析相对引用。事后再 doc.SetURL 已经太晚——样式与脚本已按空基准请求
	//   过（相对引用落到宿主进程工作目录）。
	//   注：host（LoadURL）传的是**重定向后的最终 URL**，见本函数注释。
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.SetPendingDocumentURL(docURL)
	}
	if err := wv.mainFrame.LoadHTML(src); err != nil {
		return err
	}
	// 文档 URL 写进 Document（location.href / document.URL 读它），并把
	// 「当前文档基地址」登记给 page（fetch / XHR 解析相对 URL 用）。登记
	// 的是提供器而不是快照：LoadURL 再次导航后立刻按新文档 URL 解析。
	if doc := wv.mainFrame.Document(); doc != nil {
		doc.SetURL(docURL)
		// 文档基准（document.baseURI）：`<base href>` 在解析时就已经在文档里，
		// 这里刷新一次缓存（fetch/XHR 的解释器线程只读缓存）。
		wv.refreshBaseHref()
		if wv.jsInterpreter != nil {
			// ★ 相对引用基准 = document.baseURI（含 `<base href>`）：loadExternalResource
			//   （link/script/img/@import）与 fetch/XHR 共用同一个基准定义。
			page.SetDocumentBaseProvider(wv.jsInterpreter, func() string { return wv.documentBaseURL() })
		}
	}
	// DOM bindings MUST be registered BEFORE executing page scripts so that
	// JS frameworks (Vue/React) have access to document.getElementById,
	// querySelector, Element.appendChild, etc. at boot time.
	if wv.jsInterpreter != nil && wv.mainFrame.Document() != nil {
		// matchMedia 需要真实视口上下文（尺寸随 wv 变化、颜色方案随
		// SetPrefersColorScheme 设置；指针能力暂用默认值）。
		installBridgeDispatch()
		// 上下文定义只有 WebView.mediaQueryContext 一份：matchMedia（经 bridge）
		// 与样式解析（经 resolver）必须用同一份，避免两处漂移。
		wvBridgeOf(wv).mediaQueryCtx = wv.mediaQueryContext
		// ★ 同步给样式 resolver：@media (min-resolution: …) 的求值走 resolver 的
		// 媒体查询上下文，不同步则 DSF 变化后 matchMedia 已 true 而 computed
		// style 仍停在基础规则（实测自相矛盾）。
		wv.syncMediaQueryContext()
		bindings.RegisterDOMBindings(wv.jsInterpreter, wv.mainFrame.Document())
		// ★ 模式接线（2/4）：Worker/WebSocket 是浏览器并发/长连接能力，
		//   UI 库模式下从全局隐藏（typeof Worker === "undefined" /
		//   typeof WebSocket === "undefined"），库的 feature detect 才能
		//   得到正确结论（引擎不内置真实 WebSocket 传输，Worker 是真实
		//   线程——UI 库模式下宿主不需要页面自己起线程）。
		if wv.mode.hidesThreadGlobals() {
			bindings.HideBrowserThreadGlobals(wv.jsInterpreter)
			// XMLHttpRequest 在 UI 库模式的注入阶段本就不注册（见上），
			// 这里防御性隐藏：同一解释器若曾按浏览器语义装配过，残留的
			// XHR 也被摘掉。
			bindings.HideGlobal(wv.jsInterpreter, "XMLHttpRequest")
		}
		// ★ 渲染树几何桥：Element.scrollTop/scrollHeight/clientHeight/offsetHeight/
		//   getBoundingClientRect 等 CSSOM 属性需要真实布局几何。此前只在 EvalJS
		//   中注入——cmd/desktop 与页面脚本（Vue）均走 JSInterpreter().RunJS 执行，
		//   从不经过 EvalJS → hook 保持 nil → 前端读到 0：聊天列表无法按空间
		//   加载（clientHeight/scrollHeight 恒 0）、scrollTop 赋值静默失效。
		//   在 LoadHTML 注册 DOM bindings 后、页面脚本执行前注入（渲染树可用）。
		wv.injectRenderTreeBridge()
		// Set up callback for dynamic <style> injection (Vue scoped CSS).
		// Uses dirty-flag batching: the rebuild is deferred to the next layout.
		wvBridgeOf(wv).onStyleNodeAdded = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
		}
		// ★ Go 侧 DOM API 变更（SetAttribute/SetTextContent/appendChild 等
		// dom 包调用，或 JS bindings 代理触发的节点插入/移除之外的结构
		// 变化）感知：doc.SetTreeChangeCallback 覆盖 Go 侧直改 DOM 的场景
		// （configwin 的 setInputValue、openTextEditor 的 JS 之外）、Host
		// 的 ensureTreeChangeHook 会覆盖本回调（单回调语义）——Host 路径
		// 使用其更细化的增量文本处理；裸 WebView（配置窗口/挂件）使用
		// 本默认路径：标记重建 + 布局，渲染循环按脏标记批量重建，调用方
		// 无需每次渲染前强制 RebuildRenderTree。
		if doc := wv.mainFrame.Document(); doc != nil {
			doc.SetTreeChangeCallback(func(node dom.Node) {
				// `<base>` 的插入/移除会改变文档基准（浏览器里立即生效）——
				// 结构变更回调里重新缓存一次。
				wv.refreshBaseHref()
				fr := wv.mainFrame.Frame()
				if fr == nil {
					return
				}
				// 文本变更走增量路径（同步 RenderText + InlineTextBox.text +
				// 标记所在 block dirty，下帧局部重排）；结构变更仍全量重建。
				if _, isText := node.(*dom.Text); isText && fr.ApplyTextChange(node) {
					return
				}
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			})
		}
		// DOM bindings + 几何桥已注入，EvalJS 无需重复（见 EvalJS 兜底分支）。
		wv.domBindingsInjected = true
		// ★ 文档级导航 → window.history 条目（必须在 RegisterDOMBindings 之后：
		//   历史栈是 DOM bindings 装配时创建的）。浏览器语义：文档导航追加条目、
		//   history.length 随之增长；location.replace/reload 覆盖当前条目；
		//   history 遍历只移动指针。此前宿主的 LoadHTML/LoadURL 完全不进历史栈
		//   → history.length 恒为 1、back() 永远无操作。
		bindings.NoteDocumentNavigation(wv.jsInterpreter, docURL, wv.takeNavKind())
		// Set up callback for inline style changes (el.style.xxx = ...).
		// ★ 增量优先：纯样式变更（拖拽 sidebar 宽度、range 拖动等）只更新
		//   目标元素的 ComputedStyle + SetNeedsLayout（relayout 不重建树）。
		//   此前无条件 MarkRenderTreeDirty → 每帧 RebuildRenderTree 全量
		//   重建（复杂页面 30ms+）→ 拖拽卡顿/窗口无响应（「频繁无响应」
		//   根因）。结构属性（display/position/float/clear）变化才回退全量。
		wvBridgeOf(wv).onInlineStyleChanged = func(n dom.Node) {
			fr := wv.mainFrame.Frame()
			if fr == nil {
				return
			}
			if el, ok := n.(*dom.Element); ok {
				if fr.RebuildStyleForElement(el) {
					return
				}
			}
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
		// ★ 类变化（el.className / classList.add 等）回调：祖先类影响后代
		// 选择器匹配（cm-focused 加在 cm-editor 上决定 .cm-cursor 的
		// display:block）——必须清 resolver 样式缓存（否则后代 ResolveElement
		// 命中旧缓存 display:none → 渲染树跳过光标）+ 全量重建渲染树。
		wvBridgeOf(wv).onClassChanged = func(el *dom.Element) {
			fr := wv.mainFrame.Frame()
			if fr == nil {
				return
			}
			// 失效范围取**父级子树**（2026-09 实测修正）：兄弟/后继组合器让
			// 邻近元素的匹配结果随 el 的 class/状态一起变——`.tab.active + .panel`、
			// `input:checked + .track::after`（开关滑块）。只清 el 子树时兄弟沿用
			// 旧 resolver 缓存，画面停在旧样式（实测：#c:checked + #s 在勾选后
			// #s 恒为旧色，重建渲染树也不恢复）。
			scope := el
			if p := el.ParentElement(); p != nil {
				scope = p
			}
			if rsv := fr.Resolver(); rsv != nil {
				rsv.InvalidateSubtree(scope)
			}
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
		// Set up callbacks for DOM mutations (appendChild / removeChild / etc.).
		// Uses dirty-flag batching: the rebuild is deferred to the next layout.
		wvBridgeOf(wv).onNodeInserted = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
		}
		wvBridgeOf(wv).onNodeRemoved = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
		}
	}
	// Execute page scripts AFTER DOM bindings are registered.
	// Scripts (Vue/React) may mutate the DOM — rebuild the render tree
	// so that newly created elements are included in layout/paint.
	//
	// ★ BeforePageScripts hook: window exists, page scripts not yet run —
	// desktop embedders inject fetch interception / desktopBridge here.
	if BeforePageScripts != nil && wv.jsInterpreter != nil {
		BeforePageScripts(wv.jsInterpreter)
	}
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.ExecuteScripts()
		fr.RebuildRenderTree()
	}
	// ★ DOMContentLoaded（document）：DOM 解析完、同步脚本执行完、渲染树已建。
	//   规范此刻 document.readyState 仍为 "interactive"（load 之前），因此必须在
	//   SetReadyState("complete") 之前派发。此前引擎从不派发该事件 → 所有
	//   document.addEventListener('DOMContentLoaded', fn) 的页面脚本永不执行。
	if fr := wv.mainFrame.Frame(); fr != nil && wv.jsInterpreter != nil {
		bindings.FireDocumentEvent(wv.jsInterpreter, fr.Document(), "DOMContentLoaded")
	}
	// iframe 子文档：主文档加载完成后，为带 src 的 <iframe> 创建子 Frame
	// 并加载（WebKit: FrameLoader 在解析到 iframe 元素时创建子 Frame）。
	// 子 Frame 拥有独立 ScriptEngine（独立 JS 全局环境），子文档脚本
	// 可执行；布局与绘制见 syncIFrameSizes / PaintIFrame。
	// ★ 模式接线（3/4）：iframe 子文档是浏览器导航能力，UI 库模式不装配
	//   （<iframe> 元素本身仍参与布局/绘制，只是没有子文档）。
	if wv.mode.allowsSubframes() {
		wv.loadIFrameDocuments()
	}
	// 加载流程收尾（HTML §3.1.4）：DOM 已解析、页面脚本已执行、渲染树已重建，
	// 资源也已同步就位 ⇒ 推进 readyState 到 "complete"，随后派发 load（window）。
	//
	// ★ 此前只推进 readyState 而**从不派发 load / DOMContentLoaded**：挂在两个
	//   事件上的页面脚本永不执行且不报错，页面静默停在初始状态。实测
	//   dev/fixtures/css-stack/sticky.html 的 `window.addEventListener('load',
	//   …)` 里 `sc.scrollTop = 60` 从未发生（探针 loadFired=false、scrollTop=0，
	//   而 Edge 为 60）——与 Edge 对照的整段滚动容器差异即由此而来。
	//   详见 bindings/lifecycle.go 的说明。
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.SetReadyState("complete")
		if wv.jsInterpreter != nil {
			bindings.FireWindowEvent(wv.jsInterpreter, "load")
		}
	}
	return nil
}

// loadIFrameDocuments 扫描主文档中的 <iframe> 元素，为每个有 src 且尚未
// 注册子 Frame 的元素创建子 Frame 并加载子文档（fetchURL 支持 http/https/
// file/data）。相对路径 src（如 src="page.html"）以 currentURL（主文档 URL）
// 为基准解析（WebKit completeURL 语义）。子 Frame 挂独立 ScriptEngine，
// 子文档的 <script> 在加载后执行。
func (wv *WebView) loadIFrameDocuments() {
	doc := wv.mainFrame.Document()
	if doc == nil {
		return
	}
	page.PruneIFrames(doc)
	wv.pruneSubframeJS()
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "iframe" {
			src := el.GetAttribute("src")
			if src != "" && page.IFrameFrame(el) == nil {
				// 基准是主文档的 document.baseURI（含 `<base href>`），不是裸的
				// 文档 URL——`<base href="/app/">` 下 `src="page.html"` 指向
				// /app/page.html，与浏览器一致。
				base := wv.documentBaseURL()
				abs := resolveIframeSrc(src, base)
				if abs == "" {
					page.Logf("IFrame", "skip unresolvable src=%q base=%q", src, base)
				} else {
					wv.loadSubframe(el, abs)
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc)
}

// loadSubframe 为 iframe 元素加载子文档：创建子 Frame、挂独立脚本引擎
// （ScriptEngine/ScriptLoader/StyleSheetLoader）、加载并执行子文档脚本。
func (wv *WebView) loadSubframe(el *dom.Element, absSrc string) {
	data, err := fetchURL(absSrc)
	if err != nil {
		page.Logf("IFrame", "fetch %q: %v", absSrc, err)
		return
	}
	f := page.NewFrame(wv.page)
	// 默认 300x150（iframe 替换元素默认尺寸），EnsureLayout 后由
	// syncIFrameSizes 校正。
	f.View().SetSize(300, 150)
	// subframeBase 返回子文档的解析基准：优先子文档自己的 document.baseURI
	// （子文档 URL + 子文档里的 `<base href>`），文档还没建立时回退 iframe src。
	// 子文档的 `<link>`/`<script src>`/`<img>` 都按它解析（浏览器语义：iframe
	// 内文档是一个独立文档，基准是它自己的 baseURI）。
	subframeBase := func() string {
		if sdoc := f.Document(); sdoc != nil {
			if b := sdoc.BaseURL(); b != "" {
				return b
			}
		}
		return absSrc
	}
	// 子文档的 <script> 经独立 JS 全局环境执行（见 makeSubframeScriptEngine）。
	f.ScriptEngine = wv.makeSubframeScriptEngine(f)
	f.ScriptLoader = func(src string) (string, error) {
		abs := resolveIframeSrc(src, subframeBase())
		if abs == "" {
			abs = src
		}
		// 子框架资源与主文档同策略（含宿主 ResourceResolver）。
		return wv.loadExternalResource(abs, PurposeScript)
	}
	f.StyleSheetLoader = func(href string) (string, error) {
		abs := resolveIframeSrc(href, subframeBase())
		if abs == "" {
			abs = href
		}
		return wv.loadExternalResource(abs, PurposeStylesheet)
	}
	// 子文档图片的相对引用以**子文档自己的基准**（子文档 URL + 子文档里的
	// `<base href>`）解析，不是主文档：iframe 内 `<img src="logo.png">` 应取
	// 子文档同级。frame 让 loader 能读到子文档的 document.baseURI。
	f.ImageLoader = &webViewImageLoader{wv: wv, docURL: absSrc, frame: f}
	// 子文档 URL = iframe src 的绝对地址（浏览器里 iframe 内文档的
	// document.URL/location.href 就是它），必须在装配前就位——子文档的
	// `<link>`/`<script src>`/`<img>` 都要按它解析相对引用。
	f.SetPendingDocumentURL(absSrc)
	if ferr := f.LoadHTML(data); ferr != nil {
		page.Logf("IFrame", "LoadHTML %q: %v", absSrc, ferr)
		return
	}
	f.RebuildRenderTree()
	// 子文档脚本：先注册子 Frame 的 DOM bindings，再执行 <script>。
	if rt := wv.subframeInterpreter(f); rt != nil && f.Document() != nil {
		bindings.RegisterDOMBindings(rt, f.Document())
	}
	f.ExecuteScripts()
	f.RebuildRenderTree() // 脚本可能改了 DOM，重建渲染树
	page.RegisterIFrame(el, f)
}

// makeSubframeScriptEngine 返回子 Frame 的脚本执行回调：子文档脚本在
// 该子 Frame 独立的 jsc.Interpreter 里执行（浏览器 iframe 语义：子文档
// 有独立 window/document/全局对象，脚本间互不干扰）。
func (wv *WebView) makeSubframeScriptEngine(f *page.Frame) func(code string) error {
	return func(code string) error {
		rt := wv.subframeInterpreter(f)
		if rt == nil {
			return nil
		}
		if doc := f.Document(); doc != nil {
			bindings.RegisterDOMBindings(rt, doc)
		}
		_, err := rt.RunJS(code)
		return err
	}
}

// subframeInterpreter 返回子 Frame 的 JS 解释器（惰性创建，与主文档的
// interpreter 完全独立）。
func (wv *WebView) subframeInterpreter(f *page.Frame) *jsc.Interpreter {
	if rt := wv.subframeJS[f]; rt != nil {
		return rt
	}
	rt := jsc.NewInterpreter()
	rt.SetupGlobal(&jsc.BufferLogger{})
	_ = jsc.NewEventLoop(rt)
	rt.InjectBrowserEnv()
	rt.RegisterWebAPIs()
	if wv.subframeJS == nil {
		wv.subframeJS = map[*page.Frame]*jsc.Interpreter{}
	}
	wv.subframeJS[f] = rt
	return rt
}

// pruneSubframeJS 删除不再注册的子 Frame 的 JS 环境（loadIFrameDocuments
// 里 PruneIFrames 之后调用，避免子 Frame 对象残留）。
func (wv *WebView) pruneSubframeJS() {
	for f := range wv.subframeJS {
		if page.IFrameFrameForFrame(f) == nil {
			delete(wv.subframeJS, f)
		}
	}
}

// resolveIframeSrc 把 iframe 的 src 解析为绝对 URL：绝对 scheme（http/
// https/data/file）原样返回；相对路径以 baseURL 为基准（net/url
// ResolveReference）；无基准时返回 ""（调用方跳过）。
func resolveIframeSrc(src, baseURL string) string {
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "file://") {
		return src
	}
	if baseURL == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// documentURL 返回主文档的当前 URL（无来源 URL 时 ""）。并发安全：宿主
// 线程（LoadURL/LoadHTML）写、解释器线程（fetch/XHR/资源加载）读。
func (wv *WebView) documentURL() string {
	if wv == nil {
		return ""
	}
	wv.currentURLMu.RLock()
	defer wv.currentURLMu.RUnlock()
	return wv.currentURL
}

func (wv *WebView) setDocumentURL(u string) {
	if wv == nil {
		return
	}
	wv.currentURLMu.Lock()
	wv.currentURL = u
	wv.currentURLMu.Unlock()
}

// documentBaseURL 返回主文档的**解析基准**（浏览器的 document.baseURI）：
// 文档 URL 存在 `<base href>` 时按其解析，否则就是文档 URL 本身。
//
// 页面里所有相对引用都用它当基准——`<link href>`、`<script src>`、
// `<img src>`/background-image、CSS `@import`、iframe src、fetch/XHR。
// `<base href="/assets/">` 是真实站点把静态资源挪到子目录的常规手段，此前
// 引擎完全不认它（相对引用一律按文档 URL 解析 → 全部 404）。
//
// 纯字符串（不读 DOM），因此可以在解释器线程安全调用。
func (wv *WebView) documentBaseURL() string {
	if wv == nil {
		return ""
	}
	wv.currentURLMu.RLock()
	u, href := wv.currentURL, wv.baseHref
	wv.currentURLMu.RUnlock()
	if href == "" {
		return u
	}
	return dom.ResolveURL(u, href)
}

// refreshBaseHref 重新读 `<base href>` 并缓存（宿主线程调用：装配完成时、
// DOM 结构变更回调里）。解释器线程只读缓存值（见 documentBaseURL）。
// DocumentBaseURL 返回主文档的解析基准（document.baseURI），供宿主把页面里的
// 相对引用映射到本地资源（媒体元数据探测、资源拦截、外部分析工具）。返回空串
// 表示当前文档没有来源 URL（LoadHTML 直出内容）。
func (wv *WebView) DocumentBaseURL() string { return wv.documentBaseURL() }

// refreshBaseHref 重新读 `<base href>` 并缓存（宿主线程调用：装配完成时、
// DOM 结构变更回调里）。解释器线程只读缓存值（见 documentBaseURL）。
func (wv *WebView) refreshBaseHref() {
	if wv == nil || wv.mainFrame == nil {
		return
	}
	doc := wv.mainFrame.Document()
	if doc == nil {
		return
	}
	h := doc.BaseHref()
	wv.currentURLMu.Lock()
	wv.baseHref = h
	wv.currentURLMu.Unlock()
}

func (wv *WebView) LoadURL(url string) error {
	// ★ 模式接线（4/4）：导航是浏览器能力。UI 库模式拒绝换源——宿主用
	//   LoadHTML 给出初始文档，之后不再导航（避免页面被替换后 Go 侧
	//   构建的 UI 树与事件绑定悬空）。
	if !wv.mode.allowsNavigation() {
		return fmt.Errorf("%w: LoadURL(%q) in %s mode", ErrModeNotSupported, url, wv.mode)
	}
	src, finalURL, err := fetchURLWithFinalURL(url)
	if err != nil {
		return err
	}
	// 取到内容后才换文档 URL：加载失败不应改变当前文档的基地址。
	// 用**重定向后的最终 URL**：浏览器里文档的 base URL 是最终地址，
	// 相对引用据此解析（http→https 跳转后仍按 http 解析会指错站点）。
	return wv.loadHTMLFrom(src, finalURL)
}

// Destroy 销毁 WebView：从全部全局注册表摘除并断开所有外部引用，
// 使该 WebView 的 DOM 树/渲染树/样式/JS 解释器整棵树可被 Go GC 回收。
// 挂件重建（改参数）与窗口关闭必须调用——否则 webviews/webviewBridges/
// webviewInterps 全局 map 与 bindings 监听 side-table 永久持有旧 WebView
// （每次改参数累积的内存泄漏根因）。调用后该 WebView 不可再使用
// （Render/EvalJS 等返回 ErrDestroyed）。
func (wv *WebView) Destroy() {
	if wv == nil || wv.destroyed {
		return
	}
	wv.destroyed = true
	// 交互管线状态（select 弹层元素/悬停元素）随 DOM 失效——断开
	// 引用让 GC 回收；再次 Interaction() 因 destroyed 返回 nil。
	wv.interact = nil
	// 1. 从全局注册表摘除（WebView/桥闭包/解释器归属映射）
	webviewsMu.Lock()
	delete(webviews, wv)
	delete(webviewBridges, wv)
	if wv.jsInterpreter != nil {
		if cur, ok := webviewInterps[wv.jsInterpreter]; ok && cur == wv {
			delete(webviewInterps, wv.jsInterpreter)
		}
	}
	webviewsMu.Unlock()
	// 1b. 文档基地址提供器（page 的全局登记表，闭包捕获本 WebView）——
	// 不摘除会让全局表永久持有已销毁的 WebView。
	page.ClearDocumentBaseProvider(wv.jsInterpreter)
	// 1c. 图片加载完成监听器（rendering 的全局集合同样捕获本 WebView）。
	if wv.imageLoadedOff != nil {
		wv.imageLoadedOff()
		wv.imageLoadedOff = nil
	}
	// 1c2. 播放帧交付监听器（同一条理由：rendering 的全局集合捕获本 WebView）。
	if wv.videoFrameOff != nil {
		wv.videoFrameOff()
		wv.videoFrameOff = nil
	}
	// 1d. 导航历史栈（bindings 的全局表持有 JS 值/闭包）。
	bindings.ResetNavigationStates(wv.jsInterpreter)
	// 2. iframe 子文档：注销注册表（iframeRegistry 持子 Frame 与元素）
	if wv.subframeJS != nil {
		for f := range wv.subframeJS {
			var el *dom.Element
			page.ForEachIFrame(func(e *dom.Element, sf *page.Frame) {
				if sf == f {
					el = e
				}
			})
			if el != nil {
				page.UnregisterIFrame(el)
			}
		}
		wv.subframeJS = nil
	}
	// 3. DOM bindings 全局缓存：nodeWrapperCache 持全部旧节点、
	// registeredListeners/windowEventListeners 持 JS 回调（解释器引用）、
	// observerRegistry 持目标节点——按本 WebView 的解释器/文档过滤清除
	// （多 WebView 共存：全表清空会破坏其他 WebView 的监听器）。
	if wv.jsInterpreter != nil {
		bindings.ClearPageBindingsFor(wv.jsInterpreter, wv.mainFrame.Document())
		if el := wv.jsInterpreter.GetEventLoop(); el != nil {
			el.Reset()
		}
	}
	// 4. 释放渲染 canvas（Skia Surface 等资源）
	if wv.canvas != nil {
		wv.canvas.Release()
		wv.canvas = nil
	}
	// 5. 断开内部引用链（page → frame → document → 渲染树）
	wv.page = nil
	wv.mainFrame = nil
	wv.jsInterpreter = nil
	wv.jsLogger = nil
}

// SetOnResourceLoaded 注册「异步资源（图片）取回完成」回调，传 nil 清除。
//
// 浏览器语义对照：资源到位即触发重绘，本引擎因此**不需要**宿主做任何事
// 就会重画（onAsyncImageLoaded 内部置脏 + 全量重绘）。本回调只供宿主侧
// 观察：日志、统计、把新资源同步给虚拟摄像头等。回调在取字节的 goroutine
// 里执行——不要在回调里直接操作 DOM/渲染树，应转投宿主自己的主循环。
func (wv *WebView) SetOnResourceLoaded(fn func(url string)) {
	if wv == nil {
		return
	}
	wv.resourceLoadedMu.Lock()
	wv.onResourceLoaded = fn
	wv.resourceLoadedMu.Unlock()
}

// onAsyncImageLoaded 是渲染层「图片加载完成」通知的接收端（成功与失败都
// 会触发）：标记渲染树脏 + 全量重绘，让按需渲染的宿主（app.Host 空闲帧跳过
// Paint）在下一帧画出新图片。图片可能改变布局（`<img>` 没有宽高属性时布局
// 依赖图片固有尺寸），因此一并请求重新布局——Frame.MarkRenderTreeDirty
// 内部有 cooldown 合并连续变更。
func (wv *WebView) onAsyncImageLoaded(url string) {
	if wv == nil || wv.destroyed {
		return
	}
	if mf := wv.mainFrame; mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
		if rv := mf.RenderView(); rv != nil {
			rv.MarkAllDirty()
		}
	}
	wv.resourceLoadedMu.Lock()
	fn := wv.onResourceLoaded
	wv.resourceLoadedMu.Unlock()
	if fn != nil {
		fn(url)
	}
}

// onAsyncVideoFrame 是渲染层「播放帧已交付」通知的接收端（A2-③）：标记渲染树脏 +
// 全量重绘，让按需渲染的宿主在下一帧画出新帧。
//
// 与图片那条通知（onAsyncImageLoaded）的差别：**不**请求重新布局——视频画面不参与
// 布局（元素几何由 CSS 决定，videoWidth/Height 早在元数据阶段就生效了），换帧只影响
// 绘制。这是每次播放换帧都会走的路径，少做一次 layout 请求是必要的。
//
// 回调在宿主 worker 的交付 goroutine 上执行；这里只置脏标记（引擎自带的标记有
// cooldown 合并），真正的重绘发生在宿主主循环的下一帧。
func (wv *WebView) onAsyncVideoFrame(_ string, _ float64) {
	if wv == nil || wv.destroyed {
		return
	}
	mf := wv.mainFrame
	if mf == nil {
		return
	}
	if fr := mf.Frame(); fr != nil {
		fr.MarkRenderTreeDirty()
	}
	if rv := mf.RenderView(); rv != nil {
		rv.MarkAllDirty()
	}
}

// FlushFrameBoundaryResources 在帧边界执行排队的动态资源（运行时插入的
// <script src> 与 <link rel=stylesheet>）。
//
// WebView.Render() 内部已调用（headless / 探针路径）；但窗口渲染循环
// （app.Host.Run）走 processEvents→EnsureLayout→Paint→Present，**不经过**
// Render()，必须每帧显式调用一次，否则运行时插入的插件 client 半永不执行
// （gou-ide 界面显示「标题栏未装配(ui-titlebar)」等降级占位符）。
func (wv *WebView) FlushFrameBoundaryResources() {
	wv.flushDynamicScripts()
}

// pendingScrollEls 收集**本帧内**被赋值的滚动容器（按元素去重），在帧边界统一派发。
//
// 浏览器语义：scroll 事件是**异步**的 —— 同一轮内对同一容器多次改 scrollTop，
// 只派发**一次** scroll（高频滚动在派发前被合并）。此前引擎在 setElementScrollOffset
// 里**同步**派发：连续滚动（CodeMirror 6 的程序化滚动 / 滚动条拖动回写 / scrollIntoView）
// 每赋值一次就跑一遍全部 scroll 监听器（CM6 的 scroll 处理 → requestMeasure → viewport
// 重算 → gutter 虚拟化重渲染），真实编辑器实测稳态 3948.818ms/op（A/B 对照实验 G1）。
var (
	pendingScrollMu  sync.Mutex
	pendingScrollEls = map[*dom.Element]struct{}{}
)

// queueScrollEvent 把元素加入本帧待派发集合（O(1)，按元素去重）。
func queueScrollEvent(el *dom.Element) {
	if el == nil {
		return
	}
	pendingScrollMu.Lock()
	pendingScrollEls[el] = struct{}{}
	pendingScrollMu.Unlock()
}

// flushScrollEvents 在帧边界派发去重后的 scroll 事件：同一元素**每帧最多一次**，
// 且所有元素复用同一个 Event 实例（与浏览器一致：一次滚动一个事件对象）。
func flushScrollEvents() {
	pendingScrollMu.Lock()
	if len(pendingScrollEls) == 0 {
		pendingScrollMu.Unlock()
		return
	}
	els := make([]*dom.Element, 0, len(pendingScrollEls))
	for el := range pendingScrollEls {
		els = append(els, el)
	}
	pendingScrollEls = map[*dom.Element]struct{}{}
	pendingScrollMu.Unlock()
	ev := dom.NewEvent("scroll", false, false, false)
	for _, el := range els {
		el.DispatchEvent(ev)
	}
}

func (wv *WebView) Render() ([]byte, error) {
	// 帧边界：执行本帧之前排队的动态 <script src>（JS 栈之外，见 handleDynamicScript）。
	wv.flushDynamicScripts()
	// 帧边界：派发本帧内合并（按元素去重）后的 scroll 事件 —— 浏览器语义（异步 + 合并）。
	flushScrollEvents()
	if wv.destroyed || wv.page == nil || wv.mainFrame == nil {
		return nil, ErrDestroyed
	}
	rv := wv.mainFrame.RenderView()
	if rv == nil {
		return nil, ErrNoDocument
	}
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() {
		view.Layout()
	}
	// ★ Layout（含 RebuildRenderTreeIfNeeded）可能重建渲染树→ RenderView
	// 换新实例——必须重新取，否则 Paint 旧树（画面滞后一帧/停留在旧结构，
	// 「点击后延迟生效」的另一来源）。
	rv = wv.mainFrame.RenderView()
	if rv == nil {
		return nil, ErrNoDocument
	}
	// ★ 渲染输出缓冲复用：尺寸未变时复用内部 Canvas（NewCanvas 每次创建
	// Skia RasterSurface + Paint + fontCache，高帧率下分配/释放风暴）。
	// 尺寸变化才重建；复用前清成完全透明（缓冲残留上一帧像素，新建
	// Canvas 初始即透明，清屏后行为一致——非脏区透明，匹配脏区绘制）。
	if wv.canvas == nil || wv.canvas.Width() != wv.width || wv.canvas.Height() != wv.height {
		if wv.canvas != nil {
			wv.canvas.Release()
		}
		wv.canvas = graphics.NewCanvas(wv.width, wv.height)
	}
	wv.canvas.Clear(graphics.Color{})
	dirtyRect := graphics.Rect{X: 0, Y: 0, Width: float64(wv.width), Height: float64(wv.height)}
	rendering.Paint(rv, wv.canvas, dirtyRect)
	return wv.canvas.Pixels(), nil
}

func (wv *WebView) Resize(width, height int) {
	if wv.destroyed || wv.page == nil {
		return
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	wv.width, wv.height = width, height
	// ★ window.innerWidth/innerHeight（CM6 visiblePixelRange 依赖；undefined
	// 会让 Math.min(win.innerHeight,…) 产生 NaN → viewport 永不更新 → 滚动
	// 后行号 gutter 不重渲染）。保留 ViewportWidth 兜底值（分派器
	// ViewportSizeForInterpreter 优先按解释器返回本 WebView 尺寸，多
	// WebView 场景挂件 Resize 不再污染配置窗口的 innerWidth）。
	bindings.ViewportWidth = float64(width)
	bindings.ViewportHeight = float64(height)
	if view := wv.page.MainFrame().View(); view != nil {
		view.SetSize(width, height)
	}
}

func (wv *WebView) EvalJS(script string) (jsc.JSValue, error) {
	if wv.destroyed {
		return jsc.Undefined(), ErrDestroyed
	}
	if !wv.settings.JavaScriptEnabled {
		return jsc.Undefined(), ErrJavaScriptDisabled
	}
	wv.ensureJSRuntime()
	if doc := wv.mainFrame.Document(); doc != nil && !wv.domBindingsInjected {
		// ★ 兜底注入：正常路径 LoadHTML 已在页面脚本执行前注入过
		//   （RegisterDOMBindings + injectRenderTreeBridge + OnStyleNodeAdded），
		//   EvalJS 无需重复——重复注入不仅浪费（wrapDocument + applyCanvas2DPatch
		//   的 RunJS + 重赋值 10+ 个包级几何桥闭包），还会让 JS 侧 document
		//   每次换成新对象（破坏浏览器单例语义）。仅当宿主直接 EvalJS 未走
		//   LoadHTML 时在此兜底。
		bindings.RegisterDOMBindings(wv.jsInterpreter, doc)
		wv.injectRenderTreeBridge()
		// Ensure callback for dynamic <style> injection.
		installBridgeDispatch()
		wvBridgeOf(wv).onStyleNodeAdded = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.RebuildRenderTree()
			}
		}
		wv.domBindingsInjected = true
	}
	result, err := wv.jsInterpreter.RunJS(script)
	if err != nil {
		return jsc.Undefined(), fmt.Errorf("webkit: JS eval failed: %w", err)
	}
	return result, nil
}

// CallFunction 调用页面全局 JS 函数（Go 主动调 JS）。这是宿主侧与页面
// 交互的声明式入口：页面定义 window 上的函数（Vue 组件方法、事件处理
// 等），Go 侧按名字直接调用并传参、取返回值。
//
//	name 支持点路径： "nav" / "app.nav" / "window.app.nav" 等价。
//	args 支持 Go 标量（string/bool/int/int64/float64/float32）、nil、
//	[]any、map[string]any——自动转换为对应 JS 值。
//
// 浏览器语义：函数内的 this 绑定为全局对象（window.fn() 的 this）。
// 返回值可直接用 jsc.JSValue 的 ToString/ToNumber/ToBoolean/AsObject 读取。
func (wv *WebView) CallFunction(name string, args ...any) (jsc.JSValue, error) {
	if wv.destroyed {
		return jsc.Undefined(), ErrDestroyed
	}
	if !wv.settings.JavaScriptEnabled {
		return jsc.Undefined(), ErrJavaScriptDisabled
	}
	wv.ensureJSRuntime()
	parts := strings.Split(name, ".")
	cur := wv.jsInterpreter.GlobalObject()
	for i := 0; i < len(parts)-1; i++ {
		p := strings.TrimSpace(parts[i])
		if p == "" || p == "window" || p == "globalThis" {
			continue
		}
		v, ok := cur.GetByKey(p)
		if !ok {
			return jsc.Undefined(), fmt.Errorf("webkit: CallFunction %q: %q not found", name, p)
		}
		o := v.AsObject()
		if o == nil {
			return jsc.Undefined(), fmt.Errorf("webkit: CallFunction %q: %q is not an object", name, p)
		}
		cur = o
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	fn, ok := cur.GetByKey(last)
	if !ok {
		return jsc.Undefined(), fmt.Errorf("webkit: CallFunction %q: function %q not found", name, last)
	}
	if !fn.IsCallable() {
		return jsc.Undefined(), fmt.Errorf("webkit: CallFunction %q: %q is not callable", name, last)
	}
	jsArgs := make([]jsc.JSValue, len(args))
	for i, a := range args {
		jsArgs[i] = wv.jsInterpreter.ValueOf(a)
	}
	global := wv.jsInterpreter.GlobalObject()
	result, err := wv.jsInterpreter.Call(fn, jsc.ObjectValue(global), jsArgs)
	if err != nil {
		return jsc.Undefined(), fmt.Errorf("webkit: CallFunction %q: %w", name, err)
	}
	return result, nil
}

// RenderHTML 以声明式方式更新页面 UI（"类 Vue 模板"的宿主侧入口）：
// 把 html 解析挂载到指定 id 的元素下（等价 el.innerHTML = html），并
// 标记渲染树脏 + 需要重排，下一帧自动重建渲染。宿主可拼接 HTML 模板
// 字符串（含数据）后调用，即可整体刷新一块 UI——无需逐元素命令式
// 创建/插入/改样式。
func (wv *WebView) RenderHTML(id, html string) error {
	if wv.destroyed || wv.mainFrame == nil {
		return ErrDestroyed
	}
	doc := wv.mainFrame.Document()
	if doc == nil {
		return ErrNoDocument
	}
	el := doc.GetElementById(id)
	if el == nil {
		return fmt.Errorf("webkit: RenderHTML: element #%s not found", id)
	}
	if err := el.SetInnerHTML(html); err != nil {
		return err
	}
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.MarkRenderTreeDirty()
		fr.SetNeedsLayout(true)
	}
	return nil
}

func (wv *WebView) ConsoleOutput() string {
	if wv.jsLogger == nil {
		return ""
	}
	return wv.jsLogger.String()
}

// ConsoleLogEntry 是一条带级别的控制台记录（CDP 的 Log.entryAdded 与
// Runtime.consoleAPICalled 需要级别：DevTools 里 error/warn 有红/黄区分）。
type ConsoleLogEntry struct {
	Level string // log/info/warn/error/debug
	Text  string
}

// ConsoleEntries 返回带级别的控制台记录（全量）。宿主适配层记自己的游标做增量——
// 引擎侧不做「取完即清」，避免多个消费者互相偷走对方的日志。
func (wv *WebView) ConsoleEntries() []ConsoleLogEntry {
	if wv == nil || wv.jsLogger == nil {
		return nil
	}
	entries, _ := wv.jsLogger.ConsoleEntries(0)
	out := make([]ConsoleLogEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ConsoleLogEntry{Level: e.Level, Text: e.Text})
	}
	return out
}

// SetConsoleLogger replaces the JS console's logger. After calling this,
// ConsoleOutput returns messages from the new logger.
func (wv *WebView) SetConsoleLogger(l *jsc.BufferLogger) {
	wv.jsLogger = l
	if wv.jsInterpreter != nil {
		wv.jsInterpreter.SetupGlobal(l)
	}
}

func (wv *WebView) ResetConsole() {
	if wv.jsLogger == nil {
		return
	}
	wv.jsLogger.Lines = nil
}

func (wv *WebView) ensureJSRuntime() {
	if wv.jsInterpreter != nil {
		return
	}
	wv.jsInterpreter = jsc.NewInterpreter()
	// ★ 解释器注册表：window.innerWidth 分派（ViewportSizeForInterpreter）
	// 按解释器归属查 WebView 视口尺寸（多 WebView 不被挂件 Resize 覆盖）。
	webviewsMu.Lock()
	webviewInterps[wv.jsInterpreter] = wv
	webviewsMu.Unlock()
	wv.jsLogger = &jsc.BufferLogger{}
	wv.jsInterpreter.SetupGlobal(wv.jsLogger)
	// Create event loop (needed by setTimeout/requestAnimationFrame).
	_ = jsc.NewEventLoop(wv.jsInterpreter)
	// Inject browser globals from Go implementations:
	// EventLoop timers (InjectBrowserEnv) + Web APIs (RegisterWebAPIs).
	wv.jsInterpreter.InjectBrowserEnv()
	wv.jsInterpreter.RegisterWebAPIs()
}

func (wv *WebView) JSInterpreter() *jsc.Interpreter {
	wv.ensureJSRuntime()
	return wv.jsInterpreter
}

func (wv *WebView) Document() *dom.Document {
	if wv.destroyed || wv.mainFrame == nil {
		return nil
	}
	return wv.mainFrame.Document()
}

// webkitAsRenderBox 把 RenderObject 转换为 *RenderBox（与 rendering 包
// 内 asRenderBox 等价：RenderBlock/RenderBlockFlow/RenderView 嵌入
// RenderBox，直接类型断言 *RenderBox 会失败——嵌入字段是值而非接口）。
func webkitAsRenderBox(o rendering.RenderObject) *rendering.RenderBox {
	switch v := o.(type) {
	case *rendering.RenderBox:
		return v
	case *rendering.RenderBlock:
		return &v.RenderBox
	case *rendering.RenderBlockFlow:
		return &v.RenderBlock.RenderBox
	case *rendering.RenderView:
		return &v.RenderBlockFlow.RenderBlock.RenderBox
	}
	return nil
}

// injectRenderTreeBridge wires the bindings package's render-tree callbacks
// (Element.scrollTop/scrollHeight/clientHeight/offsetHeight/... accessors) to
// this WebView's RenderView. Without it those CSSOM properties return 0 and
// frontend scroll APIs (el.scrollTop = el.scrollHeight) silently no-op.
func (wv *WebView) injectRenderTreeBridge() {
	// ★ rv 延迟获取：LoadHTML 注入时渲染树可能尚未重建（rv==nil），
	//   闭包内每次调用时再取 RenderView，保证前端 JS 读取几何时拿到最新实例。
	// ★ 强制同步布局（浏览器 forced reflow 语义）：DOM 变更（Vue patch 插入
	//   新消息）只 MarkRenderTreeDirty + SetNeedsLayout（延迟到下一帧渲染
	//   循环）。前端 scrollToBottom 在 nextTick（微任务）里读 scrollHeight /
	//   写 scrollTop——此时布局未跑，scrollHeight 还是旧值（新消息未计入
	//   内容高度）→ 跳底到旧位置/滚动条长度不对。浏览器读取几何属性会
	//   强制同步布局（forced reflow）拿到最新值；这里在几何桥入口先
	//   EnsureLayout（含脏渲染树重建）对齐浏览器语义。
	forceLayout := func() {
		if fr := wv.mainFrame.Frame(); fr != nil {
			fr.RebuildRenderTreeIfNeeded()
		}
		wv.EnsureLayout()
	}
	wrapBox := func(el *dom.Element, fn func(box *rendering.RenderBox) (float64, float64)) (float64, float64) {
		forceLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			return 0, 0
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			return 0, 0
		}
		return fn(box)
	}
	// canvas 2D drawImage(<img>) 的位图源：从渲染树取 <img> 解码图。
	bindings.SetCanvasImageSourceHook(func(el *dom.Element) *graphics.SkiaImage {
		if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
			log.Printf("[canvas2d-hook] img el=%p tag=%q src=%q", el, el.LocalName(), el.GetAttribute("src"))
		}
		if fr := wv.mainFrame.Frame(); fr != nil {
			fr.RebuildRenderTreeIfNeeded()
		}
		wv.EnsureLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
				log.Printf("[canvas2d-hook] rv/el nil (rv=%v)", rv != nil)
			}
			return nil
		}
		box := rv.FindRenderBoxForNode(el)
		var img *rendering.DecodedImage
		if box != nil {
			img = box.DecodedImage()
		}
		if img == nil || !img.Loaded() {
			// 图片尚未进入 paint 管线（display:none / 未布局）→ 按 src
			// 主动解码（data: URI 同步；其余经 WebView 的图片接线取字节，
			// 异步回缓存 —— 宿主 ResourceResolver 与模式门禁同样生效）。
			if src := el.GetAttribute("src"); src != "" {
				img = rendering.LoadImageWithLoader(src, &webViewImageLoader{wv: wv})
				if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
					log.Printf("[canvas2d-hook] LoadImage src=%q → %v", src, img != nil)
				}
			}
		}
		if img == nil || !img.Loaded() {
			if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
				log.Printf("[canvas2d-hook] img not loaded")
			}
			return nil
		}
		return img.SkiaImage()
	})
	installBridgeDispatch()
	// <img>/<video> src 变化：清除渲染盒的解码图缓存并重建渲染树
	//（下次绘制按新 src 解码 —— canvas 2D drawImage 源和 <img> 渲染共用）。
	bindings.OnImageSrcChanged = func(el *dom.Element) {
		// ★ 按元素归属解析宿主 WebView（理由同 GetElementComputedSnapshot）。
		owner := webViewForNode(el)
		if owner == nil || owner.mainFrame == nil {
			return
		}
		if fr := owner.mainFrame.Frame(); fr != nil {
			if rv := owner.RenderView(); rv != nil {
				if box := rv.FindRenderBoxForNode(el); box != nil {
					box.SetDecodedImage(nil)
				}
			}
			fr.RebuildRenderTreeIfNeeded()
		}
	}
	installBridgeDispatch()
	wvBridgeOf(wv).getElementScrollOffset = func(el *dom.Element) (float64, float64) {
		return wrapBox(el, func(box *rendering.RenderBox) (float64, float64) {
			if rv := wv.RenderView(); rv != nil {
				return rv.BoxScrollOffset(box)
			}
			return 0, 0
		})
	}
	wvBridgeOf(wv).setElementScrollOffset = func(el *dom.Element, x, y float64) {
		// ★ 滚动赋值是**纯几何操作**：只有当确有「待处理的布局 / 渲染树变更」时才同步布局。
		// 浏览器与此一致 —— 写 scrollTop/scrollLeft 不触发 forced reflow，只有**读**几何
		// 属性（scrollHeight / offsetHeight 等）才强制布局。
		//
		// 此前此处无条件调用 forceLayout()（RebuildRenderTreeIfNeeded + EnsureLayout 全量
		// 重建+布局），使连续滚动（CodeMirror 6 的 scroll 监听驱动的程序化滚动、滚动条拖动
		// 回写、scrollIntoView 等）**每一次赋值都做一次全量布局**。真实编辑器实测：
		// 稳态 3948.818ms/op（见 docs/PERF_BASELINE.md 的 A/B 对照实验 G1）。
		// 布局未脏时跳过，几何沿用当前值（ScrollRange 的 clamp 用当前几何，语义不变）。
		if fr := wv.mainFrame.Frame(); fr != nil && fr.NeedsLayout() {
			fr.RebuildRenderTreeIfNeeded()
			wv.EnsureLayout()
		}
		rv := wv.RenderView()
		box := func() *rendering.RenderBox {
			if rv == nil || el == nil {
				return nil
			}
			return rv.FindRenderBoxForNode(el)
		}()
		if box == nil {
			return
		}
		// 浏览器语义：非滚动容器上 scrollTop/scrollLeft 赋值无效（忽略）。
		// ★ 判定用滚动范围而不是滚动条几何：VerticalScrollbarMetrics.OK 表示
		// "滚动条该不该绘制"，容器小到放不下箭头按钮时会为 false（不画滚动条），
		// 但元素仍然可滚动——用 OK 当"可否滚动"会让小尺寸滚动容器上的赋值被
		// 静默丢弃（10×10 的 overflow:scroll 容器在浏览器里可以 scrollTop=15）。
		maxX, maxY, canX, canY := rendering.ScrollRange(rv, box)
		if !canX && !canY {
			return
		}
		if canY {
			if y < 0 {
				y = 0
			}
			if y > maxY {
				y = maxY
			}
		} else {
			y = 0
		}
		if canX {
			if x < 0 {
				x = 0
			}
			if x > maxX {
				x = maxX
			}
		} else {
			x = 0
		}
		rv.SetBoxScrollOffset(box, x, y)
		// ★ 浏览器标准：el.scrollTop = N 后须派发 scroll 事件（下一帧/
		// 微任务，此处同步派发等效）——CM6 监听 scroller 的 scroll 事件
		// → requestMeasure → viewport 更新 → gutter 行号虚拟化重渲染。
		// 此前 scrollTop 赋值静默（wheel 路径有 dispatchScrollEvent，JS
		// 赋值路径没有）→ 程序化滚动（scrollIntoView/滚动条拖到顶后
		// 设置值/CM6 自动滚动）后行号不刷新——用户「滚动该绘制的行号
		// 不显示，仍裁切」的直接根因。
		if el != nil {
			// ★ 2026-09 改为**异步 + 帧内合并**派发（浏览器语义）：同一帧内多次赋值只派发
			// 一次 scroll，避免连续滚动把全部 scroll 监听器（CM6 的 scroll → requestMeasure
			// → viewport 重算）跑 N 遍 —— 真实编辑器实测稳态 3948.818ms/op（A/B 对照 G1），
			// 同步派发是其中主要成本。语义不变：滚动后仍会派发，只是延到帧边界且去重。
			queueScrollEvent(el)
		}
	}
	// gutterReservesSpace 报告元素是否因 scrollbar-gutter: stable/always 需要为
	// 垂直滚动条**常驻预留**空间（CSS Overflow 4）：auto → 不预留；stable → 该轴
	// overflow 非 visible 时预留；always → 同样预留。
	gutterReservesSpace := func(st *style.ComputedStyle) bool {
		if st == nil {
			return false
		}
		g := strings.TrimSpace(st.GetProperty("scrollbar-gutter"))
		if g == "" || g == "auto" {
			return false
		}
		if !strings.HasPrefix(g, "stable") && !strings.HasPrefix(g, "always") {
			return false
		}
		return st.OverflowY != style.OverflowVisible
	}
	installBridgeDispatch()
	wvBridgeOf(wv).getElementScrollMetrics = func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool) {
		forceLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			return 0, 0, 0, 0, false
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			// ★ DOM 刚插入但渲染树未标脏时（CM6 构造早期查询
			// scrollDOM.clientHeight/scrollHeight——初始 viewport 计算
			// 依赖它），IfNeeded 重建看不到新节点 → 返回 0 → CM6 初始
			// viewport 为空 → 首次 lineHeights 测量落空 → HeightOracle
			// 停留默认 lineHeight=14 → 行号栏 14px/行与内容 18.2px 错位。
			// 与 GetElementBoxRect 同策略：强制无条件重建一次再查。
			if fr2 := wv.mainFrame.Frame(); fr2 != nil {
				fr2.RebuildRenderTree()
			}
			forceLayout()
			rv = wv.RenderView()
			if rv != nil {
				box = rv.FindRenderBoxForNode(el)
			}
		}
		if box == nil {
			return 0, 0, 0, 0, false
		}
		pb := box.PaddingBoxRect()
		tw, th := rv.BoxContentSize(box)
		// ★ H5-1：scrollWidth/scrollHeight 至少等于 padding box 尺寸
		//   （CSSOM View §6.2/6.3）。空内容的 overflow:scroll 容器其 scroll
		//   尺寸 = client 尺寸；Edge 实测 #scroll（200x60 容器、无内容）= 200x60，
		//   而 BoxContentSize 对空内容返回 0x0。
		if tw < pb.Width {
			tw = pb.Width
		}
		if th < pb.Height {
			th = pb.Height
		}
		// ★ H5-2：scrollbar-gutter: stable/always 为垂直滚动条**常驻预留**空间
		//   → clientWidth 扣掉滚动条宽度。Edge 实测：200x60 容器 + overflow:auto
		//   + scrollbar-gutter:stable → clientWidth = 185 = 200-15，而 offsetWidth
		//   仍为 200（布局宽度不变，只影响 CSSOM 读数）。
		vw := pb.Width
		if gutterReservesSpace(box.Style()) {
			if vw -= style.ScrollbarWidth(box.Style()); vw < 0 {
				vw = 0
			}
		}
		vm := rendering.VerticalScrollbarMetrics(rv, box)
		hm := rendering.HorizontalScrollbarMetrics(rv, box)
		return vw, pb.Height, tw, th, (vm.OK || hm.OK)
	}
	// GetElementBoxRectFast：布局缓存直读（不触发 rebuild/layout）。
	// computedStyleFor 的 height/width 兜底用它——CM6 measure 期间渲染树
	// 频繁 dirty，若每次强制全量 rebuild（~22ms）→ 测量-布局风暴。
	bindings.GetElementBoxRectFast = func(el *dom.Element) (left, top, width, height float64) {
		// ★ 按元素归属解析宿主 WebView（理由同 GetElementComputedSnapshot）：捕获
		// 注入时的 wv 会在「后创建者被销毁」后拿到 nil 渲染树，紧接着
		// FindRenderBoxForNode 解引用 → panic（自检 S3 判据 15 实测复现）。
		owner := webViewForNode(el)
		if owner == nil || el == nil {
			return 0, 0, 0, 0
		}
		wvBridgeOf(owner).getElementBoxRectFast = func(el *dom.Element) (left, top, width, height float64) {
			return 0, 0, 0, 0
		}
		rv := owner.RenderView()
		if rv == nil {
			return 0, 0, 0, 0
		}
		box := rv.FindRenderBoxForNode(el)
		var x0, y0, w, h float64
		hasGeom := false
		if box != nil {
			x0, y0, w, h = box.X(), box.Y(), box.Width(), box.Height()
			hasGeom = true
			if x0 == 0 && y0 == 0 {
				if ro0 := rv.FindRenderObjectForNode(el); ro0 != nil {
					if st0 := ro0.Style(); st0 != nil && st0.Display == style.DisplayInline {
						if sx, sy, ok := firstTextSegmentBase(rv, el); ok {
							x0, y0 = sx, sy
						}
					}
				}
			}
		} else if ro := rv.FindRenderObjectForNode(el); ro != nil && ro.LayoutBox() != nil {
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(ro.LayoutBox())
				x0, y0, w, h = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
				hasGeom = true
			}
		}
		if !hasGeom {
			return 0, 0, 0, 0
		}
		if math.IsNaN(w) || math.IsNaN(h) || w < 0 || h < 0 {
			return 0, 0, 0, 0
		}
		return x0, y0, w, h
	}
	// cssShorthand4 把四边值拼成 CSS 简写（与浏览器 computed 的序列化一致）：
	// 四边相同 → 1 值；上下/左右相同 → 2 值；左右相同 → 3 值；否则 4 值。
	// 例：input 的 1px/2px/1px/2px → "1px 2px"；button 的 1px/6px/1px/6px →
	// "1px 6px"；select 的 0 → "0px"。
	cssShorthand4 := func(t, r, b, l string) string {
		if t == r && r == b && b == l {
			return t
		}
		if t == b && r == l {
			return t + " " + r
		}
		if r == l {
			return t + " " + r + " " + b
		}
		return t + " " + r + " " + b + " " + l
	}
	// GetElementComputedSnapshot：把渲染树上的 resolved computed style 关键
	// 属性交给 getComputedStyle（见 bindings.GetElementComputedSnapshot）。
	// 级联 map 只含元素**自身命中**的声明，缺继承值时 JS 会读到 CSS 初始值
	// （`body{font-size:40px}` 的子元素读回 16px），而布局用的是继承来的 40px。
	// 这里给出的是布局/绘制实际使用的值（font-size 已是 D1 绝对化后的 px），
	// 保证 JS / 布局 / 绘制三方同源。
	bindings.GetElementComputedSnapshot = func(el *dom.Element) map[string]string {
		// ★ 按元素**归属**解析宿主 WebView，不捕获注入时的 wv：同一进程可同时存在
		// 多个 WebView（多窗口宿主、自检里的临时宿主），捕获写法有两个后果：
		// ① 后创建者覆盖前者 → 前者页面读到后者渲染树的数据；
		// ② 后者销毁后回调悬挂在已释放的 WebView 上 → RenderView 内部 nil 解引用
		//    直接 panic（自检 S3 判据 15 实测复现）。
		owner := webViewForNode(el)
		if owner == nil || el == nil {
			return nil
		}
		rv := owner.RenderView()
		if rv == nil {
			return nil
		}
		ro := rv.FindRenderObjectForNode(el)
		if ro == nil {
			return nil // display:none 子树 / 尚未建树
		}
		st := ro.Style()
		if st == nil {
			return nil
		}
		snap := make(map[string]string, 12)
		if st.FontSize.Value > 0 {
			snap["font-size"] = fmt.Sprintf("%gpx", st.FontSize.Value)
		}
		switch {
		case st.Color.A == 0xFF:
			snap["color"] = fmt.Sprintf("rgb(%d, %d, %d)", st.Color.R, st.Color.G, st.Color.B)
		default:
			snap["color"] = fmt.Sprintf("rgba(%d, %d, %d, %g)",
				st.Color.R, st.Color.G, st.Color.B, float64(st.Color.A)/255)
		}
		if st.FontFamily != "" {
			snap["font-family"] = st.FontFamily
		}
		if st.FontStyle != "" {
			snap["font-style"] = st.FontStyle
		}
		if st.FontVariant != "" {
			snap["font-variant"] = st.FontVariant
		}
		if st.TextTransform != "" {
			snap["text-transform"] = st.TextTransform
		}
		// text-align / white-space 在引擎里是 int 枚举（TextAlignType /
		// WhiteSpaceType），此处不做枚举→关键字映射：字符串继承 + JS 初值
		// 兜底已经能覆盖这两个属性的常见读法，宁可不写也不写错。
		if st.Direction != "" {
			snap["direction"] = st.Direction
		}
		if st.Visibility != "" {
			snap["visibility"] = st.Visibility
		}
		if st.Cursor != "" {
			snap["cursor"] = st.Cursor
		}
		// line-height：computed 值要么是关键字 normal，要么是绝对 px
		// （数值倍率在布局期已折算，此处无法判定换算基准，故只报可确定的）。
		switch st.LineHeight.Unit {
		case "normal":
			snap["line-height"] = "normal"
		case "px":
			snap["line-height"] = fmt.Sprintf("%gpx", st.LineHeight.Value)
		}
		if st.LetterSpacing.Value == 0 || st.LetterSpacing.Unit == "normal" {
			snap["letter-spacing"] = "normal"
		} else if st.LetterSpacing.Unit == "px" {
			snap["letter-spacing"] = fmt.Sprintf("%gpx", st.LetterSpacing.Value)
		}
		// ★ H2：表单控件的 UA padding / border-width（Chromium html.css）。
		//   computedStyleFor 只级联作者 <style>，UA 规则对它不可见 → 控件读到
		//   padding=undefined；而渲染树 style 是布局实际使用值（作者 + UA）。
		//   只对控件补，避免改变其它元素的 computed 输出。
		switch strings.ToLower(el.LocalName()) {
		case "input", "button", "select", "textarea":
			// 只在四边都是 px（或空单位）时补，避免把 auto/% 误报成 "0px"。
			pxLen := func(l style.Length) string {
				if l.Unit == "" || l.Unit == "px" {
					return fmt.Sprintf("%gpx", l.Value)
				}
				return ""
			}
			// set4 写四边 longhand + CSS 简写（Edge 的 computed 简写即四边拼接）。
			set4 := func(prefix string, a, b, c, d style.Length) {
				ls := [4]style.Length{a, b, c, d}
				names := [4]string{"-top", "-right", "-bottom", "-left"}
				var s [4]string
				for i := range ls {
					s[i] = pxLen(ls[i])
					if s[i] == "" {
						return
					}
					snap[prefix+names[i]] = s[i]
				}
				snap[prefix] = cssShorthand4(s[0], s[1], s[2], s[3])
			}
			set4("padding", st.PaddingTop, st.PaddingRight, st.PaddingBottom, st.PaddingLeft)
			// ★ margin 同样是 UA 规则给的：checkbox 3px 3px 3px 4px、
			//   radio 3px 3px 0 5px、range 2px，其余控件 0（Edge 实测
			//   g1_formctl 的 t2/t3/t4/t1）。
			set4("margin", st.MarginTop, st.MarginRight, st.MarginBottom, st.MarginLeft)
			set4("border-width", st.BorderTopWidth, st.BorderRightWidth, st.BorderBottomWidth, st.BorderLeftWidth)
		}
		return snap
	}
	// GetTextBasePos：返回文本节点自身首个 render segment 的绝对位置。
	// ★ 裸文本节点（CM6 行内标点/空格直接挂在 .cm-line 下，父元素是 block
	// 容器）的 Range.getClientRects 子区间测量：父 box left = 行首，漏掉
	// 该节点前面兄弟内容宽度 →「空格多的行」posAtCoords 错乱。文本节点
	// 的 RenderText segment.X 已含行内全部前缀（等于浏览器 Range 起始）。
	wvBridgeOf(wv).getTextBasePos = func(n dom.Node) (float64, float64, bool) {
		rv := wv.RenderView()
		if rv == nil || n == nil {
			return 0, 0, false
		}
		ro := rv.FindRenderObjectForNode(n)
		if rt, ok := ro.(*rendering.RenderText); ok {
			segs := rt.Segments()
			if len(segs) > 0 {
				return segs[0].X, segs[0].Y, true
			}
		}
		return 0, 0, false
	}
	wvBridgeOf(wv).getElementBoxRect = func(el *dom.Element) (left, top, width, height float64) {
		forceLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			return 0, 0, 0, 0
		}
		box := rv.FindRenderBoxForNode(el)
		// ★ 光标（cm-cursor）用渲染树 walk 找 box：FindRenderBoxForNode
		// （nodeRenderMap）在 CM6 每次 measure 重建光标元素后映射到旧 box
		// 实例 → 渲染树父链断在 .cm-editor 拿不到 .cm-scroller 滚动偏移 →
		// getBoundingClientRect 不扣滚动（光标固定屏幕坐标）。FindCursorBox
		// walk 当前渲染树，父链正确（与绘制 fallback 同语义）。
		if strings.Contains(el.ClassName(), "cm-cursor") {
			if cb := rendering.FindCursorBox(rv); cb != nil {
				box = cb
			}
		}
		// ★ WB_PAINT_TRACE=1：调试光标（cm-cursor）渲染树 box 查找——
		// 反向跟踪「光标不可见」：box 是否存在、frame 几何、样式 display。
		if os.Getenv("WB_PAINT_TRACE") != "" && strings.Contains(el.ClassName(), "cm-cursor") {
			// ★ WB_PAINT_TRACE=1 诊断光标渲染树 box 查找（反向跟踪光标不可见）
			dispStr := "n/a"
			if fr3 := wv.mainFrame.Frame(); fr3 != nil && fr3.Resolver() != nil {
				if cs3 := fr3.Resolver().ResolveElement(el); cs3 != nil {
					dispStr = fmt.Sprintf("%v", cs3.Display)
				}
			}
			fmt.Printf("[cursor-dbg] resolverDisp=%s box=%v\n", dispStr, box != nil)
			if box != nil {
				fmt.Printf("[cursor-dbg] FindRenderBoxForNode OK frame=(%.1f,%.1f %.1fx%.1f) styleDisp=%v\n",
					box.X(), box.Y(), box.Width(), box.Height(), box.Style().Display)
			} else if ro := rv.FindRenderObjectForNode(el); ro != nil {
				if lb := ro.LayoutBox(); lb != nil {
					if ls := rv.LayoutState(); ls != nil {
						g := ls.GeometryForBox(lb)
						fmt.Printf("[cursor-dbg] box=nil RenderObject OK geom=(%.1f,%.1f %.1fx%.1f)\n",
							g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
					} else {
						fmt.Printf("[cursor-dbg] box=nil RenderObject noLayoutState\n")
					}
				} else {
					fmt.Printf("[cursor-dbg] box=nil RenderObject noLayoutBox\n")
				}
			} else {
				fmt.Printf("[cursor-dbg] box=nil RenderObject=nil (渲染树无 cursor 节点)\n")
			}
		}
		if box == nil {
			// （绕过 MarkRenderTreeDirty 的 cooldown 降频——此处必须立即
			// 拿到几何）。
			if fr2 := wv.mainFrame.Frame(); fr2 != nil {
				fr2.RebuildRenderTree()
			}
			forceLayout()
			rv = wv.RenderView()
			if rv != nil {
				box = rv.FindRenderBoxForNode(el)
			}
		}
		var x0, y0, w, h float64
		hasGeom := false
		if box != nil {
			x0, y0, w, h = box.X(), box.Y(), box.Width(), box.Height()
			hasGeom = true
			// ★ inline 元素（display:inline 的 span 等）布局后 box.frame
			// 位置恒 0（文本由 TextSegment 定位，painter 画在 seg.X/seg.Y）——
			// 用渲染子树首个 RenderText 的 segment 位置兜底：CM6 高亮
			// token 内文本的 Range.getClientRects 需要真实 x/y，否则恒
			// (0,0) → posAtCoords 的 x 定位全 miss → 点击落行末（光标进
			// 下一行/光标处输入插错位置）。
			if x0 == 0 && y0 == 0 {
				if ro0 := rv.FindRenderObjectForNode(el); ro0 != nil {
					if st0 := ro0.Style(); st0 != nil && st0.Display == style.DisplayInline {
						if sx, sy, ok := firstTextSegmentBase(rv, el); ok {
							x0, y0 = sx, sy
						}
					}
				}
			}
		} else if ro := rv.FindRenderObjectForNode(el); ro != nil && ro.LayoutBox() != nil {
			// ★ inline 元素（RenderInline，如 CM6 语法高亮 span）不生成
			// CSS box，asRenderBox 返回 nil → 此前恒 (0,0)。但 inline 参与
			// 行内布局、LayoutBox 有几何——CM6 高亮 token 内文本的
			// Range.getClientRects 需要真实位置，否则 tile rect 恒 (0,0) →
			// posAtCoords 的 x 定位全部 miss → 点击落行末（head=行末尾）
			// →「点击选中行在下一行」（光标看似在下一行开头）+ 光标处输入
			// 插错位置（用户「编辑器不能编辑」）。用布局几何兜底。
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(ro.LayoutBox())
				x0, y0, w, h = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
				hasGeom = true
			}
		}
		if !hasGeom {
			return 0, 0, 0, 0
		}
		// ★ NaN/负值防御：布局未稳定时（xterm 初始化测量时刻）box 几何
		// 可能是 NaN——offsetWidth/offsetHeight 返回 NaN 会让 xterm 的
		// measure() 条件（0!==NaN 恒真）把 NaN 缓存进 _result → 行高
		// NaN。返回 0 使 xterm 条件为假、不更新缓存（保持 0 待重测）。
		if math.IsNaN(w) || math.IsNaN(h) || w < 0 || h < 0 {
			return 0, 0, 0, 0
		}
		// ★ 浏览器语义：getBoundingClientRect 返回「视口相对坐标」——
		// 扣除所有祖先滚动容器的滚动偏移。CM6 的 visiblePixelRange 用
		// contentDOM.getBoundingClientRect() 感知滚动（滚动后 rect.top
		// 应变小）→ 更新 viewport → 行号 gutter 虚拟化重渲染。此前返回
		// 未扣滚动的布局坐标 → 滚动后 rect 不变 → CM6 viewport 永不更新
		// → 滚动后行号不刷新（用户「滚动时初始超出区域的行号都没有绘制」）。
		// ★ 沿渲染树 Parent 链查找滚动容器（★ 2026-08-12 改为渲染树链：
		// 原 DOM 祖先链对 .cm-cursor 失效——光标 DOM 的祖先链不经过
		// .cm-scroller（CM6 把 cursorLayer 挂在不同层级/FindRenderBoxForNode
		// 对祖先 DOM 返回 nil），滚动后光标的 getBoundingClientRect 不扣
		// 滚动偏移 → 返回布局坐标（内容 top+容器偏移），CM6 认为光标没
		// 动（「光标固定屏幕坐标」）；fallback 绘制（caretScrollOffset）
		// 用渲染树链已验证可靠（scroll=(0,200) 正确）——几何桥与绘制
		// 必须同一坐标语义，统一走渲染树链）。
		// ★ sticky 语义：position:sticky 元素（及其子孙）只在「有
		// top/bottom inset」时钉在滚动容器视口内（CSS: 无 inset 的
		// sticky 等同 relative，不钉住）——CM6 的 .cm-gutters 是
		// position:sticky 但无 top/left → 跟随内容竖向滚动（VS Code 式
		// 行号），其 getBoundingClientRect 必须扣滚动偏移，否则行号
		// rect 返回布局坐标与绘制错位 → CM6 虚拟化把行号画在错位位置
		// （用户「滚动后行号裁切」根因）。此前无差别处理 sticky →
		// gutter 元素不扣滚动 → 行号画在布局位置（视口底部/外）。
		// 有 top/bottom 的 sticky（如 .tl-think-fold bottom:0）钉住 →
		// 不扣该滚动容器偏移；更外层滚动容器照常扣。
		sx, sy := 0.0, 0.0
		// ★ stickySeen 必须同时要求「真是 sticky 定位」：CM6 光标
		// .cm-cursor 是 position:absolute + top:4px（left/top 由 CM6 写
		// 内联样式），stickyHasInset 只看 top/bottom 属性 → 光标被误判为
		// sticky → 循环遇 cm-scroller（overflow:auto）时 continue 跳过
		// 滚动偏移累计 → getBoundingClientRect 不扣滚动（光标固定屏幕
		// 坐标）。只有真 sticky（IsStickyPositioned）且带 inset 才钉住。
		stickySeen := box != nil && box.IsStickyPositioned() && stickyHasInset(box)
		// ★ 渲染树 Parent 链（与绘制 caretScrollOffset 同语义）：从 box
		// 向上找 overflow 滚动容器累加偏移。box 为 nil（inline 无 CSS box）
		// 时退化为原 DOM 链兜底（Range 文本测量等场景 box 存在，此处
		// 主要服务元素几何）。
		// ★ 祖先链上的 transform：子元素的**视觉**矩形必须累计祖先的变换
		// （绘制端 applyTransformOpsSized 沿祖先链压栈，几何端此前只看自身
		// 元素 → 两者不一致）。并行切片记录 transform 串 + 参考尺寸 + 布局位置。
		var trT []string
		var trW, trH, trX, trY []float64
		// ★ transform-origin：变换必须**绕 origin** 进行（CSS Transforms L1 §4），
		//   祖先与自身都要带上。只应用矩阵而不含 origin 时，默认 50% 50% 的
		//   scale/rotate 会绕左上角发生 —— 视觉矩形与绘制端
		//   （applyOwnOverflowClip 的 Translate(O)→M→Translate(−O)）不一致。
		var trS []*style.ComputedStyle
		anc := rendering.RenderObject(nil)
		if box != nil {
			anc = rendering.RenderObject(box)
		}
		for anc != nil && anc.Parent() != nil {
			par := anc.Parent()
			pb := webkitAsRenderBox(par)
			if pb == nil {
				// inline 祖先（RenderInline）无 CSS box：跳过继续向上。
				anc = par
				continue
			}
			b := pb
			// ★ 记录祖先链上的 transform（见返回处的视觉矩形累计）。
			if st := b.Style(); st != nil && st.Transform != "" && st.Transform != "none" {
				trT = append(trT, st.Transform)
				trW = append(trW, b.Width())
				trH = append(trH, b.Height())
				trX = append(trX, b.X())
				trY = append(trY, b.Y())
				trS = append(trS, st)
			}
			anc = par
			if stickySeen {
				if cs := b.Style(); cs != nil &&
					(cs.OverflowX == style.OverflowAuto || cs.OverflowX == style.OverflowScroll ||
						cs.OverflowY == style.OverflowAuto || cs.OverflowY == style.OverflowScroll) {
					stickySeen = false
				}
				continue
			}
			ox, oy := rv.BoxScrollOffset(b)
			sx += ox
			sy += oy
			if b.IsStickyPositioned() && stickyHasInset(b) {
				stickySeen = true
			}
		}
		// ★ transform 元素（弹窗 translate(-50%,-50%) 居中/缩放等）：
		// getBoundingClientRect/offsetLeft 按 CSSOM-View §4.2 返回**变换
		// 后**的视口矩形（布局框四角经 transform 的轴对齐包围盒）。此前
		// 只返回布局框——弹窗视觉居中于 (500,339) 而 rect 报 (640,400)，
		// 依赖矩形定位/居中的应用全部错位（引擎层根治，应用无需补偿）。
		// 注释说明：transform 是元素局部变换，与祖先滚动平移可交换，
		// 先扣滚动再应用 transform 语义正确。
		// ★ 祖先 transform 累计（CSSOM-View §4.2：返回元素的**视觉**视口矩形）：
		// 元素位于带 transform 的祖先内时（顶栏居中组 .tb-nav 的 left:50%+
		// translate(-50%)、弹窗 translate(-50%,-50%)），视觉位置与布局位置相差
		// 祖先的变换位移——只应用自身 transform 会让子元素的 rect 停在布局位置
		// （gou-ide 顶栏胶囊 rect.x=800 而绘制实际在 534，凡按 rect 定位/命中/
		// 测量的调用方全部错位）。
		// 逐层「从内到外」把坐标映射到上一层：p = A.pos + M_A(p − A.pos)。
		// 复用 TransformRect（w=h=0 时退化为单点映射），与绘制端同一套
		// tokenizer/长度解析（含 translate 百分比按元素自身 border box 解析）。
		// trT 为空（无 transform 祖先）时下方结果与旧行为逐位一致（零回归）。
		rx0, ry0, rw, rh := x0, y0, w, h
		for i := range trT {
			lx, ly := rx0-trX[i], ry0-trY[i]
			mx, my, mw, mh, ok := rendering.TransformRectByStyleOrigin(trS[i], trW[i], trH[i], lx, ly, rw, rh)
			if !ok {
				continue
			}
			rx0, ry0, rw, rh = mx+trX[i], my+trY[i], mw, mh
		}
		if box != nil {
			// ★ 自身 transform：必须绕 transform-origin、且以**元素局部坐标**求
			//   包围盒再平移回视口。TransformRect 的语义是「绕元素原点变换」，
			//   此前直接喂 rx0-sx/ry0-sy 视口绝对坐标 → scale(0.5) 把元素自身的
			//   位置也缩了一半（实测 g2_transform 的 #sc：布局 top=140，wbui 报
			//   70 = 140×0.5，Edge 报 140）。TransformRectForStyle 内部先转局部
			//   坐标、按 origin 变换、再平移回视口，与 Edge 语义一致。
			if nx, ny, nw, nh, ok := rendering.TransformRectForStyle(box.Style(), w, h, rx0-sx, ry0-sy, rw, rh); ok {
				return nx, ny, nw, nh
			}
		}
		return rx0 - sx, ry0 - sy, rw, rh
	}
	// Range.getClientRects 文本测量需要元素 computed 字体（CodeMirror 6
	// 的 charWidth/lineHeight 探测；缺 createRange/字体时测量抛异常，
	// HeightOracle 停留默认 14 → 行号栏按 14px/行步进与内容 18.2px 错位）。
	installBridgeDispatch()
	wvBridgeOf(wv).getElementComputedFont = func(el *dom.Element) (string, float64, int, string) {
		if el == nil {
			return "sans-serif", 14, 400, "normal"
		}
		fr := wv.mainFrame.Frame()
		if fr == nil || fr.Resolver() == nil {
			return "sans-serif", 14, 400, "normal"
		}
		cs := fr.Resolver().ResolveElement(el)
		if cs == nil {
			return "sans-serif", 14, 400, "normal"
		}
		size := cs.FontSize.Value
		if size <= 0 {
			size = 14
		}
		w := 400
		switch strings.ToLower(strings.TrimSpace(cs.FontWeight)) {
		case "bold", "bolder", "600", "700", "800", "900":
			w = 700
		}
		st := "normal"
		if strings.EqualFold(cs.FontStyle, "italic") || strings.EqualFold(cs.FontStyle, "oblique") {
			st = cs.FontStyle
		}
		fam := cs.FontFamily
		if fam == "" {
			fam = "sans-serif"
		}
		return fam, size, w, st
	}
}

func (wv *WebView) RenderView() *rendering.RenderView {
	// 防御：WebView 已销毁（mainFrame 被释放）时不能解引用——全局桥回调可能
	// 在多 WebView 场景下拿到已销毁实例（见 GetElementComputedSnapshot 注入注释）。
	if wv == nil || wv.mainFrame == nil {
		return nil
	}
	return wv.mainFrame.RenderView()
}

// TopLayerRects 返回当前文档渲染层树中位于文档内容之上的浮层矩形
// （z-index>0 / fixed：遮罩/弹窗/toast/下拉）。应用层「外部合成内容」
// （画布预览挂件像素 blit）合成前查询：与浮层相交的区域不绘制，弹层
// 遮挡语义自动正确（引擎层提供层叠真相，应用无需 JS 探测弹窗）。
func (wv *WebView) TopLayerRects() []layout.LayoutRect {
	rv := wv.RenderView()
	if rv == nil {
		return nil
	}
	return rv.TopLayerRects()
}

// stickyHasInset 报告 sticky 元素是否带 top/bottom inset（CSS 语义：
// 无 inset 的 position:sticky 等同 relative，滚动时不钉住——CM6 的
// .cm-gutters 即此例，必须跟随滚动）。与 computeStickyOffset（渲染侧
// 绘制钉住判定）保持一致，保证 getBoundingClientRect 与绘制坐标同步。
func stickyHasInset(b *rendering.RenderBox) bool {
	if b == nil {
		return false
	}
	cs := b.Style()
	if cs == nil {
		return false
	}
	if t := cs.GetProperty("top"); t != "" && t != "auto" {
		return true
	}
	if btm := cs.GetProperty("bottom"); btm != "" && btm != "auto" {
		return true
	}
	return false
}

func (wv *WebView) EnsureLayout() {
	if wv.destroyed || wv.page == nil || wv.mainFrame == nil {
		return
	}
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() {
		view.Layout()
	}
	wv.syncIFrameSizes()
}

// EnsureHitTestReady 把渲染树与布局同步到最新状态（交互前调用：鼠标
// 按下/命中测试需要最新树，不能依赖渲染循环的批量重建——JS 改 DOM
// （弹窗 display 等）后的首个点击若用陈旧树命中会穿透到后方元素）。
// 通过 FlushRenderTreeDirty 忽略重建 cooldown；树无脏标记时零开销。
func (wv *WebView) EnsureHitTestReady() {
	if wv.destroyed || wv.mainFrame == nil {
		return
	}
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.FlushRenderTreeDirty()
	}
	wv.EnsureLayout()
}

// syncIFrameSizes 把主文档 iframe 元素的内容框尺寸同步到子 Frame，
// 尺寸变化时触发子文档重布局（iframe 是 replaced element，子文档内容
// 在其中渲染；paint 用内容框位置 + clip 绘制子 Frame 的 RenderView）。
func (wv *WebView) syncIFrameSizes() {
	rv := wv.RenderView()
	if rv == nil {
		return
	}
	page.ForEachIFrame(func(el *dom.Element, f *page.Frame) {
		if f == nil {
			return
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			return
		}
		st := box.Style()
		w, h := box.Width(), box.Height()
		if st != nil {
			// 内容框 = border-box 减 padding 与 border（iframe 子文档视口）。
			w -= iframePadLen(st.PaddingLeft) + iframePadLen(st.PaddingRight) +
				iframePadLen(st.BorderLeftWidth) + iframePadLen(st.BorderRightWidth)
			h -= iframePadLen(st.PaddingTop) + iframePadLen(st.PaddingBottom) +
				iframePadLen(st.BorderTopWidth) + iframePadLen(st.BorderBottomWidth)
		}
		if w < 0 {
			w = 0
		}
		if h < 0 {
			h = 0
		}
		fv := f.View()
		if fv != nil && (fv.Width() != int(w) || fv.Height() != int(h)) {
			fv.SetSize(int(w), int(h))
		}
	})
}

// iframePadLen 提取 style.Length 的像素值（auto/空为 0），与 rendering
// 包 lengthValue 等价——为避免跨包依赖在此内联。
func iframePadLen(l style.Length) float64 {
	if l.Unit == "auto" || l.Unit == "" && l.Value == 0 {
		return 0
	}
	return l.Value
}

func (wv *WebView) RebuildRenderTree() {
	if fr := wv.mainFrame.Frame(); fr != nil {
		fr.RebuildRenderTree()
	}
}

var (
	ErrNoDocument         = errors.New("webkit: no document loaded")
	ErrJavaScriptDisabled = errors.New("webkit: JavaScript is disabled")
	ErrDestroyed          = errors.New("webkit: WebView destroyed")
	ErrNotImplemented     = errors.New("webkit: not implemented")
)

// firstTextSegmentBase 返回 el 渲染子树中首个 RenderText 的文本段位置。
// inline 元素（display:inline 的 span，如 CM6 语法高亮 token）布局后
// box.frame 位置恒 0（文本由 TextSegment 定位，painter 画在 seg.X/seg.Y）——
// Range.getClientRects 对 span 内文本需要真实 x/y 基准，否则返回 (0,0)。
func firstTextSegmentBase(rv *rendering.RenderView, el *dom.Element) (float64, float64, bool) {
	ro := rv.FindRenderObjectForNode(el)
	if ro == nil {
		return 0, 0, false
	}
	var found *rendering.RenderText
	var walk func(rendering.RenderObject) bool
	walk = func(o rendering.RenderObject) bool {
		if o == nil {
			return false
		}
		if rt, ok := o.(*rendering.RenderText); ok && len(rt.Segments()) > 0 {
			found = rt
			return true
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(ro)
	if found == nil {
		return 0, 0, false
	}
	s := found.Segments()[0]
	return s.X, s.Y, true
}

// handleDynamicScript 处理**运行时插入文档**的 <script src>。
//
// 浏览器语义：脚本元素被插入文档（或已连接时设置 src）即取回并执行，
// 执行完派发 load（onload 回调），失败派发 error。此前引擎只在文档装配期
// 处理 <script>（page.Frame.executeInlineScripts），运行时 appendChild 的
// 脚本永不执行 —— gou-ide 的区域包 client 半全靠这个模式装载编译 bundle
// 并注册槽位（见 /plugins-assets/ui-titlebar/client.js），于是槽位表恒为空、
// 各区域全部停在「未装配」空态。
//
// 关键约束：appendChild 在 JS 执行栈内被调用，goja 不允许在 native 调用
// 栈内重入执行脚本，所以取回/执行**排队到事件循环**（与页面的 setTimeout
// 同一条队列，宿主每帧 ProcessTasks 驱动）。
// dynScript 是一条待处理的动态资源（元素 + 解析后的绝对 URL + 种类）。
// kind: "script"（<script src>）/ "stylesheet"（<link rel=stylesheet>）。
type dynScript struct {
	el   *dom.Element
	url  string
	kind string
}

// handleDynamicScript 处理**运行时插入文档**的 <script src>。
//
// 浏览器语义：脚本元素被插入文档即取回并执行，执行完派发 load（onload
// 回调），取回/执行失败派发 error。此前引擎只在文档装配期处理 <script>
// （page.Frame.executeInlineScripts），运行时 appendChild 的脚本永不执行 ——
// gou-ide 的区域包 client 半全靠这个模式装载编译 bundle 并注册槽位
// （见 /plugins-assets/ui-titlebar/client.js），于是槽位表恒为空、各区域
// 全部停在「未装配」空态。
//
// 关键约束：appendChild 在 JS 执行栈内被调用，goja 不允许在 native 调用
// 栈内重入执行脚本，所以这里只**入队**，由 flushDynamicScripts 在帧边界
// （Render 开头，JS 栈之外）取回并执行。
func (wv *WebView) handleDynamicScript(el *dom.Element, src string) {
	if el == nil || src == "" {
		return
	}
	if el.GetAttribute("data-wb-dynscript") == "1" {
		return // 幂等：重复 append 不重复执行
	}
	el.SetAttribute("data-wb-dynscript", "1")
	abs := dom.ResolveURL(wv.documentBaseURL(), src)
	wv.dynScriptMu.Lock()
	wv.dynScripts = append(wv.dynScripts, dynScript{el: el, url: abs, kind: "script"})
	wv.dynScriptMu.Unlock()
	if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "[dynscript] queued "+abs)
	}
}

// flushDynamicScripts 执行队列里的动态脚本（帧边界调用，见 handleDynamicScript）。
func (wv *WebView) flushDynamicScripts() {
	wv.dynScriptMu.Lock()
	pending := wv.dynScripts
	wv.dynScripts = nil
	wv.dynScriptMu.Unlock()
	for _, p := range pending {
		switch p.kind {
		case "stylesheet":
			wv.runDynamicStylesheet(p.el, p.url)
		default:
			wv.runDynamicScript(p.el, p.url)
		}
	}
}

// runDynamicScript 取回并执行一个动态 <script src>，按结果派发 load/error。
func (wv *WebView) runDynamicScript(el *dom.Element, abs string) {
	code, err := wv.loadExternalResource(abs, PurposeScript)
	if err != nil {
		if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "[dynscript] load FAIL "+abs+": "+err.Error())
		}
		bindings.FireResourceEvent(wv.JSInterpreter(), el, "error")
		return
	}
	if strings.TrimSpace(code) == "" {
		bindings.FireResourceEvent(wv.JSInterpreter(), el, "load")
		return
	}
	rt := wv.JSInterpreter()
	if rt == nil {
		return
	}
	// document.currentScript 语义（HTML §4.12.1）：脚本执行期间指向该元素
	// （与装配期 page.Frame.runScriptForElement 一致）。
	prev := bindings.CurrentScriptElement
	bindings.CurrentScriptElement = el
	_, err = rt.RunJS(code)
	bindings.CurrentScriptElement = prev
	if err != nil {
		bindings.FireResourceEvent(rt, el, "error")
		return
	}
	rt.RunJobs()
	bindings.FireResourceEvent(rt, el, "load")
}

// isStyleSheetLink 判断 <link> 是否为样式表链接（rel 含 stylesheet 关键字）。
func isStyleSheetLink(el *dom.Element) bool {
	if el == nil || el.LocalName() != "link" {
		return false
	}
	rel := strings.ToLower(el.GetAttribute("rel"))
	for _, part := range strings.Fields(rel) {
		if part == "stylesheet" {
			return true
		}
	}
	return false
}

// handleDynamicStylesheet 处理**运行时插入文档**的 <link rel=stylesheet>。
//
// 此前引擎只在装配期处理 <link>（page.Frame.extractAndAddStyles），运行时
// 插入的样式表永不生效 —— 插件包 CSS 正是这么加载的（client.js 里
// createElement('link') + rel/href + appendChild），于是插件区域全部按
// 无样式裸布局渲染（实测 ui-titlebar 的 height:40px 不生效，整条顶栏被
// 内容撑到 631px、logo 被撑成 512px）。
//
// 与动态脚本同一约束：入队、帧边界执行（goja 不允许 native 栈内重入）。
func (wv *WebView) handleDynamicStylesheet(el *dom.Element, href string) {
	if el == nil || href == "" {
		return
	}
	if el.GetAttribute("data-wb-dynstyle") == "1" {
		return // 幂等
	}
	el.SetAttribute("data-wb-dynstyle", "1")
	abs := dom.ResolveURL(wv.documentBaseURL(), href)
	wv.dynScriptMu.Lock()
	wv.dynScripts = append(wv.dynScripts, dynScript{el: el, url: abs, kind: "stylesheet"})
	wv.dynScriptMu.Unlock()
	if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "[dynstyle] queued "+abs)
	}
}

// runDynamicStylesheet 取回并接入一张运行时动态 <link rel=stylesheet>。
//
// ★ 接入方式：把取回的 CSS 文本写进一个**新建的 <style> 元素**插入
// <head>，而不是只走 Frame.AddStyleSheetFromText —— 后者在真实页面上
// 表现为「调试日志打印 applied，但样式完全不生效」（同一份 CSS 手工注入
// <style> 立即生效）。已排除的原因：CSS 语法（@keyframes / @media /
// [data-v-…] 属性选择器实测全部正确）、资源 MIME（text/css、无 nosniff）、
// 元素样式缓存（新建元素同样拿不到）、样式表被重扫移除（动态表已分离到
// Frame.dynamicStyleSheets）。
//
// <style> 通路是引擎自装配期起就在用的主通路（Frame.extractAndAddStyles），
// 与 <link> 语义等价：同为 author origin、同一级联顺序、同一组规则。实测
// 对同一批插件 CSS 立即生效（帮助菜单恢复 position:fixed + z-index:9999，
// 不再被标题栏祖先的 overflow:hidden 裁掉）。
func (wv *WebView) runDynamicStylesheet(el *dom.Element, abs string) {
	cssText, err := wv.loadExternalResource(abs, PurposeStylesheet)
	if err != nil {
		if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "[dynstyle] load FAIL "+abs+": "+err.Error())
		}
		bindings.FireResourceEvent(wv.JSInterpreter(), el, "error")
		return
	}
	// data-wb-dynstyle-src 便于在页面上核对「某外部表的文本确实进来了」。
	js := "(function(){var s=document.createElement('style');s.setAttribute('data-wb-dynstyle-src', " +
		strconv.Quote(abs) + ");s.textContent = " + strconv.Quote(cssText) +
		";document.head.appendChild(s);return 1})()"
	if _, err := wv.EvalJS(js); err != nil {
		if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "[dynstyle] style-inject FAIL "+abs+": "+err.Error())
		}
		bindings.FireResourceEvent(wv.JSInterpreter(), el, "error")
		return
	}
	if mf := wv.mainFrame; mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
	if os.Getenv("WB_DYNSCRIPT_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "[dynstyle] applied "+abs)
	}
	bindings.FireResourceEvent(wv.JSInterpreter(), el, "load")
}
