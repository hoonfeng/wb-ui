// Translation of: Source/WTF/wtf/Ref.h
//                  Source/WTF/wtf/RefCounted.h
// Completeness: 75%
// Simplifications:
//   - Ref<T> is a non-nullable owning pointer wrapping *T (Go GC manages lifetime)
//   - intrusive ref()/deref() of RefCounted<T> is replaced by an optional cleanup
//     callback run via runtime.SetFinalizer when the Ref becomes unreachable
//   - no ASAN poison/unpoison support

package wtf

import "runtime"

// Ref is the Go translation of WTF::Ref<T>. In WebKit Ref<T> is the non-nullable
// variant of RefPtr<T>: it holds a non-null pointer to a reference-counted object and
// increments the ref count on construction (deref on destruction). Because Go has
// tracing garbage collection, raw memory lifetime is handled by the runtime and the
// intrusive ref count is not needed for memory safety. The port keeps the non-null
// ownership contract and exposes an optional finalizer-based cleanup hook so that
// translated code that relied on deterministic destruction (e.g. closing a handle)
// can still run a callback when the Ref is collected.
//
// A Ref is always non-null: NewRef panics if handed a nil pointer, and the zero Ref
// value is intentionally unusable until NewRef/AdoptRef populates it.
type Ref[T any] struct {
	ptr     *T
	cleanup func(*T)
}

// NewRef creates a Ref that owns value (by copying it onto the heap) and returns a
// pointer to it. It mirrors the Ref(T&) constructor. The returned *Ref[T] is used so
// that an optional finalizer can be attached to the Ref itself.
func NewRef[T any](value T) *Ref[T] {
	return &Ref[T]{ptr: &value}
}

// NewRefFromPtr creates a Ref that wraps an existing *T without copying it. The
// caller transfers ownership: the Ref will treat ptr as its own. It panics if ptr is
// nil, matching the non-null contract of WTF::Ref.
func NewRefFromPtr[T any](ptr *T) *Ref[T] {
	if ptr == nil {
		panic("wtf.NewRefFromPtr: nil pointer")
	}
	return &Ref[T]{ptr: ptr}
}

// NewRefWithCleanup is like NewRef but attaches a cleanup callback that runs (via a
// finalizer) when the Ref becomes unreachable. It mirrors types that combine
// RefCounted<T> with a custom deref action.
func NewRefWithCleanup[T any](value T, cleanup func(*T)) *Ref[T] {
	r := &Ref[T]{ptr: &value, cleanup: cleanup}
	if cleanup != nil {
		runtime.SetFinalizer(r, func(r *Ref[T]) {
			if r.ptr != nil && r.cleanup != nil {
				r.cleanup(r.ptr)
			}
		})
	}
	return r
}

// Get returns the wrapped pointer, mirroring Ref::get() / Ref::ptr().
func (r *Ref[T]) Get() *T { return r.ptr }

// Ptr is an alias for Get retained for fidelity with WebKit naming.
func (r *Ref[T]) Ptr() *T { return r.ptr }

// Deref returns the wrapped pointer, mirroring Ref::operator&() in WebKit. It is the
// same as Get but named to match the upstream API surface used at dereference sites.
func (r *Ref[T]) Deref() *T { return r.ptr }

// Value returns a copy of the pointed-to value, a convenience for callers that want
// to read the payload without keeping the pointer.
func (r *Ref[T]) Value() T {
	if r.ptr == nil {
		panic("wtf.Ref: Value on nil")
	}
	return *r.ptr
}

// LeakRef surrenders ownership of the wrapped pointer without running cleanup,
// mirroring Ref::leakRef(). The Ref is left empty (Get returns nil) and the finalizer
// is disabled so that the returned pointer can be adopted by another owner.
func (r *Ref[T]) LeakRef() *T {
	p := r.ptr
	r.ptr = nil
	r.cleanup = nil
	runtime.SetFinalizer(r, nil)
	return p
}

// AdoptRef takes ownership of an existing *T without incrementing any reference
// count, mirroring WTF::adoptRef. It is the ownership-transfer counterpart of
// NewRefFromPtr and disables any prior finalizer on the constructed Ref.
func AdoptRef[T any](ptr *T) *Ref[T] {
	if ptr == nil {
		panic("wtf.AdoptRef: nil pointer")
	}
	return &Ref[T]{ptr: ptr}
}
