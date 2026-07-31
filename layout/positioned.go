// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
//   (out-of-flow layout helpers) + Source/WebCore/rendering/RenderBox.cpp
//   (relative positioning)
//
// Absolute and relative positioning helpers adapted for Box interface + BoxGeometry.

package layout

import (
	"wb-ui/dom"
	"wb-ui/style"
)

// containingBlockForAbsolute walks from box's parent to find the nearest positioned
// ancestor (or root). Returns root if none found.
func containingBlockForAbsolute(box *ElementBox, root *ElementBox) *ElementBox {
	for cur := box.Parent(); cur != nil; cur = cur.Parent() {
		if cur == root {
			return root
		}
		cs := cur.Style()
		if cs == nil {
			continue
		}
		switch cs.Position {
		case style.PositionRelative, style.PositionAbsolute,
			style.PositionFixed, style.PositionSticky:
			return cur
		}
	}
	return root
}

// layoutAbsolute sizes and positions an absolutely-positioned box.
func layoutAbsolute(box *ElementBox, cb *ElementBox, root *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	cbWidth, cbHeight := cbContentBoxSizeForBox(cb, root, state)
	margin, padding, border := computeBoxModel(box, cbWidth, fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	width, wAuto := resolveOffset(cs.Width, cbWidth)
	height, hAuto := resolveOffset(cs.Height, cbHeight)
	fs := fontSizeOf(box)
	minW, maxW, minWAuto, maxWAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbWidth, fs)
	minH, maxH, minHAuto, maxHAuto := resolveMinMax(cs.MinHeight, cs.MaxHeight, cbHeight, fs)

	if wAuto {
		width = shrinkToFitWidthForBox(box, cbWidth, margin, border, padding)
	}
	if isBorderBoxForBox(box) {
		width = clampSize(width, minW, maxW, minWAuto, maxWAuto)
	} else {
		width -= border.Horizontal() + padding.Horizontal()
		width = clampSize(width, minW, maxW, minWAuto, maxWAuto)
	}
	g.SetContentWidth(width)

	_ = minHAuto
	_ = maxHAuto

	// First pass: compute explicit height so we can position.
	if hAuto {
		height = layoutAbsoluteHeightForBox(box, state)
	} else if !isBorderBoxForBox(box) {
		height -= border.Vertical() + padding.Vertical()
	}
	height = clampSize(height, minH, maxH, minHAuto, maxHAuto)
	g.SetContentHeight(height)

	// Calculate x from left/right.
	cbg := state.GeometryForBox(cb)
	left, leftAuto := resolveOffset(asLength(cs.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(cs.Properties["right"]), cbWidth)
	x := cbg.ContentBoxLeft()
	switch {
	case !leftAuto && !rightAuto:
		x = cbg.ContentBoxLeft() + left + margin.Left
	case !leftAuto:
		x = cbg.ContentBoxLeft() + left + margin.Left
	case !rightAuto:
		x = cbg.ContentBoxLeft() + cbWidth - right - margin.Right - g.BorderBoxWidth()
	default:
		x = cbg.ContentBoxLeft() + margin.Left
	}
	g.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)

	// Calculate y from top/bottom, using the known border-box height.
	top, topAuto := resolveOffset(asLength(cs.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(cs.Properties["bottom"]), cbHeight)
	y := cbg.ContentBoxTop()
	switch {
	case !topAuto && !bottomAuto:
		y = cbg.ContentBoxTop() + top + margin.Top
	case !topAuto:
		y = cbg.ContentBoxTop() + top + margin.Top
	case !bottomAuto:
		y = cbg.ContentBoxTop() + cbHeight - bottom - margin.Bottom - g.BorderBoxHeight()
	default:
		y = cbg.ContentBoxTop() + margin.Top
	}
	g.SetTopLeft(y, x)

	// Layout content now that position and size are fully known.
	layoutBoxContentForBox(box, state)
}

func cbContentBoxSizeForBox(cb *ElementBox, root *ElementBox, state *LayoutState) (float64, float64) {
	if cb == root {
		return state.ViewportWidth, state.ViewportHeight
	}
	g := state.GeometryForBox(cb)
	return g.ContentWidth(), g.ContentHeight()
}

func shrinkToFitWidthForBox(box *ElementBox, cbWidth float64, margin, border, padding Edges) float64 {
	avail := cbWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
	if avail < 0 { return 0 }
	return avail
}

func layoutAbsoluteHeightForBox(box *ElementBox, state *LayoutState) float64 {
	layoutBoxContentForBox(box, state)
	g := state.GeometryForBox(box)
	h := g.BorderBoxHeight()
	if h == 0 {
		h = g.ContentHeight() + g.VerticalPadding() + g.VerticalBorder()
	}
	return h
}

func layoutBoxContentForBox(box *ElementBox, state *LayoutState) {
	ctx := contextFor(box, state)
	ctx.Layout(box, state)
}

func applyRelativeOffsetForBox(box *ElementBox, cbWidth, cbHeight float64, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)
	left, leftAuto := resolveOffset(asLength(cs.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(cs.Properties["right"]), cbWidth)
	dx := 0.0
	if !leftAuto {
		dx = left
	} else if !rightAuto {
		dx = -right
	}
	top, topAuto := resolveOffset(asLength(cs.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(cs.Properties["bottom"]), cbHeight)
	dy := 0.0
	if !topAuto {
		dy = top
	} else if !bottomAuto {
		dy = -bottom
	}
	if dx == 0 && dy == 0 { return }
	g.SetTopLeft(g.Top()+dy, g.Left()+dx)

	// Recursively offset all descendant geometries so children follow.
	offsetDescendants(box, dx, dy, state)
}

// offsetDescendants recursively applies (dx, dy) to all descendant geometries.
func offsetDescendants(box *ElementBox, dx, dy float64, state *LayoutState) {
	for _, child := range box.Children() {
		switch c := child.(type) {
		case *ElementBox:
			cg := state.GeometryForBox(c)
			cg.SetTopLeft(cg.Top()+dy, cg.Left()+dx)
			offsetDescendants(c, dx, dy, state)
		case *InlineTextBox:
			// Offset each text segment's position.
			for i := range c.TextSegments {
				c.TextSegments[i].X += dx
				c.TextSegments[i].Y += dy
				c.TextSegments[i].LineY += dy
			}
		}
	}
}

func resolveOffset(l style.Length, cbSize float64) (float64, bool) {
	r := resolveLengthAuto(l, cbSize, 0)
	if r.Auto { return 0, true }
	return r.Value, false
}

func resolveOffsetsFromElementAttributes(_ *dom.Element) {}
