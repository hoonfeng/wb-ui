package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestMixBlendMultiply: two overlapping layers — bottom gray (128) and top
// blue with mix-blend-mode: multiply — produce a darker blended pixel
// (multiply: 128 * 0 / 255 = 0 for blue on gray), unlike plain src-over
// which would show the opaque top color.
func TestMixBlendMultiply(t *testing.T) {
	canvas := graphics.NewCanvas(80, 80)
	defer canvas.Release()

	// Bottom layer: gray box.
	bottom := paintBlendBox(t, canvas, "gray", 0, 0, 60, 60, "")
	_ = bottom
	// Top layer: blue box at (20,20) with multiply — 20x20 overlap region.
	top := paintBlendBox(t, canvas, "blue", 20, 20, 60, 60, "multiply")
	_ = top

	// Overlap at (30,30): multiply(blue 0, gray 128) = 0 → blue channel 128
	// (blended), not the opaque 255 of src-over.
	ov := canvas.PixelAt(30, 30)
	if ov.B < 60 || ov.B > 210 {
		t.Fatalf("multiply overlap (30,30) = %+v, want blended blue (B≈128)", ov)
	}
	// Non-overlap top region (70,30): blue layer extends past gray's edge.
	tp := canvas.PixelAt(70, 30)
	if tp.B < 200 {
		t.Fatalf("top-only (70,30) = %+v, want blue", tp)
	}
}

func paintBlendBox(t *testing.T, canvas *graphics.Canvas, color string, x, y, w, h float64, blend string) *RenderBox {
	t.Helper()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 80})
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundColor = parseTestColor(color)
	if blend != "" {
		st.SetProperty("mix-blend-mode", blend)
	}
	box := NewRenderBox(el, st)
	box.SetLocation(x, y)
	box.SetSize(w, h)
	paintObjectBackground(box, info)
	return box
}

func parseTestColor(name string) style.Color {
	switch name {
	case "gray":
		return style.Color{R: 128, G: 128, B: 128, A: 255}
	case "blue":
		return style.Color{R: 0, G: 0, B: 255, A: 255}
	}
	return style.Color{A: 255}
}
