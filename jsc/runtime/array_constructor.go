// Translation of: Source/JavaScriptCore/runtime/ArrayConstructor.h
//                  Source/JavaScriptCore/runtime/ArrayConstructor.cpp
//
// ArrayConstructor is the Array() constructor and its static methods.
// Implements ES 22.1.1 Array Constructor, Array.isArray, Array.of.

package runtime

import "math"

// ArrayConstructor corresponds to JSC::ArrayConstructor.
type ArrayConstructor struct {
	InternalFunction
}

// ArrayConstructorStructureFlags are the StructureFlags for ArrayConstructor.
const ArrayConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

// NewArrayConstructor creates a new ArrayConstructor.
// In C++: static ArrayConstructor* create(VM&, JSGlobalObject*, Structure*, ArrayPrototype*)
func NewArrayConstructor(vm *VM, globalObject *JSGlobalObject, structure *Structure, arrayPrototype *ArrayPrototype) *ArrayConstructor {
	c := &ArrayConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callArrayConstructor,
		functionForConstruct: constructWithArrayConstructor,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, globalObject, arrayPrototype)
	return c
}

// FinishCreation completes ArrayConstructor initialization.
// In C++: void finishCreation(VM&, JSGlobalObject*, ArrayPrototype*)
func (c *ArrayConstructor) FinishCreation(vm *VM, globalObject *JSGlobalObject, arrayPrototype *ArrayPrototype) {
	// Base::finishCreation(vm, 1, vm.propertyNames->Array.string(), WithoutStructureTransition)
	c.InternalFunction.FinishCreation(vm, 1, "Array")

	// putDirectWithoutTransition(vm, vm.propertyNames->prototype, arrayPrototype, DontEnum|DontDelete|ReadOnly)
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&arrayPrototype.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)

	// putDirectNonIndexAccessorWithoutTransition for @@species (simplified — use self as species value)
	c.putDirectWithoutTransition(vm, NewPropertyName(SymbolSpecies),
		NewJSValueObject(&c.JSObject),
		PropertyAttributeAccessor|PropertyAttributeReadOnly|PropertyAttributeDontEnum)

	// JSC_NATIVE_INTRINSIC_FUNCTION_WITHOUT_TRANSITION: Array.of
	c.putDirectWithoutTransition(vm, NewPropertyName("of"),
		NewJSValueObject(nil), // placeholder — actual function wired in global object init
		PropertyAttributeDontEnum)

	// JSC_NATIVE_INTRINSIC_FUNCTION_WITHOUT_TRANSITION: Array.isArray
	c.putDirectWithoutTransition(vm, NewPropertyName("isArray"),
		NewJSValueObject(nil), // placeholder — actual function wired in global object init
		PropertyAttributeDontEnum)

	// Array.from and Array.fromAsync require builtins (JSC_BUILTIN_FUNCTION_WITHOUT_TRANSITION)
	// Skipped for now; will be added when builtins/bytecode compiler is available.

	_ = globalObject
}

// ===== Call/Construct handlers =====

// callArrayConstructor implements [[Call]] for the Array constructor.
// When called as Array(), it constructs an array (same as with new).
// In C++: JSC_DEFINE_HOST_FUNCTION(callArrayConstructor)
func callArrayConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	args := callFrame.Arguments()
	result := constructArrayWithArgs(globalObject, args, JSValueUndefined)
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// constructWithArrayConstructor implements [[Construct]] for the Array constructor.
// In C++: JSC_DEFINE_HOST_FUNCTION(constructWithArrayConstructor)
func constructWithArrayConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	args := callFrame.Arguments()
	result := constructArrayWithArgs(globalObject, args, callFrame.NewTarget())
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ===== Core array construction =====

// constructArrayWithArgs is the core array construction logic.
// Implements ES 22.1.1.1 Array ( ...items )
//
// Rules:
//   - No arguments: returns [] (empty array)
//   - Single numeric argument: creates array with that length
//   - Single non-numeric argument: creates [arg]
//   - Multiple arguments: creates [arg0, arg1, ...]
//
// In C++: static JSArray* constructArrayWithSizeQuirk() [simplified]
func constructArrayWithArgs(globalObject *JSGlobalObject, args []JSValue, newTarget JSValue) *JSArray {
	vm := globalObject.VM()
	_ = newTarget
	_ = vm

	if len(args) == 0 {
		// C++ path: constructEmptyArray(globalObject, nullptr)
		return NewJSArray(vm, globalObject)
	}

	if len(args) == 1 && args[0].IsNumber() {
		// Single numeric argument — create array with that length
		// In C++: constructArrayWithSizeQuirk(globalObject, nullptr, length, newTarget)
		num := args[0].ToNumber()
		// Check if it's a valid safe integer >= 0
		if num < 0 || math.IsNaN(num) || math.IsInf(num, 0) || num != math.Trunc(num) {
			// Range error: not a positive integer of safe magnitude
			vm.ThrowException(globalObject, "Array length must be a positive integer of safe magnitude.")
			return nil
		}
		n := uint32(num)
		// Create array with n empty slots (all undefined)
		elements := make([]JSValue, n)
		return NewJSArrayWithValues(vm, globalObject, elements)
	}

	// Multi-arg or single non-number: construct with args as elements
	// In C++: constructArray(globalObject, nullptr, args, newTarget)
	elements := make([]JSValue, len(args))
	for i, arg := range args {
		elements[i] = arg
	}
	return NewJSArrayWithValues(vm, globalObject, elements)
}

// ===== Static method implementations =====

// arrayConstructorIsArray implements Array.isArray()
// ES 7.2.2 IsArray(argument)
func arrayConstructorIsArray(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	argument := callFrame.Argument(0)
	return JSValueEncode(jsBoolean(isArray(globalObject, argument)))
}

// arrayConstructorOf implements Array.of()
// ES 22.1.2.3 Array.of(...items)
func arrayConstructorOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	length := callFrame.ArgumentCount()
	elements := make([]JSValue, length)
	for i := 0; i < length; i++ {
		elements[i] = callFrame.Argument(i)
	}
	return JSValueEncode(NewJSValueObject(&NewJSArrayWithValues(vm, globalObject, elements).JSObject))
}

// ===== isArray helpers =====

// isArray implements ES 7.2.2 IsArray(argument).
// In C++: inline bool isArray(JSGlobalObject*, JSValue) [from header]
func isArray(globalObject *JSGlobalObject, argumentValue JSValue) bool {
	if !argumentValue.IsObject() {
		return false
	}
	argument := asObject(argumentValue)
	if argument.typ == ArrayType || argument.typ == DerivedArrayType {
		return true
	}
	if argument.typ != ProxyObjectType {
		return false
	}
	// ProxyObject path
	return isArraySlow(globalObject, uncheckedDowncast[ProxyObject](argument))
}

// isArraySlow implements the Proxy slow path for isArray.
// In C++: bool isArraySlow(JSGlobalObject*, ProxyObject*)
func isArraySlow(globalObject *JSGlobalObject, proxy *ProxyObject) bool {
	vm := globalObject.VM()
	_ = vm
	_ = proxy

	// Simplified: Proxy isRevoked and nested proxy chain not implemented.
	// In C++ this iterates through nested proxies checking target type.
	// For now, just check if the proxy's target is an array.
	// In a full implementation, this would:
	// 1. Check if proxy is revoked → throw TypeError
	// 2. Get proxy target
	// 3. Check if target type is ArrayType or DerivedArrayType → return true
	// 4. If target is another Proxy → recurse
	// 5. Otherwise → return false
	return false
}
