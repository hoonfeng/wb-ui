// Translation of: Source/WebCore/rendering/RenderBlockFlow.h
//                  Source/WebCore/rendering/RenderBlockFlow.cpp
//                  Source/WebCore/rendering/LegacyLineLayout.cpp
// Completeness: 50%
// Simplifications:
//   - the modern line-layout path (LineLayout) is omitted; all inline layout is
//     delegated to the InlineFormattingContext via the associated layout box
//   - floating-object tracking (FloatingObjects) is omitted; float layout is handled
//     entirely by the layout package's float context
//   - margin-collapse bookkeeping lives in the layout package
//   - pagination / multi-column flow is omitted
//   - bidi reordering is simplified to LTR only in the inline formatting context

package rendering

import (
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderBlockFlow is the Go translation of WebCore::RenderBlockFlow. It is the most
// common block container: it participates in normal block flow and may contain either
// block-level children (laid out by a BlockFormattingContext) or inline-level children
// (laid out by an InlineFormattingContext). The childrenInline flag selects which path
// is taken during layout.
type RenderBlockFlow struct {
	RenderBlock
}

// NewRenderBlockFlow constructs a RenderBlockFlow for the given DOM node with the given
// style.
func NewRenderBlockFlow(node dom.Node, st *style.ComputedStyle) *RenderBlockFlow {
	rb := &RenderBlockFlow{}
	rb.initBase(rb, node, st)
	return rb
}

// Type returns ObjectBlockFlow.
func (b *RenderBlockFlow) Type() RenderObjectType { return ObjectBlockFlow }

// IsRenderBlockFlow reports that this object is a RenderBlockFlow, mirroring
// RenderObject::isRenderBlockFlow().
func (b *RenderBlockFlow) IsRenderBlockFlow() bool { return true }

// IsInline reports whether this block flow represents an atomic inline-level box.
func (b *RenderBlockFlow) IsInline() bool {
	if b.style == nil {
		return false
	}
	switch b.style.Display {
	case style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

// RenderName returns a debug name for the block flow.
func (b *RenderBlockFlow) RenderName() string { return "RenderBlockFlow" }

// Layout lays out the block flow and its children.
func (b *RenderBlockFlow) Layout(state *layout.LayoutState) {
	if b.layoutBox == nil || state == nil {
		b.ClearNeedsLayout()
		return
	}
	// Dispatch through the shared display-based formatter (grid / table /
	// flex / block), mirroring RenderBox::layout(). The previous hand-rolled
	// flex|inline|block branch silently treated grid and table containers as
	// blocks — grid children stacked vertically at full width, producing
	// completely broken real-page layouts (e.g. IDE app-root 3028px tall
	// instead of 800px).
	ctx := contextForBox(b.layoutBox)
	ctx.Layout(b.layoutBox, state)
	b.ClearNeedsLayout()
}

// layoutInlineChildren lays out the inline-level children.
func (b *RenderBlockFlow) layoutInlineChildren(state *layout.LayoutState) {
	ctx := &layout.InlineFormattingContext{}
	ctx.InitBase(b.layoutBox, state)
	ctx.Layout(b.layoutBox, state)
}

// AddChild overrides the base to track whether the child is inline-level.
func (b *RenderBlockFlow) AddChild(child RenderObject, beforeChild RenderObject) {
	b.renderObjectBase.AddChild(child, beforeChild)
	if child.IsInline() {
		b.childrenInline = true
	}
}

// LastChildIsInline reports whether the last child of the block is inline.
func (b *RenderBlockFlow) LastChildIsInline() bool {
	return b.LastChild() != nil && b.LastChild().IsInline()
}
