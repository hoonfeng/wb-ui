package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// paintMaskLayerFixture builds a RenderView → layer tree → Paint pipeline for a
// single masked box (or a box with descendants) and returns the painted canvas.
func paintMaskLayerFixture(t *testing.T, w, h int, cs *style.ComputedStyle, children []*RenderBox) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(w, h)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(float64(w), float64(h))
	box := NewRenderBox(doc.CreateElement("div"), cs)
	box.SetLocation(0, 0)
	box.SetSize(float64(w), float64(h))
	rv.AddChild(box, nil)
	for _, c := range children {
		box.AddChild(c, nil)
	}
	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	if rootLayer == nil {
		t.Fatal("BuildLayerTree returned nil")
	}
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})
	return canvas
}

// TestMaskPropertyNoCrash: mask-image is stored as a property (usable via
// GetProperty) and painting with it set does not panic.
func TestMaskPropertyNoCrash(t *testing.T) {
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("div")
	imgEl.SetAttribute("style", "width:40px;height:40px;background:red;mask-image:url('mask.png')")
	_ = doc
	_ = imgEl

	cs := style.NewComputedStyle()
	cs.SetProperty("mask-image", "url('mask.png')")
	if got := cs.GetProperty("mask-image"); got == "" {
		t.Fatalf("mask-image not stored")
	}

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
		t.Fatalf("background (20,20) = %+v, want red", px)
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
	return pngDataURI(t, img)
}

// solidMaskPNG returns a data-URI PNG that is fully opaque white — a mask that
// keeps everything it covers.
func solidMaskPNG(t *testing.T) string {
	t.Helper()
	const w, h = 8, 8
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return pngDataURI(t, img)
}

// grayMaskPNG returns a data-URI PNG whose left half is black and right half
// white, both fully opaque. Under mask-mode: luminance the black half (luminance
// 0) masks out while the white half (luminance 255) is kept; under default alpha
// mode both halves are kept (alpha is 255 everywhere).
func grayMaskPNG(t *testing.T) string {
	t.Helper()
	const w, h = 8, 8
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= w/2 {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
	}
	return pngDataURI(t, img)
}

func pngDataURI(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestMaskImageAlpha verifies mask-image masks the painted background at the
// layer level: the image's alpha selects which region survives (opaque → kept,
// transparent → discarded), matching CSS mask-image semantics.
func TestMaskImageAlpha(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+halfMaskPNG(t)+")")
	canvas := paintMaskLayerFixture(t, 40, 40, cs, nil)
	defer canvas.Release()

	if px := canvas.PixelAt(10, 20); px.A != 0 {
		t.Fatalf("left half (masked out) = %+v, want transparent", px)
	}
	if px := canvas.PixelAt(30, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("right half (kept) = %+v, want red", px)
	}
}

// TestMaskImageMasksDescendants verifies the mask wraps the WHOLE subtree (P3.1):
// a descendant's background is masked too, not just the masked element's own
// background. The old per-box mask let descendants leak through.
func TestMaskImageMasksDescendants(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+halfMaskPNG(t)+")")

	childCS := style.NewComputedStyle()
	childCS.BackgroundColor = style.Color{G: 255, A: 255}
	child := NewRenderBox(dom.NewDocument().CreateElement("div"), childCS)
	child.SetLocation(0, 0)
	child.SetSize(40, 40)

	canvas := paintMaskLayerFixture(t, 40, 40, cs, []*RenderBox{child})
	defer canvas.Release()

	// Left half masked out → the descendant's green must also be gone.
	if px := canvas.PixelAt(10, 20); px.A != 0 {
		t.Fatalf("left half (masked-out descendant) = %+v, want transparent", px)
	}
	// Right half kept → the descendant green (painted over the parent red) shows.
	if px := canvas.PixelAt(30, 20); px.G != 255 || px.A != 255 {
		t.Fatalf("right half (kept descendant) = %+v, want green", px)
	}
}

// TestMaskSizeNoRepeat verifies mask-size + mask-repeat: no-repeat: the mask
// tile only covers the sized sub-rect, and the area OUTSIDE the tile is masked
// out (transparent) — unlike background-repeat where the untiled area stays
// empty. mask-size: 50% 50% shrinks the 8×8 solid mask to a 20×20 tile.
func TestMaskSizeNoRepeat(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+solidMaskPNG(t)+")")
	cs.SetProperty("mask-size", "50% 50%")
	cs.SetProperty("mask-repeat", "no-repeat")
	canvas := paintMaskLayerFixture(t, 40, 40, cs, nil)
	defer canvas.Release()

	if px := canvas.PixelAt(10, 10); px.R != 255 || px.A != 255 {
		t.Fatalf("tile area (kept) = %+v, want red", px)
	}
	if px := canvas.PixelAt(30, 30); px.A != 0 {
		t.Fatalf("outside tile (masked out) = %+v, want transparent", px)
	}
}

// TestMaskModeLuminance verifies mask-mode: luminance converts the mask image's
// RGB luminance into alpha before masking: black (luminance 0) masks out, white
// (luminance 255) is kept.
func TestMaskModeLuminance(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+grayMaskPNG(t)+")")
	cs.SetProperty("mask-mode", "luminance")
	canvas := paintMaskLayerFixture(t, 40, 40, cs, nil)
	defer canvas.Release()

	if px := canvas.PixelAt(10, 20); px.A != 0 {
		t.Fatalf("black half (luminance 0 → masked out) = %+v, want transparent", px)
	}
	if px := canvas.PixelAt(30, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("white half (luminance 255 → kept) = %+v, want red", px)
	}
}
