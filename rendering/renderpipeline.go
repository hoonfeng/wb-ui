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
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
	"wb-ui/widgets"

	"github.com/hoonfeng/goskia/skia"
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
//
// Child layers are painted in CSS stacking order (CSS 2.1 §9.9 / Appendix E):
//   1. child layers with negative z-index, most-negative first
//   2. child layers with z-index:auto (or z-index:0 in a stacking context),
//      in tree order — this pass paints the layer's own subtree as well
//   3. child layers with positive z-index, smallest first
// This mirrors RenderLayer::paintLayer / paintLayerContents ordering.
func paintLayerTree(layer *RenderLayer, info *PaintInfo) {
	if layer == nil {
		return
	}
	info.canvas.Save()
	// Fixed-position layers paint against the viewport: reset the inherited
	// ancestor clip so a dialog-overlay / menu inside an overflow:auto
	// container (e.g. sidebar-content) still covers the whole window.
	// The browser never clips fixed elements by ancestor overflow unless
	// that ancestor establishes a containing block (transform/filter).
	isFixedLayer := false
	if layer.Owner() != nil {
		if st := layer.Owner().Style(); st != nil {
			isFixedLayer = st.Position == style.PositionFixed
		}
	}
	if isFixedLayer {
		info.canvas.ResetClip()
		// Fixed layers paint against the viewport: discard inherited
		// scroll translates (page-level and per-box) while keeping the
		// device content scale. Without this a dialog inside a scrolled
		// sidebar-content renders at the sidebar's scrolled position.
		info.canvas.ResetFixedTransform()
		// The fixed element's OWN overflow still clips its subtree.
		if st := layer.Owner().Style(); st != nil &&
			(st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible) {
			if rb := asRenderBox(layer.Owner()); rb != nil {
				pb := rb.PaddingBoxRect()
				info.canvas.Clip(graphics.Rect{X: pb.X, Y: pb.Y, Width: pb.Width, Height: pb.Height})
			}
		}
	} else if layerRect, clip := layer.CalculateRects(); clip.Width > 0 && clip.Height > 0 {
		_ = layerRect
		info.canvas.Clip(graphics.Rect{X: clip.X, Y: clip.Y, Width: clip.Width, Height: clip.Height})
	}
	paintLayerContent(layer, info)

	// Collect child layers and bucket them by stacking position.
	var neg, auto, pos []*RenderLayer
	for child := layer.FirstChild(); child != nil; child = child.NextSibling() {
		z := layerZIndex(child)
		switch {
		case z < 0:
			neg = append(neg, child)
		case z > 0:
			pos = append(pos, child)
		default:
			auto = append(auto, child)
		}
	}
	// Negative z-index: most negative first (e.g. -2 before -1).
	sortLayerByZ(neg, true)
	// Positive z-index: smallest first (e.g. 1 before 2, so 2 paints on top).
	sortLayerByZ(pos, true)
	// Auto/zero layers stay in tree order (already collected in order).

	for _, child := range neg {
		paintLayerTree(child, info)
	}
	for _, child := range auto {
		paintLayerTree(child, info)
	}
	for _, child := range pos {
		paintLayerTree(child, info)
	}
	info.canvas.Restore()
}

// layerZIndex returns the owner's effective z-index for stacking. A z-index only
// participates in stacking when the layer's owner is positioned (CSS 2.1 §10.6)
// or the layer establishes a stacking context (opacity/transform/filter/overflow);
// otherwise it behaves as auto (0).
func layerZIndex(layer *RenderLayer) int {
	if layer == nil || layer.owner == nil {
		return 0
	}
	st := layer.owner.Style()
	if st == nil {
		return 0
	}
	// Non-positioned elements ignore z-index unless they create a stacking
	// context via opacity/transform/filter/overflow.
	positioned := st.Position != style.PositionStatic
	stackingCtx := st.Opacity < 1.0 || st.Transform != "" || st.Filter != "" ||
		st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible
	if !positioned && !stackingCtx {
		return 0
	}
	return st.ZIndex
}

// sortLayerByZ sorts layers by owner z-index ascending (desc=true for negative
// buckets, which want most-negative first = ascending). Stable so tree order is
// preserved for equal z-index values.
func sortLayerByZ(layers []*RenderLayer, ascending bool) {
	sort.SliceStable(layers, func(i, j int) bool {
		zi, zj := layerZIndex(layers[i]), layerZIndex(layers[j])
		if ascending {
			return zi < zj
		}
		return zi > zj
	})
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

	// CSS sticky: apply the scroll-pinning translate OUTSIDE the box's own
	// transform and overflow clip, mirroring RenderBox::stickyPositionOffset
	// applied by the nearest scrolling ancestor during paint.
	needsStickyRestore := false
	if rb := asRenderBox(root); rb != nil && info != nil && info.canvas != nil && info.rv != nil {
		if rb.IsStickyPositioned() {
			if dx, dy := computeStickyOffset(rb, info.rv); dx != 0 || dy != 0 {
				info.canvas.Save()
				info.canvas.Translate(dx, dy)
				needsStickyRestore = true
			}
		}
	}

	// CSS transform: applied OUTSIDE the overflow clip (transform acts on the
	// whole box including its clip). The box's own background AND all
	// descendants paint inside the transformed space, so visit() is called
	// after applying it.
	needsTransformRestore := false
	if box := asRenderBox(root); box != nil && info != nil && info.canvas != nil {
		if st := box.Style(); st != nil && st.Transform != "" && st.AnimationName == "" {
			info.canvas.Save()
			// CSS transforms rotate/scale around the element's
			// transform-origin (default 50% 50% = box center), but the
			// canvas primitives operate around the origin. Compose:
			// T(origin) · ops · T(-origin).
			originX, originY := box.X(), box.Y()
			if ox := resolveTransformOrigin(st.TransformOriginX, box.Width()); ox >= 0 {
				originX += ox
			}
			if oy := resolveTransformOrigin(st.TransformOriginY, box.Height()); oy >= 0 {
				originY += oy
			}
			info.canvas.Translate(originX, originY)
			if applyTransformOps(info.canvas, st.Transform) {
				info.canvas.Translate(-originX, -originY)
				needsTransformRestore = true
			} else {
				info.canvas.Restore()
			}
		}
	}

	visit(root, info)

	// Determine overflow/clip and scroll offset for this box.
	var clipBox *RenderBox
	var scrollSX, scrollSY float64
	if box := asRenderBox(root); box != nil {
		// Clip when EITHER axis scrolls/clips (overflow-y:auto alone must
		// still clip + paint scrollbars). A rectangular clip is harmless for
		// the axis that does not overflow.
		if st := box.Style(); st != nil &&
			(st.OverflowX == style.OverflowHidden ||
				st.OverflowX == style.OverflowAuto ||
				st.OverflowX == style.OverflowScroll ||
				st.OverflowY == style.OverflowHidden ||
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
				scrollW := 12.0 // total scrollbar width
				const arrowSize = 12.0 // arrow button height/width
				const arrowGap = 5.0   // gap between arrow buttons and thumb track

				// CSS scrollbar-width: thin (8px) / none (hidden, still scrollable).
				switch sw := st.GetProperty("scrollbar-width"); sw {
				case "thin":
					scrollW = 8
				case "none":
					scrollW = 0
				}
				// ::-webkit-scrollbar { width: Npx } — Blink/WebKit custom width
				// (GitPanel uses 4px). Overrides the default (and scrollbar-width
				// thin above when both present, mirroring Chrome where
				// ::-webkit-scrollbar wins over the standard property).
				if wv := st.GetProperty("-webkit-scrollbar-width"); wv != "" {
					if l, ok := parseLengthAny(wv); ok && l > 0 {
						scrollW = l
					}
				}
				// WebKit/Blink custom scrollbars have NO arrow buttons and the
				// thumb fills the full scrollbar width (only the default flat
				// 12px style keeps arrows + inset thumb).
				webkitSB := webkitCustomScrollbar(st)

				if style.DiagEnabled("scrollbar") {
					cn := ""
					if el, ok := box.Node().(*dom.Element); ok {
						cn = el.GetAttribute("class")
					}
					style.Diagf("scrollbar", "ENTER %q: ovfX=%d ovfY=%d scrollW=%.0f webkitSB=%v sw=%q sbc=%q",
						cn, st.OverflowX, st.OverflowY, scrollW, webkitSB,
						st.GetProperty("scrollbar-width"), st.GetProperty("scrollbar-color"))
				}

				if scrollW > 0 && pb.Width > scrollW*2 && pb.Height > scrollW*2 {
					if info.rv != nil {
						cw, ch := info.rv.BoxContentSize(box)
						totalW := cw
						totalH := ch

						// Scroll viewport is the CONTENT box (padding-box
						// minus padding) — the text area the painter clips to.
						// needsV/needsH and thumb geometry share this viewport
						// so empty content (totalH ≤ viewH) never enables a
						// scrollbar just because padding fills the box.
						padL := lengthValue(st.PaddingLeft)
						padR := lengthValue(st.PaddingRight)
						padT := lengthValue(st.PaddingTop)
						padB := lengthValue(st.PaddingBottom)
						if padL < 0 {
							padL = 0
						}
						if padR < 0 {
							padR = 0
						}
						if padT < 0 {
							padT = 0
						}
						if padB < 0 {
							padB = 0
						}
						// clientWidth/clientHeight include padding (CSSOM: client
						// height = padding-box height minus scrollbar). Gating an
						// overflow:auto box against the content-box height makes
						// every vertically-padded container look "overflowed"
						// (BoxContentSize counts content + top padding), so
						// ws-section/project-section/sidebar-content all showed
						// spurious scrollbars even when content fit exactly.
						viewW := pb.Width
						if viewW < 1 {
							viewW = 1
						}
						viewH := pb.Height
						if viewH < 1 {
							viewH = 1
						}

						needsV := (st.OverflowY == style.OverflowScroll || (st.OverflowY == style.OverflowAuto && totalH > viewH)) && st.OverflowY != style.OverflowHidden
						needsH := (st.OverflowX == style.OverflowScroll || (st.OverflowX == style.OverflowAuto && totalW > viewW)) && st.OverflowX != style.OverflowHidden

						if needsV || needsH {
							// Windows-style scrollbars are ALWAYS visible when content
							// overflows (Chrome/Edge on Windows keep overflow:auto
							// scrollbars resident; overlay auto-hiding is a macOS /
							// touch feature). No cursor-over-box gating here.

							// Darker scrollbar colors (better contrast vs white track).
							trackCol := graphics.Color{R: 255, G: 255, B: 255, A: 255}   // #FFFFFF white track
							thumbCol := graphics.Color{R: 160, G: 160, B: 160, A: 255}   // #A0A0A0 thumb (was #C0C0C0)
							thumbHoverCol := graphics.Color{R: 128, G: 128, B: 128, A: 255} // #808080 hover (was #A0A0A0)

							arrowCol := graphics.Color{R: 96, G: 96, B: 96, A: 255}     // #606060 arrow (was #808080)

							// CSS scrollbar-color: "thumb track" overrides the
							// default palette (thumb hover uses the thumb color).
							if sc := st.GetProperty("scrollbar-color"); sc != "" && sc != "auto" {
								scParts := strings.Fields(sc)
								if len(scParts) >= 1 {
									if c, ok := parseColorSimple(scParts[0]); ok {
										thumbCol = c
										thumbHoverCol = c
									}
								}
								if len(scParts) >= 2 {
									if c, ok := parseColorSimple(scParts[1]); ok {
										trackCol = c
									}
								}
							}
							// ::-webkit-scrollbar-thumb / ::-webkit-scrollbar
							// background overrides (Blink/WebKit custom styling,
							// e.g. GitPanel's var(--scrollbar-thumb)).
							if tc := st.GetProperty("-webkit-scrollbar-thumb-color"); tc != "" {
								if c, ok := parseColorSimple(tc); ok {
									thumbCol = c
									thumbHoverCol = c
								}
							}
							if tc2 := st.GetProperty("-webkit-scrollbar-track-color"); tc2 != "" {
								if c, ok := parseColorSimple(tc2); ok {
									trackCol = c
								}
							}
							// ::-webkit-scrollbar-thumb { border-radius } —
							// used to round the thumb ends.
							thumbRadius := 0.0
							if tr := st.GetProperty("-webkit-scrollbar-thumb-radius"); tr != "" {
								if l, ok := parseLengthAny(tr); ok && l > 0 {
									thumbRadius = l
								}
							}

							sx, sy := float64(0), float64(0)
							cursorX, cursorY := float64(0), float64(0)
							if info.rv != nil {
								sx, sy = info.rv.BoxScrollOffset(box)
								// Form-control text scrolls through per-element
								// FormControlTextScroll (input caret / pre-mode textarea),
								// not BoxScrollOffset — mirror it so the horizontal thumb
								// follows the control's own scroll, not a sibling's.
								if el, ok := box.Node().(*dom.Element); ok {
									if el.LocalName() == "textarea" || el.LocalName() == "input" {
										sx = FormControlTextScroll(el)
									}
								}
								cursorX, cursorY = info.rv.CursorPos()
							}

							// (viewW/viewH already computed above for needsX; the
							// thumb geometry below reuses them.)

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

							// Track background (transparent track from ::-webkit-scrollbar-track
							// { background: transparent } is skipped — browser shows only the thumb).
							if trackCol.A > 0 {
								info.canvas.FillRect(vx, vy, scrollW, vh, trackCol)
							}

							if !webkitSB {
								// Up arrow: rounded triangle matching horizontal arrow proportions.
								upBtnY := vy
								acx := vx + scrollW/2
								acy := upBtnY + arrowSize/2
								info.canvas.FillRoundedTriangle(acx, acy-2, acx-4, acy+3, acx+4, acy+3, 0.8, arrowCol)

								// Down arrow.
								dnBtnY := vy + vh - arrowSize
								dcy := dnBtnY + arrowSize/2
								info.canvas.FillRoundedTriangle(acx, dcy+2, acx-4, dcy-3, acx+4, dcy-3, 0.8, arrowCol)
							}

							// Thumb (rounded rect, pill shape) — geometry from the shared
							// ScrollbarMetrics so host drag/wheel map identically to paint.
							if vm := VerticalScrollbarMetrics(info.rv, box); vm.OK {
								syRatio := sy / vm.MaxScroll
								if syRatio < 0 { syRatio = 0 }
								if syRatio > 1 { syRatio = 1 }
								thumbTrackSpace := vm.TrackLen - vm.ThumbLen
								thumbY := vy + syRatio*thumbTrackSpace
								if !webkitSB {
									thumbY += arrowSize + arrowGap
								}

								isHover := cursorX >= vx && cursorX <= vx+scrollW &&
									cursorY >= thumbY && cursorY <= thumbY+vm.ThumbLen
								tCol := thumbCol
								if isHover { tCol = thumbHoverCol }

								rad := 5.0
								if thumbRadius > 0 {
									rad = thumbRadius
								}
								if style.DiagEnabled("scrollbar") {
									cn := ""
									if el, ok := box.Node().(*dom.Element); ok {
										cn = el.GetAttribute("class")
									}
									style.Diagf("scrollbar", "VSB %q: scrollW=%.0f webkitSB=%v thumbCol=#%02x%02x%02x%02x trackCol=#%02x%02x%02x%02x vx=%.0f thumbY=%.0f thumbLen=%.0f trackLen=%.0f sy=%.0f maxScroll=%.0f rad=%.0f",
										cn, scrollW, webkitSB,
										thumbCol.R, thumbCol.G, thumbCol.B, thumbCol.A,
										trackCol.R, trackCol.G, trackCol.B, trackCol.A,
										vx, thumbY, vm.ThumbLen, vm.TrackLen, sy, vm.MaxScroll, rad)
								}
								if webkitSB {
									// Custom webkit scrollbar: thumb fills the FULL
									// scrollbar width like the browser (Edge headless
									// measures 8px for ::-webkit-scrollbar { width:8px }),
									// no breathing room, no arrow offset.
									info.canvas.FillRoundRect(vx, thumbY, scrollW, vm.ThumbLen, rad, tCol)
								} else {
									info.canvas.FillRoundRect(vx+2, thumbY, scrollW-4, vm.ThumbLen, rad, tCol)
								}
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

							// Track background (transparent → skip, browser-style).
							if trackCol.A > 0 {
								info.canvas.FillRect(hx, hy, hw, scrollW, trackCol)
							}

							if !webkitSB {
								// Left arrow.
								ltBtnX := hx
								aCy := hy + scrollW/2
								info.canvas.FillRoundedTriangle(ltBtnX+4, aCy, ltBtnX+arrowSize-3, aCy-4, ltBtnX+arrowSize-3, aCy+4, 0.8, arrowCol)

								// Right arrow.
								rtBtnX := hx + hw - arrowSize
								info.canvas.FillRoundedTriangle(rtBtnX+arrowSize-4, aCy, rtBtnX+3, aCy-4, rtBtnX+3, aCy+4, 0.8, arrowCol)
							}

								// Thumb — geometry from the shared ScrollbarMetrics so
								// host drag/wheel map identically to paint.
								if hm := HorizontalScrollbarMetrics(info.rv, box); hm.OK {
									sxRatio := sx / hm.MaxScroll
									if sxRatio < 0 { sxRatio = 0 }
									if sxRatio > 1 { sxRatio = 1 }
									thumbTrackSpace := hm.TrackLen - hm.ThumbLen
									thumbX := hx + sxRatio*thumbTrackSpace
									if !webkitSB {
										thumbX += arrowSize + arrowGap
									}

									isHover := cursorY >= hy && cursorY <= hy+scrollW &&
										cursorX >= thumbX && cursorX <= thumbX+hm.ThumbLen
									tCol := thumbCol
									if isHover { tCol = thumbHoverCol }

									radH := 5.0
								if thumbRadius > 0 {
									radH = thumbRadius
								}
								if webkitSB {
									info.canvas.FillRoundRect(thumbX, hy, hm.ThumbLen, scrollW, radH, tCol)
								} else {
									info.canvas.FillRoundRect(thumbX, hy+2, hm.ThumbLen, scrollW-4, radH, tCol)
								}
								}
							}
							endH:

							// Corner fill (transparent track → skip).
							if needsV && needsH && trackCol.A > 0 {
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
	if needsTransformRestore {
		info.canvas.Restore()
	}
	if needsStickyRestore {
		info.canvas.Restore()
	}
}

// resolveTransformOrigin converts a transform-origin Length into an offset
// from the box's top-left corner. Returns -1 when the length is empty
// (caller falls back to the box center).
func resolveTransformOrigin(l style.Length, boxSize float64) float64 {
	if l.Unit == "" {
		return -1
	}
	switch l.Unit {
	case "%":
		return boxSize * l.Value / 100.0
	default:
		return l.Value
	}
}

// paintBackdrop paints the blurred/filtered content behind this box as its
// background (CSS backdrop-filter). Software approximation: snapshot the
// current canvas, run the filter chain offscreen over the box region, and
// blit the result back at the box's position before its own background is
// drawn.
func paintBackdrop(box *RenderBox, info *PaintInfo, imgFilter *skia.ImageFilter) {
	w := int(box.Width())
	h := int(box.Height())
	if w <= 0 || h <= 0 {
		return
	}
	img := info.canvas.Snapshot()
	if img == nil {
		return
	}
	defer img.Release()
	surf, err := skia.NewRasterSurface(skia.ImageInfoN32Premul(w, h))
	if err != nil || surf == nil {
		return
	}
	defer surf.Release()
	sc := surf.Canvas()
	// Apply the filter via a SaveLayer with a filter-bearing paint (the same
	// mechanism as the verified CSS filter path) rather than relying on the
	// image paint's filter, which some cgo bindings drop.
	filterPaint := skia.NewPaint()
	defer filterPaint.Release()
	filterPaint.SetAntialias(true)
	filterPaint.SetImageFilter(imgFilter)
	sc.SaveLayer(nil, filterPaint)
	src := skia.RectXYWH(float32(box.X()), float32(box.Y()), float32(w), float32(h))
	dst := skia.RectXYWH(0, 0, float32(w), float32(h))
	plainPaint := skia.NewPaint()
	defer plainPaint.Release()
	plainPaint.SetAntialias(true)
	sc.DrawImageRect(img, src, dst, skia.SamplingLinear, plainPaint)
	sc.Restore()
	out := surf.Snapshot()
	if out == nil {
		return
	}
	defer out.Release()
	info.canvas.DrawImage(out, box.X(), box.Y(), box.Width(), box.Height())
}

// computeStickyOffset returns the scroll-pinning translate for a
// position:sticky box, mirroring RenderBox::stickyPositionOffset() simplified
// to the document-level scroll offset. The box sticks to the nearest
// scrollport edge when its static position would otherwise leave it:
//
//	top: N   → pins when staticY+scrollY < N, clamped so the box never
//	          passes below the bottom of the viewport.
//
// Nested scroll containers (overflow:scroll ancestors other than the view)
// are not tracked yet — sticky inside them is treated as relative.
func computeStickyOffset(box *RenderBox, view *RenderView) (float64, float64) {
	st := box.Style()
	if st == nil {
		return 0, 0
	}
	// Only top-pinning is implemented (the overwhelmingly common case);
	// bottom/left/right sticky are treated as static for now.
	topRaw := st.GetProperty("top")
	if topRaw == "" || topRaw == "auto" {
		return 0, 0
	}
	top := 0.0
	if l, ok := parseCSSLength(topRaw); ok {
		top = l
	} else {
		return 0, 0
	}
	_, sy := view.ScrollOffset()
	staticY := box.Y()
	// Viewport-space position: scrolling down (sy>0) moves content up, so the
	// element's viewport top is staticY - sy. The canvas is already translated
	// by -scroll, so the element paints at staticY; a positive dy pulls it
	// down to pin at top once its viewport position passes the top line.
	vy := staticY - sy
	if vy < top && staticY+box.Height() > 0 {
		dy := top - vy
		return 0, dy
	}
	return 0, 0
}

// parseCSSLength parses a plain CSS length ("0", "10px", "1.5em") into px.
// em is resolved against 16px (no font context available here).
func parseCSSLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' || s[i] == '+') {
		i++
	}
	num := s[:i]
	if num == "" || num == "-" || num == "." {
		return 0, false
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	unit := s[i:]
	switch unit {
	case "", "px":
		return v, true
	case "em":
		return v * 16, true
	case "%":
		return 0, false // needs container size; treated as 0
	}
	return 0, false
}

// paintObjectBackground paints the background-color and border for box-bearing objects
// during the background phase, mirroring the Background + Border phase of
// RenderBox::paint().
func paintObjectBackground(o RenderObject, info *PaintInfo) {
	box := asRenderBox(o)
	if box == nil || !box.IsVisible() {
		return
	}
	// Apply CSS backdrop-filter: blur / filter the content painted behind
	// this box before drawing its own background (software approximation of
	// WebKit's backdrop blur — snapshot, filter offscreen, blit back).
	if st := box.Style(); st != nil {
		if bf := st.GetProperty("backdrop-filter"); bf != "" && bf != "none" {
			if filters := parseCSSFilters(bf); len(filters) > 0 {
				if imgFilter := buildCSSFilterChain(filters); imgFilter != nil {
					paintBackdrop(box, info, imgFilter)
				}
			}
		}
	}
	// Apply CSS clip-path (inset/circle/polygon) around the box's own
	// background/border painting, mirroring RenderBox::paint()'s clip.
	var clipCleanup func()
	if st := box.Style(); st != nil {
		if cp := st.GetProperty("clip-path"); cp != "" && cp != "none" && !strings.HasPrefix(cp, "url(") {
			bx, by, bw, bh := box.X(), box.Y(), box.Width(), box.Height()
			if r, ok := parseCSSClipInset(cp, bx, by, bw, bh); ok {
				info.canvas.Save()
				info.canvas.Clip(r)
				clipCleanup = info.canvas.Restore
			} else if p := parseCSSClipShape(cp, bx, by, bw, bh); p != nil {
				info.canvas.Save()
				info.canvas.ClipPath(p)
				clipCleanup = info.canvas.Restore
			}
		}
	}
	// Apply CSS mix-blend-mode: wrap in an offscreen layer composited with
	// the blend mode (mirrors WebKit's blend mode layer).
	var blendCleanup func()
	if st := box.Style(); st != nil {
		if bm := parseBlendMode(st.GetProperty("mix-blend-mode")); bm != nil {
			info.canvas.SaveLayerWithBlendMode(*bm)
			blendCleanup = info.canvas.Restore
		}
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
	// Apply CSS transform if present (inside filter layer). The transform is
	// applied by walkSubtreeExcluded for the whole subtree; the per-box
	// background/border painting here must NOT re-apply it.
	PaintBackground(box, info)
	PaintBorder(box, info)
	if filterCleanup != nil {
		defer filterCleanup()
	}
	if blendCleanup != nil {
		defer blendCleanup()
	}
	if clipCleanup != nil {
		defer clipCleanup()
	}
}

// parseBlendMode maps a CSS mix-blend-mode value to a skia blend mode.
// Returns nil for "normal" or unknown values (no layer needed).
func parseBlendMode(s string) *skia.BlendMode {
	var m skia.BlendMode
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "multiply":
		m = skia.BlendModeMultiply
	case "screen":
		m = skia.BlendModeScreen
	case "overlay":
		m = skia.BlendModeOverlay
	case "darken":
		m = skia.BlendModeDarken
	case "lighten":
		m = skia.BlendModeLighten
	case "color-dodge":
		m = skia.BlendModeColorDodge
	case "color-burn":
		m = skia.BlendModeColorBurn
	case "hard-light":
		m = skia.BlendModeHardLight
	case "soft-light":
		m = skia.BlendModeSoftLight
	case "difference":
		m = skia.BlendModeDifference
	case "exclusion":
		m = skia.BlendModeExclusion
	case "hue":
		m = skia.BlendModeHue
	case "saturation":
		m = skia.BlendModeSaturation
	case "color":
		m = skia.BlendModeColor
	case "luminosity":
		m = skia.BlendModeLuminosity
	default:
		return nil
	}
	return &m
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
		// fill/stroke="currentColor" resolves to the element's CSS color
		// (e.g. white send-btn icon, muted icon color). Must be passed into
		// buildSVGDocument BEFORE walk parses currentColor references.
		var cc graphics.Color
		if st := box.Style(); st != nil {
			cc = toGraphicsColor(st.Color)
		}
		doc := buildSVGDocument(el, cc)
		if os.Getenv("WB_SVG_DEBUG") != "" {
			log.Printf("[svg] paintObjectForeground svg class=%q xy=(%.0f,%.0f) wh=(%.0f,%.0f) viewBox=%v shapes=%d currentColor=#%02x%02x%02x",
				el.GetAttribute("class"), box.X(), box.Y(), box.Width(), box.Height(),
				doc.viewBox, len(doc.shapes), cc.R, cc.G, cc.B)
		}
		if doc != nil && len(doc.shapes) > 0 {
			paintSVG(info.canvas, doc, box.X(), box.Y(), graphics.Color{})
		} else if os.Getenv("WB_SVG_DEBUG") != "" {
			log.Printf("[svg] WARNING: no shapes parsed for svg class=%q", el.GetAttribute("class"))
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
