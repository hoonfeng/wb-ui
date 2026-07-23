// Flex formatting context — lays out children in a flex container.
// Translation of: Source/WebCore/layout/formattingContexts/flex/FlexFormattingContext.cpp

package layout

import (
	"math"
	"sort"

	"wb-ui/style"
)

type FlexFormattingContext struct{}

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
}

func (c *FlexFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	isRow := cs.FlexDirection != "column" && cs.FlexDirection != "column-reverse"
	isReverse := cs.FlexDirection == "row-reverse" || cs.FlexDirection == "column-reverse"

	g := state.GeometryForBox(box)
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
	}

	c.distributeFreeSpace(items, mainSize, isRow)
	c.resolveCrossSizes(items, isRow, isReverse, false, cw, ch, state)
	c.applyPositions(items, box, isRow, isReverse, false, state)
}

func (c *FlexFormattingContext) resolveItem(box *ElementBox, isRow bool, cbWidth, cbHeight float64, state *LayoutState) *flexItem {
	cs := box.Style()
	_ = state.GeometryForBox(box)
	margin, _, _ := computeBoxModel(box, cbWidth, fontSizeOf(box))

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
	if base <= 0 { base = 150 }
	if isRow {
		return clampSize(base, it.minWidth, it.maxWidth, it.minWidth <= 0, it.maxWidth <= 0)
	}
	return clampSize(base, it.minHeight, it.maxHeight, it.minHeight <= 0, it.maxHeight <= 0)
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
	for _, it := range items {
		cs := it.box.Style()
		if cs == nil { continue }
		if isRow {
			r := resolveLengthAuto(cs.Height, cbHeight, fontSizeOf(it.box))
			if !r.Auto && r.Definite { state.GeometryForBox(it.box).SetContentHeight(r.Value) }
		} else {
			r := resolveLengthAuto(cs.Width, cbWidth, fontSizeOf(it.box))
			if !r.Auto && r.Definite { state.GeometryForBox(it.box).SetContentWidth(r.Value) }
		}
	}
}

func (c *FlexFormattingContext) applyPositions(items []*flexItem, container *ElementBox, isRow, isReverse, _ bool, state *LayoutState) {
	cg := state.GeometryForBox(container)
	cx := cg.ContentBoxLeft()
	cy := cg.ContentBoxTop()
	cw := cg.ContentWidth()
	ch := cg.ContentHeight()

	mainPos := cx
	crossPos := cy
	if isReverse {
		if isRow { mainPos = cx + cw } else { mainPos = cy + ch }
	}

	for _, it := range items {
		g := state.GeometryForBox(it.box)
		cs := it.box.Style()

		if isRow {
			ms := it.finalMainSize
			avail := cw - it.marginMain
			if ms > avail { ms = avail }
			g.SetContentWidth(ms)
			if cs != nil {
				r := resolveLengthAuto(cs.Height, ch, fontSizeOf(it.box))
				if r.Definite && !r.Auto { g.SetContentHeight(r.Value) }
			}
		} else {
			ms := it.finalMainSize
			avail := ch - it.marginMain
			if ms > avail { ms = avail }
			g.SetContentHeight(ms)
			if cs != nil {
				r := resolveLengthAuto(cs.Width, cw, fontSizeOf(it.box))
				if r.Definite && !r.Auto { g.SetContentWidth(r.Value) }
			}
		}

		if isRow {
			if isReverse { mainPos -= g.BorderBoxWidth() }
			g.SetTopLeft(mainPos, crossPos)
			if !isReverse { mainPos += g.BorderBoxWidth() }
		} else {
			if isReverse { mainPos -= g.BorderBoxHeight() }
			g.SetTopLeft(crossPos, mainPos)
			if !isReverse { mainPos += g.BorderBoxHeight() }
		}

		ctx := contextFor(it.box)
		ctx.Layout(it.box, state)
	}
}

var _ = style.DisplayFlex
var _ = math.Max
