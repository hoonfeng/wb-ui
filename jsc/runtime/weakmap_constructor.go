// WeakMapConstructor corresponds to JSC::WeakMapConstructor (runtime/WeakMapConstructor.h)
package runtime

// WeakMapConstructor corresponds to JSC::WeakMapConstructor.
type WeakMapConstructor struct {
	InternalFunction
}

// NewWeakMapConstructor creates a new WeakMapConstructor.
func NewWeakMapConstructor(vm *VM, structure *Structure, weakMapPrototype *WeakMapPrototype) *WeakMapConstructor {
	c := &WeakMapConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, weakMapPrototype)
	return c
}

// FinishCreation completes WeakMapConstructor initialization.
func (c *WeakMapConstructor) FinishCreation(vm *VM, weakMapPrototype *WeakMapPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "WeakMap")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&weakMapPrototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	_ = vm
}

// Call implements [[Call]] for WeakMapConstructor.
func (c *WeakMapConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, "TypeError: WeakMap constructor cannot be called without 'new'")
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for WeakMapConstructor.
func (c *WeakMapConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = callFrame
	wm := &JSWeakMap{}
	wm.structureID = 0
	wm.typ = ObjectType
	wm.cellState = DefinitelyWhite
	wm.properties = make(map[string]JSValue)
	wm.data = make(map[string]JSValue)
	_ = globalObject
	return NewJSValueObject(&wm.JSObject), nil
}
