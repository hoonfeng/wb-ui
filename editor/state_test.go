package editor

import "testing"

func TestEditorState_Create(t *testing.T) {
	state := NewState(Config{
		Doc: TextFromString("hello world"),
	})
	if state.Doc.Length() != 11 {
		t.Errorf("Doc Length = %d, want 11", state.Doc.Length())
	}
	if state.Selection.Main().Head() != 0 {
		t.Errorf("Selection Head = %d, want 0", state.Selection.Main().Head())
	}
}

func TestEditorState_CreateWithSelection(t *testing.T) {
	sel := SelectionRange(0, 5)
	state := NewState(Config{
		Doc:       TextFromString("hello"),
		Selection: &sel,
	})
	if state.Selection.Main().From() != 0 || state.Selection.Main().To() != 5 {
		t.Errorf("Selection = %+v", state.Selection.Main())
	}
}

func TestEditorState_Update_Insert(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("hello")})
	// Insert " world" at position 5
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 6}},
			[]Text{TextFromString(" world")},
		),
	}
	newState, tr := state.Update(spec)
	// Old state should be unchanged
	if state.Doc.String() != "hello" {
		t.Errorf("Old state Doc = %q (should be unchanged)", state.Doc.String())
	}
	// New state should have the updated document
	if newState.Doc.String() != "hello world" {
		t.Errorf("New state Doc = %q, want %q", newState.Doc.String(), "hello world")
	}
	// Transaction should record the changes
	if tr.Changes().Empty() {
		t.Error("Transaction changes should not be empty")
	}
}

func TestEditorState_Update_SelectionMapped(t *testing.T) {
	// Caret at position 3; insert 2 chars at position 0
	// → caret should move to position 5
	state := NewState(Config{
		Doc:       TextFromString("hello"),
		Selection: &[]EditorSelection{SelectionCaret(3)}[0],
	})
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 0, ToA: 0, InsertLength: 2}},
			[]Text{TextFromString("ab")},
		),
	}
	newState, _ := state.Update(spec)
	if newState.Selection.Main().Head() != 5 {
		t.Errorf("Selection Head = %d, want 5 (mapped)", newState.Selection.Main().Head())
	}
}

func TestEditorState_Update_SelectionExplicit(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("hello")})
	newSel := SelectionCaret(2)
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 0, ToA: 5, InsertLength: 2}},
			[]Text{TextFromString("hi")},
		),
		Selection:    &newSel,
		HasSelection: true,
	}
	newState, _ := state.Update(spec)
	if newState.Selection.Main().Head() != 2 {
		t.Errorf("Selection Head = %d, want 2 (explicit)", newState.Selection.Main().Head())
	}
}

func TestEditorState_Update_Delete(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("hello world")})
	// Delete " world" (positions 5-11)
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 5, ToA: 11, InsertLength: 0}},
			[]Text{Empty()},
		),
	}
	newState, _ := state.Update(spec)
	if newState.Doc.String() != "hello" {
		t.Errorf("Doc = %q, want %q", newState.Doc.String(), "hello")
	}
}

func TestEditorState_Apply(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("abc")})
	// Create a transaction
	tr := NewTransaction(&state, TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 3, ToA: 3, InsertLength: 1}},
			[]Text{TextFromString("d")},
		),
	})
	newState := state.Apply(tr)
	if newState.Doc.String() != "abcd" {
		t.Errorf("Doc = %q, want %q", newState.Doc.String(), "abcd")
	}
}

func TestEditorState_Immutability(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("hello")})
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 1}},
			[]Text{TextFromString("!")},
		),
	}
	newState, _ := state.Update(spec)
	// Original state should be unchanged
	if state.Doc.String() != "hello" {
		t.Errorf("Original Doc = %q (mutated!)", state.Doc.String())
	}
	if newState.Doc.String() != "hello!" {
		t.Errorf("New Doc = %q, want %q", newState.Doc.String(), "hello!")
	}
}

func TestEditorState_Field(t *testing.T) {
	// Create a StateField that counts characters
	counter := NewStateField(
		func(state EditorState) interface{} {
			return state.Doc.Length()
		},
		func(value interface{}, tr Transaction) interface{} {
			return value.(int) + tr.Changes().Length()
		},
	)
	state := NewState(Config{
		Doc:        TextFromString("hello"),
		Extensions: []Extension{counter},
	})
	v := state.Field(counter)
	if v.(int) != 5 {
		t.Errorf("Field = %v, want 5", v)
	}
	// Update: insert "!" (1 char)
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 1}},
			[]Text{TextFromString("!")},
		),
	}
	newState, _ := state.Update(spec)
	v = newState.Field(counter)
	if v.(int) != 6 {
		t.Errorf("Field after update = %v, want 6", v)
	}
}

func TestEditorState_Facet(t *testing.T) {
	// Create a facet that collects booleans and ANDs them
	allowMultiple := NewFacet(
		func(inputs []interface{}) interface{} {
			for _, v := range inputs {
				if b, ok := v.(bool); ok && b {
					return true
				}
			}
			return false
		},
		[]interface{}{true},
	)
	state := NewState(Config{
		Doc:        TextFromString("hello"),
		Extensions: []Extension{allowMultiple},
	})
	v := state.Facet(allowMultiple)
	if v != true {
		t.Errorf("Facet = %v, want true", v)
	}
}

func TestEditorState_SequentialUpdates(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("")})
	// Type "a"
	spec1 := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 0, ToA: 0, InsertLength: 1}},
			[]Text{TextFromString("a")},
		),
	}
	state, _ = state.Update(spec1)
	if state.Doc.String() != "a" {
		t.Errorf("After 1st update: %q", state.Doc.String())
	}
	// Type "b"
	spec2 := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 1, ToA: 1, InsertLength: 1}},
			[]Text{TextFromString("b")},
		),
	}
	state, _ = state.Update(spec2)
	if state.Doc.String() != "ab" {
		t.Errorf("After 2nd update: %q", state.Doc.String())
	}
	// Type "c"
	spec3 := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 2, ToA: 2, InsertLength: 1}},
			[]Text{TextFromString("c")},
		),
	}
	state, _ = state.Update(spec3)
	if state.Doc.String() != "abc" {
		t.Errorf("After 3rd update: %q", state.Doc.String())
	}
}
