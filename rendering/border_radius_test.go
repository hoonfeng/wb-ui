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
	// ★ 普通圆角竖线：全高 width 矩形 + 两端小圆角（FillRoundRect）。
	//   竖线中段（y=14+）为 2px 满蓝；box 顶（y=10）处圆角渐入（半圆帽）。
	if px := canvas.PixelAt(11, 20); px.B < 200 {
		t.Fatalf("left border mid (11,20) = %+v, want blue", px)
	}
	// 顶部圆角：y=10（box 顶）处竖线应为圆角渐入（A < 255 非满蓝；> 0 有圆角）。
	pxTop := canvas.PixelAt(11, 10)
	if pxTop.A == 0 || pxTop.A >= 255 {
		t.Fatalf("left border top corner (11,10) = %+v, want rounded cap (0 < A < 255)", pxTop)
	}
	// box 左上角外侧 (x=9, y=30) 应透明（clip 到 border-box，x>=10）。
	if px := canvas.PixelAt(9, 30); px.A != 0 {
		t.Fatalf("left of box (9,30) = %+v, want transparent", px)
	}
}
