// Package rendering — text caret offset calculation for form controls.
package rendering

import (
	"wb-ui/platform/graphics"
)

// CalcFormControlCaretOffset computes the character offset (into text, as
// runes) for a click at CSS (cssX, cssY) on a form control whose border-box
// starts at (boxX, boxY). isMultiLine selects <textarea> semantics (click
// Y picks the visual line — soft-wrapped like the browser — then X picks
// the column); single-line inputs use X only. font/padX/padY/lineH must
// match the painted text so the caret lands where the glyphs are.
func CalcFormControlCaretOffset(text string, isMultiLine bool, cssX, cssY, boxX, boxY, boxW float64, font graphics.Font, padX, padY, lineH float64) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}

	if isMultiLine {
		relY := cssY - boxY - padY
		line := int(relY / lineH)
		if line < 0 {
			line = 0
		}
		contentW := boxW - padX*2
		if contentW < 1 {
			contentW = 1
		}
		wrapped := wrapTextAreaLines(text, font, contentW)
		if line >= len(wrapped) {
			line = len(wrapped) - 1
		}
		wl := wrapped[line]
		relX := cssX - boxX - padX
		lineRunes := []rune(wl.text)
		totalW := 0.0
		for i, r := range lineRunes {
			charW := graphics.MeasureText(font, string(r))
			if relX < totalW+charW/2 {
				return wl.start + i
			}
			totalW += charW
		}
		return wl.end
	}

	// Single-line input: offset from X only. Tabs advance to the next tab
	// stop (8 columns) like browsers; other whitespace measures normally.
	relX := cssX - boxX - padX
	totalW := 0.0
	for i, r := range runes {
		var charW float64
		if r == '\t' {
			charW = tabStopWidth(font, totalW)
		} else {
			charW = graphics.MeasureText(font, string(r))
		}
		if relX < totalW+charW/2 {
			return i
		}
		totalW += charW
	}
	return len(runes)
}

// tabStopWidth returns the advance needed to reach the next 8-column tab
// stop, matching monospace tab rendering.
func tabStopWidth(font graphics.Font, currentW float64) float64 {
	col := int(currentW / (font.Size * 0.6))
	if col < 1 {
		col = 1
	}
	next := float64(col+1) * font.Size * 0.6
	return next - currentW
}
