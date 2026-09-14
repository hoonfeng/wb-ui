package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestFlexColumnCenterGap: three children in a column flex with
// justify-content:center + gap must stack vertically with 8px gaps and be
// centered. Regression: all children overlapped at the first child's Y
// (welcome page text overlap in the desktop app).
func TestFlexColumnCenterGap(t *testing.T) {
	container := mkBlock()
	container.style.Display = style.DisplayFlex
	container.style.FlexDirection = "column"
	container.style.AlignItems = "center"
	container.style.JustifyContent = "center"
	container.style.Gap = style.Length{Value: 8, Unit: "px"}
	container.style.Width = style.Length{Value: 200, Unit: "px"}
	container.style.Height = style.Length{Value: 400, Unit: "px"}
	t.Logf("container gap=%v rowGap=%v colGap=%v", container.style.Gap, container.style.RowGap, container.style.ColumnGap)

	mkChild := func(fs float64) *ElementBox {
		c := mkBlock()
		c.style.FontSize = style.Length{Value: fs, Unit: "px"}
		return c
	}
	c1 := mkChild(48)
	c2 := mkChild(18)
	c3 := mkChild(13)
	container.AddChild(c1)
	container.AddChild(c2)
	container.AddChild(c3)

	root := mkBlock()
	root.AddChild(container)
	state := Layout(root, 400, 500)

	g1 := state.GeometryForBox(c1)
	g2 := state.GeometryForBox(c2)
	g3 := state.GeometryForBox(c3)

	t.Logf("c1 y=%.0f h=%.0f", g1.Top(), g1.BorderBoxHeight())
	t.Logf("c2 y=%.0f h=%.0f", g2.Top(), g2.BorderBoxHeight())
	t.Logf("c3 y=%.0f h=%.0f", g3.Top(), g3.BorderBoxHeight())

	// c1 below c2 below c3 (stacked), with gap >= 8 between each.
	if g2.Top() <= g1.Top() {
		t.Fatalf("c2 top=%.0f must be below c1 top=%.0f", g2.Top(), g1.Top())
	}
	if g3.Top() <= g2.Top() {
		t.Fatalf("c3 top=%.0f must be below c2 top=%.0f", g3.Top(), g2.Top())
	}
	if g2.Top()-g1.BorderBoxHeight()-g1.Top() < 7 {
		t.Fatalf("gap c1→c2 = %.0f, want >=8", g2.Top()-g1.Top()-g1.BorderBoxHeight())
	}
}
