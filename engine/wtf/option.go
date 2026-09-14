// Translation of: Source/WTF/wtf/Optional.h
//                  Source/WTF/wtf/Markable.h
// Completeness: 85%
// Simplifications:
//   - modeled as a single generic Option[T] struct (no separate Markable<T, Traits>)
//   - uses a bool flag instead of a sentinel value, matching std::optional semantics
//   - no hashing traits; Option[T] is intentionally not comparable when T is not

package wtf

// Option is the Go translation of WTF::Optional<T> (and std::optional<T> in WebKit's
// recent code). It represents an optional value: either it holds a value of type T
// or it is empty. WebKit's Optional/Markable pair distinguishes the general
// optional<T> from the storage-optimised Markable<T, Traits> (which reuses one value
// of T as the empty marker). The Go port uses a single bool flag, mirroring
// std::optional semantics, which is simpler and works for every T without requiring a
// custom empty-value trait.
//
// Construct an empty value with None[T]() and a present value with Some(v).
type Option[T any] struct {
	value T
	set   bool
}

// Some returns an Option holding value, mirroring WTF::Optional(value).
func Some[T any](value T) Option[T] {
	return Option[T]{value: value, set: true}
}

// None returns an empty Option, mirroring WTF::nullopt / WTF::Optional().
func None[T any]() Option[T] {
	return Option[T]{set: false}
}

// IsValid reports whether the option holds a value, mirroring Optional::operator bool.
func (o Option[T]) IsValid() bool { return o.set }

// IsNone is the negation of IsValid, named for symmetry with None[T]().
func (o Option[T]) IsNone() bool { return !o.set }

// HasValue is an alias for IsValid matching std::optional::has_value.
func (o Option[T]) HasValue() bool { return o.set }

// Get returns the stored value. It panics if the option is empty, mirroring the
// RELEASE_ASSERT behaviour of Optional::value().
func (o Option[T]) Get() T {
	if !o.set {
		panic("wtf.Option: Get on empty option")
	}
	return o.value
}

// GetOr returns the stored value, or fallback when the option is empty, mirroring
// Optional::valueOr.
func (o Option[T]) GetOr(fallback T) T {
	if o.set {
		return o.value
	}
	return fallback
}

// Or returns the stored value or a value produced by factory when the option is
// empty. It mirrors Optional::valueOr combined with a lazy default, useful when the
// fallback is expensive to compute.
func (o Option[T]) Or(factory func() T) T {
	if o.set {
		return o.value
	}
	return factory()
}

// OrElse returns this option when it holds a value, or other when it is empty. It is
// the option-level fallback mirroring the "else" combinator used in WebKit call
// sites that chain optionals.
func (o Option[T]) OrElse(other Option[T]) Option[T] {
	if o.set {
		return o
	}
	return other
}

// Map applies fn to the stored value when present, returning a new Option of the
// result type. It mirrors the functional map combinator over optionals.
func Map[T any, U any](o Option[T], fn func(T) U) Option[U] {
	if !o.set {
		return None[U]()
	}
	return Some(fn(o.value))
}

// AndThen applies fn to the stored value when present, returning the Option[U]
// produced by fn. It mirrors the flat-map combinator over optionals.
func AndThen[T any, U any](o Option[T], fn func(T) Option[U]) Option[U] {
	if !o.set {
		return None[U]()
	}
	return fn(o.value)
}

// Reset clears the option back to an empty state, mirroring Optional::reset.
func (o *Option[T]) Reset() {
	var zero T
	o.value = zero
	o.set = false
}

// Set assigns value and marks the option as present, mirroring Optional::operator=.
func (o *Option[T]) Set(value T) {
	o.value = value
	o.set = true
}
