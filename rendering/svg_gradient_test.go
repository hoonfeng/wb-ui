package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGGradientFill: rect fill="url(#g)" paints a gradient (left red →
// right blue), not the flat fallback.
func TestSVGGradientFill(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "10")
	svgEl.SetAttribute("height", "10")
	defs := doc.CreateElement("defs")
	lg := doc.CreateElement("linearGradient")
	lg.SetAttribute("id", "g")
	s1 := doc.CreateElement("stop")
	s1.SetAttribute("offset", "0%")
	s1.SetAttribute("stop-color", "#ff0000")
	s2 := doc.CreateElement("stop")
	s2.SetAttribute("offset", "100%")
	s2.SetAttribute("stop-color", "#0000ff")
	lg.AppendChild(s1)
	lg.AppendChild(s2)
	defs.AppendChild(lg)
	rect := doc.CreateElement("rect")
	rect.SetAttribute("width", "10")
	rect.SetAttribute("height", "10")
	rect.SetAttribute("fill", "url(#g)")
	svgEl.AppendChild(defs)
	svgEl.AppendChild(rect)

	// Direct check: parse the gradient element standalone.
	g0 := parseGradientElement(lg)
	if g0 == nil {
		t.Fatalf("parseGradientElement returned nil")
	}
	t.Logf("direct gradient id=%q stops=%d", g0.id, len(g0.stops))

	sd := buildSVGDocument(svgEl)
	if sd == nil || len(sd.shapes) != 1 {
		t.Fatalf("shapes=%v, want 1", len(sd.shapes))
	}
	t.Logf("doc.gradients=%d clips=%d", len(sd.gradients), len(sd.clips))
	w, ok := sd.shapes[0].(*svgFilledShape)
	if !ok {
		t.Fatalf("shape type=%T, want *svgFilledShape", sd.shapes[0])
	}
	t.Logf("shape gradientID=%q fill=%+v", w.gradientID, w.fill)
	if len(sd.gradients) != 1 {
		t.Fatalf("gradients=%d, want 1", len(sd.gradients))
	}
	if w.gradientID != "g" {
		t.Fatalf("gradientID=%q, want g", w.gradientID)
	}
	g := sd.gradients["g"]
	if len(g.stops) != 2 {
		t.Fatalf("stops=%d, want 2", len(g.stops))
	}
	t.Logf("stop0=%+v stop1=%+v", g.stops[0].color, g.stops[1].color)

	canvas := graphics.NewCanvas(20, 20)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	// Left edge red, right edge blue.
	l := canvas.PixelAt(1, 5)
	r := canvas.PixelAt(9, 5)
	if l.R < 200 || l.B > 60 {
		t.Fatalf("left edge = %+v, want red-ish", l)
	}
	if r.B < 200 || r.R > 60 {
		t.Fatalf("right edge = %+v, want blue-ish", r)
	}
}

// TestSVGStroke: rect with stroke paints a colored outline plus fill.
func TestSVGStroke(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "10")
	svgEl.SetAttribute("height", "10")
	rect := doc.CreateElement("rect")
	rect.SetAttribute("x", "1")
	rect.SetAttribute("y", "1")
	rect.SetAttribute("width", "8")
	rect.SetAttribute("height", "8")
	rect.SetAttribute("fill", "#ffffff")
	rect.SetAttribute("stroke", "#00ff00")
	rect.SetAttribute("stroke-width", "2")
	svgEl.AppendChild(rect)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(20, 20)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	// Border pixel green (stroke), center white (fill).
	b := canvas.PixelAt(1, 5)
	c := canvas.PixelAt(5, 5)
	if b.G < 200 || b.R > 60 {
		t.Fatalf("border = %+v, want green stroke", b)
	}
	if c.R < 240 && c.G < 240 && c.B < 240 {
		t.Fatalf("center = %+v, want white fill", c)
	}
}
