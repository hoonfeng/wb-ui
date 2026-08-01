package graphics

import (
	"math"
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestSkiaEvenOddFill verifies that skia Path.SetFillType(evenodd) really
// excludes the self-intersection hole: (20,18) sits inside the bowtie's
// central crossing region which evenodd leaves unpainted. (A naive test
// point exactly at (20,20) lies ON the crossing boundary and anti-aliases —
// that earlier "failure" was a bad sample point, not a binding bug.)
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
	c.fillPaint.SetAntialias(false)
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
	// The crossing-region hole stays unpainted under evenodd.
	if r := at(20, 18); r != 0 {
		t.Errorf("evenodd hole at (20,18) painted (r=%d), want 0", r)
	}
}

// TestSkiaEvenOddStar reproduces the SVG star shape (outer r=14, inner r=6)
// through the same FillPath pipeline to check the center pentagon hole.
func TestSkiaEvenOddStar(t *testing.T) {
	c := NewCanvas(40, 40)
	var outer, inner [5][2]float64
	for i := 0; i < 5; i++ {
		a := -1.5707963 + 2*3.14159265*float64(i)/5
		outer[i] = [2]float64{20 + 14*math.Cos(a), 20 + 14*math.Sin(a)}
		a2 := -1.5707963 + 2*3.14159265*float64(i)/5 + 3.14159265/5
		inner[i] = [2]float64{20 + 6*math.Cos(a2), 20 + 6*math.Sin(a2)}
	}
	pts := make([]Point, 0, 11)
	add := func(p [2]float64) { pts = append(pts, Point{X: p[0], Y: p[1]}) }
	add(outer[0])
	for i := 0; i < 5; i++ {
		add(inner[(i+2)%5])
		add(outer[(i+1)%5])
	}
	// Last outer[0] duplicates the first point; drop it (Close connects back).
	pts = pts[:len(pts)-1]

	c.FillPath(pts, Color{R: 255, G: 0, B: 0, A: 255}, true)
	pix := c.Pixels()
	at := func(x, y int) uint8 {
		off := (y*40 + x) * 4
		return pix[off]
	}
	t.Logf("star center (20,23) r=%d (want 0=hole)", at(20, 23))
	t.Logf("star arm (20,9) r=%d (want 255)", at(20, 9))
	if at(20, 23) != 0 {
		t.Errorf("star hole (20,23) r=%d, want 0", at(20, 23))
	}
	if at(20, 9) < 200 {
		t.Errorf("star arm (20,9) r=%d, want filled", at(20, 9))
	}
}