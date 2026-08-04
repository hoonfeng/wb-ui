// 对照测试：raster 后端 ClipRoundRect 在圆弧边界（dist=r 切点）的 AA 行为。
// GPU probe (gpu_clip_probe.go) 显示边界点 (16,10)/(10,16) 为纯内容色（未裁切），
// 浏览器 Edge 在弧顶边界为纯背景 → 需确认 raster 正常。
package graphics

import "testing"

func TestClipRoundRectEdgeAA(t *testing.T) {
	c := NewCanvas(64, 64)
	c.ClipRoundRect(10, 10, 40, 40, 6)
	c.FillRect(0, 0, 64, 64, Color{R: 0, G: 255, B: 0, A: 255})

	// 弧心 (16,16) r=6。
	// 边界点 (dist=r=6)：
	//   (16,10) 弧顶正上方 dy=-6, dx=0 → dist=6 边界
	//   (10,16) 左弧顶     dx=-6, dy=0 → dist=6 边界
	//   (12,12) 对角 45°   dist=5.66 <6 圆内 → 绿
	// 圆外点 (dist>6)：
	//   (10,10) 对角 dist=8.49 → 背景
	//   (17,10) dy=-6 dx=1 → dist=6.08 → 背景(圆外 1px)
	checks := []struct {
		x, y int
		name string
	}{
		{16, 10, "弧顶边界 (dist=6)"},
		{10, 16, "左弧顶边界 (dist=6)"},
		{10, 10, "对角圆外 (dist=8.49)"},
		{17, 10, "弧顶外 1px (dist=6.08)"},
		{12, 12, "对角圆内 (dist=5.66)"},
		{16, 16, "弧心 (dist=0)"},
	}
	for _, chk := range checks {
		p := c.PixelAt(chk.x, chk.y)
		t.Logf("(%d,%d) %s = R%d G%d B%d", chk.x, chk.y, chk.name, p.R, p.G, p.B)
	}
	// 断言：圆内点全绿。
	p := c.PixelAt(16, 16)
	if !(p.G > 200 && p.R < 100 && p.B < 100) {
		t.Errorf("弧心 (16,16) = R%d G%d B%d, want 绿", p.R, p.G, p.B)
	}
}
