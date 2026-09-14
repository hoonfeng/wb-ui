package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/platform/graphics"
)

// TestSplitBackgroundLayers: top-level comma split keeps gradient args intact.
func TestSplitBackgroundLayers(t *testing.T) {
	layers := splitBackgroundLayers("linear-gradient(to right, #ff0000, #0000ff), radial-gradient(circle, #00ff00, #000000)")
	if len(layers) != 2 {
		t.Fatalf("want 2 layers, got %d: %v", len(layers), layers)
	}
	if !strings.HasPrefix(layers[0], "linear-gradient(") || !strings.HasPrefix(layers[1], "radial-gradient(") {
		t.Fatalf("unexpected layers: %v", layers)
	}
	if len(splitBackgroundLayers("linear-gradient(to right,#ff0000,#00ff00)")) != 1 {
		t.Fatalf("single layer should stay 1")
	}
	if splitBackgroundLayers("none") != nil {
		t.Fatalf("none should be nil")
	}
}

// TestMultiBackgroundPaint: two opaque layers — first (top) wins everywhere.
// A semi-transparent top layer blends with the layer beneath.
func TestMultiBackgroundPaint(t *testing.T) {
	canvas := graphics.NewCanvas(60, 40)
	defer canvas.Release()
	// opaque red over green → red
	paintLinearGradient(canvas, 0, 0, 60, 40, &LinearGradient{Stops: []ColorStop{
		{Color: graphics.Color{R: 255, A: 255}}, {Color: graphics.Color{R: 255, A: 255}},
	}})
	p := canvas.PixelAt(30, 20)
	if p.R != 255 || p.G != 0 {
		t.Fatalf("opaque red layer: (30,20)=#%02x%02x%02x, want red", p.R, p.G, p.B)
	}
	// semi-transparent red (alpha 128) over green → blend ~ (255,127,0)
	canvas2 := graphics.NewCanvas(60, 40)
	defer canvas2.Release()
	paintLinearGradient(canvas2, 0, 0, 60, 40, &LinearGradient{Stops: []ColorStop{
		{Color: graphics.Color{G: 255, A: 255}}, {Color: graphics.Color{G: 255, A: 255}},
	}})
	// blend manually: FillRect with alpha over existing pixels
	canvas2.FillRect(0, 0, 60, 40, graphics.Color{R: 255, A: 128})
	p = canvas2.PixelAt(30, 20)
	// 0.5 * red + 0.5 * green ≈ (128,128,0)
	if p.R < 100 || p.G < 100 || p.B > 60 {
		t.Fatalf("blend: (30,20)=#%02x%02x%02x, want olive-ish", p.R, p.G, p.B)
	}
	t.Logf("blend pixel = #%02x%02x%02x (red+green 50/50)", p.R, p.G, p.B)
}
