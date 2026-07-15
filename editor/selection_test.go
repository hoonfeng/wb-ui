package editor

import "testing"

func TestRange_Empty(t *testing.T) {
	r := NewRangeCaret(5)
	if !r.Empty() {
		t.Error("caret should be empty")
	}
	if r.From() != 5 || r.To() != 5 {
		t.Errorf("From/To = %d/%d, want 5/5", r.From(), r.To())
	}
	if r.Anchor() != 5 || r.Head() != 5 {
		t.Errorf("Anchor/Head = %d/%d, want 5/5", r.Anchor(), r.Head())
	}
}

func TestRange_NonEmpty(t *testing.T) {
	// Forward selection: anchor=2, head=5
	r := NewRange(2, 5)
	if r.Empty() {
		t.Error("range should not be empty")
	}
	if r.From() != 2 || r.To() != 5 {
		t.Errorf("From/To = %d/%d, want 2/5", r.From(), r.To())
	}
	if r.Anchor() != 2 || r.Head() != 5 {
		t.Errorf("Anchor/Head = %d/%d, want 2/5", r.Anchor(), r.Head())
	}
}

func TestRange_Backward(t *testing.T) {
	// Backward selection: anchor=5, head=2
	r := NewRange(5, 2)
	if r.Empty() {
		t.Error("range should not be empty")
	}
	// From should be min(anchor, head) = 2
	// To should be max(anchor, head) = 5
	if r.From() != 2 || r.To() != 5 {
		t.Errorf("From/To = %d/%d, want 2/5", r.From(), r.To())
	}
	if r.Anchor() != 5 || r.Head() != 2 {
		t.Errorf("Anchor/Head = %d/%d, want 5/2", r.Anchor(), r.Head())
	}
}

func TestRange_Extend(t *testing.T) {
	r := NewRangeCaret(3)
	r2 := r.Extend(7)
	if r2.Anchor() != 3 || r2.Head() != 7 {
		t.Errorf("Extend: Anchor/Head = %d/%d, want 3/7", r2.Anchor(), r2.Head())
	}
}

func TestRange_Eq(t *testing.T) {
	r1 := NewRange(2, 5)
	r2 := NewRange(2, 5)
	r3 := NewRange(5, 2)
	if !r1.Eq(r2) {
		t.Error("r1 should equal r2")
	}
	if r1.Eq(r3) {
		t.Error("r1 should not equal r3 (different anchor/head)")
	}
}

func TestRange_Bound(t *testing.T) {
	r := NewRange(-5, 100)
	b := r.Bound(50)
	if b.From() != 0 || b.To() != 50 {
		t.Errorf("Bound: From/To = %d/%d, want 0/50", b.From(), b.To())
	}
}

func TestRange_WithAssoc(t *testing.T) {
	r := NewRangeCaret(5).WithAssoc(AssocAfter)
	if r.Assoc() != AssocAfter {
		t.Errorf("Assoc = %d, want %d", r.Assoc(), AssocAfter)
	}
}

func TestEditorSelection_Single(t *testing.T) {
	s := SelectionRange(2, 5)
	if len(s.Ranges()) != 1 {
		t.Fatalf("Ranges() len = %d, want 1", len(s.Ranges()))
	}
	if s.Main().From() != 2 || s.Main().To() != 5 {
		t.Errorf("Main = %+v", s.Main())
	}
}

func TestEditorSelection_Caret(t *testing.T) {
	s := SelectionCaret(10)
	if !s.Main().Empty() {
		t.Error("caret should be empty")
	}
	if s.Main().Head() != 10 {
		t.Errorf("Head = %d, want 10", s.Main().Head())
	}
}

func TestEditorSelection_MultiRange(t *testing.T) {
	r1 := NewRange(0, 5)
	r2 := NewRange(10, 15)
	r3 := NewRange(20, 25)
	s := NewEditorSelection([]Range{r1, r2, r3}, 1) // main = r2
	if len(s.Ranges()) != 3 {
		t.Fatalf("Ranges() len = %d, want 3", len(s.Ranges()))
	}
	if s.MainIndex() != 1 {
		t.Errorf("MainIndex = %d, want 1", s.MainIndex())
	}
	if s.Main().From() != 10 {
		t.Errorf("Main From = %d, want 10", s.Main().From())
	}
}

func TestEditorSelection_AddRange(t *testing.T) {
	s := SelectionCaret(0)
	s2 := s.AddRange(NewRange(5, 10), true) // open=true → new range becomes main
	if len(s2.Ranges()) != 2 {
		t.Fatalf("Ranges() len = %d, want 2", len(s2.Ranges()))
	}
	if s2.MainIndex() != 1 {
		t.Errorf("MainIndex = %d, want 1", s2.MainIndex())
	}
}

func TestEditorSelection_AddRange_NotOpen(t *testing.T) {
	s := SelectionCaret(0)
	s2 := s.AddRange(NewRange(5, 10), false) // open=false → main unchanged
	if s2.MainIndex() != 0 {
		t.Errorf("MainIndex = %d, want 0", s2.MainIndex())
	}
}

func TestEditorSelection_ReplaceRange(t *testing.T) {
	s := NewEditorSelection([]Range{
		NewRange(0, 5),
		NewRange(10, 15),
	}, 0)
	s2 := s.ReplaceRange(NewRange(0, 8), -1) // -1 = main
	if s2.Main().To() != 8 {
		t.Errorf("Main To = %d, want 8", s2.Main().To())
	}
}

func TestEditorSelection_AsSingle(t *testing.T) {
	s := SelectionRange(2, 5)
	r := s.AsSingle()
	if r.From() != 2 || r.To() != 5 {
		t.Errorf("AsSingle = %+v", r)
	}
}

func TestEditorSelection_Eq(t *testing.T) {
	s1 := SelectionRange(2, 5)
	s2 := SelectionRange(2, 5)
	s3 := SelectionRange(3, 5)
	if !s1.Eq(s2) {
		t.Error("s1 should equal s2")
	}
	if s1.Eq(s3) {
		t.Error("s1 should not equal s3")
	}
}

func TestEditorSelection_Empty(t *testing.T) {
	// All carets → empty
	s := NewEditorSelection([]Range{
		NewRangeCaret(0),
		NewRangeCaret(5),
	}, 0)
	if !s.Empty() {
		t.Error("selection with all carets should be empty")
	}
	// Has a non-empty range
	s2 := NewEditorSelection([]Range{
		NewRangeCaret(0),
		NewRange(5, 10),
	}, 0)
	if s2.Empty() {
		t.Error("selection with a range should not be empty")
	}
}

func TestEditorSelection_Immutability(t *testing.T) {
	ranges := []Range{NewRange(0, 5)}
	s := NewEditorSelection(ranges, 0)
	// Modify original slice
	ranges[0] = NewRange(100, 200)
	// s should be unaffected
	if s.Main().From() != 0 {
		t.Errorf("immutability broken: From = %d, want 0", s.Main().From())
	}
}

func TestEditorSelection_Bound(t *testing.T) {
	s := NewEditorSelection([]Range{
		NewRange(-5, 100),
	}, 0)
	b := s.Bound(50)
	if b.Main().From() != 0 || b.Main().To() != 50 {
		t.Errorf("Bound: From/To = %d/%d, want 0/50", b.Main().From(), b.Main().To())
	}
}
