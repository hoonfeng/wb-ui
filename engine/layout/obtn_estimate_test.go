package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestFlexObtnEstimates: reproduces the real .obtn button (display:flex,
// padding 4px 8px, border 1px, gap 3px, 12px icon + 2-char label) inside a
// flex .ibb-btns (gap 2px). The intrinsic estimate must not be smaller than
// the laid-out width, otherwise post-layout adjustment pushes following
// flex items (e.g. the send button) out of the input box.
func TestFlexObtnEstimates(t *testing.T) {
	ibb := mkFlex()
	ibb.style.Gap = style.Length{Value: 2, Unit: "px"}
	ibb.style.ColumnGap = style.Length{Value: 2, Unit: "px"}
	ibb.style.Width = style.Length{Value: 200, Unit: "px"}

	labels := []string{"审核", "工具配置", "折叠", "提交", "自主"}
	var obtn []*ElementBox
	for _, lb := range labels {
		o := mkFlex()
		o.style.PaddingTop = style.Length{Value: 4, Unit: "px"}
		o.style.PaddingBottom = style.Length{Value: 4, Unit: "px"}
		o.style.PaddingLeft = style.Length{Value: 8, Unit: "px"}
		o.style.PaddingRight = style.Length{Value: 8, Unit: "px"}
		o.style.BorderTopWidth = style.Length{Value: 1, Unit: "px"}
		o.style.BorderBottomWidth = style.Length{Value: 1, Unit: "px"}
		o.style.BorderLeftWidth = style.Length{Value: 1, Unit: "px"}
		o.style.BorderRightWidth = style.Length{Value: 1, Unit: "px"}
		o.style.Gap = style.Length{Value: 3, Unit: "px"}
		o.style.ColumnGap = style.Length{Value: 3, Unit: "px"}
		o.style.FontSize = style.Length{Value: 11, Unit: "px"}
		o.AddChild(mkBlockWH(12, 12)) // icon
		o.AddChild(mkTextRun(lb))
		ibb.AddChild(o)
		obtn = append(obtn, o)
	}

	root := mkBlock()
	root.AddChild(ibb)
	state := Layout(root, 400, 300)

	prevRight := -1.0
	for i, o := range obtn {
		g := state.GeometryForBox(o)
		got := 0.0
		if prevRight >= 0 {
			got = g.Left() - prevRight
		}
		t.Logf("obtn[%d] %q x=%.0f w=%.0f gap_to_prev=%.0f", i, labels[i], g.Left(), g.BorderBoxWidth(), got)
		prevRight = g.Left() + g.BorderBoxWidth()
	}
}
