// Translation of: Source/WTF/wtf/Vector.h
//                  Source/WTF/wtf/Vector.cpp
// Completeness: 85%
// Simplifications:
//   - backed by a Go []T slice instead of a custom contiguous allocator (FastMalloc)
//   - no InlineCapacity / CompactPointerTuple machinery; capacity grows via append
//   - no custom copy/move trait dispatch; Go assignment semantics are used directly

package wtf

import (
	"sort"
)

// NotFound mirrors WTF::notFound (std::numeric_limits<unsigned>::max()). It is the
// sentinel returned by Find when no matching element exists.
const NotFound = -1

// Vector is the Go translation of WTF::Vector<T>. WebKit's Vector is a growable
// contiguous array owning its elements. The Go port backs it with a built-in slice
// []T, which provides the same grow/shrink semantics without the bespoke allocator.
// Like the upstream class it tracks Size() (logical length) and Capacity() (allocated
// backing), and exposes the Append / Remove / Shrink / Contains / Find / Reverse /
// Sort operations that downstream DOM/CSS code relies on.
//
// The zero value is an empty, ready-to-use vector.
type Vector[T any] struct {
	buf []T
}

// NewVector returns a Vector with the given initial elements. It mirrors
// Vector(std::initializer_list<T>) in WebKit.
func NewVector[T any](items ...T) Vector[T] {
	v := Vector[T]{}
	if len(items) > 0 {
		v.buf = append(v.buf, items...)
	}
	return v
}

// NewVectorWithCapacity returns an empty Vector whose backing slice has been
// preallocated to hold at least capacity elements. It mirrors
// Vector::reserveInitialCapacity.
func NewVectorWithCapacity[T any](capacity int) Vector[T] {
	return Vector[T]{buf: make([]T, 0, capacity)}
}

// Size returns the number of logical elements, mirroring Vector::size().
func (v *Vector[T]) Size() int { return len(v.buf) }

// Len is an alias for Size matching Go container convention.
func (v *Vector[T]) Len() int { return len(v.buf) }

// IsEmpty reports whether the vector has no elements, mirroring Vector::isEmpty().
func (v *Vector[T]) IsEmpty() bool { return len(v.buf) == 0 }

// Capacity returns the allocated capacity of the backing store, mirroring
// Vector::capacity().
func (v *Vector[T]) Capacity() int { return cap(v.buf) }

// ReserveCapacity ensures the backing store can hold at least newCapacity elements
// without reallocating, mirroring Vector::reserveCapacity. It never shrinks.
func (v *Vector[T]) ReserveCapacity(newCapacity int) {
	if cap(v.buf) >= newCapacity {
		return
	}
	newBuf := make([]T, len(v.buf), newCapacity)
	copy(newBuf, v.buf)
	v.buf = newBuf
}

// Append appends value to the end, mirroring Vector::append(). It grows the backing
// slice as needed.
func (v *Vector[T]) Append(value T) {
	v.buf = append(v.buf, value)
}

// AppendValue is an alias for Append retained for fidelity with WebKit naming.
func (v *Vector[T]) AppendValue(value T) { v.Append(value) }

// AppendVector appends all elements of other to this vector, mirroring
// Vector::appendVector.
func (v *Vector[T]) AppendVector(other Vector[T]) {
	v.buf = append(v.buf, other.buf...)
}

// AppendSlice appends all elements of a plain Go slice, a convenience that WebKit
// provides via std::span overloads.
func (v *Vector[T]) AppendSlice(items []T) {
	v.buf = append(v.buf, items...)
}

// At returns the element at index. It mirrors Vector::at(index) and panics on an
// out-of-range index, matching the C++ release assertion behaviour.
func (v *Vector[T]) At(index int) T {
	return v.buf[index]
}

// Set replaces the element at index, mirroring Vector::at(index) = value.
func (v *Vector[T]) Set(index int, value T) {
	v.buf[index] = value
}

// First returns the first element, mirroring Vector::first().
func (v *Vector[T]) First() T {
	return v.buf[0]
}

// Last returns the last element, mirroring Vector::last().
func (v *Vector[T]) Last() T {
	return v.buf[len(v.buf)-1]
}

// TakeLast removes and returns the last element, mirroring Vector::takeLast().
// It panics on an empty vector.
func (v *Vector[T]) TakeLast() T {
	n := len(v.buf)
	last := v.buf[n-1]
	v.buf = v.buf[:n-1]
	return last
}

// Remove removes the element at index, shifting subsequent elements left, mirroring
// Vector::remove(index). It panics if index is out of range.
func (v *Vector[T]) Remove(index int) {
	v.buf = append(v.buf[:index], v.buf[index+1:]...)
}

// RemoveFirst removes the first element equal to value (using the supplied equals
// predicate), mirroring Vector::removeFirstMatching. It returns true if an element
// was removed.
func (v *Vector[T]) RemoveFirst(value T, equals func(a, b T) bool) bool {
	idx := v.Find(value, equals)
	if idx == NotFound {
		return false
	}
	v.Remove(idx)
	return true
}

// Shrink truncates the vector to newSize elements, mirroring Vector::shrink(newSize).
// If newSize is greater than the current size it is a no-op.
func (v *Vector[T]) Shrink(newSize int) {
	if newSize < 0 {
		newSize = 0
	}
	if newSize < len(v.buf) {
		// Zero out the tail so dropped elements can be garbage collected, matching
		// the destructor behaviour of Vector::shrinkCapacity for non-trivial types.
		var zero T
		for i := newSize; i < len(v.buf); i++ {
			v.buf[i] = zero
		}
		v.buf = v.buf[:newSize]
	}
}

// Resize sets the logical size to newSize. New elements are zero-valued, mirroring
// Vector::resize(newSize).
func (v *Vector[T]) Resize(newSize int) {
	if newSize <= len(v.buf) {
		v.Shrink(newSize)
		return
	}
	for len(v.buf) < newSize {
		var zero T
		v.buf = append(v.buf, zero)
	}
}

// Clear removes all elements, mirroring Vector::clear(). The backing capacity is
// retained, matching the non-shrinking clear() variant.
func (v *Vector[T]) Clear() {
	var zero T
	for i := range v.buf {
		v.buf[i] = zero
	}
	v.buf = v.buf[:0]
}

// SwapWith swaps the contents of this vector with other, mirroring Vector::swap().
func (v *Vector[T]) SwapWith(other *Vector[T]) {
	v.buf, other.buf = other.buf, v.buf
}

// Find returns the index of the first element equal to value (using the supplied
// equals predicate) or NotFound if not present. WebKit's Vector::find uses the
// operator== trait; in Go the caller supplies the comparator explicitly. For builtin
// comparable types use wtf.Equals[T].
func (v *Vector[T]) Find(value T, equals func(a, b T) bool) int {
	if equals == nil {
		return NotFound
	}
	for i, e := range v.buf {
		if equals(e, value) {
			return i
		}
	}
	return NotFound
}

// Contains reports whether an element equal to value exists (using the supplied
// equals predicate), mirroring Vector::contains.
func (v *Vector[T]) Contains(value T, equals func(a, b T) bool) bool {
	return v.Find(value, equals) != NotFound
}

// Reverse reverses the element order in place, mirroring Vector::reverse().
func (v *Vector[T]) Reverse() {
	for i, j := 0, len(v.buf)-1; i < j; i, j = i+1, j-1 {
		v.buf[i], v.buf[j] = v.buf[j], v.buf[i]
	}
}

// Sort sorts the vector in place using the supplied less function, mirroring
// Vector::sort (which dispatches to std::sort).
func (v *Vector[T]) Sort(less func(a, b T) bool) {
	sort.Slice(v.buf, func(i, j int) bool { return less(v.buf[i], v.buf[j]) })
}

// Slice returns a copy of the elements in [start, start+length), mirroring
// Vector::subspan. The returned slice aliases the backing storage; callers should not
// retain it across mutations of the vector.
func (v *Vector[T]) Slice(start, length int) []T {
	if start < 0 {
		start = 0
	}
	end := start + length
	if end > len(v.buf) {
		end = len(v.buf)
	}
	return v.buf[start:end]
}

// AsSlice returns the whole backing slice (read-only by convention), mirroring
// Vector::span(). The slice aliases the backing storage.
func (v *Vector[T]) AsSlice() []T { return v.buf }

// Equals is a convenience comparator usable with Find/Contains/RemoveFirst for any
// comparable type T, matching the default operator== behaviour of WebKit's
// VectorTraits<T>::canCompareWithMemcmp.
func Equals[T comparable](a, b T) bool { return a == b }

// Clone returns a shallow copy of the vector, mirroring the implicit copy
// constructor of WebKit's Vector.
func (v *Vector[T]) Clone() Vector[T] {
	out := Vector[T]{buf: make([]T, len(v.buf))}
	copy(out.buf, v.buf)
	return out
}
