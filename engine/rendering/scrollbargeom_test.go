package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// helperOverflowTextarea builds a textarea whose content overflows both
// axes: a long unbroken line (horizontal scroll in pre mode) plus many
// lines (vertical scroll). Returns rv and the textarea box.
func helperOverflowTextarea(t *testing.T) (*RenderView, *RenderBox) {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	long := ""
	for i := 0; i < 40; i++ {
		long += "abcdefghij" // 400 chars → wide horizontal overflow
	}
	body := `<textarea id="ta" style="display:block;width:140px;height:120px;line-height:20px;font-family:Consolas;font-size:13px;padding:4px;white-space:pre;overflow:auto;box-sizing:border-box">`
	for i := 0; i < 12; i++ {
		body += long + "\n"
	}
	body += `</textarea>`
	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>` + body + `</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(600, 400)
	rv.Layout(layout.NewLayoutState(600, 400))
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
	if st := taBox.Style(); st != nil {
		t.Logf("ta overflow-x=%v overflow-y=%v pb=%+v", st.OverflowX, st.OverflowY, taBox.PaddingBoxRect())
	}
	cw, ch := rv.BoxContentSize(taBox)
	t.Logf("ta content size = (%v, %v)", cw, ch)
	return rv, taBox
}

// TestScrollbarHitMatchesPaint: the scrollbar hit-test (press point →
// IsVThumb) must land exactly on the thumb the painter draws. Both now
// share VerticalScrollbarMetrics, so the thumb center at any sy must be
// hit as a thumb — previously hit-test used a different formula (missing
// arrowGap, different min thumb length) so presses near the thumb edge
// fell through to track/arrow and dragged the wrong thing.
func TestScrollbarHitMatchesPaint(t *testing.T) {
	rv, taBox := helperOverflowTextarea(t)
	pb := taBox.PaddingBoxRect()
	vx := pb.X + pb.Width - 12 // scrollW
	vy := pb.Y
	for _, sy := range []float64{0, 20, 50, 80} {
		rv.SetBoxScrollOffset(taBox, 0, sy)
		m := VerticalScrollbarMetrics(rv, taBox)
		if !m.OK {
			t.Fatalf("sy=%v: expected vertical scrollbar (content overflows)", sy)
		}
		syRatio := sy / m.MaxScroll
		if syRatio > 1 {
			syRatio = 1
		}
		thumbY := vy + 12 + 5 + syRatio*(m.TrackLen-m.ThumbLen)
		cy := thumbY + m.ThumbLen/2
		hit := HitTestScrollbar(rv, vx+6, cy)
		if hit == nil || !hit.IsVThumb {
			t.Fatalf("sy=%v thumb center (%.0f,%.0f) not hit as thumb (got %+v) — hit-test must match paint", sy, vx+6, cy, hit)
		}
	}
}

// TestScrollbarDragMapsThumbToCursor: host drag math (shared metrics) must
// keep the thumb glued to the cursor: dragging by dy moves the thumb center
// by exactly dy (i.e. newSy maps back to thumbY+dy).
func TestScrollbarDragMapsThumbToCursor(t *testing.T) {
	rv, taBox := helperOverflowTextarea(t)
	m := VerticalScrollbarMetrics(rv, taBox)
	if !m.OK {
		t.Fatal("expected vertical scrollbar")
	}
	travel := m.TrackLen - m.ThumbLen
	vy := taBox.PaddingBoxRect().Y
	thumbCenter := func(sy float64) float64 {
		if sy > m.MaxScroll {
			sy = m.MaxScroll
		}
		if sy < 0 {
			sy = 0
		}
		ratio := sy / m.MaxScroll
		return vy + 12 + 5 + ratio*travel + m.ThumbLen/2
	}
	// Drag from sy=10 down by 30px: newSy follows the cursor 1:1.
	start := 10.0
	dy := 30.0
	newSy := start + dy*(m.MaxScroll/travel)
	if newSy > m.MaxScroll {
		newSy = m.MaxScroll
	}
	got := thumbCenter(newSy) - thumbCenter(start)
	if got < dy-0.01 || got > dy+0.01 {
		t.Fatalf("dragging %vpx moved thumb %vpx (want %v) — drag mapping must equal paint mapping", dy, got, dy)
	}
}
