// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
// BlockFormattingContext lays out in-flow children vertically (or horizontally in
// vertical writing mode), with margin collapse and float containment.

package layout

import (
	"fmt"
	"math"
	"os"
	"strings"

	"wb-ui/style"
)

var diagFloats *os.File

// initDiagFloats lazily opens the float diagnostics file only when the
// WBUI_FLOAT_DIAG environment variable is set, so normal runs/tests don't
// litter the working directory with diag_floats.txt.
func initDiagFloats() *os.File {
	if diagFloats != nil {
		return diagFloats
	}
	if os.Getenv("WBUI_FLOAT_DIAG") == "" {
		return nil
	}
	f, err := os.Create("diag_floats.txt")
	if err != nil {
		return nil
	}
	f.WriteString("=== float layout diagnostics ===\n")
	diagFloats = f
	return diagFloats
}

// diagf writes a diagnostic line, no-op unless WBUI_FLOAT_DIAG is set.
func diagf(format string, args ...interface{}) {
	f := initDiagFloats()
	if f == nil {
		return
	}
	fmt.Fprintf(f, format, args...)
}

type BlockFormattingContext struct {
	FormattingContextBase
}

func (c *BlockFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	if box.Style() == nil {
		// Can't set Style() through the interface; skip default assignment
	}
	g := state.GeometryForBox(box)
	if box.Parent() == nil {
		margin, padding, border := computeBoxModelForBox(box, state.ViewportWidth, fontSizeOf(box))
		g.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
		g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
		g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
		g.SetContentWidth(state.ViewportWidth - border.Horizontal() - padding.Horizontal())
		if state.ViewportHeight > 0 {
			g.SetContentHeight(state.ViewportHeight - border.Vertical() - padding.Vertical())
		}
	}

	cs := box.Style()
	isVerticalWM := IsVerticalWritingMode(cs)
	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	contentWidth := g.ContentWidth()
	if os.Getenv("WB_LAYOUT_DEBUG") != "" && box.Element() != nil && box.Element().NodeName() == "DIV" && box.Element().GetAttribute("class") == "dialog-box" {
		fmt.Printf("[bfc] dialog-box: contentWidth=%.1f parent=%v children=%d\n", contentWidth, box.Parent(), len(box.Children()))
	}

	blockStart := g.ContentBoxTop()
	if isVerticalWM {
		blockStart = g.ContentBoxLeft()
	}

	establishesBFC := box.EstablishesBlockFormattingContext()

	// Margin collapsing: if this box has no border/padding on top (so its top
	// margin is "adjoining" with its first in-flow child), seed the bubbled
	// margin with this box's own top margin. The child's collapsed offset then
	// absorbs the max of both (CSS 2.1 §8.3.1).
	if !establishesBFC && g.BorderTop() == 0 && g.PaddingTop() == 0 && box.Parent() != nil {
		state.AdjoiningTopMargin = g.MarginBefore()
	}

	var fc *floatContext
	if establishesBFC {
		// Only BFC-establishing boxes create their own float context.
		// Non-BFC boxes inherit the parent's FC, so floats placed by
		// siblings are visible to inline content for text wrapping.
		fc = newFloatContext(contentX, contentY, contentWidth)
		prev := state.setFloatContext(fc)
		defer state.restoreFloatContext(prev)
	} else {
		fc = state.currentFloatContext()
		if fc == nil {
			fc = newFloatContext(contentX, contentY, contentWidth)
			state.setFloatContext(fc)
		}
	}

	// DIAG: log container geometry for boxes with floats
	for _, ch := range box.Children() {
		if eb, ok := ch.(*ElementBox); ok && eb.IsFloated() {
			diagf("parent=%q contentX=%.0f contentY=%.0f contentWidth=%.0f bfc=%t\n",
				box.Style().Display, contentX, contentY, contentWidth, establishesBFC)
			break
		}
	}

	cursor := blockStart
	pendingMargin := 0.0
	collapseTopWithParent := !establishesBFC && g.BorderTop() == 0 && g.PaddingTop() == 0
	firstInFlow := true

		diagf("> layoutBlockChildren: box=%s contentX=%.0f contentY=%.0f contentWidth=%.0f childCount=%d\n",
		elementName(box), contentX, contentY, contentWidth, len(box.Children()))

	var deferredAbsolutes []*ElementBox

	for i, child := range box.Children() {
		if !child.IsVisible() {
			diagf("  [!INVIS i=%d child=%s type=%T]\n", i, elementNameOf(child), child)
			continue
		}
		childEb, childIsEb := child.(*ElementBox)
		if !childIsEb {
			diagf("  [!NOTEB i=%d child=%T]\n", i, child)
			continue
		}
		childCs := childEb.Style()
		diagf("  [i=%d] child=%s float=%q width=%v display=%d style=%p\n",
			i, elementName(childEb), childCs.Float, childCs.Width, childCs.Display, childCs)
		if child.IsFloated() {
			layoutFloatedChild(childEb, contentX, contentY, contentWidth, fc, state)
			continue
		}
		if child.IsAbsolutelyPositioned() {
			deferredAbsolutes = append(deferredAbsolutes, childEb)
			continue
		}

		ch := state.GeometryForBox(childEb)
		if os.Getenv("WB_LAYOUT_DEBUG") != "" && childEb.Element() == nil && box.Element() != nil && box.Element().GetAttribute("class") == "dialog-box" {
			fmt.Printf("[bfc] dialog-box child (anon) content=%.1f border=%.1f display=%d\n", ch.ContentWidth(), ch.BorderBoxWidth(), childCs.Display)
		}
		margin, padding, border := computeBoxModelForBox(childEb, contentWidth, fontSizeOf(childEb))
		ch.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
		ch.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
		ch.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

		borderBoxWidth := computeBlockChildBorderBoxWidth(childEb, contentWidth, margin, border, padding, state)
		ch.SetContentWidth(borderBoxWidth - border.Horizontal() - padding.Horizontal())
		ch.SetTopLeft(0, g.ContentBoxLeft()+margin.Left) // Y set below

		clearSide := clearSideOf(childEb)
		if clearSide != "" && fc != nil {
			cursor = fc.clearedY(cursor, clearSide)
		}

		breakBefore := child.Style().GetProperty("break-before")
		if breakBefore == "page" || breakBefore == "always" {
			pageHeight := state.ViewportHeight
			if pageHeight > 0 {
				currentPage := math.Floor(cursor / pageHeight)
				cursor = math.Max(cursor, (currentPage+1)*pageHeight)
			}
		}

		topMargin := margin.Top
		collapsedTop := 0.0
		if firstInFlow && collapseTopWithParent {
			// The child's top margin collapses with this box's own top margin
			// (CSS 2.1 §8.3.1). The box's own margin has already been applied
			// as its content offset, so the child only advances by the amount
			// that exceeds the box's margin (the larger of the two wins).
			collapsedTop = topMargin
			if bubbled := state.AdjoiningTopMargin; bubbled > collapsedTop {
				collapsedTop = bubbled
			}
			// Box's margin is already in the cursor position; only the excess
			// over the box's margin moves the child further down.
			if boxOwn := g.MarginBefore(); boxOwn > 0 {
				if excess := collapsedTop - boxOwn; excess > 0 {
					collapsedTop = excess
				} else {
					collapsedTop = 0
				}
			}
			state.AdjoiningTopMargin = collapsedTop
		} else {
			collapsedTop = math.Max(pendingMargin, topMargin)
		}
		cursor += collapsedTop
		ch.SetTopLeft(cursor, ch.Left())

		cbHeight := g.ContentHeight()
		if cbHeight <= 0 && box.Parent() != nil {
			cbHeight = state.GeometryForBox(box.Parent()).ContentHeight()
		}
		if cbHeight <= 0 {
			cbHeight = 0 // will still try to apply definite height below
		}
		cs := child.Style()
		if !heightIsAutoForBox(childEb) {
			if cbHeight > 0 {
				fs := fontSizeOf(childEb)
				hv, ok := definiteHeight(cs.Height, cbHeight, fs)
				if ok {
					if isBorderBoxForBox(childEb) {
						ch.SetContentHeight(hv - border.Vertical() - padding.Vertical())
					} else {
						ch.SetContentHeight(hv)
					}
				}
			} else {
				// Even when cbHeight is 0 (parent not yet sized), apply a
				// definite px/em height from CSS. This is critical for replaced
				// elements (input, select) whose height is set via CSS.
				fs := fontSizeOf(childEb)
				hv, ok := definiteHeight(cs.Height, 100, fs)
				if ok && hv > 0 {
					if isBorderBoxForBox(childEb) {
						ch.SetContentHeight(hv - border.Vertical() - padding.Vertical())
					} else {
						ch.SetContentHeight(hv)
					}
				}
			}
		} else if cbHeight > 0 && childNeedsHeightConstraintForBox(childEb) {
			remaining := cbHeight - (cursor - g.ContentBoxTop())
			if remaining > 0 {
				// Grid container child: give it the parent's remaining height
				// so 1fr rows can resolve (height:auto grid has no definite
				// height to distribute fr against). Column-flex children are
				// NOT stretched (childNeedsHeightConstraintForBox only admits
				// grids) — a block-level auto-height column-flex child sizes
				// to its content like Edge's .proj-empty (63px, not 597px).
				ch.SetContentHeight(math.Max(0, remaining-border.Vertical()-padding.Vertical()))
			}
		}

		childCtx := contextFor(childEb, state)
		childCtx.Layout(childEb, state)

		// Compute the list-item marker (bullet "•" / ordered "1." / "a.") for
		// display:list-item children, mirroring the marker box generation in
		// WebCore's layout (RenderListItem + CSS Lists spec §4.1).
		// Only real DOM elements get a marker: anonymous wrappers (which clone
		// the li's display style for their inline content) must not.
		if childCs.Display == style.DisplayListItem && childEb.Element() != nil {
			childEb.MarkerText = listMarkerFor(childEb, box)
		} else {
			childEb.MarkerText = ""
		}

		fs := fontSizeOf(childEb)
		if isVerticalWM {
			minW, maxW, minWAuto, maxWAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, 0, fs)
			bw := ch.BorderBoxWidth()
			ch.SetContentWidth(clampSize(bw, minW, maxW, minWAuto, maxWAuto) - border.Horizontal() - padding.Horizontal())
		} else {
			minH, maxH, minHAuto, maxHAuto := resolveMinMax(cs.MinHeight, cs.MaxHeight, 0, fs)
			bh := ch.BorderBoxHeight()
			ch.SetContentHeight(clampSize(bh, minH, maxH, minHAuto, maxHAuto) - border.Vertical() - padding.Vertical())
		}
		pendingMargin = margin.Bottom
		cursor = ch.Top() + ch.BorderBoxHeight()

		breakAfter := child.Style().GetProperty("break-after")
		if breakAfter == "page" || breakAfter == "always" {
			pageHeight := state.ViewportHeight
			if pageHeight > 0 {
				currentPage := math.Floor(cursor / pageHeight)
				cursor = math.Max(cursor, (currentPage+1)*pageHeight)
			}
		}
		firstInFlow = false
	}

	// Resolve box block size (height for horizontal-tb, width for vertical WM).
	boxCS := box.Style()
	if heightIsAutoForBox(box) {
		blockSize := cursor - blockStart
		if !establishesBFC && g.BorderBottom() == 0 && g.PaddingBottom() == 0 {
			// pendingMargin collapses out
		} else {
			blockSize += pendingMargin
		}
		if establishesBFC && fc != nil {
			if fb := fc.maxFloatBottom(); fb > contentY+blockSize {
				blockSize = fb - contentY
			}
		}
		// Even without BFC, expand height to encompass this container's own
		// float children (clearfix behavior). Without this, a container whose
		// only children are floats would have zero height.
		if !establishesBFC {
			maxFloatBottom := 0.0
			for _, child := range box.Children() {
				if eb, ok := child.(*ElementBox); ok && eb.IsFloated() {
					ch := state.GeometryForBox(eb)
					if b := ch.Top() + ch.BorderBoxHeight(); b > maxFloatBottom {
						maxFloatBottom = b
					}
				}
			}
			if maxFloatBottom > contentY+blockSize {
				blockSize = maxFloatBottom - contentY
			}
		}
		if blockSize < 0 {
			blockSize = 0
		}
		// Preserve any height already set by parent formatting context (e.g. flex cross-axis stretch).
		// Only for non-root boxes — root uses viewport as initial height which must be replaced.
		if box.Parent() != nil && blockSize < g.ContentHeight() {
			blockSize = g.ContentHeight()
		}
		g.SetContentHeight(blockSize)
	} else if box.Parent() != nil {
		fs := fontSizeOf(box)
		cbHeight := state.GeometryForBox(box.Parent()).ContentHeight()
		hv, ok := definiteHeight(boxCS.Height, cbHeight, fs)
		if ok {
			if isBorderBoxForBox(box) {
				g.SetContentHeight(hv - g.VerticalBorder() - g.VerticalPadding())
			} else {
				g.SetContentHeight(hv)
			}
		}
	}

	// Apply relative offsets to in-flow children.
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok {
			if childEb.IsRelativelyPositioned() && childEb.IsInFlow() {
				applyRelativeOffsetForBox(childEb, contentWidth, g.ContentHeight(), state)
			}
		}
	}

	// Lay out absolutely-positioned descendants.

	root := stateRootForBox(box)
	for _, child := range deferredAbsolutes {
		cb := containingBlockForAbsolute(child, root)
		layoutAbsolute(child, cb, root, state)
	}

	// If this replaced element still has 0 content height (no children),
	// apply a minimum intrinsic height so it's visible. This handles
	// <input>, <select>, and other replaced elements without explicit
	// CSS height that are laid out directly via BFC (from IFC).
	if box.IsReplaced() && g.ContentHeight() <= 0 && heightIsAutoForBox(box) {
		fs := fontSizeOf(box)
		if fs <= 0 { fs = 16 }
		lineH := fontLineGap(box)
		if lineH <= 0 { lineH = fs * 1.2 }
		g.SetContentHeight(lineH)
	}
}

// computeBlockChildBorderBoxWidth resolves border-box width of a block child.
func computeBlockChildBorderBoxWidth(child *ElementBox, cbContentWidth float64, margin, border, padding Edges, state *LayoutState) float64 {
	fs := fontSizeOf(child)
	cs := child.Style()
	w, ok := definiteWidth(cs.Width, cbContentWidth, fs)
	if !ok {
		width := cbContentWidth - margin.Horizontal()
		if width < 0 {
			width = 0
		}
		// Tables shrink-to-fit (CSS 2.1 §17.5.2.1): auto-width tables are
		// as wide as their content, never stretched to the available width.
		if child.EstablishesTableFormattingContext() {
			if pref := tablePreferredWidth(child); pref < width {
				width = pref
			}
		}
		minW, maxW, minAuto, maxAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
		return clampSize(width, minW, maxW, minAuto, maxAuto)
	}
	var borderBox float64
	if isBorderBoxForBox(child) {
		borderBox = w
	} else {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	minW, maxW, minAuto, maxAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
	if !isBorderBoxForBox(child) {
		minW += border.Horizontal() + padding.Horizontal()
		if maxAuto || maxW > 0 {
			maxW += border.Horizontal() + padding.Horizontal()
		}
	}
	return clampSize(borderBox, minW, maxW, minAuto, maxAuto)
}

func layoutFloatedChild(child *ElementBox, contentX, contentY, contentWidth float64, fc *floatContext, state *LayoutState) {
	if fc == nil {
		fc = newFloatContext(contentX, 0, contentWidth)
	}
	ch := state.GeometryForBox(child)
	margin, padding, border := computeBoxModelForBox(child, contentWidth, fontSizeOf(child))
	ch.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
	ch.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	ch.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	cs := child.Style()
	fs := fontSizeOf(child)
	w, ok := definiteWidth(cs.Width, contentWidth, fs)
	if !ok {
		w = contentWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
		if w < 0 { w = 0 }
	}
	diagf("  child=%q float=%s widthCSS=%v contentWidth=%.0f definiteW=%.0f ok=%t el=%s\n",
		cs.Display, cs.Float, cs.Width, contentWidth, w, ok, elementName(child))
	borderBox := w
	if !isBorderBoxForBox(child) {
		borderBox = w + border.Horizontal() + padding.Horizontal()
	}
	ch.SetContentWidth(borderBox - border.Horizontal() - padding.Horizontal())

	isLeft := cs.Float != "right"
	// Use container's FC-relative y as startY so placeFloat's collision
	// detection correctly sees sibling floats at the same y level.
	fcY := contentY - fc.originY
	// Include margin in the width passed to placeFloat so subsequent floats
	// are spaced apart by their margins (not placing right against each other).
	marginBoxW := borderBox + margin.Horizontal()
	x, y := fc.placeFloat(child, isLeft, marginBoxW, 0, fcY)
	// Adjust x for coordinate offset between FC origin and container content box,
	// plus margin-left so the float's border box starts at the correct position.
	// y from placeFloat is already FC-relative (starts at fcY). Convert to
	// absolute by adding fc.originY so it matches the page coordinate system.
	x += contentX - fc.originX + margin.Left
	y += fc.originY

	// Clamp float position to container content area when using inherited FC.
	// Without a BFC, the inherited FC may have wider bounds (viewport width)
	// causing floats to escape the container. This keeps them visually contained.
	// Use marginBoxW for clamping because margin was already added to x.
	rightEdge := contentX + contentWidth
	if x+borderBox > rightEdge && rightEdge > contentX {
		x = rightEdge - borderBox
	}
	if x < contentX {
		x = contentX
	}
	ch.SetTopLeft(y, x)

	childCtx := contextFor(child, state)
	childCtx.Layout(child, state)

	for i := range fc.floats {
		if fc.floats[i].box == child {
			fc.floats[i].h = ch.BorderBoxHeight()
			fc.floats[i].w = marginBoxW // store margin-box width for correct collision detection
			// Update the FC-relative y to match the actual rendered position.
			// placeFloat starts at fc.originY, but the rendered y after the
			// contentY - fc.originY adjustment may differ. Keeping the FC y
			// correct ensures IFC queries (contentEdgesAt) find floats at
			// the right vertical position.
			// NOTE: FC x is NOT updated here because placeFloat's collision
			// detection uses FC-relative x (before contentX adjustment).
			fc.floats[i].y = ch.Top() - fc.originY
			break
		}
	}
}

func clearSideOf(box *ElementBox) string {
	if box.Style() == nil { return "" }
	return box.Style().Clear
}

func heightIsAutoForBox(box *ElementBox) bool {
	if box.Style() == nil { return true }
	r := resolveLengthAuto(box.Style().Height, 0, 0)
	return r.Auto
}

// childNeedsHeightConstraintForBox reports whether a block-level child that is
// itself a grid container must be given the parent's remaining height so its
// fr rows can resolve (a grid with height:auto has no definite height to
// distribute 1fr against). Column-flex children are intentionally NOT
// stretched: CSS sizes a block-level in-flow child with height:auto to its
// content (Edge: .proj-empty 63px content-height inside .project-section,
// not the full 597px sidebar) — the stretch hack only ballooned those.
func childNeedsHeightConstraintForBox(box *ElementBox) bool {
	if box == nil || box.Style() == nil { return false }
	cs := box.Style()
	if cs.Display == style.DisplayGrid || cs.Display == style.DisplayInlineGrid { return true }
	return false
}

func stateRootForBox(box *ElementBox) *ElementBox {
	cur := box
	for cur.Parent() != nil { cur = cur.Parent() }
	return cur
}

// listMarkerFor computes the marker text for a display:list-item child of
// container. Unordered lists use disc/circle/square bullets (list-style-type);
// ordered lists use the 1-based index of the item among its ordered siblings
// formatted per the list-style-type (decimal/decimal-leading-zero/lower-alpha/
// upper-alpha/lower-roman/upper-roman). Mirrors CSS Lists & Counters §4.1 and
// WebCore's RenderListItem marker generation.
func listMarkerFor(li *ElementBox, container *ElementBox) string {
	if li == nil || container == nil {
		return ""
	}
	containerStyle := container.Style()
	if containerStyle == nil {
		return ""
	}
	// Explicit list-style-type on the <li> overrides the container's.
	lt := li.Style().ListStyleType
	if lt == "" || lt == "inherit" {
		lt = containerStyle.ListStyleType
	}
	if lt == "" {
		// Infer from the container element: <ul> → disc, <ol> → decimal.
		if el := container.Element(); el != nil {
			switch el.LocalName() {
			case "ol":
				lt = "decimal"
			default:
				lt = "disc"
			}
		} else {
			lt = "disc"
		}
	}

	switch lt {
	case "none":
		return ""
	case "disc":
		return "•"
	case "circle":
		return "◦"
	case "square":
		return "▪"
	case "decimal":
		return fmt.Sprintf("%d.", listIndexAmongSiblings(li))
	case "decimal-leading-zero":
		return fmt.Sprintf("%02d.", listIndexAmongSiblings(li))
	case "lower-alpha":
		return fmt.Sprintf("%s.", listAlphaMarker(listIndexAmongSiblings(li), 'a'))
	case "upper-alpha":
		return fmt.Sprintf("%s.", listAlphaMarker(listIndexAmongSiblings(li), 'A'))
	case "lower-roman":
		return fmt.Sprintf("%s.", listRomanMarker(listIndexAmongSiblings(li), false))
	case "upper-roman":
		return fmt.Sprintf("%s.", listRomanMarker(listIndexAmongSiblings(li), true))
	}
	// Unknown list-style-type: fall back to disc for unordered, decimal for ordered.
	if el := container.Element(); el != nil && el.LocalName() == "ol" {
		return fmt.Sprintf("%d.", listIndexAmongSiblings(li))
	}
	return "•"
}

// listIndexAmongSiblings returns the 1-based index of li among its ordered-list
// siblings (siblings that are themselves list items, within the same container).
func listIndexAmongSiblings(li *ElementBox) int {
	idx := 0
	parent := li.Parent()
	if parent == nil {
		return 1
	}
	for _, s := range parent.Children() {
		sib, ok := s.(*ElementBox)
		if !ok {
			continue
		}
		if sib.Style() != nil && sib.Style().Display == style.DisplayListItem {
			idx++
		}
		if sib == li {
			break
		}
	}
	if idx < 1 {
		idx = 1
	}
	return idx
}

// listAlphaMarker converts a 1-based index to an alphabetical marker
// (1→a, 2→b, 27→aa), using the given base letter ('a' or 'A').
func listAlphaMarker(n int, base rune) string {
	var sb []rune
	for n > 0 {
		n--
		sb = append([]rune{base + rune(n%26)}, sb...)
		n /= 26
	}
	return string(sb)
}

// listRomanMarker converts a 1-based index to a Roman numeral string.
func listRomanMarker(n int, upper bool) string {
	if n <= 0 || n > 3999 {
		return fmt.Sprintf("%d", n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	if !upper {
		for i := range syms {
			syms[i] = strings.ToLower(syms[i])
		}
	}
	var sb strings.Builder
	for i, v := range vals {
		for n >= v {
			sb.WriteString(syms[i])
			n -= v
		}
	}
	return sb.String()
}

func elementName(box *ElementBox) string {
	if box == nil { return "nil" }
	el := box.Element()
	if el == nil { return "anonymous" }
	return fmt.Sprintf("%v", el)
}

func elementNameOf(box Box) string {
	if box == nil { return "nil" }
	if eb, ok := box.(*ElementBox); ok {
		return elementName(eb)
	}
	return fmt.Sprintf("%T", box)
}

