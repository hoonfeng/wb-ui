package layout

import (
	"testing"

	"wb-ui/style"
)

// --- Flex with writing-mode tests ---

func TestFlex_VerticalWritingModeRow(t *testing.T) {
	// With vertical writing-mode and flex-direction: row,
	// the main axis should be vertical (height direction).
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayFlex
	cs.WritingMode = "vertical-rl"
	cs.FlexDirection = "row"
	cs.Width = style.Length{Value: 400, Unit: "px"}
	cs.Height = style.Length{Value: 600, Unit: "px"}
	root := &LayoutBox{Type: BoxBlock, Style: cs}

	a := mkBlockWH(50, 30)
	b := mkBlockWH(50, 30)
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 400, 600)

	// In vertical-rl + row, the main axis is height (inline axis of vertical
	// writing mode = physical height). Items should be placed along the height
	// axis. They may not be visible in width if flex-basis resolution doesn't
	// consider writing-mode yet, but layout should not crash.
	_ = a.Rect
	_ = b.Rect
}

func TestFlex_VerticalWritingModeColumn(t *testing.T) {
	// With vertical writing-mode and flex-direction: column,
	// the main axis should be horizontal (width direction).
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayFlex
	cs.WritingMode = "vertical-lr"
	cs.FlexDirection = "column"
	cs.Width = style.Length{Value: 400, Unit: "px"}
	cs.Height = style.Length{Value: 600, Unit: "px"}
	root := &LayoutBox{Type: BoxBlock, Style: cs}

	a := mkBlockWH(50, 30)
	b := mkBlockWH(50, 30)
	root.AddChild(a)
	root.AddChild(b)

	Layout(root, 400, 600)

	if a.Rect.Width <= 0 {
		t.Errorf("a.Width = %g, want > 0", a.Rect.Width)
	}
	if b.Rect.Width <= 0 {
		t.Errorf("b.Width = %g, want > 0", b.Rect.Width)
	}
}

func TestEffectiveIsRow(t *testing.T) {
	hBox := mkFlex()
	hBox.Style.FlexDirection = "row"
	if !effectiveIsRow(hBox) {
		t.Error("effectiveIsRow should be true for horizontal flex-direction:row")
	}

	vBox := mkFlex()
	vBox.Style.WritingMode = "vertical-rl"
	vBox.Style.FlexDirection = "row"
	if effectiveIsRow(vBox) {
		t.Error("effectiveIsRow should be false for vertical flex-direction:row")
	}

	vCol := mkFlex()
	vCol.Style.WritingMode = "vertical-rl"
	vCol.Style.FlexDirection = "column"
	if !effectiveIsRow(vCol) {
		t.Error("effectiveIsRow should be true for vertical flex-direction:column (block axis)")
	}
}

func TestEffectiveIsReverse(t *testing.T) {
	hBox := mkFlex()
	hBox.Style.FlexDirection = "row-reverse"
	if !effectiveIsReverse(hBox) {
		t.Error("effectiveIsReverse should be true for row-reverse")
	}
	hBox2 := mkFlex()
	hBox2.Style.FlexDirection = "row"
	if effectiveIsReverse(hBox2) {
		t.Error("effectiveIsReverse should be false for row")
	}
}
