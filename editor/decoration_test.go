package editor

import (
	"testing"
	"wb-ui/dom"
)

// TestDecorationSet_Empty tests the Empty/Size methods.
func TestDecorationSet_Empty(t *testing.T) {
	ds := NewDecorationSet(nil, false)
	if !ds.Empty() {
		t.Fatal("new set should be empty")
	}
	if ds.Size() != 0 {
		t.Fatalf("empty set size = %d, want 0", ds.Size())
	}
}

// TestDecorationSet_Sort tests that decorations are sorted by position.
func TestDecorationSet_Sort(t *testing.T) {
	// Create decorations in unsorted order.
	decos := []Decoration{
		Mark(10, 15, "c"),
		Mark(0, 5, "a"),
		Mark(5, 10, "b"),
	}
	ds := NewDecorationSet(decos, true)
	if ds.Size() != 3 {
		t.Fatalf("size = %d, want 3", ds.Size())
	}
	iter := ds.Iter()
	// Verify sorted by From.
	if iter[0].From() != 0 {
		t.Fatalf("first from = %d, want 0", iter[0].From())
	}
	if iter[1].From() != 5 {
		t.Fatalf("second from = %d, want 5", iter[1].From())
	}
	if iter[2].From() != 10 {
		t.Fatalf("third from = %d, want 10", iter[2].From())
	}
}

// TestDecorationSet_FindRange tests range-based lookup.
func TestDecorationSet_FindRange(t *testing.T) {
	decos := []Decoration{
		Mark(0, 5, "a"),
		Mark(10, 15, "b"),
		Mark(20, 25, "c"),
	}
	ds := NewDecorationSet(decos, true)

	// Find decorations overlapping [12, 22).
	result := ds.FindRange(12, 22)
	if len(result) != 2 {
		t.Fatalf("FindRange(12,22) = %d decorations, want 2", len(result))
	}
	if result[0].From() != 10 {
		t.Fatalf("first = %d, want 10", result[0].From())
	}
	if result[1].From() != 20 {
		t.Fatalf("second = %d, want 20", result[1].From())
	}

	// Find decorations overlapping [3, 8).
	result = ds.FindRange(3, 8)
	if len(result) != 1 {
		t.Fatalf("FindRange(3,8) = %d decorations, want 1", len(result))
	}
	if result[0].From() != 0 {
		t.Fatalf("first = %d, want 0", result[0].From())
	}
}

// TestDecorationSet_FindAt tests position-based lookup.
func TestDecorationSet_FindAt(t *testing.T) {
	decos := []Decoration{
		Mark(0, 5, "a"),
		Mark(10, 15, "b"),
	}
	ds := NewDecorationSet(decos, true)

	// Position 3 is inside [0, 5).
	result := ds.FindAt(3)
	if len(result) != 1 {
		t.Fatalf("FindAt(3) = %d decorations, want 1", len(result))
	}

	// Position 7 is not inside any decoration.
	result = ds.FindAt(7)
	if len(result) != 0 {
		t.Fatalf("FindAt(7) = %d decorations, want 0", len(result))
	}

	// Position 0 is the start of [0, 5) (inclusive).
	result = ds.FindAt(0)
	if len(result) != 1 {
		t.Fatalf("FindAt(0) = %d decorations, want 1", len(result))
	}

	// Position 5 is the end of [0, 5) (exclusive).
	result = ds.FindAt(5)
	if len(result) != 0 {
		t.Fatalf("FindAt(5) = %d decorations, want 0", len(result))
	}
}

// TestDecorationSet_Add tests adding decorations to an existing set.
func TestDecorationSet_Add(t *testing.T) {
	ds := NewDecorationSet([]Decoration{Mark(0, 5, "a")}, true)
	ds2 := ds.Add([]Decoration{Mark(10, 15, "b")})

	if ds.Size() != 1 {
		t.Fatalf("original size = %d, want 1 (immutability)", ds.Size())
	}
	if ds2.Size() != 2 {
		t.Fatalf("new size = %d, want 2", ds2.Size())
	}
}

// TestDecorationSet_Map_Insert tests that decorations drift when text is
// inserted before them.
func TestDecorationSet_Map_Insert(t *testing.T) {
	// Document: "hello world" (11 chars)
	// Decoration: Mark at [6, 11) ("world")
	ds := NewDecorationSet([]Decoration{Mark(6, 11, "hl")}, true)

	// Insert "big " at position 6 → "hello big world"
	// ChangeDesc: from=6, to=6, insertLen=4
	changes := NewChangeSet([]ChangeDesc{{FromA: 6, ToA: 6, InsertLength: 4}})

	mapped := ds.Map(changes)
	iter := mapped.Iter()
	if len(iter) != 1 {
		t.Fatalf("mapped set has %d decorations, want 1", len(iter))
	}
	// The decoration should have drifted to [10, 15).
	if iter[0].From() != 10 {
		t.Fatalf("mapped from = %d, want 10", iter[0].From())
	}
	if iter[0].To() != 15 {
		t.Fatalf("mapped to = %d, want 15", iter[0].To())
	}
}

// TestDecorationSet_Map_Delete tests that decorations are removed when their
// range is deleted.
func TestDecorationSet_Map_Delete(t *testing.T) {
	// Document: "hello world"
	// Decoration: Mark at [6, 11) ("world")
	ds := NewDecorationSet([]Decoration{Mark(6, 11, "hl")}, true)

	// Delete range [6, 11) → "hello "
	changes := NewChangeSet([]ChangeDesc{{FromA: 6, ToA: 11, InsertLength: 0}})

	mapped := ds.Map(changes)
	if !mapped.Empty() {
		t.Fatalf("mapped set should be empty after deleting the decoration range, got %d decorations", mapped.Size())
	}
}

// TestDecorationSet_Map_PartialDelete tests that decorations are partially
// adjusted when part of their range is deleted.
func TestDecorationSet_Map_PartialDelete(t *testing.T) {
	// Document: "hello world"
	// Decoration: Mark at [0, 11) (whole text)
	ds := NewDecorationSet([]Decoration{Mark(0, 11, "hl")}, true)

	// Delete range [5, 6) (the space) → "helloworld" (10 chars)
	changes := NewChangeSet([]ChangeDesc{{FromA: 5, ToA: 6, InsertLength: 0}})

	mapped := ds.Map(changes)
	iter := mapped.Iter()
	if len(iter) != 1 {
		t.Fatalf("mapped set has %d decorations, want 1", len(iter))
	}
	// The decoration should now be [0, 10).
	if iter[0].From() != 0 {
		t.Fatalf("mapped from = %d, want 0", iter[0].From())
	}
	if iter[0].To() != 10 {
		t.Fatalf("mapped to = %d, want 10", iter[0].To())
	}
}

// TestDecorationSet_Map_NoChanges tests that mapping with an empty ChangeSet
// returns the same set.
func TestDecorationSet_Map_NoChanges(t *testing.T) {
	ds := NewDecorationSet([]Decoration{Mark(0, 5, "a")}, true)
	mapped := ds.Map(NewChangeSet(nil))
	if mapped.Size() != 1 {
		t.Fatalf("mapped size = %d, want 1", mapped.Size())
	}
}

// TestDecorationSet_Filter tests filtering decorations.
func TestDecorationSet_Filter(t *testing.T) {
	ds := NewDecorationSet([]Decoration{
		Mark(0, 5, "a"),
		Mark(10, 15, "b"),
		Mark(20, 25, "a"),
	}, true)

	filtered := ds.Filter(func(d Decoration) bool {
		m, ok := d.(DecorationMark)
		return ok && m.Class == "a"
	})
	if filtered.Size() != 2 {
		t.Fatalf("filtered size = %d, want 2", filtered.Size())
	}
}

// TestDecorationMark_Type tests the Type method.
func TestDecorationMark_Type(t *testing.T) {
	d := Mark(0, 5, "hl")
	if d.Type() != DecorationMarkType {
		t.Fatalf("Mark type = %d, want %d", d.Type(), DecorationMarkType)
	}
}

// TestDecorationWidget_Type tests the Widget decoration type.
func TestDecorationWidget_Type(t *testing.T) {
	// Create a widget with a dummy WidgetType.
	w := &dummyWidget{}
	d := Widget(5, w)
	if d.Type() != DecorationWidgetType {
		t.Fatalf("Widget type = %d, want %d", d.Type(), DecorationWidgetType)
	}
	if d.From() != 5 {
		t.Fatalf("Widget from = %d, want 5", d.From())
	}
	if d.To() != 5 {
		t.Fatalf("Widget to = %d, want 5", d.To())
	}
}

// TestDecorationReplace_Type tests the Replace decoration type.
func TestDecorationReplace_Type(t *testing.T) {
	w := &dummyWidget{}
	d := Replace(0, 5, w)
	if d.Type() != DecorationReplaceType {
		t.Fatalf("Replace type = %d, want %d", d.Type(), DecorationReplaceType)
	}
	if d.From() != 0 {
		t.Fatalf("Replace from = %d, want 0", d.From())
	}
	if d.To() != 5 {
		t.Fatalf("Replace to = %d, want 5", d.To())
	}
}

// TestDecorationLine_Type tests the Line decoration type.
func TestDecorationLine_Type(t *testing.T) {
	d := LineDecoration(0, 10, "active-line")
	if d.Type() != DecorationLineType {
		t.Fatalf("Line type = %d, want %d", d.Type(), DecorationLineType)
	}
}

// TestDecorationSet_Map_WidgetSurvives tests that point decorations (widgets)
// survive even when their position is at the boundary of a deletion.
func TestDecorationSet_Map_WidgetSurvives(t *testing.T) {
	w := &dummyWidget{}
	ds := NewDecorationSet([]Decoration{Widget(5, w)}, true)

	// Delete range [0, 5). The widget at position 5 should survive at
	// position 0.
	changes := NewChangeSet([]ChangeDesc{{FromA: 0, ToA: 5, InsertLength: 0}})

	mapped := ds.Map(changes)
	iter := mapped.Iter()
	if len(iter) != 1 {
		t.Fatalf("mapped set has %d decorations, want 1", len(iter))
	}
	if iter[0].From() != 0 {
		t.Fatalf("widget mapped to %d, want 0", iter[0].From())
	}
}

// TestDecorationSet_Immutability tests that the original set is not modified
// by Map operations.
func TestDecorationSet_Immutability(t *testing.T) {
	ds := NewDecorationSet([]Decoration{Mark(0, 5, "a")}, true)
	changes := NewChangeSet([]ChangeDesc{{FromA: 0, ToA: 0, InsertLength: 3}})

	_ = ds.Map(changes)

	// Original should be unchanged.
	iter := ds.Iter()
	if len(iter) != 1 {
		t.Fatalf("original set size = %d, want 1", len(iter))
	}
	if iter[0].From() != 0 {
		t.Fatalf("original from = %d, want 0", iter[0].From())
	}
	if iter[0].To() != 5 {
		t.Fatalf("original to = %d, want 5", iter[0].To())
	}
}

// dummyWidget is a minimal WidgetType implementation for testing.
type dummyWidget struct {
	BaseWidget
}

func (d *dummyWidget) ToDOM(view interface{}) dom.Node {
	// Returns nil in tests; real widgets return a dom.Node.
	return nil
}
