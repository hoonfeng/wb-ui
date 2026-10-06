package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestFlex_ColumnItemMaxWidthClampsGeometry 回归（AI-PS 1920 视口缺陷，2026-10）：
// column flex 容器内、交叉轴尺寸由父级决定的 item（width:100%），其**自身 max-width**
// 必须约束 item 的交叉轴几何宽度，且与其内部子项的百分比基准保持一致。
//
// 修复前的引擎行为（本案）：
//   - item 几何未应用自身 max-width → 宽 1920（父给的 100% 基准）；
//   - 但 item 内部布局（FlexFormattingContext.Layout 的 cw 计算）按自身 max-width
//     把交叉轴基准 clamp 成 1440 → 内部子项 width:100% = 1440。
//     → 容器 1920 / 子项 1440，同一 max-width 两处取值不同（右侧留白 480）。
//
// 浏览器行为（Chromium 实测 computed max-width / 几何）：容器与子项都是 1440。
// 即 max-width 是元素自身约束，必须作用于几何，而不是只作用于子项基准。
func TestFlex_ColumnItemMaxWidthClampsGeometry(t *testing.T) {
	root := mkFlex()
	root.Style().FlexDirection = "column"

	// .screen-main 等价物：父 flex column 的 item，自身也是 column flex。
	a := mkFlex()
	a.Style().FlexDirection = "column"
	a.Style().Width = style.Length{Value: 100, Unit: "%"}
	a.Style().MaxWidth = style.Length{Value: 1440, Unit: "px"}

	// .d-main_app 等价物：靠 width:100% 铺满容器。
	b := mkBlock()
	b.Style().Width = style.Length{Value: 100, Unit: "%"}

	a.AddChild(b)
	root.AddChild(a)

	state := Layout(root, 1920, 1080)
	_, _, aw, _ := rectOf(a, state)
	_, _, bw, _ := rectOf(b, state)

	if !approxEq(aw, 1440) {
		t.Errorf("外层 item 宽度 = %g, want 1440（自身 max-width 必须约束 flex 交叉轴几何）", aw)
	}
	if !approxEq(bw, aw) {
		t.Errorf("内部子项宽度 = %g, want %g（交叉轴基准不得与容器几何脱节）", bw, aw)
	}
}

// TestFlex_ColumnItemMaxWidthPercent 实测百分比 max-width 的支持性：
// max-width 的百分比按**包含块内容宽**解析（CSS-SIZING-3 §5.2）——width:100% 的
// column flex item 配 max-width:50% ⇒ 宽度 = 容器内容宽 / 2。
func TestFlex_ColumnItemMaxWidthPercent(t *testing.T) {
	root := mkFlex()
	root.Style().FlexDirection = "column"

	a := mkFlex()
	a.Style().FlexDirection = "column"
	a.Style().Width = style.Length{Value: 100, Unit: "%"}
	a.Style().MaxWidth = style.Length{Value: 50, Unit: "%"}

	root.AddChild(a)

	state := Layout(root, 1920, 1080)
	_, _, aw, _ := rectOf(a, state)
	if !approxEq(aw, 960) {
		t.Errorf("item 宽度 = %g, want 960（max-width:50%% × 容器 1920）", aw)
	}
}
