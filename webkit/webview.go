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
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/page"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
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
}

func NewWebView() *WebView {
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
	return wv
}

func (wv *WebView) Page() *page.Page          { return wv.page }
func (wv *WebView) MainFrame() *WebFrame       { return wv.mainFrame }
func (wv *WebView) Settings() *page.Settings    { return wv.settings }
func (wv *WebView) Width() int                 { return wv.width }
func (wv *WebView) Height() int                { return wv.height }

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
		bindings.RegisterDOMBindings(wv.jsInterpreter, wv.mainFrame.Document())
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
	return nil
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

func (wv *WebView) RenderView() *rendering.RenderView {
	return wv.mainFrame.RenderView()
}

func (wv *WebView) EnsureLayout() {
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() { view.Layout() }
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
