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
	"sync"
	"time"

	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"

	"github.com/hoonfeng/goskia/skia"
)

// paintStats* accumulate per-Paint layer-tree statistics when
// WB_PAINT_STATS=1, printed at the end of Paint().
var paintStatsLayers, paintStatsClipLayers, paintStatsNoClipLayers, paintStatsFixed, paintStatsVisits int

// paintDebugEnabled / paintStatsEnabled cache the WB_PAINT_DEBUG /
// WB_PAINT_STATS environment flags: os.Getenv on Windows is a syscall
// (~10µs), and the paint path queries it per layer (3000+ layers × 3 checks
// ≈ 90ms of pure syscall overhead in the old code).
var (
	paintDebugOnce  sync.Once
	paintDebugFlag  bool
	paintStatsOnce  sync.Once
	paintStatsFlag  bool
	paintStatsCount int
)

func paintDebugEnabled() bool {
	paintDebugOnce.Do(func() { paintDebugFlag = os.Getenv("WB_PAINT_DEBUG") != "" })
	return paintDebugFlag
}

func paintStatsEnabled() bool {
	paintStatsOnce.Do(func() { paintStatsFlag = os.Getenv("WB_PAINT_STATS") != "" })
	return paintStatsFlag
}

// Paint is the top-level paint entry point, mirroring FrameView::paint() which calls
// RenderView::paint(). It walks the render tree (or, when layers are present, the layer
// tree) and drives the per-phase painters into the supplied canvas. The dirty rect
// limits which objects are rasterized. If the RenderView has a scroll offset, a
// translate is applied before painting so content appears scrolled.
func Paint(view *RenderView, canvas *graphics.Canvas, rect Rect) {
	if view == nil || canvas == nil {
		return
	}
	if os.Getenv("WB_GUTTER_DEBUG") != "" {
		log.Printf("[paint-call] rect=%.0f,%.0f %.0fx%.0f dirty=%v", rect.X, rect.Y, rect.Width, rect.Height, view.IsDirty())
	}
	if paintStatsEnabled() {
		paintStatsLayers, paintStatsClipLayers, paintStatsNoClipLayers, paintStatsFixed, paintStatsVisits = 0, 0, 0, 0, 0
		restoreCountAtEntry := graphics.CanvasRestoreCount
		defer func() {
			log.Printf("[paint-stats] layers=%d clip=%d noclip=%d fixed=%d saveCount=%d restores=%d visits=%d cgo{%s}",
				paintStatsLayers, paintStatsClipLayers, paintStatsNoClipLayers, paintStatsFixed,
				canvas.SaveCount(), graphics.CanvasRestoreCount-restoreCountAtEntry, paintStatsVisits,
				graphics.CanvasTimingSummary())
		}()
	}
	// Reset the per-frame component paint trace (WB_COMP_LOG drains it via host).
	ResetComponentPaints()
	// ★ 跨 Frame 选区：绘制帧开始时由主 Frame 构建一次全局文本列表
	// （主 Frame + 所有 iframe 子 Frame，树序）。子 Frame 的 Paint 由
	// PaintIFrame 在主绘制中途调用——直接复用该列表（不重复构建），
	// 因此每个 Frame 绘制时都能用全局文档顺序判定选区。
	if IFrameContainingFor(view.Document()) == nil {
		globalSelTexts = nil
		collectRenderTextsAll(RenderObject(view), &globalSelTexts)
	}
	// Use the view's dirty rect if set; otherwise fall back to the caller's rect.
	paintRect := rect
	if view.IsDirty() {
		paintRect = view.GetDirtyRect()
	}
	info := NewPaintInfo(canvas, paintRect)
	info.rv = view
	// ★ 存在任何容器级 scroll offset 时禁用 dirty check：painter 的
	// intersects() 用未 translate 的绝对坐标判断对象是否在脏区内。
	// 滚动后内容 translate 进入视口，但绝对坐标仍在旧视口外 →
	// intersects=false → 新进入视口的内容被跳过 → "滚动正常但内容
	// 滚上来后空白/丢失"。有滚动偏移即全量重绘（滚动时内容必须
	// 全部绘制，这是浏览器滚动后的正常重绘范围）。页面级
	// SetScrollOffset（FrameView 滚动）同样平移内容，必须一并考虑 —
	// 否则 sticky 底栏/滚动后的普通元素用布局坐标 intersects 会被
	// 误判为"不在脏区"而消失。
	sx0, sy0 := view.ScrollOffset()
	anyBoxScroll := view.HasBoxScrollOffset() || sx0 != 0 || sy0 != 0
	info.SetDirtyCheckEnabled(view.IsDirty() && !anyBoxScroll)
	if paintDebugEnabled() {
		log.Printf("[paint] dirty=%v anyScroll=%v offsets=%d", view.IsDirty(), anyBoxScroll, view.ScrollOffsetCount())
	}	// Record the save depth at entry so fixed layers can restore to a
	// clip-free state via RestoreToCount (see paintLayerTree).
	info.initialSaveCount = canvas.SaveCount()

	// Apply scroll offset as a canvas translate.
	scrollX, scrollY := view.ScrollOffset()
	if scrollX != 0 || scrollY != 0 {
		canvas.Save()
		canvas.Translate(-scrollX, -scrollY)
		defer canvas.Restore()
	}

	// ★ 光标补画兜底：层树遍历（paintLayerTree）偶尔不覆盖光标（光标 box
	// 在渲染树但层树/裁剪未到达——表现为「光标不可见」偶发）。每次 paint
	// 重置 CursorPainted，层树画到光标则置位；未置位则补画（直接 FillRect
	// 白色左边框，绕过层树/裁剪/opacity 动画——静态可见优先）。
	CursorPainted = false
	if view.RootLayer() != nil {
		if os.Getenv("WB_CTM_DEBUG") != "" {
			m0 := canvas.GetMatrix()
			log.Printf("[ctm] Paint entry scaleX=%.3f tx=%.1f ty=%.1f", m0.ScaleX, m0.TransX, m0.TransY)
		}
		tTree := time.Now()
		paintLayerTree(view.RootLayer(), info)
		if paintStatsEnabled() {
			log.Printf("[paint-timing] tree=%v", time.Since(tTree).Round(time.Microsecond))
		}
		if os.Getenv("WB_CTM_DEBUG") != "" {
			m1 := canvas.GetMatrix()
			log.Printf("[ctm] after-layerTree scaleX=%.3f tx=%.1f ty=%.1f depth=%d", m1.ScaleX, m1.TransX, m1.TransY, canvas.SaveCount())
		}
	} else {
		paintSubtreeByPhase(RenderObject(view), info, nil)
	}
	// ★ 光标补画：层树遍历（paintLayerTree）偶尔不覆盖光标（光标 box
	// 在渲染树但层树/裁剪/opacity 未到达——表现为「光标不可见」偶发）。
	// 只要渲染树有光标 box 就无条件补画（静态白左边框，优先保证可见；
	// 层树正常时重复绘制 1px 无副作用）。FindRenderBoxForNode 走 map
	// O(1)；box 缺失（display:none/未重建）时 findCursorBox 返回 nil。
	// ★ 滚动补偿：fallback 在 paintLayerTree 之后执行，滚动容器的
	// translate 已 Restore，而 cb.X()/Y() 是内容绝对坐标——不补偿
	// 容器级 scroll offset 会把光标画在滚动前的位置（固定在屏幕坐标，
	// 不随内容滚动）。页面级 view.ScrollOffset 已在 Paint 入口
	// translate（defer Restore 覆盖至此），只需补偿容器级。
	// ★ 闪烁：CM6 光标闪烁是 .cm-cursorLayer（祖先）的 opacity 动画
	// （@keyframes cm-blink 50% opacity:0）。层树正常路径经
	// paintOpacity/CumulativeOpacity 应用祖先 opacity（隐藏相位不画）；
	// 但 fallback 直接 FillRect 绕过祖先 opacity → 隐藏相位仍画光标 →
	// 光标恒定不闪。fallback 用 CumulativeOpacity 累计祖先链 opacity，
	// opacity≈0 时跳过（闪烁隐藏相位）。
	if cb := findCursorBox(view); cb != nil {
		if CumulativeOpacity(RenderObject(cb)) <= 0.01 {
			// 闪烁隐藏相位（祖先 opacity 动画为 0）：不补画。
			if os.Getenv("WB_PAINT_TRACE") != "" {
				log.Printf("[cursor-fallback] skip (opacity≈0) cursorPainted=%v", CursorPainted)
			}
			goto cursorFallbackDone
		}
		lw := lengthValue(cb.Style().BorderLeftWidth)
		if lw <= 0 {
			lw = 1.2
		}
		col := graphics.Color{R: 230, G: 237, B: 243, A: 255}
		if bc := cb.Style().BorderColor("left"); bc.A > 0 {
			col = toGraphicsColor(bc)
		}
		csx, csy := caretScrollOffset(view, cb)
		canvas.FillRect(cb.X()-csx, cb.Y()-csy, lw, cb.Height(), col)
		if el, ok := cb.Node().(*dom.Element); ok {
			RecordComponentPaint(el, cb.X()-csx, cb.Y()-csy, lw, cb.Height(), graphics.Color{}, col, true)
		}
		if os.Getenv("WB_PAINT_TRACE") != "" {
			log.Printf("[cursor-fallback] painted caret at (%.1f,%.1f %.1fx%.1f) scroll=(%.1f,%.1f) cursorPainted=%v",
				cb.X()-csx, cb.Y()-csy, lw, cb.Height(), csx, csy, CursorPainted)
		}
	}
cursorFallbackDone:

	// Clear the dirty rect after painting.
	if view.IsDirty() {
		view.ClearDirty()
	}
}

// fallbackCursorEl caches the .cm-cursor DOM element across paints so the
// per-frame caret fallback uses the O(1) nodeRenderMap lookup instead of a
// full render-tree walk. Cleared when the element no longer has a box (CM6
// re-created it / display:none), triggering a fresh walk.
var fallbackCursorEl *dom.Element

// FindCursorBox locates the .cm-cursor element's RenderBox (the caret) via
// a render-tree walk. ★ 2026-08-12：GetElementBoxRect 的滚动补偿对光标
// 必须用它——FindRenderBoxForNode（nodeRenderMap）在 CM6 每次 measure
// 重建光标元素后返回**旧 box 实例**，其渲染树父链断在 .cm-editor 之外
// 拿不到 .cm-scroller 的滚动偏移 → 光标 getBoundingClientRect 不扣滚动
// （固定屏幕坐标）。walk 找到的是当前树的光标 box，父链正确。
// 返回 nil 表示光标没有 box（display:none / 未重建）。
func FindCursorBox(view *RenderView) *RenderBox {
	return findCursorBox(view)
}

// findCursorBox locates the .cm-cursor element's RenderBox (the caret).
// Used by the Paint fallback to guarantee the caret is always drawn even
// when the layer-tree walk skipped it. Returns nil if the caret has no box
// in this tree (e.g. display:none).
func findCursorBox(view *RenderView) *RenderBox {
	if view == nil {
		return nil
	}
	if fallbackCursorEl != nil {
		if rb := view.FindRenderBoxForNode(fallbackCursorEl); rb != nil {
			return rb
		}
		fallbackCursorEl = nil
	}
	var found *RenderBox
	var walk func(ro RenderObject)
	walk = func(ro RenderObject) {
		if found != nil || ro == nil {
			return
		}
		if n := ro.Node(); n != nil {
			if el, ok := n.(*dom.Element); ok && el.HasClassName("cm-cursor") {
				if rb := asRenderBox(ro); rb != nil {
					found = rb
					fallbackCursorEl = el
					return
				}
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(view))
	return found
}

// caretScrollOffset 返回光标 box 到根之间所有滚动容器（overflow:
// auto/scroll）的累计 scroll offset (x, y)。★ 光标补画（Paint fallback）
// 在 paintLayerTree 之后执行，滚动容器的 canvas translate 已 Restore，
// 而 cb.X()/cb.Y() 是内容绝对坐标——漏掉该补偿会把光标画在滚动前的
// 位置（固定在屏幕坐标，不随内容滚动）。
func caretScrollOffset(view *RenderView, cb *RenderBox) (float64, float64) {
	if view == nil || cb == nil {
		return 0, 0
	}
	var sx, sy float64
	for p := cb.Parent(); p != nil; p = p.Parent() {
		if rb := asRenderBox(p); rb != nil {
			if st := rb.Style(); st != nil &&
				(st.OverflowX == style.OverflowAuto || st.OverflowX == style.OverflowScroll ||
					st.OverflowY == style.OverflowAuto || st.OverflowY == style.OverflowScroll) {
				ox, oy := view.BoxScrollOffset(rb)
				sx += ox
				sy += oy
			}
		}
	}
	return sx, sy
}

// CaretScreenPosition returns the .cm-cursor caret's screen (CSS pixel)
// coordinates: box layout position minus accumulated container scroll
// offsets. Used by the Host to position the Windows IME composition/candidate
// window at the caret for contenteditable editors (CM6) — the form-control
// path (FormControlCaretPosition) only covers <input>/<textarea>. Returns
// ok=false when there is no caret box (blink hidden phase / not focused).
func CaretScreenPosition(view *RenderView) (x, y float64, ok bool) {
	cb := FindCursorBox(view)
	if cb == nil {
		return 0, 0, false
	}
	if CumulativeOpacity(RenderObject(cb)) <= 0.01 {
		// 闪烁隐藏相位：候选窗口不跟随（保持上次位置，避免跳顶）。
		return 0, 0, false
	}
	sx, sy := caretScrollOffset(view, cb)
	return cb.X() - sx, cb.Y() - sy, true
}

// opacityLayerBounds computes the SaveLayer bounds for an opacity layer:
// the owner's border box in LOCAL (pre-CTM) coordinates, with 16px slack for
// shadows/overflow.
//
// ★ CRITICAL: Skia SkCanvas::saveLayer(bounds) treats bounds as LOCAL
// coordinates — it is transformed by the CURRENT matrix when allocating the
// offscreen layer. The canvas CTM is scale(1.25)+translate(0,-scrollY), so
// world (CSS) coordinates ARE local coordinates. Passing DeviceRect()-mapped
// (already device-space) coordinates made Skia apply the CTM a SECOND time:
// a status-item layer at CSS (1187,782) got bounds ≈ (1483,1222) — 220px
// BELOW the 1000px-tall surface — so the offscreen layer was allocated
// outside the canvas and the composited content (status bar text, icons,
// status dot) was silently culled. This was the "状态栏没有内容显示" root
// cause: every opacity<0.98 layer's content vanished on the GPU backend.
func opacityLayerBounds(info *PaintInfo, layerRect layout.LayoutRect) graphics.Rect {
	_ = info
	w := layerRect.Width
	h := layerRect.Height
	if w <= 0 {
		w = 4
	}
	if h <= 0 {
		h = 4
	}
	return graphics.Rect{X: layerRect.X - 16, Y: layerRect.Y - 16, Width: w + 32, Height: h + 32}
}

// paintLayerWithEffects wraps paintLayerContents with the layer owner's
// mask-image and opacity offscreen layers (mask outermost, opacity inner),
// mirroring RenderLayer::paintLayer's composited effects. The mask wraps the
// WHOLE subtree (background + foreground + outline + descendants), matching
// CSS mask-image semantics — the previous per-box mask only masked
// background+border and let text/descendants leak through.
func paintLayerWithEffects(layer *RenderLayer, info *PaintInfo, layerRect layout.LayoutRect) {
	st := layer.Owner().Style()
	if st == nil {
		paintLayerContents(layer, info)
		return
	}
	// mask-image: offscreen layer + DstIn composite (outermost, so it masks
	// the opacity-composited result too).
	hasMask := hasMaskImage(st)
	var maskRect graphics.Rect
	if hasMask {
		maskRect = graphics.Rect{X: layerRect.X, Y: layerRect.Y, Width: layerRect.Width, Height: layerRect.Height}
		info.canvas.SaveLayerForMask(maskRect)
	}
	// opacity < 0.98: offscreen layer composited at opacity (inner).
	needOpacity := st.Opacity < 1.0 && st.Opacity <= 0.98 && os.Getenv("WB_NO_SAVELAYER") == ""
	if needOpacity {
		info.canvas.SaveLayerWithOpacityBounds(st.Opacity, opacityLayerBounds(info, layerRect))
		info.opacityLayerDepth++
	}
	paintLayerContents(layer, info)
	if needOpacity {
		info.opacityLayerDepth--
		info.canvas.Restore()
	}
	if hasMask {
		var ownerEl *dom.Element
		if owner := layer.Owner(); owner != nil {
			ownerEl, _ = owner.Node().(*dom.Element)
		}
		applyMaskLayer(info.canvas, st, maskRect, ownerEl)
		info.canvas.Restore()
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
	if paintDebugEnabled() {
		n := 0
		for c := layer.FirstChild(); c != nil; c = c.NextSibling() {
			n++
		}
		log.Printf("[layer] %s children=%d", layerName(layer), n)
	}
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
	layerRect, clip := layer.CalculateRects()
	hasClip := clip.Width > 0 && clip.Height > 0
	_ = layerRect
	if os.Getenv("WB_GUTTER_DEBUG") != "" {
		m := info.canvas.GetMatrix()
		log.Printf("[layer-ctm] %s ctm_ty=%.1f hasClip=%v scrollT=(%.1f,%.1f)", layerName(layer), float64(m.TransY), hasClip, info.scrollTranslateX, info.scrollTranslateY)
	}
	if paintStatsEnabled() {
		paintStatsLayers++
		if isFixedLayer {
			paintStatsFixed++
		}
		if hasClip {
			paintStatsClipLayers++
		}
	}
	// ★ 层早退提前到 Save 之前（局部重绘核心优化）：当前实现先 Save+Clip
	// 再判断与 dirty rect 无交 → 白白付出 Save+Clip+Restore 三次 cgo。
	// 局部重绘时（hover/局部变化）1944 个 clip 层中绝大多数与脏区无交
	// （脏区 581×66 只覆盖少数层）——提前 return 省掉每层的全部 cgo。
	// 全量重绘（DirtyCheckEnabled=false）不早退；fixed 层不早退（其
	// 内容可能在视口任意处）。
	if !isFixedLayer && hasClip && info.DirtyCheckEnabled() {
		dr := info.dirtyRect
		if dr.Width > 0 && dr.Height > 0 {
			cl := clip
			cl.X += info.scrollTranslateX
			cl.Y += info.scrollTranslateY
			if cl.Width <= 0 || cl.Height <= 0 ||
				cl.X >= dr.X+dr.Width || cl.X+cl.Width <= dr.X ||
				cl.Y >= dr.Y+dr.Height || cl.Y+cl.Height <= dr.Y {
				return
			}
		}
	}
	// ★ 仅在 hasClip 或 fixed 时 Save（对齐 WebKit RenderLayer::paintLayer
	// 只在需要时才 saveGraphicsState）：普通 positioned 层（overflow:
	// visible 且祖先无 overflow，无 clip）不保护 canvas 状态——层内
	// 的容器 clip（walkSubtreeExcluded 自配对）、scroll translate
	// （paintLayerContents 自配对）、opacity SaveLayer（自配对）都不会
	// 泄漏到层外。省掉 2978 层中 1034 个无 clip 层的全部 Save/Restore
	// cgo 调用（Windows cgo ~20µs/次，这是 paint 80%+ 的时间）。
	if hasClip || isFixedLayer {
		info.canvas.Save()
	}
	if isFixedLayer {
		// ★ Fixed-position layers paint against the viewport. The previous
		// approach — ResetClip via SkClipOp::kReplace — is BROKEN on Skia's
		// GPU backend: it turns the clip into an EMPTY clip, so every
		// subsequent draw (overlay background, dialog box) is culled and
		// the dialog never appears (raster is fine, GPU is not).
		// Reliable alternative: RestoreToCount to the Paint-entry depth
		// (clip-free), then Save + ResetFixedTransform (keep device scale,
		// drop inherited scroll translates). The fixed element's own
		// overflow still clips its subtree.
		// The scroll-translate bookkeeping resets here too: RestoreToCount
		// discarded every ancestor scroll translate, so the accumulated
		// counter (used to shift layer clips back into device space) no
		// longer matches the canvas state. Save it, zero it for the
		// viewport-aligned subtree, and restore it for later siblings.
		savedScrollTX, savedScrollTY := info.scrollTranslateX, info.scrollTranslateY
		info.scrollTranslateX, info.scrollTranslateY = 0, 0
		info.canvas.RestoreToCount(info.initialSaveCount)
		info.canvas.Save()
		info.canvas.ResetFixedTransform()
		// ★ fixed 元素自身的 overflow 由 walkSubtreeExcluded 裁剪子树
		// （visit 在 clip 前，背景/边框不被裁，只裁 children）。这里
		// 不能再 clip padding box——会把 owner 自身的 border-box 背景
		// 和边框一起裁掉（popup 四边框消失的根因）。CSS 规范：
		// overflow clip 仅作用于内容，元素自身背景/边框完整绘制。
		// ★ opacity 阈值：opacity∈[0.98,1) 不建 SaveLayer（offscreen 合成在
		// raster 上 ~0.7ms/次，是 paint 的主成本——200 combos 时 405 次
		// opacity 层 ≈ 283ms/70%）。直接按 alpha 绘制（painter 的
		// CumulativeOpacity 乘法），0.98+ 的视觉差不可见。
		// ★ bounds 限制：SaveLayer 只分配元素区域（非全 surface）——
		// raster 合成从 ~0.7ms 降到 ~µs。
		// ★ mask-image 子树遮罩：mask 包裹整棵子树（含 opacity 合成），
		// 见 paintLayerWithEffects（fixed 分支同样适用）。
		paintLayerWithEffects(layer, info, layerRect)
		info.canvas.Restore()
		// Rebalance the save stack for the outer recursion: ancestors'
		// saves were popped by RestoreToCount; push a fresh save so the
		// outer Restore() has a matching entry. Later siblings paint in the
		// clip-free viewport state, which is correct for fixed layers.
		info.scrollTranslateX, info.scrollTranslateY = savedScrollTX, savedScrollTY
		info.canvas.RestoreToCount(info.initialSaveCount)
		info.canvas.Save()
		return
	}
	if hasClip {
		// ★ Layer clips from CalculateRects are ABSOLUTE coordinates, but
		// Clip() applies the current transform first. Child layers paint
		// under an ancestor scroll translate (scrollTranslate != 0), so an
		// un-shifted clip would land -scroll HIGHER in device space than its
		// content — the scrolled-in layer content (position:relative items)
		// is culled. Shift the rect back by the accumulated scroll offset:
		// after Clip() applies the transform the clip lands at the
		// screen-fixed viewport position, matching WebKit where the ancestor
		// overflow clip stays in layer coords while content scrolls inside
		// it. The scroll container itself is clipped BEFORE the translate
		// (scrollTranslate == 0 here), so its clip is not shifted.
		clip.X += info.scrollTranslateX
		clip.Y += info.scrollTranslateY
		if paintDebugEnabled() {
			m := info.canvas.GetMatrix()
			log.Printf("[paint/layer] %s clip=%.0f,%.0f %.0fx%.0f scrollT=(%.0f,%.0f) ctmY=%.1f",
				layerName(layer), clip.X, clip.Y, clip.Width, clip.Height,
				info.scrollTranslateX, info.scrollTranslateY, m.TransY)
		}
		// ★ border-radius + overflow:hidden → rounded clip, mirroring
		// RenderLayer::paintLayer clipping children to the owner's rounded
		// border box. A rect clip would let children's square corners
		// bleed past the rounded corners (e.g. a .comp-bar pill whose
		// segment children poke out of the radius). Only the layer's OWN
		// overflow establishes the rounded clip — an ancestor's clip was
		// already rounded at that ancestor's layer entry.
		radius := 0.0
		if st := layer.Owner().Style(); st != nil &&
			(st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible) {
			radius = lengthValue(st.BorderRadius)
			if radius > 0 {
				// Overflow rounded clip applies to the PADDING box (WebKit
				// RenderLayer::paintLayer clips children to the owner's
				// rounded padding box): inset the rect by the border widths
				// and shrink the radius by the border, otherwise the arc is
				// 1px larger and centered on the border edge — the child
				// segments start at the padding edge so their corners stay
				// inside the arc and the pill's ends look square instead of
				// rounded (the arc must cut INTO the padding area).
				bw := lengthValue(st.BorderLeftWidth)
				bh := lengthValue(st.BorderTopWidth)
				insetX, insetY := bw, bh
				// If a side has no border, the border edge == padding edge
				// for that side; use per-side insets.
				if bw <= 0 {
					bw = lengthValue(st.BorderRightWidth)
					insetX = bw
				}
				if bh <= 0 {
					bh = lengthValue(st.BorderBottomWidth)
					insetY = bh
				}
				cw, ch := clip.Width-2*insetX, clip.Height-2*insetY
				if cw > 0 && ch > 0 {
					clip.X += insetX
					clip.Y += insetY
					clip.Width, clip.Height = cw, ch
					radius -= (insetX + insetY) / 2
					if radius < 0 {
						radius = 0
					}
				}
			}
		}
		if radius > 0 {
			info.canvas.ClipRoundRect(clip.X, clip.Y, clip.Width, clip.Height, radius)
		} else {
			info.canvas.Clip(graphics.Rect{X: clip.X, Y: clip.Y, Width: clip.Width, Height: clip.Height})
		}
		// ★ dirty-rect 层级别早退（局部重绘核心）：启用局部重绘时，层
		// clip（视口坐标，含 scroll 偏移补偿）与 dirty rect 无交 → 整层
		// 子树都可跳过（其内容全被此 clip 限制，绝不可能落在脏区内）。
		// 全量重绘（dirty check 关闭）不受影响。这让 hover/局部变化时
		// paint 只遍历脏区附近的对象，而非整棵 12K 对象层树。
		if info.DirtyCheckEnabled() && clip.Width > 0 && clip.Height > 0 {
			dr := info.dirtyRect
			if dr.Width <= 0 || dr.Height <= 0 ||
				clip.X >= dr.X+dr.Width || clip.X+clip.Width <= dr.X ||
				clip.Y >= dr.Y+dr.Height || clip.Y+clip.Height <= dr.Y {
				info.canvas.Restore() // 抵消 paintLayerTree 开头的 Save
				return
			}
		}
	}
	// ★ opacity∈[0.98,1) 直接 alpha 绘制（省 offscreen 合成，见 fixed 分支）。
	// ★ bounds 限制：SaveLayer 只分配元素区域，raster 合成 ~0.7ms→µs。
	// ★ WB_NO_SAVELAYER=1：跳过 SaveLayer 离屏合成（opacity 直接用 alpha
	//   绘制）——GPU 后端 SaveLayer 合成丢失时（内容画进离屏层但 Restore
	//   合成不上主画布 → 元素整体消失，如状态栏 status-item 文本/圆点），
	//   用此开关验证"离屏合成是元凶"。
	// ★ mask-image 子树遮罩：mask 包裹整棵子树（含 opacity 合成），见
	//   paintLayerWithEffects。
	paintLayerWithEffects(layer, info, layerRect)
	if hasClip || isFixedLayer {
		info.canvas.Restore()
	}
}

// paintLayerContents paints the layer owner's subtree (excluding child layer
// owners) then recurses into child layers in CSS stacking order. Shared by the
// fixed-layer branch (clip-free viewport state) and the normal branch.
func paintLayerContents(layer *RenderLayer, info *PaintInfo) {
	// ★ Scrolled content — unified scroll translate. A scroll container's
	// ENTIRE content must move by -scroll as one unit: the non-layer
	// children painted via paintLayerContent AND the child layers painted by
	// the paintLayerTree recursion below. WebKit gets this implicitly
	// because layer geometry is computed against the scrolled content origin
	// (RenderLayer::updateLayerPositions subtracts the parent's scroll
	// position), so paints and clips share one coordinate system and no
	// per-content translate exists. This port keeps layout coordinates
	// absolute and applies the scroll as a canvas translate instead, so the
	// translate must wrap BOTH content paths — exactly like
	// RenderLayer::paintLayer applying the scroll offset before
	// paintLayerContents. The layer's own clip was already applied by
	// paintLayerTree (CalculateRects, before this translate), so the
	// viewport stays fixed while content moves inside it.
	var scrollRestore bool
	var scrollCheckWasEnabled bool
	var scrollDirtyShifted bool
	var scrollSX, scrollSY float64
	if info != nil && info.rv != nil {
		if rb := asRenderBox(layer.Owner()); rb != nil {
			if st := rb.Style(); st != nil &&
				(st.OverflowX == style.OverflowAuto || st.OverflowX == style.OverflowScroll ||
					st.OverflowY == style.OverflowAuto || st.OverflowY == style.OverflowScroll) {
				if sx, sy := info.rv.BoxScrollOffset(rb); sx != 0 || sy != 0 {
					if os.Getenv("WB_GUTTER_DEBUG") != "" {
						log.Printf("[scroll-tr] layer=%s off=(%.0f,%.0f) translate(-%.0f,-%.0f)", layerName(layer), sx, sy, sx, sy)
					}
					info.canvas.Save()
					info.canvas.Translate(-sx, -sy)
					scrollRestore = true
					scrollSX, scrollSY = sx, sy
					// ★ 滚动感知的 dirty check：translate 后内容用内容坐标
					// 绘制，而 dirty rect 是视口坐标——把 dirty rect 平移
					// +scroll 到内容坐标即可在滚动容器内做局部重绘（跳过
					// 视口外内容），而非全量禁用。若本轮没有局部 dirty rect
					// （全量重绘，如滚动本身 MarkAllDirty），保持全量。
					// 滚动变化路径（SetBoxScrollOffset）已 MarkAllDirty →
					// dirty rect 全屏 → intersects 全部通过，不受影响。
					scrollCheckWasEnabled = info.DirtyCheckEnabled()
					if scrollCheckWasEnabled && info.dirtyRect.Width > 0 && info.dirtyRect.Height > 0 {
						info.dirtyRect.X += sx
						info.dirtyRect.Y += sy
						scrollDirtyShifted = true
					} else {
						info.SetDirtyCheckEnabled(false)
					}
					// ★ 累计滚动偏移：walkSubtreeExcluded 的滚动条绘制在
					// translate 下运行，而滚动条几何是绝对坐标（paddingBox），
					// 不补偿就会随内容一起滚走。用累计偏移把滚动条拉回
					// 固定视口位置（这与 WebKit 中 RenderScrollbar 独立于
					// 滚动内容、按层坐标绘制的行为一致）。
					info.scrollTranslateX += sx
					info.scrollTranslateY += sy
				}
			}
		}
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
	if scrollRestore {
		info.SetDirtyCheckEnabled(scrollCheckWasEnabled)
		if scrollDirtyShifted {
			info.dirtyRect.X -= scrollSX
			info.dirtyRect.Y -= scrollSY
		}
		info.scrollTranslateX -= scrollSX
		info.scrollTranslateY -= scrollSY
		info.canvas.Restore()
	}
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
	// ★ phase 感知的文本跳过：RenderText 只在 Foreground phase 绘制
	// （background/outline 均无操作，selection 由 PaintSelection 单独处理）。
	// 文本是树叶子（无子节点），直接返回省掉 bg/outline 两个 phase 的
	// 全部文本节点遍历——消息列表类页面文本占对象数 60%+。
	if info != nil {
		if _, isText := root.(*RenderText); isText {
			if info.Phase() == PhaseBackground || info.Phase() == PhaseOutline {
				return
			}
		}
	}
	if excluded[root] {
		return
	}

	// ★ 父级子树早退（局部重绘）：overflow 裁剪容器的内容被 clip 在
	// padding box 内——若该区域与 dirty rect 无交，整棵子树（含其所有
	// 后代对象）都无需绘制，直接跳过。这是聊天列表等长内容场景的关键
	// 优化：hover 局部重绘时只遍历脏区附近的子树，而非整棵 12K 对象树
	// （每对象 × 3 phase 的 visit 调用）。sticky（滚动 pin 后可能移入
	// 视口）/ transform（改变绘制空间）/ filter / opacity<1 元素保守
	// 不跳过——它们的实际绘制范围可能与布局 box 不一致。
	if info != nil && info.DirtyCheckEnabled() {
		dr := info.dirtyRect
		if dr.Width > 0 && dr.Height > 0 {
			if box := asRenderBox(root); box != nil {
				st := box.Style()
				if st != nil && !box.IsStickyPositioned() && st.Transform == "" && st.Filter == "" && st.Opacity >= 1 &&
					(st.OverflowX == style.OverflowHidden || st.OverflowX == style.OverflowAuto || st.OverflowX == style.OverflowScroll ||
						st.OverflowY == style.OverflowHidden || st.OverflowY == style.OverflowAuto || st.OverflowY == style.OverflowScroll) {
					pb := box.PaddingBoxRect()
					if pb.X+pb.Width <= dr.X || pb.X >= dr.X+dr.Width ||
						pb.Y+pb.Height <= dr.Y || pb.Y >= dr.Y+dr.Height {
						return
					}
				}
			}
		}
	}

	if paintDebugEnabled() && info != nil && info.rv != nil {
		if rb := asRenderBox(root); rb != nil {
			if st := rb.Style(); st != nil {
				sx, sy := info.rv.BoxScrollOffset(rb)
				log.Printf("[walk] %s ovfY=%d off=(%.0f,%.0f)", objName(root), st.OverflowY, sx, sy)
			}
		}
	}

	// CSS sticky: apply the scroll-pinning translate OUTSIDE the box's own
	// transform and overflow clip, mirroring RenderBox::stickyPositionOffset
	// applied by the nearest scrolling ancestor during paint.
	needsStickyRestore := false
	savedStickyDx, savedStickyDy := 0.0, 0.0
	if rb := asRenderBox(root); rb != nil && info != nil && info.canvas != nil && info.rv != nil {
		if rb.IsStickyPositioned() {
			if dx, dy := computeStickyOffset(rb, info.rv); dx != 0 || dy != 0 {
				info.canvas.Save()
				info.canvas.Translate(dx, dy)
				needsStickyRestore = true
				savedStickyDx, savedStickyDy = info.stickyDx, info.stickyDy
				info.stickyDx, info.stickyDy = dx, dy
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
			if applyTransformOpsSized(info.canvas, st.Transform, box.Width(), box.Height()) {
				info.canvas.Translate(-originX, -originY)
				needsTransformRestore = true
			} else {
				info.canvas.Restore()
			}
		}
	}

	// ★ 对象级快速剔除（局部重绘）：paintObjectBackground/Foreground/
	// Outline 对每个对象先做 backdrop-filter/clip-path/blend/filter 的
	// GetProperty 解析再绘制——即使对象最终被 intersects 跳过，这些
	// 检查本身在 12K 对象 × 3 phase 下就是主要成本。与 dirty rect 无交
	// 的普通 box 直接跳过 visit（子对象仍遍历，各自再判——overflow
	// visible 的子内容可能越界）。sticky/transform/filter/opacity<1 对象
	// 的绘制位置与布局 box 不一致，保守不跳过。
	// ★ 对象级快速剔除（局部重绘）：paintObjectBackground/Foreground/
	// Outline 对每个对象先做 backdrop-filter/clip-path/blend/filter 的
	// GetProperty 解析再绘制——即使对象最终被 intersects 跳过，这些
	// 检查本身在 12K 对象 × 3 phase 下就是主要成本。与 dirty rect 无交
	// 的普通 box 直接跳过 visit（子对象仍遍历，各自再判——overflow
	// visible 的子内容可能越界）。sticky/transform/filter/opacity<1 对象
	// 的绘制位置与布局 box 不一致，保守不跳过。
	// ★ 几何先行：先取 box 几何判断无交（便宜），命中后才取 Style 做
	// 保守条件检查——无交对象（绝大多数）省掉 Style() 调用。
	doVisit := true
	if info != nil && info.DirtyCheckEnabled() {
		dr := info.dirtyRect
		if dr.Width > 0 && dr.Height > 0 {
			if box := asRenderBox(root); box != nil && !box.IsStickyPositioned() {
				bx, by, bw, bh := box.X(), box.Y(), box.Width(), box.Height()
				if bw > 0 && bh > 0 &&
					(bx+bw <= dr.X || bx >= dr.X+dr.Width ||
						by+bh <= dr.Y || by >= dr.Y+dr.Height) {
					if st := box.Style(); st == nil || (st.Transform == "" && st.Filter == "" && st.Opacity >= 1) {
						doVisit = false
					}
				}
			}
		}
	}
	if doVisit {
		visit(root, info)
	}

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
	scrollbarPaint := false
	if clipBox != nil && info != nil && info.canvas != nil {
		// ★ Scrolled containers (scroll≠0) previously skipped the clip here,
		// relying solely on paintLayerTree's layer clip (CalculateRects).
		// That single point of failure let overflow content (e.g. a CM6
		// .cm-content 508px wide inside a 98px .cm-scroller) paint past the
		// container when anything diverged (dirty-rect early-out, layer
		// chain gap) — matching the browser where an overflow container
		// ALWAYS clips its subtree. Now clip unconditionally: when scroll≠0
		// this walk runs under paintLayerContents' scroll translate (content
		// coordinates) while PaddingBoxRect() is absolute, so shift the clip
		// rect back by this box's own scroll — after the transform it lands
		// on the screen-fixed viewport, same as the layer clip. scrollbarPaint
		// stays true so the scrollbars below still paint (they are
		// screen-anchored via the scrollTranslate compensation further down).
		info.canvas.Save()
		pb := clipBox.PaddingBoxRect()
		if paintDebugEnabled() {
			log.Printf("[paint/walk] %s clip=%.0f,%.0f %.0fx%.0f scroll=(%.0f,%.0f)",
				objName(root), pb.X, pb.Y, pb.Width, pb.Height, scrollSX, scrollSY)
		}
		info.canvas.Clip(graphics.Rect{X: pb.X + scrollSX, Y: pb.Y + scrollSY, Width: pb.Width, Height: pb.Height})
		needsClipRestore = true
		scrollbarPaint = true
	}

	// Paint children. The scroll translate is NOT applied here: scroll
	// containers are always layers (RequiresLayer) and paintLayerContents
	// applies one unified translate around both the non-layer content
	// (this walk) and the child layers, exactly like WebKit applying the
	// scroll offset before RenderLayer::paintLayerContents. Applying it in
	// both places would double-scroll the content.
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		walkSubtreeExcluded(c, excluded, info, visit)
	}

	// ── Overflow controls (painted in clip-only state, no translate) ──
	if scrollbarPaint && info.Phase() == PhaseForeground {
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
				// ★ Scrollbar geometry is absolute (padding-box) but this
				// walk runs under paintLayerContents' scroll translate, so
				// compensate by the box's OWN scroll offset to keep the
				// scrollbar anchored to the container viewport (WebKit paints
				// RenderScrollbar in layer coords, outside the scrolled
				// content). NESTED scroll containers must NOT use the
				// accumulated info.scrollTranslateX/Y here: that includes
				// ANCESTOR scrolls (e.g. chat-messages scrolled 300px + this
				// box 100px = 400), but the ancestor translate already moved
				// the whole box (scrollbar included) in the canvas — adding
				// it again pins the scrollbar at the wrong absolute position
				// (container at device -165, scrollbar drawn at +135, a 300px
				// offset — "滚动条离开容器/滚到底后 thumb 与内容错位").
				if sx0, sy0 := info.rv.BoxScrollOffset(box); sx0 != 0 || sy0 != 0 {
					pb.X += sx0
					pb.Y += sy0
				}
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
						if style.DiagEnabled("scrollbar") && os.Getenv("WB_SB_DEBUG") != "" {
							if el, ok := box.Node().(*dom.Element); ok {
								style.Diagf("scrollbar", "VSB-GEO %q pb=(%.0f,%.0f %.0fx%.0f) scrollT=(%.0f,%.0f) self=(%.0f,%.0f) sy=%.0f",
									el.GetAttribute("class"), pb.X, pb.Y, pb.Width, pb.Height,
									info.scrollTranslateX, info.scrollTranslateY, sx, sy, sy)
							}
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
								style.Diagf("scrollbar", "VSB %q: scrollW=%.0f webkitSB=%v thumbCol=#%02x%02x%02x%02x trackCol=#%02x%02x%02x%02x vx=%.0f thumbY=%.0f thumbLen=%.0f trackLen=%.0f sy=%.0f maxScroll=%.0f rad=%.0f pbY=%.0f vy=%.0f",
									cn, scrollW, webkitSB,
									thumbCol.R, thumbCol.G, thumbCol.B, thumbCol.A,
									trackCol.R, trackCol.G, trackCol.B, trackCol.A,
									vx, thumbY, vm.ThumbLen, vm.TrackLen, sy, vm.MaxScroll, rad, box.PaddingBoxRect().Y, vy)
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
		info.stickyDx, info.stickyDy = savedStickyDx, savedStickyDy
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

// ComputeStickyOffsetForTest exposes computeStickyOffset for integration
// probes (desktop_probe) that verify sticky pinning in real layouts.
func ComputeStickyOffsetForTest(box *RenderBox, view *RenderView) (float64, float64) {
	return computeStickyOffset(box, view)
}

// computeStickyOffset returns the scroll-pinning translate for a
// position:sticky box, mirroring RenderBox::stickyPositionOffset() simplified
// to the document-level scroll offset. The box sticks to the nearest
// scrollport edge when its static position would otherwise leave it:
//
//	top: N   → pins when staticY+scrollY < N, clamped so the box never
//	          passes below the bottom of the viewport.
//	bottom: N → pins when staticBottom+scrollY > viewportBottom - N, clamped
//	          so the box never passes above the top of the viewport.
//
// The scroll offset comes from the nearest scrollable ancestor (overflow
// auto/scroll box) when one exists — sticky inside a nested scroll container
// (e.g. the "▲ 收起" button inside .chat-messages) must track THAT container's
// scroll, not the page-level FrameView offset which never changes while the
// container scrolls.
func computeStickyOffset(box *RenderBox, view *RenderView) (float64, float64) {
	st := box.Style()
	if st == nil {
		return 0, 0
	}
	// Find the nearest scrollable ancestor — the sticky scrollport. For a
	// box directly under the view the scroll offset is the page's; nested
	// scroll containers contribute their own BoxScrollOffset.
	scrollBox := (*RenderBox)(nil)
	for cur := box.Parent(); cur != nil; cur = cur.Parent() {
		if rb := asRenderBox(cur); rb != nil {
			if cs := rb.Style(); cs != nil {
				if cs.OverflowX == style.OverflowAuto || cs.OverflowX == style.OverflowScroll ||
					cs.OverflowY == style.OverflowAuto || cs.OverflowY == style.OverflowScroll {
					scrollBox = rb
					break
				}
			}
		}
	}
	var sy float64
	if scrollBox != nil {
		_, sy = view.BoxScrollOffset(scrollBox)
	} else {
		_, sy = view.ScrollOffset()
	}
	staticY := box.Y()
	staticBottom := staticY + box.Height()

	// Viewport-space position: scrolling down (sy>0) moves content up, so the
	// element's viewport top is staticY - sy. The canvas is already translated
	// by -scroll, so the element paints at staticY; a positive dy pulls it
	// down to pin at top once its viewport position passes the top line.
	if topRaw := st.GetProperty("top"); topRaw != "" && topRaw != "auto" {
		top := 0.0
		if l, ok := parseCSSLength(topRaw); ok {
			top = l
		} else {
			top = 0
		}
		vy := staticY - sy
		if vy < top && staticBottom > 0 {
			dy := top - vy
			return 0, dy
		}
	}
	// bottom: N — pins the box's bottom edge N px above the scrollport
	// bottom once its static bottom would scroll below it (like the "▲ 收起"
	// button staying visible at the bottom of the chat viewport while the
	// thinking text scrolls). Viewport bottom in this coordinate space is the
	// scroll container's padding-box bottom (or the view height for page
	// scroll) minus N; the box sticks with dy pushing it back UP into view.
	if bottomRaw := st.GetProperty("bottom"); bottomRaw != "" && bottomRaw != "auto" {
		bottom := 0.0
		if l, ok := parseCSSLength(bottomRaw); ok {
			bottom = l
		}
		var viewportBottom float64
		if scrollBox != nil {
			pb := scrollBox.PaddingBoxRect()
			viewportBottom = pb.Y + pb.Height
		} else {
			viewportBottom = view.ViewHeight()
		}
		// Element's viewport-space bottom after scroll.
		vb := staticBottom - sy
		if vb > viewportBottom-bottom {
			dy := (viewportBottom - bottom) - vb
			return 0, dy
		}
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
	if paintStatsEnabled() {
		paintStatsVisits++
	}
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
	// CSS mask-image is applied at the RENDER LAYER level (paintLayerWithEffects)
	// so it masks the whole subtree (background + foreground + descendants),
	// not just background+border. Do NOT re-apply it here.
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
// paintObjectForeground paints non-text foreground content: SVG shapes and
// native form controls (checkbox/radio/range/progress/meter/select arrow) via
// PaintFormControl mirroring RenderTheme::paint().
func paintObjectForeground(o RenderObject, info *PaintInfo) {
	if text, ok := o.(*RenderText); ok {
		PaintText(text, info)
		return
	}
	// Check for custom elements (SVG handled below).
	box := asRenderBox(o)
	if box == nil || !box.IsVisible() {
		return
	}
	el, ok := box.Node().(*dom.Element)
	if !ok {
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
	// iframe elements: paint the child frame's document into the content box
	// (RenderIFrame). Returns true when a child document was painted.
	if el.LocalName() == "iframe" {
		PaintIFrame(box, info)
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
