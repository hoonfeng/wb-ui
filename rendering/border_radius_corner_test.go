package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestPaintBorderRadiusRightTopCorner: 模拟 .folded-summary（border: 1px solid;
// border-left: 3px solid accent; border-radius: 6px）——四边宽度/颜色不一致，
// 走 per-side 圆角路径。回归测试：修复前 top/right 分支右上角弧用
// angRight(0)↔angUp(3π/2)（差值 270°），线性插值扫过右→下→左→上大弧，
// FillPath 形状错乱导致右上角向内凹陷、边线缺损；修复后右上角应呈现
// 正常外凸圆角（外弧 r 与内弧之间的月牙带）。
func TestPaintBorderRadiusRightTopCorner(t *testing.T) {
	canvas := graphics.NewCanvas(80, 60)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 60})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	borderC := style.Color{R: 240, G: 246, B: 252, A: 255} // --border-color 浅灰蓝
	accent := style.Color{R: 88, G: 166, B: 255, A: 255}   // --accent 蓝
	st.BorderTopWidth = style.Length{Value: 1, Unit: "px"}
	st.BorderRightWidth = style.Length{Value: 1, Unit: "px"}
	st.BorderBottomWidth = style.Length{Value: 1, Unit: "px"}
	st.BorderLeftWidth = style.Length{Value: 3, Unit: "px"}
	st.BorderTopColor = borderC
	st.BorderRightColor = borderC
	st.BorderBottomColor = borderC
	st.BorderLeftColor = accent
	st.BorderRadius = style.Length{Value: 6, Unit: "px"}
	st.BorderTopStyle = "solid"
	st.BorderRightStyle = "solid"
	st.BorderBottomStyle = "solid"
	st.BorderLeftStyle = "solid"

	box := NewRenderBox(el, st)
	box.SetLocation(10, 10)
	box.SetSize(60, 40)

	paintObjectBackground(box, info)
	PaintBorder(box, info)

	// 右边缘中段 (x=69, y=30) 应为 borderC（右边框 1px 竖线，x∈[69,70)）。
	if px := canvas.PixelAt(69, 30); px.A == 0 || px.B < 200 {
		t.Fatalf("right border mid (69,30) = %+v, want border color", px)
	}
	// 顶边中段 (x=50, y=10) 应为 borderC（top 边框 1px，y∈[10,11)）。
	if px := canvas.PixelAt(50, 10); px.A == 0 || px.B < 200 {
		t.Fatalf("top border mid (50,10) = %+v, want border color", px)
	}
	// ★ 右上角外凸圆角：外弧 r=6，圆心 (10+60-6, 10+6)=(64,16)。
	//   弧带内部点（外弧与内弧之间，y=13 处外弧 x∈[58.8,69.2]）应有边框色。
	//   修复前 270° 大弧把这些点挖成透明（向内凹陷）。
	if px := canvas.PixelAt(69, 13); px.A == 0 || px.B < 150 {
		t.Fatalf("top-right arc band (69,13) = %+v, want border color (no inward dent)", px)
	}
	if px := canvas.PixelAt(69, 14); px.A == 0 || px.B < 150 {
		t.Fatalf("top-right arc band (69,14) = %+v, want border color", px)
	}
	// 角外侧 (70,10) 应透明（不越过外弧顶点）。
	if px := canvas.PixelAt(70, 10); px.A != 0 {
		t.Fatalf("outside arc (70,10) = %+v, want transparent", px)
	}
	// 左上角外凸圆角也应正常：外弧顶点 (16,10) 与 (10,16) 有色。
	if px := canvas.PixelAt(16, 10); px.A == 0 || px.B < 200 {
		t.Fatalf("top-left arc apex (16,10) = %+v, want border color", px)
	}
	if px := canvas.PixelAt(10, 16); px.A == 0 || px.B < 200 {
		t.Fatalf("top-left arc left (10,16) = %+v, want border color", px)
	}
}

// TestPaintBorderRadiusRightSideRounded: 仅右边框有色 + border-radius 时，
// 与左边框对称（right 分支同样走右上/右下圆角弧）。
func TestPaintBorderRadiusRightSideRounded(t *testing.T) {
	canvas := graphics.NewCanvas(60, 60)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 60})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	blue := style.Color{R: 88, G: 166, B: 255, A: 255}
	trans := style.Color{A: 0}
	st.BorderLeftWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderTopWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderRightWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderBottomWidth = style.Length{Value: 0, Unit: "px"}
	st.BorderLeftColor = trans
	st.BorderTopColor = trans
	st.BorderRightColor = blue
	st.BorderBottomColor = trans
	st.BorderRadius = style.Length{Value: 6, Unit: "px"}
	st.BorderLeftStyle = "none"
	st.BorderTopStyle = "none"
	st.BorderRightStyle = "solid"
	st.BorderBottomStyle = "none"

	box := NewRenderBox(el, st)
	box.SetLocation(10, 10)
	box.SetSize(40, 40)

	paintObjectBackground(box, info)
	PaintBorder(box, info)

	// 右边框中段 (x=48, y=30) 应为蓝色（右边界 x=50，宽 2px → x=48,49）。
	if px := canvas.PixelAt(48, 30); px.A == 0 || px.B < 200 {
		t.Fatalf("right border mid (48,30) = %+v, want blue", px)
	}
	// 右上角外凸弧带（外弧圆心 (44,16) r=6，y=13 处外弧 x∈[38.8,49.2]）。
	if px := canvas.PixelAt(49, 13); px.A == 0 || px.B < 150 {
		t.Fatalf("right border top arc band (49,13) = %+v, want blue", px)
	}
	// 右下角外凸：y=45 处外弧 x∈[38.1,49.9]，内弧 x∈[40.1,47.9]，月牙右段 (47.9,49.9)。
	if px := canvas.PixelAt(48, 45); px.A == 0 || px.B < 150 {
		t.Fatalf("right border bottom arc band (48,45) = %+v, want blue", px)
	}
	// 角外侧 (50,10) 应透明（外弧顶点上方）。
	if px := canvas.PixelAt(50, 10); px.A != 0 {
		t.Fatalf("outside top-right arc (50,10) = %+v, want transparent", px)
	}
}
