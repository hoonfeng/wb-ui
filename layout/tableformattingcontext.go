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
	defer profileLayout("table")()
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	cw := g.ContentWidth()
	if cw <= 0 {
		return
	}
	// border-collapse: collapse — cell boxes start at the table's border-box
	// edge (cell borders replace the table border), borders merge into shared
	// single lines, and explicit cell widths are content widths (border and
	// padding add to the column).
	collapse := cs.BorderCollapse == style.BorderCollapseCollapse

	// ── Step 1: Collect all rows and cells ──
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
				w := cs.Width.Value
				if collapse {
					// Collapse mode: explicit cell width is a content width; the
					// column additionally carries the cell's border and padding
					// (verified against Edge: td width:80 + 2px borders + 1px
					// padding → 86px column, with borders drawn inside the box).
					_, pad, border := computeBoxModelForBox(cell, 0, fontSizeOf(cell))
					w += pad.Horizontal() + border.Horizontal()
				}
				colWidths[ci] = w
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
	if collapse {
		// Cells sit at the table border-box edge: their borders replace the
		// table's own outer border.
		contentLeft = g.Left()
		y = g.Top()
	}

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

	// ── Step 5: Border collapsing ──
	// border-collapse:collapse merges adjacent cell borders into a single
	// shared line. Conflicts resolve CSS-style: the cell that appears first
	// (top/left in DOM order) wins the color; the wider border wins the
	// width. The losing cell's edge is zeroed so it isn't painted twice.
	if collapse {
		applyCollapseBorders(box, rows, state)
	}
}

// tableRow groups a row box with its collected cells.
type tableRow struct {
	box   *ElementBox
	cells []*ElementBox
}

// borderWidthOf returns the resolved pixel width of a border side.
func borderWidthOf(cs *style.ComputedStyle, side string) float64 {
	if cs == nil {
		return 0
	}
	var l style.Length
	switch side {
	case "top":
		l = cs.BorderTopWidth
	case "right":
		l = cs.BorderRightWidth
	case "bottom":
		l = cs.BorderBottomWidth
	default:
		l = cs.BorderLeftWidth
	}
	return resolveLength(l, 0, 0).Value
}

// setBorderWidth overwrites a border side width on the cell's computed style.
// The painter reads border widths from the style, so this directly controls
// what gets painted (geometry is already laid out).
func setBorderWidth(box *ElementBox, side string, w float64) {
	cs := box.Style()
	if cs == nil {
		return
	}
	l := style.Length{Value: w, Unit: "px"}
	switch side {
	case "top":
		cs.BorderTopWidth = l
	case "right":
		cs.BorderRightWidth = l
	case "bottom":
		cs.BorderBottomWidth = l
	default:
		cs.BorderLeftWidth = l
	}
}

// applyCollapseBorders resolves border conflicts for a border-collapse:collapse
// table (CSS 2.1 §17.6.2.1, simplified to the dominant cases):
//   - a cell's border always beats the table's own outer border (same position;
//     the cell is painted later and covers it)
//   - adjacent cells share the line between them: the first cell (top/left in
//     DOM order) provides the color, the wider border provides the width
//   - the losing edge is zeroed so it paints once
func applyCollapseBorders(table *ElementBox, rows []tableRow, state *LayoutState) {
	ts := table.Style()
	if ts == nil {
		return
	}
	tableW := map[string]float64{
		"top":    borderWidthOf(ts, "top"),
		"right":  borderWidthOf(ts, "right"),
		"bottom": borderWidthOf(ts, "bottom"),
		"left":   borderWidthOf(ts, "left"),
	}
	maxWidth := func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	}
	for ri, row := range rows {
		lastRow := ri == len(rows)-1
		for ci, cell := range row.cells {
			cs := cell.Style()
			if cs == nil {
				continue
			}
			// Top edge: first row conflicts with the table's top border
			// (cell wins → keep width max); inner rows lose to the cell
			// above (which paints the shared line).
			if ri == 0 {
				setBorderWidth(cell, "top", maxWidth(borderWidthOf(cs, "top"), tableW["top"]))
			} else {
				setBorderWidth(cell, "top", 0)
			}
			// Left edge: first column conflicts with the table's left border
			// (cell wins); inner columns lose to the left neighbor.
			if ci == 0 {
				setBorderWidth(cell, "left", maxWidth(borderWidthOf(cs, "left"), tableW["left"]))
			} else {
				setBorderWidth(cell, "left", 0)
			}
			// Right edge: this cell paints the shared line with the right
			// neighbor — width = max of both, color = ours (we come first).
			if ci < len(row.cells)-1 {
				next := row.cells[ci+1]
				ns := next.Style()
				w := maxWidth(borderWidthOf(cs, "right"), borderWidthOf(ns, "left"))
				setBorderWidth(cell, "right", w)
			} else {
				setBorderWidth(cell, "right", maxWidth(borderWidthOf(cs, "right"), tableW["right"]))
			}
			// Bottom edge: this cell paints the shared line with the row
			// below — width = max, color = ours. Last row conflicts with the
			// table's bottom border (cell wins).
			if !lastRow {
				var below *style.ComputedStyle
				if ri+1 < len(rows) && ci < len(rows[ri+1].cells) {
					below = rows[ri+1].cells[ci].Style()
				}
				w := borderWidthOf(cs, "bottom")
				if below != nil {
					w = maxWidth(w, borderWidthOf(below, "top"))
				}
				setBorderWidth(cell, "bottom", w)
			} else {
				setBorderWidth(cell, "bottom", maxWidth(borderWidthOf(cs, "bottom"), tableW["bottom"]))
			}
		}
	}
}

// tablePreferredWidth returns the max-content width of a table: the sum of
// each column's widest cell (text advance + padding + border). Used for
// shrink-to-fit auto width (CSS 2.1 §17.5.2.1).
func tablePreferredWidth(table *ElementBox) float64 {
	rowBoxes := collectRows(table)
	var colPrefs []float64
	for _, rb := range rowBoxes {
		for ci, cell := range collectCells(rb) {
			fs := fontSizeOf(cell)
			_, padding, border := computeBoxModelForBox(cell, 0, fs)
			w := tableCellPreferredWidth(cell, fs) + padding.Horizontal() + border.Horizontal()
			// Explicit cell width wins over content-derived width.
			if cs := cell.Style(); cs != nil && cs.Width.Unit == "px" && cs.Width.Value > 0 {
				w = cs.Width.Value + padding.Horizontal() + border.Horizontal()
			}
			for len(colPrefs) <= ci {
				colPrefs = append(colPrefs, 0)
			}
			if w > colPrefs[ci] {
				colPrefs[ci] = w
			}
		}
	}
	total := 0.0
	for _, w := range colPrefs {
		total += w
	}
	return total
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
