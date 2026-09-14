// Translation of: Source/WebCore/rendering/PaintInfo.h
// Completeness: 50%
// Simplifications:
//   - no DisplayList accumulation: PaintInfo holds a direct *graphics.Canvas pointer
//     rather than a DisplayList::Recorder. Drawing happens immediately into the canvas.
//   - the PaintBehavior flag set (PaintBehaviorSelectionOnly, etc.) is collapsed to a
//     single phase enum.
//   - the override-clip / paint-container-for-* bookkeeping is omitted.
//   - the region / overflow-fragment tracking is omitted.

package rendering

import "wb-ui/engine/platform/graphics"

// PaintPhase mirrors WebCore::PaintPhase. It selects which "layer" of painting is
// currently being performed for a given object. WebKit paints a tree in multiple passes
// (background, then foreground, then outline) so that backgrounds of later siblings
// never obscure foregrounds of earlier ones.
type PaintPhase int

const (
	// PhaseBackground paints the background color / image and the borders, mirroring
	// PaintPhaseBackground. (WebKit folds border painting into the background phase.)
	PhaseBackground PaintPhase = iota
	// PhaseForeground paints text and inline / replaced content, mirroring
	// PaintPhaseForeground.
	PhaseForeground
	// PhaseOutline paints the outline, mirroring PaintPhaseOutline.
	PhaseOutline
	// PhaseSelection paints the selection highlight, mirroring PaintPhaseSelection.
	PhaseSelection
	// PhaseTextClip establishes a clip from text glyphs, mirroring PaintPhaseTextClip.
	PhaseTextClip
)

// Rect is an axis-aligned rectangle in paint coordinates. It reuses the graphics.Rect
// representation since the paint pipeline draws directly into a graphics.Canvas.
type Rect = graphics.Rect

// PaintInfo is the Go translation of WebCore::PaintInfo. It bundles everything a painter
// needs to know about the current paint operation: the destination canvas, the dirty
// (damaged) rectangle that limits what needs repainting, and the current paint phase.
//
// In WebKit PaintInfo is constructed at FrameView::paint time and threaded through every
// paint() virtual; this port passes *PaintInfo to the Painter functions for the same
// purpose.
type PaintInfo struct {
	// canvas is the destination GraphicsContext, mirroring PaintInfo::context.
	canvas *graphics.Canvas
	// skipBackgroundBox 是「背景已传播到画布」的那个盒（html 或 body，
	// 见 paintViewportBackground）。PaintBackground 对它只画阴影、不画背景：
	// CSS-BACKGROUNDS-3 §2.11.2 规定传播之后元素自身不再绘制背景，画布上那
	// 一份才是它。
	skipBackgroundBox *RenderBox
	// dirtyRect is the area that needs repainting, mirroring PaintInfo::rect.
	dirtyRect Rect
	// phase is the current paint pass, mirroring PaintInfo::phase.
	phase PaintPhase
	// rv is the RenderView root being painted, needed by PaintText to
	// query the selection state (for inverted-color rendering of selected
	// text) and to paint the caret. Set by the top-level Paint function.
	rv *RenderView

	// dirtyCheckEnabled controls whether the intersects() check restricts
	// painting to the dirty rect. When false, intersects() always returns
	// true (full repaint). Enabled by default.
	dirtyCheckEnabled bool

	// initialSaveCount is the canvas save-stack depth at Paint entry.
	// Fixed-position layers use RestoreToCount(initialSaveCount) to discard
	// every ancestor clip (a GPU-reliable alternative to ClipOpReplace,
	// which Skia's GPU backend turns into an empty clip).
	initialSaveCount int

	// scrollTranslateX / scrollTranslateY accumulate the scroll offsets
	// currently applied as canvas translates for overflow:auto/scroll boxes
	// (paintLayerContents child-layer path). Layer clips come from
	// CalculateRects in ABSOLUTE (un-translated) coordinates, but Skia's
	// Clip() applies the current transform first, so under an active scroll
	// translate the clip lands offset by -scrollY in device space and
	// intersects the ancestor clip to nothing — scrolled-in layer content
	// (position:relative items etc.) is culled and "content below the fold
	// never appears". Painters shift the clip rect back by this amount.
	scrollTranslateX float64
	scrollTranslateY float64

	// stickyDx / stickyDy hold the scroll-pinning translate applied to the
	// box currently being walked (set in walkSubtreeExcluded when a sticky
	// box pins). Painters use them to translate the dirty-rect intersection
	// test: a sticky box's PAINTED position differs from its static layout
	// position by the pin offset, so an intersects() test against the static
	// rect wrongly culls a box pinned INTO the viewport from below
	// (e.g. bottom-sticky at y=700 pinned to 390 — the static rect 700..740
	// is outside the 0..400 dirty rect and the box vanishes). WebKit has no
	// such problem because sticky positioning happens in layout; this port
	// applies it in paint, so the paint-side visibility test must follow.
	stickyDx float64
	stickyDy float64

	// opacityLayerDepth counts how many enclosing opacity transparency layers
	// (SaveLayerWithOpacity) are currently active. When > 0, painters must
	// NOT multiply colors by CumulativeOpacity — the enclosing layer already
	// applies that opacity when it composites, mirroring the browser where
	// background + border + text of an opacity<1 element are flattened and
	// then faded as a whole. Without this, text painted at 50% alpha onto a
	// 50% background looks translucent/grey instead of the browser's solid
	// dark text (and a 50% border over a 50% background leaves a visible
	// bright edge instead of blending in).
	opacityLayerDepth int

// textOverflowEllipsisPainted is set by PaintText when it draws the
	// ellipsis during text-overflow:ellipsis truncation, so that
	// walkSubtreeExcluded skips its own ellipsis paint for this container.
	textOverflowEllipsisPainted bool

	// transformDepth counts how many enclosing CSS transforms are active in
	// the current paint stack. Objects inside a transform space paint at
	// positions that differ from their layout coordinates (a translateY
	// scrolls content INTO the viewport), so dirty-rect culling against the
	// static geometry wrongly drops them — "vertical scroll content goes
	// blank after scrolling past the first viewport". Mirrors the
	// stickyDx/stickyDy handling: visibility tests must follow the painted
	// position, not the static one.
	transformDepth int

	// layerClipActive / layerClip / layerClipRadius 是 paintLayerTree 计算
	// 后传给 paintLayerContents 应用的层 overflow 裁剪（设备坐标，已含
	// scrollTranslate 补偿）。裁剪实际应用在效果层（mask/opacity
	// SaveLayer）之后——逃逸子层（包含块在本层之上的定位后代）绘制时
	// RestoreToCount 到裁剪 Save 之下以临时移除本层裁剪，同时保留祖先
	// 效果（CSS：逃逸后代不受裁剪祖先的 overflow 约束，但仍受其
	// opacity/mask 影响）。
	layerClipActive bool
	layerClip       Rect
	layerClipRadius float64
}

// NewPaintInfo constructs a PaintInfo targeting the given canvas for the given dirty
// rectangle. The phase defaults to PhaseBackground; the pipeline switches it between
// passes. Mirrors the PaintInfo constructor used by FrameView::paint.
func NewPaintInfo(canvas *graphics.Canvas, rect Rect) *PaintInfo {
	return &PaintInfo{
		canvas:    canvas,
		dirtyRect: rect,
		phase:     PhaseBackground,
		dirtyCheckEnabled: true,
	}
}

// Canvas returns the destination GraphicsContext, mirroring PaintInfo::context.
func (p *PaintInfo) Canvas() *graphics.Canvas { return p.canvas }

// DirtyRect returns the damaged rectangle, mirroring PaintInfo::rect.
func (p *PaintInfo) DirtyRect() Rect { return p.dirtyRect }

// Phase returns the current paint phase, mirroring PaintInfo::phase.
func (p *PaintInfo) Phase() PaintPhase { return p.phase }

// SetPhase switches the current paint phase. The render pipeline uses this to drive
// the multi-pass paint order (background -> foreground -> outline).
func (p *PaintInfo) SetPhase(ph PaintPhase) { p.phase = ph }

// SetDirtyCheckEnabled enables or disables dirty-rect checking. When disabled,
// intersects() returns true for all rectangles (full repaint).
func (p *PaintInfo) SetDirtyCheckEnabled(enabled bool) { p.dirtyCheckEnabled = enabled }

// DirtyCheckEnabled returns whether dirty-rect checking is active.
func (p *PaintInfo) DirtyCheckEnabled() bool { return p.dirtyCheckEnabled }

// intersects reports whether the dirty rect overlaps the given rectangle. Painters use
// this to skip objects entirely outside the damaged region, mirroring the early-out in
// WebKit's ObjectPainter / RenderBox::paint. When dirtyCheckEnabled is false (full
// repaint mode), a zero/negative dirty rect is treated as "paint everything".
func (p *PaintInfo) intersects(r Rect) bool {
	if !p.dirtyCheckEnabled {
		return true
	}
	// 在 transform 空间内绘制的对象：静态几何与绘制位置不一致
	// （translateY 把内容移进视口），一律不剔除（与 sticky 同理）。
	if p.transformDepth > 0 {
		return true
	}
	dr := p.dirtyRect
	if dr.Width <= 0 || dr.Height <= 0 {
		// A zero dirty rect means "paint everything" (no scissor set).
		return true
	}
	x0 := max2(dr.X, r.X)
	y0 := max2(dr.Y, r.Y)
	x1 := min2(dr.X+dr.Width, r.X+r.Width)
	y1 := min2(dr.Y+dr.Height, r.Y+r.Height)
	return x1 > x0 && y1 > y0
}

// max2 / min2 are local float helpers to avoid pulling in the math package for the
// single intersection test above.
func max2(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min2(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// rectFromLayout converts a layout.LayoutRect to a paint Rect, mirroring the implicit
// conversion WebKit performs between LayoutRect and FloatRect / IntRect at paint time.
func rectFromLayout(x, y, w, h float64) Rect {
	return Rect{X: x, Y: y, Width: w, Height: h}
}
