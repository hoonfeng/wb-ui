package layout

import (
	"testing"

	"wb-ui/style"
)

// TestTable_FixedLayout verifies that a fixed-layout table honours the declared column
// widths from the first row and positions cells accordingly.
func TestTable_FixedLayout(t *testing.T) {
	root := mkTable()
	setProp(root, "table-layout", "fixed")

	row := mkTableRow()
	cell1 := mkTableCell()
	cell1.Style.Width = style.Length{Value: 100, Unit: "px"}
	cell2 := mkTableCell()
	cell2.Style.Width = style.Length{Value: 200, Unit: "px"}
	row.AddChild(cell1)
	row.AddChild(cell2)
	root.AddChild(row)

	Layout(root, 300, 600)

	// Column widths match the declared 100px / 200px (table content width = 300).
	assertApprox(t, "cell1.Width", cell1.Rect.Width, 100)
	assertApprox(t, "cell1.X", cell1.Rect.X, 0)
	assertApprox(t, "cell2.Width", cell2.Rect.Width, 200)
	assertApprox(t, "cell2.X", cell2.Rect.X, 100)
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

	Layout(root, 800, 600)

	// Two columns share 800px equally -> 400px each.
	assertApprox(t, "cell1.Width", cell1.Rect.Width, 400)
	assertApprox(t, "cell2.Width", cell2.Rect.Width, 400)
	assertApprox(t, "cell2.X", cell2.Rect.X, 400)
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

	Layout(root, 800, 600)

	// The row height should be at least the cell content height (40px).
	if root.Rect.Height < 40 {
		t.Errorf("table height = %g, want >= 40", root.Rect.Height)
	}
}
