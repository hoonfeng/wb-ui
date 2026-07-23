// Translation of: Source/WebCore/layout/formattingContexts/table/TableFormattingContext.cpp
// Simplified table layout.

package layout

import (
	"math"
	"strconv"

	"wb-ui/style"
)

type TableFormattingContext struct{}

func (c *TableFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)

	cw := g.ContentWidth()


	// Collect rows from child boxes.
	type tableRow struct {
		box    *ElementBox
		cells  []*ElementBox
		height float64
	}
	var rows []*tableRow

	for _, child := range box.Children() {
		if rowBox, ok := child.(*ElementBox); ok && rowBox.IsVisible() {
			row := &tableRow{box: rowBox}
			for _, cell := range rowBox.Children() {
				if cellBox, ok := cell.(*ElementBox); ok && cellBox.IsVisible() {
					row.cells = append(row.cells, cellBox)
				}
			}
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 { return }

	// Compute column widths.
	maxCols := 0
	for _, row := range rows {
		if len(row.cells) > maxCols { maxCols = len(row.cells) }
	}
	if maxCols < 1 { maxCols = 1 }

	colWidth := cw / float64(maxCols)

	// Position rows and cells.
	y := g.ContentBoxTop()
	for _, row := range rows {
		rg := state.GeometryForBox(row.box)
		rowH := 0.0

		x := g.ContentBoxLeft()
		for ci, cell := range row.cells {
			cg := state.GeometryForBox(cell)
			cg.SetTopLeft(x, y)
			cg.SetContentWidth(colWidth - cg.MarginStart() - cg.MarginEnd())

			// Layout cell content.
			cellCtx := contextFor(cell)
			cellCtx.Layout(cell, state)

			cellH := cg.BorderBoxHeight()
			if cellH > rowH { rowH = cellH }
			x += colWidth

			// Parse colspan from HTML attribute.
			_ = ci
			if cell.Element() != nil {
				colspanStr := cell.Element().GetAttribute("colspan")
				if colspanStr != "" {
					if span, err := strconv.Atoi(colspanStr); err == nil && span > 1 {
						x += colWidth * float64(span-1)
					}
				}
			}
		}

		rg.SetTopLeft(g.ContentBoxLeft(), y)
		rg.SetContentWidth(cw)
		rg.SetContentHeight(rowH)
		y += rowH
	}

	g.SetContentHeight(math.Max(g.ContentHeight(), y-g.ContentBoxTop()))
}

var _ = style.DisplayTable
