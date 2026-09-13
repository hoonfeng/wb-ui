// Translation of: Source/WebCore/layout/formattingContexts/block/BlockFormattingContext.cpp
//   (out-of-flow layout helpers) + Source/WebCore/rendering/RenderBox.cpp
//   (relative positioning)
//
// Absolute and relative positioning helpers adapted for Box interface + BoxGeometry.

package layout

import (
	"strings"

	"wb-ui/dom"
	"wb-ui/style"
)

// containingBlockForAbsolute walks from box's parent to find the nearest positioned
// ancestor (or root). Returns root if none found.
func containingBlockForAbsolute(box *ElementBox, root *ElementBox) *ElementBox {
	// position:fixed is always positioned against the VIEWPORT (CSS 2.1
	// §10.1) — never against a positioned ancestor. Without this,
	// .toast-container (position:fixed; right:16px) inside a positioned
	// app-root was placed at x=0 (the ancestor's padding box) instead of
	// 1280−16−width (the viewport).
	if box.IsFixedPositioned() {
		// …unless an ancestor establishes a containing block for fixed
		// descendants (transform / filter / perspective / will-change /
		// containment — see createsContainingBlockForFixed).
		for cur := box.Parent(); cur != nil; cur = cur.Parent() {
			if cur == root {
				break
			}
			if createsContainingBlockForFixed(cur.Style()) {
				return cur
			}
		}
		return root
	}
	for cur := box.Parent(); cur != nil; cur = cur.Parent() {
		if cur == root {
			return root
		}
		if createsContainingBlockForAbsolute(cur.Style()) {
			return cur
		}
	}
	return root
}

// ── Containing-block triggers ──
//
// CSS 2.1 §10.1 only names *positioned* ancestors, but the modern effects and
// containment specs add more triggers: an element with a transform, filter,
// perspective, will-change hint, or layout/paint containment becomes the
// containing block of its absolutely positioned descendants.
//
// wb-ui recognised only `position`, so an absolutely positioned box inside e.g.
// a `transform`ed card resolved its insets against a much higher ancestor: the
// badge lost its anchor and landed (clipped or overlapping) somewhere else —
// silently, because the layout still produced *a* box. Fixture:
// render-repros/modern-containing-block-triggers.html (filter / perspective /
// contain:layout / will-change / content-visibility, plus the fixed variants).

// createsContainingBlockForAbsolute reports whether cs makes its element the
// containing block of absolutely positioned descendants.
func createsContainingBlockForAbsolute(cs *style.ComputedStyle) bool {
	if cs == nil {
		return false
	}
	switch cs.Position {
	case style.PositionRelative, style.PositionAbsolute,
		style.PositionFixed, style.PositionSticky:
		return true
	}
	return hasContainmentTrigger(cs)
}

// createsContainingBlockForFixed reports whether cs traps `position:fixed`
// descendants. A fixed box is positioned against the viewport (CSS 2.1 §10.1),
// but the effects/containment specs make these elements its containing block
// all the same — note that `position:relative` alone does NOT.
func createsContainingBlockForFixed(cs *style.ComputedStyle) bool {
	return hasContainmentTrigger(cs)
}

// hasContainmentTrigger reports whether cs carries any effect/containment that
// establishes a containing block (for absolute *and* fixed descendants).
func hasContainmentTrigger(cs *style.ComputedStyle) bool {
	if cs == nil {
		return false
	}
	// CSS Transforms L1 §3 / Filter Effects L1 §3 / CSS Position 3:
	// transform, filter and backdrop-filter create a containing block for every
	// non-none value (a blur(0) or translate(0) still counts).
	if !isNoneValue(cs.Transform) || !isNoneValue(cs.Filter) || !isNoneValue(cs.BackdropFilter) {
		return true
	}
	if !isNoneValue(cs.GetProperty("perspective")) {
		return true
	}
	// Will Change L1 §2: the hint is honoured as if the named property were
	// already applied.
	for _, kw := range splitCSSKeywords(cs.GetProperty("will-change")) {
		switch kw {
		case "transform", "perspective", "filter", "backdrop-filter":
			return true
		}
	}
	// CSS Containment L1 §3: layout containment (and paint containment, which
	// implies it) creates a containing block. `size` alone does not.
	for _, kw := range splitCSSKeywords(cs.GetProperty("contain")) {
		switch kw {
		case "layout", "paint", "strict", "content":
			return true
		}
	}
	// CSS Containment L2: content-visibility:auto applies layout containment.
	if strings.EqualFold(strings.TrimSpace(cs.GetProperty("content-visibility")), "auto") {
		return true
	}
	// CSS Containment L3 container queries: `container-type: size` applies
	// layout containment (containing block); `inline-size` applies inline-size
	// containment only and does NOT create one. The fixture's negative control
	// pins that distinction: a badge inside `container-type:inline-size` is
	// expected at the root's bottom-right corner.
	if strings.EqualFold(strings.TrimSpace(cs.GetProperty("container-type")), "size") {
		return true
	}
	return false
}

// isNoneValue reports whether a CSS value is absent or the `none` keyword.
func isNoneValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.EqualFold(v, "none")
}

// splitCSSKeywords splits a space/comma separated keyword list, lower-cased.
func splitCSSKeywords(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, strings.ToLower(f))
		}
	}
	return out
}

// layoutAbsolute sizes and positions an absolutely-positioned box.
func layoutAbsolute(box *ElementBox, cb *ElementBox, root *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	// Absolute containing block = the cb's PADDING box: inset:0/left:0 must
	// align to the padding-box edge and 100% resolves against the padding-box
	// size (chat-empty stretches to fill chat-messages' padded area, not its
	// content area — Edge x=429 w=601 vs content 441 w=577).
	cbWidth, cbHeight := cbPaddingBoxSizeForBox(cb, root, state)
	margin, padding, border := computeBoxModel(box, cbWidth, fontSizeOf(box))
	g.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
	g.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

	width, wAuto := resolveOffset(cs.Width, cbWidth)
	height, hAuto := resolveOffset(cs.Height, cbHeight)
	fs := fontSizeOf(box)
	minW, maxW, minWAuto, maxWAuto := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbWidth, fs)
	minH, maxH, minHAuto, maxHAuto := resolveMinMax(cs.MinHeight, cs.MaxHeight, cbHeight, fs)

	if wAuto {
		// CSS: an absolutely-positioned box with width:auto and no left/right
		// is shrink-to-fit = min(max-content, available) — .cache-ring-label
		// ("0%"+"缓存命中", ~40px) must NOT fill the 96px ring-wrap.
		// (left+right both set with width:auto is the stretch case, handled
		// below before this width is used.)
		content := intrinsicContentWidth(box, false)
		avail := cbWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
		if avail < 0 { avail = 0 }
		width = content
		if width > avail { width = avail }
		if width < 0 { width = 0 }
	}
	if isBorderBoxForBox(box) {
		// ★ box-sizing:border-box：width 含 border+padding → content 需扣减
		// （此前未扣，content 虚高 → BorderBoxWidth 多出 border，ring 圆环
		// 20x16 椭圆的根因；与 BFC layoutBoxContentForBox 的扣减一致）。
		width -= border.Horizontal() + padding.Horizontal()
		if width < 0 {
			width = 0
		}
	}
	width = clampSize(width, minW, maxW, minWAuto, maxWAuto)
	g.SetContentWidth(width)
	_ = minHAuto
	_ = maxHAuto

	// First pass: compute explicit height so we can position.
	if hAuto {
		height = layoutAbsoluteHeightForBox(box, state)
		// ★ auto 高度：layoutAbsoluteHeightForBox 返回 border-box 高度
		//   （BFC 内容高 + padding + border）。SetContentHeight 存的是
		//   content 高度，必须减 padding+border——否则 fixed/absolute
		//   弹层（dropdown/菜单/工具提示）高度虚高 padding+border：
		//   帮助菜单 dropdown 最后一项下方多 ~10px 空隙（wb-ui 234 vs
		//   浏览器 233）的根因。
		height -= border.Vertical() + padding.Vertical()
		if height < 0 {
			height = 0
		}
	} else if isBorderBoxForBox(box) {
		// ★ box-sizing:border-box：height 含 border+padding → content 需扣减
		// （与 BFC layoutBoxContentForBox 一致）。显式 height:0 + 非零 border
		// （.tri 边框三角形技巧）：扣减后 clamp 到 0，否则 contentH 为负 →
		// BorderBoxHeight 错误归零，三角形整体消失。
		height -= border.Vertical() + padding.Vertical()
		if height < 0 {
			height = 0
		}
	}
	height = clampSize(height, minH, maxH, minHAuto, maxHAuto)
	g.SetContentHeight(height)
	// ★ 显式 height 的 absolute/fixed 盒：尺寸已知，但子内容（弹层 option、
	// 菜单项等 in-flow 内容）仍须布局——此前只有 auto 高度走
	// layoutAbsoluteHeightForBox（内含 layoutBoxContentForBox），显式
	// height 的 select 下拉弹层子选项全部停留 0x0（点击命中空白、
	// 下拉「没有出现」表现）。
	if !hAuto {
		layoutBoxContentForBox(box, state)
	}

	// Calculate x from left/right.
	cbg := state.GeometryForBox(cb)
	left, leftAuto := resolveOffset(asLength(cs.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(cs.Properties["right"]), cbWidth)
	// CSS: when left AND right are both specified and width is auto, the
	// box stretches to fill the space (inset:0 → full-width overlay).
	if !leftAuto && !rightAuto && wAuto {
		stretchW := cbWidth - left - right - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
		if stretchW < 0 {
			stretchW = 0
		}
		width = stretchW
		g.SetContentWidth(width)
		wAuto = false
	}
	// CSS absolute positioning resolves against the containing block's
	// PADDING box (not the content box). left:0/inset:0 on an absolutely-
	// positioned child must align to the padding-box edge — Edge places
	// .chat-empty (absolute, inset:0) at x=429 (chat-messages padding-box
	// left), wb-ui put it at x=441 (content-box left + 12px padding).
	x := cbg.PaddingBoxLeft()
	cbIsFlex := cb.Style() != nil && (cb.Style().Display == style.DisplayFlex || cb.Style().Display == style.DisplayInlineFlex)
	cbRow := cbIsFlex && cb.Style().FlexDirection != "column" && cb.Style().FlexDirection != "column-reverse"
	// 静态位置：left/right（或 top/bottom）均为 auto 的轴上，absolute 盒应使用
	// 「假若它仍在正常流中」的位置（CSS2.1 §10.3.7 / §10.6.4），而不是贴包含块
	// padding box 边缘。
	staticX, staticY, hasStatic := staticPositionFor(box, root, state)
	switch {
	case !leftAuto && !rightAuto:
		x = cbg.PaddingBoxLeft() + left + margin.Left
	case !leftAuto:
		x = cbg.PaddingBoxLeft() + left + margin.Left
	case !rightAuto:
		x = cbg.PaddingBoxLeft() + cbWidth - right - margin.Right - g.BorderBoxWidth()
	default:
		x = cbg.PaddingBoxLeft() + margin.Left
		if hasStatic {
			x = staticX + margin.Left
		}
		// Static position of an absolutely-positioned child of a flex
		// container follows the flex alignment (CSS-FLEXBOX §5.1): the
		// cross axis uses align-items, the main axis uses justify-content.
		if cbIsFlex {
			align := cb.Style().AlignItems
			if !cbRow {
				align = cb.Style().JustifyContent
			}
			bw := g.BorderBoxWidth()
			switch align {
			case "center":
				x += (cbWidth - bw - margin.Horizontal()) / 2
			case "flex-end", "end", "right":
				x += cbWidth - bw - margin.Left - margin.Right
			}
		}
	}
	g.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)

	// Calculate y from top/bottom, using the known border-box height.
	top, topAuto := resolveOffset(asLength(cs.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(cs.Properties["bottom"]), cbHeight)
	// CSS: when top AND bottom are both specified and height is auto, the
	// box stretches to fill the space (e.g. position:fixed; inset:0 →
	// full-viewport overlay). Without this the overlay collapses to its
	// content height and ends up pinned to the top.
	if !topAuto && !bottomAuto && hAuto {
		stretchH := cbHeight - top - bottom - margin.Vertical() - border.Vertical() - padding.Vertical()
		if stretchH < 0 {
			stretchH = 0
		}
		height = stretchH
		g.SetContentHeight(height)
		hAuto = false
	}
	y := cbg.PaddingBoxTop()
	switch {
	case !topAuto && !bottomAuto:
		y = cbg.PaddingBoxTop() + top + margin.Top
	case !topAuto:
		y = cbg.PaddingBoxTop() + top + margin.Top
	case !bottomAuto:
		y = cbg.PaddingBoxTop() + cbHeight - bottom - margin.Bottom - g.BorderBoxHeight()
	default:
		y = cbg.PaddingBoxTop() + margin.Top
		if hasStatic {
			y = staticY + margin.Top
		}
		if cbIsFlex {
			justify := cb.Style().JustifyContent
			if !cbRow {
				justify = cb.Style().AlignItems
			}
			bh := g.BorderBoxHeight()
			switch justify {
			case "center":
				y += (cbHeight - bh - margin.Vertical()) / 2
			case "flex-end", "end", "bottom":
				y += cbHeight - bh - margin.Top - margin.Bottom
			}
		}
	}
	g.SetTopLeft(y, x)

	// Layout content now that position and size are fully known.
	layoutBoxContentForBox(box, state)
}

// staticPositionFor 返回 absolute/fixed 盒在**正常流**中本该处于的位置（绝对
// 坐标，不含该轴的 margin），用于 left/right（或 top/bottom）均为 auto 的轴：
// CSS2.1 §10.3.7 / §10.6.4 规定此时使用「静态位置」——即假设该盒保持 in-flow
// （position:static）时的位置，而不是贴到包含块的 padding box 边缘。
// 计算：父盒内容区原点 + 此前 in-flow 兄弟在垂直方向的累计外框高度
// （absolute-static-position 夹具四项：父的 padding 偏移 + 前导兄弟高度）。
// 返回 ok=false 表示没有静态位置可用（无父盒、父即布局根、或父是 flex/grid
// 容器——后者的静态位置由 align-items / justify-content 决定，见调用方专门分支），
// 调用方回退到包含块 padding box 边缘（旧行为）。
func staticPositionFor(box *ElementBox, root *ElementBox, state *LayoutState) (float64, float64, bool) {
	parent := box.parentBox
	if parent == nil || parent == root {
		return 0, 0, false
	}
	if parent.Style() != nil {
		switch parent.Style().Display {
		case style.DisplayFlex, style.DisplayInlineFlex,
			style.DisplayGrid, style.DisplayInlineGrid:
			return 0, 0, false
		}
	}
	pg := state.GeometryForBox(parent)
	if pg == nil {
		return 0, 0, false
	}
	sx := pg.ContentBoxLeft()
	sy := pg.ContentBoxTop()
	for _, sib := range parent.Children() {
		if sib == box {
			break
		}
		eb, ok := sib.(*ElementBox)
		if !ok || !eb.IsInFlow() || !eb.IsVisible() {
			continue
		}
		sg := state.GeometryForBox(eb)
		if sg == nil {
			continue
		}
		sy += sg.MarginBoxHeight()
	}
	return sx, sy, true
}

func cbPaddingBoxSizeForBox(cb *ElementBox, root *ElementBox, state *LayoutState) (float64, float64) {
	if cb == root {
		return state.ViewportWidth, state.ViewportHeight
	}
	g := state.GeometryForBox(cb)
	return g.PaddingBoxWidth(), g.PaddingBoxHeight()
}

func cbContentBoxSizeForBox(cb *ElementBox, root *ElementBox, state *LayoutState) (float64, float64) {
	if cb == root {
		return state.ViewportWidth, state.ViewportHeight
	}
	g := state.GeometryForBox(cb)
	return g.ContentWidth(), g.ContentHeight()
}

func shrinkToFitWidthForBox(box *ElementBox, cbWidth float64, margin, border, padding Edges) float64 {
	avail := cbWidth - margin.Horizontal() - border.Horizontal() - padding.Horizontal()
	if avail < 0 { return 0 }
	return avail
}

func layoutAbsoluteHeightForBox(box *ElementBox, state *LayoutState) float64 {
	layoutBoxContentForBox(box, state)
	g := state.GeometryForBox(box)
	h := g.BorderBoxHeight()
	if h == 0 {
		h = g.ContentHeight() + g.VerticalPadding() + g.VerticalBorder()
	}
	return h
}

func layoutBoxContentForBox(box *ElementBox, state *LayoutState) {
	ctx := contextFor(box, state)
	ctx.Layout(box, state)
}

func applyRelativeOffsetForBox(box *ElementBox, cbWidth, cbHeight float64, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)
	left, leftAuto := resolveOffset(asLength(cs.Properties["left"]), cbWidth)
	right, rightAuto := resolveOffset(asLength(cs.Properties["right"]), cbWidth)
	dx := 0.0
	if !leftAuto {
		dx = left
	} else if !rightAuto {
		dx = -right
	}
	top, topAuto := resolveOffset(asLength(cs.Properties["top"]), cbHeight)
	bottom, bottomAuto := resolveOffset(asLength(cs.Properties["bottom"]), cbHeight)
	dy := 0.0
	if !topAuto {
		dy = top
	} else if !bottomAuto {
		dy = -bottom
	}
	if dx == 0 && dy == 0 { return }
	g.SetTopLeft(g.Top()+dy, g.Left()+dx)

	// Recursively offset all descendant geometries so children follow.
	offsetDescendants(box, dx, dy, state)
}

// offsetDescendants recursively applies (dx, dy) to all descendant geometries.
func offsetDescendants(box *ElementBox, dx, dy float64, state *LayoutState) {
	for _, child := range box.Children() {
		switch c := child.(type) {
		case *ElementBox:
			cg := state.GeometryForBox(c)
			cg.SetTopLeft(cg.Top()+dy, cg.Left()+dx)
			offsetDescendants(c, dx, dy, state)
		case *InlineTextBox:
			// Offset each text segment's position.
			for i := range c.TextSegments {
				c.TextSegments[i].X += dx
				c.TextSegments[i].Y += dy
				c.TextSegments[i].LineY += dy
			}
		}
	}
}

func resolveOffset(l style.Length, cbSize float64) (float64, bool) {
	r := resolveLengthAuto(l, cbSize, 0)
	if r.Auto { return 0, true }
	return r.Value, false
}

func resolveOffsetsFromElementAttributes(_ *dom.Element) {}
