// JSPromiseConstructor corresponds to JSC::JSPromiseConstructor (runtime/JSPromiseConstructor.h)
package runtime

// JSPromiseConstructor corresponds to JSC::JSPromiseConstructor.
type JSPromiseConstructor struct {
	InternalFunction
}

// NewJSPromiseConstructor creates a new JSPromiseConstructor.
func NewJSPromiseConstructor(vm *VM, structure *Structure, prototype *JSPromisePrototype) *JSPromiseConstructor {
	c := &JSPromiseConstructor{}
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
	// Static methods
	c.putDirectWithoutTransition(vm, NewPropertyName("resolve"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("reject"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("all"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("allSettled"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("any"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("race"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	_ = vm
}

// Call implements [[Call]] for JSPromiseConstructor.
func (c *JSPromiseConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	// Promise() must be called as a constructor
	globalObject.VM().ThrowException(globalObject, "TypeError: Promise constructor cannot be called without 'new'")
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for JSPromiseConstructor.
func (c *JSPromiseConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	// Simplified: creates a basic promise object
	promise := &JSPromise{}
	promise.structureID = 0
	promise.typ = ObjectType
	promise.cellState = DefinitelyWhite
	promise.properties = make(map[string]JSValue)
	return NewJSValueObject(&promise.JSObject), nil
}
