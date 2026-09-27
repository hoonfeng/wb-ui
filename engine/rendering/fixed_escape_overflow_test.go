package rendering

import (
	"testing"

	"wb-ui/engine/platform/graphics"
)

// TestFixedEscapesAncestorOverflowClip —— 视口固定的元素不得被祖先
// overflow:hidden 裁剪（CSS 2.1 §11.1.1：fixed 的包含块恒为视口，
// 祖先的 overflow 裁剪只作用于「包含块在该裁剪元素内」的后代）。
//
// 场景来源（真实缺陷）：PairCode IDE 顶部「帮助」菜单
// （.menu-dropdown = position:fixed; z-index:9999）挂在标题栏内，而
// 标题栏祖先 .plugin-slot-host 是 overflow:hidden（40px 高）——菜单
// 被裁进标题栏，视觉上「菜单被遮挡 / 不在最前」。
//
// 实测取证（桌面壳 headless，注入 !important 规则使 computed 确认
// position=fixed、z-index=9999、背景 #00ff00）：整个菜单矩形内绿色只占
// 4%，y=29..38 可见、y>=40 全被页面底色覆盖 —— 即被祖先裁剪。
func TestFixedEscapesAncestorOverflowClip(t *testing.T) {
	docHTML := `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
		body{margin:0;overflow:hidden}
		#host{overflow:hidden;height:40px;background:#101010}
		#bar{height:40px}
		#menu{position:fixed;left:60px;top:120px;width:200px;height:160px;background:#00ff00;z-index:9999}
	</style></head><body>
		<div id="host"><div id="bar"><div id="menu"></div></div></div>
	</body></html>`
	rv := buildDocHTML(t, 400, 300, docHTML)
	dumpLayers(t, rv)

	canvas := graphics.NewCanvas(400, 300)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 400, Height: 300})

	// 菜单几何 (60,120) 200x160：完全落在 #host（y 0..40）之外，
	// 正确实现下必须按视口坐标完整绘制。
	if got := canvas.PixelAt(160, 200); got.G < 200 || got.R > 100 {
		t.Errorf("菜单中心 (160,200) = %+v，期望绿色（fixed 逃逸祖先 overflow 裁剪）", got)
	}
	if got := canvas.PixelAt(160, 130); got.G < 200 || got.R > 100 {
		t.Errorf("菜单上沿 (160,130) = %+v，期望绿色（fixed 逃逸祖先 overflow 裁剪）", got)
	}
	// 菜单下方的祖先区域仍应是 #host 自己的底色（祖先背景不被破坏）。
	if got := canvas.PixelAt(20, 20); got.R > 60 || got.G > 60 {
		t.Errorf("#host 底色 (20,20) = %+v，期望深灰 #101010", got)
	}
}
