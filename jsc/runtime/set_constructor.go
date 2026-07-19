// SetConstructor corresponds to JSC::SetConstructor (runtime/SetConstructor.h)
package runtime

// SetConstructor corresponds to JSC::SetConstructor.
type SetConstructor struct {
	InternalFunction
}

// NewSetConstructor creates a new SetConstructor.
func NewSetConstructor(vm *VM, structure *Structure, setPrototype *SetPrototype) *SetConstructor {
	c := &SetConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, setPrototype)
	return c
}

// FinishCreation completes SetConstructor initialization.
func (c *SetConstructor) FinishCreation(vm *VM, setPrototype *SetPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "Set")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&setPrototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	_ = vm
}

// Call implements [[Call]] for SetConstructor.
func (c *SetConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, "TypeError: Set constructor cannot be called without 'new'")
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for SetConstructor.
func (c *SetConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = callFrame
	s := &JSSet{}
	s.structureID = 0
	s.typ = ObjectType
	s.cellState = DefinitelyWhite
	s.properties = make(map[string]JSValue)
	s.data = make(map[string]bool)
	_ = globalObject
	return NewJSValueObject(&s.JSObject), nil
}
