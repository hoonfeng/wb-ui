package layout

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/style"
)

// TestLayoutCache_DirtyAndClean tests the LayoutBox dirty tracking.
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
	s := style.NewComputedStyle()
	s.SetProperty("break-before", "page")
	box := NewLayoutBox(BoxBlock, s)
	box.Rect = LayoutRect{X: 0, Y: 0, Width: 100, Height: 50}

	root := mkBlock()
	root.AddChild(box)
	Layout(root, 800, 600)

	// The box should be positioned at the start of the next page
	// since break-before:page forces a page break before the element.
	if box.Rect.Y != 600 {
		t.Errorf("break-before: page at y=%g, want 600 (next page)", box.Rect.Y)
	}
}

// TestBreakAfter tests that break-after:page advances the next sibling.
func TestBreakAfter(t *testing.T) {
	firstS := style.NewComputedStyle()
	firstS.SetProperty("break-after", "page")
	first := NewLayoutBox(BoxBlock, firstS)
	first.Rect = LayoutRect{X: 0, Y: 0, Width: 100, Height: 50}

	second := mkBlockWH(100, 50)

	root := mkBlock()
	root.AddChild(first)
	root.AddChild(second)
	Layout(root, 800, 600)

	// Second child should be on the next page (y >= 600).
	if second.Rect.Y < 600 {
		t.Errorf("second child y=%g, want >= 600 (next page)", second.Rect.Y)
	} else if second.Rect.Y > 660 {
		t.Errorf("second child y=%g, want ~650 (after first child)", second.Rect.Y)
	}
}

// TestLayoutCache_Reuse verifies that after marking clean, re-layout works.
func TestLayoutCache_Reuse(t *testing.T) {
	root := mkBlockWH(800, 50)
	child := mkBlockWH(100, 50)
	root.AddChild(child)
	Layout(root, 800, 600)

	if child.Rect.Y != 0 {
		t.Fatalf("child y=%g, want 0", child.Rect.Y)
	}

	// Mark clean and re-layout — should get same result.
	root.MarkClean()
	Layout(root, 800, 600)
	if child.Rect.Y != 0 {
		t.Errorf("after re-layout child y=%g, want 0", child.Rect.Y)
	}
}

// TestVerticalWritingMode_Layout tests vertical-rl block layout basics.
func TestVerticalWritingMode_Layout(t *testing.T) {
	root := mkVerticalBlock("vertical-rl")
	a := mkBlockWH(100, 50)
	b := mkBlockWH(80, 40)
	root.AddChild(a)
	root.AddChild(b)
	Layout(root, 800, 600)

	if a.Rect.Width != 100 {
		t.Errorf("a.Width = %g, want 100", a.Rect.Width)
	}
	if b.Rect.Width != 80 {
		t.Errorf("b.Width = %g, want 80", b.Rect.Width)
	}
}

// TestOrphansWidowsPlaceholder tests that orphans/widows properties
// are parsed without error (layout engine defers orphans/widows handling
// to the inline formatting context).
func TestOrphansWidowsPlaceholder(t *testing.T) {
	s := style.NewComputedStyle()
	s.SetProperty("orphans", "3")
	s.SetProperty("widows", "2")
	box := NewLayoutBox(BoxBlock, s)
	_ = box
	// Properties are stored; no error
}

// TestPagedMargin tests that margin properties work during layout.
func TestPagedMargin(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	s := style.NewComputedStyle()
	s.SetProperty("margin", "10px")
	box := NewLayoutBox(BoxBlock, s)
	box.Element = el
	box.Rect = LayoutRect{X: 0, Y: 0, Width: 100, Height: 50}

	root := mkBlock()
	root.AddChild(box)
	Layout(root, 800, 600)
	// Margin should not prevent layout from completing
}
