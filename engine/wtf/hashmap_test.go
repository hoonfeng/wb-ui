package wtf

import "testing"

func TestHashMapSetAddGet(t *testing.T) {
	h := NewHashMap[string, int]()
	if !h.IsEmpty() {
		t.Fatalf("expected empty map")
	}
	r := h.Set("a", 1)
	if !r.IsNewEntry {
		t.Fatalf("Set on missing key should be new entry")
	}
	r = h.Set("a", 2)
	if r.IsNewEntry {
		t.Fatalf("Set on existing key should not be new entry")
	}
	if got, _ := h.Get("a"); got != 2 {
		t.Fatalf("Get(a) = %d, want 2", got)
	}
	if r := h.Add("a", 3); r.IsNewEntry {
		t.Fatalf("Add on existing key should not be new entry")
	}
	if got, _ := h.Get("a"); got != 2 {
		t.Fatalf("Add should not overwrite; got %d want 2", got)
	}
	if r := h.Add("b", 9); !r.IsNewEntry {
		t.Fatalf("Add on missing key should be new entry")
	}
	if got, want := h.Size(), 2; got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
}

func TestHashMapContainsRemoveClear(t *testing.T) {
	h := NewHashMap[string, int]()
	h.Set("a", 1)
	h.Set("b", 2)
	if !h.Contains("a") {
		t.Fatalf("Contains(a) = false, want true")
	}
	if !h.Remove("a") {
		t.Fatalf("Remove(a) = false, want true")
	}
	if h.Contains("a") {
		t.Fatalf("Contains(a) after remove = true")
	}
	if h.Remove("zzz") {
		t.Fatalf("Remove(missing) = true, want false")
	}
	h.Clear()
	if !h.IsEmpty() {
		t.Fatalf("expected empty after Clear")
	}
}

func TestHashMapGetOrTakeEnsure(t *testing.T) {
	h := NewHashMap[string, int]()
	h.Set("a", 1)
	if got := h.GetOr("a", 99); got != 1 {
		t.Fatalf("GetOr existing = %d, want 1", got)
	}
	if got := h.GetOr("z", 99); got != 99 {
		t.Fatalf("GetOr missing = %d, want 99", got)
	}
	v, r := h.Ensure("c", func() int { return 42 })
	if !r.IsNewEntry || v != 42 {
		t.Fatalf("Ensure new = (%d, %v), want (42, true)", v, r)
	}
	v, r = h.Ensure("c", func() int { return 999 })
	if r.IsNewEntry || v != 42 {
		t.Fatalf("Ensure existing = (%d, %v), want (42, false)", v, r)
	}
	got, ok := h.Take("a")
	if !ok || got != 1 {
		t.Fatalf("Take(a) = (%d, %v), want (1, true)", got, ok)
	}
	if h.Contains("a") {
		t.Fatalf("Take should remove entry")
	}
}

func TestHashMapKeysValuesForEach(t *testing.T) {
	h := NewHashMap[string, int]()
	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	keys := h.Keys()
	if len(keys) != 3 {
		t.Fatalf("len(Keys) = %d, want 3", len(keys))
	}
	vals := h.Values()
	if len(vals) != 3 {
		t.Fatalf("len(Values) = %d, want 3", len(vals))
	}
	seen := map[string]int{}
	h.ForEach(func(k string, v int) { seen[k] = v })
	if seen["a"] != 1 || seen["b"] != 2 || seen["c"] != 3 {
		t.Fatalf("ForEach seen = %v", seen)
	}
}

func TestHashMapRemoveIf(t *testing.T) {
	h := NewHashMap[string, int]()
	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	n := h.RemoveIf(func(k string, v int) bool { return v%2 == 1 })
	if n != 2 {
		t.Fatalf("RemoveIf removed %d, want 2", n)
	}
	if h.Contains("a") || h.Contains("c") {
		t.Fatalf("odd values should be removed")
	}
	if !h.Contains("b") {
		t.Fatalf("even value should remain")
	}
}

func TestHashMapSwapWith(t *testing.T) {
	a := NewHashMap[string, int]()
	a.Set("x", 1)
	b := NewHashMap[string, int]()
	b.Set("y", 2)
	a.SwapWith(&b)
	if _, ok := a.Get("y"); !ok {
		t.Fatalf("swap: a should have y")
	}
	if _, ok := b.Get("x"); !ok {
		t.Fatalf("swap: b should have x")
	}
}
