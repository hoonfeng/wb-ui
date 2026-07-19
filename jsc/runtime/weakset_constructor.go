// WeakSetConstructor corresponds to JSC::WeakSetConstructor (runtime/WeakSetConstructor.h)
package runtime

// WeakSetConstructor corresponds to JSC::WeakSetConstructor.
type WeakSetConstructor struct {
	InternalFunction
}

// NewWeakSetConstructor creates a new WeakSetConstructor.
func NewWeakSetConstructor(vm *VM, structure *Structure, weakSetPrototype *WeakSetPrototype) *WeakSetConstructor {
	c := &WeakSetConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, weakSetPrototype)
	return c
}

// FinishCreation completes WeakSetConstructor initialization.
func (c *WeakSetConstructor) FinishCreation(vm *VM, weakSetPrototype *WeakSetPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "WeakSet")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&weakSetPrototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	_ = vm
}

// Call implements [[Call]] for WeakSetConstructor.
func (c *WeakSetConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, "TypeError: WeakSet constructor cannot be called without 'new'")
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for WeakSetConstructor.
func (c *WeakSetConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = callFrame
	ws := &JSWeakSet{}
	ws.structureID = 0
	ws.typ = ObjectType
	ws.cellState = DefinitelyWhite
	ws.properties = make(map[string]JSValue)
	ws.data = make(map[string]bool)
	_ = globalObject
	return NewJSValueObject(&ws.JSObject), nil
}
