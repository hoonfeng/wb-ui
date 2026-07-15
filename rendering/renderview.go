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
	v.syncGeometry()
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
// position.
func syncChildren(parentRO RenderObject, parentLB *layout.LayoutBox) {
	// Build a lookup: for each named (element-backed) layout child, map by element ptr.
	elMap := make(map[*dom.Element]*layout.LayoutBox)
	var anonCandidates []*layout.LayoutBox
	for _, lc := range parentLB.Children {
		if lc.Element != nil {
			elMap[lc.Element] = lc
		} else {
			anonCandidates = append(anonCandidates, lc)
		}
	}
	// Iterate render children and match.
	anonIdx := 0
	for rc := parentRO.FirstChild(); rc != nil; rc = rc.NextSibling() {
		rcEl := domElementOf(rc)
		if rcEl != nil {
			if lc, ok := elMap[rcEl]; ok {
				syncOne(rc, lc)
				continue
			}
		}
		// No element match: match against next anonymous layout child.
		if anonIdx < len(anonCandidates) {
			syncOne(rc, anonCandidates[anonIdx])
			anonIdx++
		}
	}
}

// syncOne copies geometry from the layout box into the render box frame, then recurses.
// For RenderText objects it also copies the inline formatting segments so the paint
// pipeline can draw each text fragment at its correct laid-out position.
func syncOne(ro RenderObject, lb *layout.LayoutBox) {
	if ro == nil || lb == nil {
		return
	}
	if box := asRenderBox(ro); box != nil {
		box.frame = lb.Rect
	}
	// Sync text segments for RenderText (mirrors the line-box list on RenderText).
	if rt, ok := ro.(*RenderText); ok && len(lb.TextSegments) > 0 {
		segs := make([]InlineTextBox, len(lb.TextSegments))
		for i, s := range lb.TextSegments {
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
