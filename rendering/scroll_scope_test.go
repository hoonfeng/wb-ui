package rendering

import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestFormControlTextScrollScoped: the horizontal text scroll is scoped PER
// ELEMENT — setting a large scroll on a textarea must never leak into a
// sibling single-line input's offset.
func TestFormControlTextScrollScoped(t *testing.T) {
	elInput := dom.NewElement(nil, "input")
	elArea := dom.NewElement(nil, "textarea")

	if v := FormControlTextScroll(elArea); v != 0 {
		t.Fatalf("fresh textarea scroll = %v, want 0", v)
	}
	SetFormControlTextScroll(elArea, 123)
	if v := FormControlTextScroll(elArea); v != 123 {
		t.Fatalf("textarea scroll = %v, want 123", v)
	}
	if v := FormControlTextScroll(elInput); v != 0 {
		t.Fatalf("single-line input scroll = %v, want 0 (leaked from textarea)", v)
	}
	SetFormControlTextScroll(elInput, 30)
	if v := FormControlTextScroll(elArea); v != 123 {
		t.Fatalf("textarea scroll changed to %v after input scroll, want 123", v)
	}
	if v := FormControlTextScroll(nil); v != 0 {
		t.Fatalf("nil element scroll = %v, want 0", v)
	}
}

// helperBuildMixedControls builds a page with one single-line input and one
// pre-mode textarea (long value → horizontally scrollable), lays it out and
// returns the render view plus both elements.
func helperBuildMixedControls(t *testing.T) (*RenderView, *dom.Element, *dom.Element) {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<input id="in" style="width:120px;font-family:Consolas;font-size:13px" value="short">
		<textarea id="ta" style="width:120px;height:50px;white-space:pre;font-family:Consolas;font-size:13px">` + strings.Repeat("ab", 60) + `</textarea>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(260, 200)
	rv.Layout(layout.NewLayoutState(260, 200))
	rv.SetCursorPos(60, 60)

	var taEl, inEl *dom.Element
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if el, ok := o.Node().(*dom.Element); ok {
			switch el.GetAttribute("id") {
			case "ta":
				taEl = el
			case "in":
				inEl = el
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))
	if taEl == nil || inEl == nil {
		t.Fatalf("elements not found (ta=%v in=%v)", taEl != nil, inEl != nil)
	}
	return rv, inEl, taEl
}

// helperPaintScrolls paints the view and reads both controls' text-scroll
// offsets back.
func helperPaintScrolls(rv *RenderView) (inputScroll, areaScroll float64) {
	canvas := graphics.NewCanvas(260, 200)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 260, Height: 200})
	var inEl, taEl *dom.Element
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if el, ok := o.Node().(*dom.Element); ok {
			switch el.GetAttribute("id") {
			case "ta":
				taEl = el
			case "in":
				inEl = el
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))
	if taEl == nil || inEl == nil {
		return 0, 0
	}
	return FormControlTextScroll(inEl), FormControlTextScroll(taEl)
}

// TestTextareaScrollDoesNotLeakToInput: the old global
// FocusedFormControlTextScroll made a single-line input inherit whatever
// textarea scrolled last. With per-element scoping, painting a page where
// the textarea holds a large horizontal scroll must leave the sibling
// input's offset at 0.
func TestTextareaScrollDoesNotLeakToInput(t *testing.T) {
	rv, _, taEl := helperBuildMixedControls(t)
	// Drive the textarea's scroll directly (as the scrollbar drag would).
	SetFormControlTextScroll(taEl, 200)

	inputScroll, areaScroll := helperPaintScrolls(rv)
	if inputScroll != 0 {
		t.Fatalf("single-line input scroll = %v, want 0 — textarea scroll leaked into it", inputScroll)
	}
	if areaScroll != 200 {
		t.Fatalf("textarea scroll = %v, want 200 (its own value preserved)", areaScroll)
	}
}
