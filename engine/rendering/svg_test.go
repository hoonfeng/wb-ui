package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGText tests SVG text element parsing and painting.
func TestSVGText(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "200")
	svgEl.SetAttribute("height", "100")
	textEl := doc.CreateElement("text")
	textEl.SetAttribute("x", "10")
	textEl.SetAttribute("y", "50")
	textEl.SetAttribute("font-family", "sans-serif")
	textEl.SetAttribute("font-size", "20")
	textEl.SetAttribute("fill", "red")
	textEl.SetTextContent("Hello SVG")
	svgEl.AppendChild(textEl)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	if len(sd.shapes) != 1 {
		t.Fatalf("expected 1 shape, got %d", len(sd.shapes))
	}
	shape := sd.shapes[0]
	// Elements with a resolved fill are wrapped in svgFilledShape; unwrap.
	if w, ok := shape.(*svgFilledShape); ok {
		shape = w.shape
	}
	if _, ok := shape.(*svgText); !ok {
		t.Fatalf("expected *svgText, got %T", sd.shapes[0])
	}
}

// TestSVGViewBox tests that viewBox parsing works correctly.
func TestSVGViewBox(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 100 100")
	svgEl.SetAttribute("width", "200")
	svgEl.SetAttribute("height", "200")

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	if !sd.hasVB {
		t.Fatal("expected viewBox to be detected")
	}
	if sd.viewBox[0] != 0 || sd.viewBox[1] != 0 || sd.viewBox[2] != 100 || sd.viewBox[3] != 100 {
		t.Fatalf("viewBox = %v, want [0 0 100 100]", sd.viewBox)
	}
}

// TestSVGGradientParsing tests that linearGradient definitions are parsed.
func TestSVGGradientParsing(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	defs := doc.CreateElement("defs")
	lg := doc.CreateElement("linearGradient")
	lg.SetAttribute("id", "myGrad")
	lg.SetAttribute("x1", "0")
	lg.SetAttribute("y1", "0")
	lg.SetAttribute("x2", "100")
	lg.SetAttribute("y2", "0")
	stop1 := doc.CreateElement("stop")
	stop1.SetAttribute("offset", "0%")
	stop1.SetAttribute("stop-color", "red")
	lg.AppendChild(stop1)
	stop2 := doc.CreateElement("stop")
	stop2.SetAttribute("offset", "100%")
	stop2.SetAttribute("stop-color", "blue")
	lg.AppendChild(stop2)
	defs.AppendChild(lg)
	svgEl.AppendChild(defs)

	// Also add a rect that references the gradient
	rect := doc.CreateElement("rect")
	rect.SetAttribute("width", "200")
	rect.SetAttribute("height", "200")
	rect.SetAttribute("fill", "url(#myGrad)")
	svgEl.AppendChild(rect)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	// There should be 1 shape (the rect, since defs shapes aren't added directly)
	// The gradient is stored in the paint context during walk
	if len(sd.shapes) == 0 {
		t.Fatal("expected at least rect shape")
	}
}

// TestSVGStyleParsing tests that inline style attributes are parsed.
func TestSVGStyleParsing(t *testing.T) {
	style := "fill:blue;stroke:black;stroke-width:2"
	m := parseStyleAttribute(style)
	if m["fill"] != "blue" {
		t.Fatalf("fill = %q, want blue", m["fill"])
	}
	if m["stroke"] != "black" {
		t.Fatalf("stroke = %q, want black", m["stroke"])
	}
	if m["stroke-width"] != "2" {
		t.Fatalf("stroke-width = %q, want 2", m["stroke-width"])
	}

	// Test with display:none
	m2 := parseStyleAttribute("display:none")
	if m2["display"] != "none" {
		t.Fatalf("display = %q, want none", m2["display"])
	}
}

// TestSVGTextPainting creates a canvas and paints SVG text to verify no crash.
func TestSVGTextPainting(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "200")
	svgEl.SetAttribute("height", "100")
	textEl := doc.CreateElement("text")
	textEl.SetAttribute("x", "10")
	textEl.SetAttribute("y", "50")
	textEl.SetAttribute("fill", "black")
	textEl.SetTextContent("Test")
	svgEl.AppendChild(textEl)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(200, 100)
	defer canvas.Release()

	// Should not panic
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
}

// TestSVGViewBoxPainting creates a canvas and paints shapes with viewBox.
func TestSVGViewBoxPainting(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 100 100")
	svgEl.SetAttribute("width", "200")
	svgEl.SetAttribute("height", "200")
	rect := doc.CreateElement("rect")
	rect.SetAttribute("width", "50")
	rect.SetAttribute("height", "50")
	rect.SetAttribute("fill", "red")
	svgEl.AppendChild(rect)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(200, 200)
	defer canvas.Release()

	// Should not panic
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
}

// TestSVGClipPathPaint: circle with clip-path="url(#myClip)" — a rect clip
// that only covers the left half — paints only inside the clip region.
func TestSVGClipPathPaint(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	defs := doc.CreateElement("defs")
	cp := doc.CreateElement("clipPath")
	cp.SetAttribute("id", "leftHalf")
	clipRect := doc.CreateElement("rect")
	clipRect.SetAttribute("width", "50")
	clipRect.SetAttribute("height", "100")
	cp.AppendChild(clipRect)
	defs.AppendChild(cp)

	rect := doc.CreateElement("rect")
	rect.SetAttribute("width", "100")
	rect.SetAttribute("height", "100")
	rect.SetAttribute("fill", "red")
	rect.SetAttribute("clip-path", "url(#leftHalf)")
	svgEl.AppendChild(defs)
	svgEl.AppendChild(rect)

	sd := buildSVGDocument(svgEl)
	if sd == nil || len(sd.shapes) != 1 {
		t.Fatalf("shapes=%v, want 1", len(sd.shapes))
	}
	w, ok := sd.shapes[0].(*svgFilledShape)
	if !ok || w.clipID != "leftHalf" {
		t.Fatalf("shape clipID=%q, want leftHalf (type=%T)", w.clipID, sd.shapes[0])
	}

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	// Left half: red (inside clip).
	if px := canvas.PixelAt(20, 50); px.R != 255 || px.G != 0 {
		t.Fatalf("left (20,50) = %+v, want red", px)
	}
	// Right half: clipped (transparent).
	if px := canvas.PixelAt(80, 50); px.A != 0 {
		t.Fatalf("right (80,50) = %+v, want clipped", px)
	}
}

// TestSVGClipPath tests clipPath parsing.
func TestSVGClipPath(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	defs := doc.CreateElement("defs")
	cp := doc.CreateElement("clipPath")
	cp.SetAttribute("id", "myClip")
	clipRect := doc.CreateElement("rect")
	clipRect.SetAttribute("width", "100")
	clipRect.SetAttribute("height", "100")
	cp.AppendChild(clipRect)
	defs.AppendChild(cp)

	circle := doc.CreateElement("circle")
	circle.SetAttribute("cx", "50")
	circle.SetAttribute("cy", "50")
	circle.SetAttribute("r", "50")
	circle.SetAttribute("clip-path", "url(#myClip)")
	svgEl.AppendChild(circle)
	svgEl.AppendChild(defs)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	if len(sd.shapes) != 1 {
		t.Fatalf("expected 1 shape (circle), got %d", len(sd.shapes))
	}
}
