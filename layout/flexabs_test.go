package layout

import (
	"testing"

	"wb-ui/style"
)

// TestFlexAbsoluteChild: an absolutely-positioned child of a flex container
// (e.g. .cache-ring-label inside a flex .cache-ring-wrap) must be laid out
// with real size and flex-aligned static position. Regression: flex dropped
// out-of-flow children entirely — the label stayed 0x0 at (0,0).
func TestFlexAbsoluteChild(t *testing.T) {
	wrap := mkBlock()
	wrap.style.Display = style.DisplayFlex
	wrap.style.Position = style.PositionRelative
	wrap.style.AlignItems = "center"
	wrap.style.JustifyContent = "center"
	wrap.style.Width = style.Length{Value: 96, Unit: "px"}
	wrap.style.Height = style.Length{Value: 96, Unit: "px"}

	label := mkBlock()
	label.style.Position = style.PositionAbsolute
	label.style.Display = style.DisplayFlex
	label.style.FlexDirection = "column"
	label.style.AlignItems = "center"

	// Text child so the label has intrinsic height.
	pct := mkBlock()
	pct.style.FontSize = style.Length{Value: 18, Unit: "px"}
	label.AddChild(pct)

	// In-flow child so the flex container isn't empty.
	inflow := mkBlock()
	inflow.style.Width = style.Length{Value: 20, Unit: "px"}
	inflow.style.Height = style.Length{Value: 20, Unit: "px"}

	wrap.AddChild(label)
	wrap.AddChild(inflow)

	root := mkBlock()
	root.AddChild(wrap)
	state := Layout(root, 400, 300)

	g := state.GeometryForBox(label)
	t.Logf("label xy=(%.0f,%.0f) wh=(%.0f,%.0f)", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
	if g.BorderBoxWidth() <= 0 {
		t.Fatalf("label has zero width, want >0 (absolute child of flex not laid out)")
	}
	// Static position should be flex-aligned (centered in the 96x96 wrap).
	if g.Top() < 20 {
		t.Fatalf("label top=%.0f, want ~38+ (flex-align center in 96px wrap)", g.Top())
	}
}
