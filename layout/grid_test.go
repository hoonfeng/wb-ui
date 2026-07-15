package layout

import (
	"testing"
)

// TestGrid_FixedTracks verifies that grid-template-columns with px values produces
// columns of the declared widths and places items in their columns.
func TestGrid_FixedTracks(t *testing.T) {
	root := mkGrid()
	root.Style.GridTemplateColumns = "100px 200px"

	a := mkBlock()
	a.Style.GridColumnStart = "1"
	a.Style.GridColumnEnd = "2"
	a.Style.GridRowStart = "1"
	a.Style.GridRowEnd = "2"

	b := mkBlock()
	b.Style.GridColumnStart = "2"
	b.Style.GridColumnEnd = "3"
	b.Style.GridRowStart = "1"
	b.Style.GridRowEnd = "2"

	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	assertApprox(t, "a.Width", a.Rect.Width, 100)
	assertApprox(t, "a.X", a.Rect.X, 0)
	assertApprox(t, "b.Width", b.Rect.Width, 200)
	assertApprox(t, "b.X", b.Rect.X, 100)
}

// TestGrid_FrDistribution verifies that 1fr tracks share the free space equally.
func TestGrid_FrDistribution(t *testing.T) {
	root := mkGrid()
	root.Style.GridTemplateColumns = "1fr 1fr"

	a := mkBlock()
	a.Style.GridColumnStart = "1"
	a.Style.GridColumnEnd = "2"
	a.Style.GridRowStart = "1"
	a.Style.GridRowEnd = "2"

	b := mkBlock()
	b.Style.GridColumnStart = "2"
	b.Style.GridColumnEnd = "3"
	b.Style.GridRowStart = "1"
	b.Style.GridRowEnd = "2"

	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// Each 1fr column gets 400px.
	assertApprox(t, "a.Width", a.Rect.Width, 400)
	assertApprox(t, "b.Width", b.Rect.Width, 400)
	assertApprox(t, "b.X", b.Rect.X, 400)
}

// TestGrid_PercentageTrack verifies that percentage tracks resolve against the
// container content width.
func TestGrid_PercentageTrack(t *testing.T) {
	root := mkGrid()
	root.Style.GridTemplateColumns = "50% 50%"

	a := mkBlock()
	a.Style.GridColumnStart = "1"
	a.Style.GridColumnEnd = "2"
	a.Style.GridRowStart = "1"
	a.Style.GridRowEnd = "2"

	b := mkBlock()
	b.Style.GridColumnStart = "2"
	b.Style.GridColumnEnd = "3"
	b.Style.GridRowStart = "1"
	b.Style.GridRowEnd = "2"

	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 800, 600)

	// 50% of 800 = 400 each.
	assertApprox(t, "a.Width", a.Rect.Width, 400)
	assertApprox(t, "b.Width", b.Rect.Width, 400)
}
