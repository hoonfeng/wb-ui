// Translation of: Source/WebCore/rendering/RenderView.h
//                  Source/WebCore/rendering/RenderView.cpp
// Completeness: 55%
// Simplifications:
//   - the FrameView / Page integration is omitted (Phase 9)
//   - the compositor is owned by RenderView but is a lightweight struct in this port
//     rather than the full RenderLayerCompositor state machine
//   - the repaint / paint-path entry points are stubbed; painting is Phase 8
//   - the viewport scroll / fixed-position containment is simplified

package rendering

import (
	"fmt"
	"os"
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/style"
	"wb-ui/widgets"
)

// RenderView is the Go translation of WebCore::RenderView. It is the root of the render
// tree: it corresponds to the Document node and owns the viewport size, the layer
// compositor and the top-level layout state. Every other render object reaches the
// RenderView via the View() method.
type RenderView struct {
	RenderBlockFlow
	document    *dom.Document
	viewWidth   float64
	viewHeight  float64
	compositor  *RenderLayerCompositor
	rootLayer   *RenderLayer
	layoutState *layout.LayoutState
	// editorRegistry manages <wb-editor> instances. It is lazily
	// initialized on first access via EditorRegistry().
	editorRegistry *widgets.EditorRegistry

	// dirtyRect tracks the damaged area that needs repainting on the next
	// Paint pass. A zero-width/height rect means nothing is dirty.
	dirtyRect Rect

	// scrollOffset tracks the current scroll position. The paint pipeline
	// applies -scrollOffset as a canvas translate so content appears
	// scrolled. Set via SetScrollOffset.
	scrollOffsetX, scrollOffsetY float64
}

// NewRenderView constructs a RenderView for the given document. The viewport size is
// initialized to zero; the caller should set it before invoking Layout.
func NewRenderView(doc *dom.Document, st *style.ComputedStyle) *RenderView {
	rv := &RenderView{document: doc}
	rv.initBase(rv, doc, st)
	rv.compositor = NewRenderLayerCompositor(rv)
	rv.editorRegistry = widgets.NewEditorRegistry()
	return rv
}

// EditorRegistry returns the registry that manages <wb-editor> views.
func (v *RenderView) EditorRegistry() *widgets.EditorRegistry {
	return v.editorRegistry
}

// Type returns ObjectView.
func (v *RenderView) Type() RenderObjectType { return ObjectView }

// IsRenderView reports that this object is the RenderView root, mirroring
// RenderObject::isRenderView().
func (v *RenderView) IsRenderView() bool { return true }

// RenderName returns a debug name for the view.
func (v *RenderView) RenderName() string { return "RenderView" }

// Document returns the document associated with this render view, mirroring
// RenderView::document().
func (v *RenderView) Document() *dom.Document { return v.document }

// View returns itself, mirroring RenderObject::view() which short-circuits for the
// root.
func (v *RenderView) View() *RenderView { return v }

// ViewWidth / ViewHeight return the viewport dimensions, mirroring
// RenderView::viewWidth() / viewHeight().
func (v *RenderView) ViewWidth() float64  { return v.viewWidth }
func (v *RenderView) ViewHeight() float64 { return v.viewHeight }

// MarkDirty marks the given rectangle as needing repainting. The new rect is
// unioned with any existing dirty rect.
func (v *RenderView) MarkDirty(r Rect) {
	if v.dirtyRect.Width <= 0 || v.dirtyRect.Height <= 0 {
		v.dirtyRect = r
		return
	}
	// Union: expand to include r.
	x0 := min2(v.dirtyRect.X, r.X)
	y0 := min2(v.dirtyRect.Y, r.Y)
	x1 := max2(v.dirtyRect.X+v.dirtyRect.Width, r.X+r.Width)
	y1 := max2(v.dirtyRect.Y+v.dirtyRect.Height, r.Y+r.Height)
	v.dirtyRect = Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

// MarkAllDirty marks the entire viewport as needing repainting.
func (v *RenderView) MarkAllDirty() {
	v.dirtyRect = Rect{X: 0, Y: 0, Width: v.viewWidth, Height: v.viewHeight}
}

// ClearDirty clears the dirty rect (called after painting).
func (v *RenderView) ClearDirty() {
	v.dirtyRect = Rect{}
}

// GetDirtyRect returns the current dirty rect.
func (v *RenderView) GetDirtyRect() Rect { return v.dirtyRect }

// IsDirty reports whether any area needs repainting.
func (v *RenderView) IsDirty() bool { return v.dirtyRect.Width > 0 && v.dirtyRect.Height > 0 }

// SetScrollOffset sets the scroll position. The paint pipeline applies a
// -scrollOffset translate so rendered content appears scrolled.
func (v *RenderView) SetScrollOffset(x, y float64) {
	v.scrollOffsetX = x
	v.scrollOffsetY = y
	// Mark the entire viewport as dirty so the next Paint re-renders.
	if v.viewWidth > 0 && v.viewHeight > 0 {
		v.MarkAllDirty()
	} else {
		// Fallback: mark a 1x1 area so IsDirty() returns true.
		v.dirtyRect = Rect{X: 0, Y: 0, Width: 1, Height: 1}
	}
}

// ScrollOffset returns the current scroll position.
func (v *RenderView) ScrollOffset() (float64, float64) {
	return v.scrollOffsetX, v.scrollOffsetY
}

// SetViewportSize sets the viewport dimensions, mirroring
// RenderView::setFrameViewSize() (which propagates the frame view size).
func (v *RenderView) SetViewportSize(w, h float64) {
	if v.viewWidth != w || v.viewHeight != h {
		v.viewWidth = w
		v.viewHeight = h
		v.Dirty()
	}
}

// RootLayer returns the root render layer, mirroring RenderView::layer().
func (v *RenderView) RootLayer() *RenderLayer { return v.rootLayer }

// SetRootLayer installs the root render layer.
func (v *RenderView) SetRootLayer(l *RenderLayer) { v.rootLayer = l }

// Compositor returns the layer compositor, mirroring RenderView::compositor().
func (v *RenderView) Compositor() *RenderLayerCompositor { return v.compositor }

// Layout lays out the entire render tree. It creates a LayoutState with the viewport
// dimensions, stretches the root to fill the viewport and dispatches to the block flow
// layout, mirroring RenderView::layout().
//
// 降级保护：如果 LayoutBox 未设置（为 nil），会尝试从 document 的根元素构建布局树。
// 这覆盖了 attachLayoutTree 因 resolver 为 nil 或 BuildLayoutTree 返回 nil 而跳过
// 的情况，使 RenderView 即使在没有显式构建布局树时也能完成布局。
func (v *RenderView) Layout(state *layout.LayoutState) {
	if state == nil {
		state = layout.NewLayoutState(v.viewWidth, v.viewHeight)
	}
	v.layoutState = state
	// Stretch the root to the viewport.
	v.frame.X = 0
	v.frame.Y = 0
	v.frame.Width = v.viewWidth
	v.frame.Height = v.viewHeight

	// 降级保护：如果 LayoutBox 未设置，尝试从 document 构建
	if v.LayoutBox() == nil && v.document != nil {
		if root := v.document.DocumentElement(); root != nil {
			// 使用默认 resolver（注意：这里可能没有完整的样式信息，但至少让布局能跑）
			defaultResolver := style.NewResolver()
			defaultResolver.AddStyleSheet(html5.NewUAStyleSheet())
			layoutRoot := layout.BuildLayoutTree(root, defaultResolver)
			if layoutRoot != nil {
				v.SetLayoutBox(layoutRoot)
			}
		}
	}

	// Dispatch to the block flow layout for children.
	v.RenderBlockFlow.Layout(state)
	// Sync geometry from layout boxes back to render boxes so that the paint
	// pipeline reads the correct positions. Layout writes to layoutBox.Rect but
	// painters read from RenderBox.frame; this step bridges the gap.
	// Debug: verify rp-body layout box after syncGeometry
	func() {
		var walk func(ro RenderObject, depth int)
		walk = func(ro RenderObject, depth int) {
			if ro == nil {
				return
			}
			if el := domElementOf(ro); el != nil {
				if cls := el.GetAttribute("class"); cls == "rp-body" || cls == "right-panel" {
					lb := ro.LayoutBox()
					fn := "nil"
					if lb != nil {
						fn = fmt.Sprintf("%.0fx%.0f", lb.Rect.Width, lb.Rect.Height)
					}
					fmt.Fprintf(os.Stderr, "[POSTSYNC] cls=%s ro=%p lb=%p lb.rect=%s node=%v\n",
						cls, ro, lb, fn, ro.Node() != nil)
				}
			}
			for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c, depth+1)
			}
		}
		walk(v, 0)
	}()
	// Update compositing layers after layout.
	if v.compositor != nil {
		v.compositor.UpdateCompositingLayers()
	}
}

// LayoutState returns the layout state from the last layout pass, mirroring
// RenderView::layoutState().
func (v *RenderView) LayoutState() *layout.LayoutState { return v.layoutState }

// syncGeometry copies geometry from the layout tree to the render tree after a layout
// pass. Layout writes positions/sizes to layoutBox.Rect, but painters read from
// RenderBox.frame. Because the layout tree and render tree are built by the same
// buildChildren logic but start from different roots (RenderView wraps the document,
// while the layout tree starts at <html>), the two trees are offset by one level: the
// RenderView's first (and only direct) child is the <html> render object, which must be
// matched against the layout root itself — not against the layout root's children.
//
// When LayoutBox is nil (no layout tree was built), this is a no-op. The Layout method
// should have already handled the nil case by either self-healing or setting a default
// frame and returning early, so in practice syncGeometry should only be called when a
// layout tree exists.
func (v *RenderView) syncGeometry() {
	layoutRoot := v.LayoutBox()
	if layoutRoot == nil {
		return
	}
	// The RenderView's frame comes from the <html> layout box (the layout tree root).
	if box := asRenderBox(v); box != nil {
		box.frame = layoutRoot.Rect
	}
	// The RenderView has one child: the <html> render object. Match it against the
	// full layout root (the <html> layout box) since the trees are offset by one level.
	for rc := v.FirstChild(); rc != nil; rc = rc.NextSibling() {
		syncOne(rc, layoutRoot)
		// Only one direct child expected; break after first.
		break
	}
}

// syncChildren pairs parentRO's children with parentLB's children in sibling order and
// copies geometry. Both trees are produced by the same buildChildren logic, so their
// sibling sequences are identical. Named elements match by DOM element identity;
// anonymous wrappers (no DOM element) match to layout children with nil Element by
// position, using sameOwner for robust pairing.
func syncChildren(parentRO RenderObject, parentLB *layout.LayoutBox) {
	// Copy layout children so we can remove matched ones.
	lChildren := make([]*layout.LayoutBox, len(parentLB.Children))
	copy(lChildren, parentLB.Children)

	for rc := parentRO.FirstChild(); rc != nil && len(lChildren) > 0; rc = rc.NextSibling() {
		matched := -1
		for i, lc := range lChildren {
			if sameOwner(rc, lc) {
				matched = i
				break
			}
		}
		if matched >= 0 {
			syncOne(rc, lChildren[matched])
			lChildren = append(lChildren[:matched], lChildren[matched+1:]...)
		} else if roIsAnonymous(rc) {
			// Anonymous render wrapper has no counterpart in the layout tree.
			// Skip without removing from layout children list so subsequent
			// render children match correctly.
			continue
		}
	}
}

// roIsAnonymous reports whether a render object is an anonymous wrapper with
// no backing DOM element.
func roIsAnonymous(ro RenderObject) bool {
	return ro.Node() == nil
}

// syncOne copies geometry from the layout box into the render box frame, then recurses.
// For RenderText objects it also copies the inline formatting segments so the paint
// pipeline can draw each text fragment at its correct laid-out position.
func syncOne(ro RenderObject, lb *layout.LayoutBox) {
	if ro == nil || lb == nil {
		return
	}
	// Clamp negative dimensions: some layout edge cases can produce negative
	// widths/heights (e.g. inline text in a narrow flex container). A negative
	// frame rect causes downstream issues in paint clipping and hit-testing.
	if lb.Rect.Width < 0 {
		lb.Rect.Width = 0
	}
	if lb.Rect.Height < 0 {
		lb.Rect.Height = 0
	}
	if box := asRenderBox(ro); box != nil {
		box.frame = lb.Rect
	}
	// Always update the layout box to reflect the laid-out geometry from
	// this layout pass (overwrites any stale pointer from linkLayoutBoxes).
	ro.SetLayoutBox(lb)
	// Sync text segments for RenderText (mirrors the line-box list on RenderText).
	// Boxes created by the layout tree may wrap text runs in BoxAnonymous boxes;
	// search recursively for the inner BoxTextRun to retrieve laid-out segments.
	textLB := lb
	if len(textLB.TextSegments) == 0 {
		textLB = findTextRun(textLB)
	}
	if rt, ok := ro.(*RenderText); ok && textLB != nil && len(textLB.TextSegments) > 0 {
		segs := make([]InlineTextBox, len(textLB.TextSegments))
		for i, s := range textLB.TextSegments {
			segs[i] = InlineTextBox{
				Start: s.Start, Len: s.Len,
				X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
				LineY: s.LineY, LineHeight: s.LineHeight,
			}
		}
		rt.SetSegments(segs)
	}
	syncChildren(ro, lb)
}

// domElementOf returns the *dom.Element backing a render object, or nil for anonymous
// wrappers and text nodes.
func domElementOf(ro RenderObject) *dom.Element {
	if ro == nil || ro.Node() == nil {
		return nil
	}
	el, _ := ro.Node().(*dom.Element)
	return el
}

// findTextRun recursively searches a layout box subtree for the first BoxTextRun
// with non-empty TextSegments. Used to retrieve text segment data when the
// sync matching paired a RenderText with an anonymous wrapper.
func findTextRun(lb *layout.LayoutBox) *layout.LayoutBox {
	if lb == nil {
		return nil
	}
	if len(lb.TextSegments) > 0 {
		return lb
	}
	for _, c := range lb.Children {
		if found := findTextRun(c); found != nil {
			return found
		}
	}
	return nil
}
