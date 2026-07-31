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

// smallRedPNGURI returns a base64 data URI for an 8x8 solid red PNG.
func smallRedPNGURI(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 255, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestPaintBackgroundRepeat: default repeat tiles the 8x8 image across a
// 40x16 box (5 x 2 tiles) — all pixels red.
func TestPaintBackgroundRepeat(t *testing.T) {
	uri := smallRedPNGURI(t)
	canvas := graphics.NewCanvas(40, 16)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 16})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	st.BackgroundSize = "8px 8px"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(40, 16)

	paintObjectBackground(box, info)

	for _, p := range []struct{ x, y int }{{2, 2}, {14, 2}, {30, 2}, {2, 12}, {30, 12}} {
		if px := canvas.PixelAt(p.x, p.y); px.R != 255 || px.G != 0 {
			t.Fatalf("tile pixel (%d,%d) = %+v, want red", p.x, p.y, px)
		}
	}
}

// TestPaintBackgroundNoRepeat: no-repeat paints a single tile at top-left,
// leaving the rest transparent.
func TestPaintBackgroundNoRepeat(t *testing.T) {
	uri := smallRedPNGURI(t)
	canvas := graphics.NewCanvas(40, 16)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 16})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	st.BackgroundSize = "8px 8px"
	st.BackgroundRepeat = "no-repeat"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(40, 16)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(4, 4); px.R != 255 {
		t.Fatalf("first tile (4,4) = %+v, want red", px)
	}
	// Second tile position must be transparent.
	if px := canvas.PixelAt(12, 4); px.A != 0 {
		t.Fatalf("second tile (12,4) = %+v, want transparent", px)
	}
}

// TestPaintBackgroundRepeatX: repeat-x tiles horizontally only; a row below
// the first tile height stays transparent.
func TestPaintBackgroundRepeatX(t *testing.T) {
	uri := smallRedPNGURI(t)
	canvas := graphics.NewCanvas(40, 16)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 16})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	st.BackgroundSize = "8px 8px"
	st.BackgroundRepeat = "repeat-x"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(40, 16)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(30, 4); px.R != 255 {
		t.Fatalf("repeat-x tile (30,4) = %+v, want red", px)
	}
	// Below the first 8px row: transparent (no vertical tiling).
	if px := canvas.PixelAt(4, 12); px.A != 0 {
		t.Fatalf("below row (4,12) = %+v, want transparent", px)
	}
}
