package rendering

import (
	"testing"
)

// TestSVGPathCompactArcFlags: Lucide 图标的圆/弧普遍用紧凑写法 —— 圆弧标志位
// 与坐标之间**不留空格**：`a8 8 0 100-16`（= rx 8 ry 8 rot 0 large-arc 1
// sweep 0 dx 0 dy -16）。它与展开写法 `a8 8 0 1 0 0 -16` 必须画出同一个圆。
//
// 实测症状：时钟/刷新等图标只画出一段弧（圆不完整），user 图标的圆头像消失
// 只剩两条肩线（看起来像「#」）。
func TestSVGPathCompactArcFlags(t *testing.T) {
	compact := renderSVGPath(t, 24, 24, "M12 20a8 8 0 100-16 8 8 0 000 16",
		map[string]string{"stroke-width": "1.6"})
	defer compact.Release()
	spaced := renderSVGPath(t, 24, 24, "M12 20a8 8 0 1 0 0 -16 8 8 0 0 0 0 16",
		map[string]string{"stroke-width": "1.6"})
	defer spaced.Release()

	// 半径 8、圆心 (12,12) 的圆：四个极值点。
	pts := [][2]int{{12, 4}, {20, 12}, {12, 20}, {4, 12}}
	for _, pt := range pts {
		if px := compact.PixelAt(pt[0], pt[1]); px.A == 0 {
			t.Errorf("紧凑写法：圆上 (%d,%d) 缺像素（圆弧没画出来）", pt[0], pt[1])
		}
		if px := spaced.PixelAt(pt[0], pt[1]); px.A == 0 {
			t.Errorf("展开写法：圆上 (%d,%d) 缺像素", pt[0], pt[1])
		}
	}
}

// TestSVGPathUserIconShape: Lucide user 图标（圆头 + 肩线）整体形状 —— 圆头若
// 缺失就只剩两条线，看起来像「#」。
func TestSVGPathUserIconShape(t *testing.T) {
	canvas := renderSVGPath(t, 24, 24,
		"M12 12a4 4 0 100-8 4 4 0 000 8M4.5 20c1.5-3.5 4.2-5 7.5-5s6 1.5 7.5 5",
		map[string]string{"stroke-width": "1.6"})
	defer canvas.Release()

	// 头部圆（圆心 12,8 半径 4）的上下极值必须有像素。
	for _, pt := range [][2]int{{12, 4}, {12, 12}, {8, 8}, {16, 8}} {
		if px := canvas.PixelAt(pt[0], pt[1]); px.A == 0 {
			t.Errorf("user 图标：头部圆上 (%d,%d) 缺像素", pt[0], pt[1])
		}
	}
}
