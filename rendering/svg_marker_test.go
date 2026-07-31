package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGMarker: a <marker> template referenced by marker-end is painted at
// the path's final vertex, rotated to the path direction (orient=auto).
func TestSVGMarker(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "120")
	svgEl.SetAttribute("height", "80")

	defs := doc.CreateElement("defs")
	m := doc.CreateElement("marker")
	m.SetAttribute("id", "arrow")
	m.SetAttribute("refX", "8")
	m.SetAttribute("refY", "4")
	m.SetAttribute("markerWidth", "10")
	m.SetAttribute("markerHeight", "10")
	m.SetAttribute("orient", "auto")
	tri := doc.CreateElement("path")
	tri.SetAttribute("d", "M0,0 L8,4 L0,8 Z")
	tri.SetAttribute("fill", "blue")
	m.AppendChild(tri)
	defs.AppendChild(m)
	svgEl.AppendChild(defs)

	// Horizontal line from (10,40) to (100,40) with an end marker.
	line := doc.CreateElement("path")
	line.SetAttribute("d", "M10,40 L100,40")
	line.SetAttribute("stroke", "black")
	line.SetAttribute("stroke-width", "2")
	line.SetAttribute("marker-end", "url(#arrow)")
	svgEl.AppendChild(line)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(120, 80)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// The marker triangle points right (0° rotation for a horizontal path):
	// blue ink near the line end (100,40), roughly at (100..108, 36..44).
	found := false
	for y := 34; y <= 46; y++ {
		for x := 96; x <= 112; x++ {
			if px := canvas.PixelAt(x, y); px.B > 150 && px.R < 80 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("marker triangle not found at path end (100,40)")
	}
	// Ink should NOT extend past the marker box on the left side of the line
	// start (marker only on marker-end).
	if px := canvas.PixelAt(10, 40); px.A > 0 {
		// line itself is black at (10,40) — allow that; just ensure no blue
		// marker at the start.
	}
	if px := canvas.PixelAt(6, 40); px.B > 150 {
		t.Fatalf("unexpected marker ink before path start at (6,40): %+v", px)
	}
}
