// Translation of: Source/WebCore/bindings/js/JSDOMConvertAny.h
//                  Source/WebCore/bindings/js/JSDOMBinding.h
//                  Source/WebCore/bindings/js/JSDOMConvertNumbers.h
//                  Source/WebCore/bindings/js/JSDOMConvertStrings.h
// Completeness: 60%
// Simplifications:
//   - objects become map[string]any; arrays become []any
//   - no host-object identity tracking (wrappers are not recovered as Go pointers)
//   - functions convert to nil (they are not callable from Go in this port)

package bindings

import (
	"errors"

	"wb-ui/jsc"
)

// ErrUnsupportedJSValue is returned by FromJSValue for a JSValue whose tag has no Go
// counterpart. It mirrors the JS TypeError thrown by WebKit conversion failures.
var ErrUnsupportedJSValue = errors.New("bindings: unsupported JSValue")

// FromJSValue converts a jsc.JSValue to a Go value, mirroring toValue(exec, value) in
// JSDOMBinding. The mapping is:
//   - undefined / null -> nil
//   - boolean -> bool
//   - number -> float64
//   - string -> string
//   - array -> []any (recursively converted)
//   - object -> map[string]any (recursively converted)
//   - function -> nil
//
// Numbers are always returned as float64 (JS numbers are IEEE-754 doubles); callers
// that need an int should range-assert the returned float64.
func FromJSValue(v jsc.JSValue) (any, error) {
	switch {
	case v.IsUndefined():
		return nil, nil
	case v.IsNull():
		return nil, nil
	case v.IsBoolean():
		return v.AsBoolean(), nil
	case v.IsNumber():
		return v.AsNumber(), nil
	case v.IsString():
		return v.AsString(), nil
	case v.IsObject():
		return fromJSObject(v.AsObject())
	case v.IsFunction():
		// Functions are opaque from Go's perspective; they convert to nil rather than
		// erroring so that round-tripping an arbitrary JS object through FromJSValue
		// never fails just because it contains a function-valued property.
		return nil, nil
	}
	return nil, ErrUnsupportedJSValue
}

// fromJSObject converts a JSObject. Arrays become []any, plain objects become
// map[string]any. Element/property values are converted recursively.
func fromJSObject(o *jsc.JSObject) (any, error) {
	if o == nil {
		return nil, nil
	}
	// Use Export() to get the Go native representation.
	// goja.Object.Export() returns:
	//   - []interface{} for arrays
	//   - map[string]interface{} for plain objects
	//   - nil for null/undefined
	exported := jsc.ObjectValue(o).Export()
	if exported == nil {
		return nil, nil
	}
	return exported, nil
}
