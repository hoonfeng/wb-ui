// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
// BlockFormattingContext lays out in-flow children vertically (or horizontally in
// vertical writing mode), with margin collapse and float containment.

package layout

import (
	"fmt"
	"math"
	"os"

	"wb-ui/style"
)

var diagFloats *os.File

func init() {
	diagFloats, _ = os.Create("diag_floats.txt")
	diagFloats.WriteString("=== float layout diagnostics ===\n")
}

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
		// Only BFC-establishing boxes create their own float context.
		// Non-BFC boxes inherit the parent's FC, so floats placed by
		// siblings are visible to inline content for text wrapping.
		fc = newFloatContext(contentX, contentY, contentWidth)
		prev := state.setFloatContext(fc)
		defer state.restoreFloatContext(prev)
	} else {
		fc = state.currentFloatContext()
		if fc == nil {
			fc = newFloatContext(contentX, contentY, contentWidth)
			state.setFloatContext(fc)
		}
	}

	// DIAG: log container geometry for boxes with floats
	for _, ch := range box.Children() {
		if eb, ok := ch.(*ElementBox); ok && eb.IsFloated() {
			fmt.Fprintf(diagFloats, "parent=%q contentX=%.0f contentY=%.0f contentWidth=%.0f bfc=%t\n",
				box.Style().Display, contentX, contentY, contentWidth, establishesBFC)
			break
		}
	}

	cursor := blockStart
	pendingMargin := 0.0
	collapseTopWithParent := !establishesBFC && g.BorderTop() == 0 && g.PaddingTop() == 0
	firstInFlow := true

	fmt.Fprintf(diagFloats, "> layoutBlockChildren: box=%s contentX=%.0f contentY=%.0f contentWidth=%.0f childCount=%d\n",
		elementName(box), contentX, contentY, contentWidth, len(box.Children()))

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
			layoutFloatedChild(childEb, contentX, contentY, contentWidth, fc, state)
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
		ch.SetTopLeft(0, g.ContentBoxLeft()+margin.Left) // Y set below

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
			collapsedTop = topMargin
		} else {
			collapsedTop = math.Max(pendingMargin, topMargin)
		}
		cursor += collapsedTop
		ch.SetTopLeft(cursor, ch.Left())

		cbHeight := g.ContentHeight()
		if cbHeight <= 0 && box.Parent() != nil {
			cbHeight = state.GeometryForBox(box.Parent()).ContentHeight()
		}
		if cbHeight <= 0 {
			cbHeight = 0 // will still try to apply definite height below
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
			} else {
				// Even when cbHeight is 0 (parent not yet sized), apply a
				// definite px/em height from CSS. This is critical for replaced
				// elements (input, select) whose height is set via CSS.
				fs := fontSizeOf(childEb)
				hv, ok := definiteHeight(cs.Height, 100, fs)
				if ok && hv > 0 {
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
			minW, maxW, minWAuto, maxWAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, 0, fs)
			bw := ch.BorderBoxWidth()
			ch.SetContentWidth(clampSize(bw, minW, maxW, minWAuto, maxWAuto) - border.Horizontal() - padding.Horizontal())
		} else {
			minH, maxH, minHAuto, maxHAuto := resolveMinMax(cs.MinHeight, cs.MaxHeight, 0, fs)
			bh := ch.BorderBoxHeight()
			ch.SetContentHeight(clampSize(bh, minH, maxH, minHAuto, maxHAuto) - border.Vertical() - padding.Vertical())
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
		// Even without BFC, expand height to encompass this container's own
		// float children (clearfix behavior). Without this, a container whose
		// only children are floats would have zero height.
		if !establishesBFC {
			maxFloatBottom := 0.0
			for _, child := range box.Children() {
				if eb, ok := child.(*ElementBox); ok && eb.IsFloated() {
					ch := state.GeometryForBox(eb)
					if b := ch.Top() + ch.BorderBoxHeight(); b > maxFloatBottom {
						maxFloatBottom = b
					}
				}
			}
			if maxFloatBottom > contentY+blockSize {
				blockSize = maxFloatBottom - contentY
			}
		}
		if blockSize < 0 {
			blockSize = 0
		}
		// Preserve any height already set by parent formatting context (e.g. flex cross-axis stretch).
		// Only for non-root boxes — root uses viewport as initial height which must be replaced.
		if box.Parent() != nil && blockSize < g.ContentHeight() {
			blockSize = g.ContentHeight()
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

	// If this replaced element still has 0 content height (no children),
	// apply a minimum intrinsic height so it's visible. This handles
	// <input>, <select>, and other replaced elements without explicit
	// CSS height that are laid out directly via BFC (from IFC).
	if box.IsReplaced() && g.ContentHeight() <= 0 && heightIsAutoForBox(box) {
		fs := fontSizeOf(box)
		if fs <= 0 { fs = 16 }
		lineH := fontLineGap(box)
		if lineH <= 0 { lineH = fs * 1.2 }
		g.SetContentHeight(lineH)
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
		minW, maxW, minAuto, maxAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
		return clampSize(width, minW, maxW, minAuto, maxAuto)
	}
	var borderBox float64
	if isBorderBoxForBox(child) {
		borderBox = w
	} else {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	minW, maxW, minAuto, maxAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
	if !isBorderBoxForBox(child) {
		minW += border.Horizontal() + padding.Horizontal()
		if maxAuto || maxW > 0 {
			maxW += border.Horizontal() + padding.Horizontal()
		}
	}
	return clampSize(borderBox, minW, maxW, minAuto, maxAuto)
}

func layoutFloatedChild(child *ElementBox, contentX, contentY, contentWidth float64, fc *floatContext, state *LayoutState) {
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
	fmt.Fprintf(diagFloats, "  child=%q float=%s widthCSS=%v contentWidth=%.0f definiteW=%.0f ok=%t el=%s\n",
		cs.Display, cs.Float, cs.Width, contentWidth, w, ok, elementName(child))
	borderBox := w
	if !isBorderBoxForBox(child) {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	ch.SetContentWidth(borderBox - border.Horizontal() - padding.Horizontal())

	isLeft := cs.Float != "right"
	// Use container's FC-relative y as startY so placeFloat's collision
	// detection correctly sees sibling floats at the same y level.
	fcY := contentY - fc.originY
	// Include margin in the width passed to placeFloat so subsequent floats
	// are spaced apart by their margins (not placing right against each other).
	marginBoxW := borderBox + margin.Horizontal()
	x, y := fc.placeFloat(child, isLeft, marginBoxW, 0, fcY)
	// Adjust x for coordinate offset between FC origin and container content box,
	// plus margin-left so the float's border box starts at the correct position.
	// y from placeFloat is already FC-relative (starts at fcY). Convert to
	// absolute by adding fc.originY so it matches the page coordinate system.
	x += contentX - fc.originX + margin.Left
	y += fc.originY

	// Clamp float position to container content area when using inherited FC.
	// Without a BFC, the inherited FC may have wider bounds (viewport width)
	// causing floats to escape the container. This keeps them visually contained.
	// Use marginBoxW for clamping because margin was already added to x.
	rightEdge := contentX + contentWidth
	if x+borderBox > rightEdge && rightEdge > contentX {
		x = rightEdge - borderBox
	}
	if x < contentX {
		x = contentX
	}
	ch.SetTopLeft(y, x)

	childCtx := contextFor(child, state)
	childCtx.Layout(child, state)

	for i := range fc.floats {
		if fc.floats[i].box == child {
			fc.floats[i].h = ch.BorderBoxHeight()
			fc.floats[i].w = marginBoxW // store margin-box width for correct collision detection
			// Update the FC-relative y to match the actual rendered position.
			// placeFloat starts at fc.originY, but the rendered y after the
			// contentY - fc.originY adjustment may differ. Keeping the FC y
			// correct ensures IFC queries (contentEdgesAt) find floats at
			// the right vertical position.
			// NOTE: FC x is NOT updated here because placeFloat's collision
			// detection uses FC-relative x (before contentX adjustment).
			fc.floats[i].y = ch.Top() - fc.originY
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

func elementName(box *ElementBox) string {
	if box == nil { return "nil" }
	el := box.Element()
	if el == nil { return "anonymous" }
	return fmt.Sprintf("%v", el)
}

