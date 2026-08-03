package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestPaintBorderRadiusLeftSideRounded: 仅左边框有色 + border-radius 时，
// 边框两端应沿圆角收尾（Edge 对 .conv-item.active 的 border-left 渲染）。
// 回归测试：修复前 per-side 路径用矩形画左边框，圆角外仍有蓝色像素。
func TestPaintBorderRadiusLeftSideRounded(t *testing.T) {
	canvas := graphics.NewCanvas(60, 60)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 60})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	// 模拟 .conv-item.active：border-left: 2px solid #58a6ff; border-radius: 6px;
	// 其余边框透明。
	blue := style.Color{R: 88, G: 166, B: 255, A: 255}
	trans := style.Color{A: 0}
	st.BorderLeftWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderTopWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderRightWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderBottomWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderLeftColor = blue
	st.BorderTopColor = trans
	st.BorderRightColor = trans
	st.BorderBottomColor = trans
	st.BorderRadius = style.Length{Value: 6, Unit: "px"}
	st.BorderLeftStyle = "solid"
	st.BorderTopStyle = "none"
	st.BorderRightStyle = "none"
	st.BorderBottomStyle = "none"

	box := NewRenderBox(el, st)
	box.SetLocation(10, 10)
	box.SetSize(40, 40)

	// 先画背景（圆角），再画边框（走 per-side 路径 + 圆角 clip）。
	paintObjectBackground(box, info)
	PaintBorder(box, info)

	// 左边框中段 (x=11, y=30) 应为蓝色。
	if px := canvas.PixelAt(11, 30); px.A == 0 || px.B < 200 {
		t.Fatalf("left border mid (11,30) = %+v, want blue", px)
	}
	// 左上圆角外 (x=11, y=11)：radius 6 的圆弧在 y=11 处 x≈12，
	// x=11 位于圆角外侧。修复前 per-side 矩形画到 (11,11)（全蓝 A=255）；
	// 修复后圆角 clip 使该处仅剩抗锯齿渐变（A 明显 < 200）。
	if px := canvas.PixelAt(11, 11); px.A >= 200 {
		t.Fatalf("left border top corner (11,11) = %+v, want faded (rounded clip)", px)
	}
	// 左下圆角外 (x=11, y=49)（box 底 50，radius 6 → y=44 起圆角）。
	if px := canvas.PixelAt(11, 49); px.A >= 200 {
		t.Fatalf("left border bottom corner (11,49) = %+v, want faded (rounded clip)", px)
	}
	// 圆弧内侧点 (x=11, y=13)：radius 6 的左上角圆弧在 y=13 处 x≈10.8，
	// x=11 位于圆弧内侧 → 左边框（x=10-12）在该处应已绘制。
	if px := canvas.PixelAt(11, 13); px.A == 0 || px.B < 150 {
		t.Fatalf("left border inside corner (11,13) = %+v, want blue (inside radius)", px)
	}
}
