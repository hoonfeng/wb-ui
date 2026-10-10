package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestFlexColumnIntrinsicChildHeight: a flex-column item whose child has an
// explicit height must size to that height (not collapse to the line gap).
// Regression: chat-input-area (flex row) got 25px instead of the 150px
// textarea it contains because intrinsicContentHeight used fontLineGap.
func TestFlexColumnIntrinsicChildHeight(t *testing.T) {
	container := mkBlock()
	container.style.Display = style.DisplayFlex
	container.style.FlexDirection = "column"
	container.style.Width = style.Length{Value: 400, Unit: "px"}
	container.style.Height = style.Length{Value: 500, Unit: "px"}

	// An auto-height flex item (row flex) containing a 150px block child.
	item := mkBlock()
	item.style.Display = style.DisplayFlex // row
	item.style.Width = style.Length{Value: 400, Unit: "px"}
	textarea := mkBlock()
	textarea.style.Height = style.Length{Value: 150, Unit: "px"}
	item.AddChild(textarea)
	container.AddChild(item)

	root := mkBlock()
	root.AddChild(container)
	state := Layout(root, 400, 500)
	g := state.GeometryForBox(item)
	if g.BorderBoxHeight() < 149 {
		t.Fatalf("flex item height=%.0f, want ≈150 (explicit child height)", g.BorderBoxHeight())
	}
}

// TestBlockIntrinsicSum: a block container with two stacked block children
// must report the SUM of their heights, not the max — so a toolbar under a
// textarea pushes the container taller.
func TestBlockIntrinsicSum(t *testing.T) {
	// Use intrinsicContentHeight directly on a block with two children.
	container := mkBlock()
	container.style.Width = style.Length{Value: 300, Unit: "px"}
	tb := mkBlock()
	tb.style.Height = style.Length{Value: 150, Unit: "px"}
	bar := mkBlock()
	bar.style.Height = style.Length{Value: 35, Unit: "px"}
	container.AddChild(tb)
	container.AddChild(bar)

	h := intrinsicContentHeight(container)
	if h < 184 { // 150 + 35 (padding 0)
		t.Fatalf("intrinsicContentHeight=%.0f, want ≥185 (sum of stacked children)", h)
	}
}

// TestFlexRowChildHeightPinned: flex row container's auto height follows the
// tallest child's explicit height (not line gap).
func TestFlexRowChildHeightPinned(t *testing.T) {
	container := mkBlock()
	container.style.Display = style.DisplayFlex // row
	container.style.Width = style.Length{Value: 300, Unit: "px"}
	child := mkBlock()
	child.style.Height = style.Length{Value: 120, Unit: "px"}
	container.AddChild(child)

	root := mkBlock()
	root.AddChild(container)
	state := Layout(root, 300, 300)
	g := state.GeometryForBox(container)
	if g.BorderBoxHeight() < 119 {
		t.Fatalf("flex row container height=%.0f, want ≈120 (tallest child)", g.BorderBoxHeight())
	}
}

// TestFlexColumnAutoItemUsesMeasuredHeight: a column-flex item whose main size
// is content-determined (basis:auto, grow:0, shrink:1) must take its MEASURED
// content height from a pre-layout, not the intrinsic estimate.
//
// Why the estimate is not enough: intrinsicContentHeight accumulates each
// child's height + its own margin-bottom, so it misses (a) collapsed adjacent
// margins (max(mb_prev, mt_next), not mb_prev) and (b) the taller line boxes of
// inline replaced controls (input[type=range] etc.). Under-measuring the item
// leaves the box shorter than its own content — with overflow:auto that wrongly
// reserves a scrollbar stealing 15px from the content width.
//
// Regression (cmd/mediastudio/web/index.html, 1280x860): div.ctrl-body intrinsic
// content 471.5 vs measured 497.5 (估低 26px) → box 487.5 < content 497.5 →
// scrollbar reserved → .field width 189 instead of the browser's 204.
func TestFlexColumnAutoItemUsesMeasuredHeight(t *testing.T) {
	container := mkFlex()
	container.style.FlexDirection = "column"
	container.style.Width = style.Length{Value: 300, Unit: "px"}
	container.style.Height = style.Length{Value: 400, Unit: "px"}

	item := mkBlock()
	item.style.OverflowY = style.OverflowAuto
	item.style.Width = style.Length{Value: 300, Unit: "px"}

	first := mkBlockWH(0, 50)
	first.style.MarginBottom = style.Length{Value: 8, Unit: "px"}
	second := mkBlockWH(0, 50)
	// margin-top:20 与上一个块的 margin-bottom:8 折叠 → 实际间距 20。
	second.style.MarginTop = style.Length{Value: 20, Unit: "px"}
	item.AddChild(first)
	item.AddChild(second)

	container.AddChild(item)
	root := mkBlock()
	root.AddChild(container)
	state := Layout(root, 300, 400)

	// 内容实测高 = 50 + max(8,20) + 50 = 120；intrinsic 估算只有 108（漏折叠）。
	if est := intrinsicContentHeight(item); est >= 119 {
		t.Logf("intrinsicContentHeight=%.1f 未偏低——估算算法若已改进，本用例可放宽", est)
	}
	g := state.GeometryForBox(item)
	if g.BorderBoxHeight() < 119 {
		t.Fatalf("flex column auto item height=%.1f, want ≥120 (measured content, incl. collapsed margin); the estimate alone gives 108",
			g.BorderBoxHeight())
	}
}
