// Translation of: Source/WebCore/rendering/RenderView.cpp (RenderView::paint)
//                  Source/WebCore/page/FrameView.cpp (FrameView::paint)
//                  Source/WebCore/rendering/RenderLayer.cpp (RenderLayer::paintLayer)
// Completeness: 80%
// Simplifications:
//   - the paint path paints directly into the GraphicsContext; the DisplayList recorder
//     that modern WebKit builds before flushing is omitted
//   - the paint is split into three global phases (background, foreground, outline)
//     traversed per subtree; WebKit interleaves some of this with PaintBehavior flags
//   - layer compositing is modeled as a recursive layer-tree traversal where each layer
//     clips and paints its owner's bounded subtree; descendant layers owned by child
//     layers are excluded from the parent pass to avoid double-painting
//   - dirty-rect tracking limits which objects are painted; MarkDirty/MarkAllDirty
//     on RenderView manage the damaged region
//   - scroll offset support: SetScrollOffset on RenderView applies a translate
//     before painting so content appears scrolled

package rendering

import (
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
	"wb-ui/widgets"
)

// Paint is the top-level paint entry point, mirroring FrameView::paint() which calls
// RenderView::paint(). It walks the render tree (or, when layers are present, the layer
// tree) and drives the per-phase painters into the supplied canvas. The dirty rect
// limits which objects are rasterized. If the RenderView has a scroll offset, a
// translate is applied before painting so content appears scrolled.
func Paint(view *RenderView, canvas *graphics.Canvas, rect Rect) {
	if view == nil || canvas == nil {
		return
	}
	// Use the view's dirty rect if set; otherwise fall back to the caller's rect.
	paintRect := rect
	if view.IsDirty() {
		paintRect = view.GetDirtyRect()
	}
	info := NewPaintInfo(canvas, paintRect)
	info.rv = view
	info.SetDirtyCheckEnabled(view.IsDirty())

	// Apply scroll offset as a canvas translate.
	scrollX, scrollY := view.ScrollOffset()
	if scrollX != 0 || scrollY != 0 {
		canvas.Save()
		canvas.Translate(-scrollX, -scrollY)
		defer canvas.Restore()
	}

	if view.RootLayer() != nil {
		paintLayerTree(view.RootLayer(), info)
	} else {
		paintSubtreeByPhase(RenderObject(view), info, nil)
	}

	// Clear the dirty rect after painting.
	if view.IsDirty() {
		view.ClearDirty()
	}
}

// paintLayerTree paints a single render layer and its descendants, mirroring
// RenderLayer::paintLayer(). The canvas state is saved, the layer's clip is applied, the
// layer owner's bounded subtree is painted by phase, then each child layer is painted on
// top (composited), and finally the canvas state is restored.
func paintLayerTree(layer *RenderLayer, info *PaintInfo) {
	if layer == nil {
		return
	}
	info.canvas.Save()
	if layerRect, clip := layer.CalculateRects(); clip.Width > 0 && clip.Height > 0 {
		_ = layerRect
		info.canvas.Clip(graphics.Rect{X: clip.X, Y: clip.Y, Width: clip.Width, Height: clip.Height})
	}
	paintLayerContent(layer, info)
	for child := layer.FirstChild(); child != nil; child = child.NextSibling() {
		paintLayerTree(child, info)
	}
	info.canvas.Restore()
}

// paintLayerContent paints the layer owner's subtree in phase order, excluding the
// subtrees rooted at descendants that own a direct child layer of this layer (those are
// painted by their own paintLayerTree recursion).
func paintLayerContent(layer *RenderLayer, info *PaintInfo) {
	owner := layer.Owner()
	if owner == nil {
		return
	}
	excluded := collectChildLayerOwners(layer)
	paintSubtreeByPhase(owner, info, excluded)
}

// collectChildLayerOwners returns the set of render objects that own a direct child layer
// of the given layer. The paint traversal skips these subtrees in the parent pass so they
// are only painted when their own layer is visited.
func collectChildLayerOwners(layer *RenderLayer) map[RenderObject]bool {
	set := map[RenderObject]bool{}
	for child := layer.FirstChild(); child != nil; child = child.NextSibling() {
		if owner := child.Owner(); owner != nil {
			set[owner] = true
		}
	}
	return set
}

// paintSubtreeByPhase walks the subtree rooted at root (inclusive) in pre-order three
// times, once per paint phase, invoking the corresponding per-object painter. Nodes in
// the excluded set are not painted and their subtrees are not descended into (they are
// owned by a child layer painted separately).
func paintSubtreeByPhase(root RenderObject, info *PaintInfo, excluded map[RenderObject]bool) {
	if root == nil {
		return
	}
	info.SetPhase(PhaseBackground)
	walkSubtreeExcluded(root, excluded, info, func(o RenderObject, _ *PaintInfo) { paintObjectBackground(o, info) })
	// Paint selection highlight after backgrounds but before text, so text
	// appears on top of the selection. Only done at the RenderView root.
	if rv, ok := root.(*RenderView); ok {
		PaintSelection(rv, info)
	}
	info.SetPhase(PhaseForeground)
	walkSubtreeExcluded(root, excluded, info, func(o RenderObject, _ *PaintInfo) { paintObjectForeground(o, info) })
	// Paint the caret after the foreground so it appears on top of text.
	if rv, ok := root.(*RenderView); ok {
		PaintCaret(rv, info)
	}
	info.SetPhase(PhaseOutline)
	walkSubtreeExcluded(root, excluded, info, func(o RenderObject, _ *PaintInfo) { paintObjectOutline(o, info) })
}

// walkSubtreeExcluded performs a pre-order traversal of the subtree rooted at root,
// invoking visit for each node except those in excluded (whose subtrees are also
// skipped). When a box has overflow:hidden on both axes, the canvas is saved and
// clipped to the box's padding box before traversing children, then restored
// after all children are done.
func walkSubtreeExcluded(root RenderObject, excluded map[RenderObject]bool, info *PaintInfo, visit func(RenderObject, *PaintInfo)) {
	if root == nil {
		return
	}
	if excluded[root] {
		return
	}
	visit(root, info)

	// Apply overflow:hidden clipping before traversing children.
	var needsClipRestore bool
	if box := asRenderBox(root); box != nil {
		if st := box.Style(); st != nil && st.OverflowX == style.OverflowHidden && st.OverflowY == style.OverflowHidden {
			if info != nil && info.canvas != nil {
				info.canvas.Save()
				pb := box.PaddingBoxRect()
				info.canvas.Clip(graphics.Rect{X: pb.X, Y: pb.Y, Width: pb.Width, Height: pb.Height})
				needsClipRestore = true
			}
		}
	}

	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		walkSubtreeExcluded(c, excluded, info, visit)
	}

	if needsClipRestore {
		info.canvas.Restore()
	}
}

// paintObjectBackground paints the background-color and border for box-bearing objects
// during the background phase, mirroring the Background + Border phase of
// RenderBox::paint().
func paintObjectBackground(o RenderObject, info *PaintInfo) {
	box := asRenderBox(o)
	if box == nil || !box.IsVisible() {
		return
	}
	// Apply CSS filter: wrap painting in a SaveLayer with ImageFilter.
	var filterCleanup func()
	if st := box.Style(); st != nil && st.Filter != "" && st.Filter != "none" {
		filters := parseCSSFilters(st.Filter)
		if imgFilter := buildCSSFilterChain(filters); imgFilter != nil {
			info.canvas.SaveLayerWithFilter(imgFilter)
			filterCleanup = info.canvas.Restore
		}
	}
	// Apply CSS transform if present (inside filter layer).
	if cleanup := tryApplyTransform(info.canvas, box); cleanup != nil {
		defer cleanup()
	}
	PaintBackground(box, info)
	PaintBorder(box, info)
	if filterCleanup != nil {
		defer filterCleanup()
	}
}
// <wb-editor> custom elements by delegating to the editor package's painter, and
// native form controls (checkbox/radio/range/progress/meter/select arrow) via
// PaintFormControl mirroring RenderTheme::paint().
func paintObjectForeground(o RenderObject, info *PaintInfo) {
	if text, ok := o.(*RenderText); ok {
		PaintText(text, info)
		return
	}
	// Check for <wb-editor> custom elements.
	box := asRenderBox(o)
	if box == nil || !box.IsVisible() {
		return
	}
	el, ok := box.Node().(*dom.Element)
	if !ok {
		return
	}
	if widgets.IsEditorElement(el) && info.rv != nil {
		registry := info.rv.EditorRegistry()
		registry.PaintEditor(el, info.canvas, box.X(), box.Y(), box.Width(), box.Height())
		return
	}
	// SVG elements: parse and paint shapes.
	if el.LocalName() == "svg" {
		doc := buildSVGDocument(el)
		if doc != nil && len(doc.shapes) > 0 {
			paintSVG(info.canvas, doc, box.X(), box.Y(), graphics.Color{})
		}
		return
	}
	// Native form controls (checkbox/radio/range/progress/meter/select arrow).
	// PaintFormControl returns true when it fully handled the element (so the
	// default text path is skipped); false means fall through to normal painting.
	if PaintFormControl(box, info) {
		return
	}
	// Image elements (<img>): paint the decoded image if one is attached.
	if el.LocalName() == "img" {
		PaintImage(box, info)
		return
	}
}

// paintObjectOutline paints the outline for box-bearing objects during the outline
// phase, mirroring the Outline phase of RenderBox::paint().
func paintObjectOutline(o RenderObject, info *PaintInfo) {
	box := asRenderBox(o)
	if box == nil || !box.IsVisible() {
		return
	}
	PaintOutline(box, info)
}

// PaintRenderObject is a per-object entry point that dispatches to the right painter for
// the current phase. It is exposed so that tests (and a future incremental repaint path)
// can drive a single object's painting without walking the whole tree, mirroring the
// RenderObject::paint() virtual dispatch.
func PaintRenderObject(o RenderObject, info *PaintInfo) {
	if o == nil || info == nil {
		return
	}
	switch info.Phase() {
	case PhaseBackground:
		paintObjectBackground(o, info)
	case PhaseForeground:
		paintObjectForeground(o, info)
	case PhaseOutline:
		paintObjectOutline(o, info)
	}
}
