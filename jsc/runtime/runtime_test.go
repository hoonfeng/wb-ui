// Tests for jsc/runtime — basic type construction and method validation.
package runtime

import (
	"math"
	"testing"
)

// TestJSTypeValues verifies the JSType enum values.
func TestJSTypeValues(t *testing.T) {
	if StringType != 2 {
		t.Errorf("StringType = %d, want 2", StringType)
	}
	if ObjectType != 33 {
		t.Errorf("ObjectType = %d, want 33", ObjectType)
	}
	if ArrayType != 46 {
		t.Errorf("ArrayType = %d, want 46", ArrayType)
	}
}

// TestJSValueCreation verifies basic JSValue construction.
func TestJSValueCreation(t *testing.T) {
	if !JSValueUndefined.IsUndefined() {
		t.Error("JSValueUndefined should be undefined")
	}
	if !JSValueNull.IsNull() {
		t.Error("JSValueNull should be null")
	}
	b := NewJSValueBool(true)
	if !b.IsBoolean() || !b.ToBoolean() {
		t.Error("NewJSValueBool(true) should be boolean true")
	}
	n := NewJSValueNumber(42)
	if !n.IsNumber() || n.ToNumber() != 42 {
		t.Error("NewJSValueNumber(42) should be number 42")
	}
	s := NewJSValueString("hello")
	if !s.IsString() || s.ToString() != "hello" {
		t.Errorf("NewJSValueString('hello') = '%s', want 'hello'", s.ToString())
	}
}

// TestJSValueHelpers verifies helper JS functions.
func TestJSValueHelpers(t *testing.T) {
	if !jsBoolean(true).IsBoolean() {
		t.Error("jsBoolean should return boolean")
	}
	if !jsUndefined().IsUndefined() {
		t.Error("jsUndefined should be undefined")
	}
	if !jsNull().IsNull() {
		t.Error("jsNull should be null")
	}
	if jsNumber(3.14).ToNumber() != 3.14 {
		t.Error("jsNumber(3.14) should be 3.14")
	}
}

// TestSameValue verifies Object.is semantics.
func TestSameValue(t *testing.T) {
	global := &JSGlobalObject{}
	a := jsNumber(42)
	b := jsNumber(42)
	if !sameValue(global, a, b) {
		t.Error("sameValue(42, 42) should be true")
	}
	nan := NewJSValueNumber(math.NaN())
	if !sameValue(global, nan, nan) {
		t.Error("sameValue(NaN, NaN) should be true")
	}
	// In Go, -0 literal becomes float64(0), use math.Copysign to create -0
	negZero := NewJSValueNumber(math.Copysign(0, -1))
	if sameValue(global, jsNumber(0), negZero) {
		t.Errorf("sameValue(0, -0) should be false")
	}
}

// TestPropertyDescriptor verifies PropertyDescriptor behavior.
func TestPropertyDescriptor(t *testing.T) {
	pd := NewPropertyDescriptor()
	if !pd.IsEmpty() {
		t.Error("new PropertyDescriptor should be empty")
	}
	pd.SetWritable(true)
	pd.SetEnumerable(true)
	pd.SetConfigurable(true)
	pd.SetValue(jsNumber(42))
	if pd.IsEmpty() {
		t.Error("filled PropertyDescriptor should not be empty")
	}
	if !pd.Writable() || !pd.Enumerable() || !pd.Configurable() {
		t.Error("all attributes should be true after set")
	}
	if !pd.IsDataDescriptor() {
		t.Error("value+write => data descriptor")
	}
}

// TestErrorTypeNames verifies error type name mapping.
func TestErrorTypeNames(t *testing.T) {
	if n := errorTypeName(ErrorTypeTypeError); n != "TypeError" {
		t.Errorf("errorTypeName(TypeError) = '%s', want 'TypeError'", n)
	}
	if n := errorTypeName(ErrorTypeRangeError); n != "RangeError" {
		t.Errorf("errorTypeName(RangeError) = '%s', want 'RangeError'", n)
	}
	if n := errorTypeName(ErrorTypeSyntaxError); n != "SyntaxError" {
		t.Errorf("errorTypeName(SyntaxError) = '%s', want 'SyntaxError'", n)
	}
}

// TestECMAMode verifies strict/sloppy mode.
func TestECMAMode(t *testing.T) {
	if !ECMAModeStrict.IsStrict() {
		t.Error("ECMAModeStrict should be strict")
	}
	if ECMAModeSloppy.IsStrict() {
		t.Error("ECMAModeSloppy should not be strict")
	}
	if ECMAModeFromBool(true) != ECMAModeStrict {
		t.Error("ECMAModeFromBool(true) should be strict")
	}
	if ECMAModeFromBool(false) != ECMAModeSloppy {
		t.Error("ECMAModeFromBool(false) should be sloppy")
	}
}

// TestPropertyOffset verifies offset calculations.
func TestPropertyOffset(t *testing.T) {
	if !isValidOffset(PropertyOffset(0)) {
		t.Error("offset 0 should be valid")
	}
	if isValidOffset(InvalidOffset) {
		t.Error("InvalidOffset should not be valid")
	}
	if !IsInlineOffset(PropertyOffset(0)) {
		t.Error("offset 0 should be inline")
	}
	if IsOutOfLineOffset(PropertyOffset(0)) {
		t.Error("offset 0 should not be out-of-line")
	}
}

// TestCallData verifies CallData construction.
func TestCallData(t *testing.T) {
	cd := NewCallData()
	if cd.Type != CallTypeNone {
		t.Error("NewCallData should have CallTypeNone")
	}
	cd2 := CallData{Type: CallTypeNative}
	if cd2.Type != CallTypeNative {
		t.Error("CallData{Type: CallTypeNative} should have CallTypeNative")
	}
}

// TestNewJSFunction verifies JSFunction creation.
func TestNewJSFunction(t *testing.T) {
	vm := &VM{Exception: nil}
	global := &JSGlobalObject{vm: vm}
	typeInfo := NewTypeInfo(ObjectType, 0)
	structInfo := &ClassInfo{}
	structure := NewStructure(vm, global, NewJSValueObject(&global.JSObject), typeInfo, structInfo)
	_ = structure
	fn := NewJSFunction(vm, global, "testFunc", 2, nil)
	if fn == nil {
		t.Fatal("NewJSFunction should return non-nil")
	}
	if fn.functionName != "testFunc" {
		t.Errorf("function name = '%s', want 'testFunc'", fn.functionName)
	}
	if fn.functionLength != 2 {
		t.Errorf("function length = %d, want 2", fn.functionLength)
	}
}
