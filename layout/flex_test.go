package layout

import (
	"testing"

	"wb-ui/style"
)

// TestFlex_RowEqualGrow verifies that two flex items with flex-grow:1 share the
// container's main-axis space equally.
func TestFlex_RowEqualGrow(t *testing.T) {
	root := mkFlex()
	a := mkBlock()
	a.Style().FlexGrow = 1
	b := mkBlock()
	b.Style().FlexGrow = 1
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	ax, _, aw, _ := rectOf(a, state)
	bx, _, bw, _ := rectOf(b, state)
	// Each item gets 400px of main size.
	assertApprox(t, "a.Width", aw, 400)
	assertApprox(t, "b.Width", bw, 400)
	// Items are laid out left-to-right.
	assertApprox(t, "a.X", ax, 0)
	assertApprox(t, "b.X", bx, 400)
	// Both on the same row.
	_, ay, _, _ := rectOf(a, state)
	_, by, _, _ := rectOf(b, state)
	assertApprox(t, "a.Y", ay, 0)
	assertApprox(t, "b.Y", by, 0)
}

// TestFlex_JustifyCenter verifies that justify-content: centre centres the items
// along the main axis.
func TestFlex_JustifyCenter(t *testing.T) {
	root := mkFlex()
	root.Style().JustifyContent = "center"
	a := mkBlockWH(100, 0)
	b := mkBlockWH(100, 0)
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	ax, _, _, _ := rectOf(a, state)
	bx, _, _, _ := rectOf(b, state)
	// Total used width = 200, free = 600, centred -> start at 300.
	assertApprox(t, "a.X", ax, 300)
	assertApprox(t, "b.X", bx, 400)
}

// TestFlex_ColumnDirection verifies that flex-direction: column stacks items
// vertically.
func TestFlex_ColumnDirection(t *testing.T) {
	root := mkFlex()
	root.Style().FlexDirection = "column"
	a := mkBlockWH(0, 50)
	b := mkBlockWH(0, 30)
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	_, ay, _, _ := rectOf(a, state)
	_, by, _, _ := rectOf(b, state)
	// Items stack vertically: a at y=0, b at y=50.
	assertApprox(t, "a.Y", ay, 0)
	assertApprox(t, "b.Y", by, 50)
}

// TestFlex_FlexGrowRatio verifies that flex-grow distributes space proportionally.
func TestFlex_FlexGrowRatio(t *testing.T) {
	root := mkFlex()
	a := mkBlock()
	a.Style().FlexGrow = 3
	b := mkBlock()
	b.Style().FlexGrow = 1
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 800, 600)

	_, _, aw, _ := rectOf(a, state)
	_, _, bw, _ := rectOf(b, state)
	// a gets 3/4 * 800 = 600, b gets 1/4 * 800 = 200.
	assertApprox(t, "a.Width", aw, 600)
	assertApprox(t, "b.Width", bw, 200)
}

// TestFlex_MinMax verifies that min-width/max-width constrain flex item sizing.
func TestFlex_MinMax(t *testing.T) {
	// Subtest A: flex-grow:1 with min-width 250 → used width ≥ 250.
	root := mkFlex()
	a := mkBlockWH(100, 20)
	a.Style().FlexGrow = 1
	a.Style().MinWidth = style.Length{Value: 250, Unit: "px"}
	b := mkBlock()
	b.Style().FlexGrow = 1
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)
	_, _, aw, _ := rectOf(a, state)
	if aw < 250 {
		t.Errorf("min-width 250 not honoured: a.Width = %g", aw)
	}

	// Subtest B: flex-grow:1 with max-width 200 → used width ≤ 200.
	root2 := mkFlex()
	c := mkBlockWH(500, 20)
	c.Style().FlexGrow = 1
	c.Style().MaxWidth = style.Length{Value: 200, Unit: "px"}
	d := mkBlock()
	d.Style().FlexGrow = 1
	root2.AddChild(c)
	root2.AddChild(d)
	state2 := Layout(root2, 800, 600)
	_, _, cw, _ := rectOf(c, state2)
	if cw > 200 {
		t.Errorf("max-width 200 not honoured: c.Width = %g", cw)
	}
}
