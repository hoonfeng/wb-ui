package wtf

import "testing"

func TestHashSetAddContainsSize(t *testing.T) {
	s := NewHashSet[int]()
	if !s.IsEmpty() {
		t.Fatalf("expected empty set")
	}
	r := s.Add(1)
	if !r.IsNewEntry {
		t.Fatalf("Add on missing should be new entry")
	}
	r = s.Add(1)
	if r.IsNewEntry {
		t.Fatalf("Add duplicate should not be new entry")
	}
	s.Add(2)
	s.Add(3)
	if got, want := s.Size(), 3; got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
	if !s.Contains(2) {
		t.Fatalf("Contains(2) = false")
	}
	if s.Contains(42) {
		t.Fatalf("Contains(42) = true, want false")
	}
}

func TestHashSetRemoveClear(t *testing.T) {
	s := NewHashSetFrom(1, 2, 3)
	if !s.Remove(2) {
		t.Fatalf("Remove(2) = false, want true")
	}
	if s.Contains(2) {
		t.Fatalf("Contains(2) after remove = true")
	}
	if s.Remove(42) {
		t.Fatalf("Remove(missing) = true, want false")
	}
	s.Clear()
	if !s.IsEmpty() {
		t.Fatalf("expected empty after Clear")
	}
}

func TestHashSetRemoveIf(t *testing.T) {
	s := NewHashSetFrom(1, 2, 3, 4, 5)
	n := s.RemoveIf(func(v int) bool { return v > 3 })
	if n != 2 {
		t.Fatalf("RemoveIf removed %d, want 2", n)
	}
	if s.Contains(4) || s.Contains(5) {
		t.Fatalf("4 and 5 should be removed")
	}
	if !s.Contains(3) {
		t.Fatalf("3 should remain")
	}
}

func TestHashSetTakeValuesForEach(t *testing.T) {
	s := NewHashSetFrom(1, 2, 3)
	v, ok := s.Take()
	if !ok {
		t.Fatalf("Take = false, want true")
	}
	if s.Contains(v) {
		t.Fatalf("Take should have removed the returned value, but %v is still present", v)
	}
	if got := s.Size(); got != 2 {
		t.Fatalf("Size after Take = %d, want 2", got)
	}
	seen := map[int]bool{}
	s.ForEach(func(v int) { seen[v] = true })
	if len(seen) != 2 {
		t.Fatalf("ForEach visited %d, want 2", len(seen))
	}
}

func TestHashSetSetAlgebra(t *testing.T) {
	a := NewHashSetFrom(1, 2, 3)
	b := NewHashSetFrom(2, 3, 4)

	u := a.UnionWith(b)
	if u.Size() != 4 {
		t.Fatalf("Union size = %d, want 4", u.Size())
	}
	if !u.Contains(1) || !u.Contains(4) {
		t.Fatalf("Union missing elements")
	}

	i := a.IntersectionWith(b)
	if i.Size() != 2 {
		t.Fatalf("Intersection size = %d, want 2", i.Size())
	}
	if !i.Contains(2) || !i.Contains(3) {
		t.Fatalf("Intersection missing 2 and 3")
	}
	if i.Contains(1) || i.Contains(4) {
		t.Fatalf("Intersection should not have 1 or 4")
	}

	d := a.DifferenceWith(b)
	if d.Size() != 1 || !d.Contains(1) {
		t.Fatalf("Difference = %v, want {1}", d.Values())
	}

	xor := a.SymmetricDifferenceWith(b)
	if xor.Size() != 2 || !xor.Contains(1) || !xor.Contains(4) {
		t.Fatalf("SymDiff = %v, want {1,4}", xor.Values())
	}
}

func TestHashSetFormIntersectionDifference(t *testing.T) {
	a := NewHashSetFrom(1, 2, 3, 4)
	b := NewHashSetFrom(2, 3, 5)
	a.FormIntersection(b)
	if a.Size() != 2 || !a.Contains(2) || !a.Contains(3) {
		t.Fatalf("FormIntersection = %v, want {2,3}", a.Values())
	}

	c := NewHashSetFrom(1, 2, 3)
	d := NewHashSetFrom(2)
	c.FormDifference(d)
	if c.Size() != 2 || !c.Contains(1) || !c.Contains(3) || c.Contains(2) {
		t.Fatalf("FormDifference = %v, want {1,3}", c.Values())
	}
}

func TestHashSetSwapWith(t *testing.T) {
	a := NewHashSetFrom(1, 2)
	b := NewHashSetFrom(3, 4, 5)
	a.SwapWith(&b)
	if a.Size() != 3 || b.Size() != 2 {
		t.Fatalf("Swap sizes wrong: a=%d b=%d", a.Size(), b.Size())
	}
	if !a.Contains(5) || !b.Contains(1) {
		t.Fatalf("Swap contents wrong")
	}
}
