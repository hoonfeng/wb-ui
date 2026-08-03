package rendering

import (
	"testing"

	"wb-ui/platform/graphics"
)

// TestClipRoundRectBasics: 圆角 clip + 整幅 FillRect，圆角外应透明。
func TestClipRoundRectBasics(t *testing.T) {
	// 矩形 clip 对照
	canvas := graphics.NewCanvas(60, 60)
	canvas.Save()
	canvas.Clip(Rect{X: 10, Y: 10, Width: 40, Height: 40})
	canvas.FillRect(0, 0, 60, 60, graphics.Color{R: 0, G: 255, B: 0, A: 255})
	canvas.Restore()
	if px := canvas.PixelAt(11, 11); px.G < 200 {
		t.Fatalf("[rect clip] inside (11,11) = %+v, want green", px)
	}
	if px := canvas.PixelAt(5, 30); px.A != 0 {
		t.Fatalf("[rect clip] outside (5,30) = %+v, want transparent", px)
	}
	canvas.Release()

	canvas = graphics.NewCanvas(60, 60)
	defer canvas.Release()
	canvas.Save()
	canvas.ClipRoundRect(10, 10, 40, 40, 6)
	canvas.FillRect(0, 0, 60, 60, graphics.Color{R: 0, G: 255, B: 0, A: 255})
	canvas.Restore()

	// 圆心区域应绿色。
	if px := canvas.PixelAt(30, 30); px.G < 200 {
		t.Fatalf("center (30,30) = %+v, want green", px)
	}
	// 左上角 (11,11)（圆角外）应仅剩抗锯齿渐变（A 明显 < 200）。
	if px := canvas.PixelAt(11, 11); px.A >= 200 {
		t.Errorf("outside corner (11,11) = %+v, want faded (rounded clip AA)", px)
	}
	// box 内圆弧内侧 (20,20) 应绿色。
	if px := canvas.PixelAt(20, 20); px.G < 200 {
		t.Fatalf("inside (20,20) = %+v, want green", px)
	}
	// box 外 (5,30) 应透明。
	if px := canvas.PixelAt(5, 30); px.A != 0 {
		t.Fatalf("outside box (5,30) = %+v, want transparent", px)
	}
	// 诊断：打印左上角区域像素
	t.Logf("pixel dump (8..20 x 8..20):")
	for y := 8; y < 20; y++ {
		row := ""
		for x := 8; x < 20; x++ {
			px := canvas.PixelAt(x, y)
			ch := "."
			if px.A > 200 {
				ch = "G"
			} else if px.A > 0 {
				ch = "g"
			}
			row += ch
		}
		t.Logf("  y=%2d %s", y, row)
	}
}
