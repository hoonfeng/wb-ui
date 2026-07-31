// Translation of: Source/WebCore/layout/formattingContexts/table/TableFormattingContext.cpp
// (simplified multi-column layout)
//
// MultiColumnFormattingContext lays out children across CSS columns.

package layout

import "math"

type MultiColumnFormattingContext struct {
	FormattingContextBase
}

func (mc *MultiColumnFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)

	colCount := resolveColumnCount(box, state)
	colGap := resolveColumnGap(box)
	if colCount <= 1 {
		ctx := &BlockFormattingContext{}
		ctx.InitBase(box, state)
		ctx.Layout(box, state)
		return
	}


	contentWidth := g.ContentWidth()
	contentHeight := g.ContentHeight()
	totalGap := colGap * float64(colCount-1)
	colWidth := math.Max(0, (contentWidth-totalGap)/float64(colCount))

	if cs.ColumnWidth.Value > 0 && cs.ColumnWidth.Unit == "px" && cs.ColumnWidth.Value < colWidth {
		preferred := cs.ColumnWidth.Value
		computedCount := int(math.Floor((contentWidth + colGap) / (preferred + colGap)))
		if computedCount < 1 { computedCount = 1 }
		if computedCount != colCount {
			colCount = computedCount
			colWidth = math.Max(0, (contentWidth-totalGap)/float64(colCount))
		}
	}

	// Preliminary block layout in an infinite-height container.
	// tempBox shares the children with the real box and is laid out at the
	// column width so each child's intrinsic geometry (top/height) is known
	// before distributing them across columns. Without this Layout call the
	// child geometries are all zero and column assignment is meaningless.
	tempBox := &ElementBox{
		nodeType:  NodeGenericElement,
		style:     cs,
		children:  box.Children(),
		parentBox: box, // non-nil parent prevents BFC's viewport-root handling
	}
	tg := state.GeometryForBox(tempBox)
	tg.SetContentWidth(colWidth)
	tg.SetContentHeight(1e6)
	tg.SetPadding(g.PaddingTop(), g.PaddingRight(), g.PaddingBottom(), g.PaddingLeft())
	blockCtx := &BlockFormattingContext{}
	blockCtx.InitBase(tempBox, state)
	blockCtx.Layout(tempBox, state)

	totalChildHeight := 0.0
	for _, c := range tempBox.Children() {
		if c.IsInFlow() && c.IsVisible() {
			cg := state.GeometryForBox(c)
			ch := cg.Top() + cg.BorderBoxHeight() + cg.MarginAfter()
			if ch > totalChildHeight { totalChildHeight = ch }
		}
	}

	if contentHeight <= 0 || totalChildHeight < contentHeight {
		contentHeight = math.Max(contentHeight, totalChildHeight)
	}
	colHeight := math.Max(50, contentHeight/float64(colCount))

	colIndex := 0
	colYStart := g.Top() + g.PaddingTop() + g.BorderTop()
	colXStart := g.Left() + g.PaddingLeft() + g.BorderLeft()
	colX := colXStart

	info := &columnLayoutInfo{
		columnCount:  colCount,
		columnGap:    colGap,
		columnWidth:  colWidth,
		columnHeight: colHeight,
	}
	box.columnInfo = info

	for _, child := range box.Children() {
		if !child.IsInFlow() || !child.IsVisible() { continue }
		childEb, ok := child.(*ElementBox)
		if !ok { continue }
		cg := state.GeometryForBox(childEb)

cg.SetTopLeft(colYStart+cg.MarginBefore(), colX+cg.MarginStart())

		childBottom := cg.Top() + cg.BorderBoxHeight() + cg.MarginAfter()
		if childBottom > colYStart+colHeight && colIndex < colCount-1 {
			colIndex++
			colX = colXStart + float64(colIndex)*(colWidth+colGap)
cg.SetTopLeft(colYStart+cg.MarginBefore(), colX+cg.MarginStart())
		}

		bw := cg.BorderBoxWidth()
		avail := colWidth - cg.MarginStart() - cg.MarginEnd()
		if bw > avail {
			cg.SetContentWidth(cg.ContentWidth() - (bw - avail))
		}
	}

	info.rulePositions = make([]float64, colCount-1)
	for i := 0; i < colCount-1; i++ {
		info.rulePositions[i] = colXStart + float64(i+1)*(colWidth+colGap) - colGap/2
	}

	// Update box content height to reflect multi-column content.
	g.SetContentHeight(math.Max(g.ContentHeight(), colHeight))
}

type columnLayoutInfo struct {
	columnCount   int
	columnGap     float64
	columnWidth   float64
	columnHeight  float64
	rulePositions []float64
}

func resolveColumnCount(box *ElementBox, state *LayoutState) int {
	cs := box.Style()
	if cs == nil { return 1 }
	if cs.ColumnCount > 0 { return cs.ColumnCount }
	if cs.ColumnWidth.Value > 0 && cs.ColumnWidth.Unit == "px" {
		g := state.GeometryForBox(box)
		available := g.ContentWidth()
		gap := resolveColumnGap(box)
		if gap+cs.ColumnWidth.Value <= 0 { return 1 }
		n := int(math.Floor((available + gap) / (cs.ColumnWidth.Value + gap)))
		if n < 1 { n = 1 }
		return n
	}
	return 1
}

func resolveColumnGap(box *ElementBox) float64 {
	cs := box.Style()
	if cs != nil && cs.ColumnGap.Unit == "px" && cs.ColumnGap.Value > 0 {
		return cs.ColumnGap.Value
	}
	return 16.0
}

func HasColumns(box Box) bool {
	cs := box.Style()
	if cs == nil { return false }
	return cs.ColumnCount > 0 || (cs.ColumnWidth.Value > 0 && cs.ColumnWidth.Unit == "px")
}

func GetColumnInfo(box *ElementBox) *columnLayoutInfo {
	return box.columnInfo
}
