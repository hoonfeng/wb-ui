package graphics

import (
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestStrokeWidthScaling: 验证 skia 的 stroke width 是否随 canvas CTM 缩放。
// scale(0.5) 后画 stroke-width=2 的线，若 CTM 生效则实际约 1px。
func TestStrokeWidthScaling(t *testing.T) {
	c := NewCanvas(20, 20)
	c.Save()
	c.Scale(0.5, 0.5)
	c.StrokeLine(2, 10, 18, 10, 2, Color{R: 255, G: 0, B: 0, A: 255})
	c.Restore()
	pix := c.Pixels()
	// 打印 x=5 列全部像素观察线位置
	for y := 0; y < 20; y++ {
		off := (y*20 + 5) * 4
		if pix[off] > 50 || pix[off+1] > 50 || pix[off+2] > 50 {
			t.Logf("x=5 y=%d rgba=(%d,%d,%d,%d)", y, pix[off], pix[off+1], pix[off+2], pix[off+3])
		}
	}
	// scale(0.5) 后线(世界 x=2..18) 画在屏幕 x=1..9。量 x=5 处 y 方向红色像素数。
	count := 0
	for y := 0; y < 20; y++ {
		off := (y*20 + 5) * 4
		if pix[off] > 100 && pix[off+1] < 100 {
			count++
		}
	}
	t.Logf("stroke thickness at x=5: %d px (scale 0.5, width 2 → want ≈1-2)", count)
	_ = skia.RGB
}
