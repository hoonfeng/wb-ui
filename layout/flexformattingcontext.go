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

package layout

import (
	"fmt"
	"math"
	"os"
	"sort"
	"wb-ui/style"
)

// FlexFormattingContext is the Go translation of WebCore::Layout::FlexFormattingContext
// / RenderFlexibleBox. It lays out a flex container's items along the main and cross
// axes per CSS Flexible Box Layout Module Level 1.
type FlexFormattingContext struct{}

// Layout lays out box's flex items. The caller sets box's border-box position and
// width; Layout computes the items' main/cross sizes and positions and box's auto
// height.
func (c *FlexFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	contentWidth := box.Rect.ContentWidth()
	contentHeight := box.Rect.ContentHeight()

	isRow := effectiveIsRow(box)
	isReverse := effectiveIsReverse(box)
	wrap := flexWrapOf(box)

	// Collect absolutely positioned children (deferred until after in-flow
	// items are laid out, matching BFC behaviour).
	var deferredAbsolutes []*LayoutBox
	for _, child := range box.Children {
		if child.IsAbsolutelyPositioned() {
			deferredAbsolutes = append(deferredAbsolutes, child)
		}
	}

	// Collect visible in-flow flex items, sorted by order.
	items := collectFlexItems(box)
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })

	// Resolve each item's hypothetical main size (flex-basis or content) and box model.
	for i := range items {
		it := &items[i]
		margin, padding, border := computeBoxModel(it.box, contentWidth, fontSizeOf(it.box))
		it.margin = margin
		it.padding = padding
		it.border = border
		// Write the box model back onto the item's rect so that the child
		// formatting context (BFC) sees non-zero padding/border when computing
		// the item's auto height. BFC.Layout only resolves the box model for
		// the root box (parent==nil); for flex items the parent (flex container)
		// is responsible for setting it, mirroring the BFC child loop.
		it.box.Rect.Margin = margin
		it.box.Rect.Padding = padding
		it.box.Rect.Border = border
		it.mainMargin = mainAxisMargin(margin, isRow)
		it.crossMargin = crossAxisMargin(margin, isRow)
		it.mainBorderPadding = mainAxisBorderPadding(border, padding, isRow)
		it.crossBorderPadding = crossAxisBorderPadding(border, padding, isRow)
		basis := resolveFlexBasis(it.box, contentWidth, contentHeight, isRow)
		// When flex-basis and the main-axis size are both auto, the used flex
		// base size is the content size (max-content). Resolve it by laying out
		// the item provisionally at the available main size and measuring the
		// rightmost content edge. Without this, auto-basis items collapse to a
		// zero main size (plus border/padding), which is wrong for e.g. buttons
		// whose width should follow their text content.
		if flexBasisIsContent(it.box, isRow) {
			measW := mainSizeAvailable(isRow, contentWidth, contentHeight)
			if !isRow {
				// Column-direction: use the container's content height (main-axis)
				// for measurement. The function already sets box.Rect.Width = viewport
				// (cross-axis) so text flows correctly. Using contentWidth (cross-size)
				// here would overflow the child's height, inflating the measurement.
				// If the container's height is auto (0), clamp to the cross-size so
				// the child's content measurement has a reasonable bound.
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
	var mainSize, crossSize float64
	if isRow {
		mainSize, crossSize = contentWidth, contentHeight
	} else {
		mainSize, crossSize = contentHeight, contentWidth
	}
	// If the container's main-axis size is auto or unresolvable (0 due to
	// circular dependency, e.g. column flex child of a row flex whose cross
	// size hasn't been set yet), use a large sentinel so items are not shrunk
	// to zero. The actual container size is determined from content at the
	// end of Layout (see heightIsAuto block below).
	// Only fire when contentHeight/contentWidth is truly 0/unavailable;
	// if a temporary cross size was set by the parent (e.g. via the
	// cross-size pre-set in the outer flex loop), use that known value.
	// If the container's main-axis size is auto or unresolvable (0 due to
	// circular dependency, e.g. column flex child of a row flex whose cross
	// size hasn't been set yet), use the viewport height so items are not shrunk
	// to zero or inflated to infinity. The actual container size is determined
	// from content at the end of Layout (see heightIsAuto block below).
	// Only fire when contentHeight/contentWidth is truly 0/unavailable;
	// if a temporary cross size was set by the parent (e.g. via the
	// cross-size pre-set in the outer flex loop), use that known value.
	if !isRow && contentHeight <= 0 {
		if state != nil && state.ViewportHeight > 0 {
			mainSize = state.ViewportHeight
		} else if containerHeight := box.Rect.ContentHeight(); containerHeight > 0 {
			mainSize = containerHeight
		} else if cbW := box.Rect.ContentWidth(); cbW > 0 {
			mainSize = cbW
		} else {
			mainSize = 800
		}
	}
	if isRow && contentWidth <= 0 {
		if state != nil && state.ViewportWidth > 0 {
			mainSize = state.ViewportWidth
		} else if containerWidth := box.Rect.ContentWidth(); containerWidth > 0 {
			mainSize = containerWidth
		} else if cbH := box.Rect.ContentHeight(); cbH > 0 {
			mainSize = cbH
		} else {
			mainSize = 1280
		}
	}

	// Line breaking (flex-wrap): pack items into lines by hypothetical main size.
	lines := breakFlexLines(items, mainSize, wrap)

	// Resolve flex base sizes and distribute free space per line.
	for li := range lines {
		ln := &lines[li]
		freezeLine := ln.items
		// Determine used flex base size: start from hypothetical.
		for i := range freezeLine {
			freezeLine[i].mainSize = freezeLine[i].hypotheticalMain
		}
		// Grow if line has positive free space; shrink if negative.
		for pass := 0; pass < 2; pass++ {
			lineMain := 0.0
			for i := range freezeLine {
				lineMain += freezeLine[i].mainSize
			}
			free := mainSize - lineMain - lineMainMargins(freezeLine)
			// When the container's main size is indefinite (auto / sentinel),
			// do not distribute positive free space via flex-grow. Growing into
			// viewport-sized space would balloon flex-grow items.
			if free > 0 && mainSize >= 800 {
				free = 0
			}
			if math.Abs(free) < 1e-6 {
				break
			}
			if free > 0 {
				// Distribute via flex-grow.
				totalGrow := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalGrow += freezeLine[i].grow
					}
				}
				if totalGrow <= 0 {
					break
				}
				any := false
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].grow <= 0 {
						continue
					}
					share := free * freezeLine[i].grow / totalGrow
					freezeLine[i].mainSize += share
					any = true
				}
				if !any {
					break
				}
			} else {
				// Distribute negative space via flex-shrink (weighted).
				totalShrink := 0.0
				for i := range freezeLine {
					if !freezeLine[i].frozen {
						totalShrink += freezeLine[i].shrink * freezeLine[i].hypotheticalMain
					}
				}
				if totalShrink <= 0 {
					break
				}
				any := false
				for i := range freezeLine {
					if freezeLine[i].frozen || freezeLine[i].shrink <= 0 {
						continue
					}
					weight := freezeLine[i].shrink * freezeLine[i].hypotheticalMain / totalShrink
					share := free * weight
					freezeLine[i].mainSize += share
					any = true
				}
				if !any {
					break
				}
			}
			// Freeze items at min/max.
			for i := range freezeLine {
				if freezeLine[i].frozen {
					continue
				}
				min, max := flexMinMax(freezeLine[i], isRow)
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
		// Resolve cross size: lay out each item to get its content, then stretch if
		// align-items is stretch.
		for i := range freezeLine {
			it := &freezeLine[i]
			setItemBorderBox(it, isRow)
			// 在布局子元素前，设置一个临时的交叉轴尺寸（容器在该轴上的内容尺寸），
			// 使内部 flex/block 布局有正确的可参考高度/宽度。
			// 仅对 block-level 子元素预设（flex/grid/block 容器需要），
			// 文本和内联元素的高度应由内容决定，不应膨胀到容器高度。
			if isRow {
				if it.box.Type != BoxTextRun && !it.box.IsInline() {
					it.box.Rect.Height = contentHeight
				}
			} else {
				if it.box.Type != BoxTextRun && !it.box.IsInline() {
					it.box.Rect.Width = contentWidth
				}
			}
			it.box.Rect.X = 0
			it.box.Rect.Y = 0
			childCtx := contextFor(it.box)
			childCtx.Layout(it.box, state)
			// Restore main-axis size from flex-grow/shrink resolution. The
			// child's own Layout may have recomputed its height/width via
			// heightIsAuto, overwriting the resolved flex-grow result.
			setItemBorderBox(it, isRow)
			// Use content-based cross-size measurement for text/inline items
			// instead of the rect dimension which may hold the pre-set value.
			// Also covers BoxBlock with display:inline (e.g., inline wrappers).
			if it.box.Type == BoxTextRun || it.box.IsInline() || it.box.IsInlineLevel() {
				if isRow {
					it.crossSize = maxContentBottom(it.box) - it.box.Rect.ContentY()
				} else {
					it.crossSize = maxContentWidth(it.box)
				}
			} else {
				it.crossSize = crossAxisSize(it.box, isRow)
			}
		}
		// Cross size of the line is the max cross size of items.
		ln.crossSize = 0
		for i := range freezeLine {
			c := freezeLine[i].crossSize + freezeLine[i].crossMargin
			if c > ln.crossSize {
				ln.crossSize = c
			}
		}
	}

	// Align-content: distribute lines along the cross axis.
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
		// Stretch each line equally.
		extra := crossSize - totalCross
		if extra > 0 {
			per := extra / float64(len(lines))
			for i := range lines {
				lines[i].crossSize += per
			}
		}
	}

	// Position items within each line along the main and cross axes.
	cursorCross := crossStart
	for li := range lines {
		ln := &lines[li]
		// Main-axis position (justify-content).
		lineMainUsed := 0.0
		for i := range ln.items {
			lineMainUsed += ln.items[i].mainSize + ln.items[i].mainMargin
		}
		mainFree := mainSize - lineMainUsed
		if mainFree < 0 {
			mainFree = 0
		}
		// When the container's main size is the sentinel 1e6 (auto/unresolvable),
		// do not distribute phantom free space for alignment — it would produce
		// enormous offsets (e.g. center = 500000+).
		if mainSize >= 1e5 {
			mainFree = 0
		}
		just := justifyContentOf(box)
		mainCursor, mainGap := justifyStart(just, mainFree, len(ln.items))
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
			// Reverse main-axis order for row-reverse / column-reverse.
			mainCursor = mainSize - mainCursor
		}
		for i := range ln.items {
			it := &ln.items[i]
			if isReverse {
				mainCursor -= it.mainMargin/2 + it.mainSize
				setItemPosition(it, mainCursor, cursorCross+ln.crossSize, isRow, box)
				// Children were laid out at (0,0); shift them to the final position.
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor -= it.mainMargin/2 + mainGap
			} else {
				mainCursor += it.mainMargin / 2
				setItemPosition(it, mainCursor, cursorCross, isRow, box)
				// Children were laid out at (0,0); shift them to the final position.
				offsetItemSubtree(it.box, it.box.Rect.X, it.box.Rect.Y)
				mainCursor += it.mainSize + it.mainMargin/2 + mainGap
			}
		}
		cursorCross += ln.crossSize + lineGap
	}

	// Auto height of the container: the cross size is the line stack height.
	if heightIsAuto(box) {
		if isRow {
			// cursorCross accumulates line cross-sizes (content heights), so add
			// the container's vertical padding/border to get the border-box height.
			box.Rect.Height = cursorCross + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
				box.Rect.Border.Top + box.Rect.Border.Bottom
		} else {
			// column direction: main axis is vertical; height encloses all lines' main.
			// totalLineMain sums items' border-box sizes + margins, so add the
			// container's vertical padding/border.
			box.Rect.Height = totalLineMain(lines) + box.Rect.Padding.Top + box.Rect.Padding.Bottom +
				box.Rect.Border.Top + box.Rect.Border.Bottom
		}
	}

	// Re-lay-out flex/grid container items so their children are calculated
	// at the final cross-axis size (set by setItemPosition stretch) instead of
	// the provisional measurement width. The item stays at its absolute position
	// (set by setItemPosition + offsetItemSubtree above), so the inner layout
	// positions children correctly without double-shifting.
	//
	// For row containers, compute flex-grow distribution here:
	// items with flex-grow > 0 share the remaining space proportionally.
	if isRow {
		totalGrow := 0.0
		fixedWidth := 0.0
		for j := range items {
			if !items[j].box.IsVisible() || !items[j].box.IsInFlow() {
				continue
			}
			if items[j].grow > 0 {
				totalGrow += items[j].grow
			} else {
				// Fixed item: use its current width.
				fixedWidth += items[j].box.Rect.Width + items[j].mainMargin
			}
		}
		if totalGrow > 0 {
			free := box.Rect.ContentWidth() - fixedWidth
			if free < 0 {
				free = 0
			}
			for j := range items {
				if items[j].grow > 0 {
					share := free * items[j].grow / totalGrow
					if share < 0 {
						share = 0
					}
					items[j].box.Rect.Width = share
				}
			}
		}
	}
	for i := range items {
		it := &items[i]
		if needsContentRelayout(it.box) {
			if !isRow {
				it.box.Rect.Width = box.Rect.ContentWidth()
			} else {
				it.box.Rect.Height = box.Rect.ContentHeight()
			}
			ctx := contextFor(it.box)
			ctx.Layout(it.box, state)
		}
	}

	// Lay out absolutely-positioned descendants (same as BFC).
	root := stateRoot(box)
	for _, child := range deferredAbsolutes {
		cb := containingBlockForAbsolute(child, root)
		layoutAbsolute(child, cb, root, state)
	}
	// Debug: find rp-body and log its width right after layout
	var scanLB func(b *LayoutBox, depth int)
	scanLB = func(b *LayoutBox, depth int) {
		if b.Element != nil {
			if cls := b.Element.GetAttribute("class"); cls == "rp-body" || cls == "file-explorer" || cls == "rp-header" || cls == "content" || cls == "chat-area" {
				fmt.Fprintf(os.Stderr, "[FLEX_END] cls=%s x=%.0f y=%.0f w=%.0f h=%.0f\n",
					cls, b.Rect.X, b.Rect.Y, b.Rect.Width, b.Rect.Height)
			} else if b.Element != nil && b.Element.GetAttribute("id") == "app" {
				fmt.Fprintf(os.Stderr, "[FLEX_END] cls=#app x=%.0f y=%.0f w=%.0f h=%.0f\n",
					b.Rect.X, b.Rect.Y, b.Rect.Width, b.Rect.Height)
			}
		}
		// Deep scan: for nodes at depth 4+, dump ALL classes/ids with their rect
		if depth >= 4 {
			name := "(anon)"
			if b.Element != nil {
				if cls := b.Element.GetAttribute("class"); cls != "" {
					name = cls
				} else if id := b.Element.GetAttribute("id"); id != "" {
					name = "#" + id
				} else {
					name = b.Element.LocalName()
				}
			}
			fmt.Fprintf(os.Stderr, "[FLEX_END:d%d] %s x=%.0f y=%.0f w=%.0f h=%.0f\n",
				depth, name, b.Rect.X, b.Rect.Y, b.Rect.Width, b.Rect.Height)
		}
		for _, c := range b.Children {
			scanLB(c, depth+1)
		}
	}
	scanLB(box, 0)
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
		// In vertical writing mode, the inline axis (row direction) is vertical.
		// So isRow effectively means the main axis is vertical, which corresponds
		// to the physical height direction. We treat this as not-a-row in the
		// traditional horizontal sense.
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
	// honour explicit flex-basis when definite.
	r := resolveLengthAuto(box.Style.FlexBasis, ternary(isRow, cbW, cbH), fs)
	if !r.Auto && r.Definite {
		return r.Value
	}
	// else use width/height depending on direction.
	if isRow {
		w, ok := definiteWidth(box.Style.Width, cbW, fs)
		if ok {
			// For border-box sizing, the declared width includes padding+border.
			// Flex-basis is always the content-box size, so subtract them.
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
	// else approximate content main size as 0 (will grow).
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
	// Log for chat-area debug
	if it.box.Element != nil {
		if cls := it.box.Element.GetAttribute("class"); cls == "chat-area" {
			cw := container.Rect.ContentWidth()
			ch := container.Rect.ContentHeight()
			fmt.Fprintf(os.Stderr, "[SETPOS] cls=%s align=%s cw=%.0f ch=%.0f crossSize=%.0fi crossMargin=%.0f cw=%.0f padH=%.0f borderH=%.0f\n",
				cls, align, cw, ch, it.crossSize, it.crossMargin, container.Rect.Width,
				container.Rect.Padding.Horizontal(), container.Rect.Border.Horizontal())
		}
	}
	// Cross size: stretch to container cross size when align is stretch.
	crossSize := it.crossSize
	if align == "stretch" {
		// Only stretch when the container has a definite cross-axis size.
		// When the container's cross size is auto (0), stretching the
		// child to 0 would collapse its content-based size.
		var containerCross float64
		if isRow {
			// Row: cross axis is height.
			containerCross = container.Rect.Height - container.Rect.Border.Vertical() - container.Rect.Padding.Vertical()
		} else {
			// Column: cross axis is width.
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
		// Log AFTER setting the width
		if it.box.Element != nil {
			if cls := it.box.Element.GetAttribute("class"); cls == "chat-area" {
				fmt.Fprintf(os.Stderr, "[AFTER_SETPOS] cls=%s containerCross=%.0f crossSize=%.0f itWidth=%.0f itHeight=%.0f\n",
					cls, containerCross, crossSize, it.box.Rect.Width, it.box.Rect.Height)
			}
		}
	}
	// Cross alignment.
	crossPos := crossOffset
	switch align {
	case "center":
		crossPos = crossOffset + (crossAxisContentSize(container, isRow)-it.crossSize-it.crossMargin)/2
	case "flex-end":
		crossPos = crossOffset + crossAxisContentSize(container, isRow) - it.crossSize - it.crossMargin
	}
	// Clamp to prevent negative cross-axis position when item is larger than
	// the container's cross axis content size (center alignment overflow).
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
func flexMinMax(it flexItem, isRow bool) (float64, float64) {
	fs := fontSizeOf(it.box)
	st := it.box.Style
	minV, maxV, minAuto, maxAuto := resolveMinMax(st.MinWidth, st.MaxWidth, 0, fs)
	if !isRow {
		minV, maxV, minAuto, maxAuto = resolveMinMax(st.MinHeight, st.MaxHeight, 0, fs)
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

func justifyStart(just string, free float64, n int) (float64, float64) {
	return 0, 0
}

func totalLineMain(lines []flexLine) float64 {
	s := 0.0
	for _, ln := range lines {
		for _, it := range ln.items {
			s += it.mainSize + it.mainMargin
		}
	}
	return s
}

// offsetSubtree shifts a layout box and all its descendants (positions and text
// segments) by (dx, dy). Used by the flex layout to fix up child geometry after
// repositioning items that were laid out at the origin (0,0) during the
// cross-size measurement pass: the item itself is positioned by setItemPosition,
// but its descendants kept their origin-relative coordinates.
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

// offsetItemSubtree offsets the descendants (and the item's own text segments)
// of a flex item by (dx, dy) without moving the item itself, which was already
// positioned by setItemPosition. The item's content was laid out relative to
// the origin during the measurement pass, so every descendant coordinate and
// text segment must be shifted by the item's final (X, Y).
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
// the item's content (max-content). This is the case when both flex-basis and
// the main-axis size property (width for row, height for column) are auto, per
// CSS flexbox §9.2. An item with flex-basis: auto uses the width/height; when
// that is also auto the used flex base size is the content size.
func flexBasisIsContent(box *LayoutBox, isRow bool) bool {
	if box.Style == nil {
		return false
	}
	fs := fontSizeOf(box)
	// flex-basis auto?
	bb := resolveLengthAuto(box.Style.FlexBasis, 0, fs)
	if !bb.Auto {
		return false
	}
	// main-axis size (width/height) auto?
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
// The item's rect is saved and restored so the measurement pass does not
// affect the subsequent final layout pass. Returns the content-box main size.
func measureFlexItemContentMain(box *LayoutBox, availableMain float64, isRow bool, state *LayoutState) float64 {
	savedX, savedY := box.Rect.X, box.Rect.Y
	defer func() { box.Rect.X, box.Rect.Y = savedX, savedY }()
	if isRow {
		box.Rect.Width = availableMain
		box.Rect.X = 0
		box.Rect.Y = 0
	} else {
		// Column container: main axis is Y (height). Set height to availableMain
		// as an approximation of the main-axis size, and give a generous width
		// (cross-axis) so text content flows naturally. The exact cross-axis
		// width will be set by setItemPosition stretch in the main layout pass.
		h := availableMain
		if h <= 0 || h >= 1e5 {
			// Use viewport height from state when availableMain is the sentinel.
			// This prevents nested column flex containers from measuring at
			// the sentinel (1e6) and inflating their content height.
			if state != nil && state.ViewportHeight > 0 {
				h = state.ViewportHeight
			} else {
				h = 1280
			}
		}
		box.Rect.Height = h
		box.Rect.Width = stateWidth(state)
		box.Rect.X = 0
		box.Rect.Y = 0
	}
	ctx := contextFor(box)
	ctx.Layout(box, state)
	if isRow {
		// Measure by line width so text-align (center/right) does not inflate
		// the max-content size of the item.
		return contentSpanWidth(box)
	}
	// column: measure content height (max bottom edge).
	contentTop := box.Rect.ContentY()
	h := maxContentBottom(box) - contentTop
	if h < 0 {
		h = 0
	}
	// Clamp measured height to viewport height to prevent cascading
	// height inflation in nested column flex containers. During the final
	// layout pass the container's actual height will be constrained correctly.
	clamp := stateHeight(state)
	if h > clamp {
		h = clamp
	}
	return h
}

// contentSpanWidth returns the total main-axis span of all children in a layout box,
// measured as the maximum child right-edge minus minimum child left-edge. This is used
// by flex item measurement to correctly report the intrinsic width of a flex container
// child (e.g. a wrapper holding multiple icon buttons), whose children are block-level
// so maxContentWidth would not include their Rect.Width.
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
			// Use child's border-box span: x + width.
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
			// Recurse into children to pick up text segments.
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
// re-laid-out after the item receives its final position and cross-axis size
// from the parent flex/grid layout. This is needed because
// measureFlexItemContentMain lays out children at a provisional width; after
// the outer layout determines the final width (via stretch), the children must
// be re-laid-out at the correct size. Only applies to non-leaf containers
// (flex/grid) whose cross-axis size was stretched to a different value.
func needsContentRelayout(box *LayoutBox) bool {
	if box.Style == nil {
		return false
	}
	disp := box.Style.Display
	return disp == style.DisplayFlex || disp == style.DisplayInlineFlex
}
