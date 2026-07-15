// Translation of: (test) Source/WebCore/editing/EditCommand.cpp (unit tests)
// Completeness: test coverage for EditCommand/CompositeEditCommand/EditCommandComposition

package editing

import (
	"testing"

	"wb-ui/dom"
)

// mockEditorClient is a no-op EditorClient used by the test harness.
type mockEditorClient struct{}

func (m *mockEditorClient) SetInputMethodState(focused *dom.Element)        {}
func (m *mockEditorClient) HandleKeyboardEvent(ev *dom.KeyboardEvent)       {}
func (m *mockEditorClient) HandleInputMethodKeydown(ev *dom.KeyboardEvent)  {}
func (m *mockEditorClient) DiscardedComposition(doc *dom.Document)          {}
func (m *mockEditorClient) CanceledComposition()                            {}
func (m *mockEditorClient) DidUpdateComposition()                            {}

// newTestEditor builds an Editor backed by a fresh Document.
func newTestEditor() *Editor {
	return NewEditor(dom.NewDocument(), &mockEditorClient{})
}

// TestInsertTextCommand_ApplyUndoRedo verifies the leaf InsertTextCommand
// can apply, unapply and reapply a text insertion.
func TestInsertTextCommand_ApplyUndoRedo(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("hello")

	cmd := NewInsertTextCommand(doc, target, 5, " world", EditActionInsertText)
	if err := cmd.DoApply(); err != nil {
		t.Fatalf("DoApply: %v", err)
	}
	if got := target.TextContent(); got != "hello world" {
		t.Fatalf("after apply: got %q, want %q", got, "hello world")
	}

	if err := cmd.DoUnapply(); err != nil {
		t.Fatalf("DoUnapply: %v", err)
	}
	if got := target.TextContent(); got != "hello" {
		t.Fatalf("after unapply: got %q, want %q", got, "hello")
	}

	if err := cmd.DoReapply(); err != nil {
		t.Fatalf("DoReapply: %v", err)
	}
	if got := target.TextContent(); got != "hello world" {
		t.Fatalf("after reapply: got %q, want %q", got, "hello world")
	}
}

// TestInsertTextCommand_RuneAware verifies rune-aware insertion so that
// multi-byte characters are not split.
func TestInsertTextCommand_RuneAware(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("你好")

	cmd := NewInsertTextCommand(doc, target, 1, "世界", EditActionInsertText)
	if err := cmd.DoApply(); err != nil {
		t.Fatalf("DoApply: %v", err)
	}
	if got := target.TextContent(); got != "你世界好" {
		t.Fatalf("rune-aware insert: got %q, want %q", got, "你世界好")
	}

	if err := cmd.DoUnapply(); err != nil {
		t.Fatalf("DoUnapply: %v", err)
	}
	if got := target.TextContent(); got != "你好" {
		t.Fatalf("rune-aware undo: got %q, want %q", got, "你好")
	}
}

// TestCompositeEditCommand_ApplyUndoReapply verifies that a composite command
// applies sub-commands in order and undoes them in reverse.
func TestCompositeEditCommand_ApplyUndoReapply(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	composite := NewCompositeBase(doc)
	cmd1 := NewInsertTextCommand(doc, target, 0, "abc", EditActionInsertText)
	if err := composite.ApplyCommand(cmd1); err != nil {
		t.Fatalf("ApplyCommand 1: %v", err)
	}
	cmd2 := NewInsertTextCommand(doc, target, 3, "XYZ", EditActionInsertText)
	if err := composite.ApplyCommand(cmd2); err != nil {
		t.Fatalf("ApplyCommand 2: %v", err)
	}
	if got := target.TextContent(); got != "abcXYZ" {
		t.Fatalf("after composite apply: got %q, want %q", got, "abcXYZ")
	}
	if !composite.HasCommands() {
		t.Fatal("composite should have commands")
	}
	if len(composite.Commands()) != 2 {
		t.Fatalf("command count: got %d, want 2", len(composite.Commands()))
	}

	// Undo reverses cmd2 then cmd1.
	if err := composite.UnapplyAll(); err != nil {
		t.Fatalf("UnapplyAll: %v", err)
	}
	if got := target.TextContent(); got != "" {
		t.Fatalf("after composite undo: got %q, want empty", got)
	}

	// Redo re-applies cmd1 then cmd2.
	if err := composite.ReapplyAll(); err != nil {
		t.Fatalf("ReapplyAll: %v", err)
	}
	if got := target.TextContent(); got != "abcXYZ" {
		t.Fatalf("after composite redo: got %q, want %q", got, "abcXYZ")
	}
}

// TestCompositeEditCommand_RemoveCommand verifies RemoveCommand splices
// the sub-command out and returns it.
func TestCompositeEditCommand_RemoveCommand(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	composite := NewCompositeBase(doc)
	cmd1 := NewInsertTextCommand(doc, target, 0, "a", EditActionInsertText)
	_ = composite.ApplyCommand(cmd1)
	cmd2 := NewInsertTextCommand(doc, target, 1, "b", EditActionInsertText)
	_ = composite.ApplyCommand(cmd2)

	removed := composite.RemoveCommand(1)
	if removed != cmd2 {
		t.Fatal("RemoveCommand should return the removed command")
	}
	if len(composite.Commands()) != 1 {
		t.Fatalf("after remove: got %d commands, want 1", len(composite.Commands()))
	}
	if composite.Commands()[0] != cmd1 {
		t.Fatal("remaining command should be cmd1")
	}
}

// TestEditCommandComposition_Wrap verifies Wrap produces a working UndoStep
// that can unapply and reapply the wrapped composite.
func TestEditCommandComposition_Wrap(t *testing.T) {
	doc := dom.NewDocument()
	target := doc.CreateElement("div")
	target.SetTextContent("start")

	synthetic := &syntheticComposite{CompositeEditCommandBase: NewCompositeBase(doc)}
	cmd := NewInsertTextCommand(doc, target, 5, "-end", EditActionInsertText)
	if err := synthetic.ApplyCommand(cmd); err != nil {
		t.Fatalf("ApplyCommand: %v", err)
	}
	if got := target.TextContent(); got != "start-end" {
		t.Fatalf("after apply: got %q, want %q", got, "start-end")
	}

	step := Wrap(synthetic, "Test")
	if step.Label() != "Test" {
		t.Fatalf("label: got %q, want %q", step.Label(), "Test")
	}
	if step.IsSimpleEditCommandForUndoReporting() {
		t.Fatal("should not be simple by default")
	}
	step.SetSimpleForUndoReporting(true)
	if !step.IsSimpleEditCommandForUndoReporting() {
		t.Fatal("should be simple after SetSimpleForUndoReporting(true)")
	}

	if err := step.Unapply(); err != nil {
		t.Fatalf("step.Unapply: %v", err)
	}
	if got := target.TextContent(); got != "start" {
		t.Fatalf("after step undo: got %q, want %q", got, "start")
	}

	if err := step.Reapply(); err != nil {
		t.Fatalf("step.Reapply: %v", err)
	}
	if got := target.TextContent(); got != "start-end" {
		t.Fatalf("after step redo: got %q, want %q", got, "start-end")
	}
}

// TestEditor_ExecuteUndoRedo verifies the Editor undo/redo stack integration
// with a SimpleEditCommand.
func TestEditor_ExecuteUndoRedo(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	cmd := NewInsertTextCommand(doc, target, 0, "hello", EditActionInsertText)
	if err := editor.ExecuteCommand(cmd); err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	if got := target.TextContent(); got != "hello" {
		t.Fatalf("after execute: got %q, want %q", got, "hello")
	}
	if !editor.CanUndo() {
		t.Fatal("should be able to undo")
	}
	if editor.CanRedo() {
		t.Fatal("should not be able to redo yet")
	}

	if !editor.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := target.TextContent(); got != "" {
		t.Fatalf("after undo: got %q, want empty", got)
	}
	if !editor.CanRedo() {
		t.Fatal("should be able to redo after undo")
	}

	if !editor.Redo() {
		t.Fatal("Redo returned false")
	}
	if got := target.TextContent(); got != "hello" {
		t.Fatalf("after redo: got %q, want %q", got, "hello")
	}
}

// TestEditor_ClearUndoRedo verifies ClearUndoRedo empties both stacks.
func TestEditor_ClearUndoRedo(t *testing.T) {
	editor := newTestEditor()
	doc := editor.Document()
	target := doc.CreateElement("div")
	target.SetTextContent("")

	cmd := NewInsertTextCommand(doc, target, 0, "x", EditActionInsertText)
	_ = editor.ExecuteCommand(cmd)
	if editor.UndoStackSize() != 1 {
		t.Fatalf("undo stack size: got %d, want 1", editor.UndoStackSize())
	}

	editor.ClearUndoRedo()
	if editor.CanUndo() {
		t.Fatal("should not be able to undo after clear")
	}
	if editor.CanRedo() {
		t.Fatal("should not be able to redo after clear")
	}
}
