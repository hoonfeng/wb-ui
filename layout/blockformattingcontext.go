// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
// BlockFormattingContext lays out in-flow children vertically (or horizontally in
// vertical writing mode), with margin collapse and float containment.

package layout

import (
	"math"

	"wb-ui/style"
)

type BlockFormattingContext struct {
	FormattingContextBase
}

func (c *BlockFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	if box.Style() == nil {
		// Can't set Style() through the interface; skip default assignment
	}
	g := state.GeometryForBox(box)
	if box.Parent() == nil {
		margin, padding, border := computeBoxModelForBox(box, state.ViewportWidth, fontSizeOf(box))
		g.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
		g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
		g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
		g.SetContentWidth(state.ViewportWidth - border.Horizontal() - padding.Horizontal())
		if state.ViewportHeight > 0 {
			g.SetContentHeight(state.ViewportHeight - border.Vertical() - padding.Vertical())
		}
	}

	style := box.Style()
	isVerticalWM := IsVerticalWritingMode(style)
	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	contentWidth := g.ContentWidth()

	blockStart := g.ContentBoxTop()
	if isVerticalWM {
		blockStart = g.ContentBoxLeft()
	}

	establishesBFC := box.EstablishesBlockFormattingContext()
	var fc *floatContext
	if establishesBFC {
		fc = newFloatContext(contentX, contentY, contentWidth)
		prev := state.setFloatContext(fc)
		defer state.restoreFloatContext(prev)
	} else {
		fc = state.currentFloatContext()
	}

	cursor := blockStart
	pendingMargin := 0.0
	collapseTopWithParent := !establishesBFC && g.BorderTop() == 0 && g.PaddingTop() == 0
	firstInFlow := true

	var deferredAbsolutes []*ElementBox

	for _, child := range box.Children() {
		if !child.IsVisible() {
			continue
		}
		childEb, childIsEb := child.(*ElementBox)
		if !childIsEb {
			continue
		}
		if child.IsFloated() {
			layoutFloatedChild(childEb, contentX, contentWidth, fc, state)
			continue
		}
		if child.IsAbsolutelyPositioned() {
			deferredAbsolutes = append(deferredAbsolutes, childEb)
			continue
		}

		ch := state.GeometryForBox(childEb)
		margin, padding, border := computeBoxModelForBox(childEb, contentWidth, fontSizeOf(childEb))
		ch.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
		ch.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
		ch.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

		borderBoxWidth := computeBlockChildBorderBoxWidth(childEb, contentWidth, margin, border, padding, state)
		ch.SetContentWidth(borderBoxWidth - border.Horizontal() - padding.Horizontal())
		ch.SetTopLeft(g.ContentBoxLeft()+margin.Left, 0) // Y set below

		clearSide := clearSideOf(childEb)
		if clearSide != "" && fc != nil {
			cursor = fc.clearedY(cursor, clearSide)
		}

		breakBefore := child.Style().GetProperty("break-before")
		if breakBefore == "page" || breakBefore == "always" {
			pageHeight := state.ViewportHeight
			if pageHeight > 0 {
				currentPage := math.Floor(cursor / pageHeight)
				cursor = math.Max(cursor, (currentPage+1)*pageHeight)
			}
		}

		topMargin := margin.Top
		collapsedTop := 0.0
		if firstInFlow && collapseTopWithParent {
			collapsedTop = 0
		} else {
			collapsedTop = math.Max(pendingMargin, topMargin)
		}
		cursor += collapsedTop
		ch.SetTopLeft(ch.Left(), cursor)

		cbHeight := g.ContentHeight()
		if cbHeight <= 0 && box.Parent() != nil {
			cbHeight = state.GeometryForBox(box.Parent()).ContentHeight()
		}
		cs := child.Style()
		if !heightIsAutoForBox(childEb) {
			if cbHeight > 0 {
				fs := fontSizeOf(childEb)
				hv, ok := definiteHeight(cs.Height, cbHeight, fs)
				if ok {
					if isBorderBoxForBox(childEb) {
						ch.SetContentHeight(hv - border.Vertical() - padding.Vertical())
					} else {
						ch.SetContentHeight(hv)
					}
				}
			}
		} else if cbHeight > 0 && childNeedsHeightConstraintForBox(childEb) {
			remaining := cbHeight - (cursor - g.ContentBoxTop())
			if remaining > 0 {
				ch.SetContentHeight(remaining)
			}
		}

		childCtx := contextFor(childEb, state)
		childCtx.Layout(childEb, state)

		fs := fontSizeOf(childEb)
		if isVerticalWM {
			minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, 0, fs)
			bw := ch.BorderBoxWidth()
			ch.SetContentWidth(clampSize(bw, minW, maxW, false, false) - border.Horizontal() - padding.Horizontal())
		} else {
			minH, maxH, _, _ := resolveMinMax(cs.MinHeight, cs.MaxHeight, 0, fs)
			bh := ch.BorderBoxHeight()
			ch.SetContentHeight(clampSize(bh, minH, maxH, false, false) - border.Vertical() - padding.Vertical())
		}
		pendingMargin = margin.Bottom
		cursor = ch.Top() + ch.BorderBoxHeight()

		breakAfter := child.Style().GetProperty("break-after")
		if breakAfter == "page" || breakAfter == "always" {
			pageHeight := state.ViewportHeight
			if pageHeight > 0 {
				currentPage := math.Floor(cursor / pageHeight)
				cursor = math.Max(cursor, (currentPage+1)*pageHeight)
			}
		}
		firstInFlow = false
	}

	// Resolve box block size (height for horizontal-tb, width for vertical WM).
	cs := box.Style()
	if heightIsAutoForBox(box) {
		blockSize := cursor - blockStart
		if !establishesBFC && g.BorderBottom() == 0 && g.PaddingBottom() == 0 {
			// pendingMargin collapses out
		} else {
			blockSize += pendingMargin
		}
		if establishesBFC && fc != nil {
			if fb := fc.maxFloatBottom(); fb > contentY+blockSize {
				blockSize = fb - contentY
			}
		}
		if blockSize < 0 {
			blockSize = 0
		}
		g.SetContentHeight(blockSize)
	} else if box.Parent() != nil {
		fs := fontSizeOf(box)
		cbHeight := state.GeometryForBox(box.Parent()).ContentHeight()
		hv, ok := definiteHeight(cs.Height, cbHeight, fs)
		if ok {
			if isBorderBoxForBox(box) {
				g.SetContentHeight(hv - g.VerticalBorder() - g.VerticalPadding())
			} else {
				g.SetContentHeight(hv)
			}
		}
	}

	// Apply relative offsets to in-flow children.
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok {
			if childEb.IsRelativelyPositioned() && childEb.IsInFlow() {
				applyRelativeOffsetForBox(childEb, contentWidth, g.ContentHeight(), state)
			}
		}
	}

	// Lay out absolutely-positioned descendants.

	root := stateRootForBox(box)
	for _, child := range deferredAbsolutes {
		cb := containingBlockForAbsolute(child, root)
		layoutAbsolute(child, cb, root, state)
	}
}

// computeBlockChildBorderBoxWidth resolves border-box width of a block child.
func computeBlockChildBorderBoxWidth(child *ElementBox, cbContentWidth float64, margin, border, padding Edges, state *LayoutState) float64 {
	fs := fontSizeOf(child)
	cs := child.Style()
	w, ok := definiteWidth(cs.Width, cbContentWidth, fs)
	if !ok {
		width := cbContentWidth - margin.Horizontal()
		if width < 0 { width = 0 }
		minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
		return clampSize(width, minW, maxW, false, false)
	}
	var borderBox float64
	if isBorderBoxForBox(child) {
		borderBox = w
	} else {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
	if !isBorderBoxForBox(child) {
		minW += border.Horizontal() + padding.Horizontal()
		if maxW > 0 { maxW += border.Horizontal() + padding.Horizontal() }
	}
	return clampSize(borderBox, minW, maxW, false, false)
}

func layoutFloatedChild(child *ElementBox, contentX, contentWidth float64, fc *floatContext, state *LayoutState) {
	if fc == nil {
		fc = newFloatContext(contentX, 0, contentWidth)
	}
	ch := state.GeometryForBox(child)
	margin, padding, border := computeBoxModelForBox(child, contentWidth, fontSizeOf(child))
	ch.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
	ch.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	ch.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	cs := child.Style()
	fs := fontSizeOf(child)
	w, ok := definiteWidth(cs.Width, contentWidth, fs)
	if !ok {
		w = contentWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
		if w < 0 { w = 0 }
	}
	borderBox := w
	if !isBorderBoxForBox(child) {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	ch.SetContentWidth(borderBox - border.Horizontal() - padding.Horizontal())

	isLeft := cs.Float != "right"
	x, y := fc.placeFloat(child, isLeft, borderBox, 0)
	ch.SetTopLeft(x, y)

	childCtx := contextFor(child, state)
	childCtx.Layout(child, state)

	for i := range fc.floats {
		if fc.floats[i].box == child {
			fc.floats[i].h = ch.BorderBoxHeight()
			fc.floats[i].w = borderBox
			break
		}
	}
}

func clearSideOf(box *ElementBox) string {
	if box.Style() == nil { return "" }
	return box.Style().Clear
}

func heightIsAutoForBox(box *ElementBox) bool {
	if box.Style() == nil { return true }
	r := resolveLengthAuto(box.Style().Height, 0, 0)
	return r.Auto
}

func childNeedsHeightConstraintForBox(box *ElementBox) bool {
	if box == nil || box.Style() == nil { return false }
	cs := box.Style()
	if cs.Display == style.DisplayFlex || cs.Display == style.DisplayInlineFlex {
		fd := cs.FlexDirection
		if fd == "column" || fd == "column-reverse" { return true }
	}
	if cs.Display == style.DisplayGrid || cs.Display == style.DisplayInlineGrid { return true }
	return false
}

func stateRootForBox(box *ElementBox) *ElementBox {
	cur := box
	for cur.Parent() != nil { cur = cur.Parent() }
	return cur
}

