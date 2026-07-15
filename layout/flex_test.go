package layout

import (
	"testing"
)

// TestFlex_RowEqualGrow verifies that two flex items with flex-grow:1 share the
// container's main-axis space equally.
func TestFlex_RowEqualGrow(t *testing.T) {
	root := mkFlex()
	a := mkBlock()
	a.Style.FlexGrow = 1
	b := mkBlock()
	b.Style.FlexGrow = 1
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// Each item gets 400px of main size.
	assertApprox(t, "a.Width", a.Rect.Width, 400)
	assertApprox(t, "b.Width", b.Rect.Width, 400)
	// Items are laid out left-to-right.
	assertApprox(t, "a.X", a.Rect.X, 0)
	assertApprox(t, "b.X", b.Rect.X, 400)
	// Both on the same row.
	assertApprox(t, "a.Y", a.Rect.Y, 0)
	assertApprox(t, "b.Y", b.Rect.Y, 0)
}

// TestFlex_JustifyCenter verifies that justify-content: centre centres the items
// along the main axis.
func TestFlex_JustifyCenter(t *testing.T) {
	root := mkFlex()
	root.Style.JustifyContent = "center"
	a := mkBlockWH(100, 0)
	b := mkBlockWH(100, 0)
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// Total used width = 200, free = 600, centred -> start at 300.
	assertApprox(t, "a.X", a.Rect.X, 300)
	assertApprox(t, "b.X", b.Rect.X, 400)
}

// TestFlex_ColumnDirection verifies that flex-direction: column stacks items
// vertically.
func TestFlex_ColumnDirection(t *testing.T) {
	root := mkFlex()
	root.Style.FlexDirection = "column"
	a := mkBlockWH(0, 50)
	b := mkBlockWH(0, 30)
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// Items stack vertically: a at y=0, b at y=50.
	assertApprox(t, "a.Y", a.Rect.Y, 0)
	assertApprox(t, "b.Y", b.Rect.Y, 50)
}

// TestFlex_FlexGrowRatio verifies that flex-grow distributes space proportionally.
func TestFlex_FlexGrowRatio(t *testing.T) {
	root := mkFlex()
	a := mkBlock()
	a.Style.FlexGrow = 3
	b := mkBlock()
	b.Style.FlexGrow = 1
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// a gets 3/4 * 800 = 600, b gets 1/4 * 800 = 200.
	assertApprox(t, "a.Width", a.Rect.Width, 600)
	assertApprox(t, "b.Width", b.Rect.Width, 200)
}
