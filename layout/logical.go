package layout

import "wb-ui/style"

// WritingMode helpers.
// These functions abstract the writing-mode direction so that formatting contexts
// can compute logical block/inline dimensions correctly.

// IsVerticalWritingMode reports whether the style specifies a vertical writing mode
// (vertical-rl or vertical-lr), where the block axis is horizontal and the inline
// axis is vertical.
func IsVerticalWritingMode(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	return s.WritingMode == "vertical-rl" || s.WritingMode == "vertical-lr"
}

// IsLineReversed reports whether the inline direction is reversed (right-to-left
// for horizontal-tb, bottom-to-top for vertical modes). Currently only
// horizontal-tb is fully supported.
func IsLineReversed(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	return s.Direction == "rtl"
}

// LogicalWidth returns the content size along the inline axis.
// For horizontal-tb: content width.
// For vertical-rl/lr: content height.
func LogicalWidth(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentHeight()
	}
	return box.Rect.ContentWidth()
}

// LogicalHeight returns the content size along the block axis.
// For horizontal-tb: content height.
// For vertical-rl/lr: content width.
func LogicalHeight(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentWidth()
	}
	return box.Rect.ContentHeight()
}

// BlockAxisStart returns the starting coordinate of the block axis (the axis
// along which block-level children stack).
// For horizontal-tb: contentY (top).
// For vertical-rl: contentX + contentWidth (right edge, going left).
// For vertical-lr: contentX (left edge, going right).
func BlockAxisStart(box *LayoutBox) float64 {
	if box.Style == nil {
		return box.Rect.ContentY()
	}
	switch box.Style.WritingMode {
	case "vertical-rl":
		return box.Rect.ContentX() + box.Rect.ContentWidth()
	case "vertical-lr":
		return box.Rect.ContentX()
	default:
		return box.Rect.ContentY()
	}
}

// BlockAxisContentSize returns the available size along the block axis.
// For horizontal-tb: content height.
// For vertical-rl/lr: content width.
func BlockAxisContentSize(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentWidth()
	}
	return box.Rect.ContentHeight()
}

// InlineAxisContentSize returns the available size along the inline axis.
// For horizontal-tb: content width.
// For vertical-rl/lr: content height.
func InlineAxisContentSize(box *LayoutBox) float64 {
	return LogicalWidth(box)
}

// LogicalCursorPosition returns the position of a child along the block axis.
// For horizontal-tb: child.Rect.Y (top).
// For vertical-rl/lr: child.Rect.X (left/right depending on direction).
func LogicalCursorPosition(child *LayoutBox) float64 {
	if IsVerticalWritingMode(child.Style) {
		return child.Rect.X
	}
	return child.Rect.Y
}

// SetLogicalCursorPosition sets the block-start position of a child.
func SetLogicalCursorPosition(child *LayoutBox, pos float64) {
	if IsVerticalWritingMode(child.Style) {
		child.Rect.X = pos
	} else {
		child.Rect.Y = pos
	}
}

// LogicalBlockSize returns the size of the box along the block axis.
// For horizontal-tb: height.
// For vertical-rl/lr: width.
func LogicalBlockSize(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.Width
	}
	return box.Rect.Height
}

// SetLogicalBlockSize sets the size of the box along the block axis.
func SetLogicalBlockSize(box *LayoutBox, size float64) {
	if IsVerticalWritingMode(box.Style) {
		box.Rect.Width = size
	} else {
		box.Rect.Height = size
	}
}

// LogicalInlineSize returns the size of the box along the inline axis.
// For horizontal-tb: width.
// For vertical-rl/lr: height.
func LogicalInlineSize(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.Height
	}
	return box.Rect.Width
}

// SetLogicalInlineSize sets the size of the box along the inline axis.
func SetLogicalInlineSize(box *LayoutBox, size float64) {
	if IsVerticalWritingMode(box.Style) {
		box.Rect.Height = size
	} else {
		box.Rect.Width = size
	}
}

// ContentBlockStart returns the content-area start along the block axis.
func ContentBlockStart(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentX()
	}
	return box.Rect.ContentY()
}

// ContentBlockEnd returns the content-area end along the block axis.
func ContentBlockEnd(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentX() + box.Rect.ContentWidth()
	}
	return box.Rect.ContentY() + box.Rect.ContentHeight()
}

// InlineStart returns the content-area start along the inline axis.
func InlineStart(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentY()
	}
	return box.Rect.ContentX()
}

// InlineContentSize returns the content-area size along the inline axis.
func InlineContentSize(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.ContentHeight()
	}
	return box.Rect.ContentWidth()
}

// BlockMarginStart returns the margin value on the block-start side.
func BlockMarginStart(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.Margin.Left
	}
	return box.Rect.Margin.Top
}

// BlockMarginEnd returns the margin value on the block-end side.
func BlockMarginEnd(box *LayoutBox) float64 {
	if IsVerticalWritingMode(box.Style) {
		return box.Rect.Margin.Right
	}
	return box.Rect.Margin.Bottom
}
