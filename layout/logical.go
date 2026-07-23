// Translation of: Source/WebCore/layout/LayoutUnits.h (logical coordinate helpers)
// Logical dimension helpers for writing modes.

package layout

import "wb-ui/style"

// IsVerticalWritingMode returns true for vertical-rl or vertical-lr.
func IsVerticalWritingMode(cs *style.ComputedStyle) bool {
	if cs == nil { return false }
	return cs.WritingMode == "vertical-rl" || cs.WritingMode == "vertical-lr"
}

// Logical accessors operate on Box interface + state.

// ContentBoxLogicalWidth returns the content-box width along the inline axis.
// For horizontal-tb = ContentWidth(); for vertical = ContentHeight().
func ContentBoxLogicalWidth(box Box, state *LayoutState) float64 {
	g := state.GeometryForBox(box)
	if IsVerticalWritingMode(box.Style()) { return g.ContentHeight() }
	return g.ContentWidth()
}

// ContentBoxLogicalHeight returns the content-box size along the block axis.
// For horizontal-tb = ContentHeight(); for vertical = ContentWidth().
func ContentBoxLogicalHeight(box Box, state *LayoutState) float64 {
	g := state.GeometryForBox(box)
	if IsVerticalWritingMode(box.Style()) { return g.ContentWidth() }
	return g.ContentHeight()
}

// InlineSize returns the size along the inline axis (width in horizontal, height in vertical).
func InlineSize(box Box, state *LayoutState) float64 {
	return ContentBoxLogicalWidth(box, state)
}

// BlockSize returns the size along the block axis (height in horizontal, width in vertical).
func BlockSize(box Box, state *LayoutState) float64 {
	return ContentBoxLogicalHeight(box, state)
}
