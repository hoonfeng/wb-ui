// Translation of: Source/WebKit/WebView.h
//                  Source/WebKit/WebView.cpp
//                  Source/WebKit/UIProcess/win/WebView.h
// Completeness: 40%

package webkit

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"

	"wb-ui/bindings"
	"wb-ui/bridge"
	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/layout"
	"wb-ui/page"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

const (
	DefaultWebViewWidth  = 800
	DefaultWebViewHeight = 600
)

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
	hoverCapability  string
	pointerCapability string

	// currentURL 是主文档的加载 URL（LoadURL 设置）。iframe 相对路径 src
	// （如 src="page.html"）依赖它做基准解析（WebKit completeURL 语义）。
	currentURL string
	// subframeJS 为每个 iframe 子 Frame 维护独立的 JS 全局环境（浏览器
	// iframe 语义：子文档有自己的 window/document，与父文档互不干扰）。
	subframeJS map[*page.Frame]*jsc.Interpreter
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

func NewWebView() *WebView {
	ensureFonts()
	settings := page.NewSettings()
	p := page.NewPage(settings)
	wv := &WebView{
		page: p, settings: settings,
		width: DefaultWebViewWidth, height: DefaultWebViewHeight,
	}
	wv.mainFrame = NewWebFrame(wv, p.MainFrame())
	if mf := p.MainFrame(); mf != nil {
		mf.StyleSheetLoader = func(href string) (string, error) {
			// Support http(s), file, and data URLs
			if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "data:") {
				return fetchURL(href)
			}
			fp := strings.TrimPrefix(href, "file://")
			d, e := os.ReadFile(fp)
			if e != nil { return "", fmt.Errorf("load stylesheet %q: %w", href, e) }
			return string(d), nil
		}
		mf.ScriptLoader = func(src string) (string, error) {
			// Support http(s), file, and data URLs
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") {
				return fetchURL(src)
			}
			fp := strings.TrimPrefix(src, "file://")
			d, e := os.ReadFile(fp)
			if e != nil { return "", fmt.Errorf("load script %q: %w", src, e) }
			return string(d), nil
		}
	}
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
	bindings.IFrameSrcChanged = wv.handleIFrameSrcChanged
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
	abs := resolveIframeSrc(src, wv.currentURL)
	// 旧子 Frame 卸载：注销注册表 + 清理其 JS 环境。
	if old := page.IFrameFrame(el); old != nil {
		page.UnregisterIFrame(el)
		delete(wv.subframeJS, old)
	}
	if abs == "" {
		page.Logf("IFrame", "src change: unresolvable %q, unloaded", src)
		return
	}
	wv.loadSubframe(el, abs)
}

func (wv *WebView) Page() *page.Page          { return wv.page }
func (wv *WebView) MainFrame() *WebFrame       { return wv.mainFrame }
func (wv *WebView) Settings() *page.Settings    { return wv.settings }
func (wv *WebView) Width() int                 { return wv.width }
func (wv *WebView) Height() int                { return wv.height }

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

// BeforePageScripts is an optional hook invoked after DOM bindings are
// registered but before any page <script> executes. The `window` global
// object exists at this point (it is created by RegisterDOMBindings), so
// embedders can inject page-environment JS (e.g. fetch interception for
// desktop mode) that must be visible to the application code.
var BeforePageScripts func(rt *jsc.Interpreter)

func (wv *WebView) LoadHTML(src string) error {
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
	page.RegisterFetch(wv.jsInterpreter)
	page.RegisterXMLHttpRequest(wv.jsInterpreter)
	bridge.InjectAll(wv.jsInterpreter)

	// Inject the bridge SDK script as inline JS before any page scripts.
	// This SDK wraps fetch() to intercept registered API routes.
	if sdk := bridge.InjectSDK(); sdk != "" {
		if _, err := wv.jsInterpreter.RunJS(sdk); err != nil {
			fmt.Fprintf(os.Stderr, "[wb-ui] bridge SDK injection failed: %v\n", err)
		}
	}

	if err := wv.mainFrame.LoadHTML(src); err != nil {
		return err
	}
	// DOM bindings MUST be registered BEFORE executing page scripts so that
	// JS frameworks (Vue/React) have access to document.getElementById,
	// querySelector, Element.appendChild, etc. at boot time.
	if wv.jsInterpreter != nil && wv.mainFrame.Document() != nil {
		// matchMedia 需要真实视口上下文（尺寸随 wv 变化、颜色方案随
		// SetPrefersColorScheme 设置；指针能力暂用默认值）。
		bindings.MediaQueryContextProvider = func() *css.MediaQueryContext {
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
				DevicePixelRatio: 1, Orientation: orientation,
				PrefersColorScheme: wv.PrefersColorScheme(),
				Hover:              wv.HoverCapability(), AnyHover: wv.HoverCapability(),
				Pointer: wv.PointerCapability(), AnyPointer: wv.PointerCapability(),
			}
		}
		bindings.RegisterDOMBindings(wv.jsInterpreter, wv.mainFrame.Document())
		// ★ 渲染树几何桥：Element.scrollTop/scrollHeight/clientHeight/offsetHeight/
		//   getBoundingClientRect 等 CSSOM 属性需要真实布局几何。此前只在 EvalJS
		//   中注入——cmd/desktop 与页面脚本（Vue）均走 JSInterpreter().RunJS 执行，
		//   从不经过 EvalJS → hook 保持 nil → 前端读到 0：聊天列表无法按空间
		//   加载（clientHeight/scrollHeight 恒 0）、scrollTop 赋值静默失效。
		//   在 LoadHTML 注册 DOM bindings 后、页面脚本执行前注入（渲染树可用）。
		wv.injectRenderTreeBridge()
		// Set up callback for dynamic <style> injection (Vue scoped CSS).
		// Uses dirty-flag batching: the rebuild is deferred to the next layout.
		bindings.OnStyleNodeAdded = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
		}
		// Set up callback for inline style changes (el.style.xxx = ...).
		// ★ 增量优先：纯样式变更（拖拽 sidebar 宽度、range 拖动等）只更新
		//   目标元素的 ComputedStyle + SetNeedsLayout（relayout 不重建树）。
		//   此前无条件 MarkRenderTreeDirty → 每帧 RebuildRenderTree 全量
		//   重建（复杂页面 30ms+）→ 拖拽卡顿/窗口无响应（「频繁无响应」
		//   根因）。结构属性（display/position/float/clear）变化才回退全量。
		bindings.OnInlineStyleChanged = func(n dom.Node) {
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
		// Set up callbacks for DOM mutations (appendChild / removeChild / etc.).
		// Uses dirty-flag batching: the rebuild is deferred to the next layout.
		bindings.OnNodeInserted = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
		}
		bindings.OnNodeRemoved = func(n dom.Node) {
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
	// iframe 子文档：主文档加载完成后，为带 src 的 <iframe> 创建子 Frame
	// 并加载（WebKit: FrameLoader 在解析到 iframe 元素时创建子 Frame）。
	// 子 Frame 拥有独立 ScriptEngine（独立 JS 全局环境），子文档脚本
	// 可执行；布局与绘制见 syncIFrameSizes / PaintIFrame。
	wv.loadIFrameDocuments()
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
				abs := resolveIframeSrc(src, wv.currentURL)
				if abs == "" {
					page.Logf("IFrame", "skip unresolvable src=%q base=%q", src, wv.currentURL)
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
	// 子文档的 <script> 经独立 JS 全局环境执行（见 makeSubframeScriptEngine）。
	f.ScriptEngine = wv.makeSubframeScriptEngine(f)
	f.ScriptLoader = func(src string) (string, error) {
		abs := resolveIframeSrc(src, absSrc)
		if abs == "" {
			abs = src
		}
		if strings.HasPrefix(abs, "http://") || strings.HasPrefix(abs, "https://") || strings.HasPrefix(abs, "data:") {
			return fetchURL(abs)
		}
		fp := strings.TrimPrefix(abs, "file://")
		d, e := os.ReadFile(fp)
		if e != nil {
			return "", fmt.Errorf("load subframe script %q: %w", abs, e)
		}
		return string(d), nil
	}
	f.StyleSheetLoader = func(href string) (string, error) {
		abs := resolveIframeSrc(href, absSrc)
		if abs == "" {
			abs = href
		}
		if strings.HasPrefix(abs, "http://") || strings.HasPrefix(abs, "https://") || strings.HasPrefix(abs, "data:") {
			return fetchURL(abs)
		}
		fp := strings.TrimPrefix(abs, "file://")
		d, e := os.ReadFile(fp)
		if e != nil {
			return "", fmt.Errorf("load subframe stylesheet %q: %w", abs, e)
		}
		return string(d), nil
	}
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

func (wv *WebView) LoadURL(url string) error {
	wv.currentURL = url
	src, err := fetchURL(url)
	if err != nil {
		return err
	}
	return wv.LoadHTML(src)
}

func (wv *WebView) Render() ([]byte, error) {
	rv := wv.mainFrame.RenderView()
	if rv == nil { return nil, ErrNoDocument }
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() { view.Layout() }
	canvas := graphics.NewCanvas(wv.width, wv.height)
	dirtyRect := graphics.Rect{X: 0, Y: 0, Width: float64(wv.width), Height: float64(wv.height)}
	rendering.Paint(rv, canvas, dirtyRect)
	return canvas.Pixels(), nil
}

func (wv *WebView) Resize(width, height int) {
	if width < 0 { width = 0 }
	if height < 0 { height = 0 }
	wv.width, wv.height = width, height
	// ★ 同步 window.innerWidth/innerHeight（CM6 visiblePixelRange 依赖；
	// undefined 会让 Math.min(win.innerHeight,…) 产生 NaN → viewport 永不
	// 更新 → 滚动后行号 gutter 不重渲染）
	bindings.ViewportWidth = float64(width)
	bindings.ViewportHeight = float64(height)
	if view := wv.page.MainFrame().View(); view != nil {
		view.SetSize(width, height)
	}
}

func (wv *WebView) EvalJS(script string) (jsc.JSValue, error) {
	if !wv.settings.JavaScriptEnabled {
		return jsc.Undefined(), ErrJavaScriptDisabled
	}
	wv.ensureJSRuntime()
	if doc := wv.mainFrame.Document(); doc != nil {
		bindings.RegisterDOMBindings(wv.jsInterpreter, doc)
		// ★ 渲染树桥：Element.scrollTop/scrollLeft/scrollHeight/scrollWidth/
		//   clientHeight/clientWidth/offsetHeight/offsetWidth 等 CSSOM 属性需要
		//   真实布局几何。前端（Vue 聊天列表 scrollToBottom 等）依赖
		//   el.scrollTop = el.scrollHeight，此前未实现 → 滚动 API 静默失效。
		//   每次注册 DOM bindings 时重新注入（渲染树可能已重建）。
		wv.injectRenderTreeBridge()
		// Ensure callback for dynamic <style> injection.
		bindings.OnStyleNodeAdded = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.RebuildRenderTree()
			}
		}
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
	if wv.jsLogger == nil { return "" }
	return wv.jsLogger.String()
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
	if wv.jsLogger == nil { return }
	wv.jsLogger.Lines = nil
}

func (wv *WebView) ensureJSRuntime() {
	if wv.jsInterpreter != nil { return }
	wv.jsInterpreter = jsc.NewInterpreter()
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
	return wv.mainFrame.Document()
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
	bindings.GetElementScrollOffset = func(el *dom.Element) (float64, float64) {
		return wrapBox(el, func(box *rendering.RenderBox) (float64, float64) {
			if rv := wv.RenderView(); rv != nil {
				return rv.BoxScrollOffset(box)
			}
			return 0, 0
		})
	}
	bindings.SetElementScrollOffset = func(el *dom.Element, x, y float64) {
		forceLayout()
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
		vm := rendering.VerticalScrollbarMetrics(rv, box)
		hm := rendering.HorizontalScrollbarMetrics(rv, box)
		if !vm.OK && !hm.OK {
			return
		}
		if vm.OK {
			if y < 0 {
				y = 0
			}
			if y > vm.MaxScroll {
				y = vm.MaxScroll
			}
		} else {
			y = 0
		}
		if hm.OK {
			if x < 0 {
				x = 0
			}
			if x > hm.MaxScroll {
				x = hm.MaxScroll
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
			el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
		}
	}
	bindings.GetElementScrollMetrics = func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool) {
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
		vm := rendering.VerticalScrollbarMetrics(rv, box)
		hm := rendering.HorizontalScrollbarMetrics(rv, box)
		return pb.Width, pb.Height, tw, th, (vm.OK || hm.OK)
	}
	bindings.GetElementBoxRect = func(el *dom.Element) (left, top, width, height float64) {
		forceLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			return 0, 0, 0, 0
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			// ★ DOM 变更后渲染树可能尚未重建（treehook 下帧才重建）——
			// offsetHeight/offsetWidth 此时返回 0 → xterm 初始化测量缓存
			// NaN → style.height="NaNpx" → 行高异常（310px）→ 终端内容
			// 画到视口外。强制重建一次再查。⚠️ 直接 RebuildRenderTree
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
		if box == nil {
			return 0, 0, 0, 0
		}
		w, h := box.Width(), box.Height()
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
		// ★ 沿 DOM 祖先链查找滚动容器（渲染树 Parent 链在 CM6 scroller
		// 结构下 Node 查找不可靠——同 Node 指针 BoxScrollOffset 结果不一
		// 致，疑似 box 实例字段差异；DOM 链 + FindRenderBoxForNode 每次
		// 命中同一 box，已验证返回正确偏移）。
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
		stickySeen := stickyHasInset(box)
		for cur := el.ParentNode(); cur != nil; cur = cur.ParentNode() {
			if el2, ok := cur.(*dom.Element); ok {
				if b := rv.FindRenderBoxForNode(el2); b != nil {
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
			}
		}
		return box.X() - sx, box.Y() - sy, w, h
	}
	// Range.getClientRects 文本测量需要元素 computed 字体（CodeMirror 6
	// 的 charWidth/lineHeight 探测；缺 createRange/字体时测量抛异常，
	// HeightOracle 停留默认 14 → 行号栏按 14px/行步进与内容 18.2px 错位）。
	bindings.GetElementComputedFont = func(el *dom.Element) (string, float64, int, string) {
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
	return wv.mainFrame.RenderView()
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
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() { view.Layout() }
	wv.syncIFrameSizes()
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
		if w < 0 { w = 0 }
		if h < 0 { h = 0 }
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
	ErrNotImplemented     = errors.New("webkit: not implemented")
)
