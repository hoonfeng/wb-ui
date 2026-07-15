package layout

import (
	"testing"

	"wb-ui/style"
)

// --- Writing-mode tests ---

func TestVerticalBlock_VerticalRL(t *testing.T) {
	root := mkVerticalBlock("vertical-rl")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	Layout(root, 800, 600)
	assertApprox(t, "a.Width", a.Rect.Width, 100)
	assertApprox(t, "a.Height", a.Rect.Height, 50)
	assertApprox(t, "b.Width", b.Rect.Width, 80)
	assertApprox(t, "b.Height", b.Rect.Height, 40)
}

func TestVerticalBlock_VerticalLR(t *testing.T) {
	root := mkVerticalBlock("vertical-lr")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	Layout(root, 800, 600)
	assertApprox(t, "a.Width", a.Rect.Width, 100)
	assertApprox(t, "a.Height", a.Rect.Height, 50)
	assertApprox(t, "b.Width", b.Rect.Width, 80)
	assertApprox(t, "b.Height", b.Rect.Height, 40)
}

func TestIsVerticalWritingMode(t *testing.T) {
	h := style.NewComputedStyle()
	h.WritingMode = "horizontal-tb"
	if IsVerticalWritingMode(h) {
		t.Error("IsVerticalWritingMode should be false for horizontal-tb")
	}
	v := style.NewComputedStyle()
	v.WritingMode = "vertical-rl"
	if !IsVerticalWritingMode(v) {
		t.Error("IsVerticalWritingMode should be true for vertical-rl")
	}
	v2 := style.NewComputedStyle()
	v2.WritingMode = "vertical-lr"
	if !IsVerticalWritingMode(v2) {
		t.Error("IsVerticalWritingMode should be true for vertical-lr")
	}
	if IsVerticalWritingMode(nil) {
		t.Error("IsVerticalWritingMode should be false for nil")
	}
}

func TestLogicalHelpers(t *testing.T) {
	hBox := mkBlock()
	hBox.Rect.Width = 200
	hBox.Rect.Height = 100
	hBox.Rect.Padding = Edges{Top: 5, Bottom: 5}
	hBox.Rect.Border = Edges{Top: 1, Bottom: 1}
	if got := LogicalWidth(hBox); got != 200 {
		t.Errorf("LogicalWidth = %g, want 200", got)
	}
	if got := LogicalHeight(hBox); got != 88 {
		t.Errorf("LogicalHeight = %g, want 88", got)
	}
	vBox := mkVerticalBlock("vertical-rl")
	vBox.Rect.Width = 200
	vBox.Rect.Height = 100
	vBox.Rect.Padding = Edges{Top: 5, Bottom: 5}
	vBox.Rect.Border = Edges{Top: 1, Bottom: 1}
	if got := LogicalWidth(vBox); got != 88 {
		t.Errorf("vertical LogicalWidth = %g, want 88 (content height)", got)
	}
	if got := LogicalHeight(vBox); got != 200 {
		t.Errorf("vertical LogicalHeight = %g, want 200 (content width)", got)
	}
}
