package rendering

import (
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// borderCornerProbe 是拐角矩形内的一个采样点：dx/dy 相对角矩形左上角，
// horizontal=true 表示该点应属于水平边（top/bottom）的颜色，false 表示属于
// 垂直边（left/right）的颜色。
type borderCornerProbe struct {
	dx, dy     int
	horizontal bool
}

// TestBorderCornerColorDirection 锁定边框拐角的颜色归属方向（CSS border
// corner joining）：拐角矩形被「外角 → 内角」的对角线分成两半，**每条边的
// 颜色占据含该边外边缘的那一半**。
//
// 参照基准是真实浏览器（Edge headless 实测四角矩阵，见 docs/CALIB.md
// 「边框拐角」节）：left 颜色在 top-left/bottom-left 角占靠左半边，right 颜色
// 在 top-right/bottom-right 角占靠右半边；top/bottom 占另一半。
//
// 回归背景：两个底部角此前把三角形填到了反侧（bottom-left 把右下三角给 left、
// bottom-right 把左下三角给 right），导致相邻边框颜色互换。这会让 cssprobe 的
// logical-borders 夹具里「右边框色块」被底边框整行切断成两个不连通块
// （10x74 + 10x5 而不是 10x79）。
func TestBorderCornerColorDirection(t *testing.T) {
	horiz := style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF} // top/bottom 色
	vert := style.Color{R: 0, G: 0, B: 0xFF, A: 0xFF}  // left/right 色
	horizPx := graphics.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	vertPx := graphics.Color{R: 0, G: 0, B: 0xFF, A: 0xFF}

	// 盒子的边框盒：位于 (2,2)，尺寸 14x16（各角矩形互不重叠）。
	const boxX, boxY, boxW, boxH = 2.0, 2.0, 14.0, 16.0

	cases := []struct {
		name                         string
		topW, rightW, bottomW, leftW float64
		// 角矩形（相对画布）与采样点
		cornerX, cornerY, cw, ch float64
		probes                   []borderCornerProbe
	}{
		{
			name: "top-left: top 色占右上、left 色占左下",
			topW: 3, leftW: 4,
			cornerX: boxX, cornerY: boxY, cw: 4, ch: 3,
			probes: []borderCornerProbe{
				{3, 0, true},  // 靠上 → top 色
				{0, 2, false}, // 靠左 → left 色
				{0, 0, true},  // 外角像素保留 top 色
			},
		},
		{
			name: "top-right: top 色占左上、right 色占右下",
			topW: 3, rightW: 6,
			cornerX: boxX + boxW - 6, cornerY: boxY, cw: 6, ch: 3,
			probes: []borderCornerProbe{
				{0, 0, true},  // 左上 → top 色
				{5, 2, false}, // 右下 → right 色
				{5, 0, true},  // 右上 → top 色
			},
		},
		{
			name:    "bottom-left: left 色占左上、bottom 色占右下",
			bottomW: 5, leftW: 4,
			cornerX: boxX, cornerY: boxY + boxH - 5, cw: 4, ch: 5,
			probes: []borderCornerProbe{
				{0, 0, false}, // 左上 → left 色（回归点：修复前为 bottom 色）
				{0, 4, false}, // 左下 → left 色
				{3, 4, true},  // 右下 → bottom 色（回归点：修复前为 left 色）
			},
		},
		{
			name:    "bottom-right: right 色占右上、bottom 色占左下",
			bottomW: 5, rightW: 6,
			cornerX: boxX + boxW - 6, cornerY: boxY + boxH - 5, cw: 6, ch: 5,
			probes: []borderCornerProbe{
				{5, 0, false}, // 右上 → right 色（回归点：修复前为 bottom 色）
				{5, 4, false}, // 右下 → right 色（回归点：修复前为 bottom 色）
				{0, 4, true},  // 左下 → bottom 色（回归点：修复前为 right 色）
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canvas := graphics.NewCanvas(20, 20)
			info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})
			st := style.NewComputedStyle()
			setSide := func(w *style.Length, c *style.Color, s *string, width float64, col style.Color) {
				if width <= 0 {
					return
				}
				*w = style.Length{Value: width, Unit: "px"}
				*c = col
				*s = "solid"
			}
			setSide(&st.BorderTopWidth, &st.BorderTopColor, &st.BorderTopStyle, tc.topW, horiz)
			setSide(&st.BorderBottomWidth, &st.BorderBottomColor, &st.BorderBottomStyle, tc.bottomW, horiz)
			setSide(&st.BorderLeftWidth, &st.BorderLeftColor, &st.BorderLeftStyle, tc.leftW, vert)
			setSide(&st.BorderRightWidth, &st.BorderRightColor, &st.BorderRightStyle, tc.rightW, vert)

			box := newBoxWithStyle(st, boxX, boxY, boxW, boxH)
			PaintBorder(box, info)

			for _, p := range tc.probes {
				x := int(tc.cornerX) + p.dx
				y := int(tc.cornerY) + p.dy
				want := vertPx
				side := "vert"
				if p.horizontal {
					want = horizPx
					side = "horiz"
				}
				if got := canvas.PixelAt(x, y); got != want {
					t.Errorf("角矩形(%v,%v %vx%v) 内 (%d,%d) = %+v，want %+v（应属于 %s 边）",
						tc.cornerX, tc.cornerY, tc.cw, tc.ch, p.dx, p.dy, got, want, side)
				}
			}
		})
	}
}
