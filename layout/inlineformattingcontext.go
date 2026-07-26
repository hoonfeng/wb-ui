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

	// Determine text-align.
	textAlign := style.TextAlignStart
	if cs != nil {
		textAlign = cs.TextAlign
	}

	type lineInfo struct {
		y, contentX float64    // line Y position and content start X
		segStart    int        // index into pending (first seg on this line)
		widthUsed   float64    // actual used width (contentX .. last-right-edge)
	}
	var lines []lineInfo
	var pending []pendingSeg

	currentLine := lineInfo{y: contentY, contentX: contentX, segStart: 0, widthUsed: 0}

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
				if nextX+wordWidth > contentWidth && currentLine.widthUsed > 0 && cs.WhiteSpace != style.WhiteSpaceNoWrap {
					// Line wrap: record line, start new line.
					lines = append(lines, currentLine)
					currentLine = lineInfo{
						y: currentLine.y + lineHeight,
						contentX: contentX,
						segStart: len(pending),
						widthUsed: 0,
					}
					firstWord = true
					nextX = 0
				}
				pending = append(pending, pendingSeg{
					textBox: cld,
					seg: TextSegment{
						Start: wordStart, Len: wordEnd - wordStart,
						X: contentX + nextX, Y: currentLine.y + centeringOffset,
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
			cldG.SetTopLeft(currentLine.y+centeringOffset, contentX+currentLine.widthUsed)
			childCtx := contextFor(cld, state)
			childCtx.Layout(cld, state)

			// If the inline-level element has a definite CSS width/height, ensure
			// the geometry reflects it. This is critical for replaced elements
			// (input, select, img) whose layout box has no children and thus may
			// not set content width/height during context.Layout().
			if cldG.ContentWidth() <= 0 {
				cs := cld.Style()
				fs := fontSizeOf(cld)
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
				// Fallback: for replaced input/button elements without explicit CSS
				// width, derive width from the HTML value attribute text.
				if cldG.ContentWidth() <= 0 && cld.IsReplaced() {
					if el := cld.Element(); el != nil && el.NodeName() == "INPUT" {
						val := el.GetAttribute("value")
						if val == "" {
							// Default labels for submit/reset if no value given.
							switch el.GetAttribute("type") {
							case "submit":
								val = "Submit"
							case "reset":
								val = "Reset"
							case "button":
								val = "Button"
							}
						}
						if val != "" {
							textW := measureText(cld, val)
							if textW > 0 {
								cldG.SetContentWidth(textW)
								// Also set content height if still 0, using line height.
								if cldG.ContentHeight() <= 0 {
									lineH := fontLineGap(cld)
									if lineH <= 0 { lineH = fs * 1.2 }
									cldG.SetContentHeight(lineH)
								}
							}
						}
					}
				}
			}
			if cldG.ContentHeight() <= 0 {
				cs := cld.Style()
				fs := fontSizeOf(cld)
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

			// Compute inline child's content width from text segments.
			// Without this, cldW=0 and subsequent text on same line overlaps.
			if cldG.BorderBoxWidth() <= 0 {
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
			cldW := cldG.BorderBoxWidth()
			if currentLine.widthUsed+cldW > contentWidth && currentLine.widthUsed > 0 && cs.WhiteSpace != style.WhiteSpaceNoWrap {
				lines = append(lines, currentLine)
				currentLine = lineInfo{
					y: currentLine.y + lineHeight,
					contentX: contentX,
					segStart: len(pending),
					widthUsed: 0,
				}
				cldG.SetTopLeft(currentLine.y+centeringOffset, contentX+currentLine.widthUsed)
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

	// Apply text-align adjustment to pending segments.
	if textAlign != style.TextAlignLeft && len(lines) > 0 {
		// For each completed line (all but possibly the last in-progress one),
		// compute the shift and apply to segments whose lineIdx matches.
		for li, ln := range lines {
			var used float64
			if li < len(lines)-1 {
				// Get width used from the next line's segStart.
				used = 0
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
				shift = (contentWidth - used) / 2
			case style.TextAlignRight, style.TextAlignEnd:
				shift = contentWidth - used
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
						if ebG.Top() >= ln.y && ebG.Top() < ln.y+lineHeight {
							ebG.SetTopLeft(ebG.Top(), ebG.Left()+shift)
						}
					}
				}
			}
		}
	}

	// Flush pending segments to their InlineTextBoxes.
	for _, ps := range pending {
		ps.textBox.TextSegments = append(ps.textBox.TextSegments, ps.seg)
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
