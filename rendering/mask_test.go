package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestMaskPropertyNoCrash: mask-image is stored as a property (usable via
// GetProperty) and painting with it set does not panic. Full mask
// rasterization (image-as-alpha) is not implemented yet — no skia API.
func TestMaskPropertyNoCrash(t *testing.T) {
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("div")
	imgEl.SetAttribute("style", "width:40px;height:40px;background:red;mask-image:url('mask.png')")
	_ = doc
	_ = imgEl

	// Resolver-level: the generic SetProperty path stores unknown props.
	cs := style.NewComputedStyle()
	cs.SetProperty("mask-image", "url('mask.png')")
	if got := cs.GetProperty("mask-image"); got == "" {
		t.Fatalf("mask-image not stored")
	}

	// Paint-level: a box with mask set renders normally.
	canvas := graphics.NewCanvas(40, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 40})
	cs2 := style.NewComputedStyle()
	cs2.BackgroundColor = style.Color{R: 255, A: 255}
	cs2.SetProperty("mask-image", "url('mask.png')")
	box := NewRenderBox(imgEl, cs2)
	box.SetLocation(0, 0)
	box.SetSize(40, 40)
	paintObjectBackground(box, info) // must not panic
	if px := canvas.PixelAt(20, 20); px.R != 255 {
		t.Fatalf("background (20,20) = %+v, want red (mask ignored for now)", px)
	}
}
