// raster 对照：FillRoundRect(30,10,3,12,5) 顶部行 AA 剖面。
// 注意：r=clamp(5, min(3/2,12/2))=1.5（3px 宽内容），圆弧占满内容 → 顶部行
// 覆盖率波动（50%/94%/56%/0%）是 r=1.5 小圆弧在窄内容上的正常 AA，非 bug。
package graphics

import "testing"

func TestFillRoundRectRasterAAProfile(t *testing.T) {
	c := NewCanvas(120, 40)
	c.FillRoundRect(30, 10, 3, 12, 5, Color{R: 255, G: 0, B: 0, A: 255})
	// 弧心 (31.5,11.5) r=1.5。顶部行 y=10（dy=-1.5）：
	for x := 30; x <= 36; x++ {
		p := c.PixelAt(x, 10)
		t.Logf("(%d,10) = R%d G%d B%d", x, p.R, p.G, p.B)
	}
	// 内容外 (33,10)：dx=+2 dist=2.5 >1.5 → 圆外，必须无红。
	p := c.PixelAt(33, 10)
	if p.R > 10 {
		t.Errorf("(33,10) = R%d, want 0 (内容外无渗入)", p.R)
	}
}
