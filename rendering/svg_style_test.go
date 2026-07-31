package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGTransformTranslate: transform="translate(30,20)" shifts the rect.
func TestSVGTransformTranslate(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	r := doc.CreateElement("rect")
	r.SetAttribute("width", "30")
	r.SetAttribute("height", "30")
	r.SetAttribute("fill", "red")
	r.SetAttribute("transform", "translate(30, 20)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Original position (15,15) is now empty; shifted (45,35) is red.
	if px := canvas.PixelAt(45, 35); px.R != 255 || px.G != 0 {
		t.Fatalf("translated (45,35) = %+v, want red", px)
	}
	if px := canvas.PixelAt(15, 15); px.A != 0 {
		t.Fatalf("original (15,15) = %+v, want transparent", px)
	}
}

// TestSVGTransformScale: transform="scale(2)" doubles the rect size.
func TestSVGTransformScale(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	r := doc.CreateElement("rect")
	r.SetAttribute("width", "20")
	r.SetAttribute("height", "20")
	r.SetAttribute("fill", "blue")
	r.SetAttribute("transform", "scale(2)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// 20x20 at scale 2 covers 40x40: (30,30) blue, (45,45) outside.
	if px := canvas.PixelAt(30, 30); px.B != 255 || px.R != 0 {
		t.Fatalf("scaled (30,30) = %+v, want blue", px)
	}
	if px := canvas.PixelAt(45, 45); px.A != 0 {
		t.Fatalf("outside scaled (45,45) = %+v, want transparent", px)
	}
}

// TestSVGPathArc: an arc command paints a curved stroke. Two half-circle
// arcs form an ellipse; the top center and bottom center are stroked.
func TestSVGPathArc(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	p := doc.CreateElement("path")
	// Two half-ellipse arcs from (10,50) to (90,50) and back.
	p.SetAttribute("d", "M 10 50 A 40 30 0 0 1 90 50 A 40 30 0 0 1 10 50")
	p.SetAttribute("fill", "none")
	p.SetAttribute("stroke", "green")
	p.SetAttribute("stroke-width", "3")
	svgEl.AppendChild(p)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Top of the upper arc ≈ (50,20) and bottom of the lower arc ≈ (50,80).
	if px := canvas.PixelAt(50, 20); px.G < 200 || px.R > 60 {
		t.Fatalf("upper arc (50,20) = %+v, want green", px)
	}
	if px := canvas.PixelAt(50, 80); px.G < 200 || px.R > 60 {
		t.Fatalf("lower arc (50,80) = %+v, want green", px)
	}
	// Inside the ellipse (50,50) is empty (fill none).
	if px := canvas.PixelAt(50, 50); px.A != 0 {
		t.Fatalf("fill none center (50,50) = %+v, want transparent", px)
	}
}

// TestSVGTextMultiline: <text> with \n paints each line at its own
// baseline; tspan content is included via TextContent.
func TestSVGTextMultiline(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "200")
	svgEl.SetAttribute("height", "100")
	text := doc.CreateElement("text")
	text.SetAttribute("x", "10")
	text.SetAttribute("y", "30")
	text.SetAttribute("font-size", "20")
	text.SetAttribute("fill", "black")
	text.SetTextContent("Line One\nLine Two")
	svgEl.AppendChild(text)

	sd := buildSVGDocument(svgEl)
	if sd == nil || len(sd.shapes) != 1 {
		t.Fatalf("shapes=%v, want 1", len(sd.shapes))
	}
	canvas := graphics.NewCanvas(200, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// First line baseline y=30 → pixels around y=25; second line y≈54.
	line1 := pixelsOnRow(canvas, 25)
	line2 := pixelsOnRow(canvas, 25+24)
	if line1 == 0 || line2 == 0 {
		t.Fatalf("multiline text rows: line1=%d line2=%d, want >0", line1, line2)
	}
}

// pixelsOnRow counts non-transparent pixels on the given row.
func pixelsOnRow(c *graphics.Canvas, y int) int {
	n := 0
	for x := 0; x < c.Width(); x++ {
		if px := c.PixelAt(x, y); px.A != 0 {
			n++
		}
	}
	return n
}

// TestSVGUserSpaceGradient: gradientUnits="userSpaceOnUse" treats x1/y1/x2/y2
// as absolute coordinates — a horizontal gradient at y=0..100 fills the whole
// shape regardless of its bbox position.
func TestSVGUserSpaceGradient(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "120")
	svgEl.SetAttribute("height", "60")
	defs := doc.CreateElement("defs")
	lg := doc.CreateElement("linearGradient")
	lg.SetAttribute("id", "us")
	lg.SetAttribute("gradientUnits", "userSpaceOnUse")
	lg.SetAttribute("x1", "0")
	lg.SetAttribute("y1", "0")
	lg.SetAttribute("x2", "100")
	lg.SetAttribute("y2", "0")
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

	// Two rects side by side; both sample the absolute 0..100 axis, so the
	// left one is red-ish and the right one is blue-ish.
	r1 := doc.CreateElement("rect")
	r1.SetAttribute("x", "0")
	r1.SetAttribute("y", "10")
	r1.SetAttribute("width", "50")
	r1.SetAttribute("height", "40")
	r1.SetAttribute("fill", "url(#us)")
	svgEl.AppendChild(r1)
	r2 := doc.CreateElement("rect")
	r2.SetAttribute("x", "60")
	r2.SetAttribute("y", "10")
	r2.SetAttribute("width", "50")
	r2.SetAttribute("height", "40")
	r2.SetAttribute("fill", "url(#us)")
	svgEl.AppendChild(r2)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(120, 60)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Left rect at x≈20 → t≈0.2 (red-ish); right rect at x≈80 → t≈0.8 (blue-ish).
	left := canvas.PixelAt(20, 30)
	right := canvas.PixelAt(80, 30)
	if left.R < 200 || left.B > 80 {
		t.Fatalf("userSpace left (20,30) = %+v, want red-ish", left)
	}
	if right.B < 200 || right.R > 80 {
		t.Fatalf("userSpace right (80,30) = %+v, want blue-ish", right)
	}
}

// TestSVGTextRotate: rotate="90" turns horizontal text vertical — ink is
// present along a vertical column and absent on the original baseline.
func TestSVGTextRotate(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	text := doc.CreateElement("text")
	text.SetAttribute("x", "30")
	text.SetAttribute("y", "50")
	text.SetAttribute("font-size", "20")
	text.SetAttribute("fill", "black")
	text.SetAttribute("rotate", "90")
	text.SetTextContent("AB")
	svgEl.AppendChild(text)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// After 90° rotation around (30,50), glyphs run downward from (30,50):
	// ink near (30,60)…(30,80); original horizontal baseline near (35,50)
	// should be mostly empty.
	vertical := pixelsOnColumn(canvas, 30)
	horizontal := pixelsOnRow(canvas, 49)
	if vertical == 0 {
		t.Fatalf("rotated text: no vertical ink")
	}
	if horizontal > vertical/2 {
		t.Fatalf("rotated text: horizontal ink %d should be < half of vertical %d", horizontal, vertical)
	}
}

// pixelsOnColumn counts non-transparent pixels on the given column.
func pixelsOnColumn(c *graphics.Canvas, x int) int {
	n := 0
	for y := 0; y < c.Height(); y++ {
		if px := c.PixelAt(x, y); px.A != 0 {
			n++
		}
	}
	return n
}

// TestSVGStyleSheet: <style> rules matching .cls apply fill to elements.
func TestSVGStyleSheet(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	styleEl := doc.CreateElement("style")
	styleEl.SetTextContent(".redBox { fill: #ff0000; } .blueBox { fill: #0000ff; }")
	svgEl.AppendChild(styleEl)

	r1 := doc.CreateElement("rect")
	r1.SetAttribute("class", "redBox")
	r1.SetAttribute("x", "0")
	r1.SetAttribute("y", "0")
	r1.SetAttribute("width", "40")
	r1.SetAttribute("height", "40")
	svgEl.AppendChild(r1)

	r2 := doc.CreateElement("rect")
	r2.SetAttribute("class", "blueBox")
	r2.SetAttribute("x", "50")
	r2.SetAttribute("y", "0")
	r2.SetAttribute("width", "40")
	r2.SetAttribute("height", "40")
	svgEl.AppendChild(r2)

	sd := buildSVGDocument(svgEl)
	if sd == nil || len(sd.shapes) != 2 {
		t.Fatalf("shapes=%v, want 2", len(sd.shapes))
	}

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	if px := canvas.PixelAt(20, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("redBox (20,20) = %+v, want red", px)
	}
	if px := canvas.PixelAt(70, 20); px.B != 255 || px.R != 0 {
		t.Fatalf("blueBox (70,20) = %+v, want blue", px)
	}
}

// TestSVGStyleInlineWins: inline fill attribute beats stylesheet rule.
func TestSVGStyleInlineWins(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "60")
	svgEl.SetAttribute("height", "60")
	styleEl := doc.CreateElement("style")
	styleEl.SetTextContent(".x { fill: red; }")
	svgEl.AppendChild(styleEl)

	r := doc.CreateElement("rect")
	r.SetAttribute("class", "x")
	r.SetAttribute("width", "50")
	r.SetAttribute("height", "50")
	r.SetAttribute("fill", "#00ff00")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(60, 60)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	if px := canvas.PixelAt(25, 25); px.G != 255 || px.R != 0 {
		t.Fatalf("inline fill (25,25) = %+v, want green", px)
	}
}
