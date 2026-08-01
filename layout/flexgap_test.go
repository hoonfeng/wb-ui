package layout

import (
	"testing"

	"wb-ui/style"
)

// TestFlexGapBetweenItems: CSS gap between flex items must space them
// apart. Regression probe: .ibb-btns (gap:2px) rendered obtn buttons
// adjacent with no gap (sometimes 1px overlap).
func TestFlexGapBetweenItems(t *testing.T) {
	f := mkFlex()
	f.style.Gap = style.Length{Value: 2, Unit: "px"}
	f.style.ColumnGap = style.Length{Value: 2, Unit: "px"}
	f.style.Width = style.Length{Value: 200, Unit: "px"}

	var items []*ElementBox
	for _, w := range []float64{38, 52, 38} {
		b := mkBlockWH(w, 24)
		b.style.FlexShrink = 0
		f.AddChild(b)
		items = append(items, b)
	}
	root := mkBlock()
	root.AddChild(f)
	state := Layout(root, 400, 300)

	prevRight := -1.0
	for i, b := range items {
		g := state.GeometryForBox(b)
		t.Logf("item[%d] x=%.0f w=%.0f", i, g.Left(), g.BorderBoxWidth())
		if prevRight >= 0 {
			got := g.Left() - prevRight
			if got != 2 {
				t.Errorf("gap between item[%d] and item[%d] = %.0f, want 2", i-1, i, got)
			}
		}
		prevRight = g.Left() + g.BorderBoxWidth()
	}
}
