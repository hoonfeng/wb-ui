package webkit

import (
	"testing"

	"wb-ui/engine/rendering"
)

// TestHitTestUsesVisualCoordsInsideTransformedAncestor 钉死：命中测试必须与
// 绘制/几何使用**同一坐标系（视觉坐标）**，而不是布局坐标。
//
// gou-ide 顶栏 .tb-nav 用 left:50% + translate(-50%) 绝对居中：
//
//	nav 视觉 750.5..849.5  （布局 800..899）
//	p1  视觉 750.5..792.5  （布局 800..842）
//
// 若命中用布局坐标：用户点击**看到的**胶囊（x≈760）会 miss（「点击无反应」），
// 反而点击视觉空白处（x≈880）会命中 p2。这正是本轮几何修复（D1）必须配套
// 覆盖的交互面；垂直方向同理（D2：绝对定位子元素的 flex 静态位置 y=8..32 vs
// 布局 0..24）。
func TestHitTestUsesVisualCoordsInsideTransformedAncestor(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1600, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="bar" style="display:flex;align-items:center;height:40px;position:relative;background:#222222">
<nav id="nav" style="position:absolute;left:50%;transform:translate(-50%);display:flex;gap:4px;background:#333333">
<button id="p1" style="width:42px;height:24px">A</button>
<button id="p2" style="width:53px;height:24px">B</button>
</nav></div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 为空")
	}
	hitID := func(x, y float64) string {
		el := rendering.HitTest(rv, x, y, "")
		if el == nil {
			return "(nil)"
		}
		if id := el.GetAttribute("id"); id != "" {
			return id
		}
		return el.NodeName()
	}
	// ① 视觉坐标 (760,15) 落在 p1 的视觉范围，但落在 nav 的**布局**范围之外
	//    → 只有按视觉坐标命中才应得到 p1。
	if got := hitID(760, 15); got != "p1" {
		t.Fatalf("视觉坐标 (760,15) 未命中 p1：got %q（命中测试是否仍用布局坐标？）", got)
	}
	// ② 反向守卫：(880,15) 落在 nav 的布局范围内、视觉上已在 nav 右侧之外
	//    → 不得命中 p2。
	if got := hitID(880, 15); got == "p2" {
		t.Fatalf("布局坐标 (880,15) 命中了 p2：命中测试未使用视觉坐标")
	}
	// ③ 垂直方向（D2）：pill 视觉 y=8..32、布局 y=0..24 → (770,30) 只在视觉
	//    范围内，应命中 p1。
	if got := hitID(770, 30); got != "p1" {
		t.Fatalf("视觉坐标 (770,30) 未命中 p1：got %q（abs 静态位置的视觉 y 未参与命中）", got)
	}
}
