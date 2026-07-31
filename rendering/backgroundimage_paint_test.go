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

// TestPaintBackgroundURL: paintObjectBackground draws a data-URI background
// image honoring background-size/position.
func TestPaintBackgroundURL(t *testing.T) {
	// 8x8 red PNG data URI.
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
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	canvas := graphics.NewCanvas(160, 80)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 160, Height: 80})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	st.BackgroundSize = "80px 40px"
	st.BackgroundPosition = "center"
	st.BackgroundRepeat = "no-repeat" // isolate the single-tile geometry test
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(160, 80)

	paintObjectBackground(box, info)

	// Image 80x40 centered in 160x80 → covers (40,20)-(120,60).
	dx, dy, dw, dh := computeBackgroundDest(0, 0, 160, 80, "80px 40px", "center", 8, 8)
	t.Logf("dest = (%v,%v %vx%v)", dx, dy, dw, dh)
	if px := canvas.PixelAt(80, 40); px.R != 255 || px.G != 0 {
		t.Fatalf("center (80,40) = %+v, want red", px)
	}
	// Outside the image rect (top-left corner) stays transparent (no
	// background-color set, image only covers the centered 80x40 area).
	if px := canvas.PixelAt(4, 4); px.A != 0 {
		t.Fatalf("corner (4,4) = %+v, want transparent", px)
	}
	// Bottom-right corner (150,70) also outside the 120x60 image area.
	if px := canvas.PixelAt(150, 70); px.A != 0 {
		t.Fatalf("bottom-right (150,70) = %+v, want transparent", px)
	}
}
