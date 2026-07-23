// Translation of: Source/WebCore/rendering/RenderBlock.h
//                  Source/WebCore/rendering/RenderBlock.cpp
//                  Source/WebCore/rendering/RenderElement.h
//                  Source/WebCore/rendering/RenderElement.cpp
// Completeness: 50%
// Simplifications:
//   - RenderElement is folded into RenderBlock; the intermediate layer that manages
//     generated content / pseudo-elements is omitted
//   - anonymous block generation for mixed inline/block content is simplified: the
//     builder wraps inline runs lazily rather than maintaining a live continuation chain
//   - line box management (RenderLineBoxList) is omitted; inline layout is delegated
//     directly to the InlineFormattingContext via the associated layout box
//   - margin-collapse / float-cleanup bookkeeping lives in the layout package, not here

package rendering

import (
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderBlock is the Go translation of WebCore::RenderBlock. It is a RenderBox that acts
// as a block-level container, managing a list of child render objects that participate
// in block or inline formatting. RenderBlock itself does not decide between block and
// inline layout; that responsibility belongs to RenderBlockFlow.
type RenderBlock struct {
	RenderBox
	// childrenInline tracks whether the block's children are inline-level (true) or
	// block-level (false), mirroring RenderBlock::childrenInline().
	childrenInline bool
}

// NewRenderBlock constructs a RenderBlock for the given DOM node with the given style.
func NewRenderBlock(node dom.Node, st *style.ComputedStyle) *RenderBlock {
	rb := &RenderBlock{}
	rb.initBase(rb, node, st)
	return rb
}

// Type returns ObjectBlock.
func (b *RenderBlock) Type() RenderObjectType { return ObjectBlock }

// IsBlock reports that this object is a block-level container.
func (b *RenderBlock) IsBlock() bool { return true }

// IsRenderBlock reports that this object is a RenderBlock, mirroring
// RenderObject::isRenderBlock().
func (b *RenderBlock) IsRenderBlock() bool { return true }

// ChildrenInline reports whether the block's children are inline-level, mirroring
// RenderBlock::childrenInline().
func (b *RenderBlock) ChildrenInline() bool { return b.childrenInline }

// SetChildrenInline records whether the block's children are inline-level, mirroring
// RenderBlock::setChildrenInline().
func (b *RenderBlock) SetChildrenInline(v bool) { b.childrenInline = v }

// RenderName returns a debug name for the block.
func (b *RenderBlock) RenderName() string { return "RenderBlock" }

// FirstChildBox returns the first child as a RenderBlock (shadowing RenderBox's version
// to return the block-typed pointer).
func (b *RenderBlock) FirstChildBlock() *RenderBlock {
	if b.firstChild == nil {
		return nil
	}
	if cb, ok := b.firstChild.(*RenderBlock); ok {
		return cb
	}
	return nil
}

// layoutBlockChildren lays out the block's in-flow block-level children by dispatching
// to the block formatting context on the associated layout box. Mirrors the block
// portion of RenderBlockFlow::layoutBlockChildren.
func (b *RenderBlock) layoutBlockChildren(state *layout.LayoutState) {
	if b.layoutBox == nil || state == nil {
		return
	}
	ctx := &layout.BlockFormattingContext{}
	ctx.InitBase(b.layoutBox, state)
	ctx.Layout(b.layoutBox, state)
}
// CanHaveChildren reports whether this block may hold render children, mirroring
// RenderElement::canHaveChildren(). Blocks always can.
func (b *RenderBlock) CanHaveChildren() bool { return true }

// CreatesAnonymousWrapper reports whether this block would generate an anonymous
// wrapper for inline content, mirroring RenderBlock::createsAnonymousWrapper().
func (b *RenderBlock) CreatesAnonymousWrapper() bool { return true }
