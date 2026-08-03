// Translation of: Source/WebCore/rendering/RenderInline.h
//                  Source/WebCore/rendering/RenderInline.cpp
// Completeness: 45%
// Simplifications:
//   - RenderInline does not generate a box in the CSS box model; it contributes to the
//     inline formatting context by splitting into inline boxes (LineBox segments). This
//     port does not model line boxes; RenderInline carries only the style and children
//     that participate in the inline formatting context
//   - continuation chains (the split-into-multiple-fragments mechanism for inline
//     elements that contain block children) are omitted
//   - inline-box painting state (InlineBoxList) is omitted; painting is handled in
//     Phase 8

package rendering

import (
	"strconv"
	"strings"

	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderInline is the Go translation of WebCore::RenderInline. It represents an
// inline-level element (e.g. <span>) that does not generate a box itself but influences
// the inline layout of its children. RenderInline holds a ComputedStyle and a list of
// child render objects; the inline formatting context reads the style to apply
// borders/backgrounds on the inline boxes it generates.
type RenderInline struct {
	renderObjectBase
}

// NewRenderInline constructs a RenderInline for the given DOM node with the given style.
func NewRenderInline(node dom.Node, st *style.ComputedStyle) *RenderInline {
	ri := &RenderInline{}
	ri.initBase(ri, node, st)
	return ri
}

// Type returns ObjectInline.
func (r *RenderInline) Type() RenderObjectType { return ObjectInline }

// IsInline reports that this object is inline-level.
func (r *RenderInline) IsInline() bool { return true }

// IsRenderInline reports that this object is a RenderInline, mirroring
// RenderObject::isRenderInline().
func (r *RenderInline) IsRenderInline() bool { return true }

// RenderName returns a debug name for the inline.
func (r *RenderInline) RenderName() string { return "RenderInline" }

// Layout is a no-op for RenderInline; inline elements are laid out by their containing
// block's inline formatting context. This mirrors RenderInline::layout() which is
// effectively empty (layout happens via the parent's line layout).
func (r *RenderInline) Layout(state *layout.LayoutState) {
	r.ClearNeedsLayout()
}

// MarginHorizontal returns the horizontal margin contribution of this inline element,
// mirroring RenderInline::marginStart()/marginEnd() (collapsed to left+right for LTR).
func (r *RenderInline) MarginHorizontal() float64 {
	if r.style == nil {
		return 0
	}
	return lengthValue(r.style.MarginLeft) + lengthValue(r.style.MarginRight)
}

// PaddingHorizontal returns the horizontal padding contribution.
func (r *RenderInline) PaddingHorizontal() float64 {
	if r.style == nil {
		return 0
	}
	return lengthValue(r.style.PaddingLeft) + lengthValue(r.style.PaddingRight)
}

// BorderHorizontal returns the horizontal border contribution.
func (r *RenderInline) BorderHorizontal() float64 {
	if r.style == nil {
		return 0
	}
	return lengthValue(r.style.BorderLeftWidth) + lengthValue(r.style.BorderRightWidth)
}

// CanHaveChildren reports whether this inline may hold render children. Inline elements
// can contain text and other inline elements, mirroring RenderElement::canHaveChildren().
func (r *RenderInline) CanHaveChildren() bool { return true }

// lengthValue extracts the numeric value of a style.Length, returning 0 for auto/empty.
func lengthValue(l style.Length) float64 {
	if l.Unit == "auto" || l.Unit == "" && l.Value == 0 {
		return 0
	}
	return l.Value
}

// parseLengthAny parses a CSS length string ("4px", "2.5px") into a pixel value.
// Used for ::-webkit-scrollbar width/radius overrides where only px makes sense.
func parseLengthAny(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "px") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "px"))
	}
	if s == "" {
		return 0, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
}
