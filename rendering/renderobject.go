// Translation of: Source/WebCore/rendering/RenderObject.h
//                  Source/WebCore/rendering/RenderObject.cpp
// Completeness: 60%
// Simplifications:
//   - no SVG render objects
//   - no repaint-after-layout optimization
//   - C++ multiple inheritance (RenderObject -> RenderElement -> RenderBox ...)
//     is replaced with a Go interface backed by a shared renderObjectBase struct
//     embedded by every concrete type; the self back-pointer mirrors nodeBase in dom
//   - intrusive reference counting (ref/deref) is omitted; Go GC manages lifetime
//   - the bit-packed TypeFlag / TypeSpecificFlags fields are collapsed into a small
//     set of boolean predicate methods overridden by each concrete type
//   - layout is delegated to the layout package; RenderObject.Layout dispatches to the
//     formatting context that owns the associated layout box

package rendering

import (
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderObjectType mirrors the subset of WebCore::RenderObject::Type that this port
// models. SVG / MathML / form-control specific types are omitted.
type RenderObjectType uint8

const (
	ObjectView RenderObjectType = iota
	ObjectBlockFlow
	ObjectBlock
	ObjectBox
	ObjectInline
	ObjectText
	ObjectReplaced
)

// RenderObject is the Go translation of WebCore::RenderObject. In WebKit RenderObject is
// the abstract base of the render tree; every node that participates in layout and
// painting derives from it. The Go port models this as an interface that every concrete
// render type satisfies by embedding renderObjectBase. Tree navigation and mutation
// follow the same parent / first-child / next-sibling sibling-list encoding as the DOM.
type RenderObject interface {
	// Type returns the render object type tag.
	Type() RenderObjectType

	// Tree navigation, mirroring RenderObject::parent() / firstChild() / etc.
	Parent() RenderObject
	FirstChild() RenderObject
	LastChild() RenderObject
	NextSibling() RenderObject
	PreviousSibling() RenderObject

	// Tree mutation, mirroring RenderTreeBuilder insert / remove.
	AddChild(child RenderObject, beforeChild RenderObject)
	RemoveChild(child RenderObject)

	// Style returns the ComputedStyle associated with this object, mirroring
	// RenderObject::style().
	Style() *style.ComputedStyle
	SetStyle(st *style.ComputedStyle)

	// Node returns the DOM node that generated this render object, mirroring
	// RenderObject::node().
	Node() dom.Node

	// View returns the root RenderView of the tree, mirroring RenderObject::view().
	View() *RenderView

	// Document returns the document that owns this render object.
	Document() *dom.Document

	// LayoutBox returns the layout box associated with this render object. In modern
	// WebKit the layout tree is separate from the render tree; this port keeps a
	// back-pointer from the render object to its layout box.
	LayoutBox() *layout.LayoutBox
	SetLayoutBox(b *layout.LayoutBox)

	// Type predicates mirroring RenderObject::is*().
	IsBox() bool
	IsInline() bool
	IsText() bool
	IsBlock() bool
	IsRenderBlock() bool
	IsRenderBlockFlow() bool
	IsRenderInline() bool
	IsRenderText() bool
	IsRenderView() bool
	IsAnonymous() bool

	// Layout is the virtual layout entry point, mirroring RenderObject::layout().
	// Concrete types override it to dispatch to the appropriate formatting context.
	Layout(state *layout.LayoutState)

	// Dirty / NeedsLayout mark the object as needing a relayout, mirroring
	// RenderObject::setNeedsLayout() / needsLayout().
	Dirty()
	NeedsLayout() bool
	ClearNeedsLayout()

	// RenderName returns a debug name for the object, mirroring renderName().
	RenderName() string

	// NextInPreOrder returns the next node in pre-order traversal, mirroring
	// RenderObject::nextInPreOrder().
	NextInPreOrder() RenderObject
}

// renderObjectBase is the shared implementation embedded by every concrete render type.
// It holds the structural fields (parent, siblings, first/last child), the owning DOM
// node, the resolved ComputedStyle, the associated layout box and the layout-dirty flag.
// The self field is a back-pointer to the outer RenderObject (set by initBase) so that
// base methods can recover the concrete node when building traversal paths.
type renderObjectBase struct {
	self          RenderObject
	node          dom.Node
	style         *style.ComputedStyle
	parent        RenderObject
	firstChild    RenderObject
	lastChild     RenderObject
	nextSibling   RenderObject
	prevSibling   RenderObject
	layoutBox     *layout.LayoutBox
	needsLayout   bool
	beingDestroyed bool
	isAnonymous   bool
}

// initBase populates the shared render object state. It is called by every concrete
// constructor with self pointing at the freshly allocated outer object so that base
// methods can later recover the concrete value.
func (b *renderObjectBase) initBase(self RenderObject, node dom.Node, st *style.ComputedStyle) {
	b.self = self
	b.node = node
	b.style = st
	b.needsLayout = true
}

// base returns the embedded renderObjectBase. It is the counterpart of the dom
// package's asNodeBase and lets tree-mutation helpers recover the shared fields from any
// RenderObject interface value.
func (b *renderObjectBase) base() *renderObjectBase { return b }

// baseOf returns the *renderObjectBase backing r, or nil for a nil render object.
func baseOf(r RenderObject) *renderObjectBase {
	if r == nil {
		return nil
	}
	type baseHolder interface {
		base() *renderObjectBase
	}
	if h, ok := r.(baseHolder); ok {
		return h.base()
	}
	return nil
}

// Parent returns the parent render object, mirroring RenderObject::parent().
func (b *renderObjectBase) Parent() RenderObject { return b.parent }

// FirstChild / LastChild mirror RenderObject::firstChild() / lastChild().
func (b *renderObjectBase) FirstChild() RenderObject { return b.firstChild }
func (b *renderObjectBase) LastChild() RenderObject  { return b.lastChild }

// NextSibling / PreviousSibling mirror RenderObject::nextSibling() / previousSibling().
func (b *renderObjectBase) NextSibling() RenderObject     { return b.nextSibling }
func (b *renderObjectBase) PreviousSibling() RenderObject { return b.prevSibling }

// Style returns the computed style, mirroring RenderObject::style().
func (b *renderObjectBase) Style() *style.ComputedStyle { return b.style }

// SetStyle replaces the computed style, mirroring RenderObject::setStyle().
func (b *renderObjectBase) SetStyle(st *style.ComputedStyle) {
	b.style = st
	b.needsLayout = true
}

// Node returns the generating DOM node, mirroring RenderObject::node().
func (b *renderObjectBase) Node() dom.Node { return b.node }

// LayoutBox returns the associated layout box.
func (b *renderObjectBase) LayoutBox() *layout.LayoutBox { return b.layoutBox }

// SetLayoutBox associates a layout box with this render object.
func (b *renderObjectBase) SetLayoutBox(box *layout.LayoutBox) { b.layoutBox = box }

// View walks up the parent chain to find the RenderView root, mirroring
// RenderObject::view().
func (b *renderObjectBase) View() *RenderView {
	for cur := b.self; cur != nil; cur = cur.Parent() {
		if v, ok := cur.(*RenderView); ok {
			return v
		}
	}
	return nil
}

// Document returns the document that owns the generating node, mirroring
// RenderObject::document().
func (b *renderObjectBase) Document() *dom.Document {
	if b.node != nil {
		return b.node.OwnerDocument()
	}
	return nil
}

// Default type predicates on the base return false; concrete types override the ones
// that apply to them by shadowing the promoted method.
func (b *renderObjectBase) IsBox() bool            { return false }
func (b *renderObjectBase) IsInline() bool         { return false }
func (b *renderObjectBase) IsText() bool           { return false }
func (b *renderObjectBase) IsBlock() bool          { return false }
func (b *renderObjectBase) IsRenderBlock() bool    { return false }
func (b *renderObjectBase) IsRenderBlockFlow() bool { return false }
func (b *renderObjectBase) IsRenderInline() bool    { return false }
func (b *renderObjectBase) IsRenderText() bool     { return false }
func (b *renderObjectBase) IsRenderView() bool     { return false }
func (b *renderObjectBase) IsAnonymous() bool       { return b.isAnonymous }

// Layout on the base is a no-op; concrete types override it.
func (b *renderObjectBase) Layout(state *layout.LayoutState) {}

// Dirty marks the object and its ancestors as needing layout, mirroring
// RenderObject::setNeedsLayout().
func (b *renderObjectBase) Dirty() {
	b.needsLayout = true
	for cur := b.parent; cur != nil; cur = cur.Parent() {
		cb := baseOf(cur)
		if cb != nil {
			cb.needsLayout = true
		}
	}
}

// NeedsLayout reports whether the object requires a layout pass, mirroring
// RenderObject::needsLayout().
func (b *renderObjectBase) NeedsLayout() bool { return b.needsLayout }

// ClearNeedsLayout resets the layout-dirty flag, mirroring
// RenderObject::clearNeedsLayout().
func (b *renderObjectBase) ClearNeedsLayout() { b.needsLayout = false }

// AddChild inserts child into b's child list. When beforeChild is nil the child is
// appended; otherwise it is inserted immediately before beforeChild. This mirrors
// RenderTreeBuilder::insertChild.
func (b *renderObjectBase) AddChild(child RenderObject, beforeChild RenderObject) {
	cb := baseOf(child)
	if cb == nil {
		return
	}
	// Detach from existing parent first.
	if cb.parent != nil {
		baseOf(cb.parent).RemoveChild(child)
	}
	cb.parent = b.self
	if beforeChild == nil {
		// Append.
		cb.prevSibling = b.lastChild
		cb.nextSibling = nil
		if b.lastChild != nil {
			baseOf(b.lastChild).nextSibling = child
		} else {
			b.firstChild = child
		}
		b.lastChild = child
	} else {
		// Insert before beforeChild.
		bc := baseOf(beforeChild)
		cb.prevSibling = bc.prevSibling
		cb.nextSibling = beforeChild
		if bc.prevSibling != nil {
			baseOf(bc.prevSibling).nextSibling = child
		} else {
			b.firstChild = child
		}
		bc.prevSibling = child
	}
}

// RemoveChild detaches child from b's child list, mirroring
// RenderTreeBuilder::removeChild.
func (b *renderObjectBase) RemoveChild(child RenderObject) {
	cb := baseOf(child)
	if cb == nil || cb.parent != b.self {
		return
	}
	if cb.prevSibling != nil {
		baseOf(cb.prevSibling).nextSibling = cb.nextSibling
	} else {
		b.firstChild = cb.nextSibling
	}
	if cb.nextSibling != nil {
		baseOf(cb.nextSibling).prevSibling = cb.prevSibling
	} else {
		b.lastChild = cb.prevSibling
	}
	cb.parent = nil
	cb.prevSibling = nil
	cb.nextSibling = nil
}

// NextInPreOrder returns the next node in a pre-order traversal, mirroring
// RenderObject::nextInPreOrder().
func (b *renderObjectBase) NextInPreOrder() RenderObject {
	if b.firstChild != nil {
		return b.firstChild
	}
	for cur := b.self; cur != nil; cur = cur.Parent() {
		if cur.NextSibling() != nil {
			return cur.NextSibling()
		}
	}
	return nil
}

// NextInPreOrderStayWithin returns the next node in pre-order, stopping when it would
// leave the subtree rooted at stayWithin, mirroring nextInPreOrder(stayWithin).
func (b *renderObjectBase) NextInPreOrderStayWithin(stayWithin RenderObject) RenderObject {
	if b.firstChild != nil {
		return b.firstChild
	}
	for cur := b.self; cur != nil && cur != stayWithin; cur = cur.Parent() {
		if cur.NextSibling() != nil {
			return cur.NextSibling()
		}
	}
	return nil
}

// IsDescendantOf reports whether this object is a descendant of other, mirroring
// RenderObject::isDescendantOf().
func (b *renderObjectBase) IsDescendantOf(other RenderObject) bool {
	if other == nil {
		return false
	}
	for cur := b.parent; cur != nil; cur = cur.Parent() {
		if cur == other {
			return true
		}
	}
	return false
}

// containingBlockBase searches the ancestor chain for the containing block, mirroring
// RenderObject::containingBlock(). For in-flow objects the containing block is the
// nearest ancestor that establishes a block formatting context; for absolutely
// positioned objects it is the nearest positioned ancestor.
func (b *renderObjectBase) containingBlockBase() RenderObject {
	st := b.style
	if st != nil && (st.Position == style.PositionAbsolute || st.Position == style.PositionFixed) {
		// Positioned objects look for the nearest positioned ancestor.
		for cur := b.parent; cur != nil; cur = cur.Parent() {
			cs := cur.Style()
			if cs != nil && cs.Position != style.PositionStatic {
				return cur
			}
		}
		return b.View()
	}
	// In-flow: walk up to the nearest block ancestor.
	for cur := b.parent; cur != nil; cur = cur.Parent() {
		if cur.IsBlock() {
			return cur
		}
	}
	return b.View()
}

// markAnonymous flags the object as anonymously generated (no associated DOM element).
func (b *renderObjectBase) markAnonymous() { b.isAnonymous = true }

// RenderName on the base returns a generic name; concrete types override it.
func (b *renderObjectBase) RenderName() string { return "RenderObject" }
