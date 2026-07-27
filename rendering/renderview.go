// Translation of: Source/WebCore/rendering/RenderView.cpp
package rendering

import (
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
	"wb-ui/widgets"
)

func init() {
	// Set layout font metrics callback using Skia's actual font metrics.
	// This ensures line-height and text positioning match what Skia renders.
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		a := graphics.GlobalFontAscent(f)
		d := graphics.GlobalFontDescent(f)
		lg := graphics.GlobalFontLineGap(f)
		return a, d, lg
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
}

type RenderView struct {
	RenderBlockFlow
	document    *dom.Document
	viewWidth   float64
	viewHeight  float64
	compositor  *RenderLayerCompositor
	rootLayer   *RenderLayer
	layoutState *layout.LayoutState
	editorRegistry *widgets.EditorRegistry
	dirtyRect   Rect
	scrollOffsetX, scrollOffsetY float64
	// scrollOffsets stores per-box scroll offsets for overflow:scroll/auto.
	// Keyed by the RenderBox pointer; only boxes that have been scrolled
	// appear in this map.
	boxScrollOffsets map[*RenderBox]graphics.Point

	// nodeRenderMap maps DOM nodes to their corresponding RenderObject.
	// Populated during syncGeometry() so hit-test and scroll container lookup
	// can go from DOM element → RenderBox without O(n) tree traversal.
	nodeRenderMap map[dom.Node]RenderObject

	// cursorX/cursorY track the last known cursor position in CSS pixels,
	// used by paint code for scrollbar hover highlighting.
	cursorX, cursorY float64
}

func NewRenderView(doc *dom.Document, st *style.ComputedStyle) *RenderView {
	rv := &RenderView{
		document:         doc,
		boxScrollOffsets: make(map[*RenderBox]graphics.Point),
		nodeRenderMap:    make(map[dom.Node]RenderObject),
	}
	rv.initBase(rv, doc, st)
	rv.compositor = NewRenderLayerCompositor(rv)
	rv.editorRegistry = widgets.NewEditorRegistry()
	return rv
}

func (v *RenderView) EditorRegistry() *widgets.EditorRegistry { return v.editorRegistry }
func (v *RenderView) Type() RenderObjectType                 { return ObjectView }
func (v *RenderView) IsRenderView() bool                     { return true }
func (v *RenderView) RenderName() string                     { return "RenderView" }
func (v *RenderView) Document() *dom.Document                { return v.document }
func (v *RenderView) View() *RenderView                     { return v }
func (v *RenderView) ViewWidth() float64                    { return v.viewWidth }
func (v *RenderView) ViewHeight() float64                   { return v.viewHeight }
func (v *RenderView) IsDirty() bool                          { return v.dirtyRect.Width > 0 && v.dirtyRect.Height > 0 }

func (v *RenderView) GetDirtyRect() Rect { return v.dirtyRect }

func (v *RenderView) MarkDirty(r Rect) {
	if v.dirtyRect.Width <= 0 || v.dirtyRect.Height <= 0 {
		v.dirtyRect = r; return
	}
	x0 := min2(v.dirtyRect.X, r.X)
	y0 := min2(v.dirtyRect.Y, r.Y)
	x1 := max2(v.dirtyRect.X+v.dirtyRect.Width, r.X+r.Width)
	y1 := max2(v.dirtyRect.Y+v.dirtyRect.Height, r.Y+r.Height)
	v.dirtyRect = Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func (v *RenderView) MarkAllDirty() {
	v.dirtyRect = Rect{X: 0, Y: 0, Width: v.viewWidth, Height: v.viewHeight}
}

func (v *RenderView) ClearDirty() { v.dirtyRect = Rect{} }

func (v *RenderView) SetScrollOffset(x, y float64) {
	v.scrollOffsetX, v.scrollOffsetY = x, y
	if v.viewWidth > 0 && v.viewHeight > 0 { v.MarkAllDirty() } else { v.dirtyRect = Rect{X: 0, Y: 0, Width: 1, Height: 1} }
}

func (v *RenderView) ScrollOffset() (float64, float64) { return v.scrollOffsetX, v.scrollOffsetY }

// SetBoxScrollOffset stores a scroll offset for an overflow:scroll box.
func (v *RenderView) SetBoxScrollOffset(box *RenderBox, x, y float64) {
	if v.boxScrollOffsets == nil {
		v.boxScrollOffsets = make(map[*RenderBox]graphics.Point)
	}
	v.boxScrollOffsets[box] = graphics.Point{X: x, Y: y}
}

// BoxScrollOffset returns the stored scroll offset for an overflow:scroll box.
func (v *RenderView) BoxScrollOffset(box *RenderBox) (float64, float64) {
	if v.boxScrollOffsets == nil {
		return 0, 0
	}
	p, ok := v.boxScrollOffsets[box]
	if !ok {
		return 0, 0
	}
	return float64(p.X), float64(p.Y)
}

// FindRenderBoxForNode returns the RenderBox for a given DOM node, or nil
func (v *RenderView) FindRenderBoxForNode(n dom.Node) *RenderBox {
	if v.nodeRenderMap == nil || n == nil {
		return nil
	}
	ro, ok := v.nodeRenderMap[n]
	if !ok {
		return nil
	}
	return asRenderBox(ro)
}

// FindScrollContainerForNode walks up from node (through DOM ancestors)
// looking for the first element whose RenderBox has overflow:scroll or
func (v *RenderView) FindScrollContainerForNode(n dom.Node) *RenderBox {
	for cur := n; cur != nil; cur = cur.ParentNode() {
		box := v.FindRenderBoxForNode(cur)
		if box == nil {
			continue
		}
		st := box.Style()
		if st == nil {
			continue
		}
		isScroll := (st.OverflowX == style.OverflowScroll || st.OverflowX == style.OverflowAuto) &&
			(st.OverflowY == style.OverflowScroll || st.OverflowY == style.OverflowAuto)
		if isScroll {
			return box
		}
	}
	return nil
}

// HitTestScrollContainer hit-tests the render tree at (x, y) and walks up
// to find the nearest scrollable ancestor RenderBox. Returns nil if no
// scroll container is found.
func (v *RenderView) HitTestScrollContainer(x, y float64) *RenderBox {
	el := HitTest(v, x, y, "")
	if el == nil {
		return nil
	}
	return v.FindScrollContainerForNode(el)
}

// BoxContentSize returns the content width and height of a scrollable box,
// computed as the bounding box of all render children relative to the
// padding box. Returns (0,0) if no children.
func (v *RenderView) BoxContentSize(box *RenderBox) (float64, float64) {
	pb := box.PaddingBoxRect()
	var maxRight, maxBottom float64
	found := false
	walkRenderChildren(box, func(child RenderObject) {
		if cb := asRenderBox(child); cb != nil {
			if r := cb.frame.X + cb.frame.Width; r > maxRight {
				maxRight = r
			}
			if b := cb.frame.Y + cb.frame.Height; b > maxBottom {
				maxBottom = b
			}
			found = true
		}
		if _, ok := child.(*RenderText); ok {
			if rt, ok2 := child.(*RenderText); ok2 {
				for _, seg := range rt.Segments() {
					if r := seg.X + seg.Width; r > maxRight {
						maxRight = r
					}
					if b := seg.Y + seg.Height; b > maxBottom {
						maxBottom = b
					}
					found = true
				}
			}
		}
	})
	if !found {
		return pb.Width, pb.Height
	}
	cw := maxRight - pb.X
	ch := maxBottom - pb.Y
	if cw < pb.Width {
		cw = pb.Width
	}
	if ch < pb.Height {
		ch = pb.Height
	}
	return cw, ch
}

// walkRenderChildren recursively visits all descendants of root.
func walkRenderChildren(root RenderObject, fn func(RenderObject)) {
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		fn(c)
		walkRenderChildren(c, fn)
	}
}

// ScrollbarHit describes which scrollbar element was hit at a given point.
type ScrollbarHit struct {
	Box         *RenderBox
	IsVThumb    bool // vertical thumb hit
	IsVTrack    bool // vertical track (non-thumb, non-arrow area)
	IsVUpArrow  bool // vertical up arrow button
	IsVDownArrow bool // vertical down arrow button
	IsHThumb    bool // horizontal thumb hit
	IsHTrack    bool // horizontal track (non-thumb, non-arrow area)
	IsHLeftArrow  bool // horizontal left arrow button
	IsHRightArrow bool // horizontal right arrow button
	IsCorner    bool // corner overlap area
}

// HitTestScrollbar checks whether (x,y) hits a scrollbar thumb or track
// of any scrollable box in the render tree. Returns nil if nothing hit.
func HitTestScrollbar(rv *RenderView, x, y float64) *ScrollbarHit {
	if rv == nil {
		return nil
	}
	// First find the deepest element at (x,y), then find its scroll container.
	el := HitTest(rv, x, y, "")
	if el == nil {
		return nil
	}
	scrollBox := rv.FindScrollContainerForNode(el)
	if scrollBox == nil {
		return nil
	}
	// Check if (x,y) is within the scrollbar area of scrollBox.
	st := scrollBox.Style()
	if st == nil {
		return nil
	}
	pb := scrollBox.PaddingBoxRect()
	scrollW := 12.0
	arrowSize := 12.0
	if pb.Width <= scrollW*2 || pb.Height <= scrollW*2 {
		return nil
	}

	// Compute scrollbar track rectangles (same logic as paint code).
	contentW := pb.Width
	contentH := pb.Height

	// Compute content bounding box from children.
	var minX, minY, maxX, maxY float64
	hasChild := false
	for c := scrollBox.FirstChild(); c != nil; c = c.NextSibling() {
		if cb := asRenderBox(c); cb != nil {
			cg := cb.FrameRect()
			if !hasChild {
				minX, minY, maxX, maxY = cg.X, cg.Y, cg.X+cg.Width, cg.Y+cg.Height
				hasChild = true
			} else {
				if cg.X < minX { minX = cg.X }
				if cg.Y < minY { minY = cg.Y }
				if cg.X+cg.Width > maxX { maxX = cg.X + cg.Width }
				if cg.Y+cg.Height > maxY { maxY = cg.Y + cg.Height }
			}
		}
	}
	if !hasChild {
		return nil
	}
	totalW := maxX - minX
	totalH := maxY - minY
	needsV := (st.OverflowY == style.OverflowScroll || (st.OverflowY == style.OverflowAuto && totalH > contentH)) && st.OverflowY != style.OverflowHidden
	needsH := (st.OverflowX == style.OverflowScroll || (st.OverflowX == style.OverflowAuto && totalW > contentW)) && st.OverflowX != style.OverflowHidden

	// Vertical scrollbar rect
	vx := pb.X + pb.Width - scrollW
	vy := pb.Y
	vh := pb.Height
	if needsH { vh -= scrollW }

	// Horizontal scrollbar rect
	hx := pb.X
	hy := pb.Y + pb.Height - scrollW
	hw := pb.Width
	if needsV { hw -= scrollW }

	// Get scroll offsets for thumb position calculations.
	sx, sy := rv.BoxScrollOffset(scrollBox)

	// Corner: check first so it takes priority over individual bar hits.
	if needsV && needsH {
		cx := pb.X + pb.Width - scrollW
		cy := pb.Y + pb.Height - scrollW
		if x >= cx && x <= cx+scrollW && y >= cy && y <= cy+scrollW {
			return &ScrollbarHit{Box: scrollBox, IsCorner: true}
		}
	}

	// ── Vertical scrollbar hit test ──
	if needsV && x >= vx && x <= vx+scrollW && y >= vy && y <= vy+vh {
		h := &ScrollbarHit{Box: scrollBox}
		if vh <= arrowSize*2 {
			return nil
		}
		upBtnY := vy
		dnBtnY := vy + vh - arrowSize

		// Check arrow buttons first.
		if y >= upBtnY && y < upBtnY+arrowSize {
			h.IsVUpArrow = true
			return h
		}
		if y >= dnBtnY && y < dnBtnY+arrowSize {
			h.IsVDownArrow = true
			return h
		}

		// Track (non-thumb area) or thumb.
		h.IsVTrack = true
		if totalH > contentH {
			trackH := vh - arrowSize*2
			thumbLen := trackH * contentH / totalH
			if thumbLen < arrowSize { thumbLen = arrowSize }
			if thumbLen > trackH-4 { thumbLen = trackH - 4 }
			maxSy := totalH - contentH
			if maxSy <= 0 { maxSy = 1 }
			syRatio := sy / maxSy
			thumbTrackSpace := trackH - thumbLen
			thumbY := vy + arrowSize + syRatio*thumbTrackSpace
			if y >= thumbY && y <= thumbY+thumbLen {
				h.IsVThumb = true
			}
		}
		return h
	}

	// ── Horizontal scrollbar hit test ──
	if needsH && y >= hy && y <= hy+scrollW && x >= hx && x <= hx+hw {
		h := &ScrollbarHit{Box: scrollBox}
		if hw <= arrowSize*2 {
			return nil
		}
		ltBtnX := hx
		rtBtnX := hx + hw - arrowSize

		// Check arrow buttons first.
		if x >= ltBtnX && x < ltBtnX+arrowSize {
			h.IsHLeftArrow = true
			return h
		}
		if x >= rtBtnX && x < rtBtnX+arrowSize {
			h.IsHRightArrow = true
			return h
		}

		// Track or thumb.
		h.IsHTrack = true
		if totalW > contentW {
			trackW := hw - arrowSize*2
			thumbLen := trackW * contentW / totalW
			if thumbLen < arrowSize { thumbLen = arrowSize }
			if thumbLen > trackW-4 { thumbLen = trackW - 4 }
			maxSx := totalW - contentW
			if maxSx <= 0 { maxSx = 1 }
			sxRatio := sx / maxSx
			thumbTrackSpace := trackW - thumbLen
			thumbX := hx + arrowSize + sxRatio*thumbTrackSpace
			if x >= thumbX && x <= thumbX+thumbLen {
				h.IsHThumb = true
			}
		}
		return h
	}

	return nil
}

func (v *RenderView) SetViewportSize(w, h float64) {
	if v.viewWidth != w || v.viewHeight != h { v.viewWidth, v.viewHeight = w, h; v.Dirty() }
}

// CursorPos returns the last tracked cursor position in CSS pixels.
func (v *RenderView) CursorPos() (float64, float64) { return v.cursorX, v.cursorY }

// SetCursorPos records the cursor position (CSS pixels) for scrollbar hover highlight.
func (v *RenderView) SetCursorPos(x, y float64) { v.cursorX, v.cursorY = x, y }

func (v *RenderView) RootLayer() *RenderLayer          { return v.rootLayer }
func (v *RenderView) SetRootLayer(l *RenderLayer)       { v.rootLayer = l }
func (v *RenderView) Compositor() *RenderLayerCompositor { return v.compositor }

func (v *RenderView) Layout(state *layout.LayoutState) {
	if state == nil { state = layout.NewLayoutState(v.viewWidth, v.viewHeight) }
	v.layoutState = state
	v.frame.X, v.frame.Y = 0, 0
	v.frame.Width, v.frame.Height = v.viewWidth, v.viewHeight

	if lb := v.LayoutBox(); lb != nil {
		g := state.GeometryForBox(lb)
		g.SetTopLeft(0, 0)
		g.SetContentWidth(v.viewWidth)
		g.SetContentHeight(v.viewHeight)
	}
	if v.LayoutBox() == nil && v.document != nil {
		if root := v.document.DocumentElement(); root != nil {
			defaultResolver := style.NewResolver()
			defaultResolver.AddStyleSheet(html5.NewUAStyleSheet())
			layoutRoot := layout.BuildLayoutTree(root, defaultResolver)
			if layoutRoot != nil {
				if rootEb, ok := layoutRoot.(*layout.ElementBox); ok {
					v.SetLayoutBox(rootEb)
				}
			}
		}
	}
	v.RenderBlockFlow.Layout(state)
	v.syncGeometry()
	if v.compositor != nil { v.compositor.UpdateCompositingLayers() }
}

func (v *RenderView) LayoutState() *layout.LayoutState { return v.layoutState }

func (v *RenderView) syncGeometry() {
	layoutRoot := v.LayoutBox()
	if layoutRoot == nil { return }
	state := v.layoutState
	if state == nil { return }
	if box := asRenderBox(v); box != nil {
		box.frame = state.GeometryForBox(layoutRoot).ToRect()
	}
	for rc := v.FirstChild(); rc != nil; rc = rc.NextSibling() {
		syncOne(rc, layoutRoot, state)
		break
	}
}

func syncOne(ro RenderObject, lb *layout.ElementBox, state *layout.LayoutState) {
	if ro == nil || lb == nil || state == nil { return }
	rect := state.GeometryForBox(lb).ToRect()
	if rect.Width < 0 { rect.Width = 0 }
	if rect.Height < 0 { rect.Height = 0 }
	if rect.X < 0 { rect.X = 0 }
	if rect.Y < 0 { rect.Y = 0 }
	if box := asRenderBox(ro); box != nil { box.frame = rect }
	ro.SetLayoutBox(lb)

	// Populate node render map for hit-test / scroll container lookup.
	if rv := ro.View(); rv != nil && ro.Node() != nil {
		rv.nodeRenderMap[ro.Node()] = ro
	}

	textLB := lb
	var textSegments []layout.TextSegment
	if len(textLB.TextSegments) > 0 {
		textSegments = textLB.TextSegments
	} else {
		textSegments = findTextSegments(textLB)
	}
	if rt, ok := ro.(*RenderText); ok && len(textSegments) > 0 {
		segs := make([]InlineTextBox, len(textSegments))
		for i, s := range textSegments {
			segs[i] = InlineTextBox{
				Start: s.Start, Len: s.Len,
				X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
				LineY: s.LineY, LineHeight: s.LineHeight,
			}
		}
		rt.SetSegments(segs)
		if box := asRenderBox(ro); box != nil && len(segs) > 0 {
			box.frame.Width = segs[0].Width
			box.frame.Height = segs[0].Height
		}
	} else if len(textSegments) > 0 {
		// ro is NOT a RenderText (e.g. anonymous wrapper). Propagate
		// segments to the first RenderText child.
		for rc := ro.FirstChild(); rc != nil; rc = rc.NextSibling() {
			if rt, ok := rc.(*RenderText); ok {
				segs := make([]InlineTextBox, len(textSegments))
				for i, s := range textSegments {
					segs[i] = InlineTextBox{
						Start: s.Start, Len: s.Len,
						X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
						LineY: s.LineY, LineHeight: s.LineHeight,
					}
				}
				rt.SetSegments(segs)
				// Expand the wrapper frame to encompass text content.
				if box := asRenderBox(ro); box != nil && len(segs) > 0 {
					textRight := segs[0].X + segs[0].Width
					frameRight := box.frame.X + box.frame.Width
					if textRight > frameRight {
						box.frame.Width = textRight - box.frame.X
					}
					textBottom := segs[0].Y + segs[0].Height
					frameBottom := box.frame.Y + box.frame.Height
					if textBottom > frameBottom {
						box.frame.Height = textBottom - box.frame.Y
					}
				}
				break
			}
		}
	}
	syncChildren(ro, lb, state)

	// After children are synced, expand this box's frame to encompass
	// any child text that extends beyond the geometry-based frame. This
	// prevents overflow:hidden from clipping text in flex items whose
	// layout geometry is narrower than actual text content.
	if box := asRenderBox(ro); box != nil && box.Parent() != nil {
		var maxRight, maxBottom float64
		frameRight := box.frame.X + box.frame.Width
		frameBottom := box.frame.Y + box.frame.Height
		for rc := ro.FirstChild(); rc != nil; rc = rc.NextSibling() {
			if rt, ok := rc.(*RenderText); ok {
				segs := rt.Segments()
				if len(segs) > 0 {
					s := segs[0]
					if r := s.X + s.Width; r > maxRight { maxRight = r }
					if b := s.Y + s.Height; b > maxBottom { maxBottom = b }
				}
			}
			if rbf, ok := rc.(*RenderBlockFlow); ok {
				for cc := rbf.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if rt, ok := cc.(*RenderText); ok {
						segs := rt.Segments()
						if len(segs) > 0 {
							s := segs[0]
							if r := s.X + s.Width; r > maxRight { maxRight = r }
							if b := s.Y + s.Height; b > maxBottom { maxBottom = b }
						}
					}
				}
			}
		}
		if maxRight > frameRight {
			box.frame.Width = maxRight - box.frame.X
		}
		if maxBottom > frameBottom {
			box.frame.Height = maxBottom - box.frame.Y
		}
	}
}

func syncChildren(parentRO RenderObject, parentLB *layout.ElementBox, state *layout.LayoutState) {
	// Must copy the slice to avoid mutating parentLB.children's backing array.
	// lChildren := parentLB.Children() shares the backing array; any
	// append(lChildren[:i], lChildren[i+1:]...) will overwrite the original
	// array, corrupting the layout tree (e.g. col-left gets replaced by col-right).
	orig := parentLB.Children()
	lChildren := make([]layout.Box, len(orig))
	copy(lChildren, orig)
	for rc := parentRO.FirstChild(); rc != nil && len(lChildren) > 0; rc = rc.NextSibling() {
		matched := -1
		for i, lc := range lChildren {
			if childEb, ok := lc.(*layout.ElementBox); ok {
				if sameOwner(rc, childEb) { matched = i; break }
			}
		}
		if matched >= 0 {
			if childEb, ok := lChildren[matched].(*layout.ElementBox); ok {
				syncOne(rc, childEb, state)
			}
			lChildren = append(lChildren[:matched], lChildren[matched+1:]...)
		} else if rc.Node() == nil {
			// Anonymous render child: match with next anonymous layout child.
			for i, lc := range lChildren {
				if childEb, ok := lc.(*layout.ElementBox); ok && childEb.Element() == nil {
					syncOne(rc, childEb, state)
					lChildren = append(lChildren[:i], lChildren[i+1:]...)
					break
				}
			}
		} else if rt, ok := rc.(*RenderText); ok {
			// RenderText: match either direct InlineTextBox or anonymous-wrapper-wrapped one.
			for i, lc := range lChildren {
				// Case 1: direct InlineTextBox child.
				if tb, ok := lc.(*layout.InlineTextBox); ok && tb.Text() == rt.OriginalText() {
					if len(tb.TextSegments) > 0 {
						segs := make([]InlineTextBox, len(tb.TextSegments))
						for j, s := range tb.TextSegments {
							segs[j] = InlineTextBox{
								Start: s.Start, Len: s.Len,
								X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
								LineY: s.LineY, LineHeight: s.LineHeight,
							}
						}
						rt.SetSegments(segs)
					}
					lChildren = append(lChildren[:i], lChildren[i+1:]...)
					break
				}
				// Case 2: anonymous ElementBox wrapper around InlineTextBox.
				if childEb, ok := lc.(*layout.ElementBox); ok && childEb.Element() == nil {
					for _, cc := range childEb.Children() {
						if tb, ok := cc.(*layout.InlineTextBox); ok && tb.Text() == rt.OriginalText() {
							if len(tb.TextSegments) > 0 {
								segs := make([]InlineTextBox, len(tb.TextSegments))
								for j, s := range tb.TextSegments {
									segs[j] = InlineTextBox{
										Start: s.Start, Len: s.Len,
										X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
										LineY: s.LineY, LineHeight: s.LineHeight,
									}
								}
								rt.SetSegments(segs)
							}
							lChildren = append(lChildren[:i], lChildren[i+1:]...)
							break
						}
					}
				}
			}
		}
	}
}


// findTextSegments walks the layout tree to find TextSegments, checking both
// ElementBox.TextSegments and InlineTextBox.TextSegments at each level.
func findTextSegments(lb *layout.ElementBox) []layout.TextSegment {
	if lb == nil { return nil }
	if len(lb.TextSegments) > 0 { return lb.TextSegments }
	for _, c := range lb.Children() {
		if tb, ok := c.(*layout.InlineTextBox); ok && len(tb.TextSegments) > 0 {
			return tb.TextSegments
		}
		if childEb, ok := c.(*layout.ElementBox); ok {
			if segs := findTextSegments(childEb); segs != nil {
				return segs
			}
		}
	}
	return nil
}
