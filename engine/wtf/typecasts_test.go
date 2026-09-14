package wtf

import "testing"

func TestIs(t *testing.T) {
	if got := Is[string]("hello"); !got {
		t.Fatalf("Is[string]('hello') should be true")
	}
	if got := Is[int]("hello"); got {
		t.Fatalf("Is[int]('hello') should be false")
	}
}

func TestDowncast(t *testing.T) {
	var v any = "hello"
	got := Downcast[string](v)
	if got != "hello" {
		t.Fatalf("Downcast = %q, want 'hello'", got)
	}
}

func TestDowncastPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Downcast should panic on type mismatch")
		}
	}()
	var v any = "hello"
	Downcast[int](v)
}

func TestDowncastWithStructTypes(t *testing.T) {
	type Base struct{ Val string }
	type Derived struct{ Base }

	var v any = &Derived{Base: Base{Val: "test"}}

	if !Is[*Derived](v) {
		t.Fatalf("Is[*Derived] should be true")
	}
	if Is[*Base](v) {
		t.Fatalf("Is[*Base] should be false (Go type assertion is exact)")
	}

	d := Downcast[*Derived](v)
	if d.Val != "test" {
		t.Fatalf("Downcast[*Derived].Val = %q, want 'test'", d.Val)
	}
}
