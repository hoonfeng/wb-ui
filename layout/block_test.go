package layout

import (
	"testing"

	"wb-ui/style"
)

// TestBlock_SimpleStack verifies that two block children stack vertically and the
// container's auto height is the sum of the children's heights.
func TestBlock_SimpleStack(t *testing.T) {
	root := mkBlock()
	a := mkBlockWH(0, 30)
	b := mkBlockWH(0, 50)
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	_, ay, _, ah := rectOf(a, state)
	_, by, _, bh := rectOf(b, state)
	_, _, _, rh := rectOf(root, state)
	assertApprox(t, "a.Y", ay, 0)
	assertApprox(t, "a.Height", ah, 30)
	assertApprox(t, "b.Y", by, 30)
	assertApprox(t, "b.Height", bh, 50)
	assertApprox(t, "root.Height", rh, 80)
}

// TestBlock_FillWidth verifies that an auto-width block child fills the container
// content width.
func TestBlock_FillWidth(t *testing.T) {
	root := mkBlock()
	root.style.PaddingLeft = style.Length{Value: 20, Unit: "px"}
	root.style.PaddingRight = style.Length{Value: 20, Unit: "px"}
	child := mkBlockWH(0, 10)
	root.AddChild(child)

	state := Layout(root, 800, 600)
	cx, _, cw, _ := rectOf(child, state)

	// content width = 800 - 40 = 760
	assertApprox(t, "child.Width", cw, 760)
	assertApprox(t, "child.X", cx, 20)
}

// TestBlock_SpecifiedWidth verifies that a child with an explicit width is sized to
// that width and left-aligned within the container.
func TestBlock_SpecifiedWidth(t *testing.T) {
	root := mkBlock()
	child := mkBlockWH(200, 40)
	root.AddChild(child)

	state := Layout(root, 800, 600)
	cx, cy, cw, _ := rectOf(child, state)
	_, _, _, rh := rectOf(root, state)

	assertApprox(t, "child.Width", cw, 200)
	assertApprox(t, "child.X", cx, 0)
	assertApprox(t, "child.Y", cy, 0)
	assertApprox(t, "root.Height", rh, 40)
}

// TestBlock_MarginCollapse verifies that adjacent sibling vertical margins collapse to
// the larger of the two.
func TestBlock_MarginCollapse(t *testing.T) {
	root := mkBlock()
	a := mkBlockWH(0, 20)
	a.style.MarginBottom = style.Length{Value: 30, Unit: "px"}
	b := mkBlockWH(0, 20)
	b.style.MarginTop = style.Length{Value: 10, Unit: "px"}
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	_, by, _, _ := rectOf(b, state)
	_, _, _, rh := rectOf(root, state)

	// a occupies [0, 20), its bottom margin 30 collapses with b's top margin 10 -> 30.
	assertApprox(t, "b.Y", by, 20+30)
	// root height encloses b's bottom (20+30+20 = 70); b's bottom margin collapses
	// with root's bottom margin (auto height, no border/padding).
	assertApprox(t, "root.Height", rh, 70)
}

// TestBlock_BFCContainsFloats verifies that a flow-root container (which establishes
// a BFC) grows to enclose its floated children.
func TestBlock_BFCContainsFloats(t *testing.T) {
	root := mkBlock()
	root.style.Display = style.DisplayFlowRoot
	floated := mkBlockWH(100, 50)
	floated.style.Float = "left"
	root.AddChild(floated)

	state := Layout(root, 800, 600)

	_, _, _, rh := rectOf(root, state)
	fx, _, fw, _ := rectOf(floated, state)

	// The BFC root must enclose the float: height >= 50.
	if rh < 50 {
		t.Errorf("BFC root height = %g, want >= 50 (must contain floats)", rh)
	}
	assertApprox(t, "floated.Width", fw, 100)
	assertApprox(t, "floated.X", fx, 0)
}

// TestBlock_NestedBlocks verifies nested block layout: a child containing its own
// children is laid out recursively and the parent height reflects the full stack.
func TestBlock_NestedBlocks(t *testing.T) {
	root := mkBlock()
	middle := mkBlock()
	leaf1 := mkBlockWH(0, 15)
	leaf2 := mkBlockWH(0, 25)
	middle.AddChild(leaf1)
	middle.AddChild(leaf2)
	root.AddChild(middle)

	state := Layout(root, 800, 600)

	l1x, l1y, _, _ := rectOf(leaf1, state)
	_, l2y, _, _ := rectOf(leaf2, state)
	_, my, _, mh := rectOf(middle, state)
	_, _, _, rh := rectOf(root, state)

	_ = l1x // leaf1 X is not tested
	assertApprox(t, "leaf1.Y", l1y, 0)
	assertApprox(t, "leaf2.Y", l2y, 15)
	assertApprox(t, "middle.Height", mh, 40)
	assertApprox(t, "middle.Y", my, 0)
	assertApprox(t, "root.Height", rh, 40)
}

// TestBlock_PaddingAffectsContent verifies that padding on the container offsets the
// children and grows the container height.
func TestBlock_PaddingAffectsContent(t *testing.T) {
	root := mkBlock()
	root.style.PaddingTop = style.Length{Value: 10, Unit: "px"}
	root.style.PaddingBottom = style.Length{Value: 5, Unit: "px"}
	child := mkBlockWH(0, 20)
	root.AddChild(child)

	state := Layout(root, 800, 600)

	_, cy, _, _ := rectOf(child, state)
	_, _, _, rh := rectOf(root, state)

	// child starts after padding-top (10).
	assertApprox(t, "child.Y", cy, 10)
	// root height = padding-top + child + padding-bottom = 10 + 20 + 5 = 35.
	assertApprox(t, "root.Height", rh, 35)
}

// TestBlock_MinMaxWidth verifies that min-width/max-width constrain the
// used width of a block child.
func TestBlock_MinMaxWidth(t *testing.T) {
	// Subtest A: explicit width 100, min-width 200 → used width = 200.
	root := mkBlock()
	a := mkBlockWH(100, 20)
	a.style.MinWidth = style.Length{Value: 200, Unit: "px"}
	root.AddChild(a)
	state := Layout(root, 800, 600)
	_, _, aw, _ := rectOf(a, state)
	assertApprox(t, "min-width 200 overrides width 100", aw, 200)

	// Subtest B: explicit width 500, max-width 300 → used width = 300.
	root2 := mkBlock()
	b := mkBlockWH(500, 20)
	b.style.MaxWidth = style.Length{Value: 300, Unit: "px"}
	root2.AddChild(b)
	state2 := Layout(root2, 800, 600)
	_, _, bw, _ := rectOf(b, state2)
	assertApprox(t, "max-width 300 overrides width 500", bw, 300)
}

// TestBlock_MinMaxHeight verifies that min-height/max-height constrain the
// used height of a block child.
func TestBlock_MinMaxHeight(t *testing.T) {
	// Subtest A: explicit height 30, min-height 60 → used height = 60.
	root := mkBlock()
	a := mkBlockWH(100, 30)
	a.style.MinHeight = style.Length{Value: 60, Unit: "px"}
	root.AddChild(a)
	state := Layout(root, 800, 600)
	_, _, _, ah := rectOf(a, state)
	assertApprox(t, "min-height 60 clamps height 30", ah, 60)

	// Subtest B: explicit height 80, max-height 40 → used height = 40.
	root2 := mkBlock()
	b := mkBlockWH(100, 80)
	b.style.MaxHeight = style.Length{Value: 40, Unit: "px"}
	root2.AddChild(b)
	state2 := Layout(root2, 800, 600)
	_, _, _, bh := rectOf(b, state2)
	assertApprox(t, "max-height 40 clamps height 80", bh, 40)
}

// TestBlock_OverflowScroll verifies that a container with overflow:scroll
// establishes a BFC (block formatting context) and clips/floats correctly.
// In this simplified layout engine, overflow:scroll primarily affects
// BFC establishment rather than scrollbar sizing.
func TestBlock_OverflowScroll(t *testing.T) {
	root := mkBlock()
	root.style.OverflowX = style.OverflowScroll
	root.style.OverflowY = style.OverflowScroll
	root.style.Width = style.Length{Value: 200, Unit: "px"}
	root.style.Height = style.Length{Value: 100, Unit: "px"}

	// A floated child that would normally escape a non-BFC container.
	floated := mkBlockWH(150, 50)
	floated.style.Float = "left"
	root.AddChild(floated)

	state := Layout(root, 800, 600)

	fx, _, _, _ := rectOf(floated, state)
	_, _, _, rh := rectOf(root, state)

	assertApprox(t, "floated.X", fx, 0)
	// The root must enclose its float (height ≥ 50).
	if rh < 50 {
		t.Errorf("overflow:scroll root height = %g, want >= 50", rh)
	}
}
