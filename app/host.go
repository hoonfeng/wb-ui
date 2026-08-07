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
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/css"
	"wb-ui/bindings"
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

// encodePNG converts RGBA pixels to PNG bytes (for WB_PAINT_DUMP diagnostics).
func encodePNG(w, h int, rgba []byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, rgba)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

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

	// firstFrame forces a render on the first loop iteration so the window
	// never stays black before any dirty rect is established.
	firstFrame bool

	// needsResizeDump is set true on EventResize, cleared after DumpRTCallback fires
	// once on the re-laid-out tree. Prevents dumping every frame.
	needsResizeDump bool
	// lastDbgCW/CH track last logged paint canvas size (WB_RESIZE_DEBUG)
	lastDbgCW, lastDbgCH int
	// paintDumped marks the one-shot WB_PAINT_DUMP canvas pixel dump
	paintDumped bool
	// snapEnabled/snapLast drive the periodic WB_SNAP layout snapshot
	snapEnabled bool
	snapLast    time.Time
	// snapFile/snapMu guard the _layout_snap.log writer (WB_SNAP)
	snapFile *os.File
	snapMu   sync.Mutex
	// paintLogFile is the WB_PAINT_LOG=1 paint trace writer (_paint_trace.log).
	// Records dirty rect / view dirty state / canvas pixel coverage per frame
	// so "打开文件前 vs 打开文件后" paint behavior can be compared exactly.
	paintLogFile *os.File
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

	// Smooth (wheel) scrolling state: wheel events set a TARGET offset and
	// the main loop interpolates toward it with an exponential approach,
	// mirroring browser wheel behavior. Scrollbar thumb drags stay 1:1 and
	// bypass this entirely (they write BoxScrollOffset directly).
	smoothBox    *rendering.RenderBox
	smoothRV     *rendering.RenderView // 拥有 smoothBox 偏移表的 RenderView（iframe 子文档时是子 Frame 视图）
	smoothCurX   float64
	smoothCurY   float64
	smoothTarX   float64
	smoothTarY   float64
	smoothActive bool
	smoothLast   time.Time

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

// scrollXFor returns the horizontal scroll offset of a scroll container.
// Form controls (input/textarea) scroll their text through the per-element
// FormControlTextScroll (set by the painter each frame), not
// BoxScrollOffset — mirror that so drag/arrow/track operations hit the
// right value, scoped to the control being dragged.
func scrollXFor(rv *rendering.RenderView, box *rendering.RenderBox) float64 {
	if el, ok := box.Node().(*dom.Element); ok {
		if el.LocalName() == "textarea" || el.LocalName() == "input" {
			return rendering.FormControlTextScroll(el)
		}
	}
	if rv == nil {
		return 0
	}
	sx, _ := rv.BoxScrollOffset(box)
	return sx
}

// setScrollXFor sets the horizontal scroll offset of a scroll container,
// routing form controls to their per-element FormControlTextScroll and
// others to BoxScrollOffset. Returns true when a change was applied.
func setScrollXFor(rv *rendering.RenderView, box *rendering.RenderBox, x float64) bool {
	if x < 0 {
		x = 0
	}
	if el, ok := box.Node().(*dom.Element); ok {
		if el.LocalName() == "textarea" || el.LocalName() == "input" {
			if rendering.FormControlTextScroll(el) == x {
				return false
			}
			rendering.SetFormControlTextScroll(el, x)
			return true
		}
	}
	if rv == nil {
		return false
	}
	sx, sy := rv.BoxScrollOffset(box)
	if sx == x {
		return false
	}
	rv.SetBoxScrollOffset(box, x, sy)
	// 水平滚动同样派发 scroll DOM 事件（前端 @scroll 懒加载依赖）。
	if el, ok := box.Node().(*dom.Element); ok {
		el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
	}
	return true
}

// markScrollDirty forces a repaint after a programmatic scroll change.
func (h *Host) markScrollDirty() {
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
}

// dispatchScrollEvent 向滚动容器派发 scroll DOM 事件（不冒泡，浏览器语义），
// 让前端 @scroll 监听器（Vue onScroll → loadMoreMessages 向上翻页等）感知
// 滚动偏移变化。此前滚轮/滚动条交互只更新引擎内偏移从不派发事件——JS 的
// el.addEventListener('scroll') 永远收不到回调，历史对话打开后向上翻页永不
// 触发，只显示初始 limit=50 条原始行（tool 消息占配额，≈最后一个 run）。
func (h *Host) dispatchScrollEvent(box *rendering.RenderBox) {
	if box == nil {
		return
	}
	if n := box.Node(); n != nil {
		if el, ok := n.(*dom.Element); ok {
			el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
		}
	}
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
	// ★ contenteditable（CodeMirror 6 输入区）：绝不 SetTextContent 全文替换
	//   ——会抹掉 CM6 的结构化 DOM（.cm-line + 高亮 span），且 CM6 的 input
	//   处理发现文本未变不会重建结构，布局永久破坏。字符插入走
	//   bindings.InsertTextAtSelection（光标处插文本节点），由 CM6 的
	//   readDOMChange 同步 state 并重建正确 DOM。
	if strings.EqualFold(el.GetAttribute("contenteditable"), "true") {
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

// formControlEditLimits returns 0 when the focused control rejects edits
// (readonly/disabled attributes), otherwise its maxlength in runes, or -1
// when unlimited. Readonly/disabled controls still receive focus but must
// not mutate their value (browser semantics).
func formControlEditLimits(el *dom.Element) int {
	if el == nil {
		return -1
	}
	if el.GetAttribute("readonly") != "" || el.GetAttribute("disabled") != "" {
		return 0
	}
	if in, ok := html5.ToInputElement(el); ok {
		return in.MaxLength()
	}
	if ta, ok := html5.ToTextAreaElement(el); ok {
		return ta.MaxLength()
	}
	return -1
}

// truncateToMaxLen clips s to at most maxLen runes (maxLen < 0 = unlimited).
func truncateToMaxLen(s string, maxLen int) string {
	if maxLen < 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
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

	_, bx, by, bw, _, sy, st := h.findFormControlBox(el)
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

	wrapMode := 0
	if st != nil && el.LocalName() == "textarea" {
		wrapMode = rendering.TextareaWrapMode(st, el)
	}
	return rendering.CalcFormControlCaretOffset(text, el.LocalName() == "textarea",
		cssX, cssY, bx, by, bw, font, padX, padY, lineH, wrapMode, sy)
}

// findFormControlBox walks the render tree to find the absolute border-box
// position/size and computed style of a form-control element.
func (h *Host) findFormControlBox(el *dom.Element) (box *rendering.RenderBox, bx, by, bw, bh, sy float64, st *style.ComputedStyle) {
	if el == nil || h.wv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	rv := h.wv.RenderView()
	if rv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	var found *rendering.RenderBox
	var walk func(rendering.RenderObject) bool
	walk = func(o rendering.RenderObject) bool {
		if o == nil {
			return false
		}
		if n := o.Node(); n != nil {
			if e, ok := n.(*dom.Element); ok && e == el {
				if b, ok := o.(*rendering.RenderBox); ok {
					found = b
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
	if found == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	bx = found.AbsoluteX()
	by = found.AbsoluteY()
	bw = found.Width()
	bh = found.Height()
	st = found.Style()
	_, sy = rv.BoxScrollOffset(found)
	return found, bx, by, bw, bh, sy, st
}

// ensureFocusedCaretVisible auto-scrolls the focused form control so the
// caret stays inside the visible content area after keyboard navigation,
// typing or paste (browser behavior). Horizontal auto-scroll already happens
// at paint time (computeTextScrollX keeps the caret visible in pre/nowrap
// rows); this handles the VERTICAL axis for textareas — moving the caret to
// a row above/below the viewport scrolls BoxScrollOffset.sy so the row is
// revealed, matching how clicking/dragging in a scrolled textarea works.
func (h *Host) ensureFocusedCaretVisible() {
	el := h.imeFocusedEl
	if el == nil || el.LocalName() != "textarea" {
		return
	}
	if h.wv == nil {
		return
	}
	// The input path runs RebuildRenderTree() (brand-new RenderBox with
	// zeroed geometry) immediately before this. Without an up-to-date
	// layout, findFormControlBox returns bh=0 → viewH clamps to 1 →
	// rowBottom>sy+1 computes a huge newSy and the content jumps out of
	// view on every keystroke (WB_SCROLL_DEBUG proved bh=0.0 and sy
	// oscillating 47↔65.5 per input). Layout first so geometry is valid.
	h.wv.EnsureLayout()
	rv := h.wv.RenderView()
	if rv == nil {
		return
	}
	box, _, _, _, bh, sy, st := h.findFormControlBox(el)
	if box == nil || st == nil {
		return
	}
	if bh <= 0 {
		// Geometry still invalid — never write a scroll derived from it.
		return
	}
	val := focusedElementValue(el)
	runes := []rune(val)
	pos := len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		pos = sel.Start
		if sel.End > pos {
			pos = sel.End
		}
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	if len(runes) == 0 {
		return
	}

	fontSize := 14.0
	family := "Consolas"
	weight := 400
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
	font := graphics.Font{Family: family, Size: fontSize, Weight: weight}

	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = fontSize * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	lineH := cssControlLineHeightForHost(st, fontSize)
	if lineH <= 0 {
		lineH = ascent + descent
	}
	if lineH <= 0 {
		lineH = fontSize * 1.2
	}
	padY := 4.0
	padB := 4.0
	if st.PaddingTop.Value > 0 && !st.PaddingTop.IsAuto() {
		padY = st.PaddingTop.Value
	}
	if st.PaddingBottom.Value > 0 && !st.PaddingBottom.IsAuto() {
		padB = st.PaddingBottom.Value
	}

	// Content box height (the viewport the painter clips to).
	viewH := bh - padY - padB
	if viewH < 1 {
		viewH = 1
	}

	// Caret's visual row (soft-wrapped) and its top/bottom in content space.
	mode := rendering.TextareaWrapMode(st, el)
	contentW := st2ContentWidth(st, box) // same as painter's contentW
	row := rendering.TextareaCaretVisualRow(val, font, contentW, mode, pos)
	rowTop := padY + float64(row)*lineH
	rowBottom := rowTop + lineH

	newSy := sy
	if rowTop < sy {
		newSy = rowTop
	} else if rowBottom > sy+viewH {
		newSy = rowBottom - viewH
	}
	if newSy < 0 {
		newSy = 0
	}
	// Clamp to the same max as the painter / scrollbar drag (shared
	// geometry) so auto-scroll never overshoots the thumb's range.
	maxScroll := 0.0
	if m := rendering.VerticalScrollbarMetrics(rv, box); m.OK {
		maxScroll = m.MaxScroll
		if newSy > m.MaxScroll {
			newSy = m.MaxScroll
		}
	}
	if os.Getenv("WB_SCROLL_DEBUG") != "" {
		log.Printf("[scroll/ensure] bh=%.1f sy=%.1f viewH=%.1f lineH=%.1f contentW=%.1f mode=%v pos=%d row=%d rowTop=%.1f rowBottom=%.1f maxScroll=%.1f -> newSy=%.1f (changed=%v)",
			bh, sy, viewH, lineH, contentW, mode, pos, row, rowTop, rowBottom, maxScroll, newSy, newSy != sy)
	}
	if newSy != sy {
		rv.SetBoxScrollOffset(box, 0, newSy)
	}
}

// cssControlLineHeightForHost mirrors rendering.cssControlLineHeight for the
// host's hit-test / caret math (px, multiplier, or %).
func cssControlLineHeightForHost(st *style.ComputedStyle, fontSize float64) float64 {
	switch st.LineHeight.Unit {
	case "px":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value
		}
	case "%":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value / 100 * fontSize
		}
	case "":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value * fontSize
		}
	}
	return 0
}

// st2ContentWidth returns the textarea's content width (padding-box minus
// horizontal padding), matching the painter's contentW.
func st2ContentWidth(st *style.ComputedStyle, box *rendering.RenderBox) float64 {
	pb := box.PaddingBoxRect()
	padL := 4.0
	padR := 4.0
	if st.PaddingLeft.Value > 0 && !st.PaddingLeft.IsAuto() {
		padL = st.PaddingLeft.Value
	}
	if st.PaddingRight.Value > 0 && !st.PaddingRight.IsAuto() {
		padR = st.PaddingRight.Value
	}
	cw := pb.Width - padL - padR
	if cw < 1 {
		cw = 1
	}
	return cw
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
	h.firstFrame = true
	h.snapEnabled = os.Getenv("WB_SNAP") != ""
	h.snapLast = time.Now()

	// Get the FrameView for scroll management.
	frameView := h.wv.Page().MainFrame().View()
	if frameView == nil {
		return
	}

	h.frameView = frameView

	// 性能统计（WB_PERF_DEBUG=1）：统计帧总数 / 渲染帧 / 空闲帧，窗口
	// 关闭时输出，用于验证按需渲染（空闲帧跳过 Paint/Present）。
	perfDebug := os.Getenv("WB_PERF_DEBUG") != ""
	perfFrames, perfRenders := 0, 0
	var perfStart time.Time
	if perfDebug {
		perfStart = time.Now()
	}

	for !h.win.ShouldClose() {
		perfFrames++
		// Fetch the GPU surface fresh each frame: resize callbacks release
		// and recreate the surface, so the cached pointer would be dangling.
		gpuSurf := h.win.GPUSurface()
		// Ensure the viewport matches the current window size. This is called
		// every frame and is a no-op (FrameView.SetSize checks for actual change)
		// but catches resize events that the FramebufferSizeCallback may have
		// missed (e.g. maximize/un-maximize on some GLFW/platform combos).
		h.wv.Resize(h.win.Width(), h.win.Height())

		h.wv.EnsureLayout()
		rv := h.wv.RenderView()

		// Smooth wheel scrolling: interpolate the per-box scroll offset
		// toward the wheel-event target with an exponential approach
		// (browser-like). Scrollbar thumb drags bypass this (1:1 direct
		// writes), so the thumb never lags the cursor.
		if h.smoothActive && rv != nil && h.smoothBox != nil {
			// ★ 渲染树可能已被重建（hover 变化触发 MarkRenderTreeDirty +
			// SetNeedsLayout → 下帧 RebuildRenderTreeIfNeeded 重建整棵树，
			// 新 RenderBox 实例）。h.smoothBox 是旧树指针，直接
			// SetBoxScrollOffset(旧box) 写入的偏移在新树上读不到 →
			// 滚动条 thumb 跟着动但内容不滚。必须每帧按 DOM 节点
			// 重新解析当前树中的 box 再写入。
			// ★ iframe 子文档滚动：偏移表在子 Frame 的 RenderView 里
			// （smoothRV，滚轮命中子 Frame 滚动容器时设置）。主 rv 的
			// FindRenderBoxForNode 查不到子文档 box——必须用子 rv。
			srv := h.smoothRV
			if srv == nil {
				srv = rv
			}
			smoothBox := srv.FindRenderBoxForNode(h.smoothBox.Node())
			if smoothBox == nil {
				// 容器被移除/不可达：放弃平滑滚动。
				h.smoothActive = false
			} else {
				now := time.Now()
				dt := now.Sub(h.smoothLast).Seconds()
				h.smoothLast = now
				if dt > 0 && dt < 0.1 {
					f := 1 - math.Exp(-dt*12)
					h.smoothCurX += (h.smoothTarX - h.smoothCurX) * f
					h.smoothCurY += (h.smoothTarY - h.smoothCurY) * f
					if math.Abs(h.smoothTarX-h.smoothCurX) < 0.5 && math.Abs(h.smoothTarY-h.smoothCurY) < 0.5 {
						h.smoothCurX, h.smoothCurY = h.smoothTarX, h.smoothTarY
						h.smoothActive = false
					}
					srv.SetBoxScrollOffset(smoothBox, h.smoothCurX, h.smoothCurY)
					// 更新持有的 box 引用，避免每帧重复查找。
					h.smoothBox = smoothBox
					// ★ 派发 scroll DOM 事件，让前端 @scroll 监听器（Vue
					// 懒加载向上翻页 loadMoreMessages 等）感知滚动偏移变化。
					// 此前滚轮只更新引擎内偏移从不派发事件——JS 的
					// el.addEventListener('scroll') 永远收不到回调，历史对话
					// 向上翻页永不触发，打开会话只显示初始 limit=50 条
					// 原始行（≈最后一个 run）。
					if n := smoothBox.Node(); n != nil {
						if el, ok := n.(*dom.Element); ok {
							el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
						}
					}
				}
			}
		}
		if DumpRTCallback != nil && rv != nil && h.needsResizeDump {
			DumpRTCallback(rv)
			h.needsResizeDump = false
		}
		// ★ WB_RESIZE_DEBUG=1：resize 后 dump 布局根 + 关键节点几何 + 滚动 + canvas，
		//   验证最大化/拖拽后布局是否跟随 viewport 更新（"内容绘制区域变小/编辑区偏移"）。
		if os.Getenv("WB_RESIZE_DEBUG") != "" && rv != nil && h.needsResizeDump && h.wv.MainFrame() != nil {
			if fr2 := h.wv.MainFrame().Frame(); fr2 != nil {
				if lb := rv.LayoutBox(); lb != nil {
					st := rv.LayoutState()
					g := st.GeometryForBox(lb)
					fv := fr2.View()
					log.Printf("[resize-aft] layoutRoot=(%.0f,%.0f %.0fx%.0f) viewport=%dx%d scrollY=%d contentSize=%dx%d",
						g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight(),
						fv.Width(), fv.Height(), fv.ScrollY(), fv.ContentWidth(), fv.ContentHeight())
					// 关键节点 dump（main-area / right-container / editor-area）
					var walkD func(ro rendering.RenderObject, depth int)
					walkD = func(ro rendering.RenderObject, depth int) {
						if ro == nil || depth > 14 {
							return
						}
						if n := ro.Node(); n != nil {
							if el, ok := n.(*dom.Element); ok {
								cls := el.GetAttribute("class")
								if cls == "main-area" || cls == "right-container" || cls == "editor-area" || cls == "right-panel" || cls == "sidebar" || cls == "activity-bar" {
									if rb := ro.LayoutBox(); rb != nil {
										gg := st.GeometryForBox(rb)
										log.Printf("[resize-aft] %s=(%.0f,%.0f %.0fx%.0f)", cls, gg.Left(), gg.Top(), gg.BorderBoxWidth(), gg.BorderBoxHeight())
									}
								}
							}
						}
						for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
							walkD(c, depth+1)
						}
					}
					walkD(rendering.RenderObject(rv), 0)
				}
				h.needsResizeDump = false
			}
		}
		animActive := false
		if rv != nil {
			// Drive CSS animations: update the global animation clock and
			// apply animated opacity to elements' ComputedStyle before paint.
			// CSS transitions interpolate style changes (:hover / :checked);
			// while one is in flight the frame needs a re-layout every frame
			// so interpolated left/top geometry updates (switch thumb slide).
			rendering.AnimationTime = time.Since(h.animStart).Seconds()
			animActive = rendering.ApplyAnimations(rv)
			if animActive {
				if mf := h.wv.MainFrame(); mf != nil {
					if fr := mf.Frame(); fr != nil {
						fr.SetNeedsLayout(true)
					}
				}
			}
		}

		// Blink the caret at ~500ms intervals, mirroring WebKit's
		// caret blink cycle. The caret is only visible when an IME
		// focus target is set or a non-selection click positioned it.
		// CaretVisibleControl follows the same cycle for form-control
		// carets (which are drawn by paintFormControlCaret, not PaintCaret).
		// A tick flags the frame that must re-render (blink state flipped).
		caretTick := false
		if rv != nil && time.Since(h.caretBlinkTime) > 500*time.Millisecond {
			rendering.CaretVisible = !rendering.CaretVisible
			rendering.CaretVisibleControl = rendering.CaretVisible
			h.caretBlinkTime = time.Now()
			caretTick = true
		}

		// ★ 按需渲染（性能核心）：仅当本帧存在任何视觉变化时才执行
		// Clear + Paint + Present；空闲帧（无 dirty、无滚动插值、无动画、
		// 无 caret 翻转、非首帧）完全跳过渲染，CPU/GPU 占用趋近于零。
		// 所有变化来源都会标记 dirty 或置位本判定：
		//   - 布局/渲染树重建（hover、DOM 变更、resize、IME 输入）→
		//     FrameView.Layout → MarkAllDirty（frameview.go）
		//   - box 滚动 / 页面滚动 → SetBoxScrollOffset / SetScrollOffset
		//     → MarkAllDirty（renderview.go / frameview.go）
		//   - 平滑滚动插值进行中 → h.smoothActive
		//   - CSS 动画/过渡活跃 → animActive（ApplyAnimations 返回值）
		//   - 光标闪烁翻转 → caretTick
		//   - 鼠标移动 → EventCursorMove 无条件 MarkAllDirty（滚动条
		//     hover 高亮依赖 cursor 位置，见 processEvents）
		needPaint := h.firstFrame || (rv != nil && (rv.IsDirty() || h.smoothActive || animActive || caretTick))
		h.firstFrame = false
		if needPaint {
			perfRenders++
		}

		// ★ WB_SNAP=1：周期性布局快照（打开文件前后对比用）。
		//   每 600ms dump 关键布局/几何/CM6 状态到 _layout_snap.log（主循环线程内
		//   执行，避免与 RunJS 并发竞态）。用户操作 desktop（打开文件/滚动）后，
		//   对比快照时间线即可定位"布局异常"发生的时刻与变化。
		if h.snapEnabled && time.Since(h.snapLast) > 600*time.Millisecond {
			h.snapLast = time.Now()
			h.dumpLayoutSnap()
		}

		if needPaint && rv != nil {
			// Software-rendered backends (X11/Cocoa) expose a CPU canvas; the
			// GLFW GPU backend wraps its framebuffer surface. Unify both into
			// gpuCanvas so the paint + Present path below works on every platform.
			var gpuCanvas *graphics.Canvas
			var ownsCanvas bool
			if gpuSurf != nil {
				gpuCanvas = graphics.NewCanvasFromSurface(gpuSurf, h.win.FramebufferWidth(), h.win.FramebufferHeight())
				ownsCanvas = true
			} else if sw := h.win.Canvas(); sw != nil {
				gpuCanvas = sw
				ownsCanvas = false
			}
			if gpuCanvas != nil {
				// Update text selection from stored coordinates against the
				// current render tree (robust to rebuilds).
				h.updateSelection(rv)

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

				// ★ 每帧重置 transform：surface.Canvas() 返回持久 skia canvas
				// （surface 跨帧持有），上一帧的 Scale/Translate/scroll 会
				// 残留累积（指数放大/错位）。ResetMatrix 清为单位矩阵并
				// 同步 c.state。★ Save 必须在 Scale 之后：fixed 层
				// （paintLayerTree 的 fixed 分支）用
				// RestoreToCount(initialSaveCount) 丢弃祖先 clip，
				// initialSaveCount = Paint 入口 SaveCount()。若 Save 在
				// Scale 之前，Saved 状态是单位矩阵（scale=1.0）——
				// RestoreToCount 弹回后 scale 变 1.0，fixed 层之后的全部
				// 内容按 1:1 画到物理 canvas → 界面等比缩小 80% + 右下空白
				// （"打开文件后绘制区域变小"：编辑器引入 fixed/层路径
				// 触发该分支）。Save 在 Scale 之后 → Saved 状态含 scale
				// 1.25，RestoreToCount 恢复正确缩放。
				gpuCanvas.ResetMatrix()
				csX, csY := h.win.ContentScale()
				gpuCanvas.Scale(csX, csY)
				gpuCanvas.Save()
				gpuCanvas.Translate(0, -float64(frameView.ScrollY()))
				dirtyRect := graphics.Rect{X: 0, Y: float64(frameView.ScrollY()), Width: float64(h.win.Width()), Height: float64(h.win.Height())}
				if os.Getenv("WB_RESIZE_DEBUG") != "" && (gpuCanvas.Width() != h.lastDbgCW || gpuCanvas.Height() != h.lastDbgCH) {
					h.lastDbgCW, h.lastDbgCH = gpuCanvas.Width(), gpuCanvas.Height()
					log.Printf("[paint] canvas=%dx%d fb=%dx%d css=%dx%d dirty=(%.0f,%.0f %.0fx%.0f) scale=%.2f",
						gpuCanvas.Width(), gpuCanvas.Height(),
						h.win.FramebufferWidth(), h.win.FramebufferHeight(),
						h.win.Width(), h.win.Height(),
						dirtyRect.X, dirtyRect.Y, dirtyRect.Width, dirtyRect.Height,
						csX)
				}
				// ★ WB_PAINT_LOG=1：Paint 前记录 view 的 dirty 状态——
				//   Paint 内部若 view.IsDirty() 会用 GetDirtyRect() 覆盖
				//   paintRect（只画局部），而 Clear 是全屏 → 局部之外空白
				//   = "内容绘制区域变小"。必须在 Paint 前抓（Paint 后
				//   ClearDirty 就丢了）。
				preDirtyStr := ""
				if os.Getenv("WB_PAINT_LOG") != "" && h.paintLogFile == nil {
					f, err := os.OpenFile("_paint_trace.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
					if err == nil {
						h.paintLogFile = f
					}
				}
				csXv, _ := h.win.ContentScale()
				if h.paintLogFile != nil {
					if rv != nil {
						preDirtyStr = fmt.Sprintf("preDirty=%v", rv.IsDirty())
						if r := rv.GetDirtyRect(); r.Width > 0 || r.Height > 0 {
							preDirtyStr += fmt.Sprintf(" preDR=(%.0f,%.0f %.0fx%.0f)", r.X, r.Y, r.Width, r.Height)
						} else {
							preDirtyStr += " preDR=none"
						}
					} else {
						preDirtyStr = "rv=nil"
					}
				}
				if os.Getenv("WB_CTM_DEBUG") != "" {
					mPre := gpuCanvas.GetMatrix()
					log.Printf("[ctm] PRE-Paint scaleX=%.3f scaleY=%.3f tx=%.1f ty=%.1f saveCount=%d",
						mPre.ScaleX, mPre.ScaleY, mPre.TransX, mPre.TransY, gpuCanvas.SaveCount())
				}
				rendering.Paint(rv, gpuCanvas, dirtyRect)
				gpuCanvas.Restore()
				if os.Getenv("WB_CTM_DEBUG") != "" {
					mPost := gpuCanvas.GetMatrix()
					log.Printf("[ctm] POST-Paint scaleX=%.3f scaleY=%.3f tx=%.1f ty=%.1f saveCount=%d",
						mPost.ScaleX, mPost.ScaleY, mPost.TransX, mPost.TransY, gpuCanvas.SaveCount())
				}

				if h.paintLogFile != nil {
					// 降采样统计非背景像素覆盖率 + 包围盒 + 亮色(编辑器)包围盒（每 8px 步长）
					cov, bb, lbb := "", "", ""
					rootInfo := ""
					if rv != nil {
						if lb := rv.LayoutBox(); lb != nil {
							if st := rv.LayoutState(); st != nil {
								g := st.GeometryForBox(lb)
								rootInfo = fmt.Sprintf(" root=(%.0f,%.0f %.0fx%.0f)", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
							}
						}
						sx, sy := rv.ScrollOffset()
						rootInfo += fmt.Sprintf(" vScroll=(%.0f,%.0f) boxScroll=%v", sx, sy, rv.HasBoxScrollOffset())
					}
					if px := gpuCanvas.Pixels(); len(px) >= 4 {
						cw, ch := gpuCanvas.Width(), gpuCanvas.Height()
						if cw > 0 && ch > 0 {
							nonBg, tot := 0, 0
							minX, minY, maxX, maxY := cw, ch, -1, -1
							lminX, lminY, lmaxX, lmaxY := cw, ch, -1, -1
							lNonBg := 0
							// 背景是 Clear(bgColor) 后的统一色，取左上角像素为参考
							refR, refG, refB, refA := px[0], px[1], px[2], px[3]
							for y := 0; y < ch; y += 8 {
								for x := 0; x < cw; x += 8 {
									idx := (y*cw + x) * 4
									tot++
									if idx+3 < len(px) {
										if px[idx+3] != refA || px[idx] != refR || px[idx+1] != refG || px[idx+2] != refB {
											nonBg++
											if x < minX {
												minX = x
											}
											if x > maxX {
												maxX = x
											}
											if y < minY {
												minY = y
											}
											if y > maxY {
												maxY = y
											}
										}
										// 亮色像素（编辑器浅色背景/文字）：R+G+B 高 → 编辑器实际绘制区
										rp, gp, bp := int(px[idx]), int(px[idx+1]), int(px[idx+2])
										if rp > 180 && gp > 180 && bp > 180 {
											lNonBg++
											if x < lminX {
												lminX = x
											}
											if x > lmaxX {
												lmaxX = x
											}
											if y < lminY {
												lminY = y
											}
											if y > lmaxY {
												lmaxY = y
											}
										}
									}
								}
							}
							cov = fmt.Sprintf("cov=%d%%", 100*nonBg/tot)
							if maxX >= 0 {
								bb = fmt.Sprintf("bb=(%d,%d %dx%d)", minX, minY, maxX-minX+1, maxY-minY+1)
							} else {
								bb = "bb=none"
							}
							if lmaxX >= 0 {
								lbb = fmt.Sprintf("lightBB=(%d,%d %dx%d) lcov=%d%%", lminX, lminY, lmaxX-lminX+1, lmaxY-lminY+1, 100*lNonBg/tot)
							} else {
								lbb = "lightBB=none"
							}
						}
					}
					line := fmt.Sprintf("[%s] css=%dx%d fb=%dx%d canvas=%dx%d scale=%.2f dirty=(%.0f,%.0f %.0fx%.0f) %s %s %s %s %s%s px{side=%s rp=%s st=%s}\n",
						time.Now().Format("15:04:05.000"), h.win.Width(), h.win.Height(),
						h.win.FramebufferWidth(), h.win.FramebufferHeight(),
						gpuCanvas.Width(), gpuCanvas.Height(),
						csXv,
						dirtyRect.X, dirtyRect.Y, dirtyRect.Width, dirtyRect.Height,
						preDirtyStr, cov, bb, lbb, rootInfo, "",
						sampleCanvasPx2(gpuCanvas, 60, 100),   // sidebar (48,30) 物理
						sampleCanvasPx2(gpuCanvas, 536, 100),  // right-panel (429,30) 物理
						sampleCanvasPx2(gpuCanvas, 750, 987)) // status-bar 物理
					h.snapMu.Lock()
					_, _ = h.paintLogFile.WriteString(line)
					_ = h.paintLogFile.Sync()
					h.snapMu.Unlock()
					// ★ WB_DUMP_PNG=1：Paint 后把 canvas（pixelCache 读回）
					//   内容存 PNG，与 BitBlt 屏幕截图对比——区分"绘制没
					//   上 canvas"与"canvas 有但屏幕没显示"。
					if os.Getenv("WB_DUMP_PNG") != "" {
						dumpCanvasPNG(gpuCanvas, "_canvas_dump.png")
					}
				}
				if ownsCanvas {
					gpuCanvas.Release()
				}
				h.win.Present()
			}
		}

		h.processEvents(rv)
		// 驱动 JS 事件循环：处理到期的 setTimeout/setInterval 宏任务、
		// Promise.then 微任务、requestAnimationFrame 动画帧回调。
		h.processEventLoop()
	}
	if perfDebug {
		elapsed := time.Since(perfStart).Seconds()
		log.Printf("[perf] frames=%d renders=%d idle=%d (%.1f%% idle) elapsed=%.2fs avgFrame=%.2fms renderRate=%.1ffps",
			perfFrames, perfRenders, perfFrames-perfRenders,
			100*float64(perfFrames-perfRenders)/float64(max(perfFrames, 1)),
			elapsed, 1000*elapsed/float64(max(perfFrames, 1)),
			float64(perfRenders)/max(elapsed, 0.001))
	}
}

// renderChildCount counts direct render children of a render object.
func renderChildCount(ro rendering.RenderObject) int {
	n := 0
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		n++
	}
	return n
}

// dumpCanvasPNG writes the canvas contents to a PNG file (debug aid).
func dumpCanvasPNG(c *graphics.Canvas, path string) {
	px := c.Pixels()
	if len(px) < 4 {
		return
	}
	cw, ch := c.Width(), c.Height()
	if cw <= 0 || ch <= 0 {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			idx := (y*cw + x) * 4
			if idx+3 >= len(px) {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: px[idx], G: px[idx+1], B: px[idx+2], A: px[idx+3]})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

// sampleCanvasPx2 returns the hex color of the canvas pixel at physical (x,y).
// Used by the WB_PAINT_LOG trace to verify whether key regions were painted.
func sampleCanvasPx2(c *graphics.Canvas, x, y int) string {
	if c == nil {
		return "?"
	}
	px := c.Pixels()
	if len(px) < 4 {
		return "?"
	}
	cw, ch := c.Width(), c.Height()
	if cw <= 0 || ch <= 0 {
		return "?"
	}
	ix, iy := x, y
	if ix < 0 {
		ix = 0
	}
	if iy < 0 {
		iy = 0
	}
	if ix >= cw {
		ix = cw - 1
	}
	if iy >= ch {
		iy = ch - 1
	}
	idx := (iy*cw + ix) * 4
	if idx+3 >= len(px) {
		return "?"
	}
	return fmt.Sprintf("#%02x%02x%02x", px[idx], px[idx+1], px[idx+2])
}

// snapLayoutJS collects key layout geometry + CM6 state from the page (WB_SNAP).
const snapLayoutJS = `(function(){  var o = {};
  o.vw = {w: window.innerWidth, h: window.innerHeight};
  o.bodyScrollH = document.body ? document.body.scrollHeight : -1;
  o.deScrollTop = document.documentElement ? document.documentElement.scrollTop : -1;
  function rect(sel, key){
    var el = document.querySelector(sel);
    if (!el) { o[key] = sel + '=NULL'; return; }
    var r = el.getBoundingClientRect();
    var cs = getComputedStyle(el);
    o[key] = sel + '=(' + Math.round(r.left) + ',' + Math.round(r.top) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height) + ') disp=' + cs.display + ' w=' + cs.width + ' pos=' + cs.position + ' ovf=' + cs.overflow;
  }
  rect('.app-root', 'app');
  rect('.main-area', 'main');
  rect('.right-container', 'right');
  rect('.editor-area', 'ea');
  rect('.editor-body', 'eb');
  rect('.editor-wrapper', 'ew');
  rect('.code-editor-wrapper', 'cw');
  rect('.cm-editor', 'cm');
  rect('.status-bar', 'sb');
  rect('.cm-scroller', 'sc');
  rect('.cm-content', 'co');
  // ★ 聊天区滚动诊断：chat-messages 的 scrollTop/scrollHeight/clientHeight
  var cm2 = document.querySelector('.chat-messages');
  if (cm2) { o.cmScrollTop = cm2.scrollTop; o.cmScrollH = cm2.scrollHeight; o.cmClientH = cm2.clientHeight; o.cmChildCount = cm2.children.length; }
  var co = document.querySelector('.cm-content');
  if (co) { o.coChildren = co.children.length; o.coTextLen = (co.textContent || '').length; o.coScrollW = co.scrollWidth; }
  o.lineCount = document.querySelectorAll('.cm-line').length;
  o.cmExists = !!document.querySelector('.cm-editor');
  var sb = document.querySelector('.status-bar');
  if (sb) {
    o.sbText = (sb.textContent || '').replace(/\s+/g, ' ').slice(0, 60);
    o.sbChildren = sb.children.length;
    o.sbHtmlLen = (sb.innerHTML || '').length;
    var csb = getComputedStyle(sb);
    o.sbColor = csb.color;
    o.sbBg = csb.backgroundColor;
    o.sbFontSize = csb.fontSize;
    o.sbFontWeight = csb.fontWeight;
    o.sbFontFamily = csb.fontFamily;
    var sl = document.querySelector('.status-left');
    if (sl) { o.sbLeftChild = sl.children.length; o.sbLeftText = (sl.textContent || '').slice(0, 40); }
    var sr = document.querySelector('.status-right');
    if (sr) { o.sbRightChild = sr.children.length; o.sbRightText = (sr.textContent || '').slice(0, 40); }
  }
  var sc = document.querySelector('.cm-scroller');
  if (sc) { o.scScrollTop = sc.scrollTop; o.scScrollH = sc.scrollHeight; o.scClientH = sc.clientHeight; }
  o.active = (document.querySelector('.file-tree-item.active .item-name') || {}).textContent || '';
  o.ftItems = document.querySelectorAll('.file-tree-item .item-row').length;
  o.errs = (window.__errs || []).join(' ;; ');
  return JSON.stringify(o);
})()`

// dumpLayoutSnap writes a periodic layout snapshot to _layout_snap.log (WB_SNAP).
// Appends with a timestamp so the user's interactions (open file / scroll) can be
// compared along the time axis: find the moment the layout broke and what changed.
func (h *Host) dumpLayoutSnap() {
	if h.wv.JSInterpreter() == nil {
		return
	}
	geo := ""
	v, err := h.wv.JSInterpreter().RunJS(snapLayoutJS)
	if err != nil {
		geo = "[err] " + err.Error()
	} else {
		geo = v.ToString()
	}
	// Go 侧补充：布局根 + 关键渲染节点几何（与 JS getBoundingClientRect 对比）
	extra := ""
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			if rv := fr.RenderView(); rv != nil {
				if lb := rv.LayoutBox(); lb != nil {
					if st := rv.LayoutState(); st != nil {
						g := st.GeometryForBox(lb)
						extra = fmt.Sprintf(" layoutRoot=(%.0f,%.0f %.0fx%.0f)", g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight())
						if fv := fr.View(); fv != nil {
							extra += fmt.Sprintf(" viewport=%dx%d scrollY=%d", fv.Width(), fv.Height(), fv.ScrollY())
						}
					}
				}

			}
		}
	}
	line := fmt.Sprintf("[%s] %s%s\n  [paint] %s", time.Now().Format("15:04:05.000"), geo, extra,
		rendering.SnapshotComponentPaints())
	h.snapMu.Lock()
	defer h.snapMu.Unlock()
	if h.snapFile == nil {
		f, err := os.OpenFile("_layout_snap.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return
		}
		h.snapFile = f
	}
	fmt.Fprintln(h.snapFile, line)
	_ = h.snapFile.Sync()
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
// ─── hover style fast-path ────────────────────────────────
//
// A :hover switch normally triggers a FULL render-tree rebuild + layout +
// repaint (each O(content)). For a mouse move that only changes which element
// is hovered this is the dominant cost — with 10k+ render objects a single
// hover change took multiple seconds ("UI responds slowly as content grows").
//
// The fast path re-resolves the :hover-affected styles of ONLY the old/new
// hovered elements, swaps them onto the existing render objects, and marks
// just their regions dirty. Layout is only re-run when the computed style
// change actually affects box geometry (fonts / box model / display / flex);
// pure visual changes (color / background / shadow / opacity) repaint the
// element's rect in place.

// hoverStyleFastPath applies the :hover style change for oldEl → newEl without
// rebuilding the render tree. Falls back to a full rebuild+layout when the
// style change affects layout (geometry).
func (h *Host) hoverStyleFastPath(rv *rendering.RenderView, fr *page.Frame, oldEl, newEl *dom.Element) {
	if fr == nil || rv == nil {
		return
	}
	resolver := fr.Resolver()
	if resolver == nil {
		return
	}
	// ★ :hover 冒泡匹配：el 自身 hovered 时其全部祖先经 hasHoveredDescendant
	// 也匹配 :hover。因此 hover 切换必须重算 old/new 两元素的完整祖先链
	// （去重），否则旧祖先的 :hover 样式残留缓存（视觉上多个高亮并存）、
	// 新祖先的 :hover 也不生效。之前只重算 oldEl/newEl 两个叶子节点，
	// 悬停从一处移到另一处时旧容器高亮残留。
	seen := map[*dom.Element]bool{}
	var els []*dom.Element
	for _, el := range []*dom.Element{oldEl, newEl} {
		for e := el; e != nil; e = e.ParentElement() {
			if !seen[e] {
				seen[e] = true
				els = append(els, e)
			}
		}
	}
	layoutDirty := false
	for _, el := range els {
		resolver.Invalidate(el)
		newCS := resolver.ResolveElement(el)
		if newCS == nil {
			continue
		}
		ro := findRenderObjectForNode(rendering.RenderObject(rv), el)
		if ro == nil {
			continue
		}
		oldCS := ro.Style()
		if !layoutDirty && layoutAffectingChanged(oldCS, newCS) {
			layoutDirty = true
		}
		ro.SetStyle(newCS)
		// Mark the element's region dirty for a local repaint (pad for
		// hover shadows/borders). Scroll containers disable the dirty check
		// at paint time, so correctness is preserved there.
		if lb := ro.LayoutBox(); lb != nil && rv.LayoutState() != nil {
			g := rv.LayoutState().GeometryForBox(lb)
			r := rendering.Rect{X: g.Left() - 2, Y: g.Top() - 2, Width: g.BorderBoxWidth() + 4, Height: g.BorderBoxHeight() + 4}
			if r.Width > 0 && r.Height > 0 {
				rv.MarkDirty(r)
			}
		} else {
			rv.MarkAllDirty()
		}
	}
	if layoutDirty {
		fr.SetNeedsLayout(true)
	}
}

// layoutAffectingChanged reports whether a computed-style change from a to b
// would alter box geometry (requiring a layout pass). Pure visual properties
// (color, background, shadow, opacity, transform…) return false — they only
// need a repaint. Positioned offsets (top/left/…) are not tracked here; hover
// rules never change them in practice.
func layoutAffectingChanged(a, b *style.ComputedStyle) bool {
	if a == nil || b == nil {
		return true
	}
	// Inline formatting / font / wrapping (inherit into text layout).
	if a.FontSize != b.FontSize || a.FontFamily != b.FontFamily || a.FontWeight != b.FontWeight ||
		a.FontStyle != b.FontStyle || a.LineHeight != b.LineHeight || a.LetterSpacing != b.LetterSpacing ||
		a.WordSpacing != b.WordSpacing || a.TextIndent != b.TextIndent || a.TextAlign != b.TextAlign ||
		a.WhiteSpace != b.WhiteSpace || a.WordBreak != b.WordBreak || a.OverflowWrap != b.OverflowWrap ||
		a.WritingMode != b.WritingMode || a.Direction != b.Direction || a.Visibility != b.Visibility {
		return true
	}
	// Box model / positioning / display / flex / grid / multi-column.
	if a.Display != b.Display || a.Position != b.Position || a.Float != b.Float || a.Clear != b.Clear ||
		a.OverflowX != b.OverflowX || a.OverflowY != b.OverflowY ||
		a.Width != b.Width || a.Height != b.Height || a.MinWidth != b.MinWidth || a.MinHeight != b.MinHeight ||
		a.MaxWidth != b.MaxWidth || a.MaxHeight != b.MaxHeight ||
		a.MarginTop != b.MarginTop || a.MarginRight != b.MarginRight || a.MarginBottom != b.MarginBottom || a.MarginLeft != b.MarginLeft ||
		a.PaddingTop != b.PaddingTop || a.PaddingRight != b.PaddingRight || a.PaddingBottom != b.PaddingBottom || a.PaddingLeft != b.PaddingLeft ||
		a.BorderTopWidth != b.BorderTopWidth || a.BorderRightWidth != b.BorderRightWidth ||
		a.BorderBottomWidth != b.BorderBottomWidth || a.BorderLeftWidth != b.BorderLeftWidth ||
		a.BorderTopStyle != b.BorderTopStyle || a.BorderRightStyle != b.BorderRightStyle ||
		a.BorderBottomStyle != b.BorderBottomStyle || a.BorderLeftStyle != b.BorderLeftStyle ||
		a.BoxSizing != b.BoxSizing || a.BorderRadius != b.BorderRadius ||
		a.FlexDirection != b.FlexDirection || a.FlexWrap != b.FlexWrap ||
		a.JustifyContent != b.JustifyContent || a.AlignItems != b.AlignItems || a.AlignContent != b.AlignContent ||
		a.Gap != b.Gap || a.RowGap != b.RowGap || a.ColumnGap != b.ColumnGap ||
		a.FlexBasis != b.FlexBasis || a.FlexGrow != b.FlexGrow || a.FlexShrink != b.FlexShrink || a.Order != b.Order ||
		a.AlignSelf != b.AlignSelf || a.JustifySelf != b.JustifySelf ||
		a.VerticalAlign != b.VerticalAlign || a.ZIndex != b.ZIndex ||
		a.ColumnCount != b.ColumnCount || a.ColumnWidth != b.ColumnWidth ||
		a.GridTemplateColumns != b.GridTemplateColumns || a.GridTemplateRows != b.GridTemplateRows {
		return true
	}
	return false
}

func (h *Host) processEvents(rv *rendering.RenderView) {
	// Consume buffered IME events (composition updates, committed chars,
	// composition end) and apply them to the focused form control. The
	// Win32 IME handler buffers these in its subclassed WndProc; without
	// this per-frame poll, composition text / confirmed characters never
	// reach the element value ("can't type anything").
	if imeEvs := h.win.PollIMEEvents(); len(imeEvs) > 0 {
		h.applyIMEEvents(imeEvs)
	}
	for _, ev := range h.win.PollEvents() {
		switch ev.Type {
		case window.EventResize:
			h.wv.Resize(h.win.Width(), h.win.Height())
			h.needsResizeDump = true
			log.Printf("[resize-ev] fb=%dx%d css=%dx%d scale=(%.2f,%.2f) viewport=%dx%d",
				h.win.FramebufferWidth(), h.win.FramebufferHeight(),
				h.win.Width(), h.win.Height(),
				func() float64 { a, _ := h.win.ContentScale(); return a }(), 0.0,
				func() int {
					if fv := h.wv.Page().MainFrame().View(); fv != nil {
						return fv.Width()
					}
					return -1
				}(), 0)
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
			// ★ ScrollTargetAt 解析滚动容器 + 其所属 RenderView：iframe 内
			// 滚动时 box 属于子 Frame，偏移表在子 RenderView（偏移读写与
			// scrollbar metrics 必须用子 rv——主 rv 查不到子 box 的偏移，
			// 写入也不生效，因为子文档绘制读自己的偏移表）。
			tgt := rv.ScrollTargetAt(cssX, cssY)
			if tgt.Box != nil {
				scrollBox := tgt.Box
				srv := tgt.RV
				if os.Getenv("WB_SCROLL_DEBUG") != "" {
					boxName := "?"
					if n := scrollBox.Node(); n != nil {
						if el, ok := n.(*dom.Element); ok {
							boxName = el.LocalName() + "." + el.GetAttribute("class")
						}
					}
					log.Printf("[scroll] hit-box=%s at (%.0f,%.0f)\n", boxName, cssX, cssY)
				}
				log.Printf("[scroll] per-box hit at (%.0f,%.0f) scrollY=%d\n",
					cssX, cssY, h.wv.Page().MainFrame().View().ScrollY())
				if os.Getenv("WB_SCROLL_DEBUG") != "" && h.wv.JSInterpreter() != nil {
					if v, err := h.wv.JSInterpreter().RunJS(`(function(){
						try {
							var msgs = document.querySelectorAll('.msg-item, .message, .chat-msg, .msg-row, .tl-row');
							var convs = document.querySelectorAll('.conv-item');
							return 'domMsgs=' + msgs.length + ' domConvs=' + convs.length;
						} catch(e){ return 'err:'+e.message; }
					})()`); err == nil {
						fmt.Fprintf(os.Stderr, "[scroll] %s\n", v.ToString())
					}
				}
				sx, sy := srv.BoxScrollOffset(scrollBox)
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
				// Clamp to the shared scrollbar geometry (content-box
				// viewport, not the padding box) so wheel scrolling reaches
				// the same max as the thumb — the painter's max is
				// totalH - viewH with viewH = padding-box minus padding.
				if vm := rendering.VerticalScrollbarMetrics(srv, scrollBox); vm.OK {
					maxY := int(vm.MaxScroll)
					if newSy > maxY {
						newSy = maxY
					}
				} else if newSy > 0 {
					newSy = 0
				}
				if hm := rendering.HorizontalScrollbarMetrics(srv, scrollBox); hm.OK {
					maxX := int(hm.MaxScroll)
					if newSx > maxX {
						newSx = maxX
					}
				} else if newSx > 0 {
					newSx = 0
				}
				// Wheel scroll is SMOOTHED (browser-like): record the target
				// and let the main loop interpolate toward it every frame.
				// A new wheel event while animating simply re-targets from
				// the current interpolated position.
				if !h.smoothActive || h.smoothBox != scrollBox {
					h.smoothBox = scrollBox
					h.smoothRV = srv
					h.smoothCurX, h.smoothCurY = sx, sy
				}
				h.smoothTarX = float64(newSx)
				h.smoothTarY = float64(newSy)
				h.smoothActive = true
				h.smoothLast = time.Now()
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
				// ★ 鼠标移动必须标记重绘：滚动条 thumb 的 hover 高亮由
				// cursor 位置决定（renderpipeline.go isHover 判定），而 hover
				// 元素可能未变（移入/移出滚动条轨道不改 DOM hover）。按需
				// 渲染下若不标记 dirty，鼠标在滚动条上滑动时画面不更新。
				// 开销：鼠标移动期间全量重绘（与按需渲染前的行为一致），
				// 鼠标静止时零重绘。
				rv.MarkAllDirty()
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
					oldHover := h.hoveredEl
					if oldHover != nil {
						oldHover.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					// ★ hover 快速路径：只重算新旧 hover 元素的样式，不重建
					// 整个渲染树（内容多时全树 rebuild+layout 需数秒）。
					// 仅当样式变化影响几何时才回退全树布局。
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							h.hoverStyleFastPath(rv, fr, oldHover, newEl)
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
					// Vertical drag: cursor delta → scroll offset delta,
					// using the same geometry as the painter (shared
					// ScrollbarMetrics) so the thumb tracks the cursor
					// 1:1 and the content follows the thumb.
					dy := cssY - h.scrollbarDragStart
					m := rendering.VerticalScrollbarMetrics(rv, h.scrollbarDragBox)
					if m.OK {
						travel := m.TrackLen - m.ThumbLen
						if travel < 1 {
							travel = 1
						}
						newSy := h.scrollbarDragScroll + dy*(m.MaxScroll/travel)
						if newSy < 0 {
							newSy = 0
						}
						if newSy > m.MaxScroll {
							newSy = m.MaxScroll
						}
						// Preserve the horizontal offset (a vertical drag
						// must never reset a box's sx).
						sx, _ := rv.BoxScrollOffset(h.scrollbarDragBox)
						rv.SetBoxScrollOffset(h.scrollbarDragBox, sx, newSy)
						h.dispatchScrollEvent(h.scrollbarDragBox)
					}
				} else {
					// Horizontal drag: cursor delta → scroll offset delta,
					// using the shared ScrollbarMetrics (same geometry as
					// the painter) so the thumb tracks the cursor 1:1.
					dx := cssX - h.scrollbarDragStart
					m := rendering.HorizontalScrollbarMetrics(rv, h.scrollbarDragBox)
					if m.OK {
						travel := m.TrackLen - m.ThumbLen
						if travel < 1 {
							travel = 1
						}
						newSx := h.scrollbarDragScroll + dx*(m.MaxScroll/travel)
						if newSx < 0 {
							newSx = 0
						}
						if newSx > m.MaxScroll {
							newSx = m.MaxScroll
						}
						if setScrollXFor(rv, h.scrollbarDragBox, newSx) {
							h.markScrollDirty()
						}
					}
				}
				// ── Hover tracking ──
				newEl := rendering.HitTest(rv, cssX, cssY, "")
				if newEl != h.hoveredEl {
					oldHover := h.hoveredEl
					if oldHover != nil {
						oldHover.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							h.hoverStyleFastPath(rv, fr, oldHover, newEl)
						}
					}
				}
			}
		case window.EventMouseButton:
			if os.Getenv("WB_EVT_DEBUG") != "" {
				log.Printf("[evt] MouseButton action=%d at=(%.0f,%.0f) cur=(%.0f,%.0f) win=%dx%d",
					ev.Action, ev.X, ev.Y, h.cursorX, h.cursorY, h.win.Width(), h.win.Height())
			}
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
						h.smoothActive = false // thumb drag is 1:1, not smoothed
						h.scrollbarDragging = true
						h.scrollbarDragBox = box
						h.scrollbarDragAxis = true // vertical
						_, sy := rv.BoxScrollOffset(box)
						h.scrollbarDragStart = cssY
						h.scrollbarDragScroll = sy
						break
					}
					if scrollHit.IsHThumb {
						h.smoothActive = false // thumb drag is 1:1, not smoothed
						h.scrollbarDragging = true
						h.scrollbarDragBox = box
						h.scrollbarDragAxis = false // horizontal
						h.scrollbarDragStart = cssX
						h.scrollbarDragScroll = scrollXFor(rv, box)
						break
					}
					// ── Arrow buttons → line scroll (clamped to the same
					// range as the thumb, shared geometry) ──
					const lineStep = 16.0
					if scrollHit.IsVUpArrow {
						sx, sy := rv.BoxScrollOffset(box)
						sy -= lineStep
						if sy < 0 {
							sy = 0
						}
						if m := rendering.VerticalScrollbarMetrics(rv, box); m.OK && sy > m.MaxScroll {
							sy = m.MaxScroll
						}
						rv.SetBoxScrollOffset(box, sx, sy)
						h.dispatchScrollEvent(box)
						h.markScrollDirty()
						break
					}
					if scrollHit.IsVDownArrow {
						sx, sy := rv.BoxScrollOffset(box)
						sy += lineStep
						if m := rendering.VerticalScrollbarMetrics(rv, box); m.OK {
							if sy < 0 {
								sy = 0
							}
							if sy > m.MaxScroll {
								sy = m.MaxScroll
							}
						}
						rv.SetBoxScrollOffset(box, sx, sy)
						h.dispatchScrollEvent(box)
						h.markScrollDirty()
						break
					}
					if scrollHit.IsHLeftArrow {
						if setScrollXFor(rv, box, scrollXFor(rv, box)-lineStep) {
							h.markScrollDirty()
						}
						break
					}
					if scrollHit.IsHRightArrow {
						if setScrollXFor(rv, box, scrollXFor(rv, box)+lineStep) {
							h.markScrollDirty()
						}
						break
					}
					// ── Track click (non-thumb) → page scroll ──
					if scrollHit.IsVTrack {
						sx, sy := rv.BoxScrollOffset(box)
						pb := box.PaddingBoxRect()
						// Determine click position relative to the thumb
						// center using the SAME geometry as the painter.
						m := rendering.VerticalScrollbarMetrics(rv, box)
						if m.OK {
							syRatio := sy / m.MaxScroll
							if syRatio < 0 {
								syRatio = 0
							}
							if syRatio > 1 {
								syRatio = 1
							}
							thumbTrackSpace := m.TrackLen - m.ThumbLen
							thumbCenterY := pb.Y + 12.0 + 5.0 + syRatio*thumbTrackSpace + m.ThumbLen/2
							pageH := m.ViewLen // one page = the visible content height
							if cssY < thumbCenterY {
								sy -= pageH
							} else {
								sy += pageH
							}
							if sy < 0 {
								sy = 0
							}
							if sy > m.MaxScroll {
								sy = m.MaxScroll
							}
							rv.SetBoxScrollOffset(box, sx, sy)
							h.dispatchScrollEvent(box)
						}
						break
					}
					if scrollHit.IsHTrack {
						pb := box.PaddingBoxRect()
						// Same geometry as the painter (shared metrics).
						m := rendering.HorizontalScrollbarMetrics(rv, box)
						if m.OK {
							hSx := scrollXFor(rv, box)
							sxRatio := hSx / m.MaxScroll
							if sxRatio < 0 {
								sxRatio = 0
							}
							if sxRatio > 1 {
								sxRatio = 1
							}
							thumbTrackSpace := m.TrackLen - m.ThumbLen
							thumbCenterX := pb.X + 12.0 + 5.0 + sxRatio*thumbTrackSpace + m.ThumbLen/2
							pageW := m.ViewLen // one page = visible content width
							newSx := hSx - pageW
							if cssX >= thumbCenterX {
								newSx = hSx + pageW
							}
							if newSx < 0 {
								newSx = 0
							}
							if newSx > m.MaxScroll {
								newSx = m.MaxScroll
							}
							if setScrollXFor(rv, box, newSx) {
								h.markScrollDirty()
							}
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
						if debugPaintLog {
							log.Printf("[dbg/click] Press at css=(%.0f,%.0f) imeFocusedEl=%v hitEl=%v localName=%q type=%q",
								cssX, cssY, h.imeFocusedEl != nil, hitEl != nil,
								func() string { if hitEl != nil { return hitEl.LocalName() }; return "" }(),
								func() string { if hitEl != nil { return hitEl.GetAttribute("type") }; return "" }())
						}
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
			// ★ Text editing keys (backspace/delete/arrows/home/end) take
			// priority over scrolling when a form control is focused. Without
			// this, Backspace/Delete did nothing and arrows scrolled the page.
			edited := false
			if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) && !h.imeComposing {
				if ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat) {
					switch ev.Key {
					case int(glfw.KeyBackspace):
						h.deleteFocusedChar(false)
						edited = true
					case int(glfw.KeyDelete):
						h.deleteFocusedChar(true)
						edited = true
					case int(glfw.KeyLeft):
						h.moveFocusedCaret(-1)
						edited = true
					case int(glfw.KeyRight):
						h.moveFocusedCaret(1)
						edited = true
					case int(glfw.KeyHome):
						h.setFocusedCaret(0)
						edited = true
					case int(glfw.KeyEnd):
						h.setFocusedCaret(-1) // clamps to end
						edited = true
					case int(glfw.KeyEnter), int(glfw.KeyKPEnter):
						// Enter inserts a newline only in multi-line
						// controls (textarea); single-line inputs submit
						// (not implemented here, so the key is a no-op).
						if h.imeFocusedEl.LocalName() == "textarea" {
							h.pasteIntoFocused("\n")
							edited = true
						}
					}
				}
			}
			if edited {
				break
			}
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
					// ★ 键盘滚动同样路由到子 Frame：iframe 内滚动容器属于
					// 子 RenderView，偏移读写用子 rv（同滚轮路径）。
					tgt := rv.ScrollTargetAt(cssX, cssY)
					if tgt.Box != nil {
						scrollBox := tgt.Box
						srv := tgt.RV
						sx, sy := srv.BoxScrollOffset(scrollBox)
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
								cw, _ := srv.BoxContentSize(scrollBox)
								if newSx < 0 {
									newSx = 0
								}
								if maxSx := cw - pb.Width; newSx > maxSx {
									newSx = maxSx
								}
								srv.SetBoxScrollOffset(scrollBox, newSx, sy)
								h.dispatchScrollEvent(scrollBox)
							} else {
								newSy := sy + delta
								_, ch := srv.BoxContentSize(scrollBox)
								if newSy < 0 {
									newSy = 0
								}
								if maxSy := ch - pb.Height; newSy > maxSy {
									newSy = maxSy
								}
								srv.SetBoxScrollOffset(scrollBox, sx, newSy)
								h.dispatchScrollEvent(scrollBox)
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
				case int(glfw.KeyA):
					// Ctrl+A: select all text in the focused form control.
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						val := focusedElementValue(h.imeFocusedEl)
						runes := []rune(val)
						rendering.FocusedFormControlSel = &rendering.FormControlSelection{
							Start: 0,
							End:   len(runes),
						}
						h.wv.RebuildRenderTree()
					}
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
	if os.Getenv("WB_EVT_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[click] handleClick entered rv=%v at=(%.0f,%.0f) action=%d\n",
			rv != nil, ev.X, ev.Y, ev.Action)
	}
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
	if os.Getenv("WB_EVT_DEBUG") != "" {
		en := "<nil>"
		if el != nil {
			en = el.LocalName() + "." + el.GetAttribute("class")
		}
		fmt.Fprintf(os.Stderr, "[click] hit-onclick=%s at (%.0f,%.0f)\n", en, clickCSSX, clickY)
	}
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
		// ★ Flush the JS microtask queue FIRST — Vue's reactive updates
		// (state.activeActivity etc.) are scheduled as Promise.then
		// microtasks, so without this the DOM still shows the OLD state
		// and every click looks like it "does nothing".
		h.processEventLoop()
		// ★ Vue 的 scheduler（nextTick/component update）用 Promise 微任务
		// （goja 队列），与 processEventLoop 驱动的 queueMicrotask（jsc 队列）
		// 是两套：dispatch 时 jsListener 里已 RunJobs 一次，但 Vue 的
		// flushJobs 可能在 await 恢复链更后面，这里再补一次确保触发。
		if h.wv.JSInterpreter() != nil {
			interp := h.wv.JSInterpreter()
			// ★ 智能等待（替代原固定 8×15ms=120ms sleep）：
			//   事件派发后 Vue 的 async 链（@click → await apiPost →
			//   await loadXXX …）由 RunJobs（goja microtask）+ ProcessTasks
			//   （宏任务/rAF）交替驱动。普通点击无异步链，PendingTasks
			//   立即排空 → 提前退出（省 120ms 固定延迟，点击响应更快）；
			//   切换工作区等长链持续驱动直到排空或超时（上限保护）。
			//   Vue 的 nextTick 是 Promise.microtask，RunJobs 一次会清空
			//   队列；仅当链中调度了宏任务（setTimeout/rAF）才需要
			//   ProcessTasks 补一轮，因此无需人为固定 sleep。
			el := interp.GetEventLoop()
			deadline := time.Now().Add(180 * time.Millisecond)
			for {
				interp.RunJobs()
				h.processEventLoop()
				if el == nil || el.PendingTasks() == 0 {
					break
				}
				if time.Now().After(deadline) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		h.wv.RebuildRenderTree()
		if os.Getenv("WB_EVT_DEBUG") != "" && h.wv.JSInterpreter() != nil {
			if v, err := h.wv.JSInterpreter().RunJS(`(function(){
				try {
					var out = [];
					var items = document.querySelectorAll('.conv-item');
					var titles = [];
					for (var i=0;i<items.length && i<20;i++) {
						var t = items[i].textContent.replace(/\s+/g,' ').slice(0,20);
						var a = items[i].className.indexOf('active')>=0 ? 'A' : ' ';
						titles.push(i + a + ':' + t);
					}
					out.push('items=' + items.length + ' [' + titles.join(' | ') + ']');
					// 找当前 active 的 title
					var actTitle = '';
					for (var i=0;i<items.length;i++) if (items[i].className.indexOf('active')>=0) { actTitle = items[i].textContent.slice(0,25); break; }
					out.push('activeTitle=' + actTitle);
					return out.join(' | ');
				} catch(e){ return 'err:'+e.message; }
			})()`); err == nil {
				fmt.Fprintf(os.Stderr, "[click] post-dispatch %s\n", v.ToString())
			}
		}
		if rendering.FocusedFormControl != prevFocus {
			h.processEventLoop()
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
		h.processEventLoop()
		// ★ Vue 的 scheduler（nextTick/组件更新）用 Promise 微任务
		// （goja 队列），与 processEventLoop 驱动的 queueMicrotask（jsc 队列）
		// 是两套：dispatch 后必须补 goja RunJobs，否则 Vue 的
		// flushJobs 不跑，DOM 永远停留在旧状态（点击"看起来没反应"）。
		if h.wv.JSInterpreter() != nil {
			interp := h.wv.JSInterpreter()
			interp.RunJobs()
			// 智能等待：点击 handler 里的 async 链（@click → await fetch →
			// Vue 更新）立即推进到排空，避免依赖下一帧渲染循环才推进。
			if el0 := interp.GetEventLoop(); el0 != nil {
				deadline := time.Now().Add(120 * time.Millisecond)
				for {
					interp.RunJobs()
					h.processEventLoop()
					if el0.PendingTasks() == 0 || time.Now().After(deadline) {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		}
		h.wv.RebuildRenderTree()
		if os.Getenv("WB_EVT_DEBUG") != "" && h.wv.JSInterpreter() != nil {
			if v, err := h.wv.JSInterpreter().RunJS(`(function(){
				try {
					var out = [];
					// 1. Vue devtools hook
					var vhook = window.__VUE__;
					out.push('VUE=' + typeof vhook);
					// 2. app 实例
					var appEl = document.querySelector('#app');
					var app = appEl && appEl.__vue_app__;
					out.push('app=' + (app ? 'yes' : 'no'));
					// 3. 遍历组件树找 RightPanel 实例读 setupState.state.currentConvId
					if (app) {
						var found = [];
						var walk = function(inst, depth) {
							if (!inst || depth > 6) return;
							if (inst.setupState && inst.setupState.state && inst.setupState.state.currentConvId !== undefined) {
								found.push('cur=' + inst.setupState.state.currentConvId + ' msgs=' + (inst.setupState.state.messages||[]).length);
							}
							if (inst.setupState && Object.keys(inst.setupState).length) {
								var ks = Object.keys(inst.setupState);
								if (ks.indexOf('switchConv') >= 0) {
									found.push('hasSwitchConv cur=' + (inst.setupState.state ? inst.setupState.state.currentConvId : '?'));
								}
							}
							if (inst.subTree && inst.subTree.component) walk(inst.subTree.component, depth+1);
							if (inst.subTree && inst.subTree.children) {
								for (var i=0;i<inst.subTree.children.length;i++) {
									if (inst.subTree.children[i] && inst.subTree.children[i].component) walk(inst.subTree.children[i].component, depth+1);
								}
							}
						};
						walk(app._instance, 0);
						out.push('FOUND=' + (found.length ? found.join(';') : 'none'));
					}
					// 4. DOM active 索引
					var items = document.querySelectorAll('.conv-item');
					var active = [];
					for (var i=0;i<items.length;i++) if (items[i].className.indexOf('active')>=0) active.push(i);
					out.push('items=' + items.length + ' active=' + JSON.stringify(active));
					return out.join(' | ');
				} catch(e){ return 'err:'+e.message; }
			})()`); err == nil {
				fmt.Fprintf(os.Stderr, "[click] post-dispatch %s\n", v.ToString())
			}
		}
		return
	}
	if strings.HasPrefix(onclickVal, "js:") {
		_, _ = h.wv.EvalJS(onclickVal[3:])
		h.processEventLoop()
		h.wv.RebuildRenderTree()
		return
	}
	el.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
	if h.clickHandler != nil {
		h.clickHandler(el, onclickVal, clickCSSX, clickCSSY)
	}
	h.processEventLoop()
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
	if formControlEditLimits(h.imeFocusedEl) == 0 {
		return // readonly/disabled: no paste
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

	if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
		newVal = truncateToMaxLen(newVal, maxLen)
		if newOffset > len([]rune(newVal)) {
			newOffset = len([]rune(newVal))
		}
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
	h.ensureFocusedCaretVisible()
}

// deleteFocusedChar deletes one character (or the active selection) in the
// focused form control. forward=true deletes after the caret (Delete key),
// forward=false deletes before it (Backspace), matching browser behavior.
func (h *Host) deleteFocusedChar(forward bool) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	val := focusedElementValue(el)
	runes := []rune(val)
	start, end := len(runes), len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		start, end = sel.Start, sel.End
		if start > end {
			start, end = end, start
		}
	}
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start == end {
		if forward {
			if end >= len(runes) {
				return // nothing after the caret
			}
			end++
		} else {
			if start <= 0 {
				return // nothing before the caret
			}
			start--
		}
	}
	newVal := string(runes[:start]) + string(runes[end:])
	setFocusedElementValue(el, newVal)
	h.imeInputText = newVal
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: start, End: start}

	inputType := "deleteContentForward"
	if !forward {
		inputType = "deleteContentBackward"
	}
	el.DispatchEvent(dom.NewInputEvent(inputType, "", false))
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	h.wv.RebuildRenderTree()
}

// moveFocusedCaret moves the caret of the focused form control by delta
// runes (negative = left). Home/End are handled via moveFocusedCaretTo.
func (h *Host) moveFocusedCaret(delta int) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	runes := []rune(focusedElementValue(el))
	pos := len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		pos = sel.Start
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	h.setFocusedCaret(pos)
}

// moveFocusedCaretTo sets the caret of the focused form control to an
// absolute rune index.
func (h *Host) setFocusedCaret(pos int) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	runes := []rune(focusedElementValue(el))
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: pos, End: pos}
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
	h.ensureFocusedCaretVisible()
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
				if formControlEditLimits(h.imeFocusedEl) == 0 {
					continue // readonly/disabled: never start a composition
				}
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
			// Normalize Enter's CR (if a platform delivers it as a char)
			// to a newline in multi-line controls; single-line inputs drop it.
			if ev.Char == '\r' {
				if h.imeFocusedEl != nil && h.imeFocusedEl.LocalName() == "textarea" {
					char = "\n"
				} else {
					break
				}
			}
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
				if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
					newText = truncateToMaxLen(newText, maxLen)
				}
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
				if formControlEditLimits(h.imeFocusedEl) == 0 {
					break // readonly/disabled: ignore the keystroke
				}
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
				if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
					newText = truncateToMaxLen(newText, maxLen)
				}
				// Move caret after the inserted char (clamped by maxlength).
				caretPos := start + 1
				if n := len([]rune(newText)); caretPos > n {
					caretPos = n
				}
				rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: caretPos, End: caretPos}
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
				if os.Getenv("WB_SCROLL_DEBUG") != "" {
					log.Printf("[scroll/input] IME char=%q value → %q len=%d sel=%s", char, newText, len([]rune(newText)),
						fmt.Sprintf("Start=%d End=%d", rendering.FocusedFormControlSel.Start, rendering.FocusedFormControlSel.End))
				}
				if strings.EqualFold(h.imeFocusedEl.GetAttribute("contenteditable"), "true") {
					// ★ contenteditable（CodeMirror 6 输入区）：光标处插入单个字符，
					//   不全文替换（全文替换会抹掉 CM6 的 .cm-line/高亮 span 结构，
					//   且 CM6 发现文本未变不重建 → 布局永久破坏）。插入后派发
					//   insertText → CM6 readDOMChange 同步 state 并重建结构。
					if !bindings.InsertTextAtSelection(char) {
						break // 无有效 selection：跳过 DOM 修改（保住现有结构）
					}
					h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
				} else {
					setFocusedElementValue(h.imeFocusedEl, newText)
					needsRebuild = true
					h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
				}

				if wasComposing {
					// End composition
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", newText))
				}

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
		h.ensureFocusedCaretVisible()
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

// owner carries the given CSS class (used by the [skia] dialog probe).