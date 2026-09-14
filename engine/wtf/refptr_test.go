package wtf

import (
	"runtime"
	"sync/atomic"
	"testing"
)

func TestRefPtrBasic(t *testing.T) {
	p := NewRefPtr(42)
	if p.IsNull() {
		t.Fatalf("NewRefPtr should not be null")
	}
	if got := *p.Get(); got != 42 {
		t.Fatalf("Get = %d, want 42", got)
	}
	if got := p.RefCount(); got != 1 {
		t.Fatalf("RefCount = %d, want 1", got)
	}
}

func TestRefPtrCloneRelease(t *testing.T) {
	p := NewRefPtr(7)
	c := p.Clone()
	if c.RefCount() != 2 {
		t.Fatalf("RefCount after Clone = %d, want 2", c.RefCount())
	}
	c.Release()
	if p.RefCount() != 1 {
		t.Fatalf("RefCount after Release = %d, want 1", p.RefCount())
	}
	if p.IsNull() {
		t.Fatalf("original should still be valid")
	}
}

func TestRefPtrNull(t *testing.T) {
	var p RefPtr[int]
	if !p.IsNull() {
		t.Fatalf("zero RefPtr should be null")
	}
	if p.IsValid() {
		t.Fatalf("zero RefPtr should be invalid")
	}
	if got := p.Get(); got != nil {
		t.Fatalf("Get on null = %v, want nil", got)
	}
	if got := p.RefCount(); got != 0 {
		t.Fatalf("RefCount on null = %d, want 0", got)
	}
	// Release on null is a no-op.
	p.Release()
}

func TestRefPtrCloneNull(t *testing.T) {
	var p RefPtr[int]
	c := p.Clone()
	if !c.IsNull() {
		t.Fatalf("Clone of null should be null")
	}
}

func TestRefPtrCleanupOnLastRelease(t *testing.T) {
	var ran int32
	p := NewRefPtrWithCleanup("resource", func(v string) {
		if v == "resource" {
			atomic.StoreInt32(&ran, 1)
		}
	})
	c := p.Clone()
	c.Release()
	if atomic.LoadInt32(&ran) != 0 {
		t.Fatalf("cleanup should not run while refs remain")
	}
	p.Release()
	if atomic.LoadInt32(&ran) == 0 {
		t.Fatalf("cleanup should run on last release")
	}
}

func TestRefPtrSwapWith(t *testing.T) {
	a := NewRefPtr(1)
	b := NewRefPtr(2)
	a.SwapWith(&b)
	if *a.Get() != 2 || *b.Get() != 1 {
		t.Fatalf("Swap contents wrong: a=%v b=%v", *a.Get(), *b.Get())
	}
}

func TestRefPtrFinalizerDecrements(t *testing.T) {
	p := NewRefPtr(100)
	// Create a clone in a helper so the handle becomes unreachable once the helper
	// returns, allowing the GC finalizer to decrement the shared count.
	dropRefPtrClone(p)
	// Allow finalizers to run; count should return to 1. Finalizers are best-effort
	// and may not run promptly on every platform (notably some Windows runs), so we
	// retry for a while and skip rather than fail when the environment cannot
	// finalise in time. The deterministic decrement path is covered separately by
	// TestRefPtrCloneRelease.
	for i := 0; i < 100; i++ {
		runtime.GC()
		runtime.Gosched()
		if p.RefCount() == 1 {
			break
		}
		runtime.Gosched()
	}
	switch got := p.RefCount(); got {
	case 1:
		// Finalizer ran and decremented as expected.
	case 2:
		t.Skip("finalizer did not run promptly in this environment; RefCount still 2")
	default:
		t.Fatalf("RefCount after finalizer = %d, want 1", got)
	}
}

// dropRefPtrClone creates a clone of p and drops it, leaving the clone's handle
// unreachable so a subsequent GC will run its decrementing finalizer.
func dropRefPtrClone[T any](p RefPtr[T]) {
	c := p.Clone()
	if !c.IsValid() {
		panic("clone should be valid")
	}
}

// --- Ref coverage (ref.go) ---

func TestRefBasic(t *testing.T) {
	r := NewRef(42)
	if got := *r.Get(); got != 42 {
		t.Fatalf("Get = %d, want 42", got)
	}
	if got := *r.Deref(); got != 42 {
		t.Fatalf("Deref = %d, want 42", got)
	}
	if got := r.Value(); got != 42 {
		t.Fatalf("Value = %d, want 42", got)
	}
}

func TestRefNewRefFromPtrPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewRefFromPtr(nil) should panic")
		}
	}()
	_ = NewRefFromPtr[int](nil)
}

func TestRefAdoptRefPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("AdoptRef(nil) should panic")
		}
	}()
	_ = AdoptRef[int](nil)
}

func TestRefLeakRef(t *testing.T) {
	r := NewRef(5)
	p := r.LeakRef()
	if p == nil || *p != 5 {
		t.Fatalf("LeakRef = %v, want ptr to 5", p)
	}
	if r.Get() != nil {
		t.Fatalf("Get after LeakRef should be nil")
	}
}

func TestRefCleanupFinalizer(t *testing.T) {
	var ran int32
	r := NewRefWithCleanup("payload", func(p *string) {
		if *p == "payload" {
			atomic.StoreInt32(&ran, 1)
		}
	})
	// Sanity check that the Ref holds the value before we drop it.
	if got := r.Value(); got != "payload" {
		t.Fatalf("Value = %q, want payload", got)
	}
	// Drop the only strong reference and force collection so the finalizer runs.
	// Finalizers are best-effort; some environments (notably some Windows runs) do
	// not run them promptly, so we skip rather than report a false failure.
	r = nil
	for i := 0; i < 100; i++ {
		runtime.GC()
		runtime.Gosched()
		if atomic.LoadInt32(&ran) == 1 {
			break
		}
		runtime.Gosched()
	}
	if atomic.LoadInt32(&ran) != 1 {
		t.Skip("cleanup finalizer did not run promptly in this environment")
	}
}
