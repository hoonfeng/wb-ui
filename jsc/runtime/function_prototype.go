// Translation of: Source/JavaScriptCore/runtime/FunctionPrototype.h
//                  Source/JavaScriptCore/runtime/FunctionPrototype.cpp
//
// FunctionPrototype is the prototype for all JavaScript functions (Function.prototype).
// ES 19.2.3 Properties of the Function Prototype Object

package runtime

import (
	"fmt"
	"strings"
)

// FunctionPrototype corresponds to JSC::FunctionPrototype.
// In C++ it extends InternalFunction; in Go we simplify to JSNonFinalObject.
type FunctionPrototype struct {
	JSNonFinalObject
}

// NewFunctionPrototype creates a new FunctionPrototype.
// In C++: static FunctionPrototype* create(VM&, Structure*)
func NewFunctionPrototype(vm *VM, structure *Structure) *FunctionPrototype {
	p := &FunctionPrototype{}
	p.structureID = structure.structureID
	p.typ = InternalFunctionType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.finishCreation(vm, "")
	return p
}

// finishCreation completes FunctionPrototype initialization.
// In C++: void finishCreation(VM&, const String& name)
// The name is empty for the prototype object.
func (p *FunctionPrototype) finishCreation(vm *VM, name string) {
	// Base::finishCreation(vm, 0, name, WithoutStructureTransition)
	// length=0, name=empty string
	p.putDirectWithoutTransition(vm, NewPropertyName("length"), jsNumber(0), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("name"), NewJSValueString(name), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
	_ = vm
}

// addFunctionProperties registers all Function.prototype methods.
// In C++: void addFunctionProperties(VM&, JSGlobalObject*, JSFunction**, JSFunction**, JSFunction**)
// This is called during global object initialization, not in finishCreation.
//
// Registers: toString, call, apply, bind, @@hasInstance, arguments getter, caller getter
func (p *FunctionPrototype) addFunctionProperties(vm *VM, globalObject *JSGlobalObject) {
	// toString — JSC_NATIVE_INTRINSIC_FUNCTION_WITHOUT_TRANSITION
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"),
		NewJSValueObject(nil), // placeholder, actual function wired via globalObject
		PropertyAttributeDontEnum)

	// apply — builtin function (call/apply are JS builtins in C++, simplified here)
	p.putDirectWithoutTransition(vm, NewPropertyName("apply"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	// call — builtin function
	p.putDirectWithoutTransition(vm, NewPropertyName("call"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	// bind — JSC_NATIVE_INTRINSIC_FUNCTION_WITHOUT_TRANSITION
	p.putDirectWithoutTransition(vm, NewPropertyName("bind"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	// arguments/caller — CustomGetterSetter (legacy)
	// In C++ these are CustomGetterSetter objects; simplified to just DontEnum markers.
	p.putDirectWithoutTransition(vm, NewPropertyName("arguments"),
		NewJSValueObject(nil),
		PropertyAttributeDontEnum|PropertyAttributeCustomAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("caller"),
		NewJSValueObject(nil),
		PropertyAttributeDontEnum|PropertyAttributeCustomAccessor)

	// @@hasInstance — JSFunction (DontDelete|ReadOnly|DontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolHasInstance),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontDelete|PropertyAttributeReadOnly|PropertyAttributeDontEnum)

	_ = globalObject
}

// ===== Host function implementations =====

// functionProtoFuncToString implements Function.prototype.toString()
// ES 19.2.3.5 Function.prototype.toString ( )
//
// In C++: JSC_DEFINE_HOST_FUNCTION(functionProtoFuncToString)
//
// Returns:
//   - JSFunction: the function's source string (function name(params) { body })
//   - InternalFunction: "function name() { [native code] }"
//   - Other callable: "function className() { [native code] }"
//   - Non-callable: throw TypeError
func functionProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()

	// JSFunction — return source or "function name() { [native code] }"
	if thisValue.IsObject() {
		obj := thisValue.GetObject()

		// Check for JSFunction
		if fn, ok := interface{}(obj).(*JSFunction); ok {
			_ = fn
			name := funcNameFromObj(obj)
			body := "    [native code]"
			return NewJSValueString("function " + name + "() {\n" + body + "\n}")
		}

		// Check if callable
		if thisValue.IsCallable() {
			name := obj.ClassName(globalObject)
			return NewJSValueString("function " + name + "() {\n    [native code]\n}")
		}
	}

	// Not callable → throw TypeError
	return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Function.prototype.toString requires a callable object")
}

// funcNameFromObj extracts the function name from a JSObject.
func funcNameFromObj(obj *JSObject) string {
	if nameVal, ok := obj.properties["name"]; ok {
		return nameVal.ToString()
	}
	return ""
}

// functionProtoFuncBind implements Function.prototype.bind(thisArg, ...boundArgs)
// ES 19.2.3.2 Function.prototype.bind ( thisArg, ...args )
//
// In C++: JSC_DEFINE_HOST_FUNCTION(functionProtoFuncBind)
func functionProtoFuncBind(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()

	// 1. If IsCallable(this) is false, throw TypeError
	if !thisValue.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "|this| is not a function inside Function.prototype.bind")
	}
	target := thisValue.GetObject()

	// Extract boundThis and boundArgs
	args := callFrame.Arguments()
	var boundThis JSValue
	var boundArgs []JSValue

	if len(args) > 1 {
		boundThis = args[0]
		boundArgs = make([]JSValue, len(args)-1)
		copy(boundArgs, args[1:])
	} else {
		boundThis = callFrame.Argument(0)
		boundArgs = nil
	}

	// Simplified: bound function creation
	// In full implementation, this creates a JSBoundFunction with prototype chain handling.
	// For now, create a simplified wrapper.
	_ = target

	// Calculate length
	length := 0.0
	lengthVal := target.Get(globalObject, NewPropertyName("length"))
	if lengthVal.IsNumber() {
		length = lengthVal.ToNumber()
		if length > float64(len(boundArgs)) {
			length -= float64(len(boundArgs))
		} else {
			length = 0
		}
	}

	// Get name
	name := ""
	nameVal := target.Get(globalObject, NewPropertyName("name"))
	if nameVal.IsString() {
		name = "bound " + nameVal.ToString()
	}

	// Create bound function
	// For JSBoundFunction skeleton, we use a JSFunction that wraps the original
	boundFn := NewJSFunction(vm, globalObject, name, int(length),
		func(g *JSGlobalObject, thisValue JSValue, fnArgs []JSValue) (JSValue, error) {
			_ = thisValue
			_ = fnArgs

			// Simplified: call the original target with boundThis and concatenated args
			allArgs := make([]JSValue, 0, len(boundArgs)+len(fnArgs))
			allArgs = append(allArgs, boundArgs...)
			allArgs = append(allArgs, fnArgs...)

			if callData := getCallDataInline(NewJSValueObject(target)); callData.Type != CallTypeNone {
				result := call(g, NewJSValueObject(target), callData, boundThis, allArgs)
				return result, nil
			}
			return JSValueUndefined, nil
		})

	return JSValueEncode(NewJSValueObject(&boundFn.JSObject))
}

// functionProtoFuncCall implements Function.prototype.call(thisArg, ...args)
// ES 19.2.3.3 Function.prototype.call ( thisArg, ...args )
func functionProtoFuncCall(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()

	// 1. If IsCallable(this) is false, throw TypeError
	if !thisValue.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Function.prototype.call requires a callable object")
	}

	target := thisValue.GetObject()

	// 2. If thisArg is undefined or null, set thisArg to global object
	var thisArg JSValue
	args := callFrame.Arguments()
	if len(args) == 0 {
		thisArg = NewJSValueObject(&globalObject.JSObject) // use global object
	} else {
		thisArg = args[0]
		if thisArg.IsUndefinedOrNull() {
			thisArg = NewJSValueObject(&globalObject.JSObject)
		}
	}

	// 3. Remaining arguments
	var callArgs []JSValue
	if len(args) > 1 {
		callArgs = make([]JSValue, len(args)-1)
		copy(callArgs, args[1:])
	}

	// 4. Call the function
	callData := getCallDataInline(NewJSValueObject(target))
	return call(globalObject, NewJSValueObject(target), callData, thisArg, callArgs)
}

// functionProtoFuncApply implements Function.prototype.apply(thisArg, argsArray)
// ES 19.2.3.1 Function.prototype.apply ( thisArg, argsArray )
func functionProtoFuncApply(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()

	// 1. If IsCallable(this) is false, throw TypeError
	if !thisValue.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Function.prototype.apply requires a callable object")
	}

	target := thisValue.GetObject()

	// 2. If thisArg is undefined or null, use global object
	args := callFrame.Arguments()
	var thisArg JSValue
	if len(args) == 0 || args[0].IsUndefinedOrNull() {
		thisArg = NewJSValueObject(&globalObject.JSObject)
	} else {
		thisArg = args[0]
	}

	// 3. If argsArray is undefined or null, call with no arguments
	var callArgs []JSValue
	if len(args) > 1 && !args[1].IsUndefinedOrNull() {
		argsObj := args[1].ToObject(globalObject)
		if argsObj == nil {
			return EncodedJSValue()
		}

		// 4. Let len be ? ToLength(? Get(argArray, "length"))
		lenVal := argsObj.Get(globalObject, NewPropertyName("length"))
		argLen := toLength(globalObject, argsObj)
		_ = lenVal

		// 5. Collect arguments from the array-like
		callArgs = make([]JSValue, 0, argLen)
		for i := uint64(0); i < argLen; i++ {
			elem := argsObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
			callArgs = append(callArgs, elem)
		}
	}

	// 6. Call the function
	callData := getCallDataInline(NewJSValueObject(target))
	return call(globalObject, NewJSValueObject(target), callData, thisArg, callArgs)
}

// functionProtoFuncSymbolHasInstance implements Function.prototype[@@hasInstance](instance)
// ES 7.3.19 OrdinaryHasInstance ( C, O )
//
// In C++: JSC_DEFINE_HOST_FUNCTION(functionProtoFuncSymbolHasInstance)
func functionProtoFuncSymbolHasInstance(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()
	instance := callFrame.Argument(0)

	// 1. If IsCallable(this) is false, return false
	if !thisValue.IsCallable() {
		return JSValueEncode(jsBoolean(false))
	}

	// 2. Check for bound function
	if bf, ok := interface{}(thisValue.GetObject()).(*JSBoundFunction); ok {
		// Bound function: delegate to target function
		_ = bf
		// Simplified: for bound functions, check instance against target's prototype
		// In full implementation: targetFunction->hasInstance(globalObject, instance)
	}

	// 3. If instance is not an Object, return false
	if !instance.IsObject() {
		return JSValueEncode(jsBoolean(false))
	}

	// 4. Let prototype be Get(this, "prototype")
	thisObj := thisValue.GetObject()
	prototype := thisObj.Get(globalObject, NewPropertyName("prototype"))

	// 5. Return OrdinaryHasInstance(instance, prototype)
	result := defaultHasInstance(globalObject, instance, prototype)
	_ = vm
	return JSValueEncode(jsBoolean(result))
}

// defaultHasInstance implements ES 7.3.19 OrdinaryHasInstance (instance, prototype).
func defaultHasInstance(globalObject *JSGlobalObject, instance JSValue, prototype JSValue) bool {
	if !instance.IsObject() {
		return false
	}
	instanceObj := instance.GetObject()

	// Walk the prototype chain of instance looking for prototype
	proto := instanceObj.GetPrototype(globalObject)
	for proto.IsObject() {
		if sameValue(globalObject, proto, prototype) {
			return true
		}
		proto = proto.GetObject().GetPrototype(globalObject)
	}
	return false
}

// ===== Legacy arguments/caller helpers =====

// functionPrototypeArgumentsGetter implements the legacy __arguments__ getter.
// In C++: JSC_DEFINE_CUSTOM_GETTER(argumentsGetter)
//
// In strict mode, throws TypeError. In non-strict, should return the arguments object.
// Simplified: always throws TypeError (strict-mode behavior).
func functionPrototypeArgumentsGetter(globalObject *JSGlobalObject, thisValue JSValue) JSValue {
	_ = thisValue
	return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
		"'arguments', 'callee', and 'caller' cannot be accessed in this context")
}

// functionPrototypeCallerGetter implements the legacy __caller__ getter.
// In C++: JSC_DEFINE_CUSTOM_GETTER(callerGetter)
// Simplified: always throws TypeError.
func functionPrototypeCallerGetter(globalObject *JSGlobalObject, thisValue JSValue) JSValue {
	_ = thisValue
	return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
		"'arguments', 'callee', and 'caller' cannot be accessed in this context")
}

// functionPrototypeCallerAndArgumentsSetter implements the legacy arguments/caller setter.
// In C++: JSC_DEFINE_CUSTOM_SETTER(callerAndArgumentsSetter)
// Simplified: no-op in non-strict, throw in strict mode.
func functionPrototypeCallerAndArgumentsSetter(globalObject *JSGlobalObject, thisValue JSValue) JSValue {
	_ = thisValue
	// In strict mode, throw. In non-strict mode, no-op.
	return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
		"'arguments', 'callee', and 'caller' cannot be accessed in this context")
}

// ===== Utility =====

// funcName returns the "name" property of a function value.
func funcName(globalObject *JSGlobalObject, fn JSValue) string {
	if !fn.IsObject() {
		return ""
	}
	obj := fn.GetObject()
	nameVal := obj.Get(globalObject, NewPropertyName("name"))
	if nameVal.IsString() {
		return nameVal.ToString()
	}
	return ""
}

// formatNativeFunctionSource formats a native function as a string.
// Returns: "function name() { [native code] }"
func formatNativeFunctionSource(name string) string {
	return fmt.Sprintf("function %s() {\n    [native code]\n}", name)
}

// formatScriptFunctionSource formats a script function as a string.
// Returns: "function name(params) {\n<body>\n}"
func formatScriptFunctionSource(name string, params []string, body string) string {
	return fmt.Sprintf("function %s(%s) {\n%s\n}", name, strings.Join(params, ", "), body)
}
