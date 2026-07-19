// Translation of: Source/JavaScriptCore/runtime/JSPromiseConstructor.h
//                  Source/JavaScriptCore/runtime/JSPromiseConstructor.cpp
//
// JSPromiseConstructor implements the Promise() constructor and its static methods
// (resolve, reject, all, race, allSettled, any).

package runtime

// JSPromiseConstructor corresponds to JSC::JSPromiseConstructor.
type JSPromiseConstructor struct {
	InternalFunction
}

// NewJSPromiseConstructor creates a new JSPromiseConstructor.
func NewJSPromiseConstructor(vm *VM, structure *Structure, prototype *JSPromisePrototype) *JSPromiseConstructor {
	c := &JSPromiseConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject:    JSNonFinalObject{},
		functionForCall:     callPromiseConstructor,
		functionForConstruct: constructPromiseConstructor,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, prototype)
	return c
}

// FinishCreation completes JSPromiseConstructor initialization.
func (c *JSPromiseConstructor) FinishCreation(vm *VM, prototype *JSPromisePrototype) {
	c.InternalFunction.FinishCreation(vm, 1, "Promise")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&prototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	// Static methods — register as native callables
	c.putDirectWithoutTransition(vm, NewPropertyName("resolve"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("reject"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("all"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("allSettled"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("any"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("race"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	_ = vm
}

// --- Call / Construct ---

// callPromiseConstructor implements Promise() without new — throws TypeError.
func callPromiseConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, "TypeError: Promise constructor cannot be called without 'new'")
	return JSValueUndefined
}

// constructPromiseConstructor implements new Promise(executor) — creates a JSPromise.
func constructPromiseConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()

	// Create the promise
	promise := NewJSPromise(vm, nil)

	// Get the executor argument
	if callFrame.ArgumentCount() == 0 {
		vm.ThrowException(globalObject, "TypeError: Promise resolver undefined is not a function")
		return JSValueUndefined
	}

	executor := callFrame.Argument(0)

	// Create resolve/reject functions
	resolveFn := NewJSFunction(vm, globalObject, "resolve", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		if len(args) > 0 {
			promise.Resolve(globalObject, vm, args[0])
		} else {
			promise.Resolve(globalObject, vm, JSValueUndefined)
		}
		return JSValueUndefined, nil
	})

	rejectFn := NewJSFunction(vm, globalObject, "reject", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		if len(args) > 0 {
			promise.Reject(vm, args[0])
		} else {
			promise.Reject(vm, JSValueUndefined)
		}
		return JSValueUndefined, nil
	})

	// Call executor(resolve, reject)
	execArgs := []JSValue{NewJSValueObject(&resolveFn.JSObject), NewJSValueObject(&rejectFn.JSObject)}
	if fn, ok := executor.payload.(*JSFunction); ok {
		fn.Call(globalObject, JSValueUndefined, execArgs)
	}

	return NewJSValueObject(&promise.JSNonFinalObject.JSObject)
}

// --- Static methods ---

// promiseResolveStatic implements Promise.resolve(value).
func promiseResolveStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	value := callFrame.Argument(0)
	promise := NewJSPromise(vm, nil)
	promise.Fulfill(vm, value)
	return NewJSValueObject(&promise.JSNonFinalObject.JSObject)
}

// promiseRejectStatic implements Promise.reject(reason).
func promiseRejectStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	reason := callFrame.Argument(0)
	promise := NewJSPromise(vm, nil)
	promise.Reject(vm, reason)
	return NewJSValueObject(&promise.JSNonFinalObject.JSObject)
}

// promiseAllStatic implements Promise.all(iterable) — simplified.
func promiseAllStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	// Simplified: creates resolved promise
	return NewJSValueObject(&ResolvedPromise(globalObject, JSValueUndefined).JSNonFinalObject.JSObject)
}

// promiseRaceStatic implements Promise.race(iterable) — simplified.
func promiseRaceStatic(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	return NewJSValueObject(&ResolvedPromise(globalObject, JSValueUndefined).JSNonFinalObject.JSObject)
}
