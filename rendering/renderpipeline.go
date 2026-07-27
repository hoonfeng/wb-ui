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
//
// Save hierarchy (two-level nesting):
//   Level 1 (outer): clip to padding box (overflow: hidden/auto/scroll)
//   Level 2 (inner): translate for per-box scroll offset
// Children are painted with both clip + translate active.
// After restoring Level 2, overflow controls (scrollbars, text-overflow ellipsis)
// are painted at Level 1 (clipped but NOT translated), matching browser behavior
// where scrollbars stay fixed at the padding-box edges regardless of scroll offset.
func walkSubtreeExcluded(root RenderObject, excluded map[RenderObject]bool, info *PaintInfo, visit func(RenderObject, *PaintInfo)) {
	if root == nil {
		return
	}
	if excluded[root] {
		return
	}
	visit(root, info)

	// Determine overflow/clip and scroll offset for this box.
	var clipBox *RenderBox
	var scrollSX, scrollSY float64
	if box := asRenderBox(root); box != nil {
		if st := box.Style(); st != nil &&
			(st.OverflowX == style.OverflowHidden ||
				st.OverflowX == style.OverflowAuto ||
				st.OverflowX == style.OverflowScroll) &&
			(st.OverflowY == style.OverflowHidden ||
				st.OverflowY == style.OverflowAuto ||
				st.OverflowY == style.OverflowScroll) {
			clipBox = box
		}
		if info.rv != nil {
			sx, sy := info.rv.BoxScrollOffset(box)
			if sx != 0 || sy != 0 {
				scrollSX, scrollSY = sx, sy
			}
		}
	}

	// Level 1: apply overflow clip (outer save).
	needsClipRestore := false
	if clipBox != nil && info != nil && info.canvas != nil {
		info.canvas.Save()
		pb := clipBox.PaddingBoxRect()
		info.canvas.Clip(graphics.Rect{X: pb.X, Y: pb.Y, Width: pb.Width, Height: pb.Height})
		needsClipRestore = true
	}

	// Level 2: apply scroll translate (inner save — nested inside clip save).
	needsScrollRestore := false
	if (scrollSX != 0 || scrollSY != 0) && info != nil && info.canvas != nil {
		if needsClipRestore {
			info.canvas.Save() // nested inside clip Save
			info.canvas.Translate(-scrollSX, -scrollSY)
			needsScrollRestore = true
		} else {
			info.canvas.Save()
			info.canvas.Translate(-scrollSX, -scrollSY)
			needsScrollRestore = true
		}
	}

	// Paint children (inside clip + translate).
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		walkSubtreeExcluded(c, excluded, info, visit)
	}

	// Restore Level 2 (scroll translate) — now we're back in clip-only state.
	if needsScrollRestore {
		info.canvas.Restore()
	}

	// ── Overflow controls (painted in clip-only state, no translate) ──
	if needsClipRestore && info.Phase() == PhaseForeground {
		info.textOverflowEllipsisPainted = false
		if box := asRenderBox(root); box != nil {
			st := box.Style()
			if st == nil {
				goto restoreClip
			}

			// ── Scroll bars (modern flat style) ──
			needsScroll := (st.OverflowX == style.OverflowScroll || st.OverflowY == style.OverflowScroll ||
				st.OverflowX == style.OverflowAuto || st.OverflowY == style.OverflowAuto)
			if needsScroll {
				pb := box.PaddingBoxRect()
				// Modern flat scrollbar: 12px wide, subtle arrow buttons, rounded rect thumb.
const scrollW = 12.0    // total scrollbar width
						const arrowSize = 12.0  // arrow button height/width
						const arrowGap = 5.0   // gap between arrow buttons and thumb track

				if pb.Width > scrollW*2 && pb.Height > scrollW*2 {
					if info.rv != nil {
						cw, ch := info.rv.BoxContentSize(box)
						totalW := cw
						totalH := ch
						contentW := pb.Width
						contentH := pb.Height

						needsV := (st.OverflowY == style.OverflowScroll || (st.OverflowY == style.OverflowAuto && totalH > contentH)) && st.OverflowY != style.OverflowHidden
						needsH := (st.OverflowX == style.OverflowScroll || (st.OverflowX == style.OverflowAuto && totalW > contentW)) && st.OverflowX != style.OverflowHidden

						if needsV || needsH {

							// Opaque light gray scrollbar colors.
							trackCol := graphics.Color{R: 255, G: 255, B: 255, A: 255}   // #FFFFFF white track
							thumbCol := graphics.Color{R: 192, G: 192, B: 192, A: 255}   // #C0C0C0 thumb
							thumbHoverCol := graphics.Color{R: 160, G: 160, B: 160, A: 255} // #A0A0A0 hover

							arrowCol := graphics.Color{R: 128, G: 128, B: 128, A: 255}   // #808080 arrow

							sx, sy := float64(0), float64(0)
							cursorX, cursorY := float64(0), float64(0)
							if info.rv != nil {
								sx, sy = info.rv.BoxScrollOffset(box)
								cursorX, cursorY = info.rv.CursorPos()
							}

// ── Vertical scrollbar ──
						if needsV {
							vx := pb.X + pb.Width - scrollW
							vy := pb.Y
							vh := pb.Height
							if needsH {
								vh -= scrollW
							}
							if vh <= arrowSize*2+arrowGap*2 {
								goto endV
							}

							// Track background.
							info.canvas.FillRect(vx, vy, scrollW, vh, trackCol)

// Up arrow: rounded triangle (fill + circles at vertices).
						upBtnY := vy
						acx := vx + scrollW/2
												acy := upBtnY + arrowSize/2
						info.canvas.FillRoundedTriangle(acx, acy-2, acx-3, acy+3, acx+3, acy+3, 1.2, arrowCol)

						// Down arrow.
						dnBtnY := vy + vh - arrowSize
						dcy := dnBtnY + arrowSize/2
						info.canvas.FillRoundedTriangle(acx, dcy+2, acx-3, dcy-3, acx+3, dcy-3, 1.2, arrowCol)

							// Thumb (rounded rect, pill shape).
							if totalH > contentH {
								trackH := vh - arrowSize*2 - arrowGap*2
								thumbLen := trackH * contentH / totalH
								if thumbLen < 18 { thumbLen = 18 }
								if thumbLen > trackH-4 { thumbLen = trackH - 4 }
								maxSy := totalH - contentH
								if maxSy <= 0 { maxSy = 1 }
								syRatio := sy / maxSy
								thumbTrackSpace := trackH - thumbLen
								thumbY := vy + arrowSize + arrowGap + syRatio*thumbTrackSpace

								isHover := cursorX >= vx && cursorX <= vx+scrollW &&
									cursorY >= thumbY && cursorY <= thumbY+thumbLen
								tCol := thumbCol
								if isHover { tCol = thumbHoverCol }

								info.canvas.FillRoundRect(vx+2, thumbY, scrollW-4, thumbLen, 5, tCol)
							}
						}
						endV:

													// ── Horizontal scrollbar ──
							if needsH {
								hx := pb.X
								hy := pb.Y + pb.Height - scrollW
								hw := pb.Width
								if needsV {
									hw -= scrollW
								}
								if hw <= arrowSize*2+arrowGap*2 {
									goto endH
								}

								// Track background.
								info.canvas.FillRect(hx, hy, hw, scrollW, trackCol)

												// Left arrow.
						ltBtnX := hx
						aCy := hy + scrollW/2
						info.canvas.FillRoundedTriangle(ltBtnX+5, aCy, ltBtnX+arrowSize-6, aCy-3, ltBtnX+arrowSize-6, aCy+3, 1.2, arrowCol)

						// Right arrow.
						rtBtnX := hx + hw - arrowSize
						info.canvas.FillRoundedTriangle(rtBtnX+arrowSize-5, aCy, rtBtnX+6, aCy-3, rtBtnX+6, aCy+3, 1.2, arrowCol)

								// Thumb.
								if totalW > contentW {
									trackW := hw - arrowSize*2 - arrowGap*2
									thumbLen := trackW * contentW / totalW
									if thumbLen < 18 { thumbLen = 18 }
									if thumbLen > trackW-4 { thumbLen = trackW - 4 }
									maxSx := totalW - contentW
									if maxSx <= 0 { maxSx = 1 }
									sxRatio := sx / maxSx
									thumbTrackSpace := trackW - thumbLen
									thumbX := hx + arrowSize + arrowGap + sxRatio*thumbTrackSpace

									isHover := cursorY >= hy && cursorY <= hy+scrollW &&
										cursorX >= thumbX && cursorX <= thumbX+thumbLen
									tCol := thumbCol
									if isHover { tCol = thumbHoverCol }

									info.canvas.FillRoundRect(thumbX, hy+2, thumbLen, scrollW-4, 5, tCol)
								}
							}
							endH:

							// Corner fill.
							if needsV && needsH {
								cx := pb.X + pb.Width - scrollW
								cy := pb.Y + pb.Height - scrollW
								info.canvas.FillRect(cx, cy, scrollW, scrollW, trackCol)
							}
						}
					}
				}
			}
		}
	}

restoreClip:
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

// walkRenderTextForBaseline walks the render subtree to find the first text
// segment and returns its Y position (0 if none found). Used to align the
// text-overflow ellipsis with the actual text baseline.
func walkRenderTextForBaseline(ro RenderObject) float64 {
	if ro == nil {
		return 0
	}
	if rt, ok := ro.(*RenderText); ok {
		segs := rt.Segments()
		if len(segs) > 0 {
			return segs[0].Y
		}
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if y := walkRenderTextForBaseline(c); y != 0 {
			return y
		}
	}
	return 0
}

// findLastTextSegmentRight walks the render subtree and returns the rightmost
// X coordinate of the last text segment found. Returns 0 if no text exists.
// Used by text-overflow:ellipsis to position "..." immediately after the last
// visible text, matching browser behavior.
func findLastTextSegmentRight(ro RenderObject) float64 {
	if ro == nil {
		return 0
	}
	if rt, ok := ro.(*RenderText); ok {
		segs := rt.Segments()
		if len(segs) > 0 {
			last := segs[len(segs)-1]
			return last.X + last.Width
		}
	}
	var right float64
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if r := findLastTextSegmentRight(c); r > right {
			right = r
		}
	}
	return right
}

// findLastTextSegmentRightBounded is like findLastTextSegmentRight but only
// considers segments whose X falls within [boundLeft, boundRight). Pass
// boundLeft=boundRight=0 to skip bounds checking.
func findLastTextSegmentRightBounded(ro RenderObject, boundLeft, boundRight float64) float64 {
	if ro == nil {
		return 0
	}
	if rt, ok := ro.(*RenderText); ok {
		segs := rt.Segments()
		if len(segs) > 0 {
			for i := len(segs) - 1; i >= 0; i-- {
				seg := segs[i]
				if boundLeft == 0 && boundRight == 0 {
					return seg.X + seg.Width
				}
				if seg.X >= boundLeft && seg.X < boundRight {
					return seg.X + seg.Width
				}
			}
			return 0
		}
	}
	var right float64
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if r := findLastTextSegmentRightBounded(c, boundLeft, boundRight); r > right {
			right = r
		}
	}
	return right
}
