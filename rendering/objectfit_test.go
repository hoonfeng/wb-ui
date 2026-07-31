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

// fitPNG returns a data URI for a 10x20 red PNG (tall portrait image).
func fitPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 20))
	red := color.RGBA{R: 255, A: 255}
	for y := 0; y < 20; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// paintFitBox renders an img box (60x30 content) with the given object-fit.
func paintFitBox(t *testing.T, fit string) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(60, 30)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 30})
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("src", fitPNG(t))
	st := style.NewComputedStyle()
	st.SetProperty("object-fit", fit)
	box := NewRenderBox(imgEl, st)
	box.SetLocation(0, 0)
	box.SetSize(60, 30)
	if !PaintImage(box, info) {
		t.Fatalf("PaintImage returned false")
	}
	return canvas
}

// TestObjectFitContain: 10x20 image in 60x30 box → contain scales to
// 15x30 centered (left/right edges transparent, image band red).
func TestObjectFitContain(t *testing.T) {
	canvas := paintFitBox(t, "contain")
	defer canvas.Release()
	// Center column red.
	if px := canvas.PixelAt(30, 15); px.R != 255 || px.G != 0 {
		t.Fatalf("center (30,15) = %+v, want red", px)
	}
	// Far left edge transparent (letterboxed).
	if px := canvas.PixelAt(2, 15); px.A != 0 {
		t.Fatalf("left letterbox (2,15) = %+v, want transparent", px)
	}
}

// TestObjectFitCover: 10x20 image in 60x30 box → cover scales to 60x120,
// cropping vertically (top and bottom bands cropped; full width red).
func TestObjectFitCover(t *testing.T) {
	canvas := paintFitBox(t, "cover")
	defer canvas.Release()
	// Any row in the box is red (image covers the full box).
	for _, y := range []int{5, 15, 25} {
		if px := canvas.PixelAt(30, y); px.R != 255 || px.G != 0 {
			t.Fatalf("cover row y=%d (30,%d) = %+v, want red", y, y, px)
		}
	}
}

// TestObjectFitNone: 10x20 image drawn at natural size, centered by the
// default object-position (50% 50%) → (25,5)-(35,25); corners transparent.
func TestObjectFitNone(t *testing.T) {
	canvas := paintFitBox(t, "none")
	defer canvas.Release()
	if px := canvas.PixelAt(30, 15); px.R != 255 || px.G != 0 {
		t.Fatalf("natural center (30,15) = %+v, want red", px)
	}
	if px := canvas.PixelAt(5, 10); px.A != 0 {
		t.Fatalf("outside natural size (5,10) = %+v, want transparent", px)
	}
}

// TestObjectFitPosition: object-fit:none + object-position:left top moves
// the natural-size image to the top-left corner.
func TestObjectFitPosition(t *testing.T) {
	canvas := graphics.NewCanvas(60, 30)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 30})
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("src", fitPNG(t))
	st := style.NewComputedStyle()
	st.SetProperty("object-fit", "none")
	st.SetProperty("object-position", "left top")
	box := NewRenderBox(imgEl, st)
	box.SetLocation(0, 0)
	box.SetSize(60, 30)
	if !PaintImage(box, info) {
		t.Fatalf("PaintImage returned false")
	}
	if px := canvas.PixelAt(5, 10); px.R != 255 || px.G != 0 {
		t.Fatalf("top-left natural (5,10) = %+v, want red", px)
	}
	if px := canvas.PixelAt(30, 15); px.A != 0 {
		t.Fatalf("away from corner (30,15) = %+v, want transparent", px)
	}
}

// TestImageAltFallback: an <img> with a broken/unavailable src renders the
// alt text instead of nothing.
func TestImageAltFallback(t *testing.T) {
	canvas := graphics.NewCanvas(120, 30)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 30})
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("src", "missing-file-xyz.png")
	imgEl.SetAttribute("alt", "broken image")
	st := style.NewComputedStyle()
	box := NewRenderBox(imgEl, st)
	box.SetLocation(0, 0)
	box.SetSize(120, 30)

	if PaintImage(box, info) {
		t.Fatalf("PaintImage should return false for unloadable image")
	}
	// The alt text is painted: at least one non-background pixel in the row.
	found := false
	for x := 0; x < 120; x++ {
		if px := canvas.PixelAt(x, 15); px.A != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("alt text not painted for broken image")
	}
}
