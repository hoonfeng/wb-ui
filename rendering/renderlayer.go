// Translation of: Source/WebCore/rendering/RenderLayer.h
//                  Source/WebCore/rendering/RenderLayer.cpp
// Completeness: 45%
// Simplifications:
//   - the clip-rects cache (ClipRects / ClipRectsCache) is omitted; clipping is
//     computed on demand from the ancestor overflow chain
//   - the scrollable-area integration is omitted; scroll offsets are not tracked
//   - the layer fragment system (LayerFragment) is omitted
//   - hit testing is omitted
//   - the painting-ancestor / repainting-ancestor bookkeeping is omitted
//   - a layer is created when the owner's style satisfies any of the conditions in
//     RequiresLayer (position != static / opacity < 1 / transform / overflow !=
//     visible / filter), matching WebKit's RenderLayer::requiresLayer()

package rendering

import (
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderLayer is the Go translation of WebCore::RenderLayer. It is a parallel tree to the
// render tree: layers are created for render objects that need to be composited, clipped
// or transformed independently of normal flow. The layer tree maintains parent / child /
// sibling links that mirror the render tree's structure but are owned by RenderLayer
// rather than RenderObject.
type RenderLayer struct {
	owner       RenderObject
	parent      *RenderLayer
	parentPane  *RenderLayer
	firstChild  *RenderLayer
	lastChild   *RenderLayer
	nextSibling *RenderLayer
	prevSibling *RenderLayer
	backing     *RenderLayerBacking
	// childrenDirty tracks whether the layer's child list needs to be rebuilt.
	childrenDirty bool
}

// NewRenderLayer constructs a RenderLayer owned by the given render object.
func NewRenderLayer(owner RenderObject) *RenderLayer {
	return &RenderLayer{owner: owner}
}

// Owner returns the render object that owns this layer, mirroring
// RenderLayer::renderer().
func (l *RenderLayer) Owner() RenderObject { return l.owner }

// Parent returns the parent layer, mirroring RenderLayer::parent().
func (l *RenderLayer) Parent() *RenderLayer { return l.parent }

// FirstChild / LastChild / NextSibling / PreviousSibling mirror the layer tree
// navigation methods on RenderLayer.
func (l *RenderLayer) FirstChild() *RenderLayer  { return l.firstChild }
func (l *RenderLayer) LastChild() *RenderLayer   { return l.lastChild }
func (l *RenderLayer) NextSibling() *RenderLayer { return l.nextSibling }
func (l *RenderLayer) PreviousSibling() *RenderLayer { return l.prevSibling }

// AddChild appends child to this layer's child list, mirroring
// RenderLayer::addChild().
func (l *RenderLayer) AddChild(child *RenderLayer) {
	if child.parent != nil {
		child.parent.RemoveChild(child)
	}
	child.parent = l
	child.prevSibling = l.lastChild
	if l.lastChild != nil {
		l.lastChild.nextSibling = child
	} else {
		l.firstChild = child
	}
	l.lastChild = child
}

// RemoveChild detaches child from this layer's child list, mirroring
// RenderLayer::removeChild().
func (l *RenderLayer) RemoveChild(child *RenderLayer) {
	if child.parent != l {
		return
	}
	if child.prevSibling != nil {
		child.prevSibling.nextSibling = child.nextSibling
	} else {
		l.firstChild = child.nextSibling
	}
	if child.nextSibling != nil {
		child.nextSibling.prevSibling = child.prevSibling
	} else {
		l.lastChild = child.prevSibling
	}
	child.parent = nil
	child.prevSibling = nil
	child.nextSibling = nil
}

// Backing returns the compositing backing for this layer, or nil when the layer is not
// independently composited, mirroring RenderLayer::backing().
func (l *RenderLayer) Backing() *RenderLayerBacking { return l.backing }

// EnsureBacking creates a RenderLayerBacking for this layer if one does not exist,
// mirroring RenderLayer::ensureBacking().
func (l *RenderLayer) EnsureBacking() *RenderLayerBacking {
	if l.backing == nil {
		l.backing = NewRenderLayerBacking(l)
	}
	return l.backing
}

// ClearBacking drops the compositing backing, mirroring RenderLayer::clearBacking().
func (l *RenderLayer) ClearBacking() { l.backing = nil }

// RequiresLayer reports whether the owner's style requires a dedicated render layer.
// This mirrors the condition set in RenderLayerModelObject::requiresLayer():
//   - position != static (relative / absolute / fixed / sticky)
//   - opacity < 1
//   - transform is set
//   - overflow != visible
//   - filter is set
func RequiresLayer(owner RenderObject) bool {
	if owner == nil {
		return false
	}
	st := owner.Style()
	if st == nil {
		return false
	}
	if st.Position != style.PositionStatic {
		return true
	}
	if st.Opacity < 1.0 {
		return true
	}
	if st.Transform != "" {
		return true
	}
	if st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible {
		// Form controls (input/textarea/select/button) scroll their content
		// internally (UA overflow:hidden/auto). Giving them a composited
		// layer would clip the :focus outline, which browsers paint OUTSIDE
		// the border box (outline is not subject to the element's own
		// overflow clip). Without this a focused textarea showed NO outline
		// at all — the outline stroke fell outside the layer's clip rect.
		if n := owner.Node(); n != nil {
			if el, ok := n.(*dom.Element); ok && isReplacedElement(el.LocalName()) {
				return false
			}
		}
		return true
	}
	if st.Filter != "" {
		return true
	}
	return false
}

// CalculateRects computes the layer's visible rectangle and clip rectangle relative to
// its owner's border box, mirroring RenderLayer::calculateRects(). In this simplified
// port the layer rect is the owner's border-box rect (for boxes) or a zero rect (for
// non-box objects), and the clip rect is the intersection of the ancestor overflow clip
// chain.
func (l *RenderLayer) CalculateRects() (layerRect, clipRect layout.LayoutRect) {
	if l.owner == nil {
		return
	}
	if box := asRenderBox(l.owner); box != nil {
		layerRect = box.BorderBoxRect()
	} else {
		layerRect = layout.LayoutRect{}
	}
	// A layer only clips its subtree when overflow is not visible. Start
	// from zero (= no clip): with overflow:visible the layer's own
	// border-box must NOT clip overflowing content (box-shadow, negative
	// margins, absolutely positioned children...). Each ancestor that has
	// overflow != visible narrows the clip to its padding-box.
	clipRect = layout.LayoutRect{}
	cs := l.owner.Style()
	if cs != nil && (cs.OverflowX != style.OverflowVisible || cs.OverflowY != style.OverflowVisible) {
		clipRect = layerRect
	}
	// Walk the ancestor layer chain intersecting with each ancestor's overflow clip.
	for cur := l.parent; cur != nil; cur = cur.parent {
		if cur.owner == nil {
			continue
		}
		cb := asRenderBox(cur.owner)
		if cb == nil {
			continue
		}
		cs := cur.owner.Style()
		if cs == nil {
			continue
		}
		if cs.OverflowX != style.OverflowVisible || cs.OverflowY != style.OverflowVisible {
			ancestorRect := cb.PaddingBoxRect()
			if clipRect.Width == 0 && clipRect.Height == 0 {
				clipRect = ancestorRect
			} else {
				clipRect = intersectRects(clipRect, ancestorRect)
			}
		}
		// A fixed-position ancestor establishes a viewport containing
		// block: it (and its descendants) left the normal flow, so
		// overflow clips of ancestors OUTSIDE it must not apply — the
		// browser never clips a fixed dialog by the overflow of a scroll
		// container it happens to be nested in. The fixed ancestor's own
		// overflow (applied above) still clips its subtree. Mirrors
		// RenderLayer::calculateClipRects treating fixed layers as clip
		// roots. Without this, an opacity<1 element inside a fixed dialog
		// gets its own layer (RequiresLayer) and paints via the normal
		// layer branch, where this chain wrongly re-applied e.g. the
		// sidebar-content overflow clip, culling the paint.
		if cs.Position == style.PositionFixed {
			break
		}
	}
	return layerRect, clipRect
}

// intersectRects returns the intersection of two layout rects. If they do not overlap
// the result is a zero rect.
func intersectRects(a, b layout.LayoutRect) layout.LayoutRect {
	x := maxF(a.X, b.X)
	y := maxF(a.Y, b.Y)
	right := minF(a.X+a.Width, b.X+b.Width)
	bottom := minF(a.Y+a.Height, b.Y+b.Height)
	if right <= x || bottom <= y {
		return layout.LayoutRect{}
	}
	return layout.LayoutRect{X: x, Y: y, Width: right - x, Height: bottom - y}
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// IsComposited reports whether this layer has a compositing backing, mirroring
// RenderLayer::isComposited().
func (l *RenderLayer) IsComposited() bool { return l.backing != nil }

// SetChildrenDirty marks the layer's child list as needing rebuild.
func (l *RenderLayer) SetChildrenDirty(v bool) { l.childrenDirty = v }
