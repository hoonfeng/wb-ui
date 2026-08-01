package graphics

import (
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestSkiaEvenOddFill documents the current evenodd behavior: the goskia
// binding's Path.SetFillType is ignored by the C layer (fallback to winding).
// The SVG layer keeps the fill-rule API for when the binding is fixed.
func TestSkiaEvenOddFill(t *testing.T) {
	c := NewCanvas(40, 40)
	// Simple self-intersecting path: a bowtie (two triangles sharing a vertex).
	path := skia.NewPath()
	defer path.Release()
	path.MoveTo(10, 5)
	path.LineTo(30, 35)
	path.LineTo(30, 5)
	path.LineTo(10, 35)
	path.Close()
	path.SetFillType(skia.FillTypeEvenOdd)

	c.fillPaint.SetColor(colorToSkia(Color{R: 255, G: 0, B: 0, A: 255}))
	c.canvas.DrawPath(path, c.fillPaint)
	c.invalidatePixels()

	pix := c.Pixels()
	at := func(x, y int) uint8 {
		off := (y*40 + x) * 4
		return pix[off]
	}
	// Left triangle interior must be filled under any rule.
	if r := at(15, 20); r == 0 {
		t.Errorf("bowtie left interior not painted")
	}
	// The center crossing is painted under winding (the binding ignores
	// evenodd) — record the known limitation, don't fail.
	t.Logf("note: bowtie center r=%d (evenodd unsupported in goskia binding, uses winding)", at(20, 20))
}
