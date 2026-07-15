// Editor input handling: converts mouse and keyboard events into editor
// transactions.
//
// This module bridges the host's event system (mouse, keyboard, IME) with
// the editor's command/transaction system. It does not directly modify the
// view state — instead, it generates TransactionSpecs or invokes Commands
// via the view's Dispatch method.
//
// Translation of: CodeMirror 6 — packages/view/src/input.ts (simplified)

package editor

import "strings"

// KeyEvent describes a keyboard event from the host.
type KeyEvent struct {
	// Key is the key name (e.g. "a", "Enter", "ArrowLeft", "Backspace").
	Key string
	// Code is the physical key code (e.g. "KeyA", "Enter", "ArrowLeft").
	Code string
	// Ctrl reports whether the Ctrl modifier is pressed.
	Ctrl bool
	// Shift reports whether the Shift modifier is pressed.
	Shift bool
	// Alt reports whether the Alt/Option modifier is pressed.
	Alt bool
	// Meta reports whether the Meta/Win/Cmd modifier is pressed.
	Meta bool
	// Text is the text produced by the key (for printable keys). Empty
	// for non-printable keys like ArrowLeft.
	Text string
}

// MouseEvent describes a mouse event from the host.
type MouseEvent struct {
	// X, Y are the pixel coordinates relative to the editor content area.
	X, Y float64
	// Button is 0 (left), 1 (middle), or 2 (right).
	Button int
	// Shift/Ctrl/Alt/Meta report modifier keys.
	Shift bool
	Ctrl  bool
	Alt   bool
	Meta  bool
}

// HandleClick processes a mouse click and returns the rune offset of the
// clicked position. If the click is a single click, it moves the cursor;
// double/triple clicks select word/line.
func HandleClick(view *EditorView, ev MouseEvent, clickCount int) int {
	pos := view.XYToPos(ev.X, ev.Y)

	switch clickCount {
	case 1:
		// Single click — move cursor.
		sel := NewEditorSelection([]Range{NewRangeCaret(pos)}, 0)
		view.Dispatch(TransactionSpec{
			Selection:     &sel,
			HasSelection:  true,
			UserEvent:     "select.pointer",
		})
	case 2:
		// Double click — select word.
		from, to := wordRangeAt(view, pos)
		sel := NewEditorSelection([]Range{NewRange(from, to)}, 0)
		view.Dispatch(TransactionSpec{
			Selection:     &sel,
			HasSelection:  true,
			UserEvent:     "select.word",
		})
		pos = from
	case 3:
		// Triple click — select line.
		line := view.state.Doc.LineAt(pos)
		from := line.From
		to := line.To
		if to < view.state.Doc.Length() {
			to++ // include the trailing newline
		}
		sel := NewEditorSelection([]Range{NewRange(from, to)}, 0)
		view.Dispatch(TransactionSpec{
			Selection:     &sel,
			HasSelection:  true,
			UserEvent:     "select.line",
		})
		pos = from
	}

	view.history.CloseTyping()
	return pos
}

// HandleDrag processes a mouse drag (extending the selection).
func HandleDrag(view *EditorView, ev MouseEvent) int {
	pos := view.XYToPos(ev.X, ev.Y)
	sel := view.state.Selection
	main := sel.Main()
	newRange := main.Extend(pos)
	newSel := sel.ReplaceRange(newRange, -1)
	view.Dispatch(TransactionSpec{
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "select.pointer",
	})
	return pos
}

// HandleShiftClick processes a shift-click (extending the selection to the
// clicked position).
func HandleShiftClick(view *EditorView, ev MouseEvent) int {
	pos := view.XYToPos(ev.X, ev.Y)
	sel := view.state.Selection
	main := sel.Main()
	newRange := main.Extend(pos)
	newSel := sel.ReplaceRange(newRange, -1)
	view.Dispatch(TransactionSpec{
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "select.pointer",
	})
	return pos
}

// HandleKey processes a keyboard event. Returns true if the key was handled
// (and the default should be prevented).
func HandleKey(view *EditorView, ev KeyEvent, keymap []KeyBinding) bool {
	// Build the key string for lookup.
	keyStr := keyEventToString(ev)

	// Look up the key in the keymap.
	for _, kb := range keymap {
		if kb.Key == keyStr {
			if kb.Shift != nil && ev.Shift {
				if kb.Shift(view) {
					return true
				}
			} else if kb.Run != nil {
				if kb.Run(view) {
					return true
				}
			}
		}
	}

	// If the key produces text and no modifier (except Shift) is pressed,
	// insert the text.
	if ev.Text != "" && !ev.Ctrl && !ev.Alt && !ev.Meta {
		insertText(view, ev.Text, "input.type")
		return true
	}

	return false
}

// keyEventToString converts a KeyEvent to a key string for keymap lookup.
// e.g. Ctrl-Z, Shift-ArrowLeft, Enter, a.
func keyEventToString(ev KeyEvent) string {
	// If there are no modifiers and the key is a single character, use
	// the lower-case key (for letter keys).
	if !ev.Ctrl && !ev.Alt && !ev.Meta {
		if len(ev.Key) == 1 {
			if ev.Shift {
				return "Shift-" + ev.Key
			}
			return ev.Key
		}
		// Non-printable keys (ArrowLeft, Enter, etc.) may have Shift.
		if ev.Shift {
			return "Shift-" + ev.Key
		}
		return ev.Key
	}

	// Build modifier prefix.
	parts := []string{}
	if ev.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if ev.Alt {
		parts = append(parts, "Alt")
	}
	if ev.Shift {
		parts = append(parts, "Shift")
	}
	if ev.Meta {
		parts = append(parts, "Meta")
	}
	key := ev.Key
	// Uppercase single-letter keys to match keymap conventions (e.g. "Ctrl-A").
	if len(key) == 1 {
		key = strings.ToUpper(key)
	}
	parts = append(parts, key)
	return joinKey(parts)
}

func joinKey(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "-"
		}
		result += p
	}
	return result
}

// wordRangeAt finds the word boundaries around the given position.
func wordRangeAt(view *EditorView, pos int) (int, int) {
	doc := view.state.Doc
	if pos < 0 || pos >= doc.Length() {
		return pos, pos
	}
	text := doc.String()
	runes := []rune(text)
	if pos >= len(runes) {
		return pos, pos
	}

	// Find the start of the word.
	start := pos
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}

	// Find the end of the word.
	end := pos
	for end < len(runes) && isWordChar(runes[end]) {
		end++
	}

	return start, end
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}

// HandleCharInput processes a character input event (from IME or direct
// character input). This is called when the host receives a character
// input that should be inserted into the editor.
func HandleCharInput(view *EditorView, text string) {
	insertText(view, text, "input.type")
}

// HandleIMEComposition processes an IME composition event.
// During composition, the editor displays the preedit text as a decoration.
// On confirmation, the preedit is replaced with the final text.
func HandleIMEComposition(view *EditorView, preedit string, confirmed bool) {
	if confirmed {
		// Insert the confirmed text.
		insertText(view, preedit, "input.type")
	}
	// For preedit (unconfirmed), we would add a decoration to show the
	// preedit text. This is a TODO for full IME support.
}

// ScrollBy scrolls the editor by the given delta.
func ScrollBy(view *EditorView, deltaX, deltaY float64) {
	view.scrollX += deltaX
	view.scrollY += deltaY
	// Clamp scroll to valid range.
	totalHeight := TotalHeight(view.layout)
	if view.scrollY < 0 {
		view.scrollY = 0
	}
	if view.scrollY > totalHeight-view.height {
		view.scrollY = totalHeight - view.height
		if view.scrollY < 0 {
			view.scrollY = 0
		}
	}
	view.updateViewport()
}
