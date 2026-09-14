package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestTable_FixedLayout verifies that a fixed-layout table honours the declared column
// widths from the first row and positions cells accordingly.
func TestTable_FixedLayout(t *testing.T) {
	root := mkTable()
	setProp(root, "table-layout", "fixed")

	row := mkTableRow()
	cell1 := mkTableCell()
	cell1.Style().Width = style.Length{Value: 100, Unit: "px"}
	cell2 := mkTableCell()
	cell2.Style().Width = style.Length{Value: 200, Unit: "px"}
	row.AddChild(cell1)
	row.AddChild(cell2)
	root.AddChild(row)

	state := Layout(root, 300, 600)

	// Column widths match the declared 100px / 200px (table content width = 300).
	c1x, _, c1w, _ := rectOf(cell1, state)
	c2x, _, c2w, _ := rectOf(cell2, state)
	assertApprox(t, "cell1.Width", c1w, 100)
	assertApprox(t, "cell1.X", c1x, 0)
	assertApprox(t, "cell2.Width", c2w, 200)
	assertApprox(t, "cell2.X", c2x, 100)
}

// TestTable_AutoLayout verifies that an auto-layout table distributes the table width
// evenly across columns when no explicit widths are declared.
func TestTable_AutoLayout(t *testing.T) {
	root := mkTable()

	row := mkTableRow()
	cell1 := mkTableCell()
	cell2 := mkTableCell()
	row.AddChild(cell1)
	row.AddChild(cell2)
	root.AddChild(row)

	state := Layout(root, 800, 600)

	// Two columns share 800px equally -> 400px each.
	_, _, c1w, _ := rectOf(cell1, state)
	_, _, c2w, _ := rectOf(cell2, state)
	assertApprox(t, "cell1.Width", c1w, 400)
	assertApprox(t, "cell2.Width", c2w, 400)
	// cell2.X should be 400 (cell1 width)
}

// TestTable_CellHeight verifies that a cell with block content grows the row height.
func TestTable_CellHeight(t *testing.T) {
	root := mkTable()

	row := mkTableRow()
	cell := mkTableCell()
	inner := mkBlockWH(0, 40)
	cell.AddChild(inner)
	row.AddChild(cell)
	root.AddChild(row)

	state := Layout(root, 800, 600)

	// The row height should be at least the cell content height (40px).
	_, _, _, rh := rectOf(root, state)
	if rh < 40 {
		t.Errorf("table height = %g, want >= 40", rh)
	}
}
