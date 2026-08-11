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
	dirtyRect   Rect
	scrollOffsetX, scrollOffsetY float64
	// scrollOffsets stores per-box scroll offsets for overflow:scroll/auto.
	// Keyed by the DOM node (NOT the RenderBox pointer): the render tree is
	// rebuilt frequently (hover :style changes, DOM mutations → brand-new
	// RenderBox instances), so pointer keys go stale and a wheel/thumb
	// offset written against the old box would never be seen by paint of
	// the new box — "scrollbar thumb moves but content does not". DOM nodes
	// survive rebuilds; paint/hit-test/scrollbar code reads by box.Node().
	boxScrollOffsets map[dom.Node]graphics.Point

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
		boxScrollOffsets: make(map[dom.Node]graphics.Point),
		nodeRenderMap:    make(map[dom.Node]RenderObject),
	}
	rv.initBase(rv, doc, st)
	rv.compositor = NewRenderLayerCompositor(rv)
	return rv
}

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
// Keyed by DOM node so the offset survives render-tree rebuilds (see the
// boxScrollOffsets field comment).
func (v *RenderView) SetBoxScrollOffset(box *RenderBox, x, y float64) {
	if box == nil {
		return
	}
	if v.boxScrollOffsets == nil {
		v.boxScrollOffsets = make(map[dom.Node]graphics.Point)
	}
	if os.Getenv("WB_SCROLL_DEBUG") != "" {
		if el, ok := box.Node().(*dom.Element); ok {
			log.Printf("[scroll/set] BoxScrollOffset %s → (%.1f, %.1f)", el.LocalName(), x, y)
		}
	}
	v.boxScrollOffsets[box.Node()] = graphics.Point{X: x, Y: y}
	// ★ 滚动偏移变化必须标记全脏：paint 的 dirty-rect 检查（intersects）
	// 用未 translate 的绝对坐标判断对象是否在脏区内。滚动后新进入
	// 视口的内容（绝对坐标仍在旧视口下方）会被误判为"不在脏区"而
	// 跳过绘制 → "滚动条 thumb 动了、内容却空白"或"滚上来后内容
	// 不显示"。与 SetScrollOffset（页面级）一致，每次 box 滚动都
	// MarkAllDirty，下一帧全量重绘。
	if v.viewWidth > 0 && v.viewHeight > 0 {
		v.MarkAllDirty()
	}
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
		v.boxScrollOffsets = make(map[dom.Node]graphics.Point)
	}
	// Keyed by DOM node, which survives the rebuild — copy directly.
	for node, p := range old.boxScrollOffsets {
		if node == nil {
			continue
		}
		v.boxScrollOffsets[node] = p
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
	if v.boxScrollOffsets == nil || box == nil || box.Node() == nil {
		return 0, 0
	}
	p, ok := v.boxScrollOffsets[box.Node()]
	if !ok {
		return 0, 0
	}
	return float64(p.X), float64(p.Y)
}

// HasBoxScrollOffset reports whether ANY overflow:scroll/auto box currently
// carries a non-zero scroll offset. Paint uses this to force a full repaint:
// per-box scroll translate moves content into the viewport whose ABSOLUTE
// (un-translated) coordinates still lie outside the dirty rect, so the
// painter's intersects() check would skip it ("scrollbar moves, scrolled-in
// content is blank"). When any box is scrolled, dirty-checking is disabled
// for the frame.
func (v *RenderView) HasBoxScrollOffset() bool {
	if v == nil || len(v.boxScrollOffsets) == 0 {
		return false
	}
	for _, p := range v.boxScrollOffsets {
		if p.X != 0 || p.Y != 0 {
			return true
		}
	}
	return false
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

// FindRenderObjectForNode returns the raw RenderObject for a given DOM node
// (not coerced to RenderBox). Inline elements (RenderInline, e.g. CM6 语法
// 高亮 span) do not generate a CSS box so asRenderBox returns nil for them,
// but they DO carry a layout box with geometry — callers like
// bindings.GetElementBoxRect need the raw object to read inline geometry.
func (v *RenderView) FindRenderObjectForNode(n dom.Node) RenderObject {
	if v.nodeRenderMap == nil || n == nil {
		return nil
	}
	ro, ok := v.nodeRenderMap[n]
	if !ok {
		return nil
	}
	return ro
}

// FindScrollContainerForNode walks up from node (through DOM ancestors)
// looking for the first element whose RenderBox has overflow:scroll or
// overflow:auto on EITHER axis. The painter's scrollbar gating
// (needsScrollbars) is per-axis (overflow-y:auto with content overflow
// draws a vertical scrollbar even when overflow-x stays visible), so a
// container with only overflow-y:auto must be hit-testable as a scroll
// container too — otherwise wheel events over such boxes fall through to
// the FrameView (page maxY=0) and every single-axis scroll container
// becomes unscrollable.
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
		isScroll := st.OverflowX == style.OverflowScroll ||
			st.OverflowX == style.OverflowAuto ||
			st.OverflowY == style.OverflowScroll ||
			st.OverflowY == style.OverflowAuto
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
	// ★ iframe 子文档元素：点击点在子 Frame 内。滚动容器应在子 Frame 的
	// RenderView 里查找（子文档元素不在主渲染树，FindScrollContainerForNode
	// 查不到）。坐标系已由 hit-test 下钻记录（lastDive：子 Frame 视图 +
	// 子坐标）。若子 Frame 里也没有滚动容器，返回 nil——不继续向主文档
	// 找（浏览器 iframe 边界语义：鼠标在 iframe 上时滚动只作用于子文档）。
	if od := el.OwnerDocument(); od != nil && od != v.Document() {
		if lastDive.ok && lastDive.sub != nil && lastDive.sub.Document() == od {
			sub := lastDive.sub
			lastDive.ok = false // 一次性消费
			if sub.RenderView() != nil {
				if sub.NeedsLayout() {
					sub.LayoutNow()
				}
				return sub.RenderView().HitTestScrollContainer(lastDive.x, lastDive.y)
			}
		}
		return nil
	}
	return v.FindScrollContainerForNode(el)
}

// ScrollTarget 是某点下滚动容器的解析结果。Box 可能属于 iframe 子文档
// （滚动容器在子渲染树里），其偏移表存在子 RenderView 里——子文档绘制
// 时读的是子 RenderView.BoxScrollOffset。滚动事件处理必须用 RV（拥有
// 偏移表的 RenderView）读写偏移，否则主 rv 查不到/写入不生效。
type ScrollTarget struct {
	RV  *RenderView
	Box *RenderBox
}

// ScrollTargetAt 解析 (x, y) 下的滚动容器及其所属 RenderView。滚轮/键盘
// 滚动写入偏移时用 tgt.RV（iframe 内滚动 = 子 Frame 的 RenderView）而非
// 主视图——这是「app 层滚动事件路由到子 Frame」的接入点。
func (v *RenderView) ScrollTargetAt(x, y float64) ScrollTarget {
	box := v.HitTestScrollContainer(x, y)
	if box == nil {
		return ScrollTarget{}
	}
	rv := v
	if n := box.Node(); n != nil {
		if od := n.OwnerDocument(); od != nil && od != v.Document() {
			if sub := IFrameContainingFor(od); sub != nil && sub.RenderView() != nil {
				rv = sub.RenderView()
			}
		}
	}
	return ScrollTarget{RV: rv, Box: box}
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
	Box *RenderBox
	// RV 是滚动条所属的 RenderView。iframe 子文档的滚动条属于子 Frame 的
	// RenderView（偏移表/几何存在子 rv）——宿主读写偏移、计算 thumb 几何
	// 必须用 RV 而非主视图（与 ScrollTarget.RV 同一语义）。
	RV *RenderView
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
// 命中 iframe 内容时递归进子 Frame（子文档滚动条由子 RenderView 判定），
// 返回的 ScrollbarHit.RV 是滚动条所属的 RenderView。
func HitTestScrollbar(rv *RenderView, x, y float64) *ScrollbarHit {
	if rv == nil {
		return nil
	}
	el := HitTest(rv, x, y, "")
	if el == nil {
		return nil
	}
	// ★ iframe 子文档元素：点击点在子 Frame 内，滚动条判定用子坐标在子
	// Frame 的 RenderView 里递归（子文档滚动容器不在主渲染树）。坐标系
	// 已由 hit-test 下钻记录（lastDive：子 Frame 视图 + 子坐标）。
	if od := el.OwnerDocument(); od != nil && od != rv.Document() {
		if lastDive.ok && lastDive.sub != nil && lastDive.sub.Document() == od &&
			lastDive.sub.RenderView() != nil {
			sub := lastDive.sub
			lastDive.ok = false // 一次性消费
			if sub.NeedsLayout() {
				sub.LayoutNow()
			}
			return HitTestScrollbar(sub.RenderView(), lastDive.x, lastDive.y)
		}
		return nil
	}
	h := hitTestScrollbarInner(rv, x, y)
	if h != nil {
		h.RV = rv
	}
	return h
}

// hitTestScrollbarInner is the single-RenderView scrollbar hit-test body.
func hitTestScrollbarInner(rv *RenderView, x, y float64) *ScrollbarHit {
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

	// ★ textarea resize 手柄让位：CSS resize:vertical/both 的 textarea 右下角
	// 15px 是拖拽手柄区（浏览器中滚动条 track 底部不覆盖它）。若滚动条矩形
	// 覆盖手柄区，Press 的 scrollbar hit 先于 resize 检测命中 → 手柄永远
	// 拖不动（子 agent 编辑框内容溢出有垂直滚动条时必现）。按 Chromium
	// 语义：垂直滚动条底端让位 15px、水平滚动条右端让位 15px。
	if el2, ok := scrollBox.Node().(*dom.Element); ok && el2.LocalName() == "textarea" {
		if st := scrollBox.Style(); st != nil && ResizeModeOf(st) != 0 {
			const rHandle = 15.0
			if needsV {
				vh -= rHandle
				if vh < 1 {
					vh = 1
				}
			}
			if needsH {
				hw -= rHandle
				if hw < 1 {
					hw = 1
				}
			}
		}
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
	if state == nil {
		// ★ 复用上一帧的 LayoutState（geometry map 跨帧保留）：布局仍然
		// 全量执行（每个 box 几何被覆盖重算），仅省去每帧新建 map +
		// 全部 box 零值几何的开销（1000+ 节点页面可省数 ms/帧）。
		// viewport 尺寸变化时不能复用（所有 box 宽度可能变），新建。
		if v.layoutState != nil && v.layoutState.ViewportWidth == v.viewWidth &&
			v.layoutState.ViewportHeight == v.viewHeight {
			state = v.layoutState
		} else {
			state = layout.NewLayoutState(v.viewWidth, v.viewHeight)
		}
	}
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
		// ★ RenderView 是视口，其几何恒为 viewport 尺寸——不能被 layout
		// root（html 元素）的高度覆盖。反例：iframe 子文档 body 无流内容
		// 时 html 高度为 0，若用 layoutRoot 覆盖则 RenderView 高度变 0，
		// hit-test 在根 box 就被 inBounds 拦截（所有子元素不可命中）。
		if v.IsRenderView() {
			box.frame.X, box.frame.Y = 0, 0
			box.frame.Width, box.frame.Height = v.viewWidth, v.viewHeight
		} else {
			box.frame = state.GeometryForBox(layoutRoot).ToRect()
		}
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
