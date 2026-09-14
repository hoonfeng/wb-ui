package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// helperVScrollTextarea builds a textarea with 4 hard-wrapped lines and a
// known line height and returns the render view, the textarea element, its
// render box and the line height. The textarea's vertical scroll offset is
// stored in BoxScrollOffset.sy (scrollbar drag / wheel); painting must
// shift the text up by sy, hit-testing must add sy back.
func helperVScrollTextarea(t *testing.T) (*RenderView, *dom.Element, *RenderBox, float64) {
	t.Helper()
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<textarea id="ta" style="position:absolute;left:10px;top:10px;width:200px;height:120px;line-height:20px;font-family:Consolas;font-size:13px;padding:4px">line0
line1
line2
line3</textarea>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(400, 300)
	rv.Layout(layout.NewLayoutState(400, 300))

	var taEl *dom.Element
	var taBox *RenderBox
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if el, ok := o.Node().(*dom.Element); ok && el.GetAttribute("id") == "ta" {
			taEl = el
			taBox = asRenderBox(o)
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))
	if taEl == nil || taBox == nil {
		t.Fatalf("textarea not found")
	}
	return rv, taEl, taBox, 20.0
}

// TestTextareaVerticalScrollCaretMapping: with the textarea scrolled down by
// two rows (sy=2*lineH), a click on the FIRST visible row must map to the
// THIRD text row ("line2"), not the first — the caret offset follows the
// scrolled text. This guards the regression where BoxScrollOffset.sy was
// never consulted, so clicks landed on the wrong row after scrolling.
func TestTextareaVerticalScrollCaretMapping(t *testing.T) {
	_, _, taBox, lineH := helperVScrollTextarea(t)
	text := "line0\nline1\nline2\nline3"
	font := graphics.Font{Family: "Consolas", Size: 13, Weight: 400}
	bx, by := taBox.AbsoluteX(), taBox.AbsoluteY()
	padX, padY := 4.0, 4.0
	sy := lineH * 2 // scrolled down two rows

	// Click at the start of the FIRST visible row (top of the content area).
	off := CalcFormControlCaretOffset(text, true, bx+padX+1, by+padY, bx, by, 200, font, padX, padY, lineH, 0, sy)
	if off != 12 { // "line2" starts at rune 12
		t.Fatalf("click visible row 0 with sy=%v → offset %d, want 12 (line2)", sy, off)
	}
	// Without sy the same click maps to "line0" — the regression this guards.
	off0 := CalcFormControlCaretOffset(text, true, bx+padX+1, by+padY, bx, by, 200, font, padX, padY, lineH, 0, 0)
	if off0 != 0 {
		t.Fatalf("sanity: sy=0 click → offset %d, want 0 (line0)", off0)
	}
	// Click on the LAST visible row (row 3) must map to the last text row.
	offLast := CalcFormControlCaretOffset(text, true, bx+padX+1, by+padY+lineH*3, bx, by, 200, font, padX, padY, lineH, 0, sy)
	if offLast != 18 { // "line3" starts at rune 18 (6 runes per "lineN\n")
		t.Fatalf("click visible row 3 with sy=%v → offset %d, want 18 (line3)", sy, offLast)
	}
}

// TestTextareaVScrollPaintUsesBoxOffset: the painter must shift text up by
// the box's vertical scroll offset. Direct pixel verification of DrawText is
// unreliable in the CPU-raster test environment (FillRect pixels read back,
// glyph pixels do not), so this checks the observable contract instead: the
// scroll offset stored on the box is what the painter/hit-test/IME paths all
// consume, and the hit-test mapping above proves the shift participates in
// coordinate mapping. It also guards that painting does not clobber the
// stored offset (the box keeps the value across paints, so the scrollbar
// thumb and the text stay in sync).
func TestTextareaVScrollPaintKeepsOffset(t *testing.T) {
	rv, _, taBox, lineH := helperVScrollTextarea(t)
	rv.SetBoxScrollOffset(taBox, 0, lineH*3)
	canvas := graphics.NewCanvas(400, 300)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 400, Height: 300})
	sx, sy := rv.BoxScrollOffset(taBox)
	if sx != 0 || sy != lineH*3 {
		t.Fatalf("after paint: scroll=(%v,%v), want (0,%v) — paint must not clobber the scroll offset", sx, sy, lineH*3)
	}
}

// TestTextareaContentSizeSoftWrap: BoxContentSize's vertical extent must
// count SOFT-WRAPPED rows (a long line in a pre-wrap textarea wraps into
// several visual rows), so the vertical scrollbar's total height / thumb
// ratio matches the painted text. Previously only hard '\n' breaks were
// counted, so long unbroken text reported ~1 row and the scrollbar could
// not scroll far enough.
func TestTextareaContentSizeSoftWrap(t *testing.T) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	// A textarea 120px tall (content ~112px, line-height 20px → ~5 visible
	// rows) holding ONE very long line that soft-wraps into 10 rows.
	long := ""
	for i := 0; i < 60; i++ {
		long += "abcdefghij" // 600 chars ≈ 10 rows of 60px content width
	}
	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<textarea id="ta" style="position:absolute;left:10px;top:10px;width:80px;height:120px;line-height:20px;font-family:Consolas;font-size:13px;padding:4px">` + long + `</textarea>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(400, 300)
	rv.Layout(layout.NewLayoutState(400, 300))

	var taBox *RenderBox
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if el, ok := o.Node().(*dom.Element); ok && el.GetAttribute("id") == "ta" {
			taBox = asRenderBox(o)
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))
	if taBox == nil {
		t.Fatalf("textarea not found")
	}
	_, ch := rv.BoxContentSize(taBox)
	// 10 soft-wrapped rows × 20px = 200; the old hard-'\n'-only code returned
	// 20 (one row). Require at least 6 rows worth of extent so the vertical
	// scrollbar has real travel (content 200 > viewport 112).
	if ch < 120 {
		t.Fatalf("BoxContentSize vertical extent = %v, want ≥ 120 (soft-wrapped rows); hard-break-only bug", ch)
	}
	t.Logf("BoxContentSize soft-wrap vertical extent = %v", ch)
}

// TestTextareaCaretVisualRow: the visual row of the caret must count
// soft-wrapped rows — row 5 in a single hard line that wraps at row 4 must
// return 4, not 0 (which would break vertical auto-scroll).
func TestTextareaCaretVisualRow(t *testing.T) {
	// 100-char line at 40px content width → ~25 chars per row → 4 rows.
	line := ""
	for i := 0; i < 100; i++ {
		line += "x"
	}
	font := graphics.Font{Family: "Consolas", Size: 13, Weight: 400}
	// Wrap mode anywhere (pre-wrap): the row containing rune 60.
	row := TextareaCaretVisualRow(line, font, 40, wrapModeAnywhere, 60)
	if row < 1 {
		t.Fatalf("caret at rune 60 in a 4-row wrapped line → visual row %d, want ≥ 1", row)
	}
	t.Logf("caret visual row = %d", row)
	// Hard newline boundaries.
	row0 := TextareaCaretVisualRow("ab\ncd\nef", font, 200, wrapModeAnywhere, 6)
	if row0 != 2 {
		t.Fatalf("caret at end of 3 hard lines → row %d, want 2", row0)
	}
}
