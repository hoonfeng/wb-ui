// Translation of: Source/WebKit/WebView.h
//                  Source/WebKit/WebView.cpp
//                  Source/WebKit/UIProcess/win/WebView.h
// Completeness: 40%
// Simplifications:
//   - single goroutine model (no multi-process); the WebContent/Network/UI split is
//     modeled in process.go as goroutine + channel abstractions only
//   - no network layer: HTML is loaded via LoadHTML string; LoadURL only supports a
//     "not implemented" placeholder and an optional local-file fallback
//   - no HWND / windowing system; Resize just updates the FrameView size
//   - no input events / IME / context menu / find-in-page / printing
//   - Render() does a full synchronous layout + paint into a fresh RGBA canvas and
//     returns the pixel bytes; there is no DisplayList / tiled backing store

package webkit

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/page"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
)

// DefaultWebViewWidth / DefaultWebViewHeight mirror the 800x600 initial size used by
// page.NewFrame's FrameView, so a freshly-created WebView can render without an
// explicit Resize call.
const (
	DefaultWebViewWidth  = 800
	DefaultWebViewHeight = 600
)

// WebView is the Go translation of the WebKit::WebView top-level embedding API
// (Source/WebKit/UIProcess/API/C/win/WKView.h, Source/WebKit/UIProcess/win/WebView.h).
// A WebView is the surface an embedder creates to load and display a web page. It
// owns the underlying page.Page, the WebFrame wrapping the main frame, and the
// settings applied at construction. In real WebKit the WebView is a UIProcess-side
// proxy that talks to a WebContent-side WebPage over IPC; this port collapses both
// halves into a single goroutine and exposes the same LoadHTML / Render / EvalJS
// surface.
type WebView struct {
	// page is the underlying WebCore::Page (mirroring WebView's m_page pointer).
	page *page.Page

	// mainFrame is the WebView-layer wrapper around page.MainFrame(), mirroring
	// the main WebFrame exposed via WebView::mainFrame().
	mainFrame *WebFrame

	// settings caches page.Settings() so that callers can mutate it without
	// re-fetching it on every call.
	settings *page.Settings

	// jsInterpreter is the JavaScript runtime backing EvalJS. It is created
	// lazily so a WebView that never runs scripts pays no parsing cost. When the
	// main frame's document is loaded, the DOM bindings are re-registered on the
	// interpreter so that document.getElementById etc. resolve to the new doc.
	jsInterpreter *jsc.Interpreter

	// jsLogger captures console output from scripts run via EvalJS, mirroring the
	// WebContent process's console message forwarding to the UIProcess. Exposed
	// via ConsoleOutput() so embedders / tests can read script output.
	jsLogger *jsc.BufferLogger

	// width / height mirror the FrameView's viewport dimensions used by Render.
	width  int
	height int
}

// NewWebView constructs a WebView with default settings and an 800x600 viewport,
// mirroring WebView::create on Windows (which builds a WebPageProxy with default
// configuration and an initial RECT). The returned WebView is ready for LoadHTML.
func NewWebView() *WebView {
	settings := page.NewSettings()
	p := page.NewPage(settings)
	wv := &WebView{
		page:     p,
		settings: settings,
		width:    DefaultWebViewWidth,
		height:   DefaultWebViewHeight,
	}
	wv.mainFrame = NewWebFrame(wv, p.MainFrame())

	// Set up the StyleSheetLoader on the main frame so that
	// <link rel="stylesheet" href="..."> elements can be resolved.
	if mf := p.MainFrame(); mf != nil {
		mf.StyleSheetLoader = func(href string) (string, error) {
			// For file:// URLs, strip the scheme and read the local file.
			filePath := href
			if strings.HasPrefix(filePath, "file://") {
				filePath = strings.TrimPrefix(filePath, "file://")
			}
			data, err := os.ReadFile(filePath)
			if err != nil {
				return "", fmt.Errorf("webkit: cannot load stylesheet %q: %w", href, err)
			}
			return string(data), nil
		}
	}
	return wv
}

// Page returns the underlying page.Page, mirroring WebView::page() / WebPageProxy.
func (wv *WebView) Page() *page.Page { return wv.page }

// MainFrame returns the WebFrame wrapping the page's main frame, mirroring
// WebView::mainFrame() (and the API-level WKPageGetMainFrame).
func (wv *WebView) MainFrame() *WebFrame { return wv.mainFrame }

// Settings returns the WebView's settings, mirroring WebPageProxy::settings().
func (wv *WebView) Settings() *page.Settings { return wv.settings }

// Width returns the current viewport width in CSS pixels.
func (wv *WebView) Width() int { return wv.width }

// Height returns the current viewport height in CSS pixels.
func (wv *WebView) Height() int { return wv.height }

// LoadHTML parses and loads an HTML source string into the main frame, mirroring
// the WebKit load entry point reached via WebPageProxy::loadHTMLString. It delegates
// to WebFrame.LoadHTML.
// LoadHTML parses an HTML source string, creates a Document, builds the
// render tree, and executes any inline <script> elements via the JS engine.
// This mirrors the WebKit load sequence where the parser emits script
// elements and the JS engine executes them before the first paint.
func (wv *WebView) LoadHTML(src string) error {
	// Set the ScriptEngine callback on the main frame so that inline scripts
	// are executed when the document is parsed.
	if wv.mainFrame != nil {
		wv.mainFrame.ScriptEngine = func(code string) error {
			_, err := wv.EvalJS(code)
			return err
		}
	}
	if err := wv.mainFrame.LoadHTML(src); err != nil {
		return err
	}
	// If the JS interpreter has been initialized already, rebind the new document
	// so subsequent EvalJS calls see the updated document object.
	if wv.jsInterpreter != nil && wv.mainFrame.Document() != nil {
		bindings.RegisterDOMBindings(wv.jsInterpreter, wv.mainFrame.Document())
	}
	return nil
}
// returns ErrNotImplemented.
func (wv *WebView) LoadURL(url string) error {
	src, err := fetchURL(url)
	if err != nil {
		return err
	}
	return wv.LoadHTML(src)
}

// Render executes a layout pass and paints the result into a fresh RGBA canvas,
// returning the pixel bytes, mirroring the painting path driven by
// WebView::onPaintEvent -> DrawingAreaProxy::paint -> WebPageProxy::drawRect.
// The returned slice is owned by the caller; the canvas backing it is discarded.
//
// The pixel data is row-major, top-row-first, with 4 bytes per pixel (R, G, B, A).
// Its length is width * height * 4 where width / height match the WebView's
// viewport size at the time of the call.
func (wv *WebView) Render() ([]byte, error) {
	rv := wv.mainFrame.RenderView()
	if rv == nil {
		return nil, ErrNoDocument
	}
	// Layout pass: ensure the FrameView is up to date before painting.
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() {
		view.Layout()
	}
	width := wv.width
	height := wv.height
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	canvas := graphics.NewCanvas(width, height)
	dirtyRect := graphics.Rect{X: 0, Y: 0, Width: float64(width), Height: float64(height)}
	rendering.Paint(rv, canvas, dirtyRect)
	return canvas.Pixels(), nil
}

// Resize updates the WebView's viewport dimensions and propagates the new size to
// the main frame's FrameView, mirroring WebView::onSizeEvent -> WebPageProxy::
// setViewNeedsDisplay / DrawingAreaProxy::setSize. The next Render call will lay
// out against the new viewport.
func (wv *WebView) Resize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	wv.width = width
	wv.height = height
	view := wv.page.MainFrame().View()
	if view != nil {
		view.SetSize(width, height)
	}
}

// EvalJS evaluates a JavaScript source string in the WebView's scripting context,
// mirroring WebPageProxy::runJavaScriptInMainFrameScriptWorld. The DOM bindings are
// registered so that document.getElementById etc. resolve to the current document.
//
// The returned JSValue mirrors the completion value semantics of an HTML <script>
// element: it is the value of the last evaluated expression statement, or undefined
// when the script performs only declarations / control flow. Scripts that need to
// surface a result to the embedder should write to console.log and read it back via
// ConsoleOutput(), mirroring how WebKit forwards console messages to the UIProcess.
func (wv *WebView) EvalJS(script string) (jsc.JSValue, error) {
	if !wv.settings.JavaScriptEnabled {
		return jsc.Undefined(), ErrJavaScriptDisabled
	}
	wv.ensureJSRuntime()
	doc := wv.mainFrame.Document()
	if doc != nil {
		// Registering twice with the same interpreter just re-installs the
		// document global; calling it before each EvalJS keeps things in sync
		// after a LoadHTML call.
		bindings.RegisterDOMBindings(wv.jsInterpreter, doc)
	}
	result, err := wv.jsInterpreter.Run(script)
	if err != nil {
		return jsc.Undefined(), fmt.Errorf("webkit: JS evaluation failed: %w", err)
	}
	return result, nil
}

// ConsoleOutput returns the accumulated console output produced by scripts run via
// EvalJS, mirroring how WebKit's UIProcess receives forwarded console messages from
// the WebContent process. The output is preserved across calls; use ResetConsole to
// clear it.
func (wv *WebView) ConsoleOutput() string {
	if wv.jsLogger == nil {
		return ""
	}
	return wv.jsLogger.String()
}

// ResetConsole clears the accumulated console output, mirroring the UIProcess-side
// console-message buffer reset that happens on a new navigation.
func (wv *WebView) ResetConsole() {
	if wv.jsLogger == nil {
		return
	}
	wv.jsLogger.Lines = nil
}

// ensureJSRuntime lazily constructs the JS interpreter and installs the built-in
// globals (console, Math, JSON, constructors) so that scripts can use the standard
// library. The console is backed by an internal BufferLogger exposed via
// ConsoleOutput().
func (wv *WebView) ensureJSRuntime() {
	if wv.jsInterpreter != nil {
		return
	}
	wv.jsInterpreter = jsc.NewInterpreter()
	wv.jsLogger = &jsc.BufferLogger{}
	wv.jsInterpreter.SetupGlobal(wv.jsLogger)
}

// JSInterpreter returns the JavaScript interpreter backing EvalJS, constructing
// it lazily if it has not yet been initialized. Exposed so embedders can register
// Go functions via bindings.RegisterGoFunction before EvalJS.
func (wv *WebView) JSInterpreter() *jsc.Interpreter {
	wv.ensureJSRuntime()
	return wv.jsInterpreter
}

// Document returns the main frame's current DOM document, or nil if no document
// has been loaded yet. Exposed so embedders can query/manipulate the DOM from Go.
func (wv *WebView) Document() *dom.Document {
	return wv.mainFrame.Document()
}

// RenderView returns the root RenderView of the main frame, or nil if no document
// has been loaded. Exposed so embedders can perform hit-testing on the render tree.
func (wv *WebView) RenderView() *rendering.RenderView {
	return wv.mainFrame.RenderView()
}

// EnsureLayout performs a layout pass if the frame view needs one, mirroring
// the FrameView::layout() call that precedes every paint in WebKit. Call this
// before rendering.Paint to ensure the render tree has up-to-date geometry.
func (wv *WebView) EnsureLayout() {
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() {
		view.Layout()
	}
}

// RebuildRenderTree reconstructs the render tree from the current DOM and marks
// layout as pending. Embedders should call this after mutating the DOM (e.g.
// from a click handler or EvalJS) so the next EnsureLayout + Paint reflects the
// changes. This mirrors the style-recalc + attach phase in WebKit that fires
// after DOM mutations outside of parsing.
func (wv *WebView) RebuildRenderTree() {
	if wv.mainFrame != nil && wv.mainFrame.frame != nil {
		wv.mainFrame.frame.RebuildRenderTree()
	}
}

// RenderToCanvas executes a layout pass and paints the result into a Canvas (which
// is returned to the caller), mirroring the internal path of Render but without
// discarding the Canvas. The caller is responsible for releasing the Canvas when
// done.
func (wv *WebView) RenderToCanvas() (*graphics.Canvas, error) {
	rv := wv.mainFrame.RenderView()
	if rv == nil {
		return nil, ErrNoDocument
	}
	view := wv.page.MainFrame().View()
	if view != nil && view.NeedsLayout() {
		view.Layout()
	}
	width := wv.width
	height := wv.height
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	canvas := graphics.NewCanvas(width, height)
	dirtyRect := graphics.Rect{X: 0, Y: 0, Width: float64(width), Height: float64(height)}
	rendering.Paint(rv, canvas, dirtyRect)
	return canvas, nil
}

// ErrNoDocument is returned by Render when no document has been loaded yet.
var ErrNoDocument = errors.New("webkit: no document loaded")

// ErrNotImplemented is returned by stubbed-out entry points (e.g. LoadURL for
// non-file URLs) where the equivalent WebKit code path requires infrastructure
// this port has not translated.
var ErrNotImplemented = errors.New("webkit: not implemented")

// ErrJavaScriptDisabled is returned by EvalJS when Settings.JavaScriptEnabled is
// false, mirroring WebPageProxy::runJavaScriptInMainFrameScriptWorld's early return
// when scripting is disabled.
var ErrJavaScriptDisabled = errors.New("webkit: JavaScript disabled")
