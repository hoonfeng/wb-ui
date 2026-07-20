package layout

import (
	"testing"

	"wb-ui/style"
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
// TestGrid_NamedAreas verifies that items assigned to named areas via grid-area
// are correctly placed by grid-template-areas.
func TestGrid_NamedAreas(t *testing.T) {
	root := mkGrid()
	root.Style.GridTemplateColumns = "100px 200px 150px"
	root.Style.GridTemplateRows = "80px 80px"
	root.Style.GridTemplateAreas = `"a b c" "a d e"`

	a := mkBlock()
	a.Style.Properties["grid-area"] = "a"
	b := mkBlock()
	b.Style.Properties["grid-area"] = "b"
	c := mkBlock()
	c.Style.Properties["grid-area"] = "c"
	d := mkBlock()
	d.Style.Properties["grid-area"] = "d"
	e := mkBlock()
	e.Style.Properties["grid-area"] = "e"

	root.AddChild(a)
	root.AddChild(b)
	root.AddChild(c)
	root.AddChild(d)
	root.AddChild(e)

	Layout(root, 800, 600)

	// a spans rows 0-2, col 0 (2 rows x 80px each = 160px height for the row span)
	assertApprox(t, "a.X", a.Rect.X, 0)
	assertApprox(t, "a.Y", a.Rect.Y, 0)
	assertApprox(t, "a.Width", a.Rect.Width, 100)

	// b is at row 0, col 1
	assertApprox(t, "b.X", b.Rect.X, 100)
	assertApprox(t, "b.Y", b.Rect.Y, 0)
	assertApprox(t, "b.Width", b.Rect.Width, 200)

	// c is at row 0, col 2
	assertApprox(t, "c.X", c.Rect.X, 300)
	assertApprox(t, "c.Y", c.Rect.Y, 0)
	assertApprox(t, "c.Width", c.Rect.Width, 150)

	// d is at row 1, col 1
	assertApprox(t, "d.X", d.Rect.X, 100)
	assertApprox(t, "d.Y", d.Rect.Y, 80)

	// e is at row 1, col 2
	assertApprox(t, "e.X", e.Rect.X, 300)
	assertApprox(t, "e.Y", e.Rect.Y, 80)
}

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

// TestGrid_ComplexNamedAreas reproduces the B6 test page layout structure to verify
// that named grid areas with multiple rows/columns correctly position all items.
func TestGrid_ComplexNamedAreas(t *testing.T) {
	root := mkGrid()
	// 3 columns: 220px + 1fr + 280px = at least 500px content width
	root.Style.GridTemplateColumns = "220px 1fr 280px"
	// 3 rows: 48px + 1fr + 32px
	root.Style.GridTemplateRows = "48px 1fr 32px"
	root.Style.GridTemplateAreas = `"header header header" "sidebar main panel" "status status status"`
	root.Style.Width = style.Length{Value: 800, Unit: "px"}

	header := mkBlock()
	header.Style.Properties["grid-area"] = "header"
	sidebar := mkBlock()
	sidebar.Style.Properties["grid-area"] = "sidebar"
	main := mkBlock()
	main.Style.Properties["grid-area"] = "main"
	panel := mkBlock()
	panel.Style.Properties["grid-area"] = "panel"
	status := mkBlock()
	status.Style.Properties["grid-area"] = "status"

	root.AddChild(header)
	root.AddChild(sidebar)
	root.AddChild(main)
	root.AddChild(panel)
	root.AddChild(status)

	Layout(root, 800, 600)

	t.Logf("Grid container: rect=(%.0f,%.0f,%.0f,%.0f)", root.Rect.X, root.Rect.Y, root.Rect.Width, root.Rect.Height)
	t.Logf("Header: rect=(%.0f,%.0f,%.0f,%.0f)", header.Rect.X, header.Rect.Y, header.Rect.Width, header.Rect.Height)
	t.Logf("Sidebar: rect=(%.0f,%.0f,%.0f,%.0f)", sidebar.Rect.X, sidebar.Rect.Y, sidebar.Rect.Width, sidebar.Rect.Height)
	t.Logf("Main: rect=(%.0f,%.0f,%.0f,%.0f)", main.Rect.X, main.Rect.Y, main.Rect.Width, main.Rect.Height)
	t.Logf("Panel: rect=(%.0f,%.0f,%.0f,%.0f)", panel.Rect.X, panel.Rect.Y, panel.Rect.Width, panel.Rect.Height)
	t.Logf("Status: rect=(%.0f,%.0f,%.0f,%.0f)", status.Rect.X, status.Rect.Y, status.Rect.Width, status.Rect.Height)

	// Header spans all 3 columns in row 0
	assertApprox(t, "header.X", header.Rect.X, 0)
	assertApprox(t, "header.Y", header.Rect.Y, 0)
	assertApprox(t, "header.Width", header.Rect.Width, 800)

	// Sidebar is at col 0, row 1 (220px wide)
	assertApprox(t, "sidebar.X", sidebar.Rect.X, 0)
	assertApprox(t, "sidebar.Y", sidebar.Rect.Y, 48)
	assertApprox(t, "sidebar.Width", sidebar.Rect.Width, 220)

	// Main is at col 1, row 1 (1fr = 800-220-280 = 300px)
	assertApprox(t, "main.X", main.Rect.X, 220)
	assertApprox(t, "main.Y", main.Rect.Y, 48)

	// Panel is at col 2, row 1 (280px wide)
	assertApprox(t, "panel.X", panel.Rect.X, 520)
	assertApprox(t, "panel.Y", panel.Rect.Y, 48)
	assertApprox(t, "panel.Width", panel.Rect.Width, 280)

	// Status spans all 3 columns in row 2
	assertApprox(t, "status.X", status.Rect.X, 0)
	assertApprox(t, "status.Width", status.Rect.Width, 800)
}
