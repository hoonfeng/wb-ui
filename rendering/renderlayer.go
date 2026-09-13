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
	"strings"

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
	// CSS mask-image: a masked element must own a layer so its mask can wrap
	// the ENTIRE subtree (background + foreground + outline + descendants) in
	// one offscreen layer — mirrors WebKit where a mask forces a RenderLayer.
	if mv := st.GetProperty("mask-image"); mv != "" && !strings.EqualFold(strings.TrimSpace(mv), "none") {
		return true
	}
	return false
}

// CalculateRects computes the layer's visible rectangle and clip rectangle relative to
// its owner's border box, mirroring RenderLayer::calculateRects(). In this simplified
// port the layer rect is the owner's border-box rect (for boxes) or a zero rect (for
// non-box objects), and the clip rect is the intersection of the ancestor overflow clip
// chain.
func (l *RenderLayer) CalculateRects() (layerRect, clipRect layout.LayoutRect, clipSpecified bool) {
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
	//
	// ★ hasClip flag, NOT a zero-rect test: intersectRects() also returns a
	// zero rect when a layer is fully outside an ancestor's overflow clip
	// (e.g. a conv-title scrolled out of its conv-list viewport). Treating
	// that zero as "no clip yet" made the loop REPLACE the (empty)
	// intersection with the next ancestor's padding box — the scrolled-out
	// title ended up clipped by an unrelated ancestor (conv-sidebar) rect
	// (1031,67,249x711) instead of being fully culled.
	//
	// ★ Device-coordinate correction for scroll containers: this port keeps
	// layout geometry ABSOLUTE and implements scrolling with a canvas
	// translate (paintLayerContents). CalculateRects must therefore return
	// the clip in DEVICE coordinates — paintLayerTree adds the accumulated
	// scrollTranslate back and Clip() maps it to the screen-fixed viewport:
	//   - the layer's own border box (content coords) minus the sum of all
	//     scroll offsets of scroll ancestors = its device position
	//   - each scroll ancestor's padding box minus the scroll offsets of
	//     ancestors OUTSIDE it (a nested scroll container's viewport is
	//     fixed in device space — it does not move with its own content)
	// Without this, a layer initially OUTSIDE the viewport (conv-title
	// below conv-list's bottom) intersects the un-shifted padding box to
	// zero and is never painted after being scrolled into view, while a
	// layer initially inside keeps a clip pinned to the OLD viewport bottom
	// and gets culled once scrolled away — "内容初始被裁切的部分滚动后
	// 永远不显示 / 显示错位".
	// ★ 只有「视口固定」的 fixed 才享受 fixed 语义（不被祖先 overflow 裁剪、
	// 不累加祖先滚动偏移——fixed 的包含块恒为视口）。被 transform/filter
	// 祖先捕获的 fixed 的包含块就是那个祖先，必须像 absolute 一样被祖先
	// 裁剪、并跟随祖先变换绘制，见 isViewportFixed。
	isFixed := isViewportFixed(l.owner)
	// ★ CSS 2.1 §11.1.1：overflow 裁剪只作用于「包含块是该裁剪元素自身
	// 或其子孙」的后代；包含块在裁剪祖先之上的定位后代不受其裁剪
	// （经典例子：非定位 body 设 overflow:hidden 且高度为 0，其内
	// absolute 子元素的包含块=视口 —— body 的 0 高裁剪不得生效，
	// 否则整棵 absolute 子树被裁没：border 挂件/形状边框全透明的根因）。
	// 层链上每一级 overflow clip 依次收窄——从最近祖先到包含块之间
	// 应用，越过包含块（更高祖先）即停止。
	isOutOfFlow := false
	var escapeCB RenderObject
	if cs := l.owner.Style(); cs != nil {
		isOutOfFlow = cs.Position == style.PositionAbsolute || cs.Position == style.PositionFixed
		if isOutOfFlow {
			if box := asRenderBox(l.owner); box != nil {
				escapeCB = box.ContainingBlock()
				if isFixed {
					// 视口固定的包含块恒为视口：所有祖先 overflow 都不
					// 适用（paintLayerTree 的 fixed 重置同样按视口处理）。
					// 被 transform 祖先捕获的 fixed 不属于此列——它的
					// 包含块就是那个祖先（box.ContainingBlock() 已正确
					// 返回它），祖先 overflow 照常裁剪。
					escapeCB = nil
				}
			}
		}
	}
	view := l.owner.View()
	totalSX, totalSY := 0.0, 0.0
	if view != nil && !isFixed {
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
				sx, sy := view.BoxScrollOffset(cb)
				totalSX += sx
				totalSY += sy
			}
			if isViewportFixed(cur.owner) {
				break
			}
		}
	}
	ownRect := layerRect
	ownRect.X -= totalSX
	ownRect.Y -= totalSY

	clipRect = layout.LayoutRect{}
	hasClip := false
	cs := l.owner.Style()
	if cs != nil && (cs.OverflowX != style.OverflowVisible || cs.OverflowY != style.OverflowVisible) {
		clipRect = ownRect
		hasClip = true
	}
	// Walk the ancestor layer chain intersecting with each ancestor's overflow clip.
	innerSX, innerSY := 0.0, 0.0
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
			// 定位后代逃逸：包含块在此祖先之上 → 该级 overflow clip 不适用。
			if isOutOfFlow && escapeCB != nil && cbStrictlyAbove(cur.owner, escapeCB) {
				continue
			}
			ancestorRect := cb.PaddingBoxRect()
			sx, sy := 0.0, 0.0
			if view != nil {
				sx, sy = view.BoxScrollOffset(cb)
			}
			// This ancestor's viewport in device space: its content-coord
			// padding box shifted by the scroll of ancestors OUTSIDE it.
			ancestorRect.X -= totalSX - innerSX - sx
			ancestorRect.Y -= totalSY - innerSY - sy
			innerSX += sx
			innerSY += sy
			if !hasClip {
				clipRect = ancestorRect
				hasClip = true
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
		if isViewportFixed(cur.owner) {
			break
		}
	}
	// ★ clipSpecified 语义：返回的 clipRect 是否「必须应用」——即使与祖先
	// overflow clip 的空交被 intersectRects 折叠成零 rect（层完全在祖先
	// 裁剪区外），该层子树也必须被整体裁剪（浏览器语义：无交=全裁）。
	// 之前 paintLayerTree 用 `clip.Width > 0 && clip.Height > 0` 判断
	// hasClip，零 rect 被当成「无 clip」→ 层内容零裁剪平铺到容器外
	// （overflow-y:auto 的 <select> popup 第 9/10 个 option 行溢出容器
	// ——「窗口捕获窗口选择下拉渲染溢出」根因）。
	return layerRect, clipRect, hasClip
}

// cbStrictlyAbove reports whether cb is a strict ancestor of a (a lies inside
// cb's subtree). Used by the overflow-clip escape rule in CalculateRects:
// a positioned descendant whose containing block is strictly above the
// clipping ancestor is not clipped by that ancestor (CSS 2.1 §11.1.1).
func cbStrictlyAbove(a, cb RenderObject) bool {
	for cur := a.Parent(); cur != nil; cur = cur.Parent() {
		if cur == cb {
			return true
		}
	}
	return false
}

// isViewportFixed reports whether box is a position:fixed box that is STILL
// anchored to the viewport — i.e. no ancestor establishes a containing block
// for fixed descendants (transform / filter / backdrop-filter / perspective /
// will-change / containment; the same trigger set layout/positioned.go uses via
// layout.CreatesContainingBlockForFixed).
//
// A fixed box trapped by such an ancestor belongs to that ancestor's
// containing-block chain: layout/positioned.go already resolves its insets
// against that ancestor (`.transformer`'s padding box in fixture
// transform-containing-block), and it must be *painted* inside that ancestor's
// transform space too, exactly like an absolute box. Treating it as
// viewport-fixed dropped the ancestor transform — paintLayerTree's fixed branch
// calls RestoreToCount(info.initialSaveCount) + ResetFixedTransform, discarding
// every ancestor canvas state including the transform matrix — so
// `.fixed-transformed` (left:150; top:100) painted at its untransformed origin
// (305,215) instead of (335,235), i.e. without `.transformer`'s
// translate(30px,20px).
//
// Ancestor overflow clips and ancestor scroll translates apply to such a box as
// well (its containing block is the transformed ancestor), which the callers
// get for free once it stops taking the viewport-fixed path.
func isViewportFixed(box RenderObject) bool {
	if box == nil {
		return false
	}
	st := box.Style()
	if st == nil || st.Position != style.PositionFixed {
		return false
	}
	for p := box.Parent(); p != nil; p = p.Parent() {
		if layout.CreatesContainingBlockForFixed(p.Style()) {
			return false
		}
	}
	return true
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
