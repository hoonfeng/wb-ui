// Translation of: Source/JavaScriptCore/runtime/JSPromisePrototype.h
//                  Source/JavaScriptCore/runtime/JSPromisePrototype.cpp
//
// JSPromisePrototype implements Promise.prototype.then / .catch / .finally.

package runtime

// JSPromisePrototype corresponds to JSC::JSPromisePrototype.
type JSPromisePrototype struct {
	JSNonFinalObject
}

// NewJSPromisePrototype creates a new JSPromisePrototype.
func NewJSPromisePrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *JSPromisePrototype {
	p := &JSPromisePrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes JSPromisePrototype initialization.
func (p *JSPromisePrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	// Instance methods — register with native function callbacks
	p.putDirectWithoutTransition(vm, NewPropertyName("then"),
		NewJSValueObject(&NewJSFunction(vm, globalObject, "then", 2,
			func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
				callFrame := NewExecState(vm)
				callFrame.SetThisValue(thisValue)
				callFrame.SetArguments(args)
				return promiseProtoThen(globalObject, callFrame), nil
			}).JSObject), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("catch"),
		NewJSValueObject(&NewJSFunction(vm, globalObject, "catch", 1,
			func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
				callFrame := NewExecState(vm)
				callFrame.SetThisValue(thisValue)
				callFrame.SetArguments(args)
				return promiseProtoCatch(globalObject, callFrame), nil
			}).JSObject), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("finally"),
		NewJSValueObject(&NewJSFunction(vm, globalObject, "finally", 1,
			func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
				callFrame := NewExecState(vm)
				callFrame.SetThisValue(thisValue)
				callFrame.SetArguments(args)
				return promiseProtoFinally(globalObject, callFrame), nil
			}).JSObject), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
	// Symbol.toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Promise"), PropertyAttributeDontEnum)
}

// --- Promise.prototype methods ---

// promiseProtoThen implements Promise.prototype.then(onFulfilled, onRejected).
func promiseProtoThen(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisVal := callFrame.ThisValue()
	onFulfilled := callFrame.Argument(0)
	onRejected := callFrame.Argument(1)

	// Extract the promise
	promise := promiseFromValue(globalObject, thisVal)
	if promise == nil {
		return JSValueUndefined
	}

	// Perform then
	resultPromise := promise.Then(globalObject, onFulfilled, onRejected)
	return NewJSValueObject(&resultPromise.JSNonFinalObject.JSObject)
}

// promiseProtoCatch implements Promise.prototype.catch(onRejected).
func promiseProtoCatch(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisVal := callFrame.ThisValue()
	onRejected := callFrame.Argument(0)

	promise := promiseFromValue(globalObject, thisVal)
	if promise == nil {
		return JSValueUndefined
	}

	resultPromise := promise.Then(globalObject, JSValueUndefined, onRejected)
	return NewJSValueObject(&resultPromise.JSNonFinalObject.JSObject)
}

// promiseProtoFinally implements Promise.prototype.finally(onFinally).
func promiseProtoFinally(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisVal := callFrame.ThisValue()
	onFinally := callFrame.Argument(0)

	promise := promiseFromValue(globalObject, thisVal)
	if promise == nil {
		return JSValueUndefined
	}

	// Simplified: call onFinally and forward the result
	if onFinally.IsCallable() {
		if fn, ok := onFinally.payload.(*JSFunction); ok {
			fn.Call(globalObject, JSValueUndefined, nil)
		}
	}
	return thisVal
}

// --- Helper ---

// promiseFromValue extracts a JSPromise from a JSValue.
func promiseFromValue(globalObject *JSGlobalObject, val JSValue) *JSPromise {
	if val.IsCell() {
		if p, ok := val.payload.(*JSPromise); ok {
			return p
		}
	}
	globalObject.VM().ThrowException(globalObject, "TypeError: Promise method called on non-Promise value")
	return nil
}
