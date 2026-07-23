// Translation of: Source/WebCore/layout/formattingContexts/grid/GridFormattingContext.cpp
// Simplified grid formatting context.

package layout

import (
	"math"
	"sort"

	"wb-ui/style"
)

type GridFormattingContext struct {
	FormattingContextBase
}

type gridItem struct {
	box    *ElementBox
	row    int
	col    int
	rowSpan int
	colSpan int
}

func (c *GridFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)
	cw := g.ContentWidth()
	ch := g.ContentHeight()

	// Parse grid-template-columns / rows (simplified: count auto tracks).
	colCount := parseTrackCount(cs.GridTemplateColumns, cw)
	rowCount := parseTrackCount(cs.GridTemplateRows, ch)
	if colCount < 1 { colCount = 1 }
	if rowCount < 1 { rowCount = 1 }

	// Place items into grid cells.
	var items []*gridItem
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok && child.IsInFlow() && child.IsVisible() {
			items = append(items, &gridItem{box: childEb, row: len(items) / colCount, col: len(items) % colCount, rowSpan: 1, colSpan: 1})
		}
	}

	if len(items) == 0 { return }

	// Compute track sizes (equal distribution).
	colW := cw / float64(colCount)
	rowH := ch / float64(rowCount)
	if ch <= 0 { rowH = 100 }

	// Sort by row then column.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].row != items[j].row { return items[i].row < items[j].row }
		return items[i].col < items[j].col
	})

	// Position items.
	for _, it := range items {
		itg := state.GeometryForBox(it.box)
		x := g.ContentBoxLeft() + float64(it.col)*colW
		y := g.ContentBoxTop() + float64(it.row)*rowH
		itg.SetTopLeft(x, y)
		itg.SetContentWidth(colW - itg.MarginStart() - itg.MarginEnd())
		itg.SetContentHeight(rowH - itg.MarginBefore() - itg.MarginAfter())

		// Content layout.
		ctx := contextFor(it.box, state)
		ctx.Layout(it.box, state)
	}

	// Update container height to content.
	maxRow := 0
	for _, it := range items { if it.row+it.rowSpan > maxRow { maxRow = it.row + it.rowSpan } }
	g.SetContentHeight(math.Max(g.ContentHeight(), float64(maxRow)*rowH))
}

func parseTrackCount(tracks string, available float64) int {
	if tracks == "" || tracks == "none" || tracks == "auto" {
		// Default to 1 when no explicit tracks defined.
		return 1
	}
	// Simplified: count comma-separated track values.
	count := 1
	for _, ch := range tracks {
		if ch == ' ' { count++ }
	}
	if count > 10 { count = 10 }
	return count
}

var _ = style.DisplayGrid
