package layout

import (
	"testing"

	"wb-ui/style"
)

// mkGridAreas builds a grid container with the given template columns and
// named areas, plus children placed by grid-area.
func mkGridAreas(cols, rows, areas string, children []struct {
	area   string
	w, h   float64 // content size hint
	cls    string
}) *ElementBox {
	b := mkBlock()
	b.style.Display = style.DisplayGrid
	b.style.GridTemplateColumns = cols
	b.style.GridTemplateRows = rows
	b.style.GridTemplateAreas = areas
	for _, c := range children {
		child := mkBlock()
		child.style.SetProperty("grid-area", c.area)
		if c.w > 0 {
			child.style.Width = style.Length{Value: c.w, Unit: "px"}
		}
		if c.h > 0 {
			child.style.Height = style.Length{Value: c.h, Unit: "px"}
		}
		b.AddChild(child)
	}
	return b
}

// TestGridTemplateAreasPlacement: grid-template-areas + grid-area: name must
// place children into the correct cells with correct spans.
func TestGridTemplateAreasPlacement(t *testing.T) {
	box := mkGridAreas(
		"48px 280px 1fr 200px", "30px 1fr 22px",
		`"title title title title" "actbar sidebar main right" "status status status status"`,
		[]struct {
			area  string
			w, h  float64
			cls   string
		}{
			{"title", 0, 0, "titlebar"},
			{"actbar", 0, 0, "actbar"},
			{"sidebar", 0, 0, "sidebar"},
			{"main", 0, 0, "main"},
			{"right", 0, 0, "right"},
			{"status", 0, 0, "status"},
		},
	)
	root := mkBlock()
	root.AddChild(box)
	state := Layout(root, 1280, 800)

	rect := func(b *ElementBox) (x, y, w, h float64) {
		g := state.GeometryForBox(b)
		return g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
	}
	children := box.Children()
	checks := []struct {
		name                       string
		x, y, w, h                 float64
	}{
		{"title", 0, 0, 1280, 30},
		{"actbar", 0, 30, 48, 748},
		{"sidebar", 48, 30, 280, 748},
		{"main", 328, 30, 752, 748},
		{"right", 1080, 30, 200, 748},
		{"status", 0, 778, 1280, 22},
	}
	for i, c := range checks {
		eb, _ := children[i].(*ElementBox)
		x, y, w, h := rect(eb)
		if x != c.x || y != c.y || w != c.w || h != c.h {
			t.Errorf("%s: got xy=(%.0f,%.0f) wh=(%.0f,%.0f), want xy=(%.0f,%.0f) wh=(%.0f,%.0f)",
				c.name, x, y, w, h, c.x, c.y, c.w, c.h)
		}
	}
}

// TestGridFrShrink: a 1fr column must NOT balloon to its content width when a
// sibling auto column already consumes the space — the grid must stay within
// the container.
func TestGridFrShrink(t *testing.T) {
	// 48px + auto + 1fr + 200px with an auto right column whose content is
	// 500px wide → 1fr gets 1280-48-500-200 = 532, NOT content width.
	box := mkBlock()
	box.style.Display = style.DisplayGrid
	box.style.GridTemplateColumns = "48px 500px 1fr 200px"
	box.style.GridTemplateRows = "30px 1fr 22px"

	child := mkBlock()
	child.style.GridColumnStart = "3"
	child.style.SetProperty("grid-area", "main")
	// Give the 1fr child a huge intrinsic text width via a text run.
	tb := &InlineTextBox{text: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", style: child.style}
	child.AddChild(tb)
	box.AddChild(child)

	root := mkBlock()
	root.AddChild(box)
	state := Layout(root, 1280, 800)
	g := state.GeometryForBox(child)
	// 1fr = 1280 - 48 - 500 - 200 - 0 gaps = 532. If fr ballooned to text
	// width (~50*7px) the grid would overflow.
	if g.BorderBoxWidth() > 600 {
		t.Fatalf("1fr column width=%.0f, want ≈532 (fr must not balloon to content)", g.BorderBoxWidth())
	}
}

// TestFlexGridItemStretchPinned: a flex container used as a grid item must
// keep the parent's row height (stretch) instead of inflating to its content
// height.
func TestFlexGridItemStretchPinned(t *testing.T) {
	box := mkBlock()
	box.style.Display = style.DisplayGrid
	box.style.GridTemplateColumns = "100px"
	box.style.GridTemplateRows = "1fr"

	item := mkBlock()
	item.style.Display = style.DisplayFlex
	item.style.FlexDirection = "column"
	// Tall content inside the flex item.
	inner := mkBlock()
	inner.style.Height = style.Length{Value: 500, Unit: "px"}
	item.AddChild(inner)
	box.AddChild(item)

	root := mkBlock()
	root.AddChild(box)
	state := Layout(root, 200, 300)
	g := state.GeometryForBox(item)
	// Row height = 300 (1fr of the 300px-tall container) — the flex item must
	// not grow to 500 to fit its child.
	if g.BorderBoxHeight() > 310 {
		t.Fatalf("flex grid-item height=%.0f, want ≤300 (stretch pins it)", g.BorderBoxHeight())
	}
}

// TestGridAutoFlowBasic: display:grid without template still auto-places
// children in a single column (matching WebKit's default single-column grid).
func TestGridAutoFlowBasic(t *testing.T) {
	box := mkBlock()
	box.style.Display = style.DisplayGrid
	box.style.Width = style.Length{Value: 300, Unit: "px"}
	for i := 0; i < 3; i++ {
		child := mkBlockWH(50, 50)
		box.AddChild(child)
	}
	root := mkBlock()
	root.AddChild(box)
	state := Layout(root, 320, 400)
	// Each child in its own implicit row: y = i*50.
	children := box.Children()
	for i := 0; i < 3; i++ {
		eb, _ := children[i].(*ElementBox)
		g := state.GeometryForBox(eb)
		if got := g.Top(); got != float64(i*50) {
			t.Fatalf("child %d top=%.0f, want %d", i, got, i*50)
		}
	}
}
