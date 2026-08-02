// Package rendering — text caret offset calculation for form controls.
package rendering

import (
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TextareaWrapMode exposes the white-space/wrap-attribute-driven wrap
// strategy to the app layer so hit-testing uses the same line-breaking as
// painting.
func TextareaWrapMode(st *style.ComputedStyle, el *dom.Element) int {
	return textareaWrapMode(st, el)
}

// CalcFormControlCaretOffset computes the character offset (into text, as
// runes) for a click at CSS (cssX, cssY) on a form control whose border-box
// starts at (boxX, boxY). isMultiLine selects <textarea> semantics (click
// Y picks the visual line — soft-wrapped per wrapMode like the browser —
// then X picks the column); single-line inputs use X only. font/padX/padY/
// lineH must match the painted text so the caret lands where the glyphs
// are. wrapMode comes from TextareaWrapMode (ignored for single-line).
// The click X is compensated by the published horizontal scroll offset so
// a scrolled (pre/nowrap) control maps clicks correctly; sy is the
// control's vertical scroll offset (BoxScrollOffset.sy) so a scrolled-down
// textarea maps the click Y to the right visual row.
func CalcFormControlCaretOffset(text string, isMultiLine bool, cssX, cssY, boxX, boxY, boxW float64, font graphics.Font, padX, padY, lineH float64, wrapMode int, sy float64) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	// The visible click X maps back to text coordinates by the scroll offset
	// (positive when the text has been scrolled left), scoped to the focused
	// control so a sibling textarea's horizontal scroll never bleeds into a
	// single-line input's hit-testing.
	relX := cssX - boxX - padX + FormControlTextScroll(FocusedFormControl)

	if isMultiLine {
		// Click Y maps back to the text row by the vertical scroll offset
		// (positive when the text has scrolled up), scoped per control via
		// BoxScrollOffset.sy — a scrolled-down textarea must hit the visual
		// row, not the row the text was laid out at.
		relY := cssY - boxY - padY + sy
		line := int(relY / lineH)
		if line < 0 {
			line = 0
		}
		contentW := boxW - padX*2
		if contentW < 1 {
			contentW = 1
		}
		wrapped := wrapTextAreaLines(text, font, contentW, wrapMode)
		if line >= len(wrapped) {
			line = len(wrapped) - 1
		}
		wl := wrapped[line]
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
