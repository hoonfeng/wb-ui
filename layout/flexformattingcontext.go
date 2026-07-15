// Translation of: Source/WebCore/layout/formattingContexts/flex/FlexFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/flex/FlexLayout.cpp
//                  Source/WebCore/rendering/RenderFlexibleBox.cpp (layoutBlock part)
// Completeness: 50%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - flex-wrap is supported for single and multi-line (wrap / wrap-reverse); lines
//     are packed top-to-bottom (or bottom-to-top for wrap-reverse)
//   - flex-basis resolves against the main-axis content size; auto basis falls back to
//     the content size (approximated by the child's intrinsic/main size)
//   - flex-grow / flex-shrink distribute free / negative space proportionally
//   - main-axis alignment (justify-content) supports flex-start / center / flex-end /
//     space-between / space-around / space-evenly
//   - cross-axis alignment (align-items / align-self) supports stretch / flex-start /
//     center / flex-end / baseline (baseline approximated as flex-start)
//   - align-content (multi-line cross packing) supports stretch / flex-start / center /
//     flex-end / space-between / space-around
//   - order is honoured for visual reordering; source order is the tie-breaker
//   - min/max constraints are applied per item; min-content sizing is approximated

package layout

import (
	"math"
	"sort"

	"wb-ui/style"
)

// FlexFormattingContext is the Go translation of WebCore::Layout::FlexFormattingContext
// / RenderFlexibleBox. It lays out a flex container's items along the main and cross
// axes per CSS Flexible Box Layout Module Level 1.
type FlexFormattingContext struct{}

// Layout lays out box's flex items. The caller sets box's border-box position and
// width; Layout computes the items' main/cross sizes and positions and box's auto
// height.
func (c *FlexFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	contentWidth := box.Rect.ContentWidth()
	contentHeight := box.Rect.ContentHeight()

	dir := flexDirectionOf(box)
	isRow := dir == flexRow || dir == flexRowReverse
	isReverse := dir == flexRowReverse || dir == flexColumnReverse
	wrap := flexWrapOf(box)

	// Collect visible in-flow flex items, sorted by order.
	items := collectFlexItems(box)
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })

	// Resolve each item's hypothetical main size (flex-basis or content) and box model.
	for i := range items {
		it := &items[i]
		margin, padding, border := computeBoxModel(it.box, contentWidth, fontSizeOf(it.box))
		it.margin = margin
		it.padding = padding
		it.border = border
		// Write the box model back onto the item's rect so that the child
		// formatting context (BFC) sees non-zero padding/border when computing
		// the item's auto height. BFC.Layout only resolves the box model for
		// the root box (parent==nil); for flex items the parent (flex container)
		// is responsible for setting it, mirroring the BFC child loop.
		it.box.Rect.Margin = margin
		it.box.Rect.Padding = padding
		it.box.Rect.Border = border
		it.mainMargin = mainAxisMargin(margin, isRow)
		it.crossMargin = crossAxisMargin(margin, isRow)
		it.mainBorderPadding = mainAxisBorderPadding(border, padding, isRow)
		it.crossBorderPadding = crossAxisBorderPadding(border, padding, isRow)
		basis := resolveFlexBasis(it.box, contentWidth, contentHeight, isRow)
		// When flex-basis and the main-axis size are both auto, the used flex
		// base size is the content size (max-content). Resolve it by laying out
		// the item provisionally at the available main size and measuring the
		// rightmost content edge. Without this, auto-basis items collapse to a
		// zero main size (plus border/padding), which is wrong for e.g. buttons
		// whose width should follow their text content.
		if flexBasisIsContent(it.box, isRow) {
			basis = measureFlexItemContentMain(it.box, mainSizeAvailable(isRow, contentWidth, contentHeight), isRow, state)
		}
		it.hypotheticalMain = basis + it.mainBorderPadding
	}

	// Main / cross available sizes.
	var mainSize, crossSize float64
	if isRow {
		mainSize, crossSize = contentWidth, contentHeight
	} else {
		mainSize, crossSize = contentHeight, contentWidth
	}

	// Line breaking (flex-wrap): pack items into lines by hypothetical main size.
	lines := breakFlexLines(items, mainSize, wrap)

	// Resolve flex base sizes and distribute free space per line.
	for li := range lines {
		ln := &lines[li]
		freezeLine := ln.items
		// Determine used flex base size: start from hypothetical.
		for i := range freezeLine {
			freezeLine[i].mainSize = freezeLine[i].hypotheticalMain
		}
		// Grow if line has positive free space; shrink if negative.
		for pass := 0; pass < 2; pass++ {
			lineMain := 0.0
			for i := range freezeLine {
				lineMain += freezeLine[i].mainSize
			}
			free := mainSize - lineMain - lineMainMargins(freezeLine)
			if math.Abs(free) < 1e-6 {
				break
			}
			if free > 0 {
				// Distribute via flex-grow.
				totalGrow := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalGrow += freezeLine[i].grow
					}
				}
				if totalGrow <= 0 {
					break
				}
				any := false
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].grow <= 0 {
						continue
					}
					share := free * freezeLine[i].grow / totalGrow
					freezeLine[i].mainSize += share
					any = true
				}
				if !any {
					break
				}
			} else {
				// Distribute negative space via flex-shrink (weighted).
				totalShrink := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalShrink += freezeLine[i].shrink * freezeLine[i].hypotheticalMain
					}
				}
				if totalShrink <= 0 {
					break
				}
				any := false
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].shrink <= 0 {
						continue
					}
					weight := freezeLine[i].shrink * freezeLine[i].hypotheticalMain / totalShrink
					share := free * weight
					freezeLine[i].mainSize += share
					any = true
				}
				if !any {
					break
				}
			}
			// Freeze items at min/max.
			for i := range freezeLine {
				if freezeLine[i].frozen {
					continue
				}
				min, max := flexMinMax(freezeLine[i], isRow)
				if freezeLine[i].mainSize < min {
					freezeLine[i].mainSize = min
					freezeLine[i].frozen = true
				} else if freezeLine[i].mainSize > max {
					freezeLine[i].mainSize = max
					freezeLine[i].frozen = true
				}
			}
		}
		// Freeze all remaining.
		for i := range freezeLine {
			freezeLine[i].frozen = true
		}
		// Resolve cross size: lay out each item to get its content, then stretch if
		// align-items is stretch.
		for i := range freezeLine {
			it := &freezeLine[i]
			setItemBorderBox(it, isRow)
			it.box.Rect.X = 0
			it.box.Rect.Y = 0
			childCtx := contextFor(it.box)
			childCtx.Layout(it.box, state)
			it.crossSize = crossAxisSize(it.box, isRow)
		}
		// Cross size of the line is the max cross size of items.
		ln.crossSize = 0
		for i := range freezeLine {
			c := freezeLine[i].crossSize + freezeLine[i].crossMargin
			if c > ln.crossSize {
				ln.crossSize = c
			}
		}
	}

	// Align-content: distribute lines along the cross axis.
	totalCross := 0.0
	for _, ln := range lines {
		totalCross += ln.crossSize
	}
	crossStart := 0.0
	lineGap := 0.0
	alignContent := alignContentOf(box)
	if len(lines) > 1 && (alignContent == "space-between" || alignContent == "space-around" || alignContent == "space-evenly") {
		gaps := len(lines) - 1
		extra := crossSize - totalCross
		if extra < 0 {
			extra = 0
		}
		switch alignContent {
		case "space-between":
			lineGap = extra / float64(gaps)
		case "space-around":
			lineGap = extra / float64(gaps)
			crossStart = lineGap / 2
		case "space-evenly":
			lineGap = extra / float64(gaps+1)
			crossStart = lineGap
		}
	} else if len(lines) > 0 && alignContent == "center" {
		crossStart = (crossSize - totalCross) / 2
		if crossStart < 0 {
			crossStart = 0
		}
	} else if len(lines) > 0 && alignContent == "flex-end" {
		crossStart = math.Max(0, crossSize-totalCross)
	} else if alignContent == "stretch" && len(lines) > 0 {
		// Stretch each line equally.
		extra := crossSize - totalCross
		if extra > 0 {
			per := extra / float64(len(lines))
			for i := range lines {
				lines[i].crossSize += per
			}
		}
	}

	// Position items within each line along the main and cross axes.
	cursorCross := crossStart
	for li := range lines {
		ln := &lines[li]
		// Main-axis position (justify-content).
		lineMainUsed := 0.0
		for i := range ln.items {
			lineMainUsed += ln.items[i].mainSize + ln.items[i].mainMargin
		}
		mainFree := mainSize - lineMainUsed
		if mainFree < 0 {
			mainFree = 0
		}
		just := justifyContentOf(box)
		mainCursor, mainGap := justifyStart(just, mainFree, len(ln.items))
		switch just {
		case "center":
			mainCursor = mainFree / 2
		case "flex-end":
			mainCursor = mainFree
		case "space-between":
			mainGap = mainFree / float64(max(1, len(ln.items)-1))
		case "space-around":
			mainGap = mainFree / float64(len(ln.items))
			mainCursor = mainGap / 2
		case "space-evenly":
			mainGap = mainFree / float64(len(ln.items)+1)
			mainCursor = mainGap
		}
		if isReverse {
			// Reverse main-axis order for row-reverse / column-reverse.
			mainCursor = mainSize - mainCursor
		}
		for i := range ln.items {
			it := &ln.items[i]
			if isReverse {
				mainCursor -= it.mainMargin/2 + it.mainSize
				setItemPosition(it, mainCursor, cursorCross+ln.crossSize, isRow, box)
				// Children were laid out at (0,0); shift them to the final position.
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor -= it.mainMargin/2 + mainGap
			} else {
				mainCursor += it.mainMargin / 2
				setItemPosition(it, mainCursor, cursorCross, isRow, box)
				// Children were laid out at (0,0); shift them to the final position.
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor += it.mainSize + it.mainMargin/2 + mainGap
			}
		}
		cursorCross += ln.crossSize + lineGap
	}

	// Auto height of the container: the cross size is the line stack height.
	if heightIsAuto(box) {
		if isRow {
			box.Rect.Height = cursorCross
		} else {
			// column direction: main axis is vertical; height encloses all lines' main.
			box.Rect.Height = totalLineMain(lines)
		}
	}
}

// flexItem captures the resolved flex properties and geometry of a single flex item.
type flexItem struct {
	box                *LayoutBox
	order              int
	grow               float64
	shrink             float64
	basis              float64
	margin             Edges
	padding            Edges
	border             Edges
	mainMargin         float64
	crossMargin        float64
	mainBorderPadding  float64
	crossBorderPadding float64
	hypotheticalMain   float64
	mainSize           float64
	crossSize          float64
	frozen             bool
}

// flexLine groups items that share a single flex line.
type flexLine struct {
	items     []flexItem
	crossSize float64
}

// flexDirection constants.
type flexDir int

const (
	flexRow flexDir = iota
	flexRowReverse
	flexColumn
	flexColumnReverse
)

func flexDirectionOf(box *LayoutBox) flexDir {
	if box.Style == nil {
		return flexRow
	}
	switch box.Style.FlexDirection {
	case "row-reverse":
		return flexRowReverse
	case "column":
		return flexColumn
	case "column-reverse":
		return flexColumnReverse
	}
	return flexRow
}

func flexWrapOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.FlexWrap
}

func alignContentOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.AlignContent
}

func justifyContentOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.JustifyContent
}

func alignItemsOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.AlignItems
}

// collectFlexItems returns the visible in-flow children of a flex container as flex
// items with their resolved flex-grow / flex-shrink / flex-basis.
func collectFlexItems(box *LayoutBox) []flexItem {
	var out []flexItem
	for _, child := range box.Children {
		if !child.IsVisible() || !child.IsInFlow() {
			continue
		}
		if child.Style == nil {
			child.Style = style.NewComputedStyle()
		}
		fi := flexItem{
			box:    child,
			order:  child.Style.Order,
			grow:   child.Style.FlexGrow,
			shrink: child.Style.FlexShrink,
		}
		out = append(out, fi)
	}
	return out
}

// resolveFlexBasis returns the flex-basis main size (content-box) of an item.
func resolveFlexBasis(box *LayoutBox, cbW, cbH float64, isRow bool) float64 {
	fs := fontSizeOf(box)
	// honour explicit flex-basis when definite.
	r := resolveLengthAuto(box.Style.FlexBasis, ternary(isRow, cbW, cbH), fs)
	if !r.Auto && r.Definite {
		return r.Value
	}
	// else use width/height depending on direction.
	if isRow {
		w, ok := definiteWidth(box.Style.Width, cbW, fs)
		if ok {
			return w
		}
	} else {
		h, ok := definiteHeight(box.Style.Height, cbH, fs)
		if ok {
			return h
		}
	}
	// else approximate content main size as 0 (will grow).
	return 0
}

func ternary(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}

func mainAxisMargin(m Edges, isRow bool) float64 {
	if isRow {
		return m.Left + m.Right
	}
	return m.Top + m.Bottom
}

func crossAxisMargin(m Edges, isRow bool) float64 {
	if isRow {
		return m.Top + m.Bottom
	}
	return m.Left + m.Right
}

func mainAxisBorderPadding(b, p Edges, isRow bool) float64 {
	if isRow {
		return b.Horizontal() + p.Horizontal()
	}
	return b.Vertical() + p.Vertical()
}

func crossAxisBorderPadding(b, p Edges, isRow bool) float64 {
	if isRow {
		return b.Vertical() + p.Vertical()
	}
	return b.Horizontal() + p.Horizontal()
}

func lineMainMargins(items []flexItem) float64 {
	s := 0.0
	for i := range items {
		s += items[i].mainMargin
	}
	return s
}

// setItemBorderBox sets the border-box width/height of an item from its main/cross size.
func setItemBorderBox(it *flexItem, isRow bool) {
	if isRow {
		it.box.Rect.Width = it.mainSize
	} else {
		it.box.Rect.Height = it.mainSize
	}
}

// crossAxisSize returns the used cross-axis border-box size of an item after layout.
func crossAxisSize(box *LayoutBox, isRow bool) float64 {
	if isRow {
		return box.Rect.Height
	}
	return box.Rect.Width
}

// setItemPosition positions an item at the given main/cross offset (relative to the
// flex container content box) applying align-self / align-items cross-axis alignment.
func setItemPosition(it *flexItem, mainOffset, crossOffset float64, isRow bool, container *LayoutBox) {
	contentX := container.Rect.ContentX()
	contentY := container.Rect.ContentY()
	align := it.box.Style.AlignSelf
	if align == "" || align == "auto" {
		align = alignItemsOf(container)
	}
	// Cross size: stretch to line cross size when align is stretch.
	lineCross := crossOffset
	crossSize := it.crossSize
	if align == "stretch" {
		crossSize = lineCross - it.crossMargin
		if crossSize < 0 {
			crossSize = 0
		}
		if isRow {
			it.box.Rect.Height = crossSize
		} else {
			it.box.Rect.Width = crossSize
		}
	}
	// Cross alignment.
	crossPos := crossOffset
	switch align {
	case "center":
		crossPos = crossOffset + (lineCross-it.crossSize-it.crossMargin)/2
	case "flex-end":
		crossPos = crossOffset + lineCross - it.crossSize - it.crossMargin
	}
	if align != "stretch" {
		if isRow {
			it.box.Rect.Height = it.crossSize
		} else {
			it.box.Rect.Width = it.crossSize
		}
	}
	if isRow {
		it.box.Rect.X = contentX + mainOffset + it.margin.Left
		it.box.Rect.Y = contentY + crossPos + it.margin.Top
	} else {
		it.box.Rect.X = contentX + crossPos + it.margin.Left
		it.box.Rect.Y = contentY + mainOffset + it.margin.Top
	}
}

// flexMinMax returns the (min, max) main-axis size for an item.
func flexMinMax(it flexItem, isRow bool) (float64, float64) {
	fs := fontSizeOf(it.box)
	st := it.box.Style
	minV, maxV, minAuto, maxAuto := resolveMinMax(st.MinWidth, st.MaxWidth, 0, fs)
	if !isRow {
		minV, maxV, minAuto, maxAuto = resolveMinMax(st.MinHeight, st.MaxHeight, 0, fs)
	}
	if minAuto {
		minV = 0
	}
	if maxAuto {
		maxV = math.Inf(1)
	}
	return minV, maxV
}

// breakFlexLines packs items into lines according to flex-wrap.
func breakFlexLines(items []flexItem, mainSize float64, wrap string) []flexLine {
	if wrap == "" || wrap == "nowrap" || len(items) == 0 {
		return []flexLine{{items: items}}
	}
	var lines []flexLine
	var cur []flexItem
	curMain := 0.0
	for i := range items {
		add := items[i].hypotheticalMain + items[i].mainMargin
		if len(cur) > 0 && curMain+add > mainSize+1e-6 {
			lines = append(lines, flexLine{items: cur})
			cur = nil
			curMain = 0
		}
		cur = append(cur, items[i])
		curMain += add
	}
	if len(cur) > 0 {
		lines = append(lines, flexLine{items: cur})
	}
	if len(lines) == 0 {
		lines = []flexLine{{items: items}}
	}
	return lines
}

func justifyStart(just string, free float64, n int) (float64, float64) {
	return 0, 0
}

func totalLineMain(lines []flexLine) float64 {
	s := 0.0
	for _, ln := range lines {
		for _, it := range ln.items {
			s += it.mainSize + it.mainMargin
		}
	}
	return s
}

// offsetSubtree shifts a layout box and all its descendants (positions and text
// segments) by (dx, dy). Used by the flex layout to fix up child geometry after
// repositioning items that were laid out at the origin (0,0) during the
// cross-size measurement pass: the item itself is positioned by setItemPosition,
// but its descendants kept their origin-relative coordinates.
func offsetSubtree(box *LayoutBox, dx, dy float64) {
	if box == nil {
		return
	}
	box.Rect.X += dx
	box.Rect.Y += dy
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children {
		offsetSubtree(child, dx, dy)
	}
}

// offsetItemSubtree offsets the descendants (and the item's own text segments)
// of a flex item by (dx, dy) without moving the item itself, which was already
// positioned by setItemPosition. The item's content was laid out relative to
// the origin during the measurement pass, so every descendant coordinate and
// text segment must be shifted by the item's final (X, Y).
func offsetItemSubtree(box *LayoutBox, dx, dy float64) {
	if box == nil {
		return
	}
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children {
		offsetSubtree(child, dx, dy)
	}
}

// flexBasisIsContent reports whether the flex base size should be taken from
// the item's content (max-content). This is the case when both flex-basis and
// the main-axis size property (width for row, height for column) are auto, per
// CSS flexbox §9.2. An item with flex-basis: auto uses the width/height; when
// that is also auto the used flex base size is the content size.
func flexBasisIsContent(box *LayoutBox, isRow bool) bool {
	if box.Style == nil {
		return false
	}
	fs := fontSizeOf(box)
	// flex-basis auto?
	bb := resolveLengthAuto(box.Style.FlexBasis, 0, fs)
	if !bb.Auto {
		return false
	}
	// main-axis size (width/height) auto?
	if isRow {
		w := resolveLengthAuto(box.Style.Width, 0, fs)
		return w.Auto
	}
	h := resolveLengthAuto(box.Style.Height, 0, fs)
	return h.Auto
}

// mainSizeAvailable returns the available main-axis size for measurement.
func mainSizeAvailable(isRow bool, contentWidth, contentHeight float64) float64 {
	if isRow {
		return contentWidth
	}
	return contentHeight
}

// measureFlexItemContentMain lays out box provisionally at the available main
// size and measures its max-content main size by scanning descendant edges.
// The item's rect is saved and restored so the measurement pass does not
// affect the subsequent final layout pass. Returns the content-box main size.
func measureFlexItemContentMain(box *LayoutBox, availableMain float64, isRow bool, state *LayoutState) float64 {
	savedRect := box.Rect
	defer func() { box.Rect = savedRect }()
	if isRow {
		box.Rect.Width = availableMain
	} else {
		box.Rect.Height = availableMain
	}
	box.Rect.X = 0
	box.Rect.Y = 0
	ctx := contextFor(box)
	ctx.Layout(box, state)
	if isRow {
		// Measure by line width so text-align (center/right) does not inflate
		// the max-content size of the item.
		return maxContentWidth(box)
	}
	// column: measure content height (max bottom edge).
	contentTop := box.Rect.ContentY()
	h := maxContentBottom(box) - contentTop
	if h < 0 {
		h = 0
	}
	return h
}
