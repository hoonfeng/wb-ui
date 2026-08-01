package layout

import (
	"testing"

	"wb-ui/style"
)

// TestFlexColumnGrow: a flex:1 child must grow to fill the leftover space in
// a fixed-height column flex container (chat-messages in the chat-area:
// container 711px, chat-input-area 196px, chat-messages should be 515px).
func TestFlexColumnGrow(t *testing.T) {
	// Nested: rp-body (row flex, fixed 711 height) stretches chat-area
	// (flex:1 width) to full height; chat-area is a column flex.
	rpBody := mkFlex()
	rpBody.style.Width = style.Length{Value: 851, Unit: "px"}
	rpBody.style.Height = style.Length{Value: 711, Unit: "px"}

	area := mkFlex()
	area.style.FlexDirection = "column"
	area.style.FlexGrow = 1
	area.style.MinWidth = style.Length{Unit: "auto"}
	rpBody.AddChild(area)

	msg := mkBlock()
	msg.style.FlexGrow = 1
	msg.style.FlexBasis = style.Length{Value: 0, Unit: "%"}
	msg.style.FlexShrink = 1
	msg.style.MinHeight = style.Length{Value: 0, Unit: "px"}
	msg.style.OverflowY = style.OverflowAuto
	msg.style.PaddingTop = style.Length{Value: 8, Unit: "px"}
	msg.style.PaddingBottom = style.Length{Value: 8, Unit: "px"}
	msg.style.PaddingLeft = style.Length{Value: 12, Unit: "px"}
	msg.style.PaddingRight = style.Length{Value: 12, Unit: "px"}
	empty := mkBlockWH(0, 217)
	msg.AddChild(empty)

	input := mkBlockWH(0, 196)
	input.style.FlexShrink = 0
	input.style.PaddingTop = style.Length{Value: 8, Unit: "px"}
	input.style.PaddingBottom = style.Length{Value: 8, Unit: "px"}
	input.style.PaddingLeft = style.Length{Value: 8, Unit: "px"}
	input.style.PaddingRight = style.Length{Value: 8, Unit: "px"}

	area.AddChild(msg)
	area.AddChild(input)

	root := mkBlock()
	root.AddChild(rpBody)
	state := Layout(root, 800, 800)

	ag := state.GeometryForBox(area)
	mg := state.GeometryForBox(msg)
	ig := state.GeometryForBox(input)
	t.Logf("area h=%.0f chat-messages h=%.0f chat-input h=%.0f", ag.BorderBoxHeight(), mg.BorderBoxHeight(), ig.BorderBoxHeight())
	if mg.BorderBoxHeight() < 500 {
		t.Errorf("flex:1 child height = %.0f, want >= 500 (leftover space)", mg.BorderBoxHeight())
	}
}
