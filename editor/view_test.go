package editor

import (
	"testing"

	"wb-ui/platform/graphics"
)

// helper to create a test EditorView
func newTestView(text string) *EditorView {
	sel := SelectionCaret(0)
	state := NewState(Config{
		Doc:       TextFromString(text),
		Selection: &sel,
	})
	font := graphics.Font{Family: "Consolas", Size: 14}
	return NewEditorView(EditorViewConfig{
		State:           state,
		Font:            font,
		ShowLineNumbers: true,
		Width:           800,
		Height:          600,
	})
}

// TestEditorView_Create tests basic EditorView creation.
func TestEditorView_Create(t *testing.T) {
	v := newTestView("hello\nworld")
	if v == nil {
		t.Fatal("view is nil")
	}
	if v.state.Doc.Length() != 11 {
		t.Fatalf("doc length = %d, want 11", v.state.Doc.Length())
	}
	if v.state.Doc.Lines() != 2 {
		t.Fatalf("doc lines = %d, want 2", v.state.Doc.Lines())
	}
}

// TestEditorView_Layout tests that layout is calculated correctly.
func TestEditorView_Layout(t *testing.T) {
	v := newTestView("line1\nline2\nline3")
	layout := v.Layout()
	if layout.LineHeight <= 0 {
		t.Fatalf("lineHeight = %f, want > 0", layout.LineHeight)
	}
	if layout.CharWidth <= 0 {
		t.Fatalf("charWidth = %f, want > 0", layout.CharWidth)
	}
	if layout.LineNumberWidth <= 0 {
		t.Fatalf("lineNumberWidth = %f, want > 0", layout.LineNumberWidth)
	}
	if layout.LineCount != 3 {
		t.Fatalf("lineCount = %d, want 3", layout.LineCount)
	}
}

// TestEditorView_PosToXY tests coordinate conversion.
func TestEditorView_PosToXY(t *testing.T) {
	v := newTestView("hello\nworld")
	x, y := v.PosToXY(0)
	if x < 0 {
		t.Fatalf("x = %f, want >= 0", x)
	}
	if y != 0 {
		t.Fatalf("y = %f, want 0 (first line)", y)
	}

	// Position 6 is the start of "world" (second line).
	x2, y2 := v.PosToXY(6)
	if y2 != v.Layout().LineHeight {
		t.Fatalf("y2 = %f, want %f (second line)", y2, v.Layout().LineHeight)
	}
	if x2 != x {
		t.Fatalf("x2 = %f, want %f (same column)", x2, x)
	}
}

// TestEditorView_XYToPos tests the reverse coordinate conversion.
func TestEditorView_XYToPos(t *testing.T) {
	v := newTestView("hello\nworld")
	layout := v.Layout()

	// Click at the beginning of the first line.
	pos := v.XYToPos(layout.ContentX, 0)
	if pos != 0 {
		t.Fatalf("pos = %d, want 0", pos)
	}

	// Click at the beginning of the second line.
	pos = v.XYToPos(layout.ContentX, layout.LineHeight)
	if pos != 6 {
		t.Fatalf("pos = %d, want 6 (start of second line)", pos)
	}
}

// TestEditorView_InsertText tests text insertion via Dispatch.
func TestEditorView_InsertText(t *testing.T) {
	v := newTestView("")
	v.Dispatch(TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 0, ToA: 0, InsertLength: 5}},
			[]Text{TextFromString("hello")},
		),
		UserEvent: "input.type",
	})
	if v.state.Doc.String() != "hello" {
		t.Fatalf("doc = %q, want %q", v.state.Doc.String(), "hello")
	}
}

// TestEditorView_Undo tests undo functionality.
func TestEditorView_Undo(t *testing.T) {
	v := newTestView("abc")

	// Insert "def" at position 3.
	v.Dispatch(TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 3, ToA: 3, InsertLength: 3}},
			[]Text{TextFromString("def")},
		),
		UserEvent: "input.type",
	})
	if v.state.Doc.String() != "abcdef" {
		t.Fatalf("after insert: doc = %q, want %q", v.state.Doc.String(), "abcdef")
	}

	// Close typing to finalize the undo group.
	v.history.CloseTyping()

	// Undo.
	ok := v.history.Undo(v)
	if !ok {
		t.Fatal("Undo returned false")
	}
	if v.state.Doc.String() != "abc" {
		t.Fatalf("after undo: doc = %q, want %q", v.state.Doc.String(), "abc")
	}
}

// TestEditorView_Redo tests redo functionality.
func TestEditorView_Redo(t *testing.T) {
	v := newTestView("abc")

	v.Dispatch(TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 3, ToA: 3, InsertLength: 3}},
			[]Text{TextFromString("def")},
		),
		UserEvent: "input.type",
	})
	v.history.CloseTyping()
	v.history.Undo(v)

	ok := v.history.Redo(v)
	if !ok {
		t.Fatal("Redo returned false")
	}
	if v.state.Doc.String() != "abcdef" {
		t.Fatalf("after redo: doc = %q, want %q", v.state.Doc.String(), "abcdef")
	}
}

// TestCommand_InsertText tests the InsertText command.
func TestCommand_InsertText(t *testing.T) {
	v := newTestView("")
	cmd := InsertText("hello")
	ok := cmd(v)
	if !ok {
		t.Fatal("InsertText returned false")
	}
	if v.state.Doc.String() != "hello" {
		t.Fatalf("doc = %q, want %q", v.state.Doc.String(), "hello")
	}
}

// TestCommand_DeleteCharBackward tests deleting backward.
func TestCommand_DeleteCharBackward(t *testing.T) {
	v := newTestView("hello")
	// Move cursor to position 5 (after "hello").
	sel := NewEditorSelection([]Range{NewRangeCaret(5)}, 0)
	v.Dispatch(TransactionSpec{Selection: &sel, HasSelection: true})

	DeleteCharBackward(v)
	if v.state.Doc.String() != "hell" {
		t.Fatalf("doc = %q, want %q", v.state.Doc.String(), "hell")
	}
}

// TestCommand_MoveCharLeft tests cursor movement.
func TestCommand_MoveCharLeft(t *testing.T) {
	v := newTestView("hello")
	// Set cursor at position 3.
	sel := NewEditorSelection([]Range{NewRangeCaret(3)}, 0)
	v.Dispatch(TransactionSpec{Selection: &sel, HasSelection: true})

	MoveCharLeft(v)
	pos := v.state.Selection.Main().Head()
	if pos != 2 {
		t.Fatalf("pos = %d, want 2", pos)
	}
}

// TestCommand_SelectAll tests the SelectAll command.
func TestCommand_SelectAll(t *testing.T) {
	v := newTestView("hello world")
	SelectAll(v)
	sel := v.state.Selection.Main()
	if sel.From() != 0 {
		t.Fatalf("from = %d, want 0", sel.From())
	}
	if sel.To() != 11 {
		t.Fatalf("to = %d, want 11", sel.To())
	}
}

// TestCommand_MoveLineUp tests moving the cursor up one line.
func TestCommand_MoveLineUp(t *testing.T) {
	v := newTestView("line1\nline2\nline3")
	// Set cursor at position 8 (middle of "line2").
	sel := NewEditorSelection([]Range{NewRangeCaret(8)}, 0)
	v.Dispatch(TransactionSpec{Selection: &sel, HasSelection: true})

	MoveLineUp(v)
	pos := v.state.Selection.Main().Head()
	// Should be on line 1 at column 2 (position 2).
	if pos != 2 {
		t.Fatalf("pos = %d, want 2", pos)
	}
}

// TestCommand_MoveLineDown tests moving the cursor down one line.
func TestCommand_MoveLineDown(t *testing.T) {
	v := newTestView("line1\nline2\nline3")
	// Set cursor at position 2 (middle of "line1").
	sel := NewEditorSelection([]Range{NewRangeCaret(2)}, 0)
	v.Dispatch(TransactionSpec{Selection: &sel, HasSelection: true})

	MoveLineDown(v)
	pos := v.state.Selection.Main().Head()
	// Should be on line 2 at column 2 (position 8).
	if pos != 8 {
		t.Fatalf("pos = %d, want 8", pos)
	}
}

// TestCommand_InsertNewline tests inserting a newline.
func TestCommand_InsertNewline(t *testing.T) {
	v := newTestView("ab")
	// Set cursor at position 1 (between a and b).
	sel := NewEditorSelection([]Range{NewRangeCaret(1)}, 0)
	v.Dispatch(TransactionSpec{Selection: &sel, HasSelection: true})

	InsertNewline(v)
	if v.state.Doc.String() != "a\nb" {
		t.Fatalf("doc = %q, want %q", v.state.Doc.String(), "a\nb")
	}
}

// TestHistory_MergeTyping tests that consecutive typing is merged into one undo step.
func TestHistory_MergeTyping(t *testing.T) {
	v := newTestView("")

	// Type "a".
	insertText(v, "a", "input.type")
	// Type "b".
	insertText(v, "b", "input.type")
	// Type "c".
	insertText(v, "c", "input.type")

	if v.state.Doc.String() != "abc" {
		t.Fatalf("doc = %q, want %q", v.state.Doc.String(), "abc")
	}

	// Close typing.
	v.history.CloseTyping()

	// Undo should remove all three characters at once.
	ok := v.history.Undo(v)
	if !ok {
		t.Fatal("Undo returned false")
	}
	if v.state.Doc.String() != "" {
		t.Fatalf("after undo: doc = %q, want empty", v.state.Doc.String())
	}
}

// TestHistory_DifferentEvents tests that different userEvents create separate undo steps.
func TestHistory_DifferentEvents(t *testing.T) {
	v := newTestView("")

	// Type "abc".
	insertText(v, "abc", "input.type")
	v.history.CloseTyping()

	// Delete one character.
	DeleteCharBackward(v)

	// Undo should undo the delete.
	ok := v.history.Undo(v)
	if !ok {
		t.Fatal("Undo returned false")
	}
	if v.state.Doc.String() != "abc" {
		t.Fatalf("after undo: doc = %q, want %q", v.state.Doc.String(), "abc")
	}

	// Undo again should undo the typing.
	ok = v.history.Undo(v)
	if !ok {
		t.Fatal("Undo returned false")
	}
	if v.state.Doc.String() != "" {
		t.Fatalf("after second undo: doc = %q, want empty", v.state.Doc.String())
	}
}

// TestViewport tests viewport calculations.
func TestViewport(t *testing.T) {
	vp := NewViewport(0, 100)
	if vp.Empty() {
		t.Fatal("viewport should not be empty")
	}
	if !vp.Contains(50) {
		t.Fatal("viewport should contain 50")
	}
	if vp.Contains(100) {
		t.Fatal("viewport should not contain 100 (exclusive)")
	}
}

// TestKeymap_Default tests that the default keymap has expected entries.
func TestKeymap_Default(t *testing.T) {
	km := DefaultKeymap()
	if len(km) == 0 {
		t.Fatal("default keymap is empty")
	}

	// Check for some expected keys.
	foundCtrlZ := false
	foundEnter := false
	foundArrowLeft := false
	for _, kb := range km {
		if kb.Key == "Ctrl-Z" {
			foundCtrlZ = true
		}
		if kb.Key == "Enter" {
			foundEnter = true
		}
		if kb.Key == "ArrowLeft" {
			foundArrowLeft = true
		}
	}
	if !foundCtrlZ {
		t.Fatal("Ctrl-Z not found in default keymap")
	}
	if !foundEnter {
		t.Fatal("Enter not found in default keymap")
	}
	if !foundArrowLeft {
		t.Fatal("ArrowLeft not found in default keymap")
	}
}

// TestInput_HandleClick tests mouse click to position.
func TestInput_HandleClick(t *testing.T) {
	v := newTestView("hello\nworld")
	layout := v.Layout()

	// Click at the beginning of the first line.
	pos := HandleClick(v, MouseEvent{
		X: layout.ContentX,
		Y: 0,
	}, 1)
	if pos != 0 {
		t.Fatalf("click pos = %d, want 0", pos)
	}
}

// TestCommand_UndoRedoChain tests a multi-step undo/redo chain.
func TestCommand_UndoRedoChain(t *testing.T) {
	v := newTestView("")

	// Type "a", then "b", then "c" (merged as one undo step).
	insertText(v, "a", "input.type")
	insertText(v, "b", "input.type")
	insertText(v, "c", "input.type")
	v.history.CloseTyping()

	// Type "d" (new undo step).
	insertText(v, "d", "input.type.delete")
	v.history.CloseTyping()

	// Undo removes "d".
	v.history.Undo(v)
	if v.state.Doc.String() != "abc" {
		t.Fatalf("after first undo: doc = %q, want %q", v.state.Doc.String(), "abc")
	}

	// Undo removes "abc".
	v.history.Undo(v)
	if v.state.Doc.String() != "" {
		t.Fatalf("after second undo: doc = %q, want empty", v.state.Doc.String())
	}

	// Redo restores "abc".
	v.history.Redo(v)
	if v.state.Doc.String() != "abc" {
		t.Fatalf("after first redo: doc = %q, want %q", v.state.Doc.String(), "abc")
	}

	// Redo restores "d".
	v.history.Redo(v)
	if v.state.Doc.String() != "abcd" {
		t.Fatalf("after second redo: doc = %q, want %q", v.state.Doc.String(), "abcd")
	}
}