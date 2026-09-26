package rendering

import (
	"testing"

	"wb-ui/engine/platform/graphics"
)

// TestPillTextUnderDprScale reproduces the desktop window's HiDPI paint path:
// app/host.go does gpuCanvas.Scale(csX, csY) and then rendering.Paint with CSS
// coordinates. The glyph ink must stay vertically centered at every device
// scale factor (the reported "tag text not vertically centered" is 1.25x only).
func TestPillTextUnderDprScale(t *testing.T) {
	const W, H = 320, 40
	for _, s := range []float64{1.0, 1.25, 1.5, 2.0} {
		devW := int(float64(W)*s + 0.5)
		devH := int(float64(H)*s + 0.5)
		rv := buildPill(t, pillHTML, W, H)
		g := collectPill(rv)
		if g.h == 0 {
			t.Fatal("no pill geometry")
		}
		canvas := graphics.NewCanvas(devW, devH)
		canvas.Scale(s, s)
		Paint(rv, canvas, Rect{X: 0, Y: 0, Width: W, Height: H})

		top := int(g.top*s + 0.5)
		bot := int((g.top+g.h)*s + 0.5)
		pad := int((g.h/2)*s) + 3 // 避开 999px 圆角
		x0 := int(g.left*s+0.5) + pad
		x1 := int((g.left+g.w)*s+0.5) - pad

		cnt := map[uint32]int{}
		for y := top; y < bot; y++ {
			for x := x0; x < x1; x++ {
				p := canvas.PixelAt(x, y)
				cnt[uint32(p.R)<<16|uint32(p.G)<<8|uint32(p.B)]++
			}
		}
		bestN, bestK := -1, uint32(0)
		for k, n := range cnt {
			if n > bestN {
				bestN, bestK = n, k
			}
		}
		bgR, bgG, bgB := uint8(bestK>>16), uint8(bestK>>8), uint8(bestK)

		ink := []int{}
		for y := top; y < bot; y++ {
			n := 0
			for x := x0; x < x1; x++ {
				p := canvas.PixelAt(x, y)
				if p.A == 0 {
					continue
				}
				d := probeAbs(int(p.R)-int(bgR)) + probeAbs(int(p.G)-int(bgG)) + probeAbs(int(p.B)-int(bgB))
				if d > 60 {
					n++
				}
			}
			if n > 0 {
				ink = append(ink, y)
			}
		}
		if len(ink) == 0 {
			t.Fatalf("scale %.2f: no ink inside pill (bg=#%02x%02x%02x)", s, bgR, bgG, bgB)
		}
		first, last := ink[0], ink[len(ink)-1]
		aboveCSS := float64(first-top) / s
		belowCSS := float64((bot-1)-last) / s
		inkH := float64(last-first+1) / s
		verdict := "CENTERED"
		if diff := aboveCSS - belowCSS; diff > 0.7 {
			verdict = "TEXT HIGH"
		} else if diff < -0.7 {
			verdict = "TEXT LOW"
		}
		t.Logf("scale=%.2f dev=%dx%d | ink dev rows %d..%d | CSS above=%.2f below=%.2f inkH=%.2f | boxH=%.1f => %s (offset %.2f)",
			s, devW, devH, first, last, aboveCSS, belowCSS, inkH, g.h, verdict, (aboveCSS-belowCSS)/2)
		if p := savePNG(canvas, "wbui_pill_scale"+formatScale(s)+".png"); p != "" {
			t.Logf("   PNG: %s", p)
		}
		canvas.Release()
	}
}

func formatScale(s float64) string {
	switch s {
	case 1.0:
		return "_100"
	case 1.25:
		return "_125"
	case 1.5:
		return "_150"
	case 2.0:
		return "_200"
	}
	return ""
}
