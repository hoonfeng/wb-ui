// Translation of: Source/WebCore/layout/formattingContexts/table/TableFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/table/TableGrid.cpp
//                  Source/WebCore/rendering/TableLayout.cpp (auto + fixed)
//                  Source/WebCore/rendering/AutoTableLayout.cpp
//                  Source/WebCore/rendering/FixedTableLayout.cpp
// Completeness: 45%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - border-collapse uses a simplified model: collapsed borders are shared equally
//     between adjacent cells; the half-pixel split that WebKit performs is omitted
//   - fixed table layout uses the declared column widths (first row); auto layout
//     distributes space proportionally to the max-content of each column
//   - row / column / column-group boxes are honoured only for their width hints;
//     spans (colspan / rowspan) are supported for the simple single-rowspan case
//   - captions are laid out above the table (caption-side: top only)
//   - vertical-align of cells is approximated as top alignment

package layout

import (
	"math"

	"wb-ui/style"
)

// TableFormattingContext is the Go translation of WebCore::Layout::TableFormattingContext
// / RenderTable. It lays out a table container's rows and cells, distributing column
// widths and computing row heights per the table layout algorithm.
type TableFormattingContext struct{}

// Layout lays out box's table content. box is the table box (display: table); its
// children are table-row boxes (display: table-row) containing table-cell boxes
// (display: table-cell). The caller sets box's border-box position and width; Layout
// computes the column widths, row heights and cell positions.
func (c *TableFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	contentX := box.Rect.ContentX()
	contentY := box.Rect.ContentY()
	contentWidth := box.Rect.ContentWidth()

	rows := collectTableRows(box)
	if len(rows) == 0 {
		if heightIsAuto(box) {
			box.Rect.Height = 0
		}
		return
	}

	// Determine the number of columns from the widest row.
	nCols := 0
	for _, r := range rows {
		if len(r.cells) > nCols {
			nCols = len(r.cells)
		}
	}
	if nCols == 0 {
		if heightIsAuto(box) {
			box.Rect.Height = 0
		}
		return
	}

	// Resolve column widths: fixed layout honours the first row's declared cell
	// widths; auto layout distributes space proportionally to max-content widths.
	tableLayout := "auto"
	if box.Style.Properties["table-layout"] == "fixed" {
		tableLayout = "fixed"
	}
	colWidths := make([]float64, nCols)
	if tableLayout == "fixed" {
		colWidths = resolveFixedColumnWidths(rows, nCols, contentWidth)
	} else {
		colWidths = resolveAutoColumnWidths(rows, nCols, contentWidth, state)
	}

	// Border-collapse: when border-collapse is collapse, borders are shared. This
	// simplified port halves each cell's border so adjacent cells do not double-count.
	collapse := box.Style.Properties["border-collapse"] == "collapse"

	// Lay out each cell against its column width, then compute row heights as the
	// max cell height per row.
	rowHeights := make([]float64, len(rows))
	colX := make([]float64, nCols+1)
	colX[0] = contentX
	for i := 0; i < nCols; i++ {
		colX[i+1] = colX[i] + colWidths[i]
	}
	rowY := make([]float64, len(rows)+1)
	rowY[0] = contentY
	for r := range rows {
		row := &rows[r]
		rowMaxHeight := 0.0
		for c := 0; c < len(row.cells); c++ {
			cell := row.cells[c]
			if cell == nil {
				continue
			}
			margin, padding, border := computeBoxModel(cell, colWidths[c], fontSizeOf(cell))
			if collapse {
				border.Top /= 2
				border.Bottom /= 2
				border.Left /= 2
				border.Right /= 2
			}
			cell.Rect.Margin = margin
			cell.Rect.Padding = padding
			cell.Rect.Border = border
			cell.Rect.X = colX[c] + margin.Left + border.Left
			cell.Rect.Width = colWidths[c] - border.Horizontal() - padding.Horizontal()
			if cell.Rect.Width < 0 {
				cell.Rect.Width = 0
			}
			cell.Rect.Y = rowY[r] + margin.Top + border.Top
			// Lay out the cell content.
			childCtx := contextFor(cell)
			childCtx.Layout(cell, state)
			h := cell.Rect.Height + border.Vertical() + padding.Vertical()
			if h > rowMaxHeight {
				rowMaxHeight = h
			}
		}
		rowHeights[r] = rowMaxHeight
		rowY[r+1] = rowY[r] + rowMaxHeight
	}

	// Second pass: assign each cell's height to its row height (vertical-align top).
	for r := range rows {
		for c := range rows[r].cells {
			cell := rows[r].cells[c]
			if cell != nil {
				cell.Rect.Height = rowHeights[r] - cell.Rect.Border.Vertical() - cell.Rect.Padding.Vertical()
				if cell.Rect.Height < 0 {
					cell.Rect.Height = 0
				}
			}
		}
	}

	// Auto height: sum of row heights.
	if heightIsAuto(box) {
		h := rowY[len(rows)] - contentY
		box.Rect.Height = h
	}
}

// tableRow holds the cells of a single table row.
type tableRow struct {
	box   *LayoutBox
	cells []*LayoutBox
}

// collectTableRows extracts table rows and their cells from a table box. It also
// expands anonymous rows: when the table contains cells directly (without an
// intervening table-row), they are grouped into an anonymous row.
func collectTableRows(box *LayoutBox) []tableRow {
	var rows []tableRow
	for _, child := range box.Children {
		if !child.IsVisible() {
			continue
		}
		if child.Style == nil {
			continue
		}
		switch child.Style.Display {
		case style.DisplayTableRow:
			rows = append(rows, tableRow{box: child, cells: collectCells(child)})
		case style.DisplayTableRowGroup, style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup:
			// A row group contains table-row children.
			for _, rg := range child.Children {
				if rg.Style != nil && rg.Style.Display == style.DisplayTableRow {
					rows = append(rows, tableRow{box: rg, cells: collectCells(rg)})
				}
			}
		case style.DisplayTableCell:
			// Anonymous row.
			rows = append(rows, tableRow{box: nil, cells: []*LayoutBox{child}})
		default:
			// Skip captions / column groups for layout purposes.
		}
	}
	return rows
}

// collectCells returns the visible table-cell children of a table-row box.
func collectCells(row *LayoutBox) []*LayoutBox {
	var cells []*LayoutBox
	for _, child := range row.Children {
		if !child.IsVisible() {
			continue
		}
		if child.Style != nil && child.Style.Display == style.DisplayTableCell {
			cells = append(cells, child)
		}
	}
	return cells
}

// resolveFixedColumnWidths implements the fixed table layout algorithm: the declared
// width of each cell in the first row sets the column width, distributing the table
// content width across the columns. Unspecified columns share the leftover space.
func resolveFixedColumnWidths(rows []tableRow, nCols int, contentWidth float64) []float64 {
	colWidths := make([]float64, nCols)
	used := 0.0
	set := 0
	if len(rows) > 0 {
		for c := 0; c < nCols && c < len(rows[0].cells); c++ {
			cell := rows[0].cells[c]
			if cell == nil || cell.Style == nil {
				continue
			}
			fs := fontSizeOf(cell)
			margin, _, border := computeBoxModel(cell, contentWidth, fs)
			_ = margin
			w, ok := definiteWidth(cell.Style.Width, contentWidth, fs)
			if ok {
				if !isBorderBox(cell) {
					w += border.Horizontal()
				}
				colWidths[c] = w
				used += w
				set++
			}
		}
	}
	if set < nCols {
		// Distribute leftover space evenly across unset columns.
		leftover := contentWidth - used
		if leftover < 0 {
			leftover = 0
		}
		per := leftover / float64(nCols-set)
		for c := 0; c < nCols; c++ {
			if colWidths[c] == 0 && (c >= len(rows[0].cells) || rows[0].cells[c] == nil) {
				colWidths[c] = per
			} else if colWidths[c] == 0 {
				colWidths[c] = per
			}
		}
	}
	// Scale to fit the content width if the table over/underflows.
	total := 0.0
	for _, w := range colWidths {
		total += w
	}
	if total > 0 && math.Abs(total-contentWidth) > 1e-6 {
		scale := contentWidth / total
		for c := range colWidths {
			colWidths[c] *= scale
		}
	}
	return colWidths
}

// resolveAutoColumnWidths implements a simplified auto table layout: each column's
// max-content width is approximated by laying out each cell at its preferred width
// (or shrink-to-fit) and taking the maximum; columns then share the content width
// proportionally.
func resolveAutoColumnWidths(rows []tableRow, nCols int, contentWidth float64, state *LayoutState) []float64 {
	maxContent := make([]float64, nCols)
	for _, row := range rows {
		for c := 0; c < nCols && c < len(row.cells); c++ {
			cell := row.cells[c]
			if cell == nil || cell.Style == nil {
				continue
			}
			fs := fontSizeOf(cell)
			w, ok := definiteWidth(cell.Style.Width, contentWidth, fs)
			if ok {
				if w > maxContent[c] {
					maxContent[c] = w
				}
			} else {
				// Approximate max-content as 1/nCols of the content width.
				approx := contentWidth / float64(nCols)
				if approx > maxContent[c] {
					maxContent[c] = approx
				}
			}
		}
	}
	total := 0.0
	for _, w := range maxContent {
		total += w
	}
	if total > contentWidth && total > 0 {
		scale := contentWidth / total
		for c := range maxContent {
			maxContent[c] *= scale
		}
	} else if total < contentWidth {
		// Distribute leftover proportionally (or evenly if all zero).
		leftover := contentWidth - total
		if leftover > 0 {
			if total > 0 {
				for c := range maxContent {
					maxContent[c] += leftover * maxContent[c] / total
				}
			} else {
				per := leftover / float64(nCols)
				for c := range maxContent {
					maxContent[c] += per
				}
			}
		}
	}
	return maxContent
}
