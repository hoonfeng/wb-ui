// Grid layout test — mimics the Vue app's CSS grid structure.
package layout

import (
	"testing"

	"wb-ui/engine/style"
)

// TestGrid_VueApp verifies that the GridFormattingContext correctly positions
// children according to grid-template-columns/rows and grid-column/grid-row
// placement, matching the Vue frontend's .app-root grid.
//
// CSS (from App.vue):
//
//	.app-root {
//	  display: grid;
//	  grid-template-columns: 48px auto 1fr auto;
//	  grid-template-rows: 30px 1fr 22px;
//	}
//	.titlebar      { grid-column: 1 / -1; grid-row: 1; }
//	.activity-bar  { grid-column: 1; grid-row: 2; }
//	.sidebar       { grid-column: 2; grid-row: 2; }
//	.main-area     { grid-column: 3; grid-row: 2; }
//	.right-panel   { grid-column: 4; grid-row: 2; }
//	.status-bar    { grid-column: 1 / -1; grid-row: 3; }
//
// Expected with viewport 1280x800:
//
//	col tracks: [48px, auto(empty→0), 1fr(→1232), auto(empty→0)]
//	row tracks: [30px, 1fr(→748), 22px]
//	col positions: [0, 48, 48, 1280, 1280]
//	row positions: [0, 30, 778, 800]
//
//	titlebar:     (0,0)    1280x30   ✓  spans all cols, row 1
//	activity-bar: (0,30)   48x748    ✓  col 1, row 2
//	sidebar:      (48,30)  0x748     ✓  col 2(auto=0), row 2
//	main-area:    (48,30)  1232x748  ✓  col 3(1fr), row 2
//	right-panel:  (1280,30) 0x748    ✓  col 4(auto=0), row 2
//	status-bar:   (0,778)  1280x22   ✓  spans all cols, row 3
func TestGrid_VueApp(t *testing.T) {
	// Create grid container with the same grid template as .app-root.
	root := mkVueGrid()
	// Set an explicit height so 1fr rows can distribute remaining space.
	root.style.Height = style.Length{Value: 800, Unit: "px"}

	titlebar := mkVueGridChild("titlebar", 1, -1, 1, 1)
	activityBar := mkVueGridChild("activity-bar", 1, 2, 2, 3)
	sidebar := mkVueGridChild("sidebar", 2, 3, 2, 3)
	mainArea := mkVueGridChild("main-area", 3, 4, 2, 3)
	rightPanel := mkVueGridChild("right-panel", 4, 5, 2, 3)
	statusBar := mkVueGridChild("status-bar", 1, -1, 3, 4)

	root.AddChild(titlebar)
	root.AddChild(activityBar)
	root.AddChild(sidebar)
	root.AddChild(mainArea)
	root.AddChild(rightPanel)
	root.AddChild(statusBar)

	state := Layout(root, 1280, 800)

	// Debug titlebar geometry.
	tg := state.GeometryForBox(titlebar)
	t.Logf("titlebar: contentW=%.0f borderW=%.0f left=%.0f top=%.0f",
		tg.ContentWidth(), tg.BorderBoxWidth(), tg.Left(), tg.Top())

	// ═══ titlebar: grid-column: 1 / -1 (spans all 4 cols), grid-row: 1 ═══
	tx, ty, tw, th := rectOf(titlebar, state)
	assertApprox(t, "titlebar.X", tx, 0)
	assertApprox(t, "titlebar.Y", ty, 0)
	assertApprox(t, "titlebar.Width", tw, 1280) // span all 4 cols
	assertApprox(t, "titlebar.Height", th, 30)

	// ═══ activity-bar: grid-column: 1, grid-row: 2 ═══
	ax, ay, aw, ah := rectOf(activityBar, state)
	assertApprox(t, "activity-bar.X", ax, 0)
	assertApprox(t, "activity-bar.Y", ay, 30)
	assertApprox(t, "activity-bar.Width", aw, 48)

	// ═══ sidebar: grid-column: 2, grid-row: 2 ═══
	sx, sy, sw, sh := rectOf(sidebar, state)
	assertApprox(t, "sidebar.Y", sy, 30)
	assertApprox(t, "sidebar.X", sx, 48) // after col 1 (48px)

	// ═══ main-area: grid-column: 3, grid-row: 2 ═══
	mx, my, mw, mh := rectOf(mainArea, state)
	assertApprox(t, "main-area.Y", my, 30)
	assertApprox(t, "main-area.X", mx, 48) // col 2 is auto=0, main-area starts at 48

	// ═══ right-panel: grid-column: 4, grid-row: 2 ═══
	rx, ry, rw, rh := rectOf(rightPanel, state)
	assertApprox(t, "right-panel.Y", ry, 30)
	assertApprox(t, "right-panel.X", rx, 48+mw) // after col 2 (0) + col 3 (main-area width)

	// ═══ status-bar: grid-column: 1 / -1, grid-row: 3 ═══
	stx, sty, stw, sth := rectOf(statusBar, state)
	// status-bar Y = row3 start = row1(30) + row2(748) = 778
	assertApprox(t, "status-bar.Y", sty, 778)
	assertApprox(t, "status-bar.X", stx, 0)
	assertApprox(t, "status-bar.Width", stw, 1280) // span all 4 cols
	assertApprox(t, "status-bar.Height", sth, 22)

	// ═══ container height should match total ═══
	_, _, _, rh = rectOf(root, state)
	assertApprox(t, "root.Height", rh, 800)

	t.Logf("titlebar: (%.0f,%.0f) %.0fx%.0f", tx, ty, tw, th)
	t.Logf("activity-bar: (%.0f,%.0f) %.0fx%.0f", ax, ay, aw, ah)
	t.Logf("sidebar: (%.0f,%.0f) %.0fx%.0f", sx, sy, sw, sh)
	t.Logf("main-area: (%.0f,%.0f) %.0fx%.0f", mx, my, mw, mh)
	t.Logf("right-panel: (%.0f,%.0f) %.0fx%.0f", rx, ry, rw, rh)
	t.Logf("status-bar: (%.0f,%.0f) %.0fx%.0f", stx, sty, stw, sth)
	t.Logf("root: (%.0f,%.0f)", 0.0, rh)
}

// mkVueGrid creates a grid container matching the .app-root CSS.
func mkVueGrid() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayGrid
	cs.GridTemplateColumns = "48px auto 1fr auto"
	cs.GridTemplateRows = "30px 1fr 22px"
	cs.BorderTopWidth = style.Length{}
	cs.BorderRightWidth = style.Length{}
	cs.BorderBottomWidth = style.Length{}
	cs.BorderLeftWidth = style.Length{}
	cs.PaddingTop = style.Length{}
	cs.PaddingRight = style.Length{}
	cs.PaddingBottom = style.Length{}
	cs.PaddingLeft = style.Length{}
	cs.BoxSizing = "border-box"
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

// mkVueGridChild creates a grid item with explicit grid-column/grid-row placement.
func mkVueGridChild(name string, colStart, colEnd, rowStart, rowEnd int) *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	cs.GridColumnStart = gridLineStr(colStart)
	cs.GridColumnEnd = gridLineStr(colEnd)
	cs.GridRowStart = gridLineStr(rowStart)
	cs.GridRowEnd = gridLineStr(rowEnd)
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

func gridLineStr(v int) string {
	if v == -1 {
		return "-1"
	}
	b := make([]byte, 0, 4)
	if v < 0 {
		b = append(b, '-')
		v = -v
	}
	if v >= 100 {
		b = append(b, byte('0'+v/100))
		v %= 100
	}
	if v >= 10 {
		b = append(b, byte('0'+v/10))
		v %= 10
	}
	b = append(b, byte('0'+v))
	return string(b)
}
