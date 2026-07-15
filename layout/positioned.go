// Translation of: Source/WebCore/rendering/RenderBox.cpp (positioned layout parts)
//                  Source/WebCore/rendering/RenderBoxModelObject.cpp (relative offsets)
//                  Source/WebCore/layout/formattingContexts/block/BlockMarginCollapse.cpp
// Completeness: 45%
// Simplifications:
//   - no subpixel layout (integer pixels only)
//   - no pagination/fragmentation
//   - WebKit resolves out-of-flow boxes through a static/absolute/fixed positioner
//     that walks the containing-block chain; this port folds the cases into a small
//     set of free functions invoked by the block formatting context after in-flow
//     layout completes
//   - sticky positioning is treated as relative for layout purposes (the sticky
//     constraint is not applied since there is no scroll)
//   - percentage offsets resolve against the containing-block padding box (per spec)
//   - z-index ordering is not applied; out-of-flow boxes are painted in DOM order

package layout

import (
	"wb-ui/dom"
	"wb-ui/style"
)

// containingBlockForAbsolute returns the nearest ancestor box that establishes a
// containing block for absolutely-positioned descendants. Per CSS 2.1 this is the
// nearest ancestor with position != static (relative/absolute/fixed/sticky). When
// none is found the initial containing block (the viewport, represented by the root
// box) is used.
func containingBlockForAbsolute(box *LayoutBox, root *LayoutBox) *LayoutBox {
	for cur := box; cur != nil; cur = cur.parent {
		if cur == root {
			return root
		}
		if cur.Style == nil {
			continue
		}
		switch cur.Style.Position {
		case style.PositionRelative, style.PositionAbsolute,
			style.PositionFixed, style.PositionSticky:
			return cur
		}
	}
	return root
}

// parent field is added on LayoutBox via a helper walk: since LayoutBox has no parent
// pointer, the block formatting context passes a chain. To keep this file decoupled,
// positionAbsoluteBox takes the containing block explicitly and the caller resolves
// the chain. The relativeOffset helper below also takes the containing block.

// resolveOffset resolves a single inset (top/right/bottom/left) for an absolutely
// positioned box. It returns the pixel value and whether it is "auto" (unset).
func resolveOffset(l style.Length, cbSize float64) (float64, bool) {
	r := resolveLengthAuto(l, cbSize, 0)
	if r.Auto {
		return 0, true
	}
	return r.Value, r.Definite && !r.Auto
}

// layoutAbsolute sizes and positions an absolutely (or fixed) positioned box against
// its containing block. cb is the containing-block box whose padding box defines the
// reference; root is the layout root used as the initial containing block for fixed
// positioning. The function lays out box content and writes its border-box origin and
// size onto box.Rect.
func layoutAbsolute(box *LayoutBox, cb *LayoutBox, root *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	// Reference size for percentage offsets/widths is the containing-block padding
	// box (CSS 2.1 10.1). For the initial containing block (root) use the viewport.
	cbWidth, cbHeight := cbContentBoxSize(cb, root, state)

	margin, padding, border := computeBoxModel(box, cbWidth, fontSizeOf(box))
	box.Rect.Padding = padding
	box.Rect.Border = border

	// Resolve width.
	width, wAuto := resolveOffset(box.Style.Width, cbWidth)
	height, hAuto := resolveOffset(box.Style.Height, cbHeight)
	minW, maxW, minWAuto, maxWAuto := resolveMinMax(box.Style.MinWidth, box.Style.MaxWidth, cbWidth, fontSizeOf(box))
	minH, maxH, minHAuto, maxHAuto := resolveMinMax(box.Style.MinHeight, box.Style.MaxHeight, cbHeight, fontSizeOf(box))

	if wAuto {
		// Shrink-to-fit: use the preferred (max-content) width clamped to the
		// available width. Without intrinsic sizing we fall back to filling the
		// containing block minus insets/margins.
		width = shrinkToFitWidth(box, cbWidth, margin, border, padding)
	}
	if isBorderBox(box) {
		width = clampSize(width, minW, maxW, minWAuto, maxWAuto)
	} else {
		width -= border.Horizontal() + padding.Horizontal()
		width = clampSize(width, minW, maxW, minWAuto, maxWAuto)
	}
	box.Rect.Width = width

	// Horizontal position: left + margin-left + ... + right against the containing
	// block. If both left and right are auto, default to the static position (after
	// the previous sibling), approximated here as the containing-block content origin.
	left, leftAuto := resolveOffset(asLength(box.Style.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(box.Style.Properties["right"]), cbWidth)
	x := cb.Rect.ContentX()
	switch {
	case !leftAuto && !rightAuto:
		// Over-constrained: honour left (LTR) and ignore right.
		x = cb.Rect.ContentX() + left + margin.Left
	case !leftAuto:
		x = cb.Rect.ContentX() + left + margin.Left
	case !rightAuto:
		x = cb.Rect.ContentX() + cbWidth - right - margin.Right - box.Rect.Width
	default:
		x = cb.Rect.ContentX() + margin.Left
	}
	box.Rect.X = x
	box.Rect.Margin = margin

	// Vertical size.
	if hAuto {
		height = layoutAbsoluteHeight(box, state)
	} else if !isBorderBox(box) {
		height -= border.Vertical() + padding.Vertical()
	}
	height = clampSize(height, minH, maxH, minHAuto, maxHAuto)
	box.Rect.Height = height

	// Vertical position.
	top, topAuto := resolveOffset(asLength(box.Style.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(box.Style.Properties["bottom"]), cbHeight)
	y := cb.Rect.ContentY()
	switch {
	case !topAuto && !bottomAuto:
		y = cb.Rect.ContentY() + top + margin.Top
	case !topAuto:
		y = cb.Rect.ContentY() + top + margin.Top
	case !bottomAuto:
		y = cb.Rect.ContentY() + cbHeight - bottom - margin.Bottom - box.Rect.Height
	default:
		y = cb.Rect.ContentY() + margin.Top
	}
	box.Rect.Y = y

	// Lay out content against the resolved border box.
	layoutBoxContent(box, state)
}

// cbContentBoxSize returns the content-box size of the containing block for an
// out-of-flow box. For the initial containing block (== root) the viewport size is
// used; otherwise the containing-block content box is used.
func cbContentBoxSize(cb *LayoutBox, root *LayoutBox, state *LayoutState) (float64, float64) {
	if cb == root {
		return state.ViewportWidth, state.ViewportHeight
	}
	return cb.Rect.ContentWidth(), cb.Rect.ContentHeight()
}

// shrinkToFitWidth computes the shrink-to-fit width of an auto-width absolutely
// positioned box. Without intrinsic sizing this approximates the preferred width as
// the content width of the containing block minus the horizontal insets and margins.
func shrinkToFitWidth(box *LayoutBox, cbWidth float64, margin, border, padding Edges) float64 {
	avail := cbWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
	if avail < 0 {
		return 0
	}
	return avail
}

// layoutAbsoluteHeight lays out box content and returns the resulting content height
// plus padding/border (the border-box height for an auto-height absolute box).
func layoutAbsoluteHeight(box *LayoutBox, state *LayoutState) float64 {
	layoutBoxContent(box, state)
	h := box.Rect.Height
	if h == 0 {
		// No block children produced height; use the inline content height.
		h = box.Rect.ContentHeight() + box.Rect.Padding.Vertical() + box.Rect.Border.Vertical()
	}
	return h
}

// layoutBoxContent lays out box children by dispatching to the formatting context
// appropriate for box display. It is shared by the absolute-positioner and the table
// cell layout so the inner content fills the resolved border box.
func layoutBoxContent(box *LayoutBox, state *LayoutState) {
	ctx := contextFor(box)
	ctx.Layout(box, state)
}

// applyRelativeOffset shifts a relatively-positioned box by its top/right/bottom/left
// offsets without affecting its in-flow position. The offset is resolved against the
// containing block (the parent content box) and applied after in-flow layout has
// placed the box. This mirrors RenderBoxModelObject::relativePositionOffsetX/Y.
func applyRelativeOffset(box *LayoutBox, cbWidth, cbHeight float64) {
	if box.Style == nil {
		return
	}
	left, leftAuto := resolveOffset(asLength(box.Style.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(box.Style.Properties["right"]), cbWidth)
	if !leftAuto {
		box.Rect.X += left
	} else if !rightAuto {
		box.Rect.X -= right
	}
	top, topAuto := resolveOffset(asLength(box.Style.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(box.Style.Properties["bottom"]), cbHeight)
	if !topAuto {
		box.Rect.Y += top
	} else if !bottomAuto {
		box.Rect.Y -= bottom
	}
}

// resolveOffsetsFromElementAttributes is a no-op kept for API symmetry with the DOM
// path; absolutely positioned boxes read their insets from ComputedStyle.Properties
// since the resolver stores non-typed properties there.
func resolveOffsetsFromElementAttributes(_ *dom.Element) {}