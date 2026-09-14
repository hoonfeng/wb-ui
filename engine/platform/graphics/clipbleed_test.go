// raster 对照：ClipRoundRect(10,10,100,12,6) + FillRect(10,10,3,12,红)
// 与 GPU probe (gpu_narrow_probe.go) 同几何。验证圆弧外各 dist 点的渗入。
package graphics

import "testing"

func TestNarrowClipRasterBleedProfile(t *testing.T) {
	c := NewCanvas(120, 40)
	c.ClipRoundRect(10, 10, 100, 12, 6)
	c.FillRect(10, 10, 3, 12, Color{R: 255, G: 0, B: 0, A: 255})

	// 弧心 (16,16)。内容 x=10-12。
	// 顶部行 y=10（dy=-6）：各 x 的 dist：
	//   x=10 → dx=-6 dist=8.49  (边界外 2.49)
	//   x=11 → dx=-5 dist=7.81  (边界外 1.81)
	//   x=12 → dx=-4 dist=7.21  (边界外 1.21)
	//   x=13 → dx=-3 dist=6.71  (边界外 0.71)
	//   x=14 → dx=-2 dist=6.32  (边界外 0.32)
	//   x=15 → dx=-1 dist=6.08  (边界外 0.08)
	checks := []struct {
		x, y int
	}{
		{10, 10}, {11, 10}, {12, 10}, {13, 10}, {14, 10}, {15, 10},
	}
	for _, chk := range checks {
		p := c.PixelAt(chk.x, chk.y)
		t.Logf("(%d,%d) = R%d G%d B%d", chk.x, chk.y, p.R, p.G, p.B)
	}
}
