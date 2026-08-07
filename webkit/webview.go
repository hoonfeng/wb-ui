// Translation of: Source/WebKit/WebView.h
//                  Source/WebKit/WebView.cpp
//                  Source/WebKit/UIProcess/win/WebView.h
// Completeness: 40%

package webkit

import (
	"errors"
	"fmt"
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
	return wv
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
		bindings.OnInlineStyleChanged = func(n dom.Node) {
			if fr := wv.mainFrame.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
				fr.SetNeedsLayout(true)
			}
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
	// 子 Frame 无 ScriptEngine → 子文档脚本不执行（静默渲染）；布局与
	// 绘制见 syncIFrameSizes / PaintIFrame。
	wv.loadIFrameDocuments()
	return nil
}

// loadIFrameDocuments 扫描主文档中的 <iframe> 元素，为每个有 src 且尚未
// 注册子 Frame 的元素创建子 Frame 并加载子文档（fetchURL 支持 http/https/
// file/data）。相对路径 src 先跳过（无父文档 URL 基准，与 WebKit 的
// completeURL 行为有差距，留待后续）。
func (wv *WebView) loadIFrameDocuments() {
	doc := wv.mainFrame.Document()
	if doc == nil {
		return
	}
	page.PruneIFrames(doc)
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "iframe" {
			src := el.GetAttribute("src")
			if src != "" && page.IFrameFrame(el) == nil {
				data, err := fetchURL(src)
				if err == nil {
					f := page.NewFrame(wv.page)
					// 默认 300x150（iframe 替换元素默认尺寸），
					// EnsureLayout 后由 syncIFrameSizes 校正。
					f.View().SetSize(300, 150)
					if ferr := f.LoadHTML(data); ferr == nil {
						f.RebuildRenderTree()
						page.RegisterIFrame(el, f)
					}
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc)
}

func (wv *WebView) LoadURL(url string) error {
	src, err := fetchURL(url)
	if err != nil { return err }
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
	}
	bindings.GetElementScrollMetrics = func(el *dom.Element) (viewW, viewH, totalW, totalH float64, scrollable bool) {
		forceLayout()
		rv := wv.RenderView()
		if rv == nil || el == nil {
			return 0, 0, 0, 0, false
		}
		box := rv.FindRenderBoxForNode(el)
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
			return 0, 0, 0, 0
		}
		return box.X(), box.Y(), box.Width(), box.Height()
	}
}

func (wv *WebView) RenderView() *rendering.RenderView {
	return wv.mainFrame.RenderView()
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
