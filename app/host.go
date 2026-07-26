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
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/platform/ime"
	"wb-ui/platform/window"
	"wb-ui/rendering"
	"wb-ui/webkit"
)

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

	// animStart is the wall-clock time when Run() started, used to compute
	// the animation clock (AnimationTime) each frame.
	animStart time.Time

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
	selecting      bool // mouse button held during drag
	shiftSelecting bool // shift+click extending selection
	mouseDownX     float64 // press position for hysteresis
	mouseDownY     float64
	hysteresisMet  bool // drag threshold (3px) exceeded
	lastClickTime  time.Time
	lastClickX     float64
	lastClickY     float64

	// cursorX, cursorY track the last known cursor position (from mouse
	// move events), used for hit-testing on scroll events.
	cursorX, cursorY float64

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
	h.imeFocusedEl = el
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
// <input>/<textarea> it reads the "value" attribute; for other elements it
// reads textContent. Mirrors the value() accessor on
// HTMLTextFormControlElement.
func focusedElementValue(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if isTextFormControl(el) {
		return el.GetAttribute("value")
	}
	return el.TextContent()
}

// setFocusedElementValue writes the text back to a focused element. For
// <input>/<textarea> it sets the "value" attribute; for other elements it
// sets textContent.
func setFocusedElementValue(el *dom.Element, text string) {
	if el == nil {
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

func (h *Host) calcTextControlOffset(el *dom.Element, cssX, cssY float64) int {
	if el == nil {
		return 0
	}
	text := focusedElementValue(el)
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}

	// Find the render box for this element to get its absolute position.
	elX := h.findFormControlBoxX(el)
	if elX == 0 {
		return 0
	}
	relX := cssX - elX - 4 // 4px for left padding

	// Use a default font for measurement (same as the rendering package uses).
	font := graphics.Font{Family: "Consolas", Size: 14, Weight: 400}
	totalW := 0.0
	for i, r := range runes {
		charW := graphics.MeasureText(font, string(r))
		if relX < totalW+charW/2 {
			return i
		}
		totalW += charW
	}
	return len(runes)
}

// findFormControlBoxX walks the render tree to find the absolute X position
func (h *Host) findFormControlBoxX(el *dom.Element) float64 {
	if el == nil || h.wv == nil {
		return 0
	}
	rv := h.wv.RenderView()
	var foundX float64
	var walk func(rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if n := o.Node(); n != nil {
			if e, ok := n.(*dom.Element); ok && e == el {
				if box, ok := o.(*rendering.RenderBox); ok {
					foundX = box.AbsoluteX()
				}
				return
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	return foundX
}




// Unfocus clears the IME focus and disables text input on the platform
// window. It removes the blinking caret and clears the form control selection.
// Call this when the user clicks outside an editable element.
func (h *Host) Unfocus() {
	if h.imeFocusedEl != nil {
		h.imeFocusedEl = nil
		h.imeInputText = ""
		h.imeComposing = false
		h.imeComposeText = ""
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
	gpuSurf := h.win.GPUSurface()
	if gpuSurf == nil {
		fmt.Println("app: GPU surface is nil")
		return
	}

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

	for !h.win.ShouldClose() {
		gpuCanvas := graphics.NewCanvasFromSurface(gpuSurf, h.win.FramebufferWidth(), h.win.FramebufferHeight())

		h.wv.EnsureLayout()
		rv := h.wv.RenderView()
		if rv != nil {
			// Drive CSS animations: update the global animation clock and
			// apply animated opacity to elements' ComputedStyle before paint.
			rendering.AnimationTime = time.Since(h.animStart).Seconds()
			rendering.ApplyAnimations(rv)

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

		bgColor := findBodyBgColor(rendering.RenderObject(rv))
		if bgColor.A == 0 {
			log.Printf("[bg] NOT FOUND, fallback to white\n")
			bgColor = graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		} else {
			log.Printf("[bg] OK #%02x%02x%02x a=%d\n", bgColor.R, bgColor.G, bgColor.B, bgColor.A)
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
				scrollY = maxY
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

		gpuCanvas.Release()
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
		case window.EventScroll:
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
				rv.MarkAllDirty()
			} else {
				h.wv.Page().MainFrame().View().ScrollBy(-int(ev.ScrollX*40), -int(ev.ScrollY*40))
			}

		case window.EventCursorMove:
			h.cursorX, h.cursorY = ev.X, ev.Y
			// Update RenderView cursor for scrollbar hover highlight.
			if rv != nil {
				csX, csY := h.win.ContentScale()
				if csX <= 0 { csX = 1 }
				if csY <= 0 { csY = 1 }
				cssX := ev.X / csX
				cssY := ev.Y / csY
				rv.SetCursorPos(cssX, cssY)
			}
			// Handle scrollbar thumb drag.
			if h.scrollbarDragging && rv != nil && h.scrollbarDragBox != nil {
				csX, csY := h.win.ContentScale()
				if csX <= 0 { csX = 1 }
				if csY <= 0 { csY = 1 }
				cssX := ev.X / csX
				cssY := ev.Y / csY
				if h.scrollbarDragAxis {
					// Vertical drag: cursor delta → scroll offset delta
					dy := cssY - h.scrollbarDragStart
					pb := h.scrollbarDragBox.PaddingBoxRect()
					vh := pb.Height
					_, ch := rv.BoxContentSize(h.scrollbarDragBox)
					if ch > pb.Height {
						trackH := vh - 14.0 // scrollW
						thumbH := trackH * pb.Height / ch
						if thumbH < 14.0*1.5 { thumbH = 14.0 * 1.5 }
						scale := (ch - pb.Height) / (trackH - thumbH)
						newSy := h.scrollbarDragScroll + dy*scale
						if newSy < 0 { newSy = 0 }
						maxY := ch - pb.Height
						if newSy > maxY { newSy = maxY }
						rv.SetBoxScrollOffset(h.scrollbarDragBox, 0, newSy)
					}
				} else {
					// Horizontal drag: cursor delta → scroll offset delta
					dx := cssX - h.scrollbarDragStart
					pb := h.scrollbarDragBox.PaddingBoxRect()
					hw := pb.Width
					cw, _ := rv.BoxContentSize(h.scrollbarDragBox)
					if cw > pb.Width {
						trackW := hw - 14.0
						thumbW := trackW * pb.Width / cw
						if thumbW < 14.0*1.5 { thumbW = 14.0 * 1.5 }
						scale := (cw - pb.Width) / (trackW - thumbW)
						newSx := h.scrollbarDragScroll + dx*scale
						if newSx < 0 { newSx = 0 }
						maxX := cw - pb.Width
						if newSx > maxX { newSx = maxX }
						rv.SetBoxScrollOffset(h.scrollbarDragBox, newSx, 0)
					}
				}
				rv.MarkAllDirty()
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
				// Check for scrollbar thumb drag start.
				scrollHit := rendering.HitTestScrollbar(rv, cssX, cssY)
				if scrollHit != nil && !scrollHit.IsCorner {
					if scrollHit.IsVThumb {
						h.scrollbarDragging = true
						h.scrollbarDragBox = scrollHit.Box
						h.scrollbarDragAxis = true // vertical
						_, sy := rv.BoxScrollOffset(scrollHit.Box)
						h.scrollbarDragStart = cssY
						h.scrollbarDragScroll = sy
						break
					}
					if scrollHit.IsHThumb {
						h.scrollbarDragging = true
						h.scrollbarDragBox = scrollHit.Box
						h.scrollbarDragAxis = false // horizontal
						sx, _ := rv.BoxScrollOffset(scrollHit.Box)
						h.scrollbarDragStart = cssX
						h.scrollbarDragScroll = sx
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

				// If the click is on a text form control, calculate the
				// character offset and set the form-control selection.
				if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
					offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
					if (ev.Mods&int(glfw.ModShift)) != 0 && rendering.FocusedFormControlSel != nil {
						// Shift+Click extends form-control selection.
						rendering.FocusedFormControlSel.End = offset
						rendering.FocusedFormControlSel.Active = true
					} else {
						rendering.FocusedFormControlSel = &rendering.FormControlSelection{
							Start:  offset,
							End:    offset,
							Active: true,
						}
					}
				} else if h.imeFocusedEl != nil {
					// Click outside a text control clears the form-control selection.
					rendering.FocusedFormControlSel = nil
				}
			}
		} else if ev.Action == int(glfw.Release) {
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
				// mouseDown.
				if !h.hysteresisMet {
					dx := cssX - h.mouseDownX
					dy := cssY - h.mouseDownY
					if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
						continue // not yet dragging
					}
				}
				// Update the cursor-move selection end point.
				// The anchor (sel start) stays at the mouse-down point.
				if rendering.FocusedFormControlSel != nil &&
					rendering.FocusedFormControlSel.Active &&
					h.imeFocusedEl != nil {
					offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
					rendering.FocusedFormControlSel.End = offset
				}
			}
		}
	case window.EventKey:
			// Keyboard scrolling for PageUp/PageDown/Arrow keys.
			if ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat) {
				if rv != nil {
					csX, csY := h.win.ContentScale()
					if csX <= 0 { csX = 1 }
					if csY <= 0 { csY = 1 }
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
								if newSx < 0 { newSx = 0 }
								if maxSx := cw - pb.Width; newSx > maxSx { newSx = maxSx }
								rv.SetBoxScrollOffset(scrollBox, newSx, sy)
							} else {
								newSy := sy + delta
								_, ch := rv.BoxContentSize(scrollBox)
								if newSy < 0 { newSy = 0 }
								if maxSy := ch - pb.Height; newSy > maxSy { newSy = maxSy }
								rv.SetBoxScrollOffset(scrollBox, sx, newSy)
							}
							rv.MarkAllDirty()
							break
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
// dispatches the onclick value: "js:" prefix → EvalJS, otherwise → click
// handler.
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
		if h.clickHandler != nil {
			h.clickHandler(deepest, "", clickCSSX, clickCSSY)
		}
		if deepest != nil {
			handleFormSubmitClick(deepest)
			if deepest.LocalName() == "a" {
				h.handleAnchorClick(deepest)
			}
		}
		if rendering.FocusedFormControl != prevFocus {
			h.wv.RebuildRenderTree()
		}
		return
	}
	onclickVal := el.GetAttribute("onclick")
	if onclickVal == "" {
		if h.clickHandler != nil {
			h.clickHandler(el, "", clickCSSX, clickCSSY)
		}
		handleFormSubmitClick(el)
		if el.LocalName() == "a" {
			h.handleAnchorClick(el)
		}
		return
	}
	if strings.HasPrefix(onclickVal, "js:") {
		_, _ = h.wv.EvalJS(onclickVal[3:])
		h.wv.RebuildRenderTree()
		return
	}
	if h.clickHandler != nil {
		h.clickHandler(el, onclickVal, clickCSSX, clickCSSY)
		h.wv.RebuildRenderTree()
	}
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
// events and rebuilding the render tree as needed.
func (h *Host) applyIMEEvents(events []ime.Event) {
	needsRebuild := false
	for _, ev := range events {
		switch ev.Kind {
		case ime.EventCompositionUpdate:
			h.imeComposing = true
			h.imeComposeText = ev.Composition
			if h.imeFocusedEl != nil {
				newText := h.imeInputText + h.imeComposeText
				setFocusedElementValue(h.imeFocusedEl, newText)
				needsRebuild = true

				// Dispatch compositionupdate event
				h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionupdate", ev.Composition))

				// Dispatch input event with insertCompositionText
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertCompositionText", ev.Composition, true))
			}

		case ime.EventCharInput:
			wasComposing := h.imeComposing
			if h.imeComposing {
				h.imeComposing = false
				h.imeComposeText = ""
			}
			char := string(ev.Char)
			h.imeInputText += char
			if h.imeFocusedEl != nil {
				setFocusedElementValue(h.imeFocusedEl, h.imeInputText)
				needsRebuild = true

				if wasComposing {
					// End composition
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", h.imeInputText))
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