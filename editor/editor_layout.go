// Editor layout calculations: line height, character width, line number
// width, and coordinate conversions (rune offset ↔ pixel coordinates).
//
// This module bridges the editor's document model (rune offsets) with the
// rendering pipeline (pixel coordinates). It uses the graphics package's
// Skia-backed font metrics for accurate measurements.
//
// Translation of: CodeMirror 6 — packages/view/src/heightmap.ts (simplified)
//                  packages/view/src/dom.ts (geometry helpers)

package editor

import (
	"math"

	"wb-ui/platform/graphics"
)

// EditorLayout holds the geometric properties of the editor content area.
// It is recalculated when the font, line count, or viewport size changes.
type EditorLayout struct {
	// LineHeight is the height of each line in pixels (ascent + descent +
	// line gap, matching CSS line-height: normal).
	LineHeight float64

	// Ascent is the font's ascent (baseline to top).
	Ascent float64
	// Descent is the font's descent (baseline to bottom).
	Descent float64

	// CharWidth is the width of a single character (for monospace fonts,
	// all characters have the same width). For proportional fonts, this
	// is the average width used for estimating column positions.
	CharWidth float64

	// LineNumberWidth is the width of the line number gutter in pixels.
	// If line numbers are disabled, this is 0.
	LineNumberWidth float64

	// ContentX is the X offset where text content starts (after the gutter).
	ContentX float64
	// ContentY is the Y offset where text content starts (after any top
	// padding).
	ContentY float64

	// LineCount is the total number of lines in the document.
	LineCount int

	// Font is the font used for rendering text.
	Font graphics.Font
}

// MeasureLayout calculates the layout properties for the given font and
// line count. The gutterWidth is 0 if line numbers are disabled, or the
// width of the line number area if enabled.
func MeasureLayout(font graphics.Font, lineCount int, showLineNumbers bool) EditorLayout {
	ascent := graphics.GlobalFontAscent(font)
	descent := graphics.GlobalFontDescent(font)
	lineGap := graphics.GlobalFontLineGap(font)
	lineHeight := ascent + descent + lineGap
	if lineHeight <= 0 {
		lineHeight = font.Size * 1.2
	}

	charWidth := graphics.MeasureText(font, "M")
	if charWidth <= 0 {
		charWidth = font.Size * 0.6 // fallback estimate
	}

	layout := EditorLayout{
		LineHeight: lineHeight,
		Ascent:     ascent,
		Descent:    descent,
		CharWidth:  charWidth,
		LineCount:  lineCount,
		Font:       font,
	}

	if showLineNumbers {
		// Line number width = width of the widest line number + padding.
		digits := numDigits(lineCount)
		if digits < 3 {
			digits = 3 // minimum 3 digits for aesthetics
		}
		sample := ""
		for i := 0; i < digits; i++ {
			sample += "8"
		}
		layout.LineNumberWidth = graphics.MeasureText(font, sample) + 10 // +10px padding
		layout.ContentX = layout.LineNumberWidth
	} else {
		layout.ContentX = 0
	}

	return layout
}

// numDigits returns the number of decimal digits in n (minimum 1).
func numDigits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// PosToXY converts a rune offset to pixel coordinates relative to the
// editor content area. Returns (x, y) where y is the top of the line
// (not the baseline).
//
// The position is assumed to be within the document. Out-of-range positions
// are clamped.
func PosToXY(layout EditorLayout, doc Text, pos int) (x, y float64) {
	if pos < 0 {
		pos = 0
	}
	if pos > doc.Length() {
		pos = doc.Length()
	}

	line := doc.LineAt(pos)
	// Y = line number * line height
	lineIndex := line.Number - 1 // 0-based
	y = float64(lineIndex) * layout.LineHeight

	// X = width of text from line start to pos
	col := pos - line.From
	if col < 0 {
		col = 0
	}
	// Measure the prefix of the line text up to the column position.
	lineRunes := []rune(line.Text)
	if col > len(lineRunes) {
		col = len(lineRunes)
	}
	prefix := string(lineRunes[:col])
	if prefix == "" {
		x = 0
	} else {
		x = graphics.MeasureText(layout.Font, prefix)
	}

	return x, y
}

// XYToPos converts pixel coordinates (relative to the editor content area)
// to a rune offset. The position is snapped to the nearest character
// boundary.
func XYToPos(layout EditorLayout, doc Text, x, y float64) int {
	// Determine the line number from Y.
	if layout.LineHeight <= 0 {
		return 0
	}
	lineIndex := int(math.Floor(y / layout.LineHeight))
	if lineIndex < 0 {
		lineIndex = 0
	}
	totalLines := doc.Lines()
	if lineIndex >= totalLines {
		lineIndex = totalLines - 1
		if lineIndex < 0 {
			return 0
		}
	}

	// Get the line at this index.
	line := doc.LineN(lineIndex + 1) // LineN is 1-based
	lineRunes := []rune(line.Text)

	// Binary search for the character whose left edge is closest to x.
	// For monospace fonts, we could just do x / charWidth, but measuring
	// gives accurate results for proportional fonts too.
	if x <= 0 || len(lineRunes) == 0 {
		return line.From
	}

	// Measure progressively to find the best position.
	bestPos := line.From
	bestX := 0.0
	for i := 1; i <= len(lineRunes); i++ {
		w := graphics.MeasureText(layout.Font, string(lineRunes[:i]))
		if w > x {
			// Check whether this position or the previous is closer.
			if (w - x) < (x - bestX) {
				return line.From + i
			}
			return bestPos
		}
		bestX = w
		bestPos = line.From + i
	}
	return bestPos
}

// LineAtY returns the 0-based line index at the given Y coordinate.
func LineAtY(layout EditorLayout, y float64) int {
	if layout.LineHeight <= 0 {
		return 0
	}
	idx := int(math.Floor(y / layout.LineHeight))
	if idx < 0 {
		return 0
	}
	return idx
}

// YOfLine returns the Y coordinate of the top of the given 0-based line.
func YOfLine(layout EditorLayout, lineIndex int) float64 {
	return float64(lineIndex) * layout.LineHeight
}

// LineTopFromPos returns the Y coordinate of the top of the line containing
// the given position.
func LineTopFromPos(layout EditorLayout, doc Text, pos int) float64 {
	line := doc.LineAt(pos)
	return YOfLine(layout, line.Number-1)
}

// TotalHeight returns the total content height (all lines).
func TotalHeight(layout EditorLayout) float64 {
	return float64(layout.LineCount) * layout.LineHeight
}
