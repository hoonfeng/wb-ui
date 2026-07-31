package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestBackdropFilterBlur: a backdrop-filter:blur(6px) box overlaps a sharp red
// square; the box region must show blurred (softened) red — color spread
// beyond the sharp square's edge — instead of an opaque box background.
func TestBackdropFilterBlur(t *testing.T) {
	canvas := graphics.NewCanvas(120, 120)
	defer canvas.Release()
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(120, 120)

	// Sharp red square at (10,10)-(50,50).
	redStyle := style.NewComputedStyle()
	redStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	red := NewRenderBox(doc.CreateElement("div"), redStyle)
	red.SetLocation(10, 10)
	red.SetSize(40, 40)
	rv.AddChild(red, nil)

	// backdrop-filter box at (30,30)-(90,90) — overlaps the red square.
	bdStyle := style.NewComputedStyle()
	bdStyle.SetProperty("backdrop-filter", "blur(6px)")
	bd := NewRenderBox(doc.CreateElement("div"), bdStyle)
	bd.SetLocation(30, 30)
	bd.SetSize(60, 60)
	rv.AddChild(bd, nil)

	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 120, Height: 120})

	// Point inside red but outside the backdrop box: sharp red.
	if px := canvas.PixelAt(20, 20); px.R < 200 || px.G > 60 {
		t.Fatalf("sharp red at (20,20) = %+v", px)
	}
	// Point inside the backdrop box and over the red square: red-ish (blurred
	// backdrop overlays the existing square — stays red).
	blurred := canvas.PixelAt(40, 40)
	t.Logf("(40,40)=%+v (20,20)=%+v", blurred, canvas.PixelAt(20, 20))
	if blurred.R < 120 {
		t.Fatalf("backdrop region (40,40) = %+v, want red-ish (blurred)", blurred)
	}
	// Blur spreads red beyond the square's edge (edge at 50,50): (52,52) is
	// inside the backdrop box but outside the sharp square — red bleed here
	// proves the backdrop was filtered (without blur it is empty).
	spread := canvas.PixelAt(52, 52)
	t.Logf("(52,52)=%+v", spread)
	if spread.R < 15 || spread.A < 15 {
		t.Fatalf("blur bleed at (52,52) = %+v, want visible red spread", spread)
	}
}
