package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// ── 常驻滚动条预留（scrollbarreserve.go）的行为测试 ──────────────────────
//
// 对应缺陷：滚动容器给子元素的可用宽度未扣除常驻滚动条，导致 width:auto /
// 100% 的子元素（含 border-bottom）铺到滚动条上。判定见 scrollbarreserve.go。
//
// 注意构造：Layout(root, ...) 会把 root 当作视口根（宽度被置为视口宽），
// 因此必须再套一层 viewport，把被测滚动容器作为其子盒（宽 200）。

// scrollCase 构造 [viewport [scrollContainer [5×40 高的子块]]]，返回 viewport
// 与滚动容器。
func scrollCase(ov style.OverflowType) (*ElementBox, *ElementBox) {
	viewport := mkBlock()
	container := mkBlockWH(200, 100)
	container.style.OverflowY = ov
	viewport.AddChild(container)
	for i := 0; i < 5; i++ {
		container.AddChild(mkBlockWH(0, 40)) // 5 × 40 = 200 > 视口 100 → 溢出
	}
	return viewport, container
}

func firstChildWidth(t *testing.T, container *ElementBox, state *LayoutState) float64 {
	t.Helper()
	if len(container.Children()) == 0 {
		t.Fatalf("container has no children")
	}
	child, ok := container.Children()[0].(*ElementBox)
	if !ok {
		t.Fatalf("child is not *ElementBox")
	}
	_, _, w, _ := rectOf(child, state)
	return w
}

// TestScrollbarReserve_MarkedContainerReservesChildWidth：overflow-y:auto 的
// 容器一旦被判定「需要常驻滚动条」（由渲染树收尾 pass 设置粘性标记，见
// rendering.syncScrollbarReserve——判定不能在布局内做，因为 flex 分配的
// 高度在 BFC 收尾时尚未确定），下一次布局必须为滚动条让出宽度：
// 子元素宽度 = 容器宽 - 15（scrollbar-color 自绘滚动条宽，有头 Edge 实测）。
//
// 构造说明：标记必须在 Layout 之前设置，且只跑一轮——多轮 Layout 会走增量
// 剪枝，子盒几何不再重算，读到的宽度不可靠（这正是判定必须由渲染树收尾
// pass 在几何就位后驱动重排的原因）。
func TestScrollbarReserve_MarkedContainerReservesChildWidth(t *testing.T) {
	viewport, container := scrollCase(style.OverflowAuto)
	if !container.SetVerticalScrollbarReserved(true) {
		t.Fatal("SetVerticalScrollbarReserved(true) 首次调用应报告标记变化")
	}
	state := Layout(viewport, 800, 600)
	assertApprox(t, "child width (reserved)", firstChildWidth(t, container, state), 185)
}

// TestSetVerticalScrollbarReserved_ChangeSemantics：粘性标记的返回值语义——
// 只有真正发生变化时才报告 true（调用方据此决定是否补跑一轮布局）。
func TestSetVerticalScrollbarReserved_ChangeSemantics(t *testing.T) {
	_, container := scrollCase(style.OverflowAuto)

	if !container.SetVerticalScrollbarReserved(true) {
		t.Error("首次置 true 应报告变化")
	}
	if container.SetVerticalScrollbarReserved(true) {
		t.Error("重复置同一值不应报告变化")
	}
	if got := container.verticalScrollbarReserve(); got != 15 {
		t.Errorf("标记为 true 时 reserve = %v, want 15", got)
	}
	if !container.SetVerticalScrollbarReserved(false) {
		t.Error("翻回 false 应报告变化")
	}
	if got := container.verticalScrollbarReserve(); got != 0 {
		t.Errorf("标记为 false 时 reserve = %v, want 0", got)
	}
}

// TestScrollbarReserve_AutoNoOverflowNoReserve：overflow-y:auto 但内容不溢出
// 时不得预留（否则容器右侧会凭空多出一条空白）。
func TestScrollbarReserve_AutoNoOverflowNoReserve(t *testing.T) {
	viewport := mkBlock()
	container := mkBlockWH(200, 300)
	container.style.OverflowY = style.OverflowAuto
	viewport.AddChild(container)
	container.AddChild(mkBlockWH(0, 40)) // 40 < 300 → 不溢出

	// 单轮即可：不溢出 → 粘性标记恒为 false，不会有重排，多轮结果相同
	// （第二轮的增量剪枝会复用几何，LayoutState 每轮新建，故不再重复断言）。
	state := Layout(viewport, 800, 600)
	assertApprox(t, "child width (no overflow)", firstChildWidth(t, container, state), 200)
}

// TestScrollbarReserve_ScrollAlwaysReserves：overflow-y:scroll 的滚动条常驻，
// 无论内容是否溢出都占位（第一轮即生效）。
func TestScrollbarReserve_ScrollAlwaysReserves(t *testing.T) {
	viewport, container := scrollCase(style.OverflowScroll)

	state := Layout(viewport, 800, 600)
	assertApprox(t, "child width under overflow:scroll", firstChildWidth(t, container, state), 185)
}

// TestScrollbarReserve_VisibleAndHiddenDoNotReserve：visible / hidden 不产生
// 常驻滚动条，不得预留。
func TestScrollbarReserve_VisibleAndHiddenDoNotReserve(t *testing.T) {
	for _, ov := range []style.OverflowType{style.OverflowVisible, style.OverflowHidden} {
		viewport, container := scrollCase(ov)
		state := Layout(viewport, 800, 600)
		assertApprox(t, "child width (no scrollbar)", firstChildWidth(t, container, state), 200)
	}
}

// TestScrollbarReserve_ScrollbarWidthNoneReservesNothing：scrollbar-width:none
// 时滚动条宽为 0（仍可滚动），因此即使 overflow:scroll 也不占位。
func TestScrollbarReserve_ScrollbarWidthNoneReservesNothing(t *testing.T) {
	viewport, container := scrollCase(style.OverflowScroll)
	container.style.Properties["scrollbar-width"] = "none"

	state := Layout(viewport, 800, 600)
	assertApprox(t, "child width (scrollbar-width:none)", firstChildWidth(t, container, state), 200)
}

// TestScrollbarWidth_Resolution 覆盖 style.ScrollbarWidth 的各分支
// （布局预留与绘制几何共用该值）。
func TestScrollbarWidth_Resolution(t *testing.T) {
	cases := []struct {
		name     string
		props    map[string]string
		expected float64
	}{
		{"default", nil, 15},
		{"thin", map[string]string{"scrollbar-width": "thin"}, 10},
		{"none", map[string]string{"scrollbar-width": "none"}, 0},
		{"webkit-4px", map[string]string{"-webkit-scrollbar-width": "4px"}, 4},
		// 说明：沿用既有实现顺序（先看标准属性 scrollbar-width，再回落到
		// -webkit-scrollbar-width），此处如实记录现状而非理想语义。
		{"thin-precedes-webkit", map[string]string{"scrollbar-width": "thin", "-webkit-scrollbar-width": "6px"}, 10},
		{"garbage-webkit-falls-back", map[string]string{"-webkit-scrollbar-width": "abc"}, 15},
		// ★ Chromium 语义（2026-09-26 有头 Edge 实测）：非 auto 的 scrollbar-color 会
		// **压制** ::-webkit-scrollbar 自定义宽度，改用其 15px 自绘滚动条。gou-ide 全页
		// 继承 html 的 scrollbar-color，走的正是这条路径。
		{"scrollbar-color-suppresses-webkit", map[string]string{"scrollbar-color": "#414b64 transparent", "-webkit-scrollbar-width": "4px"}, 15},
		{"scrollbar-color-auto-keeps-webkit", map[string]string{"scrollbar-color": "auto", "-webkit-scrollbar-width": "4px"}, 4},
	}
	for _, tc := range cases {
		cs := &style.ComputedStyle{}
		if tc.props != nil {
			cs.Properties = map[string]string{}
			for k, v := range tc.props {
				cs.Properties[k] = v
			}
		}
		if got := style.ScrollbarWidth(cs); got != tc.expected {
			t.Errorf("%s: ScrollbarWidth = %v, want %v", tc.name, got, tc.expected)
		}
	}
	if got := style.ScrollbarWidth(nil); got != 15 {
		t.Errorf("nil style: ScrollbarWidth = %v, want 15", got)
	}
}
