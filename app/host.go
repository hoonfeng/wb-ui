// Package app provides a high-level application host that ties together a
// platform window (GLFW + Skia GPU surface) and a webkit.WebView, running the
// render + event loop so embedders can focus on page logic instead of GL
// plumbing.
//
// This mirrors the embedding layer that real WebKit splits between
// WebView (page logic) and the platform Window/HostWindow (GL + event pump).
// Keeping the GPU/render/event loop inside the library means example
// programs and downstream embedders no longer have to recreate the
// Surface→Paint→Present→HitTest pipeline by hand.
package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/page"
	"wb-ui/platform/graphics"
	"wb-ui/platform/ime"
	"wb-ui/platform/window"
	"wb-ui/rendering"
	"wb-ui/style"
	"wb-ui/webkit"
)

var DumpRTCallback func(rv *rendering.RenderView)

// debugPaintLog enables verbose paint and event diagnostics printed to stderr.
// Set to true to trace hover, click, and paint operations.
var debugPaintLog = os.Getenv("WB_HOVER_DEBUG") != ""

// ClickHandler is invoked when the user clicks an element whose onclick
// attribute does not use the "js:" prefix. el is the deepest hit-tested
// element with an onclick attribute (may be nil if nothing was hit), and
// onclick is the raw value of the element's onclick attribute (empty when
// absent). clickX / clickY are the hit point in render-tree CSS pixels
// (already scroll-adjusted), useful for positioning IME composition windows.
// Embedders typically dispatch to registered Go handlers based on onclick
// and manage IME focus from here.
type ClickHandler func(el *dom.Element, onclick string, clickX, clickY float64)

// IMEHandler is invoked each frame with any IME events that arrived for the
// currently focused editable element. The handler is only called while an
// element is focused via FocusElement.
type IMEHandler func(events []ime.Event)

// Host is the top-level application host: it owns a platform.Window (GLFW +
// Skia GPU surface) and drives a webkit.WebView render/event loop. Embedders
// create a WebView, load HTML, register Go functions, then hand everything to
// NewHost and call Run.
type Host struct {
	win *window.Window
	wv  *webkit.WebView

	// clickHandler dispatches non-js: onclick values to embedder code.
	clickHandler ClickHandler
	// imeHandler receives IME events for the focused element.
	imeHandler IMEHandler

	// IME focus state. When imeFocusedEl is non-nil, incoming IME events are
	// applied to it: composition updates append a preview, char input appends
	// confirmed text, and composition end finalizes. The embedder can read
	// FocusedElement to know which element is receiving input.
	imeFocusedEl   *dom.Element
	imeInputText   string
	imeComposing   bool
	imeComposeText string
	// Composition insertion state: base is the element text WITHOUT the
	// in-progress composition, start is the caret offset where the composition
	// (and subsequent char input) is inserted. Without this, IME text was
	// always appended to the END of the value, ignoring the caret position —
	// clicking mid-text then typing put the new text at the tail.
	imeComposeBase  string
	imeComposeStart int

	// animStart is the wall-clock time when Run() started, used to compute
	// the animation clock (AnimationTime) each frame.
	animStart time.Time

	// needsResizeDump is set true on EventResize, cleared after DumpRTCallback fires
	// once on the re-laid-out tree. Prevents dumping every frame.
	needsResizeDump bool
	// lastLoggedScrollY dedupes per-frame [scroll] logs (only logs on change).
	lastLoggedScrollY int
	// lastIMEX/lastIMEY dedupe IME composition-position updates.
	lastIMEX, lastIMEY int32

	// Selection state. Text selection is tracked as CSS-pixel coordinates
	// (not RenderText pointers) so it survives render tree rebuilds. Each
	// frame, updateSelection converts these coordinates into a
	// rendering.Selection against the current render tree.
	clickCount     int                       // consecutive clicks (1-4)
	selGranularity rendering.TextGranularity // current selection granularity
	selAnchorX     float64                   // selection anchor (fixed start point)
	selAnchorY     float64
	selStartX      float64 // start position (= anchor unless shift+click)
	selStartY      float64
	selEndX        float64 // end position (drag/shift+click target)
	selEndY        float64
	selecting      bool    // mouse button held during drag
	shiftSelecting bool    // shift+click extending selection
	mouseDownX     float64 // press position for hysteresis
	mouseDownY     float64
	hysteresisMet  bool // drag threshold (3px) exceeded
	lastClickTime  time.Time
	lastClickX     float64
	lastClickY     float64

	// cursorX, cursorY track the last known cursor position (from mouse
	// move events), used for hit-testing on scroll events.
	cursorX, cursorY float64

	// hoveredEl tracks the element currently under the mouse cursor.
	// On each mouse-move, HitTest locates the deepest element and updates
	// its IsHovered state accordingly. This enables :hover pseudo-class
	// matching in the style resolver.
	hoveredEl *dom.Element
	// activeEl tracks the element being pressed (mousedown → :active).
	// Cleared on mouseup. Enables :active pseudo-class matching.
	activeEl *dom.Element

	// scrollbarDrag tracks an active scrollbar thumb drag.
	scrollbarDragging bool
	// scrollbarDragBox is the scroll container being dragged.
	scrollbarDragBox *rendering.RenderBox
	// scrollbarDragAxis: true = vertical, false = horizontal.
	scrollbarDragAxis bool
	// scrollbarDragStartY is the cursor Y at drag start (CSS pixels).
	scrollbarDragStart float64
	// scrollbarDragOffsetY is the scroll offset at drag start.
	scrollbarDragScroll float64

	// caretBlinkTime tracks the last caret visibility toggle for blinking.
	caretBlinkTime time.Time

	// frameView is the page's FrameView, cached for scroll operations in
	// processEvents (where Run's local variable is out of scope).
	frameView *page.FrameView
}

// NewHost creates a Host that drives the given WebView inside a new platform
// window of the given CSS-pixel dimensions. The window is created with DPI
// awareness so width/height are treated as logical (CSS) pixels and scaled to
// physical pixels internally. The caller should call Run to start the loop.
func NewHost(wv *webkit.WebView, width, height int, title string) (*Host, error) {
	if wv == nil {
		return nil, fmt.Errorf("app: WebView is nil")
	}
	// Initialize font manager if not already done. First try bundled resources,
	// then load from system fonts (C:\Windows\Fonts on Windows).
	if graphics.GetFontManager() == nil {
		fontDir := findFontDir()
		_ = graphics.InitFontManager(fontDir)
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	// Bridge Skia font metrics to the layout engine so inline text measurement
	// uses real glyph widths instead of fallback estimates.
	layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	wv.Resize(width, height)
	win, err := window.NewWindow(width, height, title)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	return &Host{win: win, wv: wv}, nil
}

// findFontDir searches for the wb-ui bundled font resources directory.
// Tries several common locations relative to the executable and working dir.
func findFontDir() string {
	candidates := []string{
		"resources/fonts",
		filepath.Join("..", "resources", "fonts"),
		filepath.Join("F:\\syproject\\wb-ui", "resources", "fonts"),
	}
	// Try relative to the executable.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(exeDir, "resources", "fonts"),
			filepath.Join(exeDir, "..", "..", "resources", "fonts"),
			filepath.Join(exeDir, "..", "..", "..", "wb-ui", "resources", "fonts"),
		}, candidates...)
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return "resources/fonts"
}

// SetClickHandler installs the callback invoked for non-js: onclick hits.
// js:-prefixed onclick values are executed via WebView.EvalJS directly by
// the Host and are not forwarded to this handler.
func (h *Host) SetClickHandler(fn ClickHandler) { h.clickHandler = fn }

// SetIMEHandler installs the callback invoked with IME events while an
// element is focused via FocusElement.
func (h *Host) SetIMEHandler(fn IMEHandler) { h.imeHandler = fn }

// Window returns the underlying platform window. Exposed so embedders can
// query content scale, framebuffer size, or install custom GLFW callbacks
// if needed.
func (h *Host) Window() *window.Window { return h.win }

// WebView returns the WebView driven by this host.
func (h *Host) WebView() *webkit.WebView { return h.wv }

// FocusElement marks el as the current IME focus target and enables IME
// input on the platform window. Incoming IME events will be applied to el
// until Unfocus is called or another element is focused. Call this from a
// ClickHandler when the user clicks an editable element (e.g. <input>).
//
// For <input> and <textarea> elements, the text is read from / written to the
// "value" attribute (mirroring WebCore::HTMLTextFormControlElement::value());
// for other elements, text content is used (the legacy div-based editable
// element behavior).
func (h *Host) FocusElement(el *dom.Element) {
	h.FocusElementByKeyboard(el, false)
}

// FocusElementByKeyboard is FocusElement with a focus-source hint. byKeyboard
// should be true when focus moved via Tab (keyboard); false for mouse clicks.
// It drives the :focus-visible pseudo-class: the UA default outline only
// matches keyboard focus, so clicking a button/tab no longer draws the
// (browser-mismatched) focus ring.
func (h *Host) FocusElementByKeyboard(el *dom.Element, byKeyboard bool) {
	if h.imeFocusedEl != nil && h.imeFocusedEl != el {
		h.imeFocusedEl.SetFocused(false)
	}
	if el != nil {
		el.SetFocused(true)
		el.SetFocusByKeyboard(byKeyboard)
	}
	h.imeFocusedEl = el
	h.imeComposing = false
	h.imeComposeText = ""
	h.imeComposeBase = ""
	h.imeComposeStart = 0
	// Mark frame dirty so :focus style updates.
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
	}
	if el != nil {
		h.imeInputText = focusedElementValue(el)
	}
	h.win.SetIMEEnabled(true)
	// For text-type form controls (<input>/<textarea>), register the element
	// with the rendering package so paintTextInputValue draws a blinking caret.
	// These are replaced elements with no RenderText children, so the regular
	// CaretPos/PaintCaret path (which targets RenderText segments) cannot
	// locate them.
	if el != nil && isTextFormControl(el) {
		rendering.FocusedFormControl = el
	} else {
		rendering.FocusedFormControl = nil
	}
}

// focusedElementValue returns the current text of a focused element. For
// <input> it reads the "value" attribute; for <textarea> it reads textContent;
// for other elements it reads textContent. Mirrors the value() accessor on
// HTMLTextFormControlElement.
func focusedElementValue(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if el.LocalName() == "textarea" {
		return el.TextContent()
	}
	if isTextFormControl(el) {
		return el.GetAttribute("value")
	}
	return el.TextContent()
}

// setFocusedElementValue writes the text back to a focused element. For
// <input> it sets the "value" attribute; for <textarea> it sets textContent;
// for other elements it sets textContent.
func setFocusedElementValue(el *dom.Element, text string) {
	if el == nil {
		return
	}
	if el.LocalName() == "textarea" {
		el.SetTextContent(text)
		return
	}
	if isTextFormControl(el) {
		el.SetAttribute("value", text)
		return
	}
	el.SetTextContent(text)
}

// isTextFormControl reports whether el is an <input> (non-checkbox/radio/
// hidden/range/color/file/submit/reset/button/image) or <textarea>, i.e. a
// form control whose text is carried by the value attribute and which
// accepts text entry via IME. Mirrors HTMLTextFormControlElement::childShouldCreateRenderer.
func isTextFormControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "textarea":
		return true
	case "input":
		t := el.GetAttribute("type")
		switch strings.ToLower(t) {
		case "checkbox", "radio", "range", "color", "file",
			"submit", "reset", "button", "image", "hidden":
			return false
		}
		return true
	default:
		return false
	}
}

// isFocusableElement 报告元素是否可聚焦（浏览器语义）。
// 点击可聚焦元素应触发 :focus 伪类（outline 指示器等），而不只是文本控件。
func isFocusableElement(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "input", "textarea", "select", "button", "summary", "label", "a", "area":
		return true
	}
	if strings.EqualFold(el.GetAttribute("contenteditable"), "true") {
		return true
	}
	if el.HasAttribute("tabindex") {
		return true
	}
	return false
}

func (h *Host) calcTextControlOffset(el *dom.Element, cssX, cssY float64) int {
	if el == nil {
		return 0
	}
	text := focusedElementValue(el)
	if len(text) == 0 {
		return 0
	}

	bx, by, bw, _, st := h.findFormControlBox(el)
	if bx == 0 && bw == 0 {
		return 0
	}

	// Use the element's computed style font/padding so caret placement
	// matches the painted text (previously a hardcoded Consolas 14 made
	// multi-line caret land at the wrong column, and the Y axis was
	// ignored entirely so clicking line 2+ always hit line 1).
	fontSize := 14.0
	family := "Consolas"
	weight := 400
	if st != nil {
		if st.FontSize.Value > 0 && !st.FontSize.IsAuto() {
			fontSize = st.FontSize.Value
		}
		if st.FontFamily != "" {
			family = st.FontFamily
		}
		if w, err := strconv.Atoi(st.FontWeight); err == nil && w >= 600 {
			weight = 700
		} else if strings.EqualFold(st.FontWeight, "bold") {
			weight = 700
		}
	}
	font := graphics.Font{Family: family, Size: fontSize, Weight: weight}
	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = fontSize * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	// Line height: honor the CSS line-height (multiplier, px, or %) so the
	// caret line index matches the painted text. A textarea with
	// line-height:1.5 at 13px draws 19.5px rows; using font metrics alone
	// (~16px) put clicks on line 2+ at the wrong row.
	lineH := 0.0
	if st != nil {
		switch st.LineHeight.Unit {
		case "px":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value
			}
		case "%":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value / 100 * fontSize
			}
		case "":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value * fontSize
			}
		}
	}
	if lineH <= 0 {
		lineH = ascent + descent
	}
	if lineH <= 0 {
		lineH = fontSize * 1.2
	}
	padX := 4.0
	padY := 4.0
	if st != nil {
		if v := st.PaddingLeft.Value; v > 0 && !st.PaddingLeft.IsAuto() {
			padX = v
		}
		if v := st.PaddingTop.Value; v > 0 && !st.PaddingTop.IsAuto() {
			padY = v
		}
	}

	return rendering.CalcFormControlCaretOffset(text, el.LocalName() == "textarea",
		cssX, cssY, bx, by, font, padX, padY, lineH)
}

// findFormControlBox walks the render tree to find the absolute border-box
// position/size and computed style of a form-control element.
func (h *Host) findFormControlBox(el *dom.Element) (bx, by, bw, bh float64, st *style.ComputedStyle) {
	if el == nil || h.wv == nil {
		return 0, 0, 0, 0, nil
	}
	rv := h.wv.RenderView()
	if rv == nil {
		return 0, 0, 0, 0, nil
	}
	var walk func(rendering.RenderObject) bool
	walk = func(o rendering.RenderObject) bool {
		if o == nil {
			return false
		}
		if n := o.Node(); n != nil {
			if e, ok := n.(*dom.Element); ok && e == el {
				if box, ok := o.(*rendering.RenderBox); ok {
					bx = box.AbsoluteX()
					by = box.AbsoluteY()
					bw = box.Width()
					bh = box.Height()
					st = box.Style()
					return true
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(rendering.RenderObject(rv))
	return bx, by, bw, bh, st
}

// Unfocus clears the IME focus and disables text input on the platform
// window. It removes the blinking caret and clears the form control selection.
// Call this when the user clicks outside an editable element.
func (h *Host) Unfocus() {
	if h.imeFocusedEl != nil {
		h.imeFocusedEl.SetFocused(false)
		h.imeFocusedEl = nil
		h.imeInputText = ""
		h.imeComposing = false
		h.imeComposeText = ""
		h.imeComposeBase = ""
		h.imeComposeStart = 0
		rendering.FocusedFormControl = nil
		rendering.FocusedFormControlSel = nil
		rendering.CaretVisible = false
		rendering.CaretVisibleControl = false
		h.win.SetIMEEnabled(false)
	}
}

// updateSelection updates the current text selection state from form-control
// selection data. It is called after mouse/touch events modify the selection.
func (h *Host) updateSelection(rv *rendering.RenderView) {
	// The form-control selection is already updated by calcTextControlOffset
	// during mouse event processing. This method exists as a hook for future
	// selection-change event dispatch.
}

// SetIMECompositionPos updates the IME composition/candidate window position
// to the given CSS-pixel coordinates (relative to the window). The Host
// converts these to physical pixels before forwarding to the platform window.
func (h *Host) SetIMECompositionPos(cssX, cssY float64) {
	h.win.SetIMECompositionPos(cssX, cssY)
}

// Run starts the render + event loop. It blocks until the window is closed.
// Each iteration: layouts the WebView, paints onto the GPU surface, presents,
// then processes input events (resize / scroll / mouse click / IME).
func (h *Host) Run() {
	// Set up the keyframes lookup bridge so the rendering package can find
	// @keyframes rules stored in the style resolver.
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			if rsv := fr.Resolver(); rsv != nil {
				rendering.KeyframesLookup = func(name string) *css.KeyframesRule {
					return rsv.LookupKeyframes(name)
				}
			}
		}
	}

	h.animStart = time.Now()

	// Get the FrameView for scroll management.
	frameView := h.wv.Page().MainFrame().View()
	if frameView == nil {
		return
	}

	h.frameView = frameView

	for !h.win.ShouldClose() {
		// Fetch the GPU surface fresh each frame: resize callbacks release
		// and recreate the surface, so the cached pointer would be dangling.
		gpuSurf := h.win.GPUSurface()
		// Ensure the viewport matches the current window size. This is called
		// every frame and is a no-op (FrameView.SetSize checks for actual change)
		// but catches resize events that the FramebufferSizeCallback may have
		// missed (e.g. maximize/un-maximize on some GLFW/platform combos).
		h.wv.Resize(h.win.Width(), h.win.Height())

		// Software-rendered backends (X11/Cocoa) expose a CPU canvas; the GLFW
		// GPU backend wraps its framebuffer surface. Unify both into gpuCanvas
		// so the paint + Present path below works on every platform.
		var gpuCanvas *graphics.Canvas
		var ownsCanvas bool
		if gpuSurf != nil {
			gpuCanvas = graphics.NewCanvasFromSurface(gpuSurf, h.win.FramebufferWidth(), h.win.FramebufferHeight())
			ownsCanvas = true
		} else if sw := h.win.Canvas(); sw != nil {
			gpuCanvas = sw
			ownsCanvas = false
		}
		if gpuCanvas == nil {
			h.processEvents(nil)
			h.processEventLoop()
			continue
		}

		h.wv.EnsureLayout()
		rv := h.wv.RenderView()
		if DumpRTCallback != nil && rv != nil && h.needsResizeDump {
			DumpRTCallback(rv)
			h.needsResizeDump = false
		}
		if rv != nil {
			// Drive CSS animations: update the global animation clock and
			// apply animated opacity to elements' ComputedStyle before paint.
			// CSS transitions interpolate style changes (:hover / :checked);
			// while one is in flight the frame needs a re-layout every frame
			// so interpolated left/top geometry updates (switch thumb slide).
			rendering.AnimationTime = time.Since(h.animStart).Seconds()
			if rendering.ApplyAnimations(rv) {
				if mf := h.wv.MainFrame(); mf != nil {
					if fr := mf.Frame(); fr != nil {
						fr.SetNeedsLayout(true)
					}
				}
			}

			// Update text selection from stored coordinates against the
			// current render tree (robust to rebuilds).
			h.updateSelection(rv)

			// Blink the caret at ~500ms intervals, mirroring WebKit's
			// caret blink cycle. The caret is only visible when an IME
			// focus target is set or a non-selection click positioned it.
			// CaretVisibleControl follows the same cycle for form-control
			// carets (which are drawn by paintFormControlCaret, not PaintCaret).
			if time.Since(h.caretBlinkTime) > 500*time.Millisecond {
				rendering.CaretVisible = !rendering.CaretVisible
				rendering.CaretVisibleControl = rendering.CaretVisible
				h.caretBlinkTime = time.Now()
			}

			// ★ Keep the IME composition/candidate window positioned at the
			// text caret. Previously SetIMECompositionPos was never called,
			// so Windows IME always showed its candidate list at the top-left
			// corner of the screen.
			if rendering.FocusedFormControl != nil {
				if cx, cy, ok := rendering.FormControlCaretPosition(rv); ok {
					// Caret Y is in page coordinates; account for scroll.
					cy -= float64(frameView.ScrollY())
					if cy < 0 {
						cy = 0
					}
					if h.lastIMEX != int32(cx) || h.lastIMEY != int32(cy) {
						h.lastIMEX, h.lastIMEY = int32(cx), int32(cy)
						if os.Getenv("WB_IME_DEBUG") != "" {
							log.Printf("[ime] host SetIMECompositionPos css=(%.0f,%.0f) scrollY=%d", cx, cy, frameView.ScrollY())
						}
						h.win.SetIMECompositionPos(cx, cy)
					}
				} else if os.Getenv("WB_IME_DEBUG") != "" {
					log.Printf("[ime] FormControlCaretPosition not ok (FocusedFormControl set)")
				}
			}

			bgColor := findBodyBgColor(rendering.RenderObject(rv))
			if bgColor.A == 0 {
				bgColor = graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
			}
			gpuCanvas.Clear(bgColor)

			// Diagnostic: log body frame rect on first frame
			if bodyRO := findRenderObjectForNode(rendering.RenderObject(rv), h.wv.MainFrame().Document().Body()); bodyRO != nil {
				if box, ok := bodyRO.(*rendering.RenderBox); ok {
					fr := box.FrameRect()
					log.Printf("[paint] body frame=(%.0f,%.0f %.0fx%.0f)", fr.X, fr.Y, fr.Width, fr.Height)
				}
			}

			// Clamp scroll offset to valid range after layout.
			scrollY := frameView.ScrollY()
			if scrollY < 0 {
				scrollY = 0
			}
			if maxY := frameView.MaxScrollY(); scrollY > maxY {
				if maxY > 0 {
					log.Printf("[scroll] clamp scrollY from %d to %d (maxY=%d contentH=%d viewportH=%d)\n",
						scrollY, maxY, maxY, frameView.ContentHeight(), frameView.Height())
				}
				scrollY = maxY
			}
			if scrollY != 0 && scrollY != h.lastLoggedScrollY {
				log.Printf("[scroll] scrollY=%d maxY=%d contentH=%d viewportH=%d\n",
					scrollY, frameView.MaxScrollY(), frameView.ContentHeight(), frameView.Height())
				h.lastLoggedScrollY = scrollY
			}
			frameView.SetScrollOffset(frameView.ScrollX(), scrollY)

			gpuCanvas.Save()
			csX, csY := h.win.ContentScale()
			gpuCanvas.Scale(csX, csY)
			gpuCanvas.Translate(0, -float64(frameView.ScrollY()))
			dirtyRect := graphics.Rect{X: 0, Y: float64(frameView.ScrollY()), Width: float64(h.win.Width()), Height: float64(h.win.Height())}
			rendering.Paint(rv, gpuCanvas, dirtyRect)
			gpuCanvas.Restore()
		}

		if ownsCanvas {
			gpuCanvas.Release()
		}
		h.win.Present()

		h.processEvents(rv)

		// 驱动 JS 事件循环：处理到期的 setTimeout/setInterval 宏任务、
		// Promise.then 微任务、requestAnimationFrame 动画帧回调。
		h.processEventLoop()
	}
}

// processEventLoop 驱动 JS 事件循环（对标浏览器事件循环模型）。
// 在渲染循环中每帧调用，处理到期的宏任务、微任务和动画帧回调。
func (h *Host) processEventLoop() {
	interp := h.wv.JSInterpreter()
	if interp == nil {
		return
	}
	el := interp.GetEventLoop()
	if el == nil {
		return
	}
	elapsedMs := time.Since(h.animStart).Milliseconds()
	el.ProcessTasks(elapsedMs)
}

// processEvents drains the platform event queue and dispatches each event.
// Resize updates the WebView viewport; scroll adjusts scrollY; mouse clicks
// are hit-tested against the render tree and forwarded to the click handler.
// Mouse drag/move drive text selection; Ctrl+C copies selected text.
func (h *Host) processEvents(rv *rendering.RenderView) {
	for _, ev := range h.win.PollEvents() {
		switch ev.Type {
		case window.EventResize:
			h.wv.Resize(h.win.Width(), h.win.Height())
			h.needsResizeDump = true
			// Also dump on the first frame after resize: compare scroll and content sizes
			if rv != nil && rv.LayoutState() != nil {
				lb := rv.LayoutBox()
				if lb != nil {
					log.Printf("[resize-diag] viewport=%dx%d layoutRoot=(%.0f,%.0f %.0fx%.0f)",
						h.win.Width(), h.win.Height(),
						rv.LayoutState().GeometryForBox(lb).ContentBoxLeft(),
						rv.LayoutState().GeometryForBox(lb).ContentBoxTop(),
						rv.LayoutState().GeometryForBox(lb).ContentWidth(),
						rv.LayoutState().GeometryForBox(lb).ContentHeight())
				}
			}
		case window.EventScroll:
			if rv == nil {
				break
			}
			// GLFW: ScrollY > 0 when scrolling up (away from user), ScrollX > 0 when scrolling right.
			// Browser: scroll up → see content above → scrollY decreases.
			//          scroll right → see content to the right → scrollX increases.
			// Try per-box scroll first: hit-test under cursor for overflow:scroll/auto.
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := h.cursorX / csX
			cssY := h.cursorY/csY + float64(h.wv.Page().MainFrame().View().ScrollY())
			if scrollBox := rv.HitTestScrollContainer(cssX, cssY); scrollBox != nil {
				log.Printf("[scroll] per-box hit at (%.0f,%.0f) scrollY=%d\n",
					cssX, cssY, h.wv.Page().MainFrame().View().ScrollY())
				sx, sy := rv.BoxScrollOffset(scrollBox)
				deltaX := int(ev.ScrollX * 40)  // positive = right → sx increases
				deltaY := -int(ev.ScrollY * 40) // positive = up → sy decreases
				newSx := int(sx) + deltaX
				newSy := int(sy) + deltaY
				// Clamp to valid range.
				if newSy < 0 {
					newSy = 0
				}
				if newSx < 0 {
					newSx = 0
				}
				cw, ch := rv.BoxContentSize(scrollBox)
				pb := scrollBox.PaddingBoxRect()
				maxX := int(cw - pb.Width)
				if maxX < 0 {
					maxX = 0
				}
				if newSx > maxX {
					newSx = maxX
				}
				maxY := int(ch - pb.Height)
				if maxY < 0 {
					maxY = 0
				}
				if newSy > maxY {
					newSy = maxY
				}
				rv.SetBoxScrollOffset(scrollBox, float64(newSx), float64(newSy))
			} else {
				log.Printf("[scroll] FrameView.ScrollBy(dx=%d, dy=%d) scrollY=%d maxY=%d contentH=%d viewportH=%d\n",
					-int(ev.ScrollX*40), -int(ev.ScrollY*40),
					h.frameView.ScrollY(), h.frameView.MaxScrollY(),
					h.frameView.ContentHeight(), h.frameView.Height())
				h.frameView.ScrollBy(-int(ev.ScrollX*40), -int(ev.ScrollY*40))
			}

		case window.EventCursorMove:
			h.cursorX, h.cursorY = ev.X, ev.Y
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := ev.X / csX
			cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY()) // page coords for scrollbar hover

			// Update RenderView cursor for scrollbar hover highlight.
			if rv != nil {
				rv.SetCursorPos(cssX, cssY)
			}

			// ── Drag-to-select inside a focused text form control ──
			// While the mouse button is held (selecting) and the drag
			// threshold is met, extend the selection End to the cursor.
			if h.selecting && !h.scrollbarDragging && h.imeFocusedEl != nil &&
				isTextFormControl(h.imeFocusedEl) && rendering.FocusedFormControlSel != nil {
				if !h.hysteresisMet {
					dx := cssX - h.mouseDownX
					dy := cssY - h.mouseDownY
					if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
						// Not yet dragging.
					} else {
						h.hysteresisMet = true
					}
				}
				if h.hysteresisMet {
					offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
					rendering.FocusedFormControlSel.End = offset
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
			}

			// ── Hover tracking (normal cursor move, outside scrollbar drag) ──
			if rv != nil && !h.scrollbarDragging {
				newEl := rendering.HitTest(rv, cssX, cssY, "")
				elName := "<nil>"
				if newEl != nil {
					elName = newEl.LocalName()
					if cn := newEl.ClassName(); cn != "" {
						elName += "." + cn
					}
				}
				if debugPaintLog {
					if newEl != h.hoveredEl {
						oldName := "<nil>"
						if h.hoveredEl != nil {
							oldName = h.hoveredEl.LocalName()
						}
						log.Printf("[dbg/hover] css=(%.0f,%.0f) %s → %s", cssX, cssY, oldName, elName)
					}
				}
				if newEl != h.hoveredEl {
					if h.hoveredEl != nil {
						h.hoveredEl.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
							// ★ hover 状态变化必须触发重建：只 MarkRenderTreeDirty
							// 不会驱动 EnsureLayout → Layout（NeedsLayout 仍为 false），
							// RebuildRenderTreeIfNeeded 永远不会执行 → :hover 样式
							// 从不反映到画面上。SetNeedsLayout 让下帧 Layout 重建。
							fr.SetNeedsLayout(true)
						}
					}
				}
			}

			// Handle scrollbar thumb drag.
			if h.scrollbarDragging && rv != nil && h.scrollbarDragBox != nil {
				csX, csY := h.win.ContentScale()
				if csX <= 0 {
					csX = 1
				}
				if csY <= 0 {
					csY = 1
				}
				cssX := ev.X / csX
				cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY()) // match EventMouseButton coordinate space
				if h.scrollbarDragAxis {
					// Vertical drag: cursor delta → scroll offset delta
					dy := cssY - h.scrollbarDragStart
					pb := h.scrollbarDragBox.PaddingBoxRect()
					vh := pb.Height
					_, ch := rv.BoxContentSize(h.scrollbarDragBox)
					if ch > pb.Height {
						const arrowSize = 12.0
						trackH := vh - arrowSize*2
						thumbH := trackH * pb.Height / ch
						if thumbH < arrowSize {
							thumbH = arrowSize
						}
						scale := (ch - pb.Height) / (trackH - thumbH)
						newSy := h.scrollbarDragScroll + dy*scale
						if newSy < 0 {
							newSy = 0
						}
						maxY := ch - pb.Height
						if newSy > maxY {
							newSy = maxY
						}
						rv.SetBoxScrollOffset(h.scrollbarDragBox, 0, newSy)
					}
				} else {
					// Horizontal drag: cursor delta → scroll offset delta
					dx := cssX - h.scrollbarDragStart
					pb := h.scrollbarDragBox.PaddingBoxRect()
					hw := pb.Width
					cw, _ := rv.BoxContentSize(h.scrollbarDragBox)
					if cw > pb.Width {
						const arrowSize = 12.0
						trackW := hw - arrowSize*2
						thumbW := trackW * pb.Width / cw
						if thumbW < arrowSize {
							thumbW = arrowSize
						}
						scale := (cw - pb.Width) / (trackW - thumbW)
						newSx := h.scrollbarDragScroll + dx*scale
						if newSx < 0 {
							newSx = 0
						}
						maxX := cw - pb.Width
						if newSx > maxX {
							newSx = maxX
						}
						rv.SetBoxScrollOffset(h.scrollbarDragBox, newSx, 0)
					}
				}
				// ── Hover tracking ──
				newEl := rendering.HitTest(rv, cssX, cssY, "")
				if newEl != h.hoveredEl {
					if h.hoveredEl != nil {
						h.hoveredEl.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
							// hover 需 SetNeedsLayout 才能触发下帧重建（同第一处）。
							fr.SetNeedsLayout(true)
						}
					}
				}
			}
		case window.EventMouseButton:
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := ev.X / csX
			cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY())

			if ev.Action == int(glfw.Press) {
				if debugPaintLog {
					log.Printf("[dbg/click] press at css=(%.0f,%.0f)", cssX, cssY)
				}
				// ── Active state ──
				if rv != nil {
					activeEl := rendering.HitTest(rv, cssX, cssY, "")
					if activeEl != nil {
						if debugPaintLog {
							log.Printf("[dbg/click] hit=%s class=%q type=%q", activeEl.LocalName(), activeEl.ClassName(), activeEl.GetAttribute("type"))
						}
						activeEl.SetActive(true)
						h.activeEl = activeEl

						// ── Toggle checkbox / radio on click ──
						if activeEl.LocalName() == "input" {
							inputType := activeEl.GetAttribute("type")
							if inputType == "checkbox" || inputType == "radio" {
								if in, ok := html5.ToInputElement(activeEl); ok {
									if inputType == "checkbox" {
										in.SetChecked(!in.Checked())
									} else if inputType == "radio" {
										// Uncheck all radio buttons with same name
										name := activeEl.GetAttribute("name")
										if name != "" && h.wv.MainFrame() != nil {
											doc := h.wv.MainFrame().Document()
											if doc != nil {
												allInputs := doc.GetElementsByTagName("input")
												for _, r := range allInputs {
													if r.GetAttribute("type") == "radio" && r.GetAttribute("name") == name {
														if r2, ok2 := html5.ToInputElement(r); ok2 {
															r2.SetChecked(false)
														}
													}
												}
											}
										}
										in.SetChecked(true)
									}
									if debugPaintLog {
										log.Printf("[dbg/click] toggled %s checked=%v", inputType, in.Checked())
									}
									h.wv.RebuildRenderTree()
								}
							}
						}

						if mf := h.wv.MainFrame(); mf != nil {
							if fr := mf.Frame(); fr != nil {
								fr.MarkRenderTreeDirty()
							}
						}
					}
				}
				// Check for scrollbar interaction.
				scrollHit := rendering.HitTestScrollbar(rv, cssX, cssY)
				if scrollHit != nil && !scrollHit.IsCorner {
					box := scrollHit.Box
					// ── Thumb drag ──
					if scrollHit.IsVThumb {
						h.scrollbarDragging = true
						h.scrollbarDragBox = box
						h.scrollbarDragAxis = true // vertical
						_, sy := rv.BoxScrollOffset(box)
						h.scrollbarDragStart = cssY
						h.scrollbarDragScroll = sy
						break
					}
					if scrollHit.IsHThumb {
						h.scrollbarDragging = true
						h.scrollbarDragBox = box
						h.scrollbarDragAxis = false // horizontal
						sx, _ := rv.BoxScrollOffset(box)
						h.scrollbarDragStart = cssX
						h.scrollbarDragScroll = sx
						break
					}
					// ── Arrow buttons → line scroll ──
					const lineStep = 16.0
					if scrollHit.IsVUpArrow {
						sx, sy := rv.BoxScrollOffset(box)
						rv.SetBoxScrollOffset(box, sx, sy-lineStep)
						break
					}
					if scrollHit.IsVDownArrow {
						sx, sy := rv.BoxScrollOffset(box)
						rv.SetBoxScrollOffset(box, sx, sy+lineStep)
						break
					}
					if scrollHit.IsHLeftArrow {
						sx, sy := rv.BoxScrollOffset(box)
						rv.SetBoxScrollOffset(box, sx-lineStep, sy)
						break
					}
					if scrollHit.IsHRightArrow {
						sx, sy := rv.BoxScrollOffset(box)
						rv.SetBoxScrollOffset(box, sx+lineStep, sy)
						break
					}
					// ── Track click (non-thumb) → page scroll ──
					if scrollHit.IsVTrack {
						sx, sy := rv.BoxScrollOffset(box)
						pb := box.PaddingBoxRect()
						pageH := pb.Height
						// Determine click position relative to thumb center.
						_, ch := rv.BoxContentSize(box)
						totalH := ch
						contentH := pb.Height
						trackH := pb.Height - 12.0*2 // arrowSize
						thumbLen := trackH * contentH / totalH
						if thumbLen < 12.0 {
							thumbLen = 12.0
						}
						if thumbLen > trackH-4 {
							thumbLen = trackH - 4
						}
						maxSy := totalH - contentH
						if maxSy <= 0 {
							maxSy = 1
						}
						syRatio := sy / maxSy
						thumbTrackSpace := trackH - thumbLen
						thumbCenterY := pb.Y + 12.0 + syRatio*thumbTrackSpace + thumbLen/2
						if cssY < thumbCenterY {
							rv.SetBoxScrollOffset(box, sx, sy-pageH)
						} else {
							rv.SetBoxScrollOffset(box, sx, sy+pageH)
						}
						break
					}
					if scrollHit.IsHTrack {
						sx, sy := rv.BoxScrollOffset(box)
						pb := box.PaddingBoxRect()
						pageW := pb.Width
						cw, _ := rv.BoxContentSize(box)
						totalW := cw
						contentW := pb.Width
						trackW := pb.Width - 12.0*2
						thumbLen := trackW * contentW / totalW
						if thumbLen < 12.0 {
							thumbLen = 12.0
						}
						if thumbLen > trackW-4 {
							thumbLen = trackW - 4
						}
						maxSx := totalW - contentW
						if maxSx <= 0 {
							maxSx = 1
						}
						sxRatio := sx / maxSx
						thumbTrackSpace := trackW - thumbLen
						thumbCenterX := pb.X + 12.0 + sxRatio*thumbTrackSpace + thumbLen/2
						if cssX < thumbCenterX {
							rv.SetBoxScrollOffset(box, sx-pageW, sy)
						} else {
							rv.SetBoxScrollOffset(box, sx+pageW, sy)
						}
						break
					}
				}

				now := time.Now()
				dx := ev.X - h.lastClickX
				dy := ev.Y - h.lastClickY
				isConsecutive := now.Sub(h.lastClickTime) < 500*time.Millisecond &&
					dx > -5 && dx < 5 && dy > -5 && dy < 5

				if isConsecutive {
					h.clickCount++
					if h.clickCount > 4 {
						h.clickCount = 4
					}
				} else {
					h.clickCount = 1
				}

				// Map click count to granularity: 1=char, 2=word, 3=line, 4=paragraph
				var gran rendering.TextGranularity
				switch h.clickCount {
				case 2:
					gran = rendering.GranularityWord
				case 3:
					gran = rendering.GranularityLine
				case 4:
					gran = rendering.GranularityParagraph
				default:
					gran = rendering.GranularityCharacter
				}
				h.selGranularity = gran

				// Shift+Click extends selection from the anchor.
				if (ev.Mods&int(glfw.ModShift)) != 0 && h.clickCount > 0 &&
					(h.selAnchorX != 0 || h.selAnchorY != 0) {
					h.selEndX = cssX
					h.selEndY = cssY
					h.shiftSelecting = true
					h.selecting = false
				} else {
					// Reset caret blink so the caret is immediately visible on focus.
					h.caretBlinkTime = time.Now()
					rendering.CaretVisible = true
					rendering.CaretVisibleControl = true

					// HitTest the click position to find the element under cursor.
					// If it's a form control (input/textarea/select), set focus.
					if rv != nil {
						hitEl := rendering.HitTest(rv, cssX, cssY, "")
						log.Printf("[dbg/click] Press at css=(%.0f,%.0f) imeFocusedEl=%v hitEl=%v localName=%q type=%q",
							cssX, cssY, h.imeFocusedEl != nil, hitEl != nil,
							func() string { if hitEl != nil { return hitEl.LocalName() }; return "" }(),
							func() string { if hitEl != nil { return hitEl.GetAttribute("type") }; return "" }())
						if hitEl != nil && isFocusableElement(hitEl) {
							if hitEl != h.imeFocusedEl {
								h.FocusElement(hitEl)
							}
						} else if h.imeFocusedEl != nil {
							h.Unfocus()
						}
					}

					// If the click is on a text form control, calculate the
					// character offset and set the form-control selection.
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
						// Start a mouse-drag selection from this press: the
						// anchor stays at the press point; subsequent mouse
						// moves extend the End. Previously selecting was never
						// set true, so drag-to-select did nothing.
						h.selecting = true
						h.shiftSelecting = false
						h.mouseDownX = cssX
						h.mouseDownY = cssY
						h.hysteresisMet = false
						if (ev.Mods&int(glfw.ModShift)) != 0 && rendering.FocusedFormControlSel != nil {
							// Shift+Click extends form-control selection.
							h.selecting = false
							rendering.FocusedFormControlSel.End = offset
							rendering.FocusedFormControlSel.Active = true
						} else {
							rendering.FocusedFormControlSel = &rendering.FormControlSelection{
								Start:  offset,
								End:    offset,
								Active: true,
							}
							if os.Getenv("WB_IME_DEBUG") != "" {
								log.Printf("[ime] click-pos css=(%.0f,%.0f) box=(%.0f,%.0f) offset=%d sel={%d,%d}",
									cssX, cssY, h.mouseDownX, h.mouseDownY, offset, offset, offset)
							}
						}
					} else if h.imeFocusedEl != nil {
						// Click outside a text control clears the form-control selection.
						h.selecting = false
						rendering.FocusedFormControlSel = nil
					} else {
						h.selecting = false
					}
				}
			} else if ev.Action == int(glfw.Release) {
				// ── Clear active state ──
				if h.activeEl != nil {
					h.activeEl.SetActive(false)
					h.activeEl = nil
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
				// ── Dispatch DOM click on release ──
				// Vue binds @click via addEventListener('click'); the click
				// must be dispatched here so those handlers actually run.
				if rv != nil {
					h.handleClick(rv, ev)
				}
				// End scrollbar drag if active.
				if h.scrollbarDragging {
					h.scrollbarDragging = false
					h.scrollbarDragBox = nil
				}
				if h.selecting {
					csX, csY := h.win.ContentScale()
					if csX <= 0 {
						csX = 1
					}
					if csY <= 0 {
						csY = 1
					}
					cssX := ev.X / csX
					cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY())

					// Hysteresis: only start dragging after moving > 3px from
					// mouseDown. A plain click (no movement) ends the drag
					// selection immediately — it becomes a caret placement.
					if !h.hysteresisMet {
						dx := cssX - h.mouseDownX
						dy := cssY - h.mouseDownY
						if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
							h.selecting = false
							h.hysteresisMet = false
							if rendering.FocusedFormControlSel != nil {
								rendering.FocusedFormControlSel.Active = false
							}
							continue // not yet dragging; treat as a click
						}
						h.hysteresisMet = true
					}
					// ★ Do NOT recompute End here from the release point:
					// the selection range is finalized by the last
					// mousemove while dragging. Recomputing with the release
					// coordinates makes the selection jump when the mouse is
					// released outside the control or after a fast drag
					// (browsers keep the last drag position on mouseup).
					// End the drag selection.
					h.selecting = false
					h.hysteresisMet = false
					rendering.FocusedFormControlSel.Active = false
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
			}
		case window.EventChar:
			log.Printf("[dbg/char] EventChar char=%q imeFocused=%v isText=%v", string(ev.Char), h.imeFocusedEl != nil, h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl))
			if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
				char := string(ev.Char)
				// Get current value and insert character at cursor position,
				// replacing any active selection (like a browser).
				val := focusedElementValue(h.imeFocusedEl)
				runes := []rune(val)
				sel := rendering.FocusedFormControlSel
				// Caret default = END of text when no click positioned it
				// (browsers focus with the caret at the end; inserting at 0
				// put every char at the HEAD). A click-positioned caret is
				// honored regardless of Active — Release sets Active=false
				// when the drag ends, but the caret must stay where the
				// click placed it.
				start, end := len(runes), len(runes)
				if sel != nil {
					start, end = sel.Start, sel.End
				}
				if start > end {
					start, end = end, start
				}
				if runes == nil {
					runes = []rune{}
				}
				if start < 0 {
					start = 0
				}
				if end < 0 {
					end = 0
				}
				if start > len(runes) {
					start = len(runes)
				}
				if end > len(runes) {
					end = len(runes)
				}
				newRunes := make([]rune, 0, len(runes)+1)
				newRunes = append(newRunes, runes[:start]...)
				newRunes = append(newRunes, []rune(char)...)
				newRunes = append(newRunes, runes[end:]...)
				newVal := string(newRunes)
				setFocusedElementValue(h.imeFocusedEl, newVal)
				if os.Getenv("WB_IME_DEBUG") != "" {
					log.Printf("[ime] evchar char=%q start=%d end=%d → %q", char, start, end, newVal)
				}
				// Cursor lands right after the inserted character.
				newPos := start + len([]rune(char))
				if sel == nil {
					rendering.FocusedFormControlSel = &rendering.FormControlSelection{
						Start: newPos, End: newPos, Active: true,
					}
				} else {
					sel.Start = newPos
					sel.End = newPos
				}
				if mf := h.wv.MainFrame(); mf != nil {
					if fr := mf.Frame(); fr != nil {
						fr.MarkRenderTreeDirty()
					}
				}
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
				h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
			}

		case window.EventKey:
			// Keyboard scrolling for PageUp/PageDown/Arrow keys.
			if ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat) {
				if rv != nil {
					csX, csY := h.win.ContentScale()
					if csX <= 0 {
						csX = 1
					}
					if csY <= 0 {
						csY = 1
					}
					cssX := h.cursorX / csX
					cssY := h.cursorY / csY
					scrollBox := rv.HitTestScrollContainer(cssX, cssY)
					if scrollBox != nil {
						sx, sy := rv.BoxScrollOffset(scrollBox)
						pb := scrollBox.PaddingBoxRect()
						pageH := pb.Height
						delta := 0.0
						switch ev.Key {
						case int(glfw.KeyPageUp):
							delta = -pageH * 0.8
						case int(glfw.KeyPageDown):
							delta = pageH * 0.8
						case int(glfw.KeyUp):
							delta = -60.0
						case int(glfw.KeyDown):
							delta = 60.0
						case int(glfw.KeyLeft):
							delta = -40.0
						case int(glfw.KeyRight):
							delta = 40.0
						}
						if delta != 0 {
							if ev.Key == int(glfw.KeyLeft) || ev.Key == int(glfw.KeyRight) {
								newSx := sx + delta
								cw, _ := rv.BoxContentSize(scrollBox)
								if newSx < 0 {
									newSx = 0
								}
								if maxSx := cw - pb.Width; newSx > maxSx {
									newSx = maxSx
								}
								rv.SetBoxScrollOffset(scrollBox, newSx, sy)
							} else {
								newSy := sy + delta
								_, ch := rv.BoxContentSize(scrollBox)
								if newSy < 0 {
									newSy = 0
								}
								if maxSy := ch - pb.Height; newSy > maxSy {
									newSy = maxSy
								}
								rv.SetBoxScrollOffset(scrollBox, sx, newSy)
							}
							break
						}
					} else {
						// FrameView-level keyboard scroll.
						delta := 0
						vh := h.frameView.Height()
						switch ev.Key {
						case int(glfw.KeyPageUp):
							delta = -int(float64(vh) * 0.8)
						case int(glfw.KeyPageDown):
							delta = int(float64(vh) * 0.8)
						case int(glfw.KeyUp):
							delta = -60
						case int(glfw.KeyDown):
							delta = 60
						}
						if delta != 0 {
							h.frameView.ScrollBy(0, delta)
						}
					}
				}
			}
			if ev.Action == int(glfw.Press) && (ev.Mods&int(glfw.ModControl)) != 0 {
				switch ev.Key {
				case int(glfw.KeyV):
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						clipText := h.win.GetClipboardString()
						if clipText != "" {
							h.pasteIntoFocused(clipText)
						}
					}
				case int(glfw.KeyX):
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						sel := rendering.FocusedFormControlSel
						if sel != nil && sel.Start != sel.End {
							start, end := sel.Start, sel.End
							if start > end {
								start, end = end, start
							}
							val := focusedElementValue(h.imeFocusedEl)
							runes := []rune(val)
							if start >= 0 && end <= len(runes) {
								cutText := string(runes[start:end])
								h.win.SetClipboardString(cutText)
								newVal := string(runes[:start]) + string(runes[end:])
								setFocusedElementValue(h.imeFocusedEl, newVal)
								rendering.FocusedFormControlSel = &rendering.FormControlSelection{
									Start: start, End: start,
								}
								h.imeInputText = newVal
								h.wv.RebuildRenderTree()
							}
						}
					}
				}
			}
		}
	}
}

// handleSelection processes text selection based on granularity and drag state.
func (h *Host) handleSelection(rv *rendering.RenderView, pos rendering.TextPosition) {
	rendering.SetCaret(nil)
}

// handleDragSelection handles text selection after a drag or shift+click.
// handleDragSelection handles text selection after a drag or shift+click.
func (h *Host) handleDragSelection(rv *rendering.RenderView) {
	start := rendering.HitTestText(rv, h.selAnchorX, h.selAnchorY)
	end := rendering.HitTestText(rv, h.selEndX, h.selEndY)

	if !start.IsValid() && !end.IsValid() {
		// Click outside text: clear selection and caret.
		rendering.ClearSelection()
		rendering.SetCaret(nil)
		return
	}
	if !start.IsValid() {
		start = end
	}
	if !end.IsValid() {
		end = start
	}

	// Apply granularity expansion for drag with word/line/paragraph.
	if h.selGranularity > rendering.GranularityCharacter && h.selecting {
		forward := h.selEndY > h.selAnchorY ||
			(h.selEndY == h.selAnchorY && h.selEndX >= h.selAnchorX)
		end = rendering.ExpandPosition(end, h.selGranularity, forward)
		start = rendering.ExpandPosition(start, h.selGranularity, !forward)
	}

	// When start == end (click without drag), show caret instead of empty selection.
	if !h.selecting && !h.shiftSelecting &&
		start.RT == end.RT && start.Offset == end.Offset {
		rendering.ClearSelection()
		if h.isRenderTextEditable(start.RT) {
			pos := start
			rendering.SetCaret(&pos)
		} else {
			rendering.SetCaret(nil)
		}
		return
	}

	rendering.CurrentSelection = &rendering.Selection{
		Start:       start,
		End:         end,
		Active:      h.selecting,
		Granularity: h.selGranularity,
	}
	if h.selecting || (start.RT != end.RT) || (start.Offset != end.Offset) {
		rendering.SetCaret(nil)
	}
}

// isRenderTextEditable reports whether rt belongs to the IME-focused (editable)
// element. Used to decide whether to show a caret when clicking on text.
func (h *Host) isRenderTextEditable(rt *rendering.RenderText) bool {
	if h.imeFocusedEl == nil || rt == nil {
		return false
	}
	node := rt.Node()
	if node == nil {
		return false
	}
	parent := node.ParentNode()
	if parent == nil {
		return false
	}
	el, ok := parent.(*dom.Element)
	return ok && el == h.imeFocusedEl
}

// updateCaret positions the caret for the IME-focused element (e.g. an input
// box), placing it at the end of the element's text content. Called when no
// mouse-driven selection is active.
func (h *Host) updateCaret(rv *rendering.RenderView) {
	if rv == nil || h.imeFocusedEl == nil {
		rendering.SetCaret(nil)
		return
	}
	var found *rendering.RenderText
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if rt, ok := o.(*rendering.RenderText); ok {
			if node := rt.Node(); node != nil {
				if parent := node.ParentNode(); parent != nil {
					if el, ok := parent.(*dom.Element); ok && el == h.imeFocusedEl {
						found = rt
						return
					}
				}
			}
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	if found != nil {
		pos := rendering.TextPosition{RT: found, Offset: found.Length()}
		rendering.SetCaret(&pos)
	}
}

// handleClick converts the physical-pixel click coordinates to CSS pixels
// (the render tree's coordinate space), hit-tests the render tree, and
// dispatches a DOM click event (so JS addEventListener('click') listeners,
// e.g. Vue @click, run) plus the onclick value: "js:" prefix → EvalJS,
// otherwise → click handler.
func (h *Host) handleClick(rv *rendering.RenderView, ev window.Event) {
	if rv == nil {
		return
	}
	csX, csY := h.win.ContentScale()
	if csX <= 0 {
		csX = 1
	}
	if csY <= 0 {
		csY = 1
	}
	clickCSSX := ev.X / csX
	clickCSSY := ev.Y / csY
	clickY := clickCSSY + float64(h.wv.Page().MainFrame().View().ScrollY())

	el := rendering.HitTest(rv, clickCSSX, clickY, "onclick")
	if el == nil {
		deepest := rendering.HitTest(rv, clickCSSX, clickY, "")
		prevFocus := rendering.FocusedFormControl
		// Dispatch a bubbling DOM click so JS listeners (Vue @click,
		// addEventListener) fire — previously they never ran, so every
		// button/icon/switch click did nothing.
		if deepest != nil {
			deepest.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
		}
		if h.clickHandler != nil {
			h.clickHandler(deepest, "", clickCSSX, clickCSSY)
		}
		if deepest != nil {
			handleFormSubmitClick(deepest)
			handleLabelToggle(deepest)
			if deepest.LocalName() == "a" {
				h.handleAnchorClick(deepest)
			}
		}
		// ★ Always rebuild: JS listeners may have mutated the DOM (tabs,
		// list selection, dialog visibility). Without this the tree is
		// stale until some other action (e.g. focusing an input) rebuilds.
		h.wv.RebuildRenderTree()
		if rendering.FocusedFormControl != prevFocus {
			h.wv.RebuildRenderTree()
		}
		return
	}
	onclickVal := el.GetAttribute("onclick")
	if onclickVal == "" {
		el.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
		if h.clickHandler != nil {
			h.clickHandler(el, "", clickCSSX, clickCSSY)
		}
		handleFormSubmitClick(el)
		handleLabelToggle(el)
		if el.LocalName() == "a" {
			h.handleAnchorClick(el)
		}
		h.wv.RebuildRenderTree()
		return
	}
	if strings.HasPrefix(onclickVal, "js:") {
		_, _ = h.wv.EvalJS(onclickVal[3:])
		h.wv.RebuildRenderTree()
		return
	}
	el.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
	if h.clickHandler != nil {
		h.clickHandler(el, onclickVal, clickCSSX, clickCSSY)
	}
	h.wv.RebuildRenderTree()
}
func handleFormSubmitClick(el *dom.Element) {
	if el == nil {
		return
	}
	switch el.LocalName() {
	case "input":
		in, ok := html5.ToInputElement(el)
		if !ok {
			return
		}
		if in.Type() != html5.InputSubmit && in.Type() != html5.InputImage {
			return
		}
		form := in.Form()
		if form == nil {
			return
		}
		f, ok := html5.ToFormElement(form)
		if ok {
			f.RequestSubmit(el)
		}
	case "button":
		btn, ok := html5.ToButtonElement(el)
		if !ok {
			return
		}
		// Default button type is "submit".
		if btn.Type() != html5.ButtonSubmit {
			return
		}
		form := btn.Form()
		if form == nil {
			return
		}
		f, ok := html5.ToFormElement(form)
		if ok {
			f.RequestSubmit(el)
		}
	}
}

// handleLabelToggle 实现 <label> 的点击转发：点击 label 或其任意后代，
// 切换内部包裹的 checkbox/radio 的选中状态（浏览器 label 语义）。
// 开关（switch）的 track span 点击因此能 toggle 内嵌 checkbox，
// 且 `input:checked + .track::after` 滑块随之移动。
func handleLabelToggle(el *dom.Element) {
	if el == nil {
		return
	}
	// 向上找 label 祖先。
	lab := el
	for lab != nil && lab.LocalName() != "label" {
		lab = lab.ParentElement()
	}
	if lab == nil {
		return
	}
	// 找 label 内第一个 checkbox/radio。
	for c := lab.FirstChild(); c != nil; c = c.NextSibling() {
		e, ok := c.(*dom.Element)
		if !ok || e.LocalName() != "input" {
			continue
		}
		typ := e.GetAttribute("type")
		if typ != "checkbox" && typ != "radio" {
			continue
		}
		in, ok := html5.ToInputElement(e)
		if !ok {
			continue
		}
		if typ == "checkbox" {
			in.SetChecked(!in.Checked())
		} else {
			in.SetChecked(true)
		}
		return
	}
}

// handleAnchorClick performs navigation when an <a> element is clicked.
// For external URLs (http/https) it attempts to open the system browser;
// for local file paths it reads and loads the referenced file as new HTML.
func (h *Host) handleAnchorClick(el *dom.Element) {
	a, ok := html5.ToAnchorElement(el)
	if !ok {
		return
	}
	href := a.Href()
	if href == "" || href == "#" {
		return
	}

	// For external links, log (opening the system browser is platform-specific
	// and left to the embedder; here we just log).
	if a.IsExternalLink() {
		fmt.Printf("app: external link: %s (open in system browser)\n", href)
		return
	}

	// For local links, try to load the referenced file as new HTML.
	if h.wv != nil {
		frame := h.wv.MainFrame()
		if frame != nil {
			// Resolve the path: strip file:// prefix if present.
			filePath := href
			if strings.HasPrefix(filePath, "file://") {
				filePath = strings.TrimPrefix(filePath, "file://")
			}
			// Try to read and load the file as HTML.
			data, err := os.ReadFile(filePath)
			if err == nil {
				frame.LoadHTML(string(data))
			} else {
				fmt.Printf("app: cannot load link %q: %v\n", href, err)
			}
		}
	}
}

// applyIMEEvents updates the focused element's text from IME
// applyIMEEvents updates the focused element's text from IME
// composition / character input events and dispatches the appropriate DOM
// events (input, change, compositionstart/update/end) so that JavaScript
// event listeners and the wb-ui form submission pipeline are notified.
// After modifying the element's value, the render tree is rebuilt so the
// next paint frame reflects the updated content.
// next paint frame reflects the updated content.

// FocusedElement returns the currently IME-focused element, or nil if none.
func (h *Host) FocusedElement() *dom.Element {
	return h.imeFocusedEl
}

// pasteIntoFocused inserts text into the currently focused form control,
// After updating the value, it dispatches input and change DOM events and
// rebuilds the render tree so the next frame reflects the update.
func (h *Host) pasteIntoFocused(text string) {
	if h.imeFocusedEl == nil {
		return
	}

	sel := rendering.FocusedFormControlSel
	val := focusedElementValue(h.imeFocusedEl)
	runes := []rune(val)

	var newVal string
	var newOffset int
	textRunes := []rune(text)

	if sel != nil && sel.Start != sel.End {
		// Replace selection with pasted text.
		start, end := sel.Start, sel.End
		if start > end {
			start, end = end, start
		}
		newVal = string(runes[:start]) + text + string(runes[end:])
		newOffset = start + len(textRunes)
	} else if sel != nil {
		// Insert at cursor position.
		pos := sel.Start
		if pos > len(runes) {
			pos = len(runes)
		}
		newVal = string(runes[:pos]) + text + string(runes[pos:])
		newOffset = pos + len(textRunes)
	} else {
		// Append to end.
		newVal = val + text
		newOffset = len([]rune(newVal))
	}

	setFocusedElementValue(h.imeFocusedEl, newVal)
	h.imeInputText = newVal
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{
		Start: newOffset,
		End:   newOffset,
	}

	// Dispatch input event (inputType="insertFromPaste").
	inputEvent := dom.NewInputEvent("insertFromPaste", text, false)
	h.imeFocusedEl.DispatchEvent(inputEvent)

	// Dispatch change event (bubbles, not cancelable).
	h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))

	h.wv.RebuildRenderTree()
}

// applyIMEEvents updates the focused element's text from IME
// composition/handwriting events, dispatching DOM input/composition/change
// events and rebuilding the render tree as needed. Text is inserted at the
// caret (FocusedFormControlSel.Start), replacing any selection — not appended
// to the end of the value (browser behavior).
func (h *Host) applyIMEEvents(events []ime.Event) {
	needsRebuild := false
	for _, ev := range events {
		switch ev.Kind {
		case ime.EventCompositionUpdate:
			if !h.imeComposing {
				// Composition starts: snapshot the base text (everything
				// EXCEPT the in-progress composition) and the insertion point.
				h.imeComposing = true
				h.imeComposeBase = focusedElementValue(h.imeFocusedEl)
				// Default caret = END of text when no click positioned it
				// (browsers put the caret at the end on focus; inserting at
				// 0 put new text at the HEAD).
				baseRunes := []rune(h.imeComposeBase)
				start := len(baseRunes)
				if sel := rendering.FocusedFormControlSel; sel != nil {
					start = sel.Start
					if start < 0 {
						start = 0
					}
				}
				if start > len(baseRunes) {
					start = len(baseRunes)
				}
				h.imeComposeStart = start
			}
			h.imeComposeText = ev.Composition
			if h.imeFocusedEl != nil {
				runes := []rune(h.imeComposeBase)
				pos := h.imeComposeStart
				if pos > len(runes) {
					pos = len(runes)
				}
				newText := string(runes[:pos]) + h.imeComposeText + string(runes[pos:])
				setFocusedElementValue(h.imeFocusedEl, newText)
				needsRebuild = true

				// Dispatch compositionupdate event
				h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionupdate", ev.Composition))

				// Dispatch input event with insertCompositionText
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertCompositionText", ev.Composition, true))
			}

		case ime.EventCharInput:
			wasComposing := h.imeComposing
			char := string(ev.Char)
			var newText string
			if wasComposing {
				// Composition confirmed: replace the composition preview with
				// the committed char, keeping the base text around it.
				h.imeComposing = false
				h.imeComposeText = ""
				runes := []rune(h.imeComposeBase)
				pos := h.imeComposeStart
				if pos > len(runes) {
					pos = len(runes)
				}
				newText = string(runes[:pos]) + char + string(runes[pos:])
				h.imeComposeBase = ""
				if os.Getenv("WB_IME_DEBUG") != "" {
					log.Printf("[ime] compose-commit char=%q pos=%d base=%q → %q", char, pos, string(runes), newText)
				}
			} else {
				// Plain character input: insert at the caret, replacing any
				// selection (browser behavior). Without a click-positioned
				// caret the default is the END of the text (browsers focus
				// with the caret at the end) — inserting at 0 put every
				// character at the HEAD of the value.
				val := focusedElementValue(h.imeFocusedEl)
				runes := []rune(val)
				start, end := len(runes), len(runes)
				if sel := rendering.FocusedFormControlSel; sel != nil {
					start, end = sel.Start, sel.End
					if start > end {
						start, end = end, start
					}
					if start < 0 {
						start = 0
					}
					if end > len(runes) {
						end = len(runes)
					}
				}
				newText = string(runes[:start]) + char + string(runes[end:])
				// Move caret after the inserted char.
				rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: start + 1, End: start + 1}
				if os.Getenv("WB_IME_DEBUG") != "" {
					selInfo := "nil"
					if s := rendering.FocusedFormControlSel; s != nil {
						selInfo = fmt.Sprintf("Start=%d End=%d", s.Start, s.End)
					}
					log.Printf("[ime] char char=%q start=%d end=%d sel=%s → %q", char, start, end, selInfo, newText)
				}
			}
			h.imeInputText = newText
			if h.imeFocusedEl != nil {
				setFocusedElementValue(h.imeFocusedEl, newText)
				needsRebuild = true

				if wasComposing {
					// End composition
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", newText))
				}

				// Dispatch input event with insertText
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))

				// Dispatch change event (bubbles, not cancelable)
				h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
			}

		case ime.EventCompositionEnd:
			wasComposing := h.imeComposing
			h.imeComposing = false
			h.imeComposeText = ""
			finalText := h.imeInputText
			if h.imeFocusedEl != nil {
				setFocusedElementValue(h.imeFocusedEl, finalText)
				needsRebuild = true

				if wasComposing {
					// Dispatch compositionend event
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", finalText))
				}

				// Dispatch input event with insertFromComposition
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertFromComposition", finalText, false))

				// Dispatch change event
				h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
			}
		}
	}
	if needsRebuild {
		h.wv.RebuildRenderTree()
	}

}

// findBodyBgColor walks the render tree to find the body element's background
// color. In a browser the body background propagates to the root canvas,
// filling the viewport (including the head margin area). Returns transparent
// when not found so the caller can fall back to white.
func findBodyBgColor(o rendering.RenderObject) graphics.Color {
	if o == nil {
		return graphics.Color{}
	}
	// Use the DOM Body() method for direct access to the body element,
	// then find its corresponding RenderObject via render-tree walk.
	if rv := o.View(); rv != nil {
		if doc := rv.Document(); doc != nil {
			if body := doc.Body(); body != nil {
				if bodyRO := findRenderObjectForNode(rendering.RenderObject(rv), body); bodyRO != nil {
					if st := bodyRO.Style(); st != nil {
						if st.BackgroundColor.A > 0 {
							return graphics.Color{R: st.BackgroundColor.R, G: st.BackgroundColor.G, B: st.BackgroundColor.B, A: st.BackgroundColor.A}
						}
						// Body bg is transparent — search children recursively
						// (NOT findBodyBgColor which re-enters doc.Body() path).
						if col := firstNonTransBg(bodyRO); col.A > 0 {
							return col
						}
					}
				}
			}
		}
	}
	// Fallback 1: walk from o's own subtree.
	if col := firstNonTransBg(o); col.A > 0 {
		return col
	}
	// Fallback 2: if o is a RenderView, walk from its first child (the <html> root).
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		if col := firstNonTransBg(c); col.A > 0 {
			return col
		}
	}
	return graphics.Color{}
}

// firstNonTransBg walks a render subtree and returns the first non-transparent
// background color found. Used when body bg is transparent to find the effective
// viewport background from body's children (e.g. #app div).
func firstNonTransBg(o rendering.RenderObject) graphics.Color {
	if o == nil {
		return graphics.Color{}
	}
	// Check the element itself first.
	if st := o.Style(); st != nil && st.BackgroundColor.A > 0 {
		return graphics.Color{R: st.BackgroundColor.R, G: st.BackgroundColor.G, B: st.BackgroundColor.B, A: st.BackgroundColor.A}
	}
	// Recurse into children.
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		if col := firstNonTransBg(c); col.A > 0 {
			return col
		}
	}
	return graphics.Color{}
}

// findRenderObjectForNode searches the render tree for the RenderObject
// that corresponds to the given DOM node.
func findRenderObjectForNode(ro rendering.RenderObject, target dom.Node) rendering.RenderObject {
	if ro == nil {
		return nil
	}
	if ro.Node() == target {
		return ro
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if found := findRenderObjectForNode(c, target); found != nil {
			return found
		}
	}
	return nil
}
