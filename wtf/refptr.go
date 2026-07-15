// Translation of: Source/WTF/wtf/RefPtr.h
//                  Source/WTF/wtf/RefCounted.h
// Completeness: 80%
// Simplifications:
//   - RefPtr<T> is a nullable smart pointer with atomic ref counting (atomic.Int64)
//   - intrusive RefCounted<T> base is replaced by an internal refTarget[T] wrapper
//   - count decrement happens via runtime.SetFinalizer on a per-pointer handle, since
//     Go has no destructors; explicit Release() is also available
//   - no thread-safety assertions / ASAN support

package wtf

import (
	"runtime"
	"sync/atomic"
)

// refTarget holds the owned value together with the shared reference count and an
// optional cleanup callback. It stands in for WebKit's RefCounted<T> base class: the
// count starts at 0 (no outstanding handles) and is incremented once per live
// RefPtr/handle. When the count drops back to 0 the cleanup callback (if any) runs,
// mirroring the destructor invoked from RefCounted::deref when m_count reaches 0.
type refTarget[T any] struct {
	value   T
	count   atomic.Int64
	cleanup func(T)
}

// refHandle is a per-RefPtr bookkeeping object. Each RefPtr copy allocates a fresh
// handle that increments the shared count; when the handle becomes unreachable the
// GC-attached finalizer decrements the count again. This bridges Go's tracing GC with
// WebKit's intrusive ref-counting model without requiring translated code to call
// Retain/Release manually on every copy.
type refHandle[T any] struct {
	target *refTarget[T]
}

// RefPtr is the Go translation of WTF::RefPtr<T>. In WebKit RefPtr<T> is a nullable
// intrusive reference-counting smart pointer: it extends the lifetime of the
// referenced object by calling ref() on construction/copy and deref() on destruction,
// deleting the object when the count reaches zero. The Go port keeps the nullable
// semantics and atomic counting but delegates raw memory management to the GC.
//
// Construct with NewRefPtr (owns a fresh value, count starts at one) or
// NewRefPtrWithCleanup to register a deterministic cleanup callback. Share ownership
// with Clone (increments the count) and release explicitly with Release (decrements
// and disables the finalizer for that handle). Get/IsNull provide nullable access.
//
// The zero value is a null RefPtr.
type RefPtr[T any] struct {
	h *refHandle[T]
}

// NewRefPtr creates a RefPtr owning a fresh heap copy of value with an initial
// reference count of one. It mirrors RefPtr(ptr) where ptr was produced by a +1
// factory (e.g. RefCounted::create).
func NewRefPtr[T any](value T) RefPtr[T] {
	t := &refTarget[T]{value: value}
	return RefPtr[T]{h: newHandle(t)}
}

// NewRefPtrWithCleanup is like NewRefPtr but registers a cleanup callback that runs
// (once) when the last reference is dropped. It mirrors types that derive from
// RefCounted<T> with a non-trivial destructor.
func NewRefPtrWithCleanup[T any](value T, cleanup func(T)) RefPtr[T] {
	t := &refTarget[T]{value: value, cleanup: cleanup}
	return RefPtr[T]{h: newHandle(t)}
}

// newHandle allocates a handle that owns one reference to target, increments the
// shared count, and attaches a finalizer so the count is decremented when the handle
// is collected.
func newHandle[T any](target *refTarget[T]) *refHandle[T] {
	target.count.Add(1)
	h := &refHandle[T]{target: target}
	runtime.SetFinalizer(h, func(h *refHandle[T]) {
		releaseTarget(h.target)
	})
	return h
}

// releaseTarget decrements the shared count for target and runs the cleanup callback
// if the count reaches zero. It is the analogue of RefCounted::deref().
func releaseTarget[T any](target *refTarget[T]) {
	if target == nil {
		return
	}
	if target.count.Add(-1) == 0 && target.cleanup != nil {
		target.cleanup(target.value)
	}
}

// Get returns a pointer to the wrapped value, or nil for a null RefPtr. It mirrors
// RefPtr::get().
func (p RefPtr[T]) Get() *T {
	if p.h == nil || p.h.target == nil {
		return nil
	}
	return &p.h.target.value
}

// IsNull reports whether the RefPtr is null, mirroring RefPtr::operator bool().
func (p RefPtr[T]) IsNull() bool { return p.h == nil }

// IsValid is the negation of IsNull, named for readability at call sites.
func (p RefPtr[T]) IsValid() bool { return p.h != nil }

// Clone returns a new RefPtr that shares ownership of the same value, incrementing
// the reference count. It mirrors the RefPtr copy constructor. Cloning a null RefPtr
// returns another null RefPtr.
func (p RefPtr[T]) Clone() RefPtr[T] {
	if p.h == nil {
		return RefPtr[T]{}
	}
	return RefPtr[T]{h: newHandle(p.h.target)}
}

// Release decrements the reference count for this handle, disables its finalizer,
// and resets the RefPtr to null. It is the explicit counterpart of Clone, useful when
// translated code calls RefPtr::clear() or relies on deterministic deref.
func (p *RefPtr[T]) Release() {
	if p.h == nil {
		return
	}
	runtime.SetFinalizer(p.h, nil)
	releaseTarget(p.h.target)
	p.h = nil
}

// Reset is an alias for Release mirroring RefPtr::clear().
func (p *RefPtr[T]) Reset() { p.Release() }

// RefCount returns the current number of outstanding references. It is intended for
// diagnostics and tests; production code should not branch on it.
func (p RefPtr[T]) RefCount() int64 {
	if p.h == nil || p.h.target == nil {
		return 0
	}
	return p.h.target.count.Load()
}

// SwapWith swaps ownership between two RefPtr values, mirroring RefPtr::swap.
func (p *RefPtr[T]) SwapWith(other *RefPtr[T]) {
	p.h, other.h = other.h, p.h
}
