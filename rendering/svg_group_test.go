package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGGroup: <g fill="red"> group paints its children with the group's
// fill; <g stroke="blue"> applies stroke to children.
func TestSVGGroup(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	g := doc.CreateElement("g")
	g.SetAttribute("fill", "red")
	r1 := doc.CreateElement("rect")
	r1.SetAttribute("x", "0")
	r1.SetAttribute("y", "0")
	r1.SetAttribute("width", "40")
	r1.SetAttribute("height", "40")
	g.AppendChild(r1)
	svgEl.AppendChild(g)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatalf("buildSVGDocument returned nil")
	}
	t.Logf("shapes=%d", len(sd.shapes))

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})
	if px := canvas.PixelAt(20, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("group rect (20,20) = %+v, want red", px)
	}
}

// TestSVGImage: an <image> element with a data-URI PNG paints the bitmap
// scaled to its x/y/width/height box.
func TestSVGImage(t *testing.T) {
	imgData := fitPNG(t) // 10x20 red PNG data URI

	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	img := doc.CreateElement("image")
	img.SetAttribute("x", "20")
	img.SetAttribute("y", "30")
	img.SetAttribute("width", "40")
	img.SetAttribute("height", "40")
	img.SetAttribute("href", imgData)
	svgEl.AppendChild(img)

	sd := buildSVGDocument(svgEl)
	if sd == nil || len(sd.shapes) != 1 {
		t.Fatalf("shapes=%v, want 1", len(sd.shapes))
	}

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Center of the image box: red.
	if px := canvas.PixelAt(40, 50); px.R != 255 || px.G != 0 {
		t.Fatalf("image center (40,50) = %+v, want red", px)
	}
	// Outside the image box: transparent.
	if px := canvas.PixelAt(5, 5); px.A != 0 {
		t.Fatalf("outside (5,5) = %+v, want transparent", px)
	}
}

// TestNestedSVGOffset: an inner <svg x="30" y="20"> shifts its children.
func TestNestedSVGOffset(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")
	inner := doc.CreateElement("svg")
	inner.SetAttribute("x", "30")
	inner.SetAttribute("y", "20")
	r1 := doc.CreateElement("rect")
	r1.SetAttribute("x", "0")
	r1.SetAttribute("y", "0")
	r1.SetAttribute("width", "20")
	r1.SetAttribute("height", "20")
	r1.SetAttribute("fill", "green")
	inner.AppendChild(r1)
	svgEl.AppendChild(inner)

	sd := buildSVGDocument(svgEl)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// (40,30) = inner origin (30,20) + rect (0,0) + 10/10 → green.
	if px := canvas.PixelAt(40, 30); px.G < 200 || px.R > 60 {
		t.Fatalf("nested svg rect (40,30) = %+v, want green", px)
	}
	// Without offset the rect would be at (0,0): (10,10) must stay empty.
	if px := canvas.PixelAt(10, 10); px.A != 0 {
		t.Fatalf("expected offset: (10,10) = %+v, want transparent", px)
	}
}
