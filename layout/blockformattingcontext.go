// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/block/BlockMarginCollapse.cpp
//                  Source/WebCore/layout/formattingContexts/block/BlockFormattingGeometry.cpp
// Completeness: 60%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - margin collapse uses a simplified rule: adjacent sibling margins collapse to
//     the max; first-child top margin collapses with parent top margin when the parent
//     has no border/padding-top; last-child bottom margin collapses with parent bottom
//     margin when the parent height is auto and there is no border/padding-bottom
//   - BFC establishment is detected via LayoutBox.establishesBlockFormattingContext;
//     a BFC root contains its floats (height grows to enclose them)
//   - inline-level children are wrapped into anonymous block boxes by BuildLayoutTree,
//     so the block formatting context only sees block-level children directly; the
//     anonymous wrapper dispatches to InlineFormattingContext for its inline children
//   - clear is applied by advancing the cursor past matching floats
//   - relative positioning is applied after in-flow layout

package layout

import (
	"math"

	"wb-ui/style"
)

// BlockFormattingContext is the Go translation of WebCore::Layout::BlockFormattingContext.
// It lays out a block container's in-flow children vertically, applying margin collapse
// and containing floats when the root establishes a new BFC.
type BlockFormattingContext struct{}

// Layout lays out box's in-flow descendants. The caller is responsible for box's own
// border-box position (X/Y) and width (and margin/padding/border) before invoking; for
// the root box these are set by the top-level Layout function. If box's height is auto
// it is computed from the in-flow content.
func (c *BlockFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	// The root box (no parent) has its box model resolved here since no parent did it.
	// stretchRootToViewport set the border-box size to the viewport but could not
	// subtract padding/border (they were not yet resolved).
	if box.parent == nil {
		margin, padding, border := computeBoxModel(box, state.ViewportWidth, fontSizeOf(box))
		box.Rect.Margin = margin
		box.Rect.Padding = padding
		box.Rect.Border = border
		box.Rect.Width = state.ViewportWidth
		box.Rect.Height = state.ViewportHeight
	}
	contentX := box.Rect.ContentX()
	contentY := box.Rect.ContentY()
	contentWidth := box.Rect.ContentWidth()

	// A BFC root contains its floats and does not collapse its margins with children.
	establishesBFC := box.establishesBlockFormattingContext()
	var fc *floatContext
	if establishesBFC {
		fc = newFloatContext(contentX, contentY, contentWidth)
		prev := state.setFloatContext(fc)
		defer state.restoreFloatContext(prev)
	} else {
		fc = state.currentFloatContext()
	}

	// Lay out in-flow children vertically, applying simplified margin collapse.
	cursor := contentY
	// pendingMargin holds the uncollapsed bottom margin of the previous in-flow
	// sibling that has yet to be applied to the next sibling's top.
	pendingMargin := 0.0
	// collapseTopWithParent tracks whether the first in-flow child's top margin
	// collapses with the parent's top margin (only when parent has no
	// border/padding-top and does not establish a BFC).
	collapseTopWithParent := !establishesBFC &&
		box.Rect.Border.Top == 0 && box.Rect.Padding.Top == 0
	firstInFlow := true

	var deferredAbsolutes []*LayoutBox

	for _, child := range box.Children {
		if !child.IsVisible() {
			continue
		}
		if child.IsFloated() {
			layoutFloatedChild(child, contentX, contentWidth, fc, state)
			continue
		}
		if child.IsAbsolutelyPositioned() {
			deferredAbsolutes = append(deferredAbsolutes, child)
			continue
		}

		margin, padding, border := computeBoxModel(child, contentWidth, fontSizeOf(child))
		child.Rect.Margin = margin
		child.Rect.Padding = padding
		child.Rect.Border = border

		// Width: block boxes fill the containing block unless a width is specified.
		borderBoxWidth := computeBlockChildBorderBoxWidth(child, contentWidth, margin, border, padding)
		child.Rect.Width = borderBoxWidth
		child.Rect.X = contentX + margin.Left

		// Vertical position with margin collapse.
		clearSide := clearSideOf(child)
		if clearSide != "" && fc != nil {
			cursor = fc.clearedY(cursor, clearSide)
		}

		topMargin := margin.Top
		// Collapse previous sibling's bottom margin with this child's top margin.
		collapsedTop := 0.0
		if firstInFlow && collapseTopWithParent {
			// First child's top margin collapses with parent's top margin: do not
			// add it to the cursor (it escapes the parent). The parent's own
			// margin-top is applied by the grandparent.
			collapsedTop = 0
		} else {
			collapsedTop = math.Max(pendingMargin, topMargin)
		}
		cursor += collapsedTop
		child.Rect.Y = cursor

		// Lay out child's descendants. The child's own width/position are set above;
		// the child's formatting context lays out its children and computes its
		// height if auto.
		childCtx := contextFor(child)
		childCtx.Layout(child, state)

		// Determine the child's bottom margin collapse eligibility. A child whose
		// bottom margin would collapse with the parent's bottom margin must have an
		// auto height and no following in-flow content; this is handled when sizing
		// the parent height below. Here we just record the pending margin.
		pendingMargin = margin.Bottom
		cursor = child.Rect.Y + child.Rect.Height
		firstInFlow = false
	}

	// Resolve box height. box.Rect.Height stores the border-box height.
	if heightIsAuto(box) {
		// Auto height: the parent grows to enclose the last in-flow child's bottom
		// edge (content height), then border-box = content + padding + border.
		h := cursor - contentY
		// Margin collapse: the last in-flow child's bottom margin collapses with the
		// parent's bottom margin when the parent has no border/padding-bottom and
		// height is auto. In that case the margin does not contribute to height.
		if !establishesBFC && box.Rect.Border.Bottom == 0 && box.Rect.Padding.Bottom == 0 {
			// pendingMargin collapses out; do not add it.
		} else {
			h += pendingMargin
		}
		// A BFC root encloses its floats.
		if establishesBFC && fc != nil {
			if fb := fc.maxFloatBottom(); fb > contentY+h {
				h = fb - contentY
			}
		}
		if h < 0 {
			h = 0
		}
		box.Rect.Height = h + box.Rect.Border.Vertical() + box.Rect.Padding.Vertical()
	} else {
		// Specified height: resolve against the containing-block height. The root
		// box already has its height set to the viewport; for children the specified
		// value is used (content-box for content-box sizing, border-box otherwise).
		if box.parent != nil {
			fs := fontSizeOf(box)
			cbHeight := box.parent.Rect.ContentHeight()
			hv, ok := definiteHeight(box.Style.Height, cbHeight, fs)
			if ok {
				if isBorderBox(box) {
					box.Rect.Height = hv
				} else {
					box.Rect.Height = hv + box.Rect.Border.Vertical() + box.Rect.Padding.Vertical()
				}
			}
		}
	}

	// Apply relative offsets to in-flow children (does not affect layout flow).
	for _, child := range box.Children {
		if child.IsRelativelyPositioned() && child.IsInFlow() {
			applyRelativeOffset(child, contentWidth, box.Rect.ContentHeight())
		}
	}

	// Lay out absolutely-positioned descendants against their containing block.
	root := stateRoot(box)
	for _, child := range deferredAbsolutes {
		cb := containingBlockForAbsolute(child, root)
		layoutAbsolute(child, cb, root, state)
	}
}

// computeBlockChildBorderBoxWidth resolves the border-box width of a block-level
// child against the containing-block content width. For auto width the child fills
// the container (minus its margins); for a specified width the box-sizing property
// decides whether the value is the content-box or border-box width. min/max are
// applied.
func computeBlockChildBorderBoxWidth(child *LayoutBox, cbContentWidth float64, margin, border, padding Edges) float64 {
	fs := fontSizeOf(child)
	w, ok := definiteWidth(child.Style.Width, cbContentWidth, fs)
	if !ok {
		// auto: border-box fills the container minus horizontal margins.
		width := cbContentWidth - margin.Horizontal()
		if width < 0 {
			width = 0
		}
		minW, maxW, minAuto, maxAuto := resolveMinMax(child.Style.MinWidth, child.Style.MaxWidth, cbContentWidth, fs)
		return clampSize(width, minW, maxW, minAuto, maxAuto)
	}
	var borderBox float64
	if isBorderBox(child) {
		borderBox = w
	} else {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	minW, maxW, minAuto, maxAuto := resolveMinMax(child.Style.MinWidth, child.Style.MaxWidth, cbContentWidth, fs)
	// min/max for content-box sizing refer to the content-box; convert.
	if !isBorderBox(child) {
		minW += border.Horizontal() + padding.Horizontal()
		if !maxAuto {
			maxW += border.Horizontal() + padding.Horizontal()
		}
	}
	return clampSize(borderBox, minW, maxW, minAuto, maxAuto)
}

// layoutFloatedChild positions a floated child within the active float context and
// lays out its content. The float's width is computed (shrink-to-fit for auto) and
// its border-box position is recorded so subsequent in-flow content can avoid it.
func layoutFloatedChild(child *LayoutBox, contentX, contentWidth float64, fc *floatContext, state *LayoutState) {
	if fc == nil {
		// No float context (should not happen for in-flow floats): lay out inline.
		fc = newFloatContext(contentX, 0, contentWidth)
	}
	margin, padding, border := computeBoxModel(child, contentWidth, fontSizeOf(child))
	child.Rect.Margin = margin
	child.Rect.Padding = padding
	child.Rect.Border = border

	fs := fontSizeOf(child)
	w, ok := definiteWidth(child.Style.Width, contentWidth, fs)
	if !ok {
		// Shrink-to-fit: approximate max-content by the container width.
		w = contentWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
		if w < 0 {
			w = 0
		}
	}
	borderBox := w
	if !isBorderBox(child) {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	child.Rect.Width = borderBox

	// Place the float at the top of the current content area; the float context
	// resolves overlaps with existing floats.
	isLeft := child.Style.Float != "right"
	x, y := fc.placeFloat(child, isLeft, borderBox, 0)
	child.Rect.X = x
	child.Rect.Y = y

	// Lay out the float's content to determine its height.
	childCtx := contextFor(child)
	childCtx.Layout(child, state)

	// Update the placed float's height now that content is laid out.
	for i := range fc.floats {
		if fc.floats[i].box == child {
			fc.floats[i].h = child.Rect.Height
			fc.floats[i].w = borderBox
			break
		}
	}
}

// clearSideOf returns the clear side ("left"/"right"/"both"/"") for box.
func clearSideOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.Clear
}

// heightIsAuto reports whether box has an auto (unset) height.
func heightIsAuto(box *LayoutBox) bool {
	if box.Style == nil {
		return true
	}
	r := resolveLengthAuto(box.Style.Height, 0, 0)
	return r.Auto
}

// stateRoot walks the parent chain to find the layout root (the box with no parent).
// It is used as the initial containing block for fixed positioning and as the fallback
// containing block for absolutes with no positioned ancestor.
func stateRoot(box *LayoutBox) *LayoutBox {
	cur := box
	for cur.parent != nil {
		cur = cur.parent
	}
	return cur
}
