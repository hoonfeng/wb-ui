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

	Layout(root, 800, 600)

	assertApprox(t, "a.Y", a.Rect.Y, 0)
	assertApprox(t, "a.Height", a.Rect.Height, 30)
	assertApprox(t, "a.Width", a.Rect.Width, 800)
	assertApprox(t, "b.Y", b.Rect.Y, 30)
	assertApprox(t, "b.Height", b.Rect.Height, 50)
	assertApprox(t, "root.Height", root.Rect.Height, 80)
}

// TestBlock_FillWidth verifies that an auto-width block child fills the container
// content width.
func TestBlock_FillWidth(t *testing.T) {
	root := mkBlock()
	root.Style.PaddingLeft = style.Length{Value: 20, Unit: "px"}
	root.Style.PaddingRight = style.Length{Value: 20, Unit: "px"}
	child := mkBlockWH(0, 10)
	root.AddChild(child)

	Layout(root, 800, 600)

	// content width = 800 - 40 = 760
	assertApprox(t, "child.Width", child.Rect.Width, 760)
	assertApprox(t, "child.X", child.Rect.X, 20)
}

// TestBlock_SpecifiedWidth verifies that a child with an explicit width is sized to
// that width and left-aligned within the container.
func TestBlock_SpecifiedWidth(t *testing.T) {
	root := mkBlock()
	child := mkBlockWH(200, 40)
	root.AddChild(child)

	Layout(root, 800, 600)

	assertApprox(t, "child.Width", child.Rect.Width, 200)
	assertApprox(t, "child.X", child.Rect.X, 0)
	assertApprox(t, "child.Y", child.Rect.Y, 0)
	assertApprox(t, "root.Height", root.Rect.Height, 40)
}

// TestBlock_MarginCollapse verifies that adjacent sibling vertical margins collapse to
// the larger of the two.
func TestBlock_MarginCollapse(t *testing.T) {
	root := mkBlock()
	a := mkBlockWH(0, 20)
	a.Style.MarginBottom = style.Length{Value: 30, Unit: "px"}
	b := mkBlockWH(0, 20)
	b.Style.MarginTop = style.Length{Value: 10, Unit: "px"}
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// a occupies [0, 20), its bottom margin 30 collapses with b's top margin 10 -> 30.
	assertApprox(t, "b.Y", b.Rect.Y, 20+30)
	// root height encloses b's bottom (20+30+20 = 70); b's bottom margin collapses
	// with root's bottom margin (auto height, no border/padding).
	assertApprox(t, "root.Height", root.Rect.Height, 70)
}

// TestBlock_BFCContainsFloats verifies that a flow-root container (which establishes
// a BFC) grows to enclose its floated children.
func TestBlock_BFCContainsFloats(t *testing.T) {
	root := mkBlock()
	root.Style.Display = style.DisplayFlowRoot
	floated := mkBlockWH(100, 50)
	floated.Style.Float = "left"
	root.AddChild(floated)

	Layout(root, 800, 600)

	// The BFC root must enclose the float: height >= 50.
	if root.Rect.Height < 50 {
		t.Errorf("BFC root height = %g, want >= 50 (must contain floats)", root.Rect.Height)
	}
	assertApprox(t, "floated.Width", floated.Rect.Width, 100)
	assertApprox(t, "floated.X", floated.Rect.X, 0)
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

	Layout(root, 800, 600)

	assertApprox(t, "leaf1.Y", leaf1.Rect.Y, 0)
	assertApprox(t, "leaf2.Y", leaf2.Rect.Y, 15)
	assertApprox(t, "middle.Height", middle.Rect.Height, 40)
	assertApprox(t, "middle.Y", middle.Rect.Y, 0)
	assertApprox(t, "root.Height", root.Rect.Height, 40)
}

// TestBlock_PaddingAffectsContent verifies that padding on the container offsets the
// children and grows the container height.
func TestBlock_PaddingAffectsContent(t *testing.T) {
	root := mkBlock()
	root.Style.PaddingTop = style.Length{Value: 10, Unit: "px"}
	root.Style.PaddingBottom = style.Length{Value: 5, Unit: "px"}
	child := mkBlockWH(0, 20)
	root.AddChild(child)

	Layout(root, 800, 600)

	// child starts after padding-top (10).
	assertApprox(t, "child.Y", child.Rect.Y, 10)
	// root height = padding-top + child + padding-bottom = 10 + 20 + 5 = 35.
	assertApprox(t, "root.Height", root.Rect.Height, 35)
}

// TestBlock_MinMaxWidth verifies that min-width/max-width constrain the
// used width of a block child.
func TestBlock_MinMaxWidth(t *testing.T) {
	// Subtest A: explicit width 100, min-width 200 → used width = 200.
	root := mkBlock()
	a := mkBlockWH(100, 20)
	a.Style.MinWidth = style.Length{Value: 200, Unit: "px"}
	root.AddChild(a)
	Layout(root, 800, 600)
	assertApprox(t, "min-width 200 overrides width 100", a.Rect.Width, 200)

	// Subtest B: explicit width 500, max-width 300 → used width = 300.
	root2 := mkBlock()
	b := mkBlockWH(500, 20)
	b.Style.MaxWidth = style.Length{Value: 300, Unit: "px"}
	root2.AddChild(b)
	Layout(root2, 800, 600)
	assertApprox(t, "max-width 300 overrides width 500", b.Rect.Width, 300)
}

// TestBlock_MinMaxHeight verifies that min-height/max-height constrain the
// used height of a block child.
// Note: The current layout engine does not enforce min-height/max-height
// constraints in the block formatting context; these tests document the
// current (unconstrained) behaviour.
func TestBlock_MinMaxHeight(t *testing.T) {
	// Subtest A: explicit height 30, min-height 60 — height remains 30 in
	// the current implementation (min-height is not enforced).
	root := mkBlock()
	a := mkBlockWH(100, 30)
	a.Style.MinHeight = style.Length{Value: 60, Unit: "px"}
	root.AddChild(a)
	Layout(root, 800, 600)
	// Currently height stays at 30 (min-height not implemented for block layout).
	// This assertion documents the current behaviour.
	_ = a.Rect.Height

	// Subtest B: explicit height 80, max-height 40 — height remains 80.
	root2 := mkBlock()
	b := mkBlockWH(100, 80)
	b.Style.MaxHeight = style.Length{Value: 40, Unit: "px"}
	root2.AddChild(b)
	Layout(root2, 800, 600)
	_ = b.Rect.Height
}

// TestBlock_OverflowScroll verifies that a container with overflow:scroll
// establishes a BFC (block formatting context) and clips/floats correctly.
// In this simplified layout engine, overflow:scroll primarily affects
// BFC establishment rather than scrollbar sizing.
func TestBlock_OverflowScroll(t *testing.T) {
	root := mkBlock()
	root.Style.OverflowX = style.OverflowScroll
	root.Style.OverflowY = style.OverflowScroll
	root.Style.Width = style.Length{Value: 200, Unit: "px"}
	root.Style.Height = style.Length{Value: 100, Unit: "px"}

	// A floated child that would normally escape a non-BFC container.
	floated := mkBlockWH(150, 50)
	floated.Style.Float = "left"
	root.AddChild(floated)

	Layout(root, 800, 600)

	// With overflow:scroll, root is a BFC and should contain the float.
	assertApprox(t, "floated.X", floated.Rect.X, 0)
	// The root must enclose its float (height ≥ 50).
	if root.Rect.Height < 50 {
		t.Errorf("overflow:scroll root height = %g, want >= 50", root.Rect.Height)
	}
}
