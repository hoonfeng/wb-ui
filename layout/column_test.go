package layout

import (
	"testing"

	"wb-ui/style"
)

// --- Multi-column layout tests ---

func TestMultiColumn_SingleColumnFallsBack(t *testing.T) {
	root := mkColumnBlock(1)
	a := mkBlockWH(0, 50)
	b := mkBlockWH(0, 30)
	root.AddChild(a)
	root.AddChild(b)
	Layout(root, 800, 600)
	assertApprox(t, "a.Y", a.Rect.Y, 0)
	assertApprox(t, "b.Y", b.Rect.Y, 50)
	assertApprox(t, "root.Height", root.Rect.Height, 80)
}

func TestMultiColumn_TwoColumns(t *testing.T) {
	root := mkColumnBlock(2)
	a := mkBlockWH(0, 100)
	b := mkBlockWH(0, 100)
	root.AddChild(a)
	root.AddChild(b)
	Layout(root, 800, 600)
	if a.Rect.Width < 380 || a.Rect.Width > 400 {
		t.Errorf("a.Width = %g, want ~392", a.Rect.Width)
	}
}

func TestMultiColumn_ColumnInfoStored(t *testing.T) {
	root := mkColumnBlock(3)
	root.Style.Width = style.Length{Value: 800, Unit: "px"}
	root.Style.Height = style.Length{Value: 600, Unit: "px"}
	Layout(root, 800, 600)
	info := GetColumnInfo(root)
	if info == nil {
		t.Fatal("GetColumnInfo returned nil, want non-nil")
	}
	if info.columnCount != 3 {
		t.Errorf("columnCount = %d, want 3", info.columnCount)
	}
	if info.columnGap != 16 {
		t.Errorf("columnGap = %g, want 16", info.columnGap)
	}
	if len(info.rulePositions) != 2 {
		t.Errorf("rulePositions len = %d, want 2", len(info.rulePositions))
	}
}

func TestMultiColumn_HasColumns(t *testing.T) {
	noCol := mkBlock()
	if HasColumns(noCol) {
		t.Error("HasColumns should be false for plain block")
	}
	col := mkColumnBlock(2)
	if !HasColumns(col) {
		t.Error("HasColumns should be true for column-count: 2")
	}
	widthCol := mkBlock()
	widthCol.Style.ColumnWidth = style.Length{Value: 200, Unit: "px"}
	if !HasColumns(widthCol) {
		t.Error("HasColumns should be true for column-width: 200px")
	}
}
