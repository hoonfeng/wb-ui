package wtf

import (
	"testing"
)

func TestWeakPtrValid(t *testing.T) {
	target := &struct{ X int }{X: 7}
	w := NewWeakPtr(target)
	if !w.IsValid() {
		t.Fatalf("WeakPtr should be valid while target is reachable")
	}
	if got := w.TryGet(); got == nil || got.X != 7 {
		t.Fatalf("TryGet = %v, want X=7", got)
	}
	if w.IsNull() {
		t.Fatalf("IsNull should be false while valid")
	}
	// Get is documented as an alias for TryGet.
	if got := w.Get(); got == nil || got.X != 7 {
		t.Fatalf("Get = %v, want X=7", got)
	}
}

func TestWeakPtrZeroValue(t *testing.T) {
	var w WeakPtr[int]
	if w.IsValid() {
		t.Fatalf("zero-value WeakPtr should not be valid")
	}
	if !w.IsNull() {
		t.Fatalf("zero-value WeakPtr should be null")
	}
	if got := w.TryGet(); got != nil {
		t.Fatalf("TryGet on zero-value = %v, want nil", got)
	}
	// Revoke on a null WeakPtr must be a no-op (must not panic).
	w.Revoke()
}

func TestWeakPtrNullOnNilTarget(t *testing.T) {
	w := NewWeakPtr[int](nil)
	if w.IsValid() {
		t.Fatalf("NewWeakPtr(nil) should be invalid")
	}
	if got := w.TryGet(); got != nil {
		t.Fatalf("TryGet on null = %v, want nil", got)
	}
}

func TestWeakPtrRevoke(t *testing.T) {
	target := &struct{ V int }{V: 42}
	w := NewWeakPtr(target)
	if !w.IsValid() {
		t.Fatalf("should be valid before Revoke")
	}
	if got := w.TryGet(); got == nil || got.V != 42 {
		t.Fatalf("TryGet before Revoke = %v, want V=42", got)
	}
	w.Revoke()
	if w.IsValid() {
		t.Fatalf("should be invalid after Revoke")
	}
	if !w.IsNull() {
		t.Fatalf("IsNull should be true after Revoke")
	}
	if got := w.TryGet(); got != nil {
		t.Fatalf("TryGet after Revoke = %v, want nil", got)
	}
}

// TestWeakPtrRevokeInvalidatesSiblings verifies that all WeakPtrs created from the
// same target share one weakImpl, so a single Revoke invalidates every sibling at
// once. This mirrors WebKit's WeakPtrFactory::revokeAll.
func TestWeakPtrRevokeInvalidatesSiblings(t *testing.T) {
	target := &struct{ N int }{N: 9}
	w1 := NewWeakPtr(target)
	w2 := w1 // value copy shares the same impl
	w3 := w1
	if !w1.IsValid() || !w2.IsValid() || !w3.IsValid() {
		t.Fatalf("all sibling WeakPtrs should be valid before Revoke")
	}
	w1.Revoke()
	if w1.IsValid() || w2.IsValid() || w3.IsValid() {
		t.Fatalf("Revoke should invalidate every sibling sharing the impl")
	}
	if w2.TryGet() != nil || w3.TryGet() != nil {
		t.Fatalf("TryGet on sibling after Revoke should be nil")
	}
}

// TestWeakPtrResetOnlyAffectsSelf verifies that Reset detaches only this WeakPtr
// from the shared impl (mirroring assignment of nullptr to a single WeakPtr in
// WebKit), leaving sibling WeakPtrs intact.
func TestWeakPtrResetOnlyAffectsSelf(t *testing.T) {
	target := &struct{ S string }{S: "hi"}
	w1 := NewWeakPtr(target)
	w2 := w1
	w1.Reset()
	if w1.IsValid() {
		t.Fatalf("w1 should be invalid after Reset")
	}
	if !w2.IsValid() {
		t.Fatalf("w2 should still be valid after w1.Reset (Reset only affects self)")
	}
	if got := w2.TryGet(); got == nil || got.S != "hi" {
		t.Fatalf("w2 TryGet after w1.Reset = %v, want S=hi", got)
	}
}

func TestWeakPtrRevokeIdempotent(t *testing.T) {
	target := &struct{ K int }{K: 5}
	w := NewWeakPtr(target)
	w.Revoke()
	w.Revoke() // second Revoke must be a safe no-op
	w.Revoke()
	if w.IsValid() {
		t.Fatalf("should remain invalid after multiple Revoke calls")
	}
}

func TestWeakPtrMultipleShareEntry(t *testing.T) {
	target := &struct{ N int }{N: 9}
	w1 := NewWeakPtr(target)
	w2 := w1 // value copy shares the same entry pointer
	if !w1.IsValid() || !w2.IsValid() {
		t.Fatalf("both weak pointers should be valid")
	}
	if got := w2.TryGet(); got == nil || got.N != 9 {
		t.Fatalf("w2 TryGet = %v, want N=9", got)
	}
}
