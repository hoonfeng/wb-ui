// Flex formatting context — lays out children in a flex container.
// Translation of: Source/WebCore/layout/formattingContexts/flex/FlexFormattingContext.cpp

package layout

import (
	"math"
	"sort"

	"wb-ui/style"
)

type FlexFormattingContext struct {
	FormattingContextBase
}

type flexItem struct {
	box             *ElementBox
	flexGrow        float64
	flexShrink      float64
	flexBasis       float64
	minWidth        float64
	maxWidth        float64
	minHeight       float64
	maxHeight       float64
	baseSize        float64
	hypothetical    float64
	frozen          bool
	targetSize      float64
	finalMainSize   float64
	marginMain      float64
	marginCross     float64
	order           int
	baselineOffset  float64
}

func (c *FlexFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	isRow := cs.FlexDirection != "column" && cs.FlexDirection != "column-reverse"
	isReverse := cs.FlexDirection == "row-reverse" || cs.FlexDirection == "column-reverse"

	g := state.GeometryForBox(box)
	_, padding, border := computeBoxModel(box, g.ContentWidth(), fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
	g.SetContentWidth(g.ContentWidth() - padding.Left - padding.Right - border.Left - border.Right)
	cw := g.ContentWidth()
	ch := g.ContentHeight()

	var items []*flexItem
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok && child.IsInFlow() && child.IsVisible() {
			item := c.resolveItem(childEb, isRow, cw, ch, state)
			items = append(items, item)
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })
	if len(items) == 0 { return }

	mainSize := cw
	if !isRow { mainSize = ch }

	for _, it := range items {
		it.baseSize = it.resolveBaseSize(mainSize, isRow)
		it.hypothetical = it.baseSize
		it.targetSize = it.baseSize
	}

	c.distributeFreeSpace(items, mainSize, isRow)

	// Before resolving cross sizes and positions, compute a preliminary
	// container content height from children so that cross-axis centering
	// and flex-end alignment work correctly when the container has auto-height.
	// Without this, ch=0 causes negative offsets (children float above parent).
	if heightIsAutoForBox(box) {
		if isRow {
			// Row flex: estimate auto height from the tallest child,
			// including each child's actual vertical padding and border.
			estH := 0.0
			for _, it := range items {
				childH := fontLineGap(it.box)
				cg := state.GeometryForBox(it.box)
				childH += cg.PaddingTop() + cg.PaddingBottom() + cg.BorderTop() + cg.BorderBottom()
				if childH > estH {
					estH = childH
				}
			}
			if estH > ch {
				g.SetContentHeight(estH)
				ch = estH
			}
		} else {
			// Column flex: estimate auto height from the sum of child main-sizes,
			// including each child's actual start/end padding and border.
			estH := 0.0
			for _, it := range items {
				childH := it.finalMainSize + it.marginMain
				cg := state.GeometryForBox(it.box)
				childH += cg.PaddingTop() + cg.PaddingBottom() + cg.BorderTop() + cg.BorderBottom()
				estH += childH
			}
			if estH > ch {
				g.SetContentHeight(estH)
				ch = estH
			}
		}
	}

	c.resolveCrossSizes(items, isRow, isReverse, false, cw, ch, state)
	c.applyPositions(items, box, isRow, isReverse, false, state)

	// Compute auto container height from children.
	// Preserve any height already set by parent formatting context (e.g. grid row height).
	if heightIsAutoForBox(box) {
		contentTop := g.ContentBoxTop()
		maxChildBottom := contentTop
		for _, it := range items {
			cg := state.GeometryForBox(it.box)
			if bottom := cg.Top() + cg.BorderBoxHeight(); bottom > maxChildBottom {
				maxChildBottom = bottom
			}
		}
		blockSize := maxChildBottom - contentTop
		if blockSize < 0 { blockSize = 0 }
		// If parent set a larger height (e.g. grid row), keep it.
		if box.Parent() != nil && blockSize < g.ContentHeight() {
			blockSize = g.ContentHeight()
		}
		g.SetContentHeight(blockSize)
	}
}

func (c *FlexFormattingContext) resolveItem(box *ElementBox, isRow bool, cbWidth, cbHeight float64, state *LayoutState) *flexItem {
	cs := box.Style()
	g := state.GeometryForBox(box)
	margin, padding, border := computeBoxModel(box, cbWidth, fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	mm, mc := margin.Left+margin.Right, margin.Top+margin.Bottom
	if !isRow { mm, mc = margin.Top+margin.Bottom, margin.Left+margin.Right }

	fs := fontSizeOf(box)
	flexGrow := cs.FlexGrow
	flexShrink := cs.FlexShrink

	var flexBasis float64
	fb := cs.FlexBasis
	if fb.Unit == "auto" || fb.Unit == "" {
		if isRow {
			r := resolveLengthAuto(cs.Width, cbWidth, fs)
			if !r.Auto && r.Definite { flexBasis = r.Value }
		} else {
			r := resolveLengthAuto(cs.Height, cbHeight, fs)
			if !r.Auto && r.Definite { flexBasis = r.Value }
		}
	} else {
		flexBasis = resolveOrZero(fb, cbWidth, fs)
	}

	minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbWidth, fs)
	minH, maxH, _, _ := resolveMinMax(cs.MinHeight, cs.MaxHeight, cbHeight, fs)

	return &flexItem{
		box: box, flexGrow: flexGrow, flexShrink: flexShrink,
		flexBasis: flexBasis, minWidth: minW, maxWidth: maxW,
		minHeight: minH, maxHeight: maxH,
		marginMain: mm, marginCross: mc, order: cs.Order,
	}
}

func (it *flexItem) resolveBaseSize(containerMainSize float64, isRow bool) float64 {
	base := it.flexBasis
	if base <= 0 {
		// CSS default: flex-basis:auto + width:auto → max-content size.
		// For now compute a simple intrinsic width from the box's children.
		base = intrinsicContentWidth(it.box, isRow)
		if base <= 0 { base = 0 }
	}
	if isRow {
		return clampSize(base, it.minWidth, it.maxWidth, it.minWidth <= 0, it.maxWidth <= 0)
	}
	return clampSize(base, it.minHeight, it.maxHeight, it.minHeight <= 0, it.maxHeight <= 0)
}

// intrinsicContentWidth returns the max-content width of a box by inspecting
// its children without performing full layout. For ElementBox children it
// recurses; for InlineTextBox children it measures each word individually
// (matching InlineFormattingContext behavior) to avoid the "sum of parts
// exceeds whole" discrepancy that causes unwanted line wraps.
func intrinsicContentWidth(box *ElementBox, isRow bool) float64 {
	cs := box.Style()
	// For row-direction flex containers, the max-content inline size is the SUM
	// of children (plus gap), matching CSS-FLEXBOX §9.9.2. For block/non-flex
	// containers it's the max of children.
	isFlexRow := cs != nil && box.EstablishesFlexFormattingContext() &&
		cs.FlexDirection != "column" && cs.FlexDirection != "column-reverse"

	total := 0.0
	maxW := 0.0
	spaceW := measureText(box, " ")
	if spaceW <= 0 {
		spaceW = measureText(box, " ")
	}
	for _, child := range box.Children() {
		if !child.IsInFlow() { continue }
		switch c := child.(type) {
			case *InlineTextBox:
				// Measure word-by-word (matching IFC behavior) so the flex item
				// width matches what InlineFormattingContext.Layout expects.
				w := measureTextWordSum(box, c.Text(), spaceW)
				if isFlexRow { total += w }
				if w > maxW { maxW = w }
		case *ElementBox:
			cw := intrinsicContentWidth(c, isRow)
			if isFlexRow { total += cw }
			if cw > maxW { maxW = cw }
		}
	}
	if isFlexRow {
		maxW = total
		// Add gap between flex items.
		if cs.Gap.Value > 0 || cs.ColumnGap.Value > 0 {
			gapV := cs.Gap.Value
			if gapV <= 0 { gapV = cs.ColumnGap.Value }
			unit := cs.Gap.Unit
			if unit == "" { unit = cs.ColumnGap.Unit }
			gap := gapV
			if unit == "em" { gap *= fontSizeOf(box) }
			if gap > 0 {
				count := 0
				for _, child := range box.Children() {
					if child.IsInFlow() { count++ }
				}
				maxW += gap * float64(count-1)
			}
		}
	}
	// Add the box's own padding + border (inline direction).
	if cs != nil {
		fs := fontSizeOf(box)
		_, p, b := computeBoxModel(box, maxW, fs)
		maxW += p.Left + p.Right + b.Left + b.Right
	}
	return maxW
}

func (c *FlexFormattingContext) distributeFreeSpace(items []*flexItem, containerMainSize float64, isRow bool) {
	totalFlexGrow := 0.0
	totalBaseSize := 0.0
	for _, it := range items {
		totalFlexGrow += it.flexGrow
		totalBaseSize += it.baseSize + it.marginMain
	}

	freeSpace := containerMainSize - totalBaseSize

	if freeSpace > 0 && totalFlexGrow > 0 {
		remaining := freeSpace
		for _, it := range items { it.frozen = it.flexGrow <= 0 }
		for remaining > 1e-3 {
			activeGrow := 0.0
			for _, it := range items {
				if !it.frozen { activeGrow += it.flexGrow }
			}
			if activeGrow <= 0 { break }
			for _, it := range items {
				if it.frozen { continue }
				share := remaining * it.flexGrow / activeGrow
				it.targetSize = it.baseSize + share
			}
			break
		}
	} else if freeSpace < 0 {
		totalFlexShrink := 0.0
		scaledBaseSum := 0.0
		for _, it := range items {
			if it.flexShrink > 0 {
				totalFlexShrink += it.flexShrink
				scaledBaseSum += it.flexShrink * it.baseSize
			}
		}
		shrinkSpace := -freeSpace
		if totalFlexShrink > 0 && scaledBaseSum > 0 {
			for _, it := range items {
				if it.flexShrink <= 0 { it.targetSize = it.baseSize; continue }
				shrink := shrinkSpace * (it.flexShrink * it.baseSize) / scaledBaseSum
				it.targetSize = it.baseSize - shrink
				if it.targetSize < 0 { it.targetSize = 0 }
			}
		} else {
			for _, it := range items { it.targetSize = it.baseSize }
		}
	} else {
		for _, it := range items { it.targetSize = it.baseSize }
	}
	for _, it := range items { it.finalMainSize = it.targetSize }
}

func (c *FlexFormattingContext) resolveCrossSizes(items []*flexItem, isRow, _, _ bool, cbWidth, cbHeight float64, state *LayoutState) {
	containerCS := c.Root().Style()
	alignItems := "stretch"
	if containerCS != nil && containerCS.AlignItems != "" {
		alignItems = containerCS.AlignItems
	}
	for _, it := range items {
		cs := it.box.Style()
		if cs == nil { continue }
		g := state.GeometryForBox(it.box)
		align := alignItems
		if cs.AlignSelf != "" && cs.AlignSelf != "auto" {
			align = cs.AlignSelf
		}
		if isRow {
			r := resolveLengthAuto(cs.Height, cbHeight, fontSizeOf(it.box))
			if !r.Auto && r.Definite {
				g.SetContentHeight(r.Value)
			} else if align == "stretch" {
				stretchH := cbHeight - it.marginCross
				if stretchH < 0 { stretchH = 0 }
				g.SetContentHeight(stretchH)
			}
		} else {
			r := resolveLengthAuto(cs.Width, cbWidth, fontSizeOf(it.box))
			if !r.Auto && r.Definite {
				g.SetContentWidth(r.Value)
			} else if align == "stretch" {
				stretchW := cbWidth - it.marginCross
				if stretchW < 0 { stretchW = 0 }
				g.SetContentWidth(stretchW)
			}
		}
	}
}

func (c *FlexFormattingContext) applyPositions(items []*flexItem, container *ElementBox, isRow, isReverse, _ bool, state *LayoutState) {
	cg := state.GeometryForBox(container)
	cx := cg.ContentBoxLeft()
	cy := cg.ContentBoxTop()
	cw := cg.ContentWidth()
	ch := cg.ContentHeight()
	containerCS := container.Style()

	mainPos := cx
	crossPos := cy
	if !isRow {
		// Column flex: main axis is Y (vertical), cross axis is X (horizontal)
		mainPos = cy
		crossPos = cx
	}
	if isReverse {
		if isRow { mainPos = cx + cw } else { mainPos = cy + ch }
	}

	// Apply justify-content by adjusting initial mainPos.
	justify := "flex-start"
	if containerCS != nil && containerCS.JustifyContent != "" {
		justify = containerCS.JustifyContent
	}
	totalMain := 0.0
	for _, it := range items {
		totalMain += it.marginMain + it.finalMainSize
	}

	// CSS gap (flex gap) between items.
	gap := 0.0
	if containerCS != nil {
		gap = flexGap(containerCS, isRow, fontSizeOf(container))
	}

	if isRow {
		if totalMain < cw && (justify == "center" || justify == "flex-end") {
			freeGap := cw - totalMain
			if justify == "center" {
				if isReverse { mainPos -= freeGap / 2 } else { mainPos += freeGap / 2 }
			} else { // flex-end
				if isReverse { mainPos -= freeGap } else { mainPos += freeGap }
			}
		}
	} else {
		if totalMain < ch && (justify == "center" || justify == "flex-end") {
			freeGap := ch - totalMain
			if justify == "center" {
				if isReverse { mainPos -= freeGap / 2 } else { mainPos += freeGap / 2 }
			} else { // flex-end
				if isReverse { mainPos -= freeGap } else { mainPos += freeGap }
			}
		}
	}
	// Pre-compute baseline offsets for align-items:baseline support.
	maxBO := 0.0
	for _, it := range items {
		align := alignOf(it.box, containerCS)
		if align == "baseline" {
			it.baselineOffset = baselineOffset(it.box, state)
			if it.baselineOffset > maxBO {
				maxBO = it.baselineOffset
			}
		}
	}

	for _, it := range items {
		g := state.GeometryForBox(it.box)
		cs := it.box.Style()
		fs := fontSizeOf(it.box)
		// Fixed main-axis margin: shift position by margin-start before item.
		if cs != nil {
			if isRow {
				mainPos += resolveOrZero(cs.MarginLeft, cw, fs)
			} else {
				mainPos += resolveOrZero(cs.MarginTop, cw, fs)
			}
		}
		// Auto main-axis margin: absorb remaining free space (CSS-FLEXBOX §9.5).
		if cs != nil && isRow && cs.MarginLeft.Unit == "auto" {
			if rem := cw - totalMain; rem > 0 { mainPos += rem }
		} else if cs != nil && !isRow && cs.MarginTop.Unit == "auto" {
			if rem := ch - totalMain; rem > 0 { mainPos += rem }
		}


		if isRow {
			ms := it.finalMainSize
			avail := cw - it.marginMain
			if ms > avail { ms = avail }
			g.SetContentWidth(ms)
			// box-sizing: border-box → convert total to content.
			if isBorderBox(it.box) {
				hp := g.PaddingLeft() + g.PaddingRight() + g.BorderLeft() + g.BorderRight()
				g.SetContentWidth(math.Max(0, ms - hp))
			}
			if cs != nil {
				r := resolveLengthAuto(cs.Height, ch, fontSizeOf(it.box))
				if r.Definite && !r.Auto { g.SetContentHeight(r.Value) }
			}
		} else {
			ms := it.finalMainSize
			avail := ch - it.marginMain
			if ms > avail { ms = avail }
			g.SetContentHeight(ms)
			// box-sizing: border-box for column flex.
			if isBorderBox(it.box) {
				vp := g.PaddingTop() + g.PaddingBottom() + g.BorderTop() + g.BorderBottom()
				g.SetContentHeight(math.Max(0, ms - vp))
			}
			if cs != nil {
				r := resolveLengthAuto(cs.Width, cw, fontSizeOf(it.box))
				if r.Definite && !r.Auto { g.SetContentWidth(r.Value) }
			}
		}

		if isRow {
			if isReverse { mainPos -= g.BorderBoxWidth() }
			// Set position FIRST so children use correct absolute coordinates.
			bh := g.BorderBoxHeight()
			if bh <= 0 {
				// Estimate intrinsic cross-size from text children for initial
				// positioning before child layout (post-layout corrects it).
				bh = fontLineGap(it.box)
			}
			align := alignOf(it.box, containerCS)
			crossAdjusted := crossPos
			// When container auto-height (ch=0), defer centering/flex-end
			// until after child layout so we can use the actual child height.
			if ch > 0 {
				switch align {
				case "center":
					crossAdjusted = crossPos + (ch-bh)/2
				case "baseline":
					crossAdjusted = crossPos + (maxBO - it.baselineOffset)
				case "flex-end":
					crossAdjusted = crossPos + ch - bh
				}
			}
			g.SetTopLeft(crossAdjusted, mainPos)
			if !isReverse { mainPos += g.BorderBoxWidth() + resolveOrZero(cs.MarginRight, cw, fs) + gap }

			ctx := contextFor(it.box, state)
			ctx.Layout(it.box, state)

			// Propagate auto cross-size from children.
			if heightIsAutoForBox(it.box) {
				oldTop := g.Top()
				bh2 := g.BorderBoxHeight()
				newCross := crossPos
				if ch > 0 {
					switch align {
					case "center":
						newCross = crossPos + (ch-bh2)/2
					case "baseline":
						newCross = crossPos + (maxBO - it.baselineOffset)
					case "flex-end":
						newCross = crossPos + ch - bh2
					}
				} else {
					// Container auto-height: center within actual child height.
					switch align {
					case "center":
						newCross = crossPos
					case "baseline":
						newCross = crossPos + (maxBO - it.baselineOffset)
					case "flex-end":
						newCross = crossPos
					}
				}
				delta := newCross - oldTop
				if delta != 0 {
					shiftBoxAndDescendants(it.box, delta, 0, state)
				}


			}
		} else {
			if isReverse { mainPos -= g.BorderBoxHeight() }
			// Set position FIRST.
			bw := g.BorderBoxWidth()
			align := alignOf(it.box, containerCS)
			crossAdjusted := crossPos
			switch align {
			case "center":
				crossAdjusted = crossPos + (cw-bw)/2
			case "baseline":
				crossAdjusted = crossPos + (maxBO - it.baselineOffset)
			case "flex-end":
				crossAdjusted = crossPos + cw - bw
			}
			g.SetTopLeft(mainPos, crossAdjusted)
			if !isReverse { mainPos += g.BorderBoxHeight() + resolveOrZero(cs.MarginBottom, cw, fs) + gap }

			ctx := contextFor(it.box, state)
			ctx.Layout(it.box, state)

			// Propagate auto cross-size (width) from children.
			oldLeft := g.Left()
			bw2 := g.BorderBoxWidth()
			newCross := crossPos
			switch align {
			case "center":
				newCross = crossPos + (cw-bw2)/2
			case "baseline":
				newCross = crossPos + (maxBO - it.baselineOffset)
			case "flex-end":
				newCross = crossPos + cw - bw2
			}
			delta := newCross - oldLeft
			if delta != 0 {
				shiftBoxAndDescendants(it.box, 0, delta, state)
			}

			// Propagate auto main-size (height) from children.
			// For column flex, main axis = Y. justify-content:center/flex-end
			// may need re-centering after child layout determines actual height.
			if heightIsAutoForBox(it.box) {
				oldTop := g.Top()
				bh2 := g.BorderBoxHeight()
				newMain := cy
				switch justify {
				case "center":
					newMain = cy + (ch-bh2)/2
				case "flex-end":
					newMain = cy + ch - bh2
				default:
					newMain = oldTop
				}
				delta2 := newMain - oldTop
				if delta2 != 0 {
					shiftBoxAndDescendants(it.box, delta2, 0, state)
				}
			}
		}
	}
}

// shiftBoxAndDescendants adds (dy, dx) to the geometry top-left position of box
// and all its layout descendants, including text segment coordinates.
func shiftBoxAndDescendants(box *ElementBox, dy, dx float64, state *LayoutState) {
	if box == nil { return }
	g := state.GeometryForBox(box)
	g.SetTopLeft(g.Top()+dy, g.Left()+dx)
	// Shift text segment positions too.
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children() {
		switch c := child.(type) {
		case *ElementBox:
			shiftBoxAndDescendants(c, dy, dx, state)
		case *InlineTextBox:
			for i := range c.TextSegments {
				c.TextSegments[i].X += dx
				c.TextSegments[i].Y += dy
			}
		}
	}
}

// maxChildContentHeight returns the maximum intrinsic border-box height of all
// in-flow children, used to determine auto cross-size from content.
// This uses the child's own computed height (not its positioned bottom edge)
// to avoid the chicken-and-egg problem where centering within height=0 gives
// a negative offset that would produce a wrong auto-height.
func maxChildContentHeight(box *ElementBox, state *LayoutState) float64 {
	maxH := 0.0
	for _, child := range box.Children() {
		if !child.IsInFlow() {
			continue
		}
		if eb, ok := child.(*ElementBox); ok {
			g := state.GeometryForBox(eb)
			bh := g.BorderBoxHeight()
			if bh > maxH {
				maxH = bh
			}
		}
	}
	return maxH
}

// alignOf returns the effective align-self value for box in its flex container.
func alignOf(box *ElementBox, containerCS *style.ComputedStyle) string {
	cs := box.Style()
	if cs != nil && cs.AlignSelf != "" && cs.AlignSelf != "auto" {
		return cs.AlignSelf
	}
	if containerCS != nil && containerCS.AlignItems != "" {
		return containerCS.AlignItems
	}
	return "stretch"
}

// flexGap returns the effective gap for the flex container.
func flexGap(cs *style.ComputedStyle, isRow bool, fontSize float64) float64 {
	var gapV float64
	if isRow {
		gapV = cs.ColumnGap.Value
		if cs.Gap.Value > 0 {
			gapV = cs.Gap.Value
		}
	} else {
		gapV = cs.RowGap.Value
		if cs.Gap.Value > 0 {
			gapV = cs.Gap.Value
		}
	}
	if cs.Gap.Unit == "em" {
		gapV *= fontSize
	}
	return gapV
}

var _ = style.DisplayFlex
var _ = math.Max

// baselineOffset returns the distance from a box's border-box top edge to its
// text baseline, used for align-items:baseline alignment in flex layout.
// For a flex container, we walk the first in-flow child to find the real
// text baseline, accounting for nested padding/border.
func baselineOffset(box *ElementBox, state *LayoutState) float64 {
	return baselineOffsetRec(box, state, 0)
}

func baselineOffsetRec(box *ElementBox, state *LayoutState, depth int) float64 {
	g := state.GeometryForBox(box)
	padTop := g.PaddingTop() + g.BorderTop()

	// For flex/grid containers, recurse into the first in-flow child.
	if depth < 10 && (box.EstablishesFlexFormattingContext() || box.EstablishesGridFormattingContext()) {
		for _, child := range box.Children() {
			if !child.IsInFlow() || child.IsAbsolutelyPositioned() {
				continue
			}
			if eb, ok := child.(*ElementBox); ok {
				return padTop + baselineOffsetRec(eb, state, depth+1)
			}
			if tb, ok := child.(*InlineTextBox); ok && len(tb.TextSegments) > 0 {
				return padTop + tb.TextSegments[0].Height
			}
			break
		}
	}

	// Default: use font ascent from this box's resolved font.
	ascent, _ := fontAscentDescent(box)
	return padTop + ascent
}
