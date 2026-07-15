// Translation of: (test) Source/WebCore/editing/TypingCommand.cpp (unit tests)
// Completeness: test coverage for TypingCommand merge/closeTyping/undo semantics

package editing

import (
	"testing"

	"wb-ui/dom"
)

// TestTypingCommand_InsertText verifies a single InsertText typing command
// applies text and advances the caret, and that undo reverts both.
func TestTypingCommand_InsertText(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	cmd := NewTypingCommand(doc, target, 0, TypingInsertText, "abc", 0)
	if err := cmd.DoApply(); err != nil {
		t.Fatalf("DoApply: %v", err)
	}
	if got := target.TextContent(); got != "abc" {
		t.Fatalf("after insert: got %q, want %q", got, "abc")
	}
	if cmd.CaretOffset != 3 {
		t.Fatalf("caret: got %d, want 3", cmd.CaretOffset)
	}

	if err := cmd.DoUnapply(); err != nil {
		t.Fatalf("DoUnapply: %v", err)
	}
	if got := target.TextContent(); got != "" {
		t.Fatalf("after undo: got %q, want empty", got)
	}
	if cmd.CaretOffset != 0 {
		t.Fatalf("caret after undo: got %d, want 0", cmd.CaretOffset)
	}
}

// TestTypingCommand_ConsecutiveMerge verifies that consecutive InsertTextStatic
// calls merge into a single open TypingCommand, producing one undo step.
func TestTypingCommand_ConsecutiveMerge(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	cmd1 := InsertTextStatic(editor, target, 0, "a", 0)
	if cmd1 == nil {
		t.Fatal("first insert returned nil")
	}
	if got := target.TextContent(); got != "a" {
		t.Fatalf("after first insert: got %q, want %q", got, "a")
	}
	if !cmd1.IsOpenForMoreTyping() {
		t.Fatal("first command should be open for more typing")
	}

	// Second keystroke merges into cmd1.
	cmd2 := InsertTextStatic(editor, target, 1, "b", 0)
	if cmd2 != cmd1 {
		t.Fatal("second insert should merge into the same command")
	}
	if got := target.TextContent(); got != "ab" {
		t.Fatalf("after second insert: got %q, want %q", got, "ab")
	}

	// Third keystroke still merges.
	cmd3 := InsertTextStatic(editor, target, 2, "c", 0)
	if cmd3 != cmd1 {
		t.Fatal("third insert should merge into the same command")
	}
	if got := target.TextContent(); got != "abc" {
		t.Fatalf("after third insert: got %q, want %q", got, "abc")
	}

	// While typing is open, nothing is on the undo stack yet.
	if editor.LastTypingCommand() != cmd1 {
		t.Fatal("lastTypingCommand should be cmd1")
	}
	if editor.UndoStackSize() != 0 {
		t.Fatalf("undo stack should be empty while typing, got %d", editor.UndoStackSize())
	}

	// Closing commits a single undo step.
	editor.CloseTyping()
	if editor.LastTypingCommand() != nil {
		t.Fatal("lastTypingCommand should be nil after close")
	}
	if editor.UndoStackSize() != 1 {
		t.Fatalf("undo stack should have 1 step, got %d", editor.UndoStackSize())
	}

	// Undo reverts all three characters in one step.
	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "" {
		t.Fatalf("after undo of merged typing: got %q, want empty", got)
	}
	if editor.UndoStackSize() != 0 {
		t.Fatalf("undo stack should be empty after undo, got %d", editor.UndoStackSize())
	}
	if editor.RedoStackSize() != 1 {
		t.Fatalf("redo stack should have 1 step, got %d", editor.RedoStackSize())
	}

	// Redo restores all three characters.
	if !editor.Redo() {
		t.Fatal("Redo returned false")
	}
	if got := target.TextContent(); got != "abc" {
		t.Fatalf("after redo: got %q, want %q", got, "abc")
	}
}

// TestTypingCommand_CloseTypingFlag verifies isOpenForMoreTyping transitions.
func TestTypingCommand_CloseTypingFlag(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	cmd := NewTypingCommand(doc, target, 0, TypingInsertText, "x", 0)
	if !cmd.IsOpenForMoreTyping() {
		t.Fatal("new typing command should be open")
	}

	cmd.CloseTyping()
	if cmd.IsOpenForMoreTyping() {
		t.Fatal("after CloseTyping should be closed")
	}
}

// TestTypingCommand_DeleteKey verifies backspace behavior and that each
// DeleteKey is its own undo step (closes the typing session).
func TestTypingCommand_DeleteKey(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("hello")

	cmd := DeleteKeyPressedStatic(editor, target, 5, 0)
	if cmd == nil {
		t.Fatal("DeleteKeyPressedStatic returned nil")
	}
	if got := target.TextContent(); got != "hell" {
		t.Fatalf("after delete key: got %q, want %q", got, "hell")
	}
	if cmd.CaretOffset != 4 {
		t.Fatalf("caret after delete: got %d, want 4", cmd.CaretOffset)
	}
	// DeleteKey immediately closes its own session.
	if cmd.IsOpenForMoreTyping() {
		t.Fatal("DeleteKey should close typing session")
	}
	// The DeleteKey command is committed as its own undo step.
	if editor.UndoStackSize() != 1 {
		t.Fatalf("undo stack should have 1 step, got %d", editor.UndoStackSize())
	}

	// Undo restores the deleted character.
	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "hello" {
		t.Fatalf("after undo delete: got %q, want %q", got, "hello")
	}
}

// TestTypingCommand_InsertParagraphSeparator verifies Enter behavior and that
// it closes the typing session as its own undo step.
func TestTypingCommand_InsertParagraphSeparator(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("ab")

	cmd := InsertParagraphSeparatorStatic(editor, target, 1, 0)
	if cmd == nil {
		t.Fatal("InsertParagraphSeparatorStatic returned nil")
	}
	if got := target.TextContent(); got != "a\nb" {
		t.Fatalf("after paragraph separator: got %q, want %q", got, "a\nb")
	}
	if cmd.CaretOffset != 2 {
		t.Fatalf("caret: got %d, want 2", cmd.CaretOffset)
	}
	if cmd.IsOpenForMoreTyping() {
		t.Fatal("InsertParagraphSeparator should close typing session")
	}
	if editor.UndoStackSize() != 1 {
		t.Fatalf("undo stack should have 1 step, got %d", editor.UndoStackSize())
	}
}

// TestTypingCommand_InsertThenDeleteThenInsert verifies that a DeleteKey
// after an InsertText commits the InsertText as a separate undo step and
// starts a fresh typing session.
func TestTypingCommand_InsertThenDeleteThenInsert(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	// Type "ab" (one merged session).
	InsertTextStatic(editor, target, 0, "a", 0)
	InsertTextStatic(editor, target, 1, "b", 0)
	// DeleteKey commits "ab" as step 1, then DeleteKey as step 2.
	DeleteKeyPressedStatic(editor, target, 2, 0)
	if got := target.TextContent(); got != "a" {
		t.Fatalf("after delete: got %q, want %q", got, "a")
	}
	if editor.UndoStackSize() != 2 {
		t.Fatalf("undo stack should have 2 steps (insert + delete), got %d", editor.UndoStackSize())
	}

	// Type "c" — fresh session.
	InsertTextStatic(editor, target, 1, "c", 0)
	if got := target.TextContent(); got != "ac" {
		t.Fatalf("after fresh insert: got %q, want %q", got, "ac")
	}

	// Undo the "c" (still open, closeTyping pushes it then pops).
	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "a" {
		t.Fatalf("after undo c: got %q, want %q", got, "a")
	}

	// Undo the DeleteKey.
	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "ab" {
		t.Fatalf("after undo delete: got %q, want %q", got, "ab")
	}

	// Undo the "ab" insert.
	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "" {
		t.Fatalf("after undo ab: got %q, want empty", got)
	}
}
