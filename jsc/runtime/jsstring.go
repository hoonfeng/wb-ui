// Translation of: Source/JavaScriptCore/runtime/JSString.h
//                  Source/JavaScriptCore/runtime/JSString.cpp
//
// JSString is the JSC string type. In the Go port, it wraps a Go string.

package runtime

import (
	"math"
	"strconv"
)

// JSString corresponds to JSC::JSString. It holds a JavaScript string value.
type JSString struct {
	JSCell
	value string
}

// Structure flags for JSString subclasses.
const JSStringStructureFlags uint32 = JSCellStructureFlags

// NewJSString creates a JSString.
func NewJSString(vm *VM, value string) *JSString {
	_ = vm
	s := &JSString{value: value}
	s.typ = StringType
	s.cellState = DefinitelyWhite
	return s
}

// NewJSStringFromCell creates a JSString from a JSCell (used by parser/interpreter).
func NewJSStringFromCell(cell *JSCell) *JSString {
	return AsString(cell)
}

// Length returns the length of the string.
func (s *JSString) Length() int { return len(s.value) }

// Value returns the underlying Go string.
func (s *JSString) Value(globalObject *JSGlobalObject) string {
	_ = globalObject
	return s.value
}

// ToString returns the string value (identity for JSString).
func (s *JSString) ToString(globalObject *JSGlobalObject) string {
	_ = globalObject
	return s.value
}

// ToNumber converts the string to a number per ECMA-262.
func (s *JSString) ToNumber(globalObject *JSGlobalObject) float64 {
	_ = globalObject
	f, err := strconv.ParseFloat(s.value, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// ToObject wraps this string in a StringObject.
func (s *JSString) ToObject(globalObject *JSGlobalObject) *JSObject {
	_ = globalObject
	// TODO: create actual StringObject
	return nil
}

// ToBoolean returns true if the string is non-empty.
func (s *JSString) ToBoolean(globalObject *JSGlobalObject) bool {
	_ = globalObject
	return len(s.value) > 0
}

// ToPrimitive returns the string itself (already primitive).
func (s *JSString) ToPrimitive(globalObject *JSGlobalObject, preferred PreferredPrimitiveType) JSValue {
	_ = globalObject
	_ = preferred
	return JSValue{tag: TagString, payload: s.value}
}

// Equal checks string equality.
func (s *JSString) Equal(other *JSString) bool {
	return s.value == other.value
}

// CharAt returns the character at the given index (as a single-char string).
func (s *JSString) CharAt(index int) string {
	if index < 0 || index >= len(s.value) {
		return ""
	}
	return string(s.value[index])
}

// Substring returns a substring [start, end).
func (s *JSString) Substring(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s.value) {
		end = len(s.value)
	}
	if start >= end {
		return ""
	}
	return s.value[start:end]
}
