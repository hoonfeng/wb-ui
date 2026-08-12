package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
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

// halfMaskPNG returns a data-URI PNG whose left half is fully transparent and
// right half fully opaque — a minimal mask for verifying mask-image semantics.
func halfMaskPNG(t *testing.T) string {
	t.Helper()
	const w, h = 8, 8
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(0)
			if x >= w/2 {
				a = 255
			}
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestMaskImageAlpha verifies mask-image actually masks the painted background:
// the image's alpha channel selects which region survives (opaque → kept,
// transparent → discarded), matching CSS mask-image semantics.
func TestMaskImageAlpha(t *testing.T) {
	canvas := graphics.NewCanvas(40, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 40})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+halfMaskPNG(t)+")")
	box := NewRenderBox(el, cs)
	box.SetLocation(0, 0)
	box.SetSize(40, 40)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(10, 20); px.A != 0 {
		t.Fatalf("left half (masked out) = %+v, want transparent", px)
	}
	if px := canvas.PixelAt(30, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("right half (kept) = %+v, want red", px)
	}
}
