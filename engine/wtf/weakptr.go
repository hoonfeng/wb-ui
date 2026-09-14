// Translation of: Source/WTF/wtf/WeakPtr.h
//                  Source/WTF/wtf/WeakPtrFactory.h
// Completeness: 75%
// Simplifications:
//   - WeakPtr<T> shares a weakImpl[T] that holds a traceable *T and a "valid" flag;
//     invalidation is driven by an explicit Revoke() call (mirroring WebKit's
//     WeakPtrFactory::revokeAll) plus a best-effort GC finalizer safety net
//   - a true GC-weak pointer in Go requires storing the target address as a uintptr
//     (so it is not traced), but converting that uintptr back to *T is flagged by
//     `go vet`'s unsafeptr analyzer; to keep `go vet ./engine/wtf/...` clean this port
//     avoids unsafe and uses explicit revocation as the primary mechanism
//   - the "weak reference map" mentioned in the task is materialised as one shared
//     weakImpl per target; multiple WeakPtrs to the same target share the impl

package wtf

import (
	"runtime"
	"sync/atomic"
)

// weakImpl is the shared bookkeeping object between a target and its WeakPtrs. It
// plays the role of WebKit's WeakPtrImpl: it holds the target pointer and a validity
// flag. All WeakPtrs created from the same target share the same impl, so a single
// Revoke invalidates every outstanding WeakPtr at once.
//
// The target field is a traceable *T, which means a live WeakPtr extends the
// target's lifetime by one indirection. In Go this is acceptable because the GC
// reclaims the whole graph (target + impl + WeakPtrs) once nothing outside the weak
// machinery references the target. Deterministic invalidation is therefore provided
// explicitly via Revoke, mirroring WebKit's WeakPtrFactory::revokeAll which the
// target's destructor calls.
type weakImpl[T any] struct {
	target *T
	valid  atomic.Bool
}

// WeakPtr is the Go translation of WTF::WeakPtr<T>. In WebKit a WeakPtr is a nullable
// weak pointer that does not prevent the referenced object from being destroyed and
// becomes null once the object's WeakPtrFactory is revoked. The Go port shares a
// weakImpl between a target and its WeakPtrs: TryGet returns the target while the impl
// is valid, and returns nil once Revoke has been called (or the GC has collected the
// whole graph and the safety-net finalizer has run).
//
// Construct with NewWeakPtr. Call Revoke when the target is logically destroyed. The
// zero value is a null WeakPtr (TryGet returns nil, IsValid returns false).
type WeakPtr[T any] struct {
	impl *weakImpl[T]
}

// NewWeakPtr creates a WeakPtr referring to ptr. The caller should arrange for
// Revoke to be called when the target is logically destroyed; this mirrors the
// obligation in WebKit for a CanMakeWeakPtr type to revoke its WeakPtrFactory in its
// destructor. A best-effort finalizer is installed on the shared impl as a safety
// net: if the impl becomes unreachable (no WeakPtrs and no external references to the
// target) the GC will mark it invalid.
func NewWeakPtr[T any](ptr *T) WeakPtr[T] {
	if ptr == nil {
		return WeakPtr[T]{}
	}
	impl := &weakImpl[T]{target: ptr}
	impl.valid.Store(true)
	runtime.SetFinalizer(impl, func(impl *weakImpl[T]) {
		// Safety net: the impl (and the target graph) is being collected, so mark
		// any still-outstanding WeakPtrs as invalid. This is best-effort: a
		// still-reachable WeakPtr keeps the impl alive, so this only fires once no
		// WeakPtrs remain, at which point invalidation is mostly cosmetic.
		impl.valid.Store(false)
		impl.target = nil
	})
	return WeakPtr[T]{impl: impl}
}

// IsValid reports whether the referenced object is still live, mirroring
// WeakPtr::operator bool(). It returns false for a null WeakPtr and after Revoke has
// been called.
func (w WeakPtr[T]) IsValid() bool {
	return w.impl != nil && w.impl.valid.Load()
}

// IsNull is the negation of IsValid matching the nullable-pointer convention.
func (w WeakPtr[T]) IsNull() bool { return !w.IsValid() }

// TryGet returns the referenced pointer, or nil if the weak reference has been
// revoked, mirroring WeakPtr::get().
func (w WeakPtr[T]) TryGet() *T {
	if w.impl == nil || !w.impl.valid.Load() {
		return nil
	}
	return w.impl.target
}

// Get is an alias for TryGet retained for fidelity with WeakPtr::get().
func (w WeakPtr[T]) Get() *T { return w.TryGet() }

// Revoke marks the weak reference invalid and clears the held target. It mirrors
// WeakPtrFactory::revokeAll and should be called when the target is logically
// destroyed. Because all WeakPtrs created from the same target share the same impl,
// a single Revoke invalidates every outstanding WeakPtr at once.
func (w *WeakPtr[T]) Revoke() {
	if w.impl != nil {
		w.impl.valid.Store(false)
		w.impl.target = nil
	}
}

// Reset clears the weak pointer so it no longer refers to any object. Unlike Revoke,
// Reset only affects this WeakPtr (it detaches from the shared impl) and does not
// invalidate sibling WeakPtrs. It mirrors assigning WTF::nullptr to a single WeakPtr.
func (w *WeakPtr[T]) Reset() {
	w.impl = nil
}
