package layout

import (
	"math"
)

// MultiColumnFormattingContext lays out a block container's children across
// multiple CSS columns (multi-col). It implements CSS Multi-column Layout
// (CSS Multi-col) for the common case.
//
// The strategy:
//  1. Compute column count and column width from CSS properties.
//  2. Partition the content height evenly across the columns.
//  3. Lay out children into each column, clipping overflow.
//  4. Position columns side by side with gaps.
//  5. Draw column rules between columns.
type MultiColumnFormattingContext struct {
	parent *BlockFormattingContext
}

// Layout lays out box and its descendants as a multi-column container.
func (mc *MultiColumnFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	// Determine column parameters.
	colCount := resolveColumnCount(box)
	colGap := resolveColumnGap(box)
	if colCount <= 1 {
		// Fall back to block layout for single column.
		ctx := &BlockFormattingContext{}
		ctx.Layout(box, state)
		return
	}

	contentWidth := box.Rect.ContentWidth()
	contentHeight := box.Rect.ContentHeight()

	// Compute total gap width.
	totalGap := colGap * float64(colCount-1)

	// Available width per column.
	colWidth := math.Max(0, (contentWidth-totalGap)/float64(colCount))

	// If column-width is set and produces a narrower width, use it.
	if box.Style != nil && box.Style.ColumnWidth.Value > 0 {
		preferred := box.Style.ColumnWidth.Value
		if box.Style.ColumnWidth.Unit == "px" && preferred < colWidth {
			// Recalculate column count based on preferred width.
			computedCount := int(math.Floor((contentWidth + colGap) / (preferred + colGap)))
			if computedCount < 1 {
				computedCount = 1
			}
			if computedCount != colCount {
				colCount = computedCount
				colWidth = math.Max(0, (contentWidth-totalGap)/float64(colCount))
			}
		}
	}

	// Create column spanners and wrappers.
	// In this simplified implementation, we:
	//   - Lay out children in document order
	//   - Assign them to columns round-robin or by measuring content height
	//   - Position each column's children with appropriate offsets

	// First, lay out all children in a single flow to get their heights.
	// We do a preliminary block layout in an infinite-height container.
	tempBox := &LayoutBox{
		Type:     BoxAnonymous,
		Style:    box.Style,
		Children: box.Children,
		Rect: LayoutRect{
			Width:  colWidth,
			Height: 1e6, // effectively infinite
		},
	}
	tempBox.Rect.Padding = box.Rect.Padding
	tempBox.Rect.Border = box.Rect.Border

	// Run block layout to measure children.
	blockCtx := &BlockFormattingContext{}
	blockCtx.Layout(tempBox, state)

	// Calculate total content height needed.
	totalChildHeight := 0.0
	for _, c := range tempBox.Children {
		if c.IsInFlow() && c.IsVisible() {
			ch := c.Rect.Y + c.Rect.Height + c.Rect.Margin.Bottom
			if ch > totalChildHeight {
				totalChildHeight = ch
			}
		}
	}

	// If we have a fixed content height, distribute; otherwise use totalChildHeight.
	if contentHeight <= 0 || totalChildHeight < contentHeight {
		contentHeight = math.Max(contentHeight, totalChildHeight)
	}

	// Column height: evenly distribute content.
	colHeight := math.Max(50, contentHeight/float64(colCount))

	// Now position children in columns.
	colIndex := 0
	colY := box.Rect.Y + box.Rect.Padding.Top + box.Rect.Border.Top
	colXStart := box.Rect.X + box.Rect.Padding.Left + box.Rect.Border.Left
	colX := colXStart

	// Store column geometry on the box for the render tree.
	box.columnInfo = &columnLayoutInfo{
		columnCount: colCount,
		columnGap:   colGap,
		columnWidth: colWidth,
		columnHeight: colHeight,
	}

	// Measure and distribute children across columns.
	// This simplified version places children in columns sequentially.
	for _, child := range box.Children {
		if !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		// Determine column based on content accumulation.
		// Simple round-robin: each child fills current column until height exceeded.
		childRect := &child.Rect

		// Set column position.
		childRect.X = colX + childRect.Margin.Left
		childRect.Y = colY + childRect.Margin.Top

		// Track remaining column height.
		childBottom := childRect.Y + childRect.Height + childRect.Margin.Bottom
		if childBottom > colY+colHeight && colIndex < colCount-1 {
			// Move to next column.
			colIndex++
			colX = colXStart + float64(colIndex)*(colWidth+colGap)
			childRect.X = colX + childRect.Margin.Left
			childRect.Y = colY + childRect.Margin.Top
		}

		// Clamp width to column width minus margins.
		childRect.Width = math.Min(childRect.Width, colWidth-childRect.Margin.Horizontal())

		// Apply column rule position data.
		if colIndex > 0 && colIndex-1 < len(box.columnInfo.rulePositions) {
			// Use stored rule positions from after positioning.
		}
	}

	// Store column rule positions on the box.
	box.columnInfo.rulePositions = make([]float64, colCount-1)
	for i := 0; i < colCount-1; i++ {
		ruleX := colXStart + float64(i+1)*(colWidth+colGap) - colGap/2
		box.columnInfo.rulePositions[i] = ruleX
	}

	// Update the box's own content height to reflect the actual multi-column height.
	box.Rect.Height = math.Max(box.Rect.Height, colHeight+box.Rect.Padding.Vertical()+box.Rect.Border.Vertical())
}

// columnLayoutInfo stores multi-column layout data computed during layout.
type columnLayoutInfo struct {
	columnCount   int
	columnGap     float64
	columnWidth   float64
	columnHeight  float64
	rulePositions []float64
}

// resolveColumnCount returns the effective number of columns.
func resolveColumnCount(box *LayoutBox) int {
	if box.Style == nil {
		return 1
	}
	if box.Style.ColumnCount > 0 {
		return box.Style.ColumnCount
	}
	// If column-width is set, compute column count from available width.
	if box.Style.ColumnWidth.Value > 0 && box.Style.ColumnWidth.Unit == "px" {
		available := box.Rect.ContentWidth()
		gap := resolveColumnGap(box)
		if gap+box.Style.ColumnWidth.Value <= 0 {
			return 1
		}
		n := int(math.Floor((available + gap) / (box.Style.ColumnWidth.Value + gap)))
		if n < 1 {
			n = 1
		}
		return n
	}
	return 1
}

// resolveColumnGap returns the column gap in pixels.
func resolveColumnGap(box *LayoutBox) float64 {
	if box.Style != nil && box.Style.ColumnGap.Unit == "px" {
		if box.Style.ColumnGap.Value > 0 {
			return box.Style.ColumnGap.Value
		}
	}
	// Default gap (1em ≈ 16px).
	return 16.0
}

// HasColumns reports whether the box should use multi-column layout.
func HasColumns(box *LayoutBox) bool {
	if box.Style == nil {
		return false
	}
	return box.Style.ColumnCount > 0 ||
		(box.Style.ColumnWidth.Value > 0 && box.Style.ColumnWidth.Unit == "px")
}

// GetColumnInfo returns the column layout info stored on the box, or nil.
func GetColumnInfo(box *LayoutBox) *columnLayoutInfo {
	return box.columnInfo
}
