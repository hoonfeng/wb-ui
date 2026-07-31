// Translation of: Source/WebCore/layout/formattingContexts/inline/InlineFormattingContext.cpp
package layout

import (
	"math"

	"wb-ui/style"
)

type InlineFormattingContext struct {
	FormattingContextBase
}

// pendingSeg holds a segment before its X position is finalized.
type pendingSeg struct {
	textBox *InlineTextBox
	seg     TextSegment
	lineIdx int
}

func (c *InlineFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)

	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	boxHeight := g.ContentHeight()
	contentWidth := g.ContentWidth()
	fs := fontSizeOf(box)
	lineHeight := fontLineGap(box)
	if lineHeight <= 0 { lineHeight = fs * 1.2 }
	// Use CSS line-height if explicitly set (overrides font metrics).
	cssLH := cssLineHeight(box)
	if cssLH > 0 {
		lineHeight = cssLH
	}
	// Text segment height should be the actual font metrics height, not CSS
	// line-height. The line-height determines line spacing and centering.
	textHeight := fontLineGap(box)
	if textHeight <= 0 { textHeight = fs * 1.2 }
	// Compute the vertical centering offset: when line-height > font metrics,
	// shift text down so it appears vertically centered within the line.
	centeringOffset := 0.0
	if cssLH > 0 && textHeight < cssLH {
		centeringOffset = (cssLH - textHeight) / 2
	}

	// When contentWidth is auto (derived from intrinsic text width), widen it
	// slightly to prevent floating-point discrepancies from triggering unwanted
	// line wraps inside flex items.
	//
	// Only auto-width boxes get this widening: boxes with an explicit CSS width
	// (or width:auto) should respect their declared width so that overflow
	// clipping (overflow:hidden, text-overflow:ellipsis) works correctly.
	{
		hasExplicitWidth := cs != nil && cs.Width.Unit != "" && cs.Width.Unit != "auto"
		if !hasExplicitWidth {
			// Auto-width expansion: only expand for auto-width inline-level
			// boxes (e.g. span, inline-block). Block-level children get their
			// width from the parent BFC and must not be expanded, otherwise
			// text would not wrap (causing overflow beyond the container).
			if box.IsInlineLevel() {
				allText := ""
				for _, child := range box.Children() {
					if tb, ok := child.(*InlineTextBox); ok {
						allText += tb.Text()
					}
				}
				if totalW := measureText(box, allText); totalW > 0 {
					if contentWidth < totalW+20 {
						// This box has auto-width based on text content.
						// Widen slightly to prevent float-epsilon line wraps.
						contentWidth = totalW + 20
					}
				}
			}
		}
	}

	// Determine text-align.
	textAlign := style.TextAlignStart
	if cs != nil {
		textAlign = cs.TextAlign
	}
	// Reference width for text-align: the container's content-box width
	// (captured before we widen it to the text extent below). Centering/right
	// alignment must be computed against the container, not the text width.
	containerWidth := contentWidth

	// Get float context for text wrapping around floats.
	fc := state.currentFloatContext()

	// availableLineWidth returns the usable width for a line at the given page Y.
	// When floats intrude at this Y, the line is narrowed accordingly.
	availableLineWidth := func(lineY float64) (lineContentX, lineWidth float64) {
		if fc != nil {
			// fcY is in FC-relative coordinates. Since lineY is absolute
			// (contentY = g.ContentBoxTop() is absolute), convert to FC-relative.
			fcY := lineY - fc.originY
			left, right := fc.contentEdgesAt(fcY)
			// Shift from FC-relative to container-relative coordinates.
			// contentEdgesAt returns edges relative to fc.originX, but the
			// container's content box starts at contentX. The offset between
			// the two must be applied before clamping to the container bounds.
			if fc.originX != contentX {
				dx := contentX - fc.originX
				left += dx
				right += dx
			}
			if left < contentX {
				left = contentX
			}
			if right > contentX+contentWidth {
				right = contentX + contentWidth
			}
			return left, right - left
		}
		return contentX, contentWidth
	}

	type lineInfo struct {
		y, contentX float64    // line Y position and content start X
		segStart    int        // index into pending (first seg on this line)
		widthUsed   float64    // actual used width (contentX .. last-right-edge)
		availWidth  float64    // available width for this line (adjusted for floats)
	}

	// Initialize first line with float-aware available width.
	lineCx, lineCw := availableLineWidth(contentY)
	var lines []lineInfo
	var pending []pendingSeg

	currentLine := lineInfo{y: contentY, contentX: lineCx, segStart: 0, widthUsed: 0, availWidth: lineCw}

	for _, child := range box.Children() {
		switch cld := child.(type) {
		case *InlineTextBox:
			text := cld.Text()
			if text == "" { continue }
			// Clear segments from any previous layout pass (e.g. auto-height
			// re-layout in flex formatting context).
			cld.TextSegments = cld.TextSegments[:0]
			runes := []rune(text)
			spaceWidth := measureText(box, " ")
			if spaceWidth <= 0 { spaceWidth = measureText(box, " ") }
			cursor := 0
			firstWord := true
			// If this text node starts with whitespace and there's already
			// content on the current line, advance by spaceWidth. This handles
			// both pure-whitespace text nodes (" " between inline elements)
			// and leading-whitespace text nodes (" main" after </span>).
			if len(runes) > 0 && isInlineWhitespace(runes[0]) && currentLine.widthUsed > 0 {
				currentLine.widthUsed += spaceWidth
			}
			for cursor < len(runes) {
				// Skip leading whitespace.
				for cursor < len(runes) && isInlineWhitespace(runes[cursor]) {
					cursor++
				}
				if cursor >= len(runes) { break }
				wordStart := cursor
				for cursor < len(runes) && !isInlineWhitespace(runes[cursor]) {
					cursor++
				}
				wordEnd := cursor
				word := string(runes[wordStart:wordEnd])
				wordWidth := measureText(box, word)

				// Compute the x where this word would be placed.
				nextX := currentLine.widthUsed
				if !firstWord {
					nextX += spaceWidth
				}
				if nextX+wordWidth > currentLine.availWidth && currentLine.widthUsed > 0 && cs.WhiteSpace != style.WhiteSpaceNoWrap {
					// Line wrap: record line, start new line with float-aware width.
					lines = append(lines, currentLine)
					newY := currentLine.y + lineHeight
					newCx, newCw := availableLineWidth(newY)
					currentLine = lineInfo{
						y: newY,
						contentX: newCx,
						segStart: len(pending),
						widthUsed: 0,
						availWidth: newCw,
					}
					firstWord = true
					nextX = 0
				}
				pending = append(pending, pendingSeg{
					textBox: cld,
					seg: TextSegment{
						Start: wordStart, Len: wordEnd - wordStart,
						X: currentLine.contentX + nextX, Y: currentLine.y + centeringOffset,
						Width: wordWidth, Height: textHeight,
						LineY: currentLine.y, LineHeight: lineHeight,
					},
					lineIdx: len(lines), // current (in-progress) line
				})
				currentLine.widthUsed = nextX + wordWidth
				firstWord = false
			}
			case *ElementBox:
			if !cld.IsInlineLevel() { continue }
			cldG := state.GeometryForBox(cld)

			// Compute margin/padding/border BEFORE the child's Layout so the
			// child's formatting context (e.g. BFC.Layout for inline-block) sees
			// the correct ContentBoxLeft/ContentBoxTop. Without this,
			// padding/border are treated as zero and text inside the child gets
			// positioned at the child's border-box top-left instead of its
			// content-box origin.
			fs := fontSizeOf(cld)
			margin, padding, border := computeBoxModelForBox(cld, contentWidth, fs)
			cldG.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
			cldG.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
			cldG.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
			// Horizontal margins shift the child and consume line space;
			// margin-top lowers the child inside the line box.
			cldG.SetTopLeft(currentLine.y+centeringOffset+margin.Top, currentLine.contentX+currentLine.widthUsed+margin.Left)

			// Set CSS width if definite BEFORE Layout so box-sizing:border-box
			// correctly limits the content width used by the child's Layout.
			if cldG.ContentWidth() <= 0 {
				cs := cld.Style()
				if cs != nil {
					if w, ok := definiteWidth(cs.Width, contentWidth, fs); ok && w > 0 {
						if isBorderBoxForBox(cld) {
							b := cldG.BorderLeft() + cldG.BorderRight()
							p := cldG.PaddingLeft() + cldG.PaddingRight()
							cldG.SetContentWidth(w - b - p)
						} else {
							cldG.SetContentWidth(w)
						}
					}
				}
			}

			childCtx := contextFor(cld, state)
			childCtx.Layout(cld, state)

			// Re-apply explicit width: the child's Layout (IFC for inline
			// content) collapses the box to content width; a definite CSS
			// width on an inline-block must win.
			if cs := cld.Style(); cs != nil {
				if w, ok := definiteWidth(cs.Width, contentWidth, fs); ok && w > 0 {
					if isBorderBoxForBox(cld) {
						b := cldG.BorderLeft() + cldG.BorderRight()
						p := cldG.PaddingLeft() + cldG.PaddingRight()
						cldG.SetContentWidth(w - b - p)
					} else {
						cldG.SetContentWidth(w)
					}
				}
			}

			// Fallback: for replaced input/button elements without explicit CSS
			// width, derive width from the HTML value attribute text. Text
			// inputs get the browser default ~20ch width (Edge reports ~177px
			// at 13.33px font) regardless of the value attribute.
			if cldG.ContentWidth() <= 0 && cld.IsReplaced() {
				if el := cld.Element(); el != nil && el.NodeName() == "INPUT" {
					typ := el.GetAttribute("type")
					val := el.GetAttribute("value")
					switch typ {
					case "text", "password", "search", "email", "url", "tel",
						"number", "date", "time", "month", "week", "datetime-local":
						// Browser default text-field width (~20ch at 13.33px;
						// Edge reports 177px border-box). The input UA styles
						// set box-sizing:border-box with 4px padding + 1px
						// borders, so the content width is 177-10=167.
						cldG.SetContentWidth(167)
					case "submit":
						val = "Submit"
					case "reset":
						val = "Reset"
					case "button":
						val = "Button"
					}
					if val != "" && cldG.ContentWidth() <= 0 {
						textW := measureText(cld, val)
						if textW > 0 {
							cldG.SetContentWidth(textW)
						}
					}
					if cldG.ContentWidth() > 0 && cldG.ContentHeight() <= 0 {
						lineH := fontLineGap(cld)
						if lineH <= 0 {
							lineH = fs * 1.2
						}
						cldG.SetContentHeight(lineH)
					}
				} else if el := cld.Element(); el != nil {
					// SELECT / TEXTAREA default sizes (browser defaults):
					// select ≈ 45px wide, textarea ≈ 2 columns x 2 rows.
					switch el.LocalName() {
					case "select":
						cldG.SetContentWidth(45)
						if cldG.ContentHeight() <= 0 {
							cldG.SetContentHeight(fs * 1.4)
						}
					case "textarea":
						chW := measureText(cld, "0")
						if chW <= 0 {
							chW = fs * 0.5
						}
						// ~20 cols x 2 rows, plus padding.
						cldG.SetContentWidth(20*chW + 4)
						cldG.SetContentHeight(2*fontLineGap(cld) + 4)
					}
				}
			}
			if cldG.ContentHeight() <= 0 {
				cs := cld.Style()
				if cs != nil {
					if h, ok := definiteHeight(cs.Height, contentWidth, fs); ok && h > 0 {
						if isBorderBoxForBox(cld) {
							b := cldG.BorderTop() + cldG.BorderBottom()
							p := cldG.PaddingTop() + cldG.PaddingBottom()
							cldG.SetContentHeight(h - b - p)
						} else {
							cldG.SetContentHeight(h)
						}
					}
				}
			}
			// A definite height on an inline-block/replaced child wins over
			// the content-derived height (e.g. button height:34px). Plain
			// inline (span) heights have no layout effect per CSS.
			if cld.IsInline() && cld.Style().Display == style.DisplayInlineBlock {
				if cs := cld.Style(); cs != nil {
					if h, ok := definiteHeight(cs.Height, contentWidth, fs); ok && h > 0 {
						if isBorderBoxForBox(cld) {
							b := cldG.BorderTop() + cldG.BorderBottom()
							p := cldG.PaddingTop() + cldG.PaddingBottom()
							cldG.SetContentHeight(h - b - p)
						} else {
							cldG.SetContentHeight(h)
						}
					}
				}
			}

			// If content height is still 0 (no CSS height), use line height.
			if cldG.ContentHeight() <= 0 {
				lineH := fontLineGap(cld)
				if lineH <= 0 { lineH = fs * 1.2 }
				cldG.SetContentHeight(lineH)
			}

			// vertical-align (CSS 2.1 §10.8): inline-block/replaced children
			// with vertical-align:middle are centered against the line box.
			// The child was initially placed at currentLine.y+centeringOffset;
			// shift it so its vertical center aligns with the line's center.
			{
				va := ""
				if cldCS := cld.Style(); cldCS != nil {
					va = cldCS.Properties["vertical-align"]
				}
				if va == "middle" {
					// The child's vertical margin participates in the line
					// box: line box height = child border-box + margins.
					childBH := cldG.BorderBoxHeight()
					childH := childBH + margin.Top + margin.Bottom
					lineH := lineHeight
					if childH > lineH {
						lineH = childH
					}
					if childH > 0 && lineH > 0 {
						// Line box: from currentLine.y to currentLine.y+lineH.
						// Place child's middle at line's middle, then shift
						// by margin-top so the margin stays outside.
						lineTop := currentLine.y
						targetTop := lineTop + (lineH-childH)/2 + margin.Top
						cldG.SetTopLeft(targetTop, cldG.Left())
					}
				}
			}

			// Compute inline child's content width from text segments.
			// Without this, cldW=0 and subsequent text on same line overlaps.
			if cldG.ContentWidth() <= 0 {
				if cw := computeInlineContentWidth(cld, state); cw > 0 {
					cldG.SetContentWidth(cw)
				}
			}
			// Expand lineHeight to match the inline child's actual height.
			// The child (e.g. anonymous wrapper around text) may use a
			// different font-size (inherited or from CSS), producing a
			// taller line than fontLineGap(box) estimates.
			if cldBH := cldG.BorderBoxHeight(); cldBH > lineHeight {
				lineHeight = cldBH
			}
			cldW := cldG.BorderBoxWidth() + margin.Horizontal()
			if currentLine.widthUsed+cldW > currentLine.availWidth && currentLine.widthUsed > 0 && cs.WhiteSpace != style.WhiteSpaceNoWrap {
				lines = append(lines, currentLine)
				newY := currentLine.y + lineHeight
				newCx, newCw := availableLineWidth(newY)
				currentLine = lineInfo{
					y: newY,
					contentX: newCx,
					segStart: len(pending),
					widthUsed: 0,
					availWidth: newCw,
				}
				cldG.SetTopLeft(currentLine.y+centeringOffset, currentLine.contentX+currentLine.widthUsed)
			}
			// Apply relative offset to inline-level elements that are
			// relatively positioned (e.g. position:relative with top/left).
			if cld.IsRelativelyPositioned() && cld.IsInFlow() {
				applyRelativeOffsetForBox(cld, contentWidth, lineHeight, state)
			}
			currentLine.widthUsed += cldW
		}
	}

	// Append the final in-progress line.
	if currentLine.widthUsed > 0 {
		lines = append(lines, currentLine)
	}

	// Compute container height from line count.
	totalHeight := 0.0
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		totalHeight = (lastLine.y - contentY) + lineHeight
	}

	// Update content width to match the actual text content width. This
	// ensures the layout box geometry reflects the real text extent so that
	// syncOne sets the correct frame width. Without this, flex items with
	// overflow:hidden would clip the text because their frame is narrower
	// than the actual text content.
	var totalWidth float64
	for _, ps := range pending {
		right := ps.seg.X + ps.seg.Width - contentX
		if right > totalWidth {
			totalWidth = right
		}
	}
	// Also include inline ElementBox children's widths (e.g. span > anonymous
	// wrapper > text). These children may have their own content width updated
	// by their IFC, but the parent box's width needs to encompass them.
	for _, child := range box.Children() {
		if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
			cg := state.GeometryForBox(eb)
			if right := cg.Left() + cg.BorderBoxWidth() - contentX; right > totalWidth {
				totalWidth = right
			}
		}
	}
	if totalWidth > 0 {
		g.SetContentWidth(totalWidth)
	}

	// Apply text-align adjustment AFTER setting content width so the shift
	// is computed against the final (non-expanded) content width rather than
	// the expanded auto-width estimate. Without this, text-align:center inside
	// buttons lands the text at the wrong X because contentWidth was widened
	// by +20 during auto-width expansion but then corrected to totalWidth.
	contentWidth = totalWidth
	if textAlign != style.TextAlignLeft && len(lines) > 0 {
		for li, ln := range lines {
			var used float64
			if li < len(lines)-1 {
				for i := ln.segStart; i < lines[li+1].segStart && i < len(pending); i++ {
					s := pending[i].seg
					r := s.X + s.Width - contentX
					if r > used { used = r }
				}
			} else {
				used = ln.widthUsed
			}

			var shift float64
			switch textAlign {
			case style.TextAlignCenter:
				shift = (containerWidth - used) / 2
			case style.TextAlignRight, style.TextAlignEnd:
				shift = containerWidth - used
			}
			if shift > 0 {
				for i := ln.segStart; i < len(pending); i++ {
					if pending[i].lineIdx != li && i >= (func() int { if li+1 < len(lines) { return lines[li+1].segStart }; return len(pending) })() {
						break
					}
					pending[i].seg.X += shift
				}
				// Also shift the inline ElementBox children on this line.
				for _, child := range box.Children() {
					if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
						ebG := state.GeometryForBox(eb)
						ebG.SetTopLeft(ebG.Top(), ebG.Left()+shift)
					}
				}
			}
		}
	}

	// Vertically center single-line content when box is taller than the text.
	if len(lines) <= 1 && totalHeight > 0 && len(pending) > 0 {
		shift := (boxHeight - totalHeight) / 2
		if shift < 0 {
			shift = 0 // text taller than box; keep at top
		}
		if shift > 0 {
			for i := range pending {
				pending[i].seg.Y += shift
			}
			// Also shift inline ElementBox children on this line.
			for _, child := range box.Children() {
				if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
					ebG := state.GeometryForBox(eb)
					ebG.SetTopLeft(ebG.Top()+shift, ebG.Left())
				}
			}
		}
	}

	// Flush pending segments to their InlineTextBoxes. Must run AFTER the
	// text-align and vertical-centering adjustments above, otherwise the
	// segments copied into TextSegments would keep their pre-adjustment X/Y.
	for _, ps := range pending {
		ps.textBox.TextSegments = append(ps.textBox.TextSegments, ps.seg)
	}

	g.SetContentHeight(math.Max(boxHeight, totalHeight))
}

// isInlineWhitespace reports whether r is a CSS whitespace character that
// separates words in inline layout.
func isInlineWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// computeInlineContentWidth computes the inline content width of an ElementBox
// from its text segments. InlineFormattingContext.Layout sets ContentHeight but
// not ContentWidth, so inline ElementBox children would get zero width causing
// subsequent text to overlap.
func computeInlineContentWidth(box *ElementBox, state *LayoutState) float64 {
	g := state.GeometryForBox(box)
	base := g.ContentBoxLeft()
	maxRight := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				for _, seg := range itb.TextSegments {
					r := seg.X + seg.Width
					if r > maxRight {
						maxRight = r
					}
				}
			}
			if eb, ok := c.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)
	if maxRight <= base {
		return 0
	}
	return maxRight - base
}

var _ = style.DisplayInline
