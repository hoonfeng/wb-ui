package rendering

import (
	"testing"

	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestTransformOrigin: resolveTransformOrigin + T(origin)·ops·T(-origin)
// composition — scale(2) around left-top corner must extend right/down from
// the corner, not around the center.
func TestTransformOrigin(t *testing.T) {
	// box at (10,10) size 40x20, origin left top
	ox := resolveTransformOrigin(style.Length{Value: 0, Unit: "%"}, 40)
	oy := resolveTransformOrigin(style.Length{Value: 0, Unit: "%"}, 20)
	if ox != 0 || oy != 0 {
		t.Fatalf("left top origin = (%v,%v), want (0,0)", ox, oy)
	}
	// percent center
	ox = resolveTransformOrigin(style.Length{Value: 50, Unit: "%"}, 40)
	if ox != 20 {
		t.Fatalf("50%% of 40 = %v, want 20", ox)
	}
	// px
	ox = resolveTransformOrigin(style.Length{Value: 10, Unit: "px"}, 40)
	if ox != 10 {
		t.Fatalf("10px = %v, want 10", ox)
	}
	// empty → -1 (caller falls back to center)
	if v := resolveTransformOrigin(style.Length{}, 40); v != -1 {
		t.Fatalf("empty = %v, want -1", v)
	}

	// Full pipeline: scale(2) around corner (10,10) of a 40x20 box paints
	// (10,10)-(90,50). Points at (30,20) and (80,40) must be red; (50,50)
	// (outside, below the box) must be white.
	canvas := graphics.NewCanvas(120, 80)
	defer canvas.Release()
	canvas.Save()
	canvas.Translate(10, 10)
	applyTransformOps(canvas, "scale(2)")
	canvas.Translate(-10, -10)
	canvas.FillRect(10, 10, 40, 20, graphics.Color{R: 255, A: 255})
	canvas.Restore()
	for _, p := range [][2]int{{30, 20}, {80, 40}, {50, 50}, {85, 45}} {
		px := canvas.PixelAt(p[0], p[1])
		t.Logf("(%d,%d)=#%02x%02x%02x", p[0], p[1], px.R, px.G, px.B)
	}
}
