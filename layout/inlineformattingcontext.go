// Translation of: Source/WebCore/layout/formattingContexts/inline/InlineFormattingContext.cpp
package layout

import (
	"wb-ui/style"
)

type InlineFormattingContext struct {
	FormattingContextBase
}

func (c *InlineFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil { return }
	g := state.GeometryForBox(box)

	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	contentWidth := g.ContentWidth()

	type line struct {
		x, y, width float64
	}
	var lines []*line
	currentLine := &line{x: contentX, y: contentY, width: contentWidth}
	fs := fontSizeOf(box)
	lineHeight := fontLineGap(box)
	if lineHeight <= 0 { lineHeight = fs * 1.2 }

	for _, child := range box.Children() {
		switch cld := child.(type) {
		case *InlineTextBox:
			text := cld.Text()
			if text == "" { continue }
			runes := []rune(text)
			cursor := 0
			for cursor < len(runes) {
				// Skip whitespace runs (inter-word spaces)
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
				if currentLine.x+wordWidth > contentX+contentWidth && currentLine.x > contentX {
					lines = append(lines, currentLine)
					currentLine = &line{x: contentX, y: currentLine.y + lineHeight, width: contentWidth}
				}
				seg := TextSegment{
					Start: wordStart, Len: wordEnd - wordStart,
					X: currentLine.x, Y: currentLine.y,
					Width: wordWidth, Height: lineHeight,
					LineY: currentLine.y, LineHeight: lineHeight,
				}
				cld.TextSegments = append(cld.TextSegments, seg)
				currentLine.x += wordWidth
			}
		case *ElementBox:
			if !cld.IsInlineLevel() { continue }
			cldG := state.GeometryForBox(cld)
			childCtx := contextFor(cld, state)
			childCtx.Layout(cld, state)

			cldW := cldG.BorderBoxWidth()
			if currentLine.x+cldW > contentX+contentWidth && currentLine.x > contentX {
				lines = append(lines, currentLine)
				currentLine = &line{x: contentX, y: currentLine.y + lineHeight, width: contentWidth}
			}
			cldG.SetTopLeft(currentLine.y, currentLine.x)
			currentLine.x += cldW
		}
	}

	if currentLine.x > contentX {
		lines = append(lines, currentLine)
	}
	totalHeight := 0.0
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		totalHeight = (lastLine.y - contentY) + lineHeight
	}
	if totalHeight > g.ContentHeight() {
		g.SetContentHeight(totalHeight)
	}
}

// isInlineWhitespace reports whether r is a CSS whitespace character that
// separates words in inline layout.
func isInlineWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

var _ = style.DisplayInline
