package wtf

import "testing"

func TestAtomStringInterning(t *testing.T) {
	a := NewAtomString("hello")
	b := NewAtomString("hello")
	if !a.Equals(b) {
		t.Fatalf("interned atoms from same value should be equal")
	}
	// Pointer equality is the whole point of interning: two atoms from the same
	// value must share the same interned pointer.
	if a.Impl() != b.Impl() {
		t.Fatalf("Impl pointers should match for same value")
	}
	if got, want := a.String(), "hello"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestAtomStringDifferent(t *testing.T) {
	a := NewAtomString("foo")
	b := NewAtomString("bar")
	if a.Equals(b) {
		t.Fatalf("atoms from different values should not be equal")
	}
	if a.Impl() == b.Impl() {
		t.Fatalf("Impl pointers should differ for different values")
	}
}

func TestAtomStringNullEmpty(t *testing.T) {
	var a AtomString
	if !a.IsNull() {
		t.Fatalf("zero AtomString should be null")
	}
	if !a.IsEmpty() {
		t.Fatalf("zero AtomString should be empty")
	}
	if got := a.String(); got != "" {
		t.Fatalf("String() on null = %q, want empty", got)
	}
}

func TestAtomStringEmptyInterned(t *testing.T) {
	a := NewAtomString("")
	if a.IsNull() {
		t.Fatalf("interned empty string should not be null")
	}
	if !a.IsEmpty() {
		t.Fatalf("interned empty string should be empty")
	}
}

func TestAtomStringEqualsString(t *testing.T) {
	a := NewAtomString("match")
	if !a.EqualsString("match") {
		t.Fatalf("EqualsString(match) = false, want true")
	}
	if a.EqualsString("nope") {
		t.Fatalf("EqualsString(nope) = true, want false")
	}
}

func TestAtomStringFromString(t *testing.T) {
	s := NewString("from-wtf-string")
	a := NewAtomStringFromString(s)
	if got, want := a.String(), "from-wtf-string"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	// Interning should bridge String->AtomString and plain value paths.
	b := NewAtomString("from-wtf-string")
	if !a.Equals(b) {
		t.Fatalf("NewAtomStringFromString should intern consistently with NewAtomString")
	}
}

func TestAtomStringConcurrentInterning(t *testing.T) {
	// Many goroutines interning the same value must all observe the same pointer.
	const value = "concurrent-value"
	done := make(chan AtomString, 16)
	for i := 0; i < 16; i++ {
		go func() { done <- NewAtomString(value) }()
	}
	first := <-done
	for i := 0; i < 15; i++ {
		other := <-done
		if first.Impl() != other.Impl() {
			t.Fatalf("concurrent interning produced different pointers")
		}
	}
}
