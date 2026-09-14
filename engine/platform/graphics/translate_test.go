package graphics

import (
	"testing"
)

func TestClipTranslateDrawText(t *testing.T) {
	c := NewCanvas(200, 200)
	defer c.Release()
	// 模拟 scroller：clip (0,60,200,140) + translate(0,-100)
	c.Save()
	c.Clip(Rect{X: 0, Y: 60, Width: 200, Height: 140})
	c.Translate(0, -100)
	// baseline 155 → device 55（clip 外 60 上方）
	c.DrawText(50, 155, "A", Font{Family: "sans-serif", Size: 14, Weight: 400, Style: "normal"}, Color{R: 255, G: 255, B: 0, A: 255})
	// baseline 170 → device 70（clip 内）
	c.DrawText(50, 170, "B", Font{Family: "sans-serif", Size: 14, Weight: 400, Style: "normal"}, Color{R: 255, G: 0, B: 0, A: 255})
	// 读像素：B 在 device (52, 57-70)
	hitB := false
	for _, yy := range []int{58, 60, 62, 64, 66, 68} {
		p := c.PixelAt(52, yy)
		if p.R > 150 && p.G < 100 {
			hitB = true
			t.Logf("  B PixelAt(52,%d)=#%02x%02x%02x", yy, p.R, p.G, p.B)
		}
	}
	if !hitB {
		t.Errorf("clip+translate 内文字 B 未画出")
	}
	c.Restore()
	t.Log("clip+translate+drawtext 测试完成")
}
