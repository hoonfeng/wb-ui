package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// helperRenderScrollBox renders an overflow:auto box with overflowing content
// at cursor (cx, cy) through the full pipeline and returns the canvas.
func helperRenderScrollBox(t *testing.T, overflow string, cx, cy float64) *graphics.Canvas {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<div id="s" style="width:120px;height:80px;overflow-y:` + overflow + `">
			<div style="height:400px;background:#eee"></div>
		</div>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetViewportSize(120, 80)
	state := layout.NewLayoutState(120, 80)
	rv.Layout(state)
	rv.SetCursorPos(cx, cy)

	// Find the scroll box (div is a RenderBlockFlow; match by node).
	var sbox *RenderBox
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if sbox != nil {
			return
		}
		if el, ok2 := o.Node().(*dom.Element); ok2 && el.GetAttribute("id") == "s" {
			sbox = asRenderBox(o)
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))
	if sbox != nil {
		cw, ch := rv.BoxContentSize(sbox)
		_ = cw
		_ = ch
	}

	canvas := graphics.NewCanvas(120, 80)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 120, Height: 80})
	return canvas
}

// TestScrollbarAutoAlwaysVisible: overflow:auto shows the scrollbar whenever
// content overflows — Windows-style resident scrollbars (Chrome/Edge on
// Windows keep overflow:auto scrollbars visible; overlay auto-hiding is a
// macOS/touch feature). Cursor position is irrelevant.
func TestScrollbarAutoAlwaysVisible(t *testing.T) {
	canvas := helperRenderScrollBox(t, "auto", 60, 40)
	defer canvas.Release()
	found := false
	for y := 14; y < 70; y++ {
		px := canvas.PixelAt(110, y)
		if px.A != 0 && !(px.R > 230 && px.G > 230 && px.B > 230) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auto scrollbar not painted with cursor over box")
	}

	// Same box, cursor far away — still painted.
	canvas2 := helperRenderScrollBox(t, "auto", -50, -50)
	defer canvas2.Release()
	found = false
	for y := 14; y < 70; y++ {
		px := canvas2.PixelAt(110, y)
		if px.A != 0 && !(px.R > 230 && px.G > 230 && px.B > 230) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auto scrollbar not painted with cursor far away")
	}
}

// TestScrollbarAlwaysVisible: overflow:scroll shows the scrollbar even with
// the cursor far away.
func TestScrollbarAlwaysVisible(t *testing.T) {
	canvas := helperRenderScrollBox(t, "scroll", -50, -50)
	defer canvas.Release()
	found := false
	for y := 14; y < 70; y++ {
		px := canvas.PixelAt(110, y)
		if px.A != 0 && !(px.R > 230 && px.G > 230 && px.B > 230) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("scroll scrollbar not painted with cursor outside")
	}
}
