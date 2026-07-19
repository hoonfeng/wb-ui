// Helpers corresponds to JSC assertion macros and utility functions
// (originally from various .h files)
package runtime

import "math"

// ASSERT corresponds to JSC's ASSERT macro.
func ASSERT(cond bool) {
	if !cond {
		panic("ASSERTION FAILED")
	}
}

// RELEASE_ASSERT corresponds to JSC's RELEASE_ASSERT macro.
func RELEASE_ASSERT(cond bool) {
	if !cond {
		panic("RELEASE_ASSERT FAILED")
	}
}

// RELEASE_ASSERT_NOT_REACHED corresponds to JSC's RELEASE_ASSERT_NOT_REACHED macro.
func RELEASE_ASSERT_NOT_REACHED() {
	panic("RELEASE_ASSERT_NOT_REACHED")
}

// EXCEPTION_ASSERT corresponds to JSC's EXCEPTION_ASSERT macro.
func EXCEPTION_ASSERT(cond bool) {
	if !cond {
		// non-fatal in debug
	}
}

// UNUSED_PARAM corresponds to JSC's UNUSED_PARAM macro.
func UNUSED_PARAM(x interface{}) {
	_ = x
}

// --- JSValue helper functions ---

// jsBoolean creates a boolean JSValue.
func jsBoolean(b bool) JSValue {
	return NewJSValueBool(b)
}

// jsUndefined returns the undefined value.
func jsUndefined() JSValue {
	return JSValueUndefined
}

// jsNull returns the null value.
func jsNull() JSValue {
	return JSValueNull
}

// jsNumber creates a number JSValue.
func jsNumber(n float64) JSValue {
	return NewJSValueNumber(n)
}

// JSValueEncode encodes a JSValue into an EncodedJSValue (simplified).
func JSValueEncode(v JSValue) JSValue { return v }

// EncodedJSValue returns JSValueEncode(jsUndefined()).
func EncodedJSValue() JSValue { return JSValueUndefined }

// JSValueDecode decodes an EncodedJSValue (simplified).
func JSValueDecode(v JSValue) JSValue { return v }

// --- Exception helpers ---

// throwVMTypeError throws a TypeError in the VM.
func throwVMTypeError(globalObject *JSGlobalObject, scope ThrowScope, msg ...string) JSValue {
	vm := globalObject.VM()
	errMsg := "TypeError"
	if len(msg) > 0 {
		errMsg = msg[0]
	}
	vm.ThrowException(globalObject, errMsg)
	return JSValueUndefined
}

// throwTypeError throws a TypeError without VM.
func throwTypeError(globalObject *JSGlobalObject, scope ThrowScope, msg string) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, msg)
}

// throwOutOfMemoryError throws an OOM error.
func throwOutOfMemoryError(globalObject *JSGlobalObject, scope ThrowScope) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, "Out of memory")
}

// getVM returns the VM from a JSGlobalObject.
func getVM(globalObject *JSGlobalObject) *VM {
	return globalObject.VM()
}

// --- as* helper functions ---

// asString returns the JSString from a JSValue (simplified).
func asString(v JSValue) *JSString {
	if v.IsString() {
		return nil // TODO: convert to JSString*
	}
	return nil
}

// asObject returns the JSObject from a JSValue.
func asObject(v JSValue) *JSObject {
	if v.IsObject() {
		return v.GetObject()
	}
	return nil
}

// uncheckedDowncast performs a direct type assertion (no check, like C++ uncheckedDowncast).
func uncheckedDowncast[T any](obj interface{}) *T {
	result, ok := obj.(*T)
	if !ok {
		panic("uncheckedDowncast failed")
	}
	return result
}

// dynamicDowncast performs a safe type assertion with nil on failure.
func dynamicDowncast[T any](obj interface{}) *T {
	result, ok := obj.(*T)
	if !ok {
		return nil
	}
	return result
}

// --- call/construct helper functions ---

// getCallDataInline returns CallData for a JSValue.
func getCallDataInline(value JSValue) CallData {
	if !value.IsCell() {
		return CallData{Type: CallTypeNone}
	}
	if obj := value.GetObject(); obj != nil {
		return GetCallData(&obj.JSCell)
	}
	return CallData{Type: CallTypeNone}
}

// getConstructDataInline returns ConstructData for a JSValue.
func getConstructDataInline(value JSValue) ConstructData {
	if !value.IsCell() {
		return ConstructData{Type: ConstructTypeNone}
	}
	if obj := value.GetObject(); obj != nil {
		return GetConstructData(&obj.JSCell)
	}
	return ConstructData{Type: ConstructTypeNone}
}

// call invokes [[Call]] on a value.
func call(globalObject *JSGlobalObject, value JSValue, callData CallData, thisValue JSValue, args []JSValue) JSValue {
	_ = callData
	if fn := value.AsFunction(); fn != nil {
		result, err := fn.Call(globalObject, thisValue, args)
		if err != nil {
			globalObject.VM().ThrowException(globalObject, err.Error())
			return JSValueUndefined
		}
		return result
	}
	return JSValueUndefined
}

// RELEASE_AND_RETURN corresponds to JSC's RELEASE_AND_RETURN macro.
func RELEASE_AND_RETURN(scope ThrowScope, value JSValue) JSValue {
	if scope.vm != nil && scope.vm.Exception != nil {
		return JSValueUndefined
	}
	return value
}

// jsOwnedString creates an owned JS string value.
func jsOwnedString(vm *VM, s string) JSValue {
	_ = vm
	return NewJSValueString(s)
}

// jsOwnedStringFromImpl creates a JSValue from a string implementation.
func jsOwnedStringFromImpl(vm *VM, impl string) JSValue {
	return jsOwnedString(vm, impl)
}

// --- SameValue (Object.is) ---

// sameValue corresponds to JSC's sameValue (used by Object.is).
func sameValue(globalObject *JSGlobalObject, a, b JSValue) bool {
	_ = globalObject
	if a.IsNumber() && b.IsNumber() {
		if a.IsNaN() && b.IsNaN() {
			return true
		}
		// Handle -0 vs +0 (Object.is distinguishes them)
		af := a.ToNumber()
		bf := b.ToNumber()
		if af == 0 && bf == 0 {
			// Check sign bit
			return math.Float64bits(af) == math.Float64bits(bf)
		}
		return af == bf
	}
	if a.Tag() != b.Tag() {
		return false
	}
	return a == b
}

// propertyNameFromIndex converts an index to a property name string.
func propertyNameFromIndex(index uint32) string {
	if index == 0 {
		return "0"
	}
	if index == 1 {
		return "1"
	}
	buf := [20]byte{}
	i := len(buf)
	for index > 0 {
		i--
		buf[i] = byte('0' + index%10)
		index /= 10
	}
	return string(buf[i:])
}

// PreferredPrimitiveType hints for ToPrimitive.
type PreferredPrimitiveType uint8

const (
	NoPreference PreferredPrimitiveType = iota
	PreferNumber
	PreferString
)

// math constants
func mathNaN() float64   { return math.NaN() }
func mathInf() float64   { return math.Inf(1) }
func mathInfNeg() float64 { return math.Inf(-1) }
