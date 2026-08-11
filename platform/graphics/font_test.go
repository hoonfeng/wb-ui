package graphics

import (
	"testing"
)

func TestGetMatrixTranslate(t *testing.T) {
	c := NewCanvas(200, 200)
	defer c.Release()
	m := c.GetMatrix()
	t.Logf("初始: scaleX=%.2f scaleY=%.2f skewX=%.2f skewY=%.2f tx=%.2f ty=%.2f",
		float64(m.ScaleX), float64(m.ScaleY), float64(m.SkewX), float64(m.SkewY), float64(m.TransX), float64(m.TransY))
	c.Save()
	c.Translate(0, -400)
	m2 := c.GetMatrix()
	t.Logf("Translate(0,-400) 后: ty=%.2f tx=%.2f", float64(m2.TransY), float64(m2.TransX))
	// 再验证：translate 后画文字，像素应在 y=-400 附近（视口外）
	c.DrawText(100, 100, "T", Font{Family: "sans-serif", Size: 20, Weight: 400, Style: "normal"}, Color{R: 255, G: 0, B: 0, A: 255})
	p := c.PixelAt(100, 96)
	t.Logf("画在 (100,100) translate 后，PixelAt(100,96) = #%02x%02x%02x（预期背景=全0）", p.R, p.G, p.B)
	c.Restore()
	m3 := c.GetMatrix()
	t.Logf("Restore 后: ty=%.2f", float64(m3.TransY))
}
