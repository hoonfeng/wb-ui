package editor

import "testing"

func TestChangeDesc_Empty(t *testing.T) {
	c := ChangeDesc{FromA: 0, ToA: 0, InsertLength: 0}
	if !c.Empty() {
		t.Error("zero change should be empty")
	}
}

func TestChangeDesc_LenDelta(t *testing.T) {
	// Delete 5 runes, insert 3 → delta = -2
	c := ChangeDesc{FromA: 0, ToA: 5, InsertLength: 3}
	if c.LenDelta() != -2 {
		t.Errorf("LenDelta = %d, want -2", c.LenDelta())
	}
	// Pure insertion: delete 0, insert 5 → delta = +5
	c = ChangeDesc{FromA: 0, ToA: 0, InsertLength: 5}
	if c.LenDelta() != 5 {
		t.Errorf("LenDelta = %d, want 5", c.LenDelta())
	}
	// Pure deletion: delete 5, insert 0 → delta = -5
	c = ChangeDesc{FromA: 0, ToA: 5, InsertLength: 0}
	if c.LenDelta() != -5 {
		t.Errorf("LenDelta = %d, want -5", c.LenDelta())
	}
}

func TestChangeSet_Empty(t *testing.T) {
	cs := ChangeSet{}
	if !cs.Empty() {
		t.Error("empty ChangeSet should be Empty()")
	}
	if cs.Length() != 0 {
		t.Errorf("Length = %d, want 0", cs.Length())
	}
}

func TestChangeSet_Apply_Insert(t *testing.T) {
	// Insert "world" at position 6 in "hello "
	doc := TextFromString("hello ")
	cs := NewChangeSetWithText(
		[]ChangeDesc{{FromA: 6, ToA: 6, InsertLength: 5}},
		[]Text{TextFromString("world")},
	)
	result := cs.Apply(doc)
	if result.String() != "hello world" {
		t.Errorf("Apply = %q, want %q", result.String(), "hello world")
	}
}

func TestChangeSet_Apply_Replace(t *testing.T) {
	// Replace "world" (positions 6-11) with "Go" in "hello world"
	doc := TextFromString("hello world")
	cs := NewChangeSetWithText(
		[]ChangeDesc{{FromA: 6, ToA: 11, InsertLength: 2}},
		[]Text{TextFromString("Go")},
	)
	result := cs.Apply(doc)
	if result.String() != "hello Go" {
		t.Errorf("Apply = %q, want %q", result.String(), "hello Go")
	}
}

func TestChangeSet_Apply_Delete(t *testing.T) {
	// Delete positions 5-11 (remove " world") from "hello world"
	doc := TextFromString("hello world")
	cs := NewChangeSetWithText(
		[]ChangeDesc{{FromA: 5, ToA: 11, InsertLength: 0}},
		[]Text{Empty()},
	)
	result := cs.Apply(doc)
	if result.String() != "hello" {
		t.Errorf("Apply = %q, want %q", result.String(), "hello")
	}
}

func TestChangeSet_Apply_MultipleChanges(t *testing.T) {
	// Apply two changes: insert at 0 and replace at end
	doc := TextFromString("hello")
	cs := NewChangeSetWithText(
		[]ChangeDesc{
			{FromA: 0, ToA: 0, InsertLength: 2},  // insert ">>" at start
			{FromA: 5, ToA: 5, InsertLength: 2},  // insert "<<" at end
		},
		[]Text{TextFromString(">>"), TextFromString("<<")},
	)
	result := cs.Apply(doc)
	if result.String() != ">>hello<<" {
		t.Errorf("Apply = %q, want %q", result.String(), ">>hello<<")
	}
}

func TestChangeSet_MapPos_BeforeChange(t *testing.T) {
	// Insert at position 5; position 3 should be unchanged
	cs := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 3}})
	if cs.MapPos(3, 1) != 3 {
		t.Errorf("MapPos(3) = %d, want 3", cs.MapPos(3, 1))
	}
}

func TestChangeSet_MapPos_AfterChange(t *testing.T) {
	// Insert 3 runes at position 5; position 10 should map to 13
	cs := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 3}})
	if cs.MapPos(10, 1) != 13 {
		t.Errorf("MapPos(10) = %d, want 13", cs.MapPos(10, 1))
	}
}

func TestChangeSet_MapPos_AtInsertionAssocAfter(t *testing.T) {
	// Insert "XYZ" at position 5 (replace nothing); position 5 with assoc=1
	// should map to 5+3=8 (after the insertion)
	cs := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 3}})
	if cs.MapPos(5, 1) != 8 {
		t.Errorf("MapPos(5, assoc=1) = %d, want 8", cs.MapPos(5, 1))
	}
}

func TestChangeSet_MapPos_AtInsertionAssocBefore(t *testing.T) {
	// Insert "XYZ" at position 5 (replace nothing); position 5 with assoc=-1
	// should map to 5 (before the insertion)
	cs := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 5, InsertLength: 3}})
	if cs.MapPos(5, -1) != 5 {
		t.Errorf("MapPos(5, assoc=-1) = %d, want 5", cs.MapPos(5, -1))
	}
}

func TestChangeSet_MapPos_AtDeletionBoundary(t *testing.T) {
	// Delete positions 5-10; position 10 (at end of deletion)
	// with assoc=1 → maps to 5+0=5 (end of insertion = start, since nothing inserted)
	// with assoc=-1 → maps to 5 (start of deletion)
	cs := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 10, InsertLength: 0}})
	if cs.MapPos(10, 1) != 5 {
		t.Errorf("MapPos(10, assoc=1) = %d, want 5", cs.MapPos(10, 1))
	}
	if cs.MapPos(10, -1) != 5 {
		t.Errorf("MapPos(10, assoc=-1) = %d, want 5", cs.MapPos(10, -1))
	}
	// Position 5 (at start of deletion)
	if cs.MapPos(5, 1) != 5 {
		t.Errorf("MapPos(5, assoc=1) = %d, want 5", cs.MapPos(5, 1))
	}
}

func TestChangeSet_Length(t *testing.T) {
	// Insert 3 + delete 5 = net -2
	cs := NewChangeSet([]ChangeDesc{
		{FromA: 0, ToA: 0, InsertLength: 3},
		{FromA: 10, ToA: 15, InsertLength: 0},
	})
	if cs.Length() != -2 {
		t.Errorf("Length = %d, want -2", cs.Length())
	}
}

func TestChangeSet_Invert(t *testing.T) {
	// Replace "world" with "Go" → invert should restore
	doc := TextFromString("hello world")
	cs := NewChangeSetWithText(
		[]ChangeDesc{{FromA: 6, ToA: 11, InsertLength: 2}},
		[]Text{TextFromString("Go")},
	)
	result := cs.Apply(doc)
	if result.String() != "hello Go" {
		t.Fatalf("Apply = %q, want %q", result.String(), "hello Go")
	}
	// Invert and apply to restore original
	inverted := cs.Invert(doc)
	restored := inverted.Apply(result)
	if restored.String() != "hello world" {
		t.Errorf("Invert+Apply = %q, want %q", restored.String(), "hello world")
	}
}

func TestTransaction_New(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("hello world")})
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 6, ToA: 11, InsertLength: 2}},
			[]Text{TextFromString("Go")},
		),
	}
	tr := NewTransaction(&state, spec)
	if tr.Changes().Empty() {
		t.Error("transaction changes should not be empty")
	}
}

func TestTransaction_UserEvent(t *testing.T) {
	state := NewState(Config{Doc: TextFromString("abc")})
	spec := TransactionSpec{
		Changes: NewChangeSetWithText(
			[]ChangeDesc{{FromA: 3, ToA: 3, InsertLength: 1}},
			[]Text{TextFromString("d")},
		),
		UserEvent: "input.type",
	}
	tr := NewTransaction(&state, spec)
	if tr.UserEvent() != "input.type" {
		t.Errorf("UserEvent = %q", tr.UserEvent())
	}
	if tr.Annotation("userEvent") != "input.type" {
		t.Errorf("Annotation(userEvent) = %q", tr.Annotation("userEvent"))
	}
}
