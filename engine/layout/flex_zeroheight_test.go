package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestFlexZeroHeightWithContent: a flex-column child with an explicit
// height:0px (e.g. CodeMirror 6 gutter spacer: style.height="0px") must NOT
// be inflated to its text content's line height. flex-basis:auto takes the
// main-size property's definite value (0px) as the flex basis (CSS §7.2.3);
// the automatic minimum (min-height:auto) is min(content, specified) = 0.
func TestFlexZeroHeightWithContent(t *testing.T) {
	col := mkFlex()
	col.style.FlexDirection = "column"
	col.style.Height = style.Length{Value: 100, Unit: "px"}

	spacer := mkBlock()
	spacer.style.Height = style.Length{Value: 0, Unit: "px"} // definite 0
	spacer.style.Visibility = "hidden"
	spacer.style.PaddingLeft = style.Length{Value: 5, Unit: "px"}
	spacer.style.PaddingRight = style.Length{Value: 3, Unit: "px"}
	spacer.style.MinWidth = style.Length{Value: 20, Unit: "px"}
	spacer.AddChild(mkTextRun("99"))

	line1 := mkBlockWH(0, 18)
	col.AddChild(spacer)
	col.AddChild(line1)

	root := mkBlock()
	root.AddChild(col)
	state := Layout(root, 400, 400)

	_, _, _, h := rectOf(spacer, state)
	_, _, _, h1 := rectOf(line1, state)
	assertApprox(t, "spacer height (expect 0, not content line height)", h, 0)
	if h1 != 18 {
		t.Errorf("line1 height = %g, want 18", h1)
	}
	_, sy, _, _ := rectOf(spacer, state)
	_, y1, _, _ := rectOf(line1, state)
	if y1-sy > 0.5 {
		t.Errorf("line1 top = %g, spacer top = %g: line1 should sit directly below a 0-height spacer (no gap)", y1, sy)
	}
}

// TestFlexHeightZeroNoBasis: same as above but via inline style only
// (no min-width), verifying the flex-basis:auto + height:0 path generally.
func TestFlexHeightZeroNoBasis(t *testing.T) {
	col := mkFlex()
	col.style.FlexDirection = "column"
	col.style.Height = style.Length{Value: 100, Unit: "px"}

	spacer := mkBlock()
	spacer.style.Height = style.Length{Value: 0, Unit: "px"}
	spacer.AddChild(mkTextRun("99"))

	col.AddChild(spacer)
	root := mkBlock()
	root.AddChild(col)
	state := Layout(root, 400, 400)

	_, _, _, h := rectOf(spacer, state)
	assertApprox(t, "spacer height", h, 0)
}
