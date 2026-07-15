package editor

import (
	"testing"
)

// selPtr returns a pointer to an EditorSelection.
func selPtr(s EditorSelection) *EditorSelection { return &s }

// TestHandleKey_Printable tests that HandleKey inserts printable text.
func TestHandleKey_Printable(t *testing.T) {
	v := newTestView("")
	km := DefaultKeymap()
	handled := HandleKey(v, KeyEvent{Key: "H", Code: "KeyH", Text: "H"}, km)
	if !handled {
		t.Fatal("HandleKey should handle printable key")
	}
	if v.state.Doc.String() != "H" {
		t.Fatalf("doc = %q, want H", v.state.Doc.String())
	}
}

// TestHandleKey_Enter tests that Enter inserts a newline.
func TestHandleKey_Enter(t *testing.T) {
	v := newTestView("abc")
	km := DefaultKeymap()
	v.Dispatch(TransactionSpec{
		Selection:    selPtr(SelectionCaret(3)),
		HasSelection: true,
	})
	handled := HandleKey(v, KeyEvent{Key: "Enter", Code: "Enter"}, km)
	if !handled {
		t.Fatal("HandleKey should handle Enter")
	}
	if v.state.Doc.String() != "abc\n" {
		t.Fatalf("doc = %q, want abc\n", v.state.Doc.String())
	}
}

// TestHandleKey_Backspace tests Backspace deletes backward.
func TestHandleKey_Backspace(t *testing.T) {
	v := newTestView("hello")
	km := DefaultKeymap()
	v.Dispatch(TransactionSpec{
		Selection:    selPtr(SelectionCaret(5)),
		HasSelection: true,
	})
	handled := HandleKey(v, KeyEvent{Key: "Backspace", Code: "Backspace"}, km)
	if !handled {
		t.Fatal("HandleKey should handle Backspace")
	}
	if v.state.Doc.String() != "hell" {
		t.Fatalf("doc = %q, want hell", v.state.Doc.String())
	}
}

// TestHandleKey_ArrowLeft tests ArrowLeft moves cursor left.
func TestHandleKey_ArrowLeft(t *testing.T) {
	v := newTestView("abc")
	km := DefaultKeymap()
	v.Dispatch(TransactionSpec{
		Selection:    selPtr(SelectionCaret(3)),
		HasSelection: true,
	})
	handled := HandleKey(v, KeyEvent{Key: "ArrowLeft", Code: "ArrowLeft"}, km)
	if !handled {
		t.Fatal("HandleKey should handle ArrowLeft")
	}
	if v.state.Selection.Main().From() != 2 {
		t.Fatalf("cursor pos = %d, want 2", v.state.Selection.Main().From())
	}
}

// TestHandleKey_ArrowRight tests ArrowRight moves cursor right.
func TestHandleKey_ArrowRight(t *testing.T) {
	v := newTestView("abc")
	km := DefaultKeymap()
	handled := HandleKey(v, KeyEvent{Key: "ArrowRight", Code: "ArrowRight"}, km)
	if !handled {
		t.Fatal("HandleKey should handle ArrowRight")
	}
	if v.state.Selection.Main().From() != 1 {
		t.Fatalf("cursor pos = %d, want 1", v.state.Selection.Main().From())
	}
}

// TestHandleKey_ModifierCombos tests Ctrl+A, Ctrl+Z, Ctrl+Y.
func TestHandleKey_ModifierCombos(t *testing.T) {
	v := newTestView("hello world")
	km := DefaultKeymap()

	handled := HandleKey(v, KeyEvent{Key: "a", Code: "KeyA", Ctrl: true}, km)
	if !handled {
		t.Fatal("HandleKey should handle Ctrl+A")
	}
	sel := v.state.Selection.Main()
	if sel.From() != 0 || sel.To() != 11 {
		t.Fatalf("Ctrl+A selection = [%d,%d], want [0,11]", sel.From(), sel.To())
	}

	v.Dispatch(TransactionSpec{
		Selection:    selPtr(SelectionCaret(11)),
		HasSelection: true,
	})
	insertText(v, "!", "input.type")
	v.history.CloseTyping()
	if v.state.Doc.String() != "hello world!" {
		t.Fatalf("doc = %q, want hello world!", v.state.Doc.String())
	}

	handled = HandleKey(v, KeyEvent{Key: "z", Code: "KeyZ", Ctrl: true}, km)
	if !handled {
		t.Fatal("HandleKey should handle Ctrl+Z")
	}
	if v.state.Doc.String() != "hello world" {
		t.Fatalf("after Ctrl+Z: doc = %q, want hello world", v.state.Doc.String())
	}

	handled = HandleKey(v, KeyEvent{Key: "y", Code: "KeyY", Ctrl: true}, km)
	if !handled {
		t.Fatal("HandleKey should handle Ctrl+Y")
	}
	if v.state.Doc.String() != "hello world!" {
		t.Fatalf("after Ctrl+Y: doc = %q, want hello world!", v.state.Doc.String())
	}
}
