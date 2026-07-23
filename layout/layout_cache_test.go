package layout

import (
	"testing"

	"wb-ui/style"
)

// TestLayoutCache_DirtyAndClean tests the ElementBox dirty tracking.
func TestLayoutCache_DirtyAndClean(t *testing.T) {
	root := mkBlock()
	if root.IsDirty() {
		t.Fatal("new box should not be dirty")
	}

	root.MarkDirty()
	if !root.IsDirty() {
		t.Fatal("expected dirty after MarkDirty")
	}

	root.MarkClean()
	if root.IsDirty() {
		t.Fatal("expected clean after MarkClean")
	}

	// MarkDirty on a child should mark ancestors too.
	child := mkBlock()
	root.AddChild(child)
	root.MarkClean()
	child.MarkDirty()
	if !root.IsDirty() {
		t.Fatal("expected root dirty after child.MarkDirty")
	}
}

// TestBreakBefore tests that break-before:page advances the cursor.
func TestBreakBefore(t *testing.T) {
	box := mkBlockWH(100, 50)
	box.style.SetProperty("break-before", "page")

	root := mkBlock()
	root.AddChild(box)
	state := Layout(root, 800, 600)

	_, by, _, _ := rectOf(box, state)
	if by != 600 {
		t.Errorf("break-before: page at y=%g, want 600 (next page)", by)
	}
}

// TestBreakAfter tests that break-after:page advances the next sibling.
func TestBreakAfter(t *testing.T) {
	first := mkBlockWH(100, 50)
	first.style.SetProperty("break-after", "page")

	second := mkBlockWH(100, 50)

	root := mkBlock()
	root.AddChild(first)
	root.AddChild(second)
	state := Layout(root, 800, 600)

	_, sy, _, _ := rectOf(second, state)
	if sy < 600 {
		t.Errorf("second child y=%g, want >= 600 (next page)", sy)
	} else if sy > 660 {
		t.Errorf("second child y=%g, want ~650 (after first child)", sy)
	}
}

// TestLayoutCache_Reuse verifies that after marking clean, re-layout works.
func TestLayoutCache_Reuse(t *testing.T) {
	root := mkBlockWH(800, 50)
	child := mkBlockWH(100, 50)
	root.AddChild(child)
	state := Layout(root, 800, 600)

	_, cy, _, _ := rectOf(child, state)
	if cy != 0 {
		t.Fatalf("child y=%g, want 0", cy)
	}

	// Mark clean and re-layout — should get same result.
	root.MarkClean()
	state2 := Layout(root, 800, 600)
	_, cy2, _, _ := rectOf(child, state2)
	if cy2 != 0 {
		t.Errorf("after re-layout child y=%g, want 0", cy2)
	}
}

// TestVerticalWritingMode_Layout tests vertical-rl block layout basics.
func TestVerticalWritingMode_Layout(t *testing.T) {
	root := mkVerticalBlock("vertical-rl")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	state := Layout(root, 800, 600)

	_, _, aw, _ := rectOf(a, state)
	_, _, bw, _ := rectOf(b, state)
	if aw != 100 {
		t.Errorf("a.Width = %g, want 100", aw)
	}
	if bw != 80 {
		t.Errorf("b.Width = %g, want 80", bw)
	}
}

// TestOrphansWidowsPlaceholder tests that orphans/widows properties
// are parsed without error.
func TestOrphansWidowsPlaceholder(t *testing.T) {
	s := style.NewComputedStyle()
	s.SetProperty("orphans", "3")
	s.SetProperty("widows", "2")
	_ = s
	// Properties are stored; no error
}

// TestPagedMargin tests that margin properties work during layout.
func TestPagedMargin(t *testing.T) {
	box := mkBlockWH(100, 50)
	box.style.SetProperty("margin", "10px")

	root := mkBlock()
	root.AddChild(box)
	_ = Layout(root, 800, 600)
	// Margin should not prevent layout from completing
}
