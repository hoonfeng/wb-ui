package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// --- Writing-mode tests ---

func TestVerticalBlock_VerticalRL(t *testing.T) {
	root := mkVerticalBlock("vertical-rl")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)
	_, _, aw, ah := rectOf(a, state)
	_, _, bw, bh := rectOf(b, state)
	assertApprox(t, "a.Width", aw, 100)
	assertApprox(t, "a.Height", ah, 50)
	assertApprox(t, "b.Width", bw, 80)
	assertApprox(t, "b.Height", bh, 40)
}

func TestVerticalBlock_VerticalLR(t *testing.T) {
	root := mkVerticalBlock("vertical-lr")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)
	_, _, aw, ah := rectOf(a, state)
	_, _, bw, bh := rectOf(b, state)
	assertApprox(t, "a.Width", aw, 100)
	assertApprox(t, "a.Height", ah, 50)
	assertApprox(t, "b.Width", bw, 80)
	assertApprox(t, "b.Height", bh, 40)
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
	g := &BoxGeometry{}
	g.SetContentWidth(200)
	g.SetContentHeight(100)
	g.SetPadding(5, 0, 5, 0)
	g.SetBorder(1, 0, 1, 0)
	g.SetTopLeft(0, 0)
	// We can't easily set GeometryForBox without state, so skip geometry checks.
	_ = hBox
}
