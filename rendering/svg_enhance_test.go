package rendering

import (
	"fmt"
	"math"
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// renderSVGToCanvas renders an SVG element to a small offscreen canvas and
// returns the canvas so tests can inspect painted pixels.
func renderSVGToCanvas(t *testing.T, svgEl *dom.Element, w, h int) *graphics.Canvas {
	t.Helper()
	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	canvas := graphics.NewCanvas(w, h)
	paintSVG(canvas, sd, 0, 0, graphics.Color{R: 0, G: 0, B: 0, A: 0xFF})
	return canvas
}

// TestSVGLineCapRound: a line with stroke-linecap="round" must have round
// ends — i.e. painted pixels beyond the raw endpoints (a butt cap line
// would stop exactly at the endpoints).
func TestSVGLineCapRound(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "40")
	svgEl.SetAttribute("height", "20")
	lineEl := doc.CreateElement("line")
	lineEl.SetAttribute("x1", "10")
	lineEl.SetAttribute("y1", "10")
	lineEl.SetAttribute("x2", "30")
	lineEl.SetAttribute("y2", "10")
	lineEl.SetAttribute("stroke", "red")
	lineEl.SetAttribute("stroke-width", "6")
	lineEl.SetAttribute("stroke-linecap", "round")
	svgEl.AppendChild(lineEl)

	c := renderSVGToCanvas(t, svgEl, 40, 20)
	// Butt cap would paint x=10..30; round cap extends half the width
	// (3px) past each end → x≈7..33. Check a pixel just left of x=10.
	pix := c.Pixels()
	// Pixels are RGBA per row-major.
	at := func(x, y int) (uint8, uint8, uint8) {
		off := (y*40 + x) * 4
		return pix[off], pix[off+1], pix[off+2]
	}
	r, _, _ := at(8, 10)
	if r == 0 {
		t.Errorf("round cap: pixel at (8,10) not painted (r=%d), expected red", r)
	}
	// Middle must be painted.
	r, _, _ = at(20, 10)
	if r == 0 {
		t.Errorf("line middle (20,10) not painted")
	}
}

// TestSVGLineCapButt: default (butt) cap stops exactly at endpoints — the
// pixel just outside must be unpainted.
func TestSVGLineCapButt(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "40")
	svgEl.SetAttribute("height", "20")
	lineEl := doc.CreateElement("line")
	lineEl.SetAttribute("x1", "10")
	lineEl.SetAttribute("y1", "10")
	lineEl.SetAttribute("x2", "30")
	lineEl.SetAttribute("y2", "10")
	lineEl.SetAttribute("stroke", "red")
	lineEl.SetAttribute("stroke-width", "6")
	svgEl.AppendChild(lineEl)

	c := renderSVGToCanvas(t, svgEl, 40, 20)
	pix := c.Pixels()
	at := func(x, y int) (uint8, uint8, uint8) {
		off := (y*40 + x) * 4
		return pix[off], pix[off+1], pix[off+2]
	}
	r, _, _ := at(8, 10)
	if r != 0 {
		t.Errorf("butt cap: pixel at (8,10) painted (r=%d), want unpainted", r)
	}
	r, _, _ = at(20, 10)
	if r == 0 {
		t.Errorf("line middle (20,10) not painted")
	}
}

// TestSVGFillRuleEvenOdd: fill-rule is parsed and forwarded to the painter;
// the goskia binding currently ignores evenodd (falls back to winding), so we
// verify the API surface + nonzero default rather than the hole geometry.
func TestSVGFillRuleEvenOdd(t *testing.T) {
	// Star points (outer radius 14 centered at 20,20).
	outer := [5][2]float64{}
	inner := [5][2]float64{}
	for i := 0; i < 5; i++ {
		a := -math.Pi/2 + 2*math.Pi*float64(i)/5
		outer[i] = [2]float64{20 + 14*math.Cos(a), 20 + 14*math.Sin(a)}
		a2 := -math.Pi/2 + 2*math.Pi*float64(i)/5 + math.Pi/5
		inner[i] = [2]float64{20 + 6*math.Cos(a2), 20 + 6*math.Sin(a2)}
	}
	pts := "M" + f2(outer[0]) + " L" + f2(inner[2]) + " L" + f2(outer[1]) +
		" L" + f2(inner[3]) + " L" + f2(outer[2]) + " L" + f2(inner[4]) +
		" L" + f2(outer[3]) + " L" + f2(inner[0]) + " L" + f2(outer[4]) +
		" L" + f2(inner[1]) + " Z"

	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "40")
	svgEl.SetAttribute("height", "40")
	pathEl := doc.CreateElement("path")
	pathEl.SetAttribute("d", pts)
	pathEl.SetAttribute("fill", "red")
	pathEl.SetAttribute("fill-rule", "evenodd")
	svgEl.AppendChild(pathEl)

	// The wrapper must carry fillRule="evenodd" through to paint time.
	sd := buildSVGDocument(svgEl)
	if len(sd.shapes) == 0 {
		t.Fatal("no shapes built")
	}
	w, ok := sd.shapes[0].(*svgFilledShape)
	if !ok {
		t.Fatalf("shape %T, want *svgFilledShape", sd.shapes[0])
	}
	if w.fillRule != "evenodd" {
		t.Errorf("fillRule = %q, want evenodd", w.fillRule)
	}

	c := renderSVGToCanvas(t, svgEl, 40, 40)
	pix := c.Pixels()
	at := func(x, y int) (uint8, uint8, uint8) {
		off := (y*40 + x) * 4
		return pix[off], pix[off+1], pix[off+2]
	}
	// An outer arm point must be painted under any fill rule.
	r, _, _ := at(20, 9)
	if r == 0 {
		t.Errorf("star arm (20,9) not painted")
	}
	// The center pentagon is an evenodd hole. (20,20) sits ON the vertical
	// edge outer[0]→inner[2], so use (20,23) which is inside the hole.
	r, _, _ = at(20, 23)
	if r > 20 {
		t.Errorf("evenodd star hole (20,23) painted (r=%d), want empty", r)
	}
}

// TestSVGOpacity: fill-opacity="0.4" on red fill should give alpha ≈ 0.4*255.
func TestSVGOpacity(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "20")
	svgEl.SetAttribute("height", "20")
	rectEl := doc.CreateElement("rect")
	rectEl.SetAttribute("x", "2")
	rectEl.SetAttribute("y", "2")
	rectEl.SetAttribute("width", "16")
	rectEl.SetAttribute("height", "16")
	rectEl.SetAttribute("fill", "red")
	rectEl.SetAttribute("fill-opacity", "0.4")
	svgEl.AppendChild(rectEl)

	c := renderSVGToCanvas(t, svgEl, 20, 20)
	pix := c.Pixels()
	off := (10*20 + 10) * 4
	a := pix[off+3]
	if a < 80 || a > 130 {
		t.Errorf("fill-opacity 0.4: alpha = %d, want ≈102 (0.4*255)", a)
	}
}

// TestSVGViewBoxMeet: a square viewBox rendered into a non-square viewport
// must keep the aspect ratio (uniform scale) and center — not stretch.
func TestSVGViewBoxMeet(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 100 100")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "50")
	rectEl := doc.CreateElement("rect")
	rectEl.SetAttribute("x", "0")
	rectEl.SetAttribute("y", "0")
	rectEl.SetAttribute("width", "100")
	rectEl.SetAttribute("height", "100")
	rectEl.SetAttribute("fill", "red")
	svgEl.AppendChild(rectEl)

	c := renderSVGToCanvas(t, svgEl, 100, 50)
	pix := c.Pixels()
	at := func(x, y int) (uint8, uint8, uint8) {
		off := (y*100 + x) * 4
		return pix[off], pix[off+1], pix[off+2]
	}
	// Uniform scale = 0.5: the square occupies x=25..75, y=0..50.
	r, _, _ := at(50, 25)
	if r == 0 {
		t.Errorf("meet: center (50,25) not painted")
	}
	// Beyond the scaled square (x=10) must be empty.
	r, _, _ = at(10, 25)
	if r != 0 {
		t.Errorf("meet: (10,25) painted (r=%d), want empty (stretched)", r)
	}
}

// helpers

func f2(p [2]float64) string {
	return fmt.Sprint(p[0]) + " " + fmt.Sprint(p[1])
}
