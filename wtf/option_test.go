package wtf

import "testing"

func TestOptionSomeNone(t *testing.T) {
	s := Some(42)
	if !s.IsValid() {
		t.Fatalf("Some should be valid")
	}
	if s.IsNone() {
		t.Fatalf("Some should not be none")
	}
	if got := s.Get(); got != 42 {
		t.Fatalf("Get = %d, want 42", got)
	}
	n := None[int]()
	if n.IsValid() {
		t.Fatalf("None should be invalid")
	}
	if !n.IsNone() {
		t.Fatalf("None should report IsNone")
	}
}

func TestOptionGetOr(t *testing.T) {
	s := Some("hi")
	if got := s.GetOr("default"); got != "hi" {
		t.Fatalf("GetOr on Some = %q, want hi", got)
	}
	n := None[string]()
	if got := n.GetOr("default"); got != "default" {
		t.Fatalf("GetOr on None = %q, want default", got)
	}
}

func TestOptionOr(t *testing.T) {
	s := Some(1)
	called := false
	got := s.Or(func() int { called = true; return 99 })
	if got != 1 || called {
		t.Fatalf("Or on Some should not call factory")
	}
	n := None[int]()
	got = n.Or(func() int { return 7 })
	if got != 7 {
		t.Fatalf("Or on None = %d, want 7", got)
	}
}

func TestOptionOrElse(t *testing.T) {
	s := Some("a")
	if got := s.OrElse(Some("b")).Get(); got != "a" {
		t.Fatalf("OrElse on Some = %q, want a", got)
	}
	n := None[string]()
	if got := n.OrElse(Some("b")).Get(); got != "b" {
		t.Fatalf("OrElse on None = %q, want b", got)
	}
}

func TestOptionMapAndThen(t *testing.T) {
	s := Some(3)
	doubled := Map(s, func(x int) int { return x * 2 })
	if !doubled.IsValid() || doubled.Get() != 6 {
		t.Fatalf("Map(Some) = %v, want 6", doubled)
	}
	n := None[int]()
	none2 := Map(n, func(x int) int { return x * 2 })
	if none2.IsValid() {
		t.Fatalf("Map(None) should be None")
	}

	chained := AndThen(Some(2), func(x int) Option[string] {
		if x > 0 {
			return Some("positive")
		}
		return None[string]()
	})
	if !chained.IsValid() || chained.Get() != "positive" {
		t.Fatalf("AndThen = %v, want positive", chained)
	}
}

func TestOptionResetSet(t *testing.T) {
	o := Some(1)
	o.Reset()
	if o.IsValid() {
		t.Fatalf("after Reset should be invalid")
	}
	o.Set(99)
	if !o.IsValid() || o.Get() != 99 {
		t.Fatalf("after Set = %v, want 99", o)
	}
}

func TestOptionGetPanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Get on None should panic")
		}
	}()
	n := None[int]()
	_ = n.Get()
}
