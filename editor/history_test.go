package editor

import (
	"testing"
)

// TestHistory_NewEditClearsRedo tests that a new edit after an undo clears the redo stack.
func TestHistory_NewEditClearsRedo(t *testing.T) {
	v := newTestView("")
	insertText(v, "a", "input.type")
	v.history.CloseTyping()

	v.history.Undo(v)
	if !v.history.CanRedo() {
		t.Fatal("should be able to redo after undo")
	}

	insertText(v, "b", "input.type")
	v.history.CloseTyping()
	if v.history.CanRedo() {
		t.Fatal("redo should be cleared after new edit")
	}
}

// TestHistory_RecordMultipleUndo tests multiple undo steps.
func TestHistory_RecordMultipleUndo(t *testing.T) {
	v := newTestView("")
	insertText(v, "a", "input.type")
	v.history.CloseTyping()
	insertText(v, "b", "input.type")
	v.history.CloseTyping()
	insertText(v, "c", "input.type")
	v.history.CloseTyping()

	if v.state.Doc.String() != "abc" {
		t.Fatalf("doc = %q, want abc", v.state.Doc.String())
	}

	v.history.Undo(v)
	if v.state.Doc.String() != "ab" {
		t.Fatalf("after undo c: doc = %q, want ab", v.state.Doc.String())
	}

	v.history.Undo(v)
	if v.state.Doc.String() != "a" {
		t.Fatalf("after undo b: doc = %q, want a", v.state.Doc.String())
	}

	v.history.Undo(v)
	if v.state.Doc.String() != "" {
		t.Fatalf("after undo a: doc = %q, want empty", v.state.Doc.String())
	}

	v.history.Redo(v)
	if v.state.Doc.String() != "a" {
		t.Fatalf("after redo a: doc = %q, want a", v.state.Doc.String())
	}
}
