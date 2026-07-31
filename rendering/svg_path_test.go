package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGPathCubic: a cubic-bezier path fills across the curve (points in
// the middle of the curve painted; corner outside stays transparent).
func TestSVGPathCubic(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	p := doc.CreateElement("path")
	// Filled semicircle-ish shape using cubic commands:
	// M 10 50 C 10 10, 90 10, 90 50 C 90 90, 10 90, 10 50 Z
	p.SetAttribute("d", "M 10 50 C 10 10 90 10 90 50 C 90 90 10 90 10 50 Z")
	p.SetAttribute("fill", "red")
	svgEl.AppendChild(p)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Center of the shape: red.
	if px := canvas.PixelAt(50, 50); px.R != 255 || px.G != 0 {
		t.Fatalf("center (50,50) = %+v, want red", px)
	}
	// Outside the curve (top-right corner area): transparent.
	if px := canvas.PixelAt(95, 5); px.A != 0 {
		t.Fatalf("outside (95,5) = %+v, want transparent", px)
	}
}

// TestSVGPathQuadratic: a quadratic path with stroke paints colored curve.
func TestSVGPathQuadratic(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	p := doc.CreateElement("path")
	// Open quadratic curve M 10 80 Q 50 10 90 80 (no fill → only stroke).
	p.SetAttribute("d", "M 10 80 Q 50 10 90 80")
	p.SetAttribute("fill", "none")
	p.SetAttribute("stroke", "blue")
	p.SetAttribute("stroke-width", "3")
	svgEl.AppendChild(p)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Some point along the curve (near the apex y≈45) is blue.
	found := false
	for x := 35; x <= 65 && !found; x++ {
		if px := canvas.PixelAt(x, 45); px.B > 200 && px.R < 60 {
			found = true
		}
	}
	if !found {
		t.Fatalf("quadratic stroke not found near apex")
	}
}
