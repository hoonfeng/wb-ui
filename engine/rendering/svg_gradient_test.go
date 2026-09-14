package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGGradientDiagonal: a linearGradient with x1/y1/x2/y2 diagonal axis
// paints a gradient that changes along that axis (top-left red → bottom-right
// blue). Percentages are the unambiguous spelling for objectBoundingBox
// coordinates ("100%" = 100% of the bbox; bare "1" also means 100%, bare
// "100" means 100 bbox-widths and renders all-red — verified against Edge).
func TestSVGGradientDiagonal(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	defs := doc.CreateElement("defs")
	lg := doc.CreateElement("linearGradient")
	lg.SetAttribute("id", "diag")
	lg.SetAttribute("x1", "0")
	lg.SetAttribute("y1", "0")
	lg.SetAttribute("x2", "100%")
	lg.SetAttribute("y2", "100%")
	s1 := doc.CreateElement("stop")
	s1.SetAttribute("offset", "0%")
	s1.SetAttribute("stop-color", "red")
	s2 := doc.CreateElement("stop")
	s2.SetAttribute("offset", "100%")
	s2.SetAttribute("stop-color", "blue")
	lg.AppendChild(s1)
	lg.AppendChild(s2)
	defs.AppendChild(lg)
	svgEl.AppendChild(defs)
	r := doc.CreateElement("rect")
	r.SetAttribute("width", "100")
	r.SetAttribute("height", "100")
	r.SetAttribute("fill", "url(#diag)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Top-left: red-dominant. Bottom-right: blue-dominant.
	tl := canvas.PixelAt(10, 10)
	br := canvas.PixelAt(90, 90)
	if tl.R < 200 || tl.B > 60 {
		t.Fatalf("top-left (10,10) = %+v, want red-dominant", tl)
	}
	if br.B < 200 || br.R > 60 {
		t.Fatalf("bottom-right (90,90) = %+v, want blue-dominant", br)
	}
}

// TestSVGRadialGradient: a radialGradient paints a radial fill (center bright,
// edge dark), confirming isRadial is honored (was a case-sensitivity bug
// where radial gradients fell into the linear path).
func TestSVGRadialGradient(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	defs := doc.CreateElement("defs")
	rg := doc.CreateElement("radialGradient")
	rg.SetAttribute("id", "rad")
	s1 := doc.CreateElement("stop")
	s1.SetAttribute("offset", "0%")
	s1.SetAttribute("stop-color", "white")
	s2 := doc.CreateElement("stop")
	s2.SetAttribute("offset", "100%")
	s2.SetAttribute("stop-color", "black")
	rg.AppendChild(s1)
	rg.AppendChild(s2)
	defs.AppendChild(rg)
	svgEl.AppendChild(defs)
	c := doc.CreateElement("circle")
	c.SetAttribute("cx", "50")
	c.SetAttribute("cy", "50")
	c.SetAttribute("r", "40")
	c.SetAttribute("fill", "url(#rad)")
	svgEl.AppendChild(c)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("nil doc")
	}
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Radial gradient: center brighter than edge.
	center := canvas.PixelAt(50, 50)
	edge := canvas.PixelAt(50, 89)
	if center.R < 200 {
		t.Fatalf("center (50,50) = %+v, want bright (radial)", center)
	}
	if edge.R > 120 {
		t.Fatalf("edge (50,89) = %+v, want dark (radial falloff)", edge)
	}
}
