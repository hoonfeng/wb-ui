// Translation of: Source/WebCore/layout/formattingContexts/flex/FlexFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/flex/FlexLayout.cpp
//                  Source/WebCore/rendering/RenderFlexibleBox.cpp (layoutBlock part)
// Completeness: 50%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - flex-wrap is supported for single and multi-line (wrap / wrap-reverse); lines
//     are packed top-to-bottom (or bottom-to-top for wrap-reverse)
//   - flex-basis resolves against the main-axis content size; auto basis falls back to
//     the content size (approximated by the child's intrinsic/main size)
//   - flex-grow / flex-shrink distribute free / negative space proportionally
//   - main-axis alignment (justify-content) supports flex-start / center / flex-end /
//     space-between / space-around / space-evenly
//   - cross-axis alignment (align-items / align-self) supports stretch / flex-start /
//     center / flex-end / baseline (baseline approximated as flex-start)
//   - align-content (multi-line cross packing) supports stretch / flex-start / center /
//     flex-end / space-between / space-around
//   - order is honoured for visual reordering; source order is the tie-breaker
//   - min/max constraints are applied per item; min-content sizing is approximated
//
// Architecture note (2026-07 refactoring):
//   FlexFormattingContext.Layout() is split into three phases so that
//   needsContentRelayout can re-lay-out children at the final cross-axis size
//   WITHOUT re-running flex-grow/shrink. Each phase is a named method:
//
//     Phase 1: measureItems       — collect items, resolve basis, measure content
//     Phase 2: distributeLines    — line breaking + flex-grow/shrink allocation
//     Phase 3: positionAndFinalize — cross-axis sizing, alignment, positioning,
//                                    auto-height, child re-layout
//
//   Layout() calls all three phases. The public RelayoutCrossAxis() calls only the
//   cross-axis micro-adjustment (derived from Phase 3) for a single item, WITHOUT
//   re-running flex-grow or re-measuring content. This breaks the cascading
//   inflation chain that plagued earlier versions.

package layout

import (
	"math"
	"sort"
	"wb-ui/style"
)

// FlexFormattingContext is the Go translation of WebCore::Layout::FlexFormattingContext
// / RenderFlexibleBox. It lays out a flex container's items along the main and cross
// axes per CSS Flexible Box Layout Module Level 1.
type FlexFormattingContext struct{}

// Layout lays out box's flex items in three phases:
//   1. measureItems — collect, measure, resolve basis
//   2. distributeLines — line breaking + flex-grow/shrink
//   3. positionAndFinalize — position, stretch, auto-height, child re-layout
func (c *FlexFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}

	// ─── Phase 1: 测量 ──────────────────────────────────────────────
	items, isRow, isReverse, mainSize, crossSize, skipDistribution, lines, deferredAbsolutes := c.measureItems(box, state)

	// ─── Phase 2: 分配 flex-grow/shrink ──────────────────────────────
	c.distributeLines(lines, mainSize, skipDistribution, isRow)

	// ─── Phase 3: 定位 + 交叉轴调整 + auto-height + 子重排 ──────────
	c.positionAndFinalize(box, state, items, lines, isRow, isReverse, mainSize, crossSize, deferredAbsolutes)
}

// measureItems performs Phase 1: collect items, resolve box model, measure content,
// determine main/cross sizes, line breaking. Returns all intermediate structures
// needed by subsequent phases.
func (c *FlexFormattingContext) measureItems(box *LayoutBox, state *LayoutState) (
	items []flexItem,
	isRow, isReverse bool,
	mainSize, crossSize float64,
	skipDistribution bool,
	lines []flexLine,
	deferredAbsolutes []*LayoutBox,
) {
	contentWidth := box.Rect.ContentWidth()
	contentHeight := box.Rect.ContentHeight()

	isRow = effectiveIsRow(box)
	isReverse = effectiveIsReverse(box)
	wrap := flexWrapOf(box)

	// Collect absolutely positioned children (deferred).
	for _, child := range box.Children {
		if child.IsAbsolutelyPositioned() {
			deferredAbsolutes = append(deferredAbsolutes, child)
		}
	}

	// Collect visible in-flow flex items, sorted by order.
	items = collectFlexItems(box)
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })

	// Resolve each item's hypothetical main size and box model.
	for i := range items {
		it := &items[i]
		margin, padding, border := computeBoxModel(it.box, contentWidth, fontSizeOf(it.box))
		it.margin = margin
		it.padding = padding
		it.border = border
		it.box.Rect.Margin = margin
		it.box.Rect.Padding = padding
		it.box.Rect.Border = border
		it.mainMargin = mainAxisMargin(margin, isRow)
		it.crossMargin = crossAxisMargin(margin, isRow)
		it.mainBorderPadding = mainAxisBorderPadding(border, padding, isRow)
		it.crossBorderPadding = crossAxisBorderPadding(border, padding, isRow)

		basis := resolveFlexBasis(it.box, contentWidth, contentHeight, isRow)
		if flexBasisIsContent(it.box, isRow) {
			measW := mainSizeAvailable(isRow, contentWidth, contentHeight)
			if !isRow {
				if contentHeight <= 0 {
					measW = contentWidth
				} else {
					measW = contentHeight
				}
			}
			basis = measureFlexItemContentMain(it.box, measW, isRow, state)
		}
		it.hypotheticalMain = basis + it.mainBorderPadding
	}

	// Main / cross available sizes.
	if isRow {
		mainSize, crossSize = contentWidth, contentHeight
	} else {
		mainSize, crossSize = contentHeight, contentWidth
	}

	// When the container's main axis size is auto (CSS height/width:auto),
	// flex-grow and flex-shrink do NOT apply per spec — items keep their
	// content-based sizes.
	// NOTE: we check the actual content size (which may come from
	// stretchRootToViewport via the box.Rect), not the CSS property. A flex
	// container that fills the viewport has a definite main-axis dimension
	// even though its CSS width/height is auto (unset). Per spec, flex-grow
	// applies when the container's used main-axis size is definite.
	if isRow {
		skipDistribution = contentWidth <= 0
	} else {
		skipDistribution = contentHeight <= 0
	}

	// Line breaking.
	lines = breakFlexLines(items, mainSize, wrap)

	return
}

// distributeLines performs Phase 2: flex-grow/shrink distribution per line.
// This phase is NOT re-run during needsContentRelayout.
func (c *FlexFormattingContext) distributeLines(lines []flexLine, mainSize float64, skipDistribution bool, isRow bool) {
	for li := range lines {
		ln := &lines[li]
		freezeLine := ln.items
		for i := range freezeLine {
			freezeLine[i].mainSize = freezeLine[i].hypotheticalMain
		}
		if skipDistribution {
			// Freeze all without distribution.
			for i := range freezeLine {
				freezeLine[i].frozen = true
			}
			continue
		}
		// Iterative flex-grow / flex-shrink with min/max clamping.
		for pass := 0; pass < 2; pass++ {
			lineMain := 0.0
			for i := range freezeLine {
				lineMain += freezeLine[i].mainSize
			}
			free := mainSize - lineMain - lineMainMargins(freezeLine)
			if math.Abs(free) < 1e-6 {
				break
			}
			if free > 0 {
				totalGrow := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalGrow += freezeLine[i].grow
					}
				}
				if totalGrow <= 0 {
					break
				}
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].grow <= 0 {
						continue
					}
					freezeLine[i].mainSize += free * freezeLine[i].grow / totalGrow
				}
			} else {
				totalShrink := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalShrink += freezeLine[i].shrink * freezeLine[i].hypotheticalMain
					}
				}
				if totalShrink <= 0 {
					break
				}
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].shrink <= 0 {
						continue
					}
					weight := freezeLine[i].shrink * freezeLine[i].hypotheticalMain / totalShrink
					freezeLine[i].mainSize += free * weight
				}
			}
			// Freeze items at min/max.
			for i := range freezeLine {
				if freezeLine[i].frozen {
					continue
				}
				min, max := flexMinMax(freezeLine[i], isRow, mainSize)
				if freezeLine[i].mainSize < min {
					freezeLine[i].mainSize = min
					freezeLine[i].frozen = true
				} else if freezeLine[i].mainSize > max {
					freezeLine[i].mainSize = max
					freezeLine[i].frozen = true
				}
			}
		}
		// Freeze all remaining.
		for i := range freezeLine {
			freezeLine[i].frozen = true
		}
	}
}

// positionAndFinalize performs Phase 3: cross-axis sizing, alignment,
// positioning, auto-height, and cross-axis child re-layout.
// This is called ONCE by Layout() and can also be called per-item by
// RelayoutCrossAxis() (without re-running Phase 1 or Phase 2).
func (c *FlexFormattingContext) positionAndFinalize(
	box *LayoutBox, state *LayoutState,
	items []flexItem,
	lines []flexLine,
	isRow, isReverse bool,
	mainSize, crossSize float64,
	deferredAbsolutes []*LayoutBox,
) {
	contentWidth := box.Rect.ContentWidth()
	contentHeight := box.Rect.ContentHeight()

	// 3a. Cross-size measurement: lay out each item at its allocated main size,
	// measure the cross-axis content.
	for li := range lines {
		ln := &lines[li]
		freezeLine := ln.items
		for i := range freezeLine {
			it := &freezeLine[i]
			setItemBorderBox(it, isRow)

			// Pre-set provisional cross-axis so inner formatting contexts
			// have a reference for percentage sizing. Only for block-level
			// containers (text/inline should use content-based sizing).
			//
			// When the parent's cross-axis is indefinite (contentHeight=0 for
			// row, contentWidth=0 for column and CrossSizeDefinite=false),
			// use CrossSizeFallback for measurement so children have room
			// to compute their natural sizes. The cross-axis is restored to
			// 0 after measurement so stretch is correctly skipped in Phase 3c.
			crossDefinite := false
			if isRow {
				crossDefinite = contentHeight > 0 && state.CrossSizeDefinite
				if it.box.Type != BoxTextRun && !it.box.IsInline() {
					if crossDefinite {
						it.box.Rect.Height = contentHeight
					} else if state.CrossSizeFallback > 0 {
						it.box.Rect.Height = state.CrossSizeFallback
					}
				}
			} else {
				crossDefinite = contentWidth > 0 && state.CrossSizeDefinite
				if it.box.Type != BoxTextRun && !it.box.IsInline() {
					if crossDefinite {
						it.box.Rect.Width = contentWidth
					} else if state.CrossSizeFallback > 0 {
						it.box.Rect.Width = state.CrossSizeFallback
					}
				}
			}
			it.box.Rect.X = 0
			it.box.Rect.Y = 0

			// Lay out item's content at the allocated main size.
			childCtx := contextFor(it.box)
			childCtx.Layout(it.box, state)

			// Restore main-axis size after child layout (child may have
			// overwritten it via auto-sizing).
			setItemBorderBox(it, isRow)

			// Measure content-based cross size.
			if it.box.Type == BoxTextRun || it.box.IsInline() || it.box.IsInlineLevel() {
				if isRow {
					it.crossSize = maxContentBottom(it.box) - it.box.Rect.ContentY()
				} else {
					it.crossSize = maxContentWidth(it.box)
				}
			} else {
				it.crossSize = crossAxisSize(it.box, isRow)
			}

			// If cross-size is indefinite and fallback was used, restore item's
			// provisional cross-axis to 0 so stretch is skipped in Phase 3c.
			if !crossDefinite {
				if isRow {
					it.box.Rect.Height = 0
				} else {
					it.box.Rect.Width = 0
				}
			}
		}
		// Line cross size = max of item cross sizes.
		ln.crossSize = 0
		for i := range freezeLine {
			c := freezeLine[i].crossSize + freezeLine[i].crossMargin
			if c > ln.crossSize {
				ln.crossSize = c
			}
		}
	}

	// 3b. Align-content: distribute lines along cross axis.
	totalCross := 0.0
	for _, ln := range lines {
		totalCross += ln.crossSize
	}
	crossStart := 0.0
	lineGap := 0.0
	alignContent := alignContentOf(box)
	if len(lines) > 1 && (alignContent == "space-between" || alignContent == "space-around" || alignContent == "space-evenly") {
		gaps := len(lines) - 1
		extra := crossSize - totalCross
		if extra < 0 {
			extra = 0
		}
		switch alignContent {
		case "space-between":
			lineGap = extra / float64(gaps)
		case "space-around":
			lineGap = extra / float64(gaps)
			crossStart = lineGap / 2
		case "space-evenly":
			lineGap = extra / float64(gaps+1)
			crossStart = lineGap
		}
	} else if len(lines) > 0 && alignContent == "center" {
		crossStart = (crossSize - totalCross) / 2
		if crossStart < 0 {
			crossStart = 0
		}
	} else if len(lines) > 0 && alignContent == "flex-end" {
		crossStart = math.Max(0, crossSize-totalCross)
	} else if alignContent == "stretch" && len(lines) > 0 {
		extra := crossSize - totalCross
		if extra > 0 {
			per := extra / float64(len(lines))
			for i := range lines {
				lines[i].crossSize += per
			}
		}
	}

	// 3c. Position items within lines (justify-content + align-items/self).
	cursorCross := crossStart
	for li := range lines {
		ln := &lines[li]
		lineMainUsed := 0.0
		for i := range ln.items {
			lineMainUsed += ln.items[i].mainSize + ln.items[i].mainMargin
		}
		mainFree := mainSize - lineMainUsed
		if mainFree < 0 {
			mainFree = 0
		}
		if mainSize >= 1e5 {
			mainFree = 0
		}
		just := justifyContentOf(box)
		mainCursor, mainGap := 0.0, 0.0
		switch just {
		case "center":
			mainCursor = mainFree / 2
		case "flex-end":
			mainCursor = mainFree
		case "space-between":
			mainGap = mainFree / float64(max(1, len(ln.items)-1))
		case "space-around":
			mainGap = mainFree / float64(len(ln.items))
			mainCursor = mainGap / 2
		case "space-evenly":
			mainGap = mainFree / float64(len(ln.items)+1)
			mainCursor = mainGap
		}
		if isReverse {
			mainCursor = mainSize - mainCursor
		}
		for i := range ln.items {
			it := &ln.items[i]
			if isReverse {
				mainCursor -= it.mainMargin/2 + it.mainSize
				setItemPosition(it, mainCursor, cursorCross+ln.crossSize, isRow, box)
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor -= it.mainMargin/2 + mainGap
			} else {
				mainCursor += it.mainMargin / 2
				setItemPosition(it, mainCursor, cursorCross, isRow, box)
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor += it.mainSize + it.mainMargin/2 + mainGap
			}
		}
		cursorCross += ln.crossSize + lineGap
	}

	// 3d. Auto height of the container.
	if state != nil && state.CrossAxisRelayout {
		// 交叉轴重排模式：从 children 的实际内容计算 auto-height，
		// 而不是使用 flex system 的 mainSize（skipDistribution=true 时为 0）。
		if heightIsAuto(box) {
			if isRow {
				box.Rect.Height = cursorCross + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
					box.Rect.Border.Top + box.Rect.Border.Bottom
			} else {
				ch := contentHeightOfBox(box)
				if ch > 0 {
					box.Rect.Height = ch + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
						box.Rect.Border.Top + box.Rect.Border.Bottom
				}
				// 如果 ch == 0，保留主轴尺寸不变（由父 flex-grow 设置）
			}
		}
	} else if heightIsAuto(box) {
		if isRow {
			box.Rect.Height = cursorCross + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
				box.Rect.Border.Top + box.Rect.Border.Bottom
		} else {
			box.Rect.Height = totalLineMain(lines) + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
				box.Rect.Border.Top + box.Rect.Border.Bottom
		}
		maxAutoH := 800.0
		if state != nil && state.ViewportHeight > 0 {
			maxAutoH = state.ViewportHeight
		}
		if box.Rect.Height > maxAutoH {
			box.Rect.Height = maxAutoH
		}
	}

	// 3e. Cross-axis child re-layout: items whose cross-axis was changed
	// by stretch need their children re-laid-out at the final size.
	// This does NOT re-run flex-grow — it only adjusts the cross-axis
	// of nested flex/grid containers and re-lays-out their children.
	// NOTE: we do NOT reset the main-axis size (height for column parent,
	// width for row parent). The main-axis was set by Phase 2 flex-grow
	// or by the parent's stretch and is the correct used size.
	//
	// CRITICAL: use the POST-auto-height content dimensions, not the
	// Phase 3a values (which may be 0 for indefinite cross-axis).
	finalContentW := box.Rect.ContentWidth()
	finalContentH := box.Rect.ContentHeight()
	for i := range items {
		it := &items[i]
		if needsContentRelayout(it.box) {
			savedX, savedY := it.box.Rect.X, it.box.Rect.Y
			savedMainAxis := it.box.Rect.Height
			if isRow {
				savedMainAxis = it.box.Rect.Width
			}
			if isRow {
				// Parent is row: set child's cross-axis (height) to container content height.
				it.box.Rect.Height = finalContentH
			} else {
				// Parent is column: set child's cross-axis (width) to container content width.
				it.box.Rect.Width = finalContentW
			}
			// 在重排期间启用 stretch 和交叉轴重排标记
			savedDef := state.CrossSizeDefinite
			savedRelayout := state.CrossAxisRelayout
			state.CrossSizeDefinite = true
			state.CrossAxisRelayout = true
			ctx := contextFor(it.box)
			ctx.Layout(it.box, state)
			state.CrossSizeDefinite = savedDef
			state.CrossAxisRelayout = savedRelayout
			// 恢复位置和主轴尺寸
			it.box.Rect.X = savedX
			it.box.Rect.Y = savedY
			if isRow {
				it.box.Rect.Width = savedMainAxis
			} else {
				it.box.Rect.Height = savedMainAxis
			}
		}
	}

	// 3f. Lay out absolutely-positioned descendants.
	root := stateRoot(box)
	for _, child := range deferredAbsolutes {
		cb := containingBlockForAbsolute(child, root)
		layoutAbsolute(child, cb, root, state)
	}

	// (Debug output removed in 2026-07 refactoring. Use WB_VERBOSE logging
	// or the desktop diagnostic command for per-element rect tracing.)
}

// flexItem captures the resolved flex properties and geometry of a single flex item.
type flexItem struct {
	box                *LayoutBox
	order              int
	grow               float64
	shrink             float64
	basis              float64
	margin             Edges
	padding            Edges
	border             Edges
	mainMargin         float64
	crossMargin        float64
	mainBorderPadding  float64
	crossBorderPadding float64
	hypotheticalMain   float64
	mainSize           float64
	crossSize          float64
	frozen             bool
}

// flexLine groups items that share a single flex line.
type flexLine struct {
	items     []flexItem
	crossSize float64
}

// flexDirection constants.
type flexDir int

const (
	flexRow flexDir = iota
	flexRowReverse
	flexColumn
	flexColumnReverse
)

func flexDirectionOf(box *LayoutBox) flexDir {
	if box.Style == nil {
		return flexRow
	}
	switch box.Style.FlexDirection {
	case "row-reverse":
		return flexRowReverse
	case "column":
		return flexColumn
	case "column-reverse":
		return flexColumnReverse
	}
	return flexRow
}

// effectiveIsRow reports whether the main axis is the row axis, considering
// writing-mode. In horizontal-tb this is equivalent to flex-direction: row.
// In vertical writing-mode (vertical-rl/lr), the inline axis is vertical,
// so flex-direction: row maps to the vertical axis (returns false).
func effectiveIsRow(box *LayoutBox) bool {
	dir := flexDirectionOf(box)
	isRowPhys := dir == flexRow || dir == flexRowReverse
	if IsVerticalWritingMode(box.Style) {
		return !isRowPhys
	}
	return isRowPhys
}

// effectiveIsReverse reports whether the main axis direction is reversed.
func effectiveIsReverse(box *LayoutBox) bool {
	dir := flexDirectionOf(box)
	return dir == flexRowReverse || dir == flexColumnReverse
}

func flexWrapOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.FlexWrap
}

func alignContentOf(box *LayoutBox) string {
	if box.Style == nil {
		return "stretch"
	}
	v := box.Style.AlignContent
	if v == "" {
		return "stretch"
	}
	return v
}

func justifyContentOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.JustifyContent
}

func alignItemsOf(box *LayoutBox) string {
	if box.Style == nil {
		return "stretch"
	}
	v := box.Style.AlignItems
	if v == "" {
		return "stretch"
	}
	return v
}

// collectFlexItems returns the visible in-flow children of a flex container as flex
// items with their resolved flex-grow / flex-shrink / flex-basis.
func collectFlexItems(box *LayoutBox) []flexItem {
	var out []flexItem
	for _, child := range box.Children {
		if !child.IsVisible() || !child.IsInFlow() {
			continue
		}
		if child.Style == nil {
			child.Style = style.NewComputedStyle()
		}
		fi := flexItem{
			box:    child,
			order:  child.Style.Order,
			grow:   child.Style.FlexGrow,
			shrink: child.Style.FlexShrink,
		}
		out = append(out, fi)
	}
	return out
}

// resolveFlexBasis returns the flex-basis main size (content-box) of an item.
func resolveFlexBasis(box *LayoutBox, cbW, cbH float64, isRow bool) float64 {
	fs := fontSizeOf(box)
	r := resolveLengthAuto(box.Style.FlexBasis, ternary(isRow, cbW, cbH), fs)
	if !r.Auto && r.Definite {
		return r.Value
	}
	if isRow {
		w, ok := definiteWidth(box.Style.Width, cbW, fs)
		if ok {
			if isBorderBox(box) {
				w -= box.Rect.Border.Horizontal() + box.Rect.Padding.Horizontal()
				if w < 0 {
					w = 0
				}
			}
			return w
		}
	} else {
		h, ok := definiteHeight(box.Style.Height, cbH, fs)
		if ok {
			if isBorderBox(box) {
				h -= box.Rect.Border.Vertical() + box.Rect.Padding.Vertical()
				if h < 0 {
					h = 0
				}
			}
			return h
		}
	}
	return 0
}

func ternary(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}

func mainAxisMargin(m Edges, isRow bool) float64 {
	if isRow {
		return m.Left + m.Right
	}
	return m.Top + m.Bottom
}

func crossAxisMargin(m Edges, isRow bool) float64 {
	if isRow {
		return m.Top + m.Bottom
	}
	return m.Left + m.Right
}

func mainAxisBorderPadding(b, p Edges, isRow bool) float64 {
	if isRow {
		return b.Horizontal() + p.Horizontal()
	}
	return b.Vertical() + p.Vertical()
}

func crossAxisBorderPadding(b, p Edges, isRow bool) float64 {
	if isRow {
		return b.Vertical() + p.Vertical()
	}
	return b.Horizontal() + p.Horizontal()
}

func lineMainMargins(items []flexItem) float64 {
	s := 0.0
	for i := range items {
		s += items[i].mainMargin
	}
	return s
}

// setItemBorderBox sets the border-box width/height of an item from its main/cross size.
func setItemBorderBox(it *flexItem, isRow bool) {
	if isRow {
		it.box.Rect.Width = it.mainSize
	} else {
		it.box.Rect.Height = it.mainSize
	}
}

// crossAxisSize returns the used cross-axis border-box size of an item after layout.
func crossAxisSize(box *LayoutBox, isRow bool) float64 {
	if isRow {
		return box.Rect.Height
	}
	return box.Rect.Width
}

// crossAxisContentSize returns the content-box size on the cross axis.
func crossAxisContentSize(box *LayoutBox, isRow bool) float64 {
	if isRow {
		return box.Rect.ContentHeight()
	}
	return box.Rect.ContentWidth()
}

// setItemPosition positions an item at the given main/cross offset (relative to the
// flex container content box) applying align-self / align-items cross-axis alignment.
func setItemPosition(it *flexItem, mainOffset, crossOffset float64, isRow bool, container *LayoutBox) {
	contentX := container.Rect.ContentX()
	contentY := container.Rect.ContentY()
	align := it.box.Style.AlignSelf
	if align == "" || align == "auto" {
		align = alignItemsOf(container)
	}
	crossSize := it.crossSize
	if align == "stretch" {
		var containerCross float64
		if isRow {
			containerCross = container.Rect.Height - container.Rect.Border.Vertical() - container.Rect.Padding.Vertical()
		} else {
			containerCross = container.Rect.Width - container.Rect.Border.Horizontal() - container.Rect.Padding.Horizontal()
		}
		if containerCross > 0 {
			crossSize = containerCross - it.crossMargin
			if crossSize < 0 {
				crossSize = 0
			}
		}
		if isRow {
			it.box.Rect.Height = crossSize
		} else {
			it.box.Rect.Width = crossSize
		}
	}
	crossPos := crossOffset
	switch align {
	case "center":
		crossPos = crossOffset + (crossAxisContentSize(container, isRow)-it.crossSize-it.crossMargin)/2
	case "flex-end":
		crossPos = crossOffset + crossAxisContentSize(container, isRow) - it.crossSize - it.crossMargin
	}
	if crossPos < 0 {
		crossPos = 0
	}
	if align != "stretch" {
		if isRow {
			it.box.Rect.Height = it.crossSize
		} else {
			it.box.Rect.Width = it.crossSize
		}
	}
	if isRow {
		it.box.Rect.X = contentX + mainOffset + it.margin.Left
		it.box.Rect.Y = contentY + crossPos + it.margin.Top
	} else {
		it.box.Rect.X = contentX + crossPos + it.margin.Left
		it.box.Rect.Y = contentY + mainOffset + it.margin.Top
	}
}

// flexMinMax returns the (min, max) main-axis size for an item.
func flexMinMax(it flexItem, isRow bool, containerMainSize float64) (float64, float64) {
	fs := fontSizeOf(it.box)
	st := it.box.Style
	ref := containerMainSize
	if ref <= 0 {
		ref = 0
	}
	minV, maxV, minAuto, maxAuto := resolveMinMax(st.MinWidth, st.MaxWidth, ref, fs)
	if !isRow {
		minV, maxV, minAuto, maxAuto = resolveMinMax(st.MinHeight, st.MaxHeight, ref, fs)
	}
	if minAuto {
		minV = 0
	}
	if maxAuto {
		maxV = math.Inf(1)
	}
	return minV, maxV
}

// breakFlexLines packs items into lines according to flex-wrap.
func breakFlexLines(items []flexItem, mainSize float64, wrap string) []flexLine {
	if wrap == "" || wrap == "nowrap" || len(items) == 0 {
		return []flexLine{{items: items}}
	}
	var lines []flexLine
	var cur []flexItem
	curMain := 0.0
	for i := range items {
		add := items[i].hypotheticalMain + items[i].mainMargin
		if len(cur) > 0 && curMain+add > mainSize+1e-6 {
			lines = append(lines, flexLine{items: cur})
			cur = nil
			curMain = 0
		}
		cur = append(cur, items[i])
		curMain += add
	}
	if len(cur) > 0 {
		lines = append(lines, flexLine{items: cur})
	}
	if len(lines) == 0 {
		lines = []flexLine{{items: items}}
	}
	return lines
}

func totalLineMain(lines []flexLine) float64 {
	s := 0.0
	for _, ln := range lines {
		for _, it := range ln.items {
			// When mainSize is 0 (auto-height container with skipDistribution=true),
			// use the actual box height (computed by IFC/BFC during measurement)
			// as the fallback content size.
			h := it.mainSize
			if h <= 0 {
				h = it.box.Rect.Height
				if h <= 0 {
					h = contentHeightOfBox(it.box)
				}
			}
			s += h + it.mainMargin
		}
	}
	return s
}

// contentHeightOfBox returns the actual content height of box by walking the
// subtree and finding the max (child.Y + child.Height). It uses text segments
// (IFC-computed positions) for inline text and children's Rect for block-level
// boxes. Absolutely-positioned descendants are excluded.
func contentHeightOfBox(box *LayoutBox) float64 {
	if box == nil {
		return 0
	}
	maxBottom := 0.0
	var walk func(b *LayoutBox)
	walk = func(b *LayoutBox) {
		for _, seg := range b.TextSegments {
			if seg.Y >= 0 && !math.IsNaN(seg.Y) && !math.IsInf(seg.Y, 0) {
				bottom := seg.Y + seg.Height
				if bottom > maxBottom && bottom < 1e7 {
					maxBottom = bottom
		for _, c := range b.Children {
			if c.IsAbsolutelyPositioned() {
				continue
			}
			if c.Rect.Y >= 0 && !math.IsNaN(c.Rect.Y) && c.Rect.Height >= 0 && !math.IsNaN(c.Rect.Height) {
				bottom := c.Rect.Y + c.Rect.Height
				if bottom > maxBottom && bottom < 1e7 {
					maxBottom = bottom
				}
			}
			walk(c)
		}

// offsetSubtree shifts a layout box and all its descendants by (dx, dy).
func offsetSubtree(box *LayoutBox, dx, dy float64) {
	if box == nil {
		return
	}
	box.Rect.X += dx
	box.Rect.Y += dy
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children {
		offsetSubtree(child, dx, dy)
	}
}
}

// offsetItemSubtree offsets the descendants of a flex item without moving the item
// itself, which was already positioned by setItemPosition.
func offsetItemSubtree(box *LayoutBox, dx, dy float64) {
	if box == nil {
		return
	}
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children {
		offsetSubtree(child, dx, dy)
	}
}

// flexBasisIsContent reports whether the flex base size should be taken from
// the item's content (max-content).
func flexBasisIsContent(box *LayoutBox, isRow bool) bool {
	if box.Style == nil {
		return false
	}
	fs := fontSizeOf(box)
	bb := resolveLengthAuto(box.Style.FlexBasis, 0, fs)
	if !bb.Auto {
		return false
	}
	if isRow {
		w := resolveLengthAuto(box.Style.Width, 0, fs)
		return w.Auto
	}
	h := resolveLengthAuto(box.Style.Height, 0, fs)
	return h.Auto
}

// widthIsAuto reports whether the box has an auto width.
func widthIsAuto(box *LayoutBox) bool {
	if box.Style == nil {
		return true
	}
	r := resolveLengthAuto(box.Style.Width, 0, 0)
	return r.Auto
}

// mainSizeAvailable returns the available main-axis size for measurement.
func mainSizeAvailable(isRow bool, contentWidth, contentHeight float64) float64 {
	if isRow {
		return contentWidth
	}
	return contentHeight
}

// measureFlexItemContentMain lays out box provisionally at the available main
// size and measures its max-content main size by scanning descendant edges.
func measureFlexItemContentMain(box *LayoutBox, availableMain float64, isRow bool, state *LayoutState) float64 {
	savedX, savedY := box.Rect.X, box.Rect.Y
	defer func() { box.Rect.X, box.Rect.Y = savedX, savedY }()
	if isRow {
		box.Rect.Width = availableMain
		box.Rect.X = 0
		box.Rect.Y = 0
	} else {
		h := availableMain
		if h <= 0 || h >= 1e5 {
			if state != nil && state.ViewportHeight > 0 {
				h = state.ViewportHeight
			} else {
				h = 1280
			}
		}
		box.Rect.Height = h
		// FIX: 列 flex 子项测量时宽度=父容器 content width，不是视口宽度！
		// 旧代码用 stateWidth(state)=视口宽度(1280)，导致子项(如 file-explorer)
		// 测量宽度=1280 而非 Grid 列宽(167)，布局全部错乱。
		crossW := stateWidth(state)
		if box.parent != nil && box.parent.Rect.ContentWidth() > 0 {
			crossW = box.parent.Rect.ContentWidth()
		}
		box.Rect.Width = crossW
		box.Rect.X = 0
		box.Rect.Y = 0
	}
	ctx := contextFor(box)
	ctx.Layout(box, state)
	if isRow {
		return contentSpanWidth(box)
	}
	contentTop := box.Rect.ContentY()
	h := maxContentBottom(box) - contentTop
	if h < 0 {
		h = 0
	}
	clamp := stateHeight(state)
	if h > clamp {
		h = clamp
	}
	return h
}

// contentSpanWidth returns the total main-axis span of all children in a layout box.
func contentSpanWidth(box *LayoutBox) float64 {
	minLeft := math.MaxFloat64
	maxRight := 0.0
	found := false
	var scan func(b *LayoutBox)
	scan = func(b *LayoutBox) {
		for _, c := range b.Children {
			if c.IsAbsolutelyPositioned() {
				continue
			}
			if c.Rect.Width > 0 || c.Rect.Height > 0 {
				left := c.Rect.X
				right := c.Rect.X + c.Rect.Width
				if left < minLeft {
					minLeft = left
				}
				if right > maxRight {
					maxRight = right
				}
				found = true
			}
			for _, seg := range c.TextSegments {
				left := c.Rect.X + seg.X
				right := left + seg.Width
				if left < minLeft {
					minLeft = left
				}
				if right > maxRight {
					maxRight = right
				}
				found = true
			}
			scan(c)
		}
	}
	scan(box)
	if !found {
		return maxContentWidth(box)
	}
	if minLeft < 0 {
		minLeft = 0
	}
	cw := maxRight - minLeft
	if cw < 0 {
		cw = 0
	}
	return cw
}

// stateWidth returns the viewport width from the layout state, defaulting to 1280.
func stateWidth(state *LayoutState) float64 {
	if state != nil && state.ViewportWidth > 0 {
		return state.ViewportWidth
	}
	return 1280
}

// stateHeight returns the viewport height from the layout state, defaulting to 800.
func stateHeight(state *LayoutState) float64 {
	if state != nil && state.ViewportHeight > 0 {
		return state.ViewportHeight
	}
	return 800
}

// needsContentRelayout reports whether a flex/grid item's content needs to be
// re-laid-out after the item receives its final cross-axis size from the parent
// flex/grid layout. Only applies to non-leaf flex/grid containers that have
// children whose layout depends on the container's cross-axis size.
//
// NOTE: This returns false for leaf flex containers (no children). Re-laying-out
// a leaf flex container would just reset its auto-sized cross-axis to 0,
// destroying the stretch result from the parent's setItemPosition.
func needsContentRelayout(box *LayoutBox) bool {
	if box.Style == nil {
		return false
	}
	if len(box.Children) == 0 {
		return false
	}
	// Flex/grid containers always need re-layout after stretch.
	disp := box.Style.Display
	if disp == style.DisplayFlex || disp == style.DisplayInlineFlex ||
		disp == style.DisplayGrid || disp == style.DisplayInlineGrid {
		return true
	}
	// Block containers with flex/grid children also need re-layout.
	// Without this, display:block containers (e.g. sidebar-content) that
	// contain flex/grid children (e.g. file-explorer) are never re-laid-out
	// after the flex layout sets their cross-size, so their children keep
	// the measurement width (viewport width) instead of the constrained width.
	for _, c := range box.Children {
		if c.Style != nil {
			cd := c.Style.Display
			if cd == style.DisplayFlex || cd == style.DisplayInlineFlex ||
				cd == style.DisplayGrid || cd == style.DisplayInlineGrid {
				return true
			}
		}
	}
	return false
}
