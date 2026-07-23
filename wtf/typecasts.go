// Translation of: Source/WTF/wtf/TypeCasts.h
// Completeness: 90%
// Simplifications:
//   - Go's builtin type assertion (x.(T)) already covers the is<T> / downcast<T>
//     pattern; these wrappers exist so that translated code reads 1:1 with upstream
//   - No SPECIALIZE_TYPE_TRAITS_BEGIN/END macros needed; Go interfaces fill the role
//   - No static_assert for base types; Go generics with any work universally
//   - downcast panics (like WebKit's RELEASE_ASSERT) when the type doesn't match;
//     use Is() to check before downcasting

package wtf

// Is reports whether value can be asserted to type T. It mirrors WTF::is<T>(source).
// In WebKit this dispatches through TypeCastTraits; in Go it uses the builtin type
// assertion.
//
// Usage:
//
//	if wtf.Is[dom.Element](node) { ... }
func Is[T any](value any) bool {
	_, ok := value.(T)
	return ok
}

// Downcast asserts value to type T and panics if the assertion fails. It mirrors
// WTF::downcast<T>(source) which uses RELEASE_ASSERT.
//
// Usage:
//
//	elt := wtf.Downcast[*dom.Element](node)
func Downcast[T any](value any) T {
	result, ok := value.(T)
	if !ok {
		panic("wtf.Downcast: type assertion failed")
	}
	return result
}
