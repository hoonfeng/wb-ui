package wtf

import "testing"

func TestVectorAppendAndSize(t *testing.T) {
	v := NewVector[int]()
	if !v.IsEmpty() {
		t.Fatalf("expected empty vector, got size %d", v.Size())
	}
	v.Append(1)
	v.Append(2)
	v.Append(3)
	if got, want := v.Size(), 3; got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
	if got, want := v.First(), 1; got != want {
		t.Fatalf("First = %v, want %v", got, want)
	}
	if got, want := v.Last(), 3; got != want {
		t.Fatalf("Last = %v, want %v", got, want)
	}
}

func TestVectorAtSet(t *testing.T) {
	v := NewVector("a", "b", "c")
	if got, want := v.At(1), "b"; got != want {
		t.Fatalf("At(1) = %q, want %q", got, want)
	}
	v.Set(1, "B")
	if got, want := v.At(1), "B"; got != want {
		t.Fatalf("At(1) after Set = %q, want %q", got, want)
	}
}

func TestVectorRemoveAndShrink(t *testing.T) {
	v := NewVector(1, 2, 3, 4, 5)
	v.Remove(2) // remove 3
	if got, want := v.Size(), 4; got != want {
		t.Fatalf("Size after Remove = %d, want %d", got, want)
	}
	if got, want := v.At(2), 4; got != want {
		t.Fatalf("At(2) after Remove = %v, want %v", got, want)
	}
	v.Shrink(2)
	if got, want := v.Size(), 2; got != want {
		t.Fatalf("Size after Shrink = %d, want %d", got, want)
	}
	if got, want := v.At(1), 2; got != want {
		t.Fatalf("At(1) after Shrink = %v, want %v", got, want)
	}
}

func TestVectorResize(t *testing.T) {
	v := NewVector(1, 2)
	v.Resize(4)
	if got, want := v.Size(), 4; got != want {
		t.Fatalf("Size after Resize = %d, want %d", got, want)
	}
	if got := v.At(2); got != 0 {
		t.Fatalf("new element = %v, want zero 0", got)
	}
	v.Resize(1)
	if got, want := v.Size(), 1; got != want {
		t.Fatalf("Size after Resize down = %d, want %d", got, want)
	}
}

func TestVectorFindContains(t *testing.T) {
	v := NewVector(1, 2, 3, 2, 1)
	eq := Equals[int]
	if got, want := v.Find(2, eq), 1; got != want {
		t.Fatalf("Find(2) = %d, want %d", got, want)
	}
	if got, want := v.Find(42, eq), NotFound; got != want {
		t.Fatalf("Find(42) = %d, want %d", got, want)
	}
	if !v.Contains(3, eq) {
		t.Fatalf("Contains(3) = false, want true")
	}
	if v.Contains(99, eq) {
		t.Fatalf("Contains(99) = true, want false")
	}
}

func TestVectorRemoveFirst(t *testing.T) {
	v := NewVector(1, 2, 3, 2)
	eq := Equals[int]
	if !v.RemoveFirst(2, eq) {
		t.Fatalf("RemoveFirst(2) = false, want true")
	}
	if got, want := v.At(1), 3; got != want {
		t.Fatalf("At(1) after RemoveFirst = %v, want %v", got, want)
	}
	if v.RemoveFirst(42, eq) {
		t.Fatalf("RemoveFirst(42) = true, want false")
	}
}

func TestVectorReverse(t *testing.T) {
	v := NewVector(1, 2, 3, 4)
	v.Reverse()
	want := []int{4, 3, 2, 1}
	for i, w := range want {
		if got := v.At(i); got != w {
			t.Fatalf("At(%d) after Reverse = %v, want %v", i, got, w)
		}
	}
}

func TestVectorSort(t *testing.T) {
	v := NewVector(3, 1, 4, 1, 5, 9, 2, 6)
	v.Sort(func(a, b int) bool { return a < b })
	want := []int{1, 1, 2, 3, 4, 5, 6, 9}
	for i, w := range want {
		if got := v.At(i); got != w {
			t.Fatalf("At(%d) after Sort = %v, want %v", i, got, w)
		}
	}
}

func TestVectorAppendVectorAndSlice(t *testing.T) {
	v := NewVector(1, 2)
	v.AppendSlice([]int{3, 4})
	other := NewVector(5, 6)
	v.AppendVector(other)
	if got, want := v.Size(), 6; got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
	want := []int{1, 2, 3, 4, 5, 6}
	for i, w := range want {
		if got := v.At(i); got != w {
			t.Fatalf("At(%d) = %v, want %v", i, got, w)
		}
	}
}

func TestVectorTakeLastClear(t *testing.T) {
	v := NewVector(1, 2, 3)
	if got, want := v.TakeLast(), 3; got != want {
		t.Fatalf("TakeLast = %v, want %v", got, want)
	}
	if got := v.Size(); got != 2 {
		t.Fatalf("Size after TakeLast = %d, want 2", got)
	}
	v.Clear()
	if !v.IsEmpty() {
		t.Fatalf("expected empty after Clear, got size %d", v.Size())
	}
}

func TestVectorReserveCapacityAndClone(t *testing.T) {
	v := NewVectorWithCapacity[int](8)
	v.Append(1)
	v.Append(2)
	if got := v.Capacity(); got < 8 {
		t.Fatalf("Capacity = %d, want >= 8", got)
	}
	v.ReserveCapacity(16)
	if got := v.Capacity(); got < 16 {
		t.Fatalf("Capacity after reserve = %d, want >= 16", got)
	}
	c := v.Clone()
	if c.At(0) != 1 || c.At(1) != 2 {
		t.Fatalf("Clone contents wrong: %v %v", c.At(0), c.At(1))
	}
	c.Set(0, 99)
	if v.At(0) == 99 {
		t.Fatalf("Clone should not alias backing storage")
	}
}

func TestVectorSwapWith(t *testing.T) {
	a := NewVector(1, 2)
	b := NewVector(3, 4, 5)
	a.SwapWith(&b)
	if a.Size() != 3 || b.Size() != 2 {
		t.Fatalf("Swap sizes wrong: a=%d b=%d", a.Size(), b.Size())
	}
	if a.First() != 3 || b.First() != 1 {
		t.Fatalf("Swap contents wrong")
	}
}
