package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// --- Multi-column layout tests ---

func TestMultiColumn_SingleColumnFallsBack(t *testing.T) {
	root := mkColumnBlock(1)
	a := mkBlockWH(0, 50)
	b := mkBlockWH(0, 30)
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)
	_, ay, _, _ := rectOf(a, state)
	_, by, _, _ := rectOf(b, state)
	_, _, _, rh := rectOf(root, state)
	assertApprox(t, "a.Y", ay, 0)
	assertApprox(t, "b.Y", by, 50)
	assertApprox(t, "root.Height", rh, 80)
}

func TestMultiColumn_TwoColumns(t *testing.T) {
	root := mkColumnBlock(2)
	a := mkBlockWH(0, 100)
	b := mkBlockWH(0, 100)
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)
	_, _, aw, _ := rectOf(a, state)
	if aw < 380 || aw > 400 {
		t.Errorf("a.Width = %g, want ~392", aw)
	}
}

func TestMultiColumn_ColumnInfoStored(t *testing.T) {
	root := mkColumnBlock(3)
	root.style.Width = style.Length{Value: 800, Unit: "px"}
	root.style.Height = style.Length{Value: 600, Unit: "px"}
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
	widthCol.style.ColumnWidth = style.Length{Value: 200, Unit: "px"}
	if !HasColumns(widthCol) {
		t.Error("HasColumns should be true for column-width: 200px")
	}
}
