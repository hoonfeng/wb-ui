package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGUseElement: <use href="#r"> paints the referenced rect at the
// use's x/y offset.
func TestSVGUseElement(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	defs := doc.CreateElement("defs")
	rect := doc.CreateElement("rect")
	rect.SetAttribute("id", "r")
	rect.SetAttribute("width", "20")
	rect.SetAttribute("height", "20")
	rect.SetAttribute("fill", "blue")
	defs.AppendChild(rect)
	use := doc.CreateElement("use")
	use.SetAttribute("href", "#r")
	use.SetAttribute("x", "30")
	use.SetAttribute("y", "40")
	svgEl.AppendChild(defs)
	svgEl.AppendChild(use)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatalf("buildSVGDocument returned nil")
	}
	t.Logf("shapes=%d elementByID=%d", len(sd.shapes), len(sd.elementByID))
	for k, v := range sd.elementByID {
		t.Logf("  id=%q tag=%q", k, v.LocalName())
	}
	t.Logf("href parse: %q", parseURLReference(use.GetAttribute("href")))

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	// The rect should be at (30,40)-(50,60) in blue.
	if px := canvas.PixelAt(40, 50); px.B < 200 || px.R > 60 {
		t.Fatalf("use rect pixel (40,50) = %+v, want blue", px)
	}
	// Outside the use target stays transparent.
	if px := canvas.PixelAt(5, 5); px.A != 0 {
		t.Fatalf("outside (5,5) = %+v, want transparent", px)
	}
}
