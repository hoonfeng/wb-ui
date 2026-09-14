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
