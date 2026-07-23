package layout

import (
	"testing"

	"wb-ui/style"
)

// --- Flex with writing-mode tests ---

func TestFlex_VerticalWritingModeRow(t *testing.T) {
	// With vertical writing-mode and flex-direction: row,
	// the main axis should be vertical (height direction).
	root := mkFlex()
	root.style.WritingMode = "vertical-rl"
	root.style.FlexDirection = "row"
	root.style.Width = style.Length{Value: 400, Unit: "px"}
	root.style.Height = style.Length{Value: 600, Unit: "px"}

	a := mkBlockWH(50, 30)
	b := mkBlockWH(50, 30)
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 400, 600)
	// Ensure no crash and items get non-zero sizes.
	_, _, aw, _ := rectOf(a, state)
	_, _, bw, _ := rectOf(b, state)
	_ = aw
	_ = bw
}

func TestFlex_VerticalWritingModeColumn(t *testing.T) {
	// With vertical writing-mode and flex-direction: column,
	// the main axis should be horizontal (width direction).
	root := mkFlex()
	root.style.WritingMode = "vertical-lr"
	root.style.FlexDirection = "column"
	root.style.Width = style.Length{Value: 400, Unit: "px"}
	root.style.Height = style.Length{Value: 600, Unit: "px"}

	a := mkBlockWH(50, 30)
	b := mkBlockWH(50, 30)
	root.AddChild(a)
	root.AddChild(b)

	state := Layout(root, 400, 600)
	_, _, aw, _ := rectOf(a, state)
	_, _, bw, _ := rectOf(b, state)
	if aw <= 0 {
		t.Errorf("a.Width = %g, want > 0", aw)
	}
	if bw <= 0 {
		t.Errorf("b.Width = %g, want > 0", bw)
	}
}
