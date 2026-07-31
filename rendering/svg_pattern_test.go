package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGPattern: a <pattern> with a small circle is tiled across a rect's
// fill box at the pattern's width/height intervals.
func TestSVGPattern(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	defs := doc.CreateElement("defs")
	pat := doc.CreateElement("pattern")
	pat.SetAttribute("id", "dots")
	pat.SetAttribute("width", "20")
	pat.SetAttribute("height", "20")
	dot := doc.CreateElement("circle")
	dot.SetAttribute("cx", "10")
	dot.SetAttribute("cy", "10")
	dot.SetAttribute("r", "5")
	dot.SetAttribute("fill", "red")
	pat.AppendChild(dot)
	defs.AppendChild(pat)
	svgEl.AppendChild(defs)

	r := doc.CreateElement("rect")
	r.SetAttribute("width", "100")
	r.SetAttribute("height", "100")
	r.SetAttribute("fill", "url(#dots)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Tile centers at (10,10), (30,10), (10,30), (30,30)…: red dots.
	on := []struct{ x, y int }{{10, 10}, {30, 10}, {30, 30}, {50, 50}}
	for _, p := range on {
		if px := canvas.PixelAt(p.x, p.y); px.R != 255 || px.G != 0 {
			t.Fatalf("pattern dot (%d,%d) = %+v, want red", p.x, p.y, px)
		}
	}
	// Gap between tiles: transparent (no fill color).
	if px := canvas.PixelAt(20, 20); px.A != 0 {
		t.Fatalf("pattern gap (20,20) = %+v, want transparent", px)
	}
}
