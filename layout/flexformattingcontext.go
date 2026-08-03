// Flex formatting context — lays out children in a flex container.
// Translation of: Source/WebCore/layout/formattingContexts/flex/FlexFormattingContext.cpp

package layout

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"wb-ui/style"
)

var wbFlexDebug = os.Getenv("WB_FLEX_DEBUG") != ""

func flexName(box *ElementBox) string {
	if box == nil { return "<nil>" }
	if el := box.Element(); el != nil {
		if cls := el.GetAttribute("class"); cls != "" {
			return el.LocalName() + "." + cls
		}
		return el.LocalName()
	}
	return "<anon>"
}

type FlexFormattingContext struct {
	FormattingContextBase
}

type flexItem struct {
	box             *ElementBox
	flexGrow        float64
	flexShrink      float64
	flexBasis       float64
	basisExplicit   bool // flex-basis explicitly set (e.g. 0% from flex:1) — must NOT fall back to content size
	minWidth        float64
	maxWidth        float64
	minHeight       float64
	maxHeight       float64
	baseSize        float64
	hypothetical    float64
	frozen          bool
	targetSize      float64
	finalMainSize   float64
	marginMain      float64
	marginCross     float64
	paddingMain     float64 // main-axis padding+border (border-box items)
	order           int
	baselineOffset  float64
}

func (c *FlexFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	isRow := cs.FlexDirection != "column" && cs.FlexDirection != "column-reverse"
	isReverse := cs.FlexDirection == "row-reverse" || cs.FlexDirection == "column-reverse"

	g := state.GeometryForBox(box)
	_, padding, border := computeBoxModel(box, g.ContentWidth(), fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
	// g.ContentWidth() is already the content area width (set by parent).
	// Do NOT subtract padding/border here - the content area is what children
	// need. ContentBoxLeft() already accounts for the padding offset.
	cw := g.ContentWidth()
	ch := g.ContentHeight()
	initialContentHeight := ch

	var items []*flexItem
	var deferredAbsolutes []*ElementBox
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok && child.IsVisible() {
			if child.IsAbsolutelyPositioned() {
				// Out-of-flow children of a flex container are NOT flex items;
				// they are positioned against the flex container as containing
				// block (CSS-FLEXBOX §5.1). Must be laid out like block-level
				// absolutes — previously they were silently dropped, leaving
				// e.g. .cache-ring-label (absolute, inside a flex wrap) at 0x0.
				deferredAbsolutes = append(deferredAbsolutes, childEb)
				continue
			}
			if child.IsInFlow() {
				item := c.resolveItem(childEb, isRow, cw, ch, state)
				items = append(items, item)
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })
	if len(items) == 0 && len(deferredAbsolutes) == 0 {
		return
	}

	mainSize := cw
	if !isRow {
		mainSize = ch
	}

	for _, it := range items {
		it.baseSize = it.resolveBaseSize(mainSize, isRow)
		it.hypothetical = it.baseSize
		it.targetSize = it.baseSize
		if wbFlexDebug && (flexName(box) == "div.titlebar" || flexName(box) == "div.chat-area") {
			fmt.Fprintf(os.Stderr, "[flex/item] titlebar child %s: grow=%.1f shrink=%.1f basis=%.1f base=%.1f padMain=%.1f marginMain=%.1f\n",
				flexName(it.box), it.flexGrow, it.flexShrink, it.flexBasis, it.baseSize, it.paddingMain, it.marginMain)
		}
	}

	wrapMode := cs.FlexWrap == "wrap" || cs.FlexWrap == "wrap-reverse"
	if wrapMode && len(items) > 0 {
		// flex-wrap: wrap / wrap-reverse：按行分组布局（CSS-FLEXBOX §8）。
		c.layoutWrapped(items, box, isRow, isReverse, cw, ch, state)
	} else {
		c.distributeFreeSpace(items, mainSize, isRow)

	// Before resolving cross sizes and positions, compute a preliminary
	// container content height from children so that cross-axis centering
	// and flex-end alignment work correctly when the container has auto-height.
	// Without this, ch=0 causes negative offsets (children float above parent).
	if heightIsAutoForBox(box) {
		if isRow {
			// Row flex: estimate auto height from the tallest child,
			// including each child's actual vertical padding and border.
			// Use the child's intrinsic content height (an svg/icon child is
			// 14px, a text child is its line box) instead of fontLineGap —
			// fontLineGap(13px font)≈15.6 inflates a 20px toolbar button to
			// 21.8px and balloons .explorer-toolbar 29→32.6 (Edge reference).
			estH := 0.0
			for _, it := range items {
				childH := 0.0
				// Explicit height on the child wins (a 1px ::before divider
				// line must contribute 1px, not fontLineGap 17.2).
				if csc := it.box.Style(); csc != nil {
					if hv, ok := definiteHeight(csc.Height, 0, fontSizeOf(it.box)); ok && hv > 0 {
						childH = hv
					}
				}
				if childH <= 0 {
					childH = intrinsicContentHeight(it.box)
				}
				cg := state.GeometryForBox(it.box)
				if childH <= 0 {
					// No intrinsic content: fall back to the line box plus the
					// child's own padding+border (intrinsicContentHeight already
					// includes padding+border when it has content, so we must
					// NOT add them again here — that double-counted .tb-btn
					// 14px+6 → 20 then +6 → 26px).
					childH = fontLineGap(it.box)
					childH += cg.PaddingTop() + cg.PaddingBottom() + cg.BorderTop() + cg.BorderBottom()
				}
				if wbFlexDebug {
					fmt.Fprintf(os.Stderr, "[flex/est] %s childH=%.1f (intr=%.1f)\n",
						flexName(it.box), childH, intrinsicContentHeight(it.box))
				}
				if childH > estH {
					estH = childH
				}
			}
			if wbFlexDebug {
				fmt.Fprintf(os.Stderr, "[flex/est] %s estH=%.1f ch=%.1f\n", flexName(box), estH, ch)
			}
			if box.Parent() != nil && initialContentHeight > 0 {
				// Parent set height (grid row stretch / explicit height /
				// nested flex sizing) pins the container — do NOT inflate it
				// to the tallest child. Without this, a row-flex grid item
				// (e.g. .right-container holding the chat panel) ballooned to
				// its 2397px content height instead of staying in its 748px
				// grid row, pushing the conversation list and chat input off
				// the viewport. Mirrors the column-flex branch below.
			} else if estH > ch {
				g.SetContentHeight(estH)
				ch = estH
			}
		} else {
			// Column flex: estimate auto height from the sum of child main-sizes,
			// including each child's actual start/end padding and border.
			estH := 0.0
			for _, it := range items {
				childH := it.finalMainSize + it.marginMain
				cg := state.GeometryForBox(it.box)
				childH += cg.PaddingTop() + cg.PaddingBottom() + cg.BorderTop() + cg.BorderBottom()
				estH += childH
			}
			if box.Parent() != nil && initialContentHeight > 0 {
				// Parent set height - don't inflate
			} else if estH > ch {
				g.SetContentHeight(estH)
				ch = estH
			}
		}
	}

		crossStart := g.ContentBoxTop()
		if !isRow {
			crossStart = g.ContentBoxLeft()
		}
		c.resolveCrossSizes(items, isRow, isReverse, false, cw, ch, state, ch)
		c.applyPositions(items, box, isRow, isReverse, false, state, crossStart)
	}

	// Lay out absolute-positioned children against this flex container as
	// their containing block (CSS-FLEXBOX §5.1). Deferred to after items so
	// the container's content box is final.
	if len(deferredAbsolutes) > 0 {
		root := stateRootForBox(box)
		for _, ab := range deferredAbsolutes {
			cb := containingBlockForAbsolute(ab, root)
			layoutAbsolute(ab, cb, root, state)
		}
	}

	// Compute auto container height from children.
	// Preserve any height already set by parent formatting context (e.g. grid row height).
	if heightIsAutoForBox(box) {
		contentTop := g.ContentBoxTop()
		maxChildBottom := contentTop
		for _, it := range items {
			cg := state.GeometryForBox(it.box)
			if bottom := cg.Top() + cg.BorderBoxHeight(); bottom > maxChildBottom {
				maxChildBottom = bottom
			}
		}
		blockSize := maxChildBottom - contentTop
		if blockSize < 0 { blockSize = 0 }
		// If parent set a larger height (e.g. grid row), keep it.
		// If parent set a SMALLER height, also keep it (don't overflow).
		if box.Parent() != nil {
			// A parent-set height (grid row stretch / explicit height) pins
			// the container; overflowing content must clip, not inflate it.
			// Without this, a flex grid-item ballooned to its content height
			// (e.g. right-panel 1068px) instead of staying in its 748px row.
			if initialContentHeight <= 0 && blockSize > initialContentHeight {
				g.SetContentHeight(blockSize)
			}
		} else {
			g.SetContentHeight(blockSize)
		}
	}
}

// layoutWrapped 处理 flex-wrap:wrap / wrap-reverse：将 items 按主轴空间
// 分组为多行（CSS-FLEXBOX §9.1：行基于 flex base size 确定，超宽换行），
// 每行独立分配主轴空间（distributeFreeSpace）并定位；跨轴方向行间递增。
// 行内 justify-content / align-items 与单行共用 applyPositions（传行起点）。
func (c *FlexFormattingContext) layoutWrapped(items []*flexItem, container *ElementBox, isRow, isReverse bool, cw, ch float64, state *LayoutState) {
	cs := container.Style()
	g := state.GeometryForBox(container)
	mainSize := cw
	if !isRow {
		mainSize = ch
	}
	gapMain := flexGap(cs, isRow, fontSizeOf(container))
	gapCross := flexGap(cs, !isRow, fontSizeOf(container))
	wrapReverse := cs.FlexWrap == "wrap-reverse"

	// 1. 行分组：累计 base+margin+gap，超 mainSize 开新行。
	//    mainSize<=0（auto 容器）时无法判断换行，全部放一行。
	var lines [][]*flexItem
	var cur []*flexItem
	used := 0.0
	first := true
	for _, it := range items {
		it.targetSize = it.baseSize
		it.hypothetical = it.baseSize
		need := it.baseSize + it.marginMain
		if !first {
			need += gapMain
		}
		if !first && mainSize > 0 && used+need > mainSize {
			lines = append(lines, cur)
			cur = nil
			used = 0
			first = true
			need = it.baseSize + it.marginMain
		}
		cur = append(cur, it)
		used += need
		first = false
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		return
	}
	if wrapReverse {
		// wrap-reverse：第一行在跨轴末端，行序反转。
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}

	// 2. 每行：分配主轴空间 + 计算行跨轴尺寸（stretch 用行高）。
	lineHeights := make([]float64, len(lines))
	for li, line := range lines {
		c.distributeFreeSpace(line, mainSize, isRow)
		maxH := 0.0
		for _, it := range line {
			if h := itemCrossSize(it, isRow, state); h > maxH {
				maxH = h
			}
		}
		if maxH <= 0 {
			maxH = fontLineGap(container)
		}
		lineHeights[li] = maxH
		c.resolveCrossSizes(line, isRow, isReverse, false, cw, ch, state, maxH)
	}

	// 3. 定位：跨轴起点从容器内容起点开始，行间 += 行高 + gap。
	crossStart := g.ContentBoxTop()
	if !isRow {
		crossStart = g.ContentBoxLeft()
	}
	for li, line := range lines {
		if li > 0 {
			crossStart += lineHeights[li-1] + gapCross
		}
		c.applyPositions(line, container, isRow, isReverse, false, state, crossStart)
	}
}

// itemCrossSize 返回 flex item 的跨轴尺寸（row 为高度、column 为宽度），
// 用于 wrap 时确定行高。显式尺寸优先，其次布局几何，最后字体行高兜底。
func itemCrossSize(it *flexItem, isRow bool, state *LayoutState) float64 {
	g := state.GeometryForBox(it.box)
	cs := it.box.Style()
	fs := fontSizeOf(it.box)
	if isRow {
		if cs != nil {
			if hv, ok := definiteHeight(cs.Height, 0, fs); ok && hv > 0 {
				return hv
			}
		}
		if h := g.BorderBoxHeight(); h > 0 {
			return h
		}
		return fontLineGap(it.box) + g.PaddingTop() + g.PaddingBottom() + g.BorderTop() + g.BorderBottom()
	}
	if cs != nil && !cs.Width.IsAuto() {
		if wv, ok := definiteWidth(cs.Width, 0, fs); ok && wv > 0 {
			return wv
		}
	}
	if w := g.BorderBoxWidth(); w > 0 {
		return w
	}
	return 0
}

func (c *FlexFormattingContext) resolveItem(box *ElementBox, isRow bool, cbWidth, cbHeight float64, state *LayoutState) *flexItem {
	cs := box.Style()
	g := state.GeometryForBox(box)
	margin, padding, border := computeBoxModel(box, cbWidth, fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	mm, mc := margin.Left+margin.Right, margin.Top+margin.Bottom
	if !isRow { mm, mc = margin.Top+margin.Bottom, margin.Left+margin.Right }
	if wbFlexDebug && strings.HasPrefix(flexName(box), "div.plan-container") {
		fmt.Fprintf(os.Stderr, "[flex/margin] plan-container: mT=%.1f mR=%.1f mB=%.1f mL=%.1f mm=%.1f\n",
			margin.Top, margin.Right, margin.Bottom, margin.Left, mm)
	}

	fs := fontSizeOf(box)
	flexGrow := cs.FlexGrow
	flexShrink := cs.FlexShrink

	var flexBasis float64
	basisExplicit := false
	fb := cs.FlexBasis
	if fb.Unit == "auto" || fb.Unit == "" {
		if isRow {
			r := resolveLengthAuto(cs.Width, cbWidth, fs)
			if !r.Auto && r.Definite { flexBasis = r.Value }
		} else {
			r := resolveLengthAuto(cs.Height, cbHeight, fs)
			if !r.Auto && r.Definite { flexBasis = r.Value }
		}
	} else {
		flexBasis = resolveOrZero(fb, cbWidth, fs)
		basisExplicit = true
	}

	minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbWidth, fs)
	minH, maxH, _, _ := resolveMinMax(cs.MinHeight, cs.MaxHeight, cbHeight, fs)

	// Main-axis padding+border for border-box items. When flex-basis is 0%
	// (flex:1), the resolved content size grown by free space must leave room
	// for this padding — otherwise a border-box item ends up
	// content+padding larger than the slot (project-section 605px in a 601px
	// slot, overflowing the sidebar and triggering a wrong scrollbar).
	paddingMain := 0.0
	if isBorderBox(box) {
		_, pb, bd := computeBoxModel(box, cbWidth, fs)
		if isRow {
			paddingMain = pb.Left + pb.Right + bd.Left + bd.Right
		} else {
			paddingMain = pb.Top + pb.Bottom + bd.Top + bd.Bottom
		}
	}

	return &flexItem{
		box: box, flexGrow: flexGrow, flexShrink: flexShrink,
		flexBasis: flexBasis, basisExplicit: basisExplicit, minWidth: minW, maxWidth: maxW,
		minHeight: minH, maxHeight: maxH,
		marginMain: mm, marginCross: mc, paddingMain: paddingMain, order: cs.Order,
	}
}

func (it *flexItem) resolveBaseSize(containerMainSize float64, isRow bool) float64 {
	base := it.flexBasis
	if base <= 0 && !it.basisExplicit {
		if isRow {
			// Row flex: main axis = width → use intrinsic content width.
			base = intrinsicContentWidth(it.box, isRow)
		} else {
			// Column flex: main axis = height → use intrinsic content height.
			base = intrinsicContentHeight(it.box)
		}
		if base <= 0 { base = 0 }
	}
	if isRow {
		return clampSize(base, it.minWidth, it.maxWidth, it.minWidth <= 0, it.maxWidth <= 0)
	}
	return clampSize(base, it.minHeight, it.maxHeight, it.minHeight <= 0, it.maxHeight <= 0)
}

// intrinsicContentWidth returns the max-content width of a box by inspecting
// its children without performing full layout. For ElementBox children it
// recurses; for InlineTextBox children it measures each word individually
// (matching InlineFormattingContext behavior) to avoid the "sum of parts
// exceeds whole" discrepancy that causes unwanted line wraps.
func intrinsicContentWidth(box *ElementBox, isRow bool) float64 {
	cs := box.Style()
	// An explicit CSS width is authoritative for the box's own contribution —
	// otherwise flex items with a fixed width (e.g. a 40px-wide activity-bar
	// button whose only child is an 18px icon) would shrink to their content
	// width, overflow the flex container, and misposition the icon.
	if cs != nil && !cs.Width.IsAuto() {
		if w, ok := definiteWidth(cs.Width, 0, fontSizeOf(box)); ok && w > 0 {
			if isBorderBox(box) {
				return w
			}
			_, p, b := computeBoxModel(box, w, fontSizeOf(box))
			return w + p.Left + p.Right + b.Left + b.Right
		}
	}
	// For row-direction flex containers, the max-content inline size is the SUM
	// of children (plus gap), matching CSS-FLEXBOX §9.9.2. For block/non-flex
	// containers it's the max of children.
	isFlexRow := cs != nil && box.EstablishesFlexFormattingContext() &&
		cs.FlexDirection != "column" && cs.FlexDirection != "column-reverse"

	total := 0.0
	maxW := 0.0
	spaceW := measureText(box, " ")
	if spaceW <= 0 {
		spaceW = measureText(box, " ")
	}
	for _, child := range box.Children() {
		if !child.IsInFlow() { continue }
		switch c := child.(type) {
			case *InlineTextBox:
				// Measure word-by-word (matching IFC behavior) so the flex item
				// width matches what InlineFormattingContext.Layout expects.
				w := measureTextWordSum(box, c.Text(), spaceW)
				if isFlexRow { total += w }
				if w > maxW { maxW = w }
		case *ElementBox:
			cw := intrinsicContentWidth(c, isRow)
			// Replaced / SVG elements (svg with width="N" attribute, img,
			// canvas, iframe) size from their attribute/intrinsic dimensions,
			// NOT their (usually empty) children — otherwise a 12px svg icon
			// contributes 0 and flex items with icons shrink below content.
			if el := c.Element(); el != nil {
				if cw <= 0 || el.LocalName() == "svg" || el.LocalName() == "img" {
					if w := el.GetAttribute("width"); w != "" {
						if f, err := strconv.ParseFloat(w, 64); err == nil && f > 0 {
							cw = f
						}
					}
				}
			}
			if isFlexRow { total += cw }
			if cw > maxW { maxW = cw }
		}
	}
	if isFlexRow {
		maxW = total
		// Add gap between flex items.
		if cs.Gap.Value > 0 || cs.ColumnGap.Value > 0 {
			gapV := cs.Gap.Value
			if gapV <= 0 { gapV = cs.ColumnGap.Value }
			unit := cs.Gap.Unit
			if unit == "" { unit = cs.ColumnGap.Unit }
			gap := gapV
			if unit == "em" { gap *= fontSizeOf(box) }
			if gap > 0 {
				count := 0
				for _, child := range box.Children() {
					if child.IsInFlow() { count++ }
				}
				maxW += gap * float64(count-1)
			}
		}
	}
	// Add the box's own padding + border (inline direction).
	if cs != nil {
		fs := fontSizeOf(box)
		_, p, b := computeBoxModel(box, maxW, fs)
		maxW += p.Left + p.Right + b.Left + b.Right
	}
	return maxW
}

// minContentWidth returns the min-content inline size: the width of the
// longest unbreakable segment (longest whitespace-delimited word for text;
// CJK characters are individually breakable so a CJK run contributes only
// one character's width). Used for fit-content sizing of flex cross axis.
func minContentWidth(box *ElementBox) float64 {
	if box == nil {
		return 0
	}
	if el := box.Element(); el != nil {
		ln := el.LocalName()
		if ln == "svg" || ln == "img" || ln == "canvas" {
			if w := el.GetAttribute("width"); w != "" {
				if f, err := strconv.ParseFloat(w, 64); err == nil && f > 0 {
					return f
				}
			}
		}
	}
	mw := 0.0
	wordW := func(text string) float64 {
		longest := 0.0
		for _, word := range strings.Fields(text) {
			// A CJK run is breakable per character: longest segment is the
			// widest CJK char (or ASCII word).
			runes := []rune(word)
			w := 0.0
			cjk := false
			for _, r := range runes {
				if r > 0x2E80 {
					cjk = true
					break
				}
			}
			if cjk {
				for _, r := range runes {
					cw := measureText(box, string(r))
					if cw > w {
						w = cw
					}
				}
			} else {
				w = measureText(box, word)
			}
			if w > longest {
				longest = w
			}
		}
		return longest
	}
	for _, child := range box.Children() {
		if !child.IsInFlow() {
			continue
		}
		switch c := child.(type) {
		case *InlineTextBox:
			if w := wordW(c.text); w > mw {
				mw = w
			}
		case *ElementBox:
			if w := minContentWidth(c); w > mw {
				mw = w
			}
		}
	}
	if mw <= 0 {
		// No text: an empty box contributes nothing, but a replaced element
		// with no attribute size still has a usable min-content of 0.
		mw = 0
	}
	return mw
}

func intrinsicContentHeight(box *ElementBox) float64 {
	cs := box.Style()
	// A replaced element (svg/img/canvas) sizes from its attribute height,
	// not its (empty) children — otherwise a 14px svg icon falls back to
	// fontLineGap (~17px) and inflates every button containing one
	// (.tb-btn 21.8px vs Edge 20px = 14 icon + 2×2 padding + 2×1 border).
	if el := box.Element(); el != nil {
		ln := el.LocalName()
		if ln == "svg" || ln == "img" || ln == "canvas" {
			if ht := el.GetAttribute("height"); ht != "" {
				if f, err := strconv.ParseFloat(ht, 64); err == nil && f > 0 {
					return f
				}
			}
		}
	}
	isColFlex := cs != nil && box.EstablishesFlexFormattingContext() &&
		(cs.FlexDirection == "column" || cs.FlexDirection == "column-reverse")
	// Any flex/grid container sizes itself by max-child on the cross axis —
	// summing their children's heights balloons a flex-row wrapper (e.g. the
	// chat input bar ballooned to 396px from a 150px textarea). Only true
	// block containers (non-flex/grid) sum stacked block children.
	isFlexOrGrid := cs != nil && box.EstablishesFlexFormattingContext() ||
		(cs != nil && (cs.Display == style.DisplayGrid || cs.Display == style.DisplayInlineGrid))

	total := 0.0
	maxH := 0.0
	blockChildCount := 0
	for _, child := range box.Children() {
		if !child.IsInFlow() { continue }
		switch c := child.(type) {
		case *ElementBox:
			// Use the child's real height: explicit height wins, otherwise
			// recurse for intrinsic content height, falling back to the line
			// gap only for text-only boxes. Using fontLineGap unconditionally
			// collapsed flex column items with tall children (e.g. a 150px
			// textarea → container height ~25px, child overflowed).
			h := 0.0
			if wbFlexDebug && flexName(box) == "div.ws-divider" {
				fmt.Fprintf(os.Stderr, "[flex/intr] ws-divider child %s: el=%v cssH=%v display=%v\n",
					flexName(c), c.Element() != nil, c.Style().Height, c.Style().Display)
			}
			if cs := c.Style(); cs != nil {
				if hv, ok := definiteHeight(cs.Height, 0, fontSizeOf(c)); ok && hv > 0 {
					h = hv
				} else {
					h = intrinsicContentHeight(c)
				}
			}
			// Replaced / SVG elements (svg height="N" attribute, img, canvas)
			// size from their attribute dimensions, NOT their (empty) children
			// — otherwise a 14px svg icon falls back to fontLineGap (~16px) and
			// inflates column-flex items and blocks containing icons.
			if el := c.Element(); el != nil {
				if h <= 0 || el.LocalName() == "svg" || el.LocalName() == "img" {
					if ht := el.GetAttribute("height"); ht != "" {
						if f, err := strconv.ParseFloat(ht, 64); err == nil && f > 0 {
							h = f
						}
					}
				}
				if wbFlexDebug && flexName(box) == "button.tb-btn" && el.LocalName() == "svg" {
					fmt.Fprintf(os.Stderr, "[flex/intr] tb-btn child svg: cssH=%v attrH=%q → h=%.1f\n",
						c.Style().Height, el.GetAttribute("height"), h)
				}
			}
			if h <= 0 { h = fontLineGap(c) }
			if isColFlex {
				total += h
			} else if !isFlexOrGrid && !c.IsFloated() {
				// Block container: block-level children stack vertically, so
				// their heights sum (a toolbar under a textarea must push the
				// container taller, not be capped by max(child)).
				blockChildCount++
				total += h
			}
			if h > maxH { maxH = h }
		case *InlineTextBox:
			// Whitespace-only text (e.g. the newline between <button> and
			// <svg> in Vue templates) contributes NO height — a button whose
			// only child is an svg icon must size to the icon + padding, not
			// to fontLineGap (13.3px×1.5=20px). Edge: .tb-btn = 14px icon +
			// 2×2 padding + 2×1 border = 20px, not 26px.
			raw := strings.TrimSpace(c.text)
			if raw == "" {
				continue
			}
			// ★ Text line height honors the element's CSS line-height first
			// (ws-name line-height:1.2 → 15.6), falling back to font metrics
			// (Arial 13px ≈ 17.2) only when line-height is unset/normal.
			// Using font metrics unconditionally inflated .ws-item 28→28.8px
			// (Edge 28px) and shifted project-section 6px down.
			h := cssLineHeight(box)
			if h <= 0 {
				h = fontLineGap(box)
			}
			if isColFlex { total += h }
			if h > maxH { maxH = h }
		}
	}
		if wbFlexDebug && (flexName(box) == "button.tb-btn" || flexName(box) == "div.explorer-toolbar") {
			fmt.Fprintf(os.Stderr, "[flex/intr] %s: children=%d\n", flexName(box), len(box.Children()))
		}
		if !isColFlex && blockChildCount > 0 && !isFlexOrGrid {
		// True block container: multiple block children stack vertically, so
		// their heights sum. Flex/grid containers do NOT sum — their children
		// share one line/row and the cross-axis size is the MAX child height
		// (a row-flex toolbar with tb-title(13px)+tb-spacer(15px)+tb-btn(20px)
		// must be 20px tall, not 48px summed — Edge reference).
		maxH = total
	}
	if isColFlex {
		maxH = total
		if cs.Gap.Value > 0 || cs.RowGap.Value > 0 {
			gapV := cs.Gap.Value
			if gapV <= 0 { gapV = cs.RowGap.Value }
			unit := cs.Gap.Unit
			if unit == "" { unit = cs.RowGap.Unit }
			gap := gapV
			if unit == "em" { gap *= fontSizeOf(box) }
			if gap > 0 {
				count := 0
				for _, child := range box.Children() {
					if child.IsInFlow() { count++ }
				}
				maxH += gap * float64(count-1)
			}
		}
	}
	if cs != nil {
		fs := fontSizeOf(box)
		_, p, b := computeBoxModel(box, 0, fs)
		maxH += p.Top + p.Bottom + b.Top + b.Bottom
	}
	return maxH
}



func (c *FlexFormattingContext) distributeFreeSpace(items []*flexItem, containerMainSize float64, isRow bool) {
	// Per CSS-FLEXBOX §9.7 (Resolving Flexible Lengths):
	//   1. resolve flex base sizes (done in resolveBaseSize, already clamped)
	//   2. freeze items with no flex factor
	//   3. loop: distribute free space to unfrozen items, then re-clamp each
	//      item's target size by min/max; freeze items that hit a boundary and
	//      re-distribute the remaining space until no item changes.
	clamp := func(it *flexItem, v float64) float64 {
		if isRow {
			return clampSize(v, it.minWidth, it.maxWidth, it.minWidth <= 0, it.maxWidth <= 0)
		}
		return clampSize(v, it.minHeight, it.maxHeight, it.minHeight <= 0, it.maxHeight <= 0)
	}

	// The flex free space subtracts the inter-item gaps (CSS-FLEXBOX §9.7):
	// free space = container size − Σ flex base sizes − gaps. Missing this
	// made grow distribute too much (e.g. conv-stats-detail got 137px in a
	// 129px slot), overflowing the container's right padding and clipping
	// the cs-val text against the panel edge.
	gap := 0.0
	if len(items) > 1 {
		if p := items[0].box.Parent(); p != nil {
			if pcs := p.Style(); pcs != nil {
				gap = flexGap(pcs, isRow, fontSizeOf(items[0].box))
			}
		}
	}
	if wbFlexDebug && len(items) > 0 && items[0].box.Parent() != nil && flexName(items[0].box.Parent()) == "div.chat-area" {
		pcs := items[0].box.Parent().Style()
		fmt.Fprintf(os.Stderr, "[flex/gap] chat-area: gap=%.1f rowGap=%.1f colGap=%.1f isRow=%v\n",
			gap, pcs.RowGap.Value, pcs.ColumnGap.Value, isRow)
	}
	gapTotal := gap * float64(len(items)-1)

	for _, it := range items {
		it.targetSize = it.baseSize
		it.frozen = it.flexGrow <= 0 && it.flexShrink <= 0
	}

		for {
			anyUnfrozen := false
			activeGrow, activeShrink := 0.0, 0.0
			totalUsed := 0.0
			for _, it := range items {
				totalUsed += it.targetSize + it.marginMain
				if it.frozen {
					continue
			}
			anyUnfrozen = true
			activeGrow += it.flexGrow
			activeShrink += it.flexShrink
		}
		if !anyUnfrozen {
			break
		}

		freeSpace := containerMainSize - totalUsed - gapTotal
		if freeSpace > 0 && activeGrow > 0 {
			// Free space distributes to the items' main size. targetSize is
			// the border-box main size for border-box items; baseSize already
			// excludes (basisExplicit 0%) or includes (intrinsic auto) the
			// item's own padding, so NO separate padSum deduction is needed —
			// subtracting padding here shrunk .title-center 1178→1168 (Edge:
			// 1280 − 48 logo − 46 menubar − 8 title-right = 1178 exactly).
			for _, it := range items {
				if it.frozen || it.flexGrow <= 0 {
					continue
				}
				it.targetSize += freeSpace * it.flexGrow / activeGrow
			}
		} else if freeSpace < 0 && activeShrink > 0 && containerMainSize > 0 {
			// Auto-sized containers (mainSize 0, e.g. an absolutely-positioned
			// flex column like cache-ring-label) must NOT shrink their items:
			// freeSpace = 0 - sum(base) is always negative, so every item was
			// collapsed to 0 and the container stayed 0-height, stacking the
			// "0%" and "缓存命中" spans on top of each other.
			scaledBaseSum := 0.0
			for _, it := range items {
				if it.frozen || it.flexShrink <= 0 {
					continue
				}
				scaledBaseSum += it.flexShrink * it.baseSize
			}
			if scaledBaseSum > 0 {
				for _, it := range items {
					if it.frozen || it.flexShrink <= 0 {
						continue
					}
					shrink := -freeSpace * (it.flexShrink * it.baseSize) / scaledBaseSum
					it.targetSize -= shrink
					if it.targetSize < 0 {
						it.targetSize = 0
					}
				}
			}
		}

		// Re-clamp unfrozen items; freeze any that hit a min/max boundary.
		frozeAny := false
		for _, it := range items {
			if it.frozen {
				continue
			}
			clamped := clamp(it, it.targetSize)
			if clamped != it.targetSize {
				it.targetSize = clamped
				it.frozen = true
				frozeAny = true
			}
		}
		// If nothing froze this pass, the space is fully consumed — stop.
		if !frozeAny {
			break
		}
	}

	for _, it := range items {
		it.finalMainSize = it.targetSize
	}
}

func (c *FlexFormattingContext) resolveCrossSizes(items []*flexItem, isRow, _, _ bool, cbWidth, cbHeight float64, state *LayoutState, lineCross float64) {
	containerCS := c.Root().Style()
	alignItems := "stretch"
	if containerCS != nil && containerCS.AlignItems != "" {
		alignItems = containerCS.AlignItems
	}
	for _, it := range items {
		cs := it.box.Style()
		if cs == nil { continue }
		g := state.GeometryForBox(it.box)
		align := alignItems
		if cs.AlignSelf != "" && cs.AlignSelf != "auto" {
			align = cs.AlignSelf
		}

		if isRow {
			r := resolveLengthAuto(cs.Height, cbHeight, fontSizeOf(it.box))
			if !r.Auto && r.Definite {
				h := r.Value
				// border-box: explicit height includes border+padding, so the
				// content height must shrink (e.g. switch track 34x18 with 1px
				// border → content 16). Without this the box renders 2px taller.
				if isBorderBox(it.box) {
					h -= g.BorderTop() + g.BorderBottom() + g.PaddingTop() + g.PaddingBottom()
				}
				if h < 0 {
					h = 0
				}
				g.SetContentHeight(h)
			} else if align == "stretch" {
				stretchH := lineCross - it.marginCross - g.VerticalBorderAndPadding()
				if stretchH < 0 { stretchH = 0 }
				g.SetContentHeight(stretchH)
			}
		} else {
			r := resolveLengthAuto(cs.Width, cbWidth, fontSizeOf(it.box))
			if !r.Auto && r.Definite {
				w := r.Value
				// border-box: explicit width includes border+padding, so the
				// content width must shrink (e.g. activity-bar button 40px with
				// 2px border-left → content 38; without this the button renders
				// 42px and overflows its 40px column).
				if isBorderBox(it.box) {
					w -= g.BorderLeft() + g.BorderRight() + g.PaddingLeft() + g.PaddingRight()
				}
				if w < 0 {
					w = 0
				}
				g.SetContentWidth(w)
			} else if align == "stretch" {
				stretchW := cbWidth - it.marginCross - g.HorizontalBorderAndPadding()
				if stretchW < 0 { stretchW = 0 }
				g.SetContentWidth(stretchW)
			} else {
				// Non-stretch: cross-axis size = fit-content =
				//   min(max-content, max(min-content, available))
				// max-content: .welcome-logo "PairCode" 48px font = 217px —
				//   an unbreakable word must NOT shrink (Edge x=268 center).
				// available: .welcome-sub is normal-wrapping text, so its
				//   min-content is tiny and it wraps to the 97px container
				//   (Edge h=64 multi-line, NOT a 317px single line).
				iw := intrinsicContentWidth(it.box, false)
				minC := minContentWidth(it.box)
				avail := cbWidth - it.marginCross - g.HorizontalBorderAndPadding()
				if avail < 0 { avail = 0 }
				fit := minC
				if fit < avail { fit = avail }
				if iw < fit { fit = iw }
				if fit < 0 { fit = 0 }
				g.SetContentWidth(fit)
			}
		}
	}
}

func (c *FlexFormattingContext) applyPositions(items []*flexItem, container *ElementBox, isRow, isReverse, _ bool, state *LayoutState, crossStart float64) {
	cg := state.GeometryForBox(container)
	cx := cg.ContentBoxLeft()
	cy := cg.ContentBoxTop()
	cw := cg.ContentWidth()
	ch := cg.ContentHeight()
	containerCS := container.Style()

	mainPos := cx
	crossPos := crossStart
	if !isRow {
		// Column flex: main axis is Y (vertical), cross axis is X (horizontal)
		mainPos = cy
		crossPos = crossStart
	}
	if isReverse {
		if isRow { mainPos = cx + cw } else { mainPos = cy + ch }
	}

	// Apply justify-content by adjusting initial mainPos.
	justify := "flex-start"
	if containerCS != nil && containerCS.JustifyContent != "" {
		justify = containerCS.JustifyContent
	}
	totalMain := 0.0
	for _, it := range items {
		totalMain += it.marginMain + it.finalMainSize
	}

	// CSS gap (flex gap) between items.
	gap := 0.0
	if containerCS != nil {
		gap = flexGap(containerCS, isRow, fontSizeOf(container))
	}

	if isRow {
		if totalMain < cw && justify != "flex-start" {
			switch justify {
			case "center", "flex-end":
				// Deduct the explicit inter-item gaps so center/flex-end align
				// the group (items + gaps) as one block — otherwise center
				// shifts by half the gap sum too far (welcome content sat 8px
				// low in a 3-item column with 2×8px gaps).
				freeGap := cw - totalMain - gap*float64(len(items)-1)
				if freeGap < 0 {
					freeGap = 0
				}
				if justify == "center" {
					if isReverse { mainPos -= freeGap / 2 } else { mainPos += freeGap / 2 }
				} else {
					if isReverse { mainPos -= freeGap } else { mainPos += freeGap }
				}
			case "space-between", "space-around":
				itemCount := len(items)
				if itemCount > 1 {
					// freeGap is the leftover after items AND the explicit gap
					// spacings; the justify gaps are distributed over the
					// remaining free space only (CSS Flexbox §8.2).
					freeGap := cw - totalMain
					gapSum := gap * float64(itemCount-1)
					if justify == "space-around" { gapSum = gap * float64(itemCount) }
					freeSpace := freeGap - gapSum
					if freeSpace < 0 { freeSpace = 0 }
					divisor := float64(itemCount - 1)
					if justify == "space-around" { divisor = float64(itemCount) }
					extraGap := freeSpace / divisor
					gap += extraGap
				}
			}
		}
	} else {
		if totalMain < ch && justify != "flex-start" {
			switch justify {
			case "center", "flex-end":
				freeGap := ch - totalMain - gap*float64(len(items)-1)
				if freeGap < 0 {
					freeGap = 0
				}
				if justify == "center" {
					if isReverse { mainPos -= freeGap / 2 } else { mainPos += freeGap / 2 }
				} else {
					if isReverse { mainPos -= freeGap } else { mainPos += freeGap }
				}
			case "space-between", "space-around":
				itemCount := len(items)
				if itemCount > 1 {
					freeGap := ch - totalMain
					gapSum := gap * float64(itemCount-1)
					if justify == "space-around" { gapSum = gap * float64(itemCount) }
					freeSpace := freeGap - gapSum
					if freeSpace < 0 { freeSpace = 0 }
					divisor := float64(itemCount - 1)
					if justify == "space-around" { divisor = float64(itemCount) }
					extraGap := freeSpace / divisor
					gap += extraGap
				}
			}
		}
	}
	// Pre-compute baseline offsets for align-items:baseline support.
	maxBO := 0.0
	for _, it := range items {
		align := alignOf(it.box, containerCS)
		if align == "baseline" {
			it.baselineOffset = baselineOffset(it.box, state)
			if it.baselineOffset > maxBO {
				maxBO = it.baselineOffset
			}
		}
	}

	for _, it := range items {
		g := state.GeometryForBox(it.box)
		cs := it.box.Style()
		fs := fontSizeOf(it.box)
		// Fixed main-axis margin: shift position by margin-start before item.
		if cs != nil {
			if isRow {
				mainPos += resolveOrZero(cs.MarginLeft, cw, fs)
			} else {
				mainPos += resolveOrZero(cs.MarginTop, cw, fs)
			}
		}
		// Auto main-axis margin: absorb remaining free space (CSS-FLEXBOX §9.5).
		// The remaining space is the container main size minus what earlier
		// siblings already occupied (mainPos-cx, which includes gaps) minus
		// this item's own main size — using `cw-totalMain` double-counted the
		// gaps and pushed margin-left:auto items past the content edge
		// (conv-stats-total right=1284 > sidebar edge 1280, clipping text).
		if cs != nil && isRow && cs.MarginLeft.Unit == "auto" {
			if rem := cw - (mainPos - cx) - it.finalMainSize; rem > 0 {
				mainPos += rem
			}
		} else if cs != nil && !isRow && cs.MarginTop.Unit == "auto" {
			if rem := ch - (mainPos - cy) - it.finalMainSize; rem > 0 {
				mainPos += rem
			}
		}


		if isRow {
			ms := it.finalMainSize
			g.SetContentWidth(ms)
			// box-sizing: border-box → convert total to content.
			if isBorderBox(it.box) {
				hp := g.PaddingLeft() + g.PaddingRight() + g.BorderLeft() + g.BorderRight()
				g.SetContentWidth(math.Max(0, ms - hp))
			}
			if cs != nil {
				r := resolveLengthAuto(cs.Height, ch, fontSizeOf(it.box))
				if r.Definite && !r.Auto {
					h := r.Value
					if isBorderBox(it.box) {
						h -= g.PaddingTop() + g.PaddingBottom() + g.BorderTop() + g.BorderBottom()
					}
					if h < 0 {
						h = 0
					}
					g.SetContentHeight(h)
				}
			}
		} else {
			ms := it.finalMainSize
			g.SetContentHeight(ms)
			// box-sizing: border-box for column flex.
			if isBorderBox(it.box) {
				vp := g.PaddingTop() + g.PaddingBottom() + g.BorderTop() + g.BorderBottom()
				g.SetContentHeight(math.Max(0, ms - vp))
			}
			if cs != nil {
				r := resolveLengthAuto(cs.Width, cw, fontSizeOf(it.box))
				if r.Definite && !r.Auto {
					w := r.Value
					if isBorderBox(it.box) {
						w -= g.PaddingLeft() + g.PaddingRight() + g.BorderLeft() + g.BorderRight()
					}
					if w < 0 {
						w = 0
					}
					g.SetContentWidth(w)
				}
			}
		}

		if isRow {
			if isReverse { mainPos -= g.BorderBoxWidth() }
			// Set position FIRST so children use correct absolute coordinates.
			bh := g.BorderBoxHeight()
			if bh <= 0 {
				// Estimate intrinsic cross-size from text children for initial
				// positioning before child layout (post-layout corrects it).
				bh = fontLineGap(it.box)
			}
			if wbFlexDebug {
				fmt.Fprintf(os.Stderr, "[flex/pos] %s row: bh=%.1f ch=%.1f cs.H=%v\n", flexName(it.box), bh, ch, cs.Height)
			}
			align := alignOf(it.box, containerCS)
			crossAdjusted := crossPos
			// When container auto-height (ch=0), defer centering/flex-end
			// until after child layout so we can use the actual child height.
			if ch > 0 {
				// Center/flex-end must not push a child above the container's
				// content top when the child is TALLER than the container
				// (auto-height row with a 96px ring in a still-unmeasured
				// body): a negative (ch-bh)/2 offset lifted the cache-ring
				// above its title. Clamp the reference to the child size.
				refH := ch
				if refH < bh {
					refH = bh
				}
				switch align {
				case "center":
					crossAdjusted = crossPos + (refH-bh)/2
				case "baseline":
					crossAdjusted = crossPos + (maxBO - it.baselineOffset)
				case "flex-end":
					crossAdjusted = crossPos + refH - bh
				}
			}
			g.SetTopLeft(crossAdjusted, mainPos)
			bw := g.BorderBoxWidth()
			if !isReverse { mainPos += g.BorderBoxWidth() + resolveOrZero(cs.MarginRight, cw, fs) + gap }

			ctx := contextFor(it.box, state)
			ctx.Layout(it.box, state)

			// Re-apply the explicit cross size (height for row flex): the
			// child's own layout (e.g. a blockified span's BFC) collapses the
			// content height back to the CSS height without subtracting
			// border-box border/padding — a switch track 34x18 with 1px border
			// would render 20px tall (18 content + 2 border). The flex-resolved
			// cross size must win.
			if isRow && cs != nil {
				if h, ok := definiteHeight(cs.Height, g.ContentHeight(), fs); ok && h > 0 {
					if isBorderBox(it.box) {
						h -= g.BorderTop() + g.BorderBottom() + g.PaddingTop() + g.PaddingBottom()
					}
					if h < 0 {
						h = 0
					}
					g.SetContentHeight(h)
				}
			}

			// The flex-resolved main size is authoritative: a child's own
			// layout must not override it with its content size (e.g. a
			// flex:1 project-section whose tall child balloons it past the
			// flex container — this used to overflow the sidebar bottom by
			// ~52px and inflate the frame content size to 829px).
			if it.flexGrow > 0 || it.flexShrink > 0 || it.flexBasis > 0 {
				ms := it.finalMainSize
				if isBorderBox(it.box) {
					hp := g.PaddingLeft() + g.PaddingRight() + g.BorderLeft() + g.BorderRight()
					ms = math.Max(0, ms-hp)
				}
				g.SetContentWidth(ms)
			}

			// Post-layout: inline content (IFC) may have expanded the content
			// width. Adjust mainPos for the next sibling accordingly.
			if isRow {
				newBW := g.BorderBoxWidth()
				if newBW > bw {
					mainPos += newBW - bw
				}
			}

			// Propagate auto cross-size from children.
			if heightIsAutoForBox(it.box) {
				oldTop := g.Top()
				bh2 := g.BorderBoxHeight()
				newCross := crossPos
				if ch > 0 {
					refH := ch
					if refH < bh2 {
						refH = bh2
					}
					switch align {
					case "center":
						newCross = crossPos + (refH-bh2)/2
					case "baseline":
						newCross = crossPos + (maxBO - it.baselineOffset)
					case "flex-end":
						newCross = crossPos + refH - bh2
					}
				} else {
					// Container auto-height: center within actual child height.
					switch align {
					case "center":
						newCross = crossPos
					case "baseline":
						newCross = crossPos + (maxBO - it.baselineOffset)
					case "flex-end":
						newCross = crossPos
					}
				}
				delta := newCross - oldTop
				if delta != 0 {
					shiftBoxAndDescendants(it.box, delta, 0, state)
				}


			}
		} else {
			if isReverse { mainPos -= g.BorderBoxHeight() }
			// Set position FIRST.
			bw := g.BorderBoxWidth()
			align := alignOf(it.box, containerCS)
			crossAdjusted := crossPos
			switch align {
			case "center":
				crossAdjusted = crossPos + (cw-bw)/2
			case "baseline":
				crossAdjusted = crossPos + (maxBO - it.baselineOffset)
			case "flex-end":
				crossAdjusted = crossPos + cw - bw
			}
			if wbFlexDebug && (flexName(it.box) == "div.welcome-logo" || flexName(it.box) == "div.welcome-text" || flexName(it.box) == "div.welcome-sub") {
				fmt.Fprintf(os.Stderr, "[flex/pos] %s col: bw=%.1f cw=%.1f align=%q crossPos=%.1f → crossAdjusted=%.1f\n",
					flexName(it.box), bw, cw, align, crossPos, crossAdjusted)
			}
			g.SetTopLeft(mainPos, crossAdjusted)
			
			ctx := contextFor(it.box, state)
			ctx.Layout(it.box, state)

			// Restore the flex-resolved main size (height for column flex)
			// after child layout so a tall child cannot balloon the item
			// past the flex container (see row branch above). Honor the flex
			// item's automatic minimum (min-height:auto): the item must never
			// shrink below its content's intrinsic height — .welcome-sub's
			// text wraps to 4 lines (62px) but intrinsicContentHeight
			// pre-layout estimated 1 line (17px); pinning to the estimate
			// clipped the multi-line content.
			//
			// EXCEPTION (CSS-FLEXBOX §4.5): when the item's overflow is NOT
			// visible (auto/hidden/scroll), its automatic minimum size is
			// ZERO — the flex-resolved height wins and overflowing content is
			// clipped/scrolled, not inflated. Without this, .project-section
			// (flex:1; overflow-y:auto) ballooned to its full 2776px content
			// height instead of the flex slot (~605px), overflowing the
			// sidebar and spawning the wrong scrollbars.
			if it.flexGrow > 0 || it.flexShrink > 0 || it.flexBasis > 0 {
				ms := it.finalMainSize
				if isBorderBox(it.box) {
					vp := g.PaddingTop() + g.PaddingBottom() + g.BorderTop() + g.BorderBottom()
					ms = math.Max(0, ms-vp)
				}
				if itc := it.box.Style(); itc != nil && flexOverflowVisible(itc) {
					if after := g.ContentHeight(); after > ms {
						// min-height:auto — content wrapped taller than the slot.
						ms = after
					}
				}
				g.SetContentHeight(ms)
			}

			if !isReverse { mainPos += g.BorderBoxHeight() + resolveOrZero(cs.MarginBottom, cw, fs) + gap }
			if wbFlexDebug && flexName(it.box) == "div.chat-messages" {
				fmt.Fprintf(os.Stderr, "[flex/pos] chat-messages final: ms=%.1f top=%.1f h=%.1f bh=%.1f\n",
					it.finalMainSize, g.Top(), g.ContentHeight(), g.BorderBoxHeight())
			}

			// Propagate auto cross-size (width) from children.
			oldLeft := g.Left()
			bw2 := g.BorderBoxWidth()
			newCross := crossPos
			// Width centering uses the CONTAINER width cw — do NOT clamp the
			// reference to the item width (that collapses (cw-bw2)/2 to 0 when
			// the item is wider than the container, undoing the centering:
			// .welcome-logo 217px in a 97px main-area must overflow-center at
			// x=268 like Edge, not snap back to x=327). The row branch clamps
			// refH for height centering to avoid pushing items above the
			// container top; width has no such constraint.
			switch align {
			case "center":
				newCross = crossPos + (cw-bw2)/2
			case "baseline":
				newCross = crossPos + (maxBO - it.baselineOffset)
			case "flex-end":
				newCross = crossPos + cw - bw2
			}
			delta := newCross - oldLeft
			if delta != 0 {
				shiftBoxAndDescendants(it.box, 0, delta, state)
			}
			if wbFlexDebug && flexName(it.box) == "div.welcome-logo" {
				fmt.Fprintf(os.Stderr, "[flex/pos] welcome-logo final: x=%.1f y=%.1f w=%.1f (oldLeft=%.1f bw2=%.1f delta=%.1f)\n",
					g.Left(), g.Top(), g.BorderBoxWidth(), oldLeft, bw2, delta)
			}

			// NOTE: no per-item re-centering on the main axis here. The old
			// code re-centered EVERY auto-height item to cy+(ch-bh)/2 when
			// justify-content:center — collapsing all children to the single
			// container-center position (the welcome-page text overlap bug).
			// Group re-centering for auto-height containers happens once
			// after the loop below.
		}
	}

	// Column flex: justify-content was applied against the ESTIMATED container
	// height at the top of applyPositions. After child layout determines real
	// heights (min-height:auto may have grown an item — .welcome-sub wrapped
	// to 62px vs a 17px pre-layout estimate), re-center / flex-end the whole
	// group (single shift, not per-item) so items keep their stacked positions.
	// Applies to definite-height containers too (welcome has height:100%).
	if !isRow && justify != "flex-start" && len(items) > 0 {
		g0 := state.GeometryForBox(items[0].box)
		gLast := state.GeometryForBox(items[len(items)-1].box)
		groupTop := g0.Top()
		groupBottom := gLast.Top() + gLast.BorderBoxHeight()
		groupH := groupBottom - groupTop
		if groupH > 0 {
			var shift float64
			switch justify {
			case "center":
				shift = cy + (ch-groupH)/2 - groupTop
			case "flex-end":
				shift = cy + ch - groupH - groupTop
			default:
				shift = 0
			}
			if shift != 0 {
				for _, it := range items {
					shiftBoxAndDescendants(it.box, shift, 0, state)
				}
			}
		}
	}
}

// shiftBoxAndDescendants adds (dy, dx) to the geometry top-left position of box
// and all its layout descendants, including text segment coordinates.
func shiftBoxAndDescendants(box *ElementBox, dy, dx float64, state *LayoutState) {
	if box == nil { return }
	g := state.GeometryForBox(box)
	g.SetTopLeft(g.Top()+dy, g.Left()+dx)
	// Shift text segment positions too.
	for i := range box.TextSegments {
		box.TextSegments[i].X += dx
		box.TextSegments[i].Y += dy
	}
	for _, child := range box.Children() {
		switch c := child.(type) {
		case *ElementBox:
			shiftBoxAndDescendants(c, dy, dx, state)
		case *InlineTextBox:
			for i := range c.TextSegments {
				c.TextSegments[i].X += dx
				c.TextSegments[i].Y += dy
			}
		}
	}
}

// maxChildContentHeight returns the maximum intrinsic border-box height of all
// in-flow children, used to determine auto cross-size from content.
// This uses the child's own computed height (not its positioned bottom edge)
// to avoid the chicken-and-egg problem where centering within height=0 gives
// a negative offset that would produce a wrong auto-height.
func maxChildContentHeight(box *ElementBox, state *LayoutState) float64 {
	maxH := 0.0
	for _, child := range box.Children() {
		if !child.IsInFlow() {
			continue
		}
		if eb, ok := child.(*ElementBox); ok {
			g := state.GeometryForBox(eb)
			bh := g.BorderBoxHeight()
			if bh > maxH {
				maxH = bh
			}
		}
	}
	return maxH
}

// alignOf returns the effective align-self value for box in its flex container.
func alignOf(box *ElementBox, containerCS *style.ComputedStyle) string {
	cs := box.Style()
	if cs != nil && cs.AlignSelf != "" && cs.AlignSelf != "auto" {
		return cs.AlignSelf
	}
	if containerCS != nil && containerCS.AlignItems != "" {
		return containerCS.AlignItems
	}
	return "stretch"
}

// flexGap returns the effective gap for the flex container.
func flexGap(cs *style.ComputedStyle, isRow bool, fontSize float64) float64 {
	var gapV float64
	if isRow {
		gapV = cs.ColumnGap.Value
		if cs.Gap.Value > 0 {
			gapV = cs.Gap.Value
		}
	} else {
		gapV = cs.RowGap.Value
		if cs.Gap.Value > 0 {
			gapV = cs.Gap.Value
		}
	}
	if cs.Gap.Unit == "em" {
		gapV *= fontSize
	}
	return gapV
}

var _ = style.DisplayFlex
var _ = math.Max

// baselineOffset returns the distance from a box's border-box top edge to its
// text baseline, used for align-items:baseline alignment in flex layout.
// For a flex container, we walk the first in-flow child to find the real
// text baseline, accounting for nested padding/border.
func baselineOffset(box *ElementBox, state *LayoutState) float64 {
	return baselineOffsetRec(box, state, 0)
}

func baselineOffsetRec(box *ElementBox, state *LayoutState, depth int) float64 {
	g := state.GeometryForBox(box)
	padTop := g.PaddingTop() + g.BorderTop()

	// For flex/grid containers, recurse into the first in-flow child.
	if depth < 10 && (box.EstablishesFlexFormattingContext() || box.EstablishesGridFormattingContext()) {
		for _, child := range box.Children() {
			if !child.IsInFlow() || child.IsAbsolutelyPositioned() {
				continue
			}
			if eb, ok := child.(*ElementBox); ok {
				return padTop + baselineOffsetRec(eb, state, depth+1)
			}
			if tb, ok := child.(*InlineTextBox); ok && len(tb.TextSegments) > 0 {
				return padTop + tb.TextSegments[0].Height
			}
			break
		}
	}

	// Default: use font ascent from this box's resolved font.
	ascent, _ := fontAscentDescent(box)
	return padTop + ascent
}
