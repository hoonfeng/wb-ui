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

// helperRenderHSTextarea renders an overflow:auto textarea whose value is a
// single long line (white-space:pre → no soft wrap, horizontal scrollbar
// needed) through the full pipeline.
func helperRenderHSTextarea(t *testing.T, overflow, text string, cx, cy float64) (*graphics.Canvas, *RenderBox, float64) {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<textarea id="s" style="width:200px;height:60px;padding:4px 6px;overflow:` + overflow + `;white-space:pre;font-family:Consolas;font-size:13px">` + text + `</textarea>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(260, 160)
	state := layout.NewLayoutState(260, 160)
	rv.Layout(state)
	rv.SetCursorPos(cx, cy)

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

	canvas := graphics.NewCanvas(260, 160)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 260, Height: 160})
	if sbox != nil {
		cw, _ := rv.BoxContentSize(sbox)
		t.Logf("textarea pb=(%.0f,%.0f %.0fx%.0f) contentW=%.0f", sbox.PaddingBoxRect().X, sbox.PaddingBoxRect().Y, sbox.PaddingBoxRect().Width, sbox.PaddingBoxRect().Height, cw)
	}
	return canvas, sbox, 0
}

// TestScrollbarResidentNoHover: overflow:auto horizontal scrollbar is painted
// even with the cursor far away — Windows-style resident scrollbars, not
// macOS-style overlay auto-hiding.
func TestScrollbarResidentNoHover(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), -100, -100)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	pb := sbox.PaddingBoxRect()
	hy := pb.Y + pb.Height - 12
	if hy < 1 {
		hy = 1
	}
	foundTrack := false
	for x := 20; x < int(pb.Width)-10 && x < 200; x += 10 {
		px := canvas.PixelAt(x, int(hy)+3)
		if px.R > 230 && px.G > 230 && px.B > 230 {
			foundTrack = true
			break
		}
	}
	if !foundTrack {
		t.Fatalf("overflow:auto horizontal track not painted with cursor far away (pb=%+v)", pb)
	}
}

// TestScrollbarThumbNotCoverRightArrow: at maximum scroll the thumb's right
// edge stays left of the right-arrow button, so the arrow remains visible.
func TestScrollbarThumbNotCoverRightArrow(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), 60, 60)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	cw, _ := sbox.View().BoxContentSize(sbox)
	pb := sbox.PaddingBoxRect()
	viewW := pb.Width - 6 - 6 // padding 6px each side
	if el, ok := sbox.Node().(*dom.Element); ok {
		SetFormControlTextScroll(el, cw-viewW) // max scroll
	} else {
		t.Fatalf("scroll box has no element")
	}

	canvas2 := graphics.NewCanvas(260, 160)
	Paint(sbox.View(), canvas2, Rect{X: 0, Y: 0, Width: 260, Height: 160})
	defer canvas2.Release()

	// Right-arrow zone: last 12px of the track. Track y = pb bottom - 12.
	hy := int(pb.Y + pb.Height - 12)
	// Thumb grey is #A0A0A0 (R=160); the arrow triangle is #606060 (R=96)
	// and the track is white. A thumb covering the arrow shows R≈160.
	for x := int(pb.X+pb.Width-12) - 6; x < int(pb.X+pb.Width)-1; x++ {
		px := canvas2.PixelAt(x, hy+3)
		if px.R >= 140 && px.R <= 185 && px.G >= 140 && px.G <= 185 {
			t.Fatalf("thumb pixel (%d,%d) = %+v — thumb covers the right arrow", x, hy+3, px)
		}
	}
}

// TestHitTestScrollbarTextarea: the horizontal scrollbar of a textarea
// (pre-mode, long value) must be hit-testable — previously HitTestScrollbar
// only measured render-tree children, so a textarea (text lives in
// textContent, no children) always returned nil and scrollbar clicks fell
// through to the input control.
func TestHitTestScrollbarTextarea(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), 60, 60)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	rv := sbox.View()
	pb := sbox.PaddingBoxRect()
	st := sbox.Style()
	cw2, ch2 := rv.BoxContentSize(sbox)
	t.Logf("overflowY=%v overflowX=%v content=(%.1f,%.1f) view=(%.1f,%.1f)", st.OverflowY, st.OverflowX, cw2, ch2, pb.Width-6-6, pb.Height-4-4)
	trackY := pb.Y + pb.Height - 12 + 3 // inside the 12px horizontal track
	if trackY < pb.Y {
		trackY = pb.Y
	}
	// Left arrow zone: x in [pb.X, pb.X+12).
	hit := HitTestScrollbar(rv, pb.X+4, trackY)
	if hit == nil {
		t.Fatalf("left arrow not hit — scrollbar clicks fall through (trackY=%.0f)", trackY)
	}
	if !hit.IsHLeftArrow {
		t.Fatalf("left arrow zone → %+v, want IsHLeftArrow", hit)
	}
	// Right arrow zone: last 12px.
	hit = HitTestScrollbar(rv, pb.X+pb.Width-4, trackY)
	if hit == nil || !hit.IsHRightArrow {
		t.Fatalf("right arrow zone → %+v, want IsHRightArrow", hit)
	}
	// Track zone (middle, away from arrows and thumb at scroll=0): thumb
	// starts at hx+arrowSize with scroll=0, so the middle is track.
	hit = HitTestScrollbar(rv, pb.X+pb.Width/2, trackY)
	if hit == nil {
		t.Fatalf("track middle not hit")
	}
	if !hit.IsHTrack && !hit.IsHThumb {
		t.Fatalf("track middle → %+v, want IsHTrack/IsHThumb", hit)
	}
}

