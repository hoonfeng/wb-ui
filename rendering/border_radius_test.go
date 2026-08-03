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

	// 先画背景（圆角），再画边框（per-side 路径 + 圆角外弧描边）。
	paintObjectBackground(box, info)
	PaintBorder(box, info)

	// 左边框中段 (x=11, y=30) 应为蓝色。
	if px := canvas.PixelAt(11, 30); px.A == 0 || px.B < 200 {
		t.Fatalf("left border mid (11,30) = %+v, want blue", px)
	}
	// 竖线宽 2px：x=10 与 x=11 都蓝（clip 到 box 从 x=10 起）；x=12 应透明。
	if px := canvas.PixelAt(10, 30); px.B < 200 {
		t.Fatalf("left border outer col (10,30) = %+v, want blue", px)
	}
	if px := canvas.PixelAt(12, 30); px.A != 0 {
		t.Fatalf("left border right of width (12,30) = %+v, want transparent", px)
	}
	// ★ 竖线 = 圆角矩形的一条边（左边框）：中段直边（y+4 起 2px）+ 端部沿
	//   内缩弧（圆心 x+innerR）描边带（弧带从 box 顶渐入，像蓝色阴影）。
	if px := canvas.PixelAt(11, 20); px.B < 200 {
		t.Fatalf("left border mid (11,20) = %+v, want blue", px)
	}
	// 顶部月牙：y=10（box 顶）处竖线沿外弧 r 与内弧 r-width 之间的月牙
	// 填充（包着圆角矩形）——(13,10)（月牙顶部 x=box.x+3）应有蓝色。
	if pxTop := canvas.PixelAt(13, 10); pxTop.A == 0 || pxTop.B < 150 {
		t.Fatalf("left border top lune (13,10) = %+v, want blue (lune wraps rounded rect)", pxTop)
	}
	// box 左上角外侧 (x=9, y=30) 应透明（clip 到 border-box，x>=10）。
	if px := canvas.PixelAt(9, 30); px.A != 0 {
		t.Fatalf("left of box (9,30) = %+v, want transparent", px)
	}
}
