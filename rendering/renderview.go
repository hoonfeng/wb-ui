// Translation of: Source/WebCore/rendering/RenderView.cpp
package rendering

import (
	"log"
	"os"
	"strings"

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

	// resolver holds the style resolver used to build this tree; paint code
	// (e.g. PaintSelection for ::selection colors) reads it lazily.
	resolver *style.Resolver
}

// SetResolver attaches the style resolver used to build the render tree.
func (v *RenderView) SetResolver(r *style.Resolver) { v.resolver = r }

// Resolver returns the attached style resolver (may be nil).
func (v *RenderView) Resolver() *style.Resolver { return v.resolver }

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
	if os.Getenv("WB_SCROLL_DEBUG") != "" && box != nil {
		if el, ok := box.Node().(*dom.Element); ok {
			log.Printf("[scroll/set] BoxScrollOffset %s → (%.1f, %.1f)", el.LocalName(), x, y)
		}
	}
	v.boxScrollOffsets[box] = graphics.Point{X: x, Y: y}
}

// RestoreScrollOffsetsFrom carries per-box scroll offsets from a previous
// incarnation of the render tree into this one, mapping boxes through their
// DOM nodes. RebuildRenderTree() builds a brand-new tree (new RenderBox
// objects, empty boxScrollOffsets), so without this every rebuild — e.g.
// each keystroke in a textarea — silently reset all vertical scroll to 0,
// then auto-scroll yanked the content to the caret row ("content jumps out
// of view as soon as I type"). Horizontal form-control text scroll lives in
// a per-ELEMENT map and survives rebuilds; this covers the vertical /
// overflow-container path.
func (v *RenderView) RestoreScrollOffsetsFrom(old *RenderView) {
	if old == nil || len(old.boxScrollOffsets) == 0 {
		return
	}
	if v.boxScrollOffsets == nil {
		v.boxScrollOffsets = make(map[*RenderBox]graphics.Point)
	}
	for ob, p := range old.boxScrollOffsets {
		if ob == nil {
			continue
		}
		// Match by DOM node via tree walk: nodeRenderMap is only
		// populated during syncGeometry, which runs later at layout
		// time — the rebuild itself cannot rely on it.
		var nb *RenderBox
		var walk func(RenderObject)
		walk = func(o RenderObject) {
			if nb != nil {
				return
			}
			if o != nil && o.Node() == ob.Node() {
				if b := asRenderBox(o); b != nil {
					nb = b
					return
				}
			}
			for c := o.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
			}
		}
		walk(RenderObject(v))
		if nb != nil {
			v.boxScrollOffsets[nb] = p
		}
	}
}

// ScrollOffsetCount returns the number of boxes with a stored scroll
// offset (diagnostics).
func (v *RenderView) ScrollOffsetCount() int {
	if v == nil || v.boxScrollOffsets == nil {
		return 0
	}
	return len(v.boxScrollOffsets)
}

// FindRenderBoxForNode returns the RenderBox for a given DOM node, or nil

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
	// ★ Custom recursion that SKIPS subtrees of overflow-clipping containers
	// (auto/scroll/hidden): their clipped content (e.g. .project-section's
	// file tree reaching y=2080) must not inflate an ANCESTOR's content size,
	// or sidebar-content grows a spurious scrollbar on top of the container's
	// own one ("three scrollbars" / hover-background-covers-scrollbar).
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if o == nil {
			return
		}
		if cb := asRenderBox(o); cb != nil {
			// Fixed-position boxes are viewport-anchored and NEVER contribute
			// to an ancestor's scroll size (CSS 2.1 §10.1). Without this a
			// dialog overlay (position:fixed; inset:0; 1280px wide) inside
			// file-explorer inflates sidebar-content's content width to
			// 1280-48=1232px → spurious horizontal scrollbar over the whole
			// sidebar after clicking "新建工作区".
			if st := cb.Style(); st != nil && st.Position == style.PositionFixed {
				return
			}
			if r := cb.frame.X + cb.frame.Width; r > maxRight {
				maxRight = r
			}
			if b := cb.frame.Y + cb.frame.Height; b > maxBottom {
				maxBottom = b
			}
			found = true
			if st := cb.Style(); st != nil && overflowClipsContentStyle(st) {
				return // content clipped: do not recurse into this subtree
			}
		}
		if rt, ok := o.(*RenderText); ok {
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
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for c := box.FirstChild(); c != nil; c = c.NextSibling() {
		walk(c)
	}
	_ = pb

	// overflowClipsContentStyle reports whether either overflow axis clips content
	// (auto/scroll/hidden). BoxContentSize skips clipped subtrees so an ancestor's
	// content size never includes a scroll container's overflowing children.
	// (Defined below; see also page/frameview.go updateContentSize for the same
	// rule at frame level.)

// Form controls (input/textarea) carry their text in value/textContent,
	// not as render-tree children — measure it so scrollbars appear when the
	// text overflows (a pre-mode textarea scrolls horizontally, a long input
	// scrolls too). The scroll extent must at least cover the control.
	if el, ok := box.Node().(*dom.Element); ok {
		local := el.LocalName()
		if local == "textarea" || local == "input" {
			var text string
			if local == "textarea" {
				text = el.TextContent()
			} else {
				text = el.GetAttribute("value")
			}
			if st := box.Style(); st != nil {
				font := toGraphicsFont(st)
				lineH := 0.0
				// Content-box viewport padding (scrollWidth/scrollHeight
				// include the padding — a scrolled control must show the
				// padding at the far edge like a browser).
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
				if text != "" {
					lines := strings.Split(text, "\n")
					maxW := 0.0
					for i, line := range lines {
						w := graphics.MeasureText(font, line)
						if w > maxW {
							maxW = w
						}
						if i == 0 {
							lineH = cssControlLineHeight(st, font.Size)
						}
					}
					// Horizontal extent = left padding + text + right padding.
					if padL+maxW+padR > maxRight-pb.X {
						maxRight = pb.X + padL + maxW + padR
					}
					if local == "textarea" && lineH > 0 {
						// Vertical extent must count SOFT-WRAPPED rows (a
						// pre-wrap textarea wraps long lines into several
						// visual rows), not just hard '\n' breaks — otherwise
						// the scrollbar's total height / thumb ratio is too
						// small and long text can't scroll far enough.
						contentW := pb.Width - padL - padR
						if contentW < 1 {
							contentW = 1
						}
						mode := textareaWrapMode(st, el)
						wrapped := wrapTextAreaLines(text, font, contentW, mode)
						rows := float64(len(wrapped))
						if padT+rows*lineH+padB > maxBottom-pb.Y {
							maxBottom = pb.Y + padT + rows*lineH + padB
						}
					}
				}
				// Form controls always contribute their (possibly empty)
				// content so needsX tests compare against the content-box
				// viewport, never fall into the no-children fallback below.
				found = true
			}
		}
	}

	// No content at all: report zero extent. Callers compare against the
	// content-box viewport, so an empty box must NOT claim the padding-box
	// size (that would spuriously enable scrollbars on padding alone).
	if !found {
		return 0, 0
	}
	cw := maxRight - pb.X
	ch := maxBottom - pb.Y
	if cw < 0 {
		cw = 0
	}
	if ch < 0 {
		ch = 0
	}
	return cw, ch
}

// overflowClipsContentStyle reports whether either overflow axis clips content
// (auto/scroll/hidden). BoxContentSize skips clipped subtrees so an ancestor's
// content size never includes a scroll container's overflowing children
// (same rule as page/frameview.go updateContentSize).
func overflowClipsContentStyle(st *style.ComputedStyle) bool {
	if st == nil {
		return false
	}
	return st.OverflowX == style.OverflowHidden ||
		st.OverflowX == style.OverflowAuto ||
		st.OverflowX == style.OverflowScroll ||
		st.OverflowY == style.OverflowHidden ||
		st.OverflowY == style.OverflowAuto ||
		st.OverflowY == style.OverflowScroll
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
	scrollW := scrollbarWidthFor(st)
	arrowSize := 12.0
	arrowGap := 5.0
	if scrollW <= 0 || pb.Width <= scrollW*2 || pb.Height <= scrollW*2 {
		return nil
	}
	webkit := webkitCustomScrollbar(st)

	// Content size via BoxContentSize — this handles form controls
	// (input/textarea) whose text lives in value/textContent instead of
	// render-tree children, so their scrollbars are hit-testable too.
	cw, ch := rv.BoxContentSize(scrollBox)
	totalW := cw
	totalH := ch

	// Scroll viewport is the CONTENT box (padding-box minus padding), the
	// same viewport the paint code uses for thumb geometry.
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
	// clientWidth/clientHeight include padding (CSSOM) — see paint gating in
	// renderpipeline.go. Comparing against the content-box height makes every
	// padded overflow:auto container spuriously scrollable.
	contentW := pb.Width
	if contentW < 1 {
		contentW = 1
	}
	contentH := pb.Height
	if contentH < 1 {
		contentH = 1
	}
	needsV := (st.OverflowY == style.OverflowScroll || (st.OverflowY == style.OverflowAuto && totalH > contentH)) && st.OverflowY != style.OverflowHidden
	needsH := (st.OverflowX == style.OverflowScroll || (st.OverflowX == style.OverflowAuto && totalW > contentW)) && st.OverflowX != style.OverflowHidden

	// Vertical scrollbar rect
	vx := pb.X + pb.Width - scrollW
	vy := pb.Y
	vh := pb.Height
	if needsH {
		vh -= scrollW
	}

	// Horizontal scrollbar rect
	hx := pb.X
	hy := pb.Y + pb.Height - scrollW
	hw := pb.Width
	if needsV {
		hw -= scrollW
	}

	// Get scroll offsets for thumb position calculations. Form controls
	// scroll their text through per-element FormControlTextScroll, not
	// BoxScrollOffset — mirror that so the thumb position matches paint.
	sx, sy := rv.BoxScrollOffset(scrollBox)
	if el2, ok := scrollBox.Node().(*dom.Element); ok {
		if el2.LocalName() == "textarea" || el2.LocalName() == "input" {
			sx = FormControlTextScroll(el2)
		}
	}

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
		if !webkit && vh <= arrowSize*2 {
			return nil
		}
		if !webkit {
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
		}

		// Track (non-thumb area) or thumb — same geometry as the painter
		// (shared ScrollbarMetrics) so a press lands on the drawn thumb.
		h.IsVTrack = true
		if m := VerticalScrollbarMetrics(rv, scrollBox); m.OK {
			syRatio := sy / m.MaxScroll
			if syRatio < 0 {
				syRatio = 0
			}
			if syRatio > 1 {
				syRatio = 1
			}
			thumbTrackSpace := m.TrackLen - m.ThumbLen
			thumbY := vy + syRatio*thumbTrackSpace
			if !webkit {
				thumbY += arrowSize + arrowGap
			}
			if y >= thumbY && y <= thumbY+m.ThumbLen {
				h.IsVThumb = true
			}
		}
		return h
	}

	// ── Horizontal scrollbar hit test ──
	if needsH && y >= hy && y <= hy+scrollW && x >= hx && x <= hx+hw {
		h := &ScrollbarHit{Box: scrollBox}
		if !webkit && hw <= arrowSize*2 {
			return nil
		}
		if !webkit {
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
		}

		// Track or thumb — same geometry as the painter (shared metrics).
		h.IsHTrack = true
		if m := HorizontalScrollbarMetrics(rv, scrollBox); m.OK {
			sxRatio := sx / m.MaxScroll
			if sxRatio < 0 {
				sxRatio = 0
			}
			if sxRatio > 1 {
				sxRatio = 1
			}
			thumbTrackSpace := m.TrackLen - m.ThumbLen
			thumbX := hx + sxRatio*thumbTrackSpace
			if !webkit {
				thumbX += arrowSize + arrowGap
			}
			if x >= thumbX && x <= thumbX+m.ThumbLen {
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
					if textRight > frameRight && !renderIsFlexItem(ro) {
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
			if !renderIsFlexItem(ro) {
				box.frame.Width = maxRight - box.frame.X
			}
		}
		if maxBottom > frameBottom {
			box.frame.Height = maxBottom - box.frame.Y
		}
	}
}

// renderIsFlexItem reports whether ro is an in-flow child of a flex container.
// Flex items' frame width must stay at the flex-resolved size (the text may
// overflow and be ellipsized/clipped) — syncOne must NOT widen them back to
// the raw text extent (that made a 238px flex-shrunk .item-name render 260px
// and overflow its item-row).
func renderIsFlexItem(ro RenderObject) bool {
	lb := ro.LayoutBox()
	if lb == nil || lb.Parent() == nil {
		return false
	}
	pcs := lb.Parent().Style()
	if pcs == nil {
		return false
	}
	d := pcs.Display
	return (d == style.DisplayFlex || d == style.DisplayInlineFlex) && !lb.IsAbsolutelyPositioned()
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
