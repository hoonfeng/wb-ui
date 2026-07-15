// Editor painter: renders the EditorView's state to a Skia Canvas.
//
// This module is the integration point between the editor package and wb-ui's
// rendering pipeline. It uses the graphics package's Canvas API to draw:
//   - Background
//   - Line number gutter
//   - Syntax-highlighted text (using tokens + HighlightStyle)
//   - Decorations (line highlights, marks)
//   - Selection (semi-transparent rectangles)
//   - Cursor (caret, with blink animation)
//
// Translation of: CodeMirror 6 — packages/view/src/index.ts (rendering part)
//                  wb-ui rendering/painter.go (style reference)

package editor

import (
	"wb-ui/platform/graphics"
)

// EditorColors holds the color scheme for the editor.
type EditorColors struct {
	// Background is the editor background color.
	Background graphics.Color
	// Foreground is the default text color.
	Foreground graphics.Color
	// LineNumber is the color for line numbers.
	LineNumber graphics.Color
	// ActiveLineNumber is the color for the active line's number.
	ActiveLineNumber graphics.Color
	// ActiveLine is the background color for the active line.
	ActiveLine graphics.Color
	// Selection is the selection highlight color (semi-transparent).
	Selection graphics.Color
	// Cursor is the caret color.
	Cursor graphics.Color
	// GutterBorder is the color of the line between gutter and content.
	GutterBorder graphics.Color
}

// DefaultDarkColors returns the default dark theme colors for the editor.
func DefaultDarkColors() EditorColors {
	return EditorColors{
		Background:        graphics.Color{R: 30, G: 30, B: 30, A: 255},
		Foreground:        graphics.Color{R: 212, G: 212, B: 212, A: 255},
		LineNumber:        graphics.Color{R: 133, G: 133, B: 133, A: 255},
		ActiveLineNumber:  graphics.Color{R: 200, G: 200, B: 200, A: 255},
		ActiveLine:        graphics.Color{R: 40, G: 40, B: 40, A: 255},
		Selection:         graphics.Color{R: 38, G: 79, B: 120, A: 120},
		Cursor:            graphics.Color{R: 255, G: 255, B: 255, A: 255},
		GutterBorder:      graphics.Color{R: 60, G: 60, B: 60, A: 255},
	}
}

// PaintEditor renders the editor view to the given canvas.
// The (x, y) offset is the top-left corner of the editor area within the
// canvas. The (w, h) dimensions clip the painting area.
func PaintEditor(canvas *graphics.Canvas, view *EditorView, x, y, w, h float64) {
	colors := DefaultDarkColors()
	PaintEditorWithColors(canvas, view, x, y, w, h, colors)
}

// PaintEditorWithColors renders the editor with a custom color scheme.
func PaintEditorWithColors(canvas *graphics.Canvas, view *EditorView, x, y, w, h float64, colors EditorColors) {
	if view == nil || w <= 0 || h <= 0 {
		return
	}

	canvas.Save()
	defer canvas.Restore()

	// Clip to the editor area.
	canvas.Clip(graphics.Rect{X: x, Y: y, Width: w, Height: h})

	// Paint background.
	canvas.FillRect(x, y, w, h, colors.Background)

	// Translate to the content origin.
	canvas.Translate(x, y)

	layout := view.Layout()

	// Paint line numbers.
	if view.showLineNumbers {
		paintLineNumbers(canvas, view, layout, colors)
	}

	// Paint active line background.
	paintActiveLine(canvas, view, layout, colors)

	// Paint selection.
	paintSelection(canvas, view, layout, colors)

	// Paint text with syntax highlighting.
	paintText(canvas, view, layout, colors)

	// Paint cursor (caret).
	paintCursor(canvas, view, layout, colors)
}

// paintLineNumbers draws the line number gutter.
func paintLineNumbers(canvas *graphics.Canvas, view *EditorView, layout EditorLayout, colors EditorColors) {
	doc := view.state.Doc
	totalLines := doc.Lines()

	// Determine the first and last visible lines.
	firstLine := int(view.scrollY / layout.LineHeight)
	if firstLine < 0 {
		firstLine = 0
	}
	lastLine := firstLine + int(view.height/layout.LineHeight) + 2
	if lastLine >= totalLines {
		lastLine = totalLines - 1
	}

	activeLine := -1
	if view.state.Selection.Main().Empty() {
		pos := view.state.Selection.Main().Head()
		activeLine = doc.LineAt(pos).Number - 1
	}

	for i := firstLine; i <= lastLine && i < totalLines; i++ {
		lineNum := i + 1 // 1-based
		text := intToString(lineNum)
		y := float64(i) * layout.LineHeight
		// Adjust for scroll.
		y -= view.scrollY

		color := colors.LineNumber
		if i == activeLine {
			color = colors.ActiveLineNumber
		}

		// Right-align the line number within the gutter.
		textWidth := graphics.MeasureText(layout.Font, text)
		textX := layout.LineNumberWidth - textWidth - 5 // 5px right padding
		// Draw text (y is top of line; need baseline).
		baselineY := y + layout.Ascent
		canvas.DrawText(textX, baselineY, text, layout.Font, color)
	}

	// Draw gutter border.
	gutterX := layout.LineNumberWidth
	canvas.FillRect(gutterX, 0, 1, view.height, colors.GutterBorder)
}

// paintActiveLine highlights the line containing the cursor.
func paintActiveLine(canvas *graphics.Canvas, view *EditorView, layout EditorLayout, colors EditorColors) {
	sel := view.state.Selection
	if !sel.Main().Empty() {
		return // only highlight when there's a caret (no selection)
	}
	pos := sel.Main().Head()
	line := view.state.Doc.LineAt(pos)
	lineIndex := line.Number - 1
	y := float64(lineIndex) * layout.LineHeight - view.scrollY
	canvas.FillRect(layout.ContentX, y, view.width-layout.ContentX, layout.LineHeight, colors.ActiveLine)
}

// paintSelection draws the selection highlight rectangles.
func paintSelection(canvas *graphics.Canvas, view *EditorView, layout EditorLayout, colors EditorColors) {
	sel := view.state.Selection
	for _, r := range sel.Ranges() {
		if r.Empty() {
			continue
		}
		from := r.From()
		to := r.To()
		doc := view.state.Doc

		// Paint selection for each line spanned by the range.
		startLine := doc.LineAt(from)
		endLine := doc.LineAt(to)

		for lineNum := startLine.Number; lineNum <= endLine.Number; lineNum++ {
			line := doc.LineN(lineNum)
			y := float64(lineNum-1) * layout.LineHeight - view.scrollY

			// Calculate the X range for this line.
			lineFrom := line.From
			lineTo := line.To
			if lineNum == startLine.Number {
				lineFrom = from
			}
			if lineNum == endLine.Number {
				lineTo = to
			}

			// Measure text widths.
			startRunes := []rune(line.Text)[:lineFrom-line.From]
			endRunes := []rune(line.Text)[:lineTo-line.From]
			startX := graphics.MeasureText(layout.Font, string(startRunes))
			endX := graphics.MeasureText(layout.Font, string(endRunes))

			if from < line.From && to > line.To {
				// Full line selection.
				startX = 0
				endX = graphics.MeasureText(layout.Font, line.Text)
			}

			canvas.FillRect(layout.ContentX+startX, y, endX-startX, layout.LineHeight, colors.Selection)
		}
	}
}

// paintText draws the document text with syntax highlighting.
func paintText(canvas *graphics.Canvas, view *EditorView, layout EditorLayout, colors EditorColors) {
	doc := view.state.Doc
	totalLines := doc.Lines()

	// Determine the visible line range.
	firstLine := int(view.scrollY / layout.LineHeight)
	if firstLine < 0 {
		firstLine = 0
	}
	lastLine := firstLine + int(view.height/layout.LineHeight) + 2
	if lastLine >= totalLines {
		lastLine = totalLines - 1
	}

	// Get tokens for syntax highlighting.
	tokens := view.Tokens()

	for i := firstLine; i <= lastLine && i < totalLines; i++ {
		line := doc.LineN(i + 1) // 1-based
		y := float64(i) * layout.LineHeight - view.scrollY
		baselineY := y + layout.Ascent

		if line.Text == "" {
			continue
		}

		// Get tokens for this line.
		var lineTokens []Token
		if i < len(tokens) {
			lineTokens = tokens[i].Tokens
		}

		if len(lineTokens) == 0 || view.theme == nil {
			// No tokens or no theme — draw plain text.
			canvas.DrawText(layout.ContentX, baselineY, line.Text, layout.Font, colors.Foreground)
			continue
		}

		// Draw each token with its highlight color.
		lineRunes := []rune(line.Text)
		for _, tok := range lineTokens {
			if tok.StartIndex >= len(lineRunes) || tok.EndIndex > len(lineRunes) {
				continue
			}
			text := string(lineRunes[tok.StartIndex:tok.EndIndex])
			if text == "" {
				continue
			}

			// Calculate the X position: measure the text before this token.
			prefix := string(lineRunes[:tok.StartIndex])
			tokenX := layout.ContentX + graphics.MeasureText(layout.Font, prefix)

			// Get the highlight style for this token.
			style := view.theme.GetByString(tok.ScopeString())
			color := colors.Foreground
			font := layout.Font
			if style.Color.A != 0 {
				// Convert editor.Color to graphics.Color.
				color = graphics.Color{
					R: style.Color.R, G: style.Color.G,
					B: style.Color.B, A: style.Color.A,
				}
			}
			if style.Bold {
				font.Weight = 700
			}
			if style.Italic {
				font.Style = "italic"
			}

			canvas.DrawText(tokenX, baselineY, text, font, color)
		}
	}
}

// paintCursor draws the caret (blinking vertical line).
func paintCursor(canvas *graphics.Canvas, view *EditorView, layout EditorLayout, colors EditorColors) {
	sel := view.state.Selection
	if !sel.Main().Empty() {
		return // no caret when there's a selection
	}

	pos := sel.Main().Head()
	x, y := PosToXY(layout, view.state.Doc, pos)

	// Adjust for content offset and scroll.
	x += layout.ContentX - view.scrollX
	y -= view.scrollY

	// Draw a 2px wide caret.
	caretWidth := 2.0
	canvas.FillRect(x-caretWidth/2, y, caretWidth, layout.LineHeight, colors.Cursor)
}

// intToString converts an integer to its decimal string representation.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
