// Translation of: CodeMirror 6 — packages/commands/src/commands.ts
//                  packages/keymap/src/keymap.ts
//
// Completeness: 40%
// Differences from CM6:
//   - CM6 commands are extensions that hook into the view's update cycle;
//     this port uses a simpler function-based approach: a Command is a
//     function that takes an EditorView and returns whether it succeeded.
//   - Key bindings are matched by a key string (e.g. "Ctrl-Z", "ArrowLeft")
//     rather than by platform-specific key codes.
//   - Methods use PascalCase (Go convention).
//
// The command system maps keyboard input to editor actions. A KeyBinding
// associates a key string with a Command function.

package editor

// Command is a function that performs an editor action. It returns true
// if the command was handled (and the default action should be prevented),
// or false if it was not handled.
type Command func(view *EditorView) bool

// KeyBinding associates a key combination with a command.
type KeyBinding struct {
	// Key is the key combination, e.g. "Ctrl-Z", "Shift-ArrowLeft",
	// "Enter", "Backspace".
	Key string
	// Run is the command to execute when the key is pressed.
	Run Command
	// Shift is an optional command to run when the key is pressed with
	// Shift. If nil, Shift+key is handled by Run with a shift modifier.
	Shift Command
	// PreventDefault reports whether the default browser behavior should
	// be prevented (always true in this port since we handle all input).
	PreventDefault bool
}

// DefaultKeymap returns the default set of key bindings for the editor.
// This includes cursor movement, text editing, and clipboard commands.
func DefaultKeymap() []KeyBinding {
	return []KeyBinding{
		// Cursor movement.
		{Key: "ArrowLeft", Run: MoveCharLeft, PreventDefault: true},
		{Key: "ArrowRight", Run: MoveCharRight, PreventDefault: true},
		{Key: "ArrowUp", Run: MoveLineUp, PreventDefault: true},
		{Key: "ArrowDown", Run: MoveLineDown, PreventDefault: true},
		{Key: "Home", Run: MoveLineStart, PreventDefault: true},
		{Key: "End", Run: MoveLineEnd, PreventDefault: true},
		{Key: "PageUp", Run: MovePageUp, PreventDefault: true},
		{Key: "PageDown", Run: MovePageDown, PreventDefault: true},

		// Selection movement (Shift + arrow).
		{Key: "Shift-ArrowLeft", Run: SelectCharLeft, PreventDefault: true},
		{Key: "Shift-ArrowRight", Run: SelectCharRight, PreventDefault: true},
		{Key: "Shift-ArrowUp", Run: SelectLineUp, PreventDefault: true},
		{Key: "Shift-ArrowDown", Run: SelectLineDown, PreventDefault: true},
		{Key: "Shift-Home", Run: SelectLineStart, PreventDefault: true},
		{Key: "Shift-End", Run: SelectLineEnd, PreventDefault: true},

		// Text editing.
		{Key: "Backspace", Run: DeleteCharBackward, PreventDefault: true},
		{Key: "Delete", Run: DeleteCharForward, PreventDefault: true},
		{Key: "Enter", Run: InsertNewline, PreventDefault: true},
		{Key: "Tab", Run: InsertTab, PreventDefault: true},

		// Word movement.
		{Key: "Ctrl-ArrowLeft", Run: MoveWordLeft, PreventDefault: true},
		{Key: "Ctrl-ArrowRight", Run: MoveWordRight, PreventDefault: true},
		{Key: "Ctrl-Backspace", Run: DeleteWordBackward, PreventDefault: true},
		{Key: "Ctrl-Delete", Run: DeleteWordForward, PreventDefault: true},

		// Document navigation.
		{Key: "Ctrl-Home", Run: MoveDocStart, PreventDefault: true},
		{Key: "Ctrl-End", Run: MoveDocEnd, PreventDefault: true},
		{Key: "Ctrl-A", Run: SelectAll, PreventDefault: true},

		// Undo/Redo.
		{Key: "Ctrl-Z", Run: Undo, PreventDefault: true},
		{Key: "Ctrl-Y", Run: Redo, PreventDefault: true},
		{Key: "Ctrl-Shift-Z", Run: Redo, PreventDefault: true},
	}
}

// --- Built-in commands ---

// insertText inserts the given text at the current selection. If the
// selection is a range, the range is replaced. This is the core text
// insertion command used by character input and IME.
func insertText(view *EditorView, text string, userEvent string) {
	sel := view.state.Selection
	ranges := sel.Ranges()
	var changes []ChangeDesc
	var inserted []Text
	for _, r := range ranges {
		from := r.From()
		to := r.To()
		changes = append(changes, ChangeDesc{
			FromA:        from,
			ToA:          to,
			InsertLength: len([]rune(text)),
		})
		inserted = append(inserted, TextFromString(text))
	}
	cs := NewChangeSetWithText(changes, inserted)

	// Compute the new selection: all carets at the end of the inserted text.
	newRanges := make([]Range, len(ranges))
	for i, r := range ranges {
		newPos := cs.MapPos(r.From(), 1) // assoc after insertion
		newRanges[i] = NewRangeCaret(newPos)
	}
	newSel := NewEditorSelection(newRanges, 0)

	view.Dispatch(TransactionSpec{
		Changes:       cs,
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     userEvent,
	})
}

// InsertText inserts text at the current selection (replacing any selected
// range).
func InsertText(text string) Command {
	return func(view *EditorView) bool {
		insertText(view, text, "input.type")
		return true
	}
}

// InsertNewline inserts a newline character at the current selection.
func InsertNewline(view *EditorView) bool {
	insertText(view, "\n", "input.type")
	return true
}

// InsertTab inserts a tab character at the current selection.
func InsertTab(view *EditorView) bool {
	insertText(view, "\t", "input.type")
	return true
}

// DeleteCharBackward deletes the character before the cursor (or the
// selected range).
func DeleteCharBackward(view *EditorView) bool {
	sel := view.state.Selection
	ranges := sel.Ranges()
	var changes []ChangeDesc
	var inserted []Text
	for _, r := range ranges {
		from := r.From()
		to := r.To()
		if from == to {
			// No selection — delete one character backward.
			if from == 0 {
				continue // at document start, nothing to delete
			}
			from = from - 1
		}
		changes = append(changes, ChangeDesc{
			FromA:        from,
			ToA:          to,
			InsertLength: 0,
		})
		inserted = append(inserted, Empty())
	}
	if len(changes) == 0 {
		return false
	}
	cs := NewChangeSetWithText(changes, inserted)
	newSel := sel.Map(cs)
	view.Dispatch(TransactionSpec{
		Changes:       cs,
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "delete.contentBackward",
	})
	return true
}

// DeleteCharForward deletes the character after the cursor (or the
// selected range).
func DeleteCharForward(view *EditorView) bool {
	sel := view.state.Selection
	ranges := sel.Ranges()
	var changes []ChangeDesc
	var inserted []Text
	for _, r := range ranges {
		from := r.From()
		to := r.To()
		if from == to {
			// No selection — delete one character forward.
			if to >= view.state.Doc.Length() {
				continue // at document end, nothing to delete
			}
			to = to + 1
		}
		changes = append(changes, ChangeDesc{
			FromA:        from,
			ToA:          to,
			InsertLength: 0,
		})
		inserted = append(inserted, Empty())
	}
	if len(changes) == 0 {
		return false
	}
	cs := NewChangeSetWithText(changes, inserted)
	newSel := sel.Map(cs)
	view.Dispatch(TransactionSpec{
		Changes:       cs,
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "delete.contentForward",
	})
	return true
}

// --- Cursor movement commands ---

// moveSelection moves the primary selection by the given delta.
// If extend is true, the selection is extended (Shift+arrow behavior).
func moveSelection(view *EditorView, delta int, extend bool, userEvent string) {
	sel := view.state.Selection
	main := sel.Main()
	var newRange Range
	if extend {
		newRange = main.Extend(main.Head() + delta)
	} else {
		newRange = NewRangeCaret(main.Head() + delta)
	}
	newRange = newRange.Bound(view.state.Doc.Length())
	newSel := sel.ReplaceRange(newRange, -1)
	view.Dispatch(TransactionSpec{
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     userEvent,
	})
}

func MoveCharLeft(view *EditorView) bool  { moveSelection(view, -1, false, "move.charLeft"); return true }
func MoveCharRight(view *EditorView) bool { moveSelection(view, 1, false, "move.charRight"); return true }
func SelectCharLeft(view *EditorView) bool  { moveSelection(view, -1, true, "select.charLeft"); return true }
func SelectCharRight(view *EditorView) bool { moveSelection(view, 1, true, "select.charRight"); return true }

func MoveLineUp(view *EditorView) bool {
	// Move to the previous line at the same column.
	doc := view.state.Doc
	sel := view.state.Selection
	main := sel.Main()
	pos := main.Head()
	line := doc.LineAt(pos)
	if line.Number <= 1 {
		// First line — move to start.
		moveSelection(view, -pos, false, "move.lineUp")
		return true
	}
	prevLine := doc.LineN(line.Number - 1)
	// Calculate the column.
	col := pos - line.From
	targetPos := prevLine.From + col
	if targetPos > prevLine.To {
		targetPos = prevLine.To
	}
	delta := targetPos - pos
	moveSelection(view, delta, false, "move.lineUp")
	return true
}

func MoveLineDown(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	main := sel.Main()
	pos := main.Head()
	line := doc.LineAt(pos)
	if line.Number >= doc.Lines() {
		// Last line — move to end.
		delta := doc.Length() - pos
		moveSelection(view, delta, false, "move.lineDown")
		return true
	}
	nextLine := doc.LineN(line.Number + 1)
	col := pos - line.From
	targetPos := nextLine.From + col
	if targetPos > nextLine.To {
		targetPos = nextLine.To
	}
	delta := targetPos - pos
	moveSelection(view, delta, false, "move.lineDown")
	return true
}

func SelectLineUp(view *EditorView) bool {
	delta := -1
	// Approximate: move by one line up (using the same logic as MoveLineUp
	// but with extend=true).
	doc := view.state.Doc
	sel := view.state.Selection
	main := sel.Main()
	pos := main.Head()
	line := doc.LineAt(pos)
	if line.Number <= 1 {
		delta = -pos
	} else {
		prevLine := doc.LineN(line.Number - 1)
		col := pos - line.From
		targetPos := prevLine.From + col
		if targetPos > prevLine.To {
			targetPos = prevLine.To
		}
		delta = targetPos - pos
	}
	moveSelection(view, delta, true, "select.lineUp")
	return true
}

func SelectLineDown(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	main := sel.Main()
	pos := main.Head()
	line := doc.LineAt(pos)
	var delta int
	if line.Number >= doc.Lines() {
		delta = doc.Length() - pos
	} else {
		nextLine := doc.LineN(line.Number + 1)
		col := pos - line.From
		targetPos := nextLine.From + col
		if targetPos > nextLine.To {
			targetPos = nextLine.To
		}
		delta = targetPos - pos
	}
	moveSelection(view, delta, true, "select.lineDown")
	return true
}

func MoveLineStart(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	line := doc.LineAt(pos)
	delta := line.From - pos
	moveSelection(view, delta, false, "move.lineStart")
	return true
}

func MoveLineEnd(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	line := doc.LineAt(pos)
	delta := line.To - pos
	moveSelection(view, delta, false, "move.lineEnd")
	return true
}

func SelectLineStart(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	line := doc.LineAt(pos)
	delta := line.From - pos
	moveSelection(view, delta, true, "select.lineStart")
	return true
}

func SelectLineEnd(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	line := doc.LineAt(pos)
	delta := line.To - pos
	moveSelection(view, delta, true, "select.lineEnd")
	return true
}

func MoveDocStart(view *EditorView) bool {
	sel := view.state.Selection
	pos := sel.Main().Head()
	moveSelection(view, -pos, false, "move.docStart")
	return true
}

func MoveDocEnd(view *EditorView) bool {
	sel := view.state.Selection
	pos := sel.Main().Head()
	delta := view.state.Doc.Length() - pos
	moveSelection(view, delta, false, "move.docEnd")
	return true
}

func MovePageUp(view *EditorView) bool {
	// Approximate: move by viewport height / line height lines.
	visibleLines := 1
	if view.layout.LineHeight > 0 && view.height > 0 {
		visibleLines = int(view.height / view.layout.LineHeight)
	}
	if visibleLines < 1 {
		visibleLines = 1
	}
	delta := -visibleLines
	moveSelection(view, delta, false, "move.pageUp")
	return true
}

func MovePageDown(view *EditorView) bool {
	visibleLines := 1
	if view.layout.LineHeight > 0 && view.height > 0 {
		visibleLines = int(view.height / view.layout.LineHeight)
	}
	if visibleLines < 1 {
		visibleLines = 1
	}
	delta := visibleLines
	moveSelection(view, delta, false, "move.pageDown")
	return true
}

// SelectAll selects the entire document.
func SelectAll(view *EditorView) bool {
	sel := NewEditorSelection([]Range{NewRange(0, view.state.Doc.Length())}, 0)
	view.Dispatch(TransactionSpec{
		Selection:     &sel,
		HasSelection:  true,
		UserEvent:     "select.all",
	})
	return true
}

// Undo undoes the last editing action.
func Undo(view *EditorView) bool {
	return view.history.Undo(view)
}

// Redo redoes the last undone action.
func Redo(view *EditorView) bool {
	return view.history.Redo(view)
}

// --- Word-level movement (simplified) ---

func MoveWordLeft(view *EditorView) bool {
	// Simplified: find the previous word boundary.
	doc := view.state.Doc
	pos := view.state.Selection.Main().Head()
	if pos == 0 {
		return true
	}
	// Skip whitespace backward, then skip word characters backward.
	text := doc.SliceString(0, pos)
	i := len([]rune(text)) - 1
	runes := []rune(text)
	// Skip trailing whitespace.
	for i >= 0 && isSpace(runes[i]) {
		i--
	}
	// Skip word characters.
	for i >= 0 && !isSpace(runes[i]) {
		i--
	}
	targetPos := i + 1
	delta := targetPos - pos
	moveSelection(view, delta, false, "move.wordLeft")
	return true
}

func MoveWordRight(view *EditorView) bool {
	doc := view.state.Doc
	pos := view.state.Selection.Main().Head()
	if pos >= doc.Length() {
		return true
	}
	text := doc.SliceString(pos, doc.Length())
	runes := []rune(text)
	i := 0
	// Skip leading whitespace.
	for i < len(runes) && isSpace(runes[i]) {
		i++
	}
	// Skip word characters.
	for i < len(runes) && !isSpace(runes[i]) {
		i++
	}
	delta := i
	moveSelection(view, delta, false, "move.wordRight")
	return true
}

func DeleteWordBackward(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	if pos == 0 {
		return true
	}
	text := doc.SliceString(0, pos)
	runes := []rune(text)
	i := len(runes) - 1
	for i >= 0 && isSpace(runes[i]) {
		i--
	}
	for i >= 0 && !isSpace(runes[i]) {
		i--
	}
	from := i + 1
	cs := NewChangeSetWithText([]ChangeDesc{{
		FromA: from, ToA: pos, InsertLength: 0,
	}}, []Text{Empty()})
	newSel := sel.Map(cs)
	view.Dispatch(TransactionSpec{
		Changes:       cs,
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "delete.wordBackward",
	})
	return true
}

func DeleteWordForward(view *EditorView) bool {
	doc := view.state.Doc
	sel := view.state.Selection
	pos := sel.Main().Head()
	if pos >= doc.Length() {
		return true
	}
	text := doc.SliceString(pos, doc.Length())
	runes := []rune(text)
	i := 0
	for i < len(runes) && isSpace(runes[i]) {
		i++
	}
	for i < len(runes) && !isSpace(runes[i]) {
		i++
	}
	to := pos + i
	cs := NewChangeSetWithText([]ChangeDesc{{
		FromA: pos, ToA: to, InsertLength: 0,
	}}, []Text{Empty()})
	newSel := sel.Map(cs)
	view.Dispatch(TransactionSpec{
		Changes:       cs,
		Selection:     &newSel,
		HasSelection:  true,
		UserEvent:     "delete.wordForward",
	})
	return true
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
