package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// paintClipBox paints a 80x80 red box (with optional clip-path) via the
// background pipeline.
func paintClipBox(t *testing.T, clip string) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(100, 100)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{R: 255, G: 0, B: 0, A: 255}
	if clip != "" {
		st.SetProperty("clip-path", clip)
	}
	box := NewRenderBox(el, st)
	box.SetLocation(10, 10)
	box.SetSize(80, 80)
	paintObjectBackground(box, info)
	return canvas
}

// TestClipPathInset: inset(20px) shrinks the painted box by 20px on all
// sides — the inset border stays empty, the center stays red.
func TestClipPathInset(t *testing.T) {
	canvas := paintClipBox(t, "inset(20px)")
	defer canvas.Release()
	// Box at (10,10,80,80), inset 20 → painted area (30,30)-(70,70).
	if px := canvas.PixelAt(50, 50); px.R != 255 {
		t.Fatalf("center (50,50) = %+v, want red", px)
	}
	for _, p := range []struct{ x, y int }{{20, 20}, {25, 50}, {50, 25}, {75, 50}} {
		if px := canvas.PixelAt(p.x, p.y); px.A != 0 {
			t.Fatalf("inset area (%d,%d) = %+v, want transparent", p.x, p.y, px)
		}
	}
}

// TestClipPathCircle: circle(50%) clips the square to an inscribed circle —
// corners empty, center red.
func TestClipPathCircle(t *testing.T) {
	canvas := paintClipBox(t, "circle(50%)")
	defer canvas.Release()
	// Box center (50,50), radius 40. Center red; corner (12,12) outside.
	if px := canvas.PixelAt(50, 50); px.R != 255 {
		t.Fatalf("center (50,50) = %+v, want red", px)
	}
	if px := canvas.PixelAt(12, 12); px.A != 0 {
		t.Fatalf("corner (12,12) = %+v, want transparent (circle clip)", px)
	}
}

// TestClipPathInsetShadow: box-shadow is drawn inside PaintBackground, so an
// inset clip also trims the shadow spill.
func TestClipPathInsetShadow(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{R: 255, G: 0, B: 0, A: 255}
	st.SetProperty("box-shadow", "30px 30px 0px green")
	st.SetProperty("clip-path", "inset(10px)")
	box := NewRenderBox(el, st)
	box.SetLocation(10, 10)
	box.SetSize(80, 80)
	paintObjectBackground(box, info)
	defer canvas.Release()

	// Shadow offset (30,30) from the box would spill to (120,120) without the
	// clip; inside the inset box (20..90) the shadow area (40..90) is green.
	// Outside the inset box (e.g. 15,15) stays empty.
	if px := canvas.PixelAt(50, 50); px.R != 255 {
		t.Fatalf("center (50,50) = %+v, want red", px)
	}
	if px := canvas.PixelAt(15, 15); px.A != 0 {
		t.Fatalf("outside inset (15,15) = %+v, want transparent", px)
	}
	// Shadow lands inside the box (offset 30,30): at (50,50) red bg wins? No —
	// shadow paints first, background over it. Check shadow edge region that
	// is outside the background? All within box. Verify shadow+clip don't
	// leak outside: (95,95) transparent.
	if px := canvas.PixelAt(95, 95); px.A != 0 {
		t.Fatalf("shadow clipped (95,95) = %+v, want transparent", px)
	}
}

// TestClipPathPolygon: triangle polygon clips the box — inside the triangle
// red, bottom-right corner empty.
func TestClipPathPolygon(t *testing.T) {
	canvas := paintClipBox(t, "polygon(0% 0%, 100% 0%, 0% 100%)")
	defer canvas.Release()
	// Triangle covering top-left half. Inside near (20,20); outside (80,80).
	if px := canvas.PixelAt(20, 20); px.R != 255 {
		t.Fatalf("triangle inside (20,20) = %+v, want red", px)
	}
	if px := canvas.PixelAt(80, 80); px.A != 0 {
		t.Fatalf("triangle outside (80,80) = %+v, want transparent", px)
	}
}
