package rendering

import (
	"testing"

	"wb-ui/platform/graphics"
)

func TestPaintLinearGradientAngles(t *testing.T) {
	mk := func(angle float64, name string) {
		canvas := graphics.NewCanvas(30, 30)
		defer canvas.Release()
		lg := &LinearGradient{
			Direction: GradientDirection{Angle: angle, IsAngle: true},
			Stops: []ColorStop{
				{Color: graphics.Color{R: 255, G: 0, B: 0, A: 255}, Position: 0},
				{Color: graphics.Color{R: 255, G: 255, B: 0, A: 255}, Position: 1},
			},
		}
		paintLinearGradient(canvas, 0, 0, 30, 30, lg)
		// 采样 3 行 3 列
		for _, y := range []int{2, 15, 27} {
			var row []string
			for _, x := range []int{2, 15, 27} {
				p := canvas.PixelAt(x, y)
				row = append(row, "#"+hex2(p.R)+hex2(p.G)+hex2(p.B))
			}
			t.Logf("%s y=%d: %v", name, y, row)
		}
	}
	mk(0, "0deg")
	mk(45, "45deg")
	mk(90, "90deg")
	mk(135, "135deg")
}

func hex2(v uint8) string {
	const s = "0123456789abcdef"
	return string([]byte{s[v>>4], s[v&15]})
}
