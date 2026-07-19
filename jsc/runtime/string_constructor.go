// Translation of: Source/JavaScriptCore/runtime/StringConstructor.h
//                  Source/JavaScriptCore/runtime/StringConstructor.cpp
//
// StringConstructor implements the String() constructor.
// ES 21.1.1 The String Constructor
//
// NOTE: In C++, StringConstructor inherits JSFunction (not InternalFunction).
// In our Go translation, we use InternalFunction for consistency with the
// existing constructor pattern (like NumberConstructor, BooleanConstructor).
// The C++ distinction (JSFunction vs InternalFunction) is a memory optimization
// detail that does not affect the external behavior.

package runtime

// StringConstructor corresponds to JSC::StringConstructor.
type StringConstructor struct {
	InternalFunction
}

// StringConstructorStructureFlags are the StructureFlags for StringConstructor.
const StringConstructorStructureFlags uint32 = JSFunctionStructureFlags | HasStaticPropertyTable

// NewStringConstructor creates a new StringConstructor.
// In C++: static StringConstructor* create(VM&, Structure*, StringPrototype*)
func NewStringConstructor(vm *VM, structure *Structure, stringPrototype *StringPrototype) *StringConstructor {
	c := &StringConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callStringConstructorNative,
		functionForConstruct: constructWithStringConstructorNative,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, stringPrototype)
	return c
}

// FinishCreation completes StringConstructor initialization.
// In C++: void finishCreation(VM&, StringPrototype*)
func (c *StringConstructor) FinishCreation(vm *VM, stringPrototype *StringPrototype) {
	// Base::finishCreation(vm, 1, "String", WithoutStructureTransition)
	c.InternalFunction.FinishCreation(vm, 1, "String")

	// putDirectWithoutTransition(vm, vm.propertyNames->prototype, stringPrototype, ReadOnly|DontEnum|DontDelete)
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&stringPrototype.JSObject),
		PropertyAttributeReadOnly|PropertyAttributeDontEnum|PropertyAttributeDontDelete)

	// Static methods (C++: stringConstructorTable via .lut.h)
	// fromCharCode — DontEnum|Function 1
	c.putDirectWithoutTransition(vm, NewPropertyName("fromCharCode"),
		NewJSValueObject(nil), // placeholder — actual function wired in global object init
		PropertyAttributeDontEnum)

	// fromCodePoint — DontEnum|Function 1
	c.putDirectWithoutTransition(vm, NewPropertyName("fromCodePoint"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	// raw — DontEnum|Function 1 (JSBuiltin, requires bytecode)
	c.putDirectWithoutTransition(vm, NewPropertyName("raw"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	_ = vm
}

// ===== Call/Construct =====

// callStringConstructorNative implements [[Call]] for the String constructor.
// ES 21.1.1.1 String ( value ) — called as a function.
// In C++: String() → jsEmptyString, String(value) → ToString(value)
func callStringConstructorNative(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	if callFrame.ArgumentCount() == 0 {
		return JSValueEncode(NewJSValueString(""))
	}
	argument := callFrame.Argument(0)
	return JSValueEncode(stringConstructor(globalObject, argument))
}

// constructWithStringConstructorNative implements [[Construct]] for the String constructor.
// ES 21.1.1.1 String ( value ) — called with new.
// In C++: new String() → StringObject::create, new String(value) → ToString(value) then wrap
func constructWithStringConstructorNative(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	if callFrame.ArgumentCount() == 0 {
		// new String() → ""
		return JSValueEncode(NewJSValueString(""))
	}

	argument := callFrame.Argument(0)
	str := stringConstructor(globalObject, argument)
	return JSValueEncode(str)
}

// ===== Core functions =====

// stringConstructor is the core String() conversion logic.
// In C++: JSString* stringConstructor(JSGlobalObject*, JSValue)
//   - If argument is Symbol → Symbol::toString(globalObject)
//   - Otherwise → argument.toString(globalObject)
func stringConstructor(globalObject *JSGlobalObject, argument JSValue) JSValue {
	if argument.IsSymbol() {
		// Symbol → Symbol.prototype.toString()
		// Simplified: convert symbol to its description string
		symStr := argument.ToString()
		_ = globalObject
		return NewJSValueString(symStr)
	}
	// Default: ToString
	str := argument.ToString()
	_ = globalObject
	return NewJSValueString(str)
}

// ===== Static methods =====

// stringFromCharCodeStatic implements String.fromCharCode()
// ES 21.1.2.1 String.fromCharCode ( ...codes )
// In C++: JSC_DEFINE_HOST_FUNCTION(stringFromCharCode)
//
// Converts each argument to a UTF-16 code unit and returns the concatenated string.
func stringFromCharCodeStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	argCount := callFrame.ArgumentCount()

	if argCount == 0 {
		return JSValueEncode(NewJSValueString(""))
	}

	if argCount == 1 {
		// Fast path: single character
		code := uint32(callFrame.Argument(0).ToNumber())
		if code > 0xFFFF {
			code = 0xFFFD // replacement character for out-of-range
		}
		return JSValueEncode(NewJSValueString(string(rune(code))))
	}

	// Multi-argument: build string from code units
	var result []rune
	for i := 0; i < argCount; i++ {
		code := uint32(callFrame.Argument(i).ToNumber())
		if code > 0xFFFF {
			code = 0xFFFD
		}
		result = append(result, rune(code))
	}
	_ = vm
	return JSValueEncode(NewJSValueString(string(result)))
}

// stringFromCodePointStatic implements String.fromCodePoint()
// ES 21.1.2.2 String.fromCodePoint ( ...codePoints )
// In C++: JSC_DEFINE_HOST_FUNCTION(stringFromCodePoint)
//
// Converts each argument to a Unicode code point and returns the concatenated string.
// Throws RangeError for out-of-range values.
func stringFromCodePointStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	argCount := callFrame.ArgumentCount()

	if argCount == 0 {
		return JSValueEncode(NewJSValueString(""))
	}

	var result []rune
	for i := 0; i < argCount; i++ {
		codePoint := callFrame.Argument(i).ToNumber()

		// Check for valid code point range (0 to 0x10FFFF)
		if codePoint < 0 || codePoint > 0x10FFFF || codePoint != float64(uint32(codePoint)) {
			vm.ThrowException(globalObject, "RangeError: Invalid code point in String.fromCodePoint")
			return EncodedJSValue()
		}

		cp := uint32(codePoint)
		if cp <= 0xFFFF {
			// BMP code point — single rune
			result = append(result, rune(cp))
		} else {
			// Supplementary code point — encoded as surrogate pair
			// U+10000..U+10FFFF → surrogate pair
			cp -= 0x10000
			lead := rune(0xD800 + (cp >> 10))
			trail := rune(0xDC00 + (cp & 0x3FF))
			result = append(result, lead, trail)
		}
	}
	_ = vm
	return JSValueEncode(NewJSValueString(string(result)))
}

// stringFromCharCode is the single-char version.
// In C++: JSString* stringFromCharCode(JSGlobalObject*, int32_t)
func stringFromCharCode(globalObject *JSGlobalObject, code int32) JSValue {
	_ = globalObject
	if code < 0 || code > 0xFFFF {
		code = 0xFFFD
	}
	return NewJSValueString(string(rune(code)))
}

// stringFromCodePoint is the single-code-point version.
// In C++: JSString* stringFromCodePoint(JSGlobalObject*, int32_t)
func stringFromCodePoint(globalObject *JSGlobalObject, code int32) JSValue {
	_ = globalObject
	if code < 0 || uint32(code) > 0x10FFFF {
		return JSValueUndefined // Caller should check/throw
	}
	cp := uint32(code)
	if cp <= 0xFFFF {
		return NewJSValueString(string(rune(cp)))
	}
	// Supplementary code point
	cp -= 0x10000
	lead := rune(0xD800 + (cp >> 10))
	trail := rune(0xDC00 + (cp & 0x3FF))
	return NewJSValueString(string([]rune{lead, trail}))
}
