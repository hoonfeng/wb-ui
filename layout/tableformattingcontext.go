// Translation of: Source/WebCore/layout/formattingContexts/table/TableFormattingContext.cpp
// Simplified table layout.

package layout

import (
	"math"
	"strconv"

	"wb-ui/style"
)

type TableFormattingContext struct {
	FormattingContextBase
}

// collectRows flattens the table's child tree into a row list, handling
// thead/tbody/tfoot grouping elements.
func collectRows(box *ElementBox) []*ElementBox {
	var rows []*ElementBox
	for _, child := range box.Children() {
		eb, ok := child.(*ElementBox)
		if !ok || !eb.IsVisible() {
			continue
		}
		if eb.IsTableRow() {
			rows = append(rows, eb)
		} else if eb.IsTableSection() {
			for _, cc := range eb.Children() {
				if rowEb, ok := cc.(*ElementBox); ok && rowEb.IsVisible() && rowEb.IsTableRow() {
					rows = append(rows, rowEb)
				}
			}
		}
	}
	return rows
}

// collectCells returns all visible cell (table-cell) children of a row.
func collectCells(row *ElementBox) []*ElementBox {
	var cells []*ElementBox
	for _, child := range row.Children() {
		if cellEb, ok := child.(*ElementBox); ok && cellEb.IsVisible() && cellEb.IsTableCell() {
			cells = append(cells, cellEb)
		}
	}
	return cells
}

// parseColspan returns the column span for a cell from HTML attribute, default 1.
func parseColspan(cell *ElementBox) int {
	if cell.Element() == nil {
		return 1
	}
	s := cell.Element().GetAttribute("colspan")
	if s == "" {
		return 1
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return 1
}

var _ = style.DisplayTable

func (c *TableFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	cw := g.ContentWidth()
	if cw <= 0 {
		return
	}

	// ── Step 1: Collect all rows and cells ──
	type tableRow struct {
		box   *ElementBox
		cells []*ElementBox
	}

	rowBoxes := collectRows(box)
	if len(rowBoxes) == 0 {
		return
	}

	var rows []tableRow
	maxCols := 0
	for _, rb := range rowBoxes {
		rr := tableRow{box: rb, cells: collectCells(rb)}
		if len(rr.cells) > maxCols {
			maxCols = len(rr.cells)
		}
		rows = append(rows, rr)
	}
	if maxCols < 1 {
		return
	}

	// ── Step 2: Compute column widths ──
	colWidths := make([]float64, maxCols)
	explicitCols := make([]bool, maxCols)

	// First pass: check cells in each column for explicit widths.
	for _, row := range rows {
		for ci, cell := range row.cells {
			if ci >= maxCols {
				break
			}
			if explicitCols[ci] {
				continue
			}
			cs := cell.Style()
			if cs != nil && cs.Width.Unit == "px" && cs.Width.Value > 0 {
				colWidths[ci] = cs.Width.Value
				explicitCols[ci] = true
			} else if cs != nil && cs.Width.Unit == "%" && cs.Width.Value > 0 && cw > 0 {
				colWidths[ci] = cw * cs.Width.Value / 100.0
				explicitCols[ci] = true
			}
			// Check HTML width attribute as fallback.
			if !explicitCols[ci] && cell.Element() != nil {
				attrW := cell.Element().GetAttribute("width")
				if attrW != "" {
					if w, err := strconv.ParseFloat(attrW, 64); err == nil && w > 0 {
						colWidths[ci] = w
						explicitCols[ci] = true
					}
				}
			}
		}
	}

	// Distribute the remaining width among non-explicit columns using the
	// table auto-layout heuristic: each column's preferred width is the
	// widest cell content (text advance + horizontal padding + border) in
	// that column; remaining space is then split in proportion to those
	// preferred widths (CSS2.1 §17.5.2.2). This matches Edge for both
	// single-char and multi-char cells.
	explicitTotal := 0.0
	autoCount := maxCols
	for ci := 0; ci < maxCols; ci++ {
		if explicitCols[ci] {
			explicitTotal += colWidths[ci]
			autoCount--
		}
	}
	remaining := cw - explicitTotal
	if remaining < 0 {
		remaining = 0
	}

	// Compute per-column preferred widths from content.
	prefCols := make([]float64, maxCols)
	for _, row := range rows {
		for ci, cell := range row.cells {
			if ci >= maxCols {
				break
			}
			if explicitCols[ci] {
				continue
			}
			fs := fontSizeOf(cell)
			_, padding, border := computeBoxModelForBox(cell, remaining, fs)
			hp := padding.Left + padding.Right + border.Left + border.Right
			w := tableCellPreferredWidth(cell, fs) + hp
			if w > prefCols[ci] {
				prefCols[ci] = w
			}
		}
	}
	sumPref := 0.0
	for ci := 0; ci < maxCols; ci++ {
		if !explicitCols[ci] {
			sumPref += prefCols[ci]
		}
	}
	if autoCount > 0 && sumPref > 0 {
		for ci := 0; ci < maxCols; ci++ {
			if !explicitCols[ci] {
				colWidths[ci] = prefCols[ci] * remaining / sumPref
			}
		}
	} else if autoCount > 0 {
		autoWidth := remaining / float64(autoCount)
		for ci := 0; ci < maxCols; ci++ {
			if !explicitCols[ci] {
				colWidths[ci] = autoWidth
			}
		}
	}

	// ── Step 3: Layout rows and cells ──
	contentLeft := g.ContentBoxLeft()
	y := g.ContentBoxTop()

	for _, row := range rows {
		rg := state.GeometryForBox(row.box)
		rowH := 0.0

		x := contentLeft
		ci := 0
		for _, cell := range row.cells {
			colspan := parseColspan(cell)
			if colspan < 1 {
				colspan = 1
			}

			// Compute total width for this cell (sum of column widths it spans).
			cellTotalWidth := 0.0
			for s := 0; s < colspan && ci+s < maxCols; s++ {
				cellTotalWidth += colWidths[ci+s]
			}

			// Compute box model for the cell.
			fs := fontSizeOf(cell)
			cg := state.GeometryForBox(cell)
			margin, padding, border := computeBoxModelForBox(cell, cellTotalWidth, fs)
			cg.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
			cg.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
			cg.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

			// Set cell's border-box position.
			cellLeft := x + margin.Left
			cellTop := y + margin.Top
			cg.SetTopLeft(cellTop, cellLeft)

			// Compute and set content width.
			borderBoxW := cellTotalWidth - margin.Horizontal()
			if borderBoxW < 0 {
				borderBoxW = 0
			}
			contentW := borderBoxW - border.Horizontal() - padding.Horizontal()
			if contentW < 0 {
				contentW = 0
			}
			cg.SetContentWidth(contentW)

			// Layout cell content.
			cellCtx := contextFor(cell, state)
			if cellCtx != nil {
				cellCtx.Layout(cell, state)
			}

			// Update row height from cell height.
			cellH := cg.BorderBoxHeight()
			if cellH > rowH {
				rowH = cellH
			}

			ci += colspan
			x += cellTotalWidth
		}

		// Equalize all cells in this row to the computed row height.
		for _, cell := range row.cells {
			cg2 := state.GeometryForBox(cell)
			cellContentH := rowH - cg2.VerticalBorderAndPadding()
			if cellContentH < 1 {
				cellContentH = 1
			}
			cg2.SetContentHeight(cellContentH)
		}

		// Set row geometry.
		rg.SetTopLeft(y, contentLeft)
		rg.SetContentWidth(cw)
		if rowH < 1 {
			rowH = 1
		}
		rg.SetContentHeight(rowH)

		y += rowH
	}

	totalHeight := y - g.ContentBoxTop()
	if totalHeight < 0 {
		totalHeight = 0
	}
	g.SetContentHeight(math.Max(g.ContentHeight(), totalHeight))

	// ── Step 4: Size table sections (tbody/thead/tfoot). These grouping
	// elements wrap the rows and must cover their rows' extent; the flat
	// row loop above skips them, leaving their geometry at zero.
	for _, child := range box.Children() {
		eb, ok := child.(*ElementBox)
		if !ok || !eb.IsVisible() || !eb.IsTableSection() {
			continue
		}
		sg := state.GeometryForBox(eb)
		var top, bottom float64
		first := true
		for _, row := range rows {
			if row.box.Parent() != eb {
				continue
			}
			rg := state.GeometryForBox(row.box)
			if first {
				top = rg.Top()
				first = false
			}
			if rb := rg.Top() + rg.BorderBoxHeight(); rb > bottom {
				bottom = rb
			}
		}
		if first {
			continue
		}
		sg.SetTopLeft(top, contentLeft)
		sg.SetContentWidth(cw)
		sg.SetContentHeight(math.Max(1, bottom-top))
	}
}

// tableCellPreferredWidth returns the preferred (max-content) width of a
// table cell: the advance width of its text content, or the widest child
// element for non-text content.
func tableCellPreferredWidth(cell *ElementBox, fs float64) float64 {
	total := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				total += measureText(b, itb.Text())
			} else if eb, ok := c.(*ElementBox); ok {
				// Inline-level children (span/em) contribute their width;
				// block children reset the total (each starts a new line).
				childW := tableCellPreferredWidth(eb, fontSizeOf(eb))
				if eb.IsInlineLevel() {
					total += childW
				} else if childW > total {
					total = childW
				}
			}
		}
	}
	walk(cell)
	if total <= 0 {
		total = fs * 0.5
	}
	return total
}

// ── helper methods for ElementBox ──

func (b *ElementBox) IsTableRow() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableRow
}

func (b *ElementBox) IsTableSection() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableRowGroup ||
		b.style.Display == style.DisplayTableHeaderGroup ||
		b.style.Display == style.DisplayTableFooterGroup
}

func (b *ElementBox) IsTableCell() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableCell
}
