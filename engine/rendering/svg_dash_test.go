package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGDashArray: a <line> with stroke-dasharray paints a dashed stroke —
// on-segments colored, off-segments transparent along the line.
func TestSVGDashArray(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "20")
	line := doc.CreateElement("line")
	line.SetAttribute("x1", "0")
	line.SetAttribute("y1", "10")
	line.SetAttribute("x2", "100")
	line.SetAttribute("y2", "10")
	line.SetAttribute("stroke", "red")
	line.SetAttribute("stroke-width", "4")
	line.SetAttribute("stroke-dasharray", "10 6")
	svgEl.AppendChild(line)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 20)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Pattern: 10 on, 6 off, repeating from x=0.
	// on segments: 0-10, 16-26, 32-42…; off segments: 10-16, 26-32…
	onAt := []int{2, 18}   // inside on (0-10) and on (16-26)
	offAt := []int{12, 30} // inside off (10-16) and off (26-32)
	for _, x := range onAt {
		if px := canvas.PixelAt(x, 10); px.R != 255 {
			t.Fatalf("dash on at x=%d = %+v, want red", x, px)
		}
	}
	for _, x := range offAt {
		if px := canvas.PixelAt(x, 10); px.A != 0 {
			t.Fatalf("dash off at x=%d = %+v, want transparent", x, px)
		}
	}
}
