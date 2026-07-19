// BooleanConstructor corresponds to JSC::BooleanConstructor (runtime/BooleanConstructor.h)
package runtime

// BooleanConstructor corresponds to JSC::BooleanConstructor.
type BooleanConstructor struct {
	InternalFunction
}

// NewBooleanConstructor creates a new BooleanConstructor.
func NewBooleanConstructor(vm *VM, structure *Structure, booleanPrototype *BooleanPrototype) *BooleanConstructor {
	c := &BooleanConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, booleanPrototype)
	return c
}

// FinishCreation completes BooleanConstructor initialization.
func (c *BooleanConstructor) FinishCreation(vm *VM, booleanPrototype *BooleanPrototype) {
	c.InternalFunction.FinishCreation(vm, 1, "Boolean")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&booleanPrototype.BooleanObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
}

// Call implements [[Call]] for BooleanConstructor.
func (c *BooleanConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	// Boolean() called as a function converts the argument to boolean
	if callFrame.argumentCount > 0 {
		return jsBoolean(callFrame.arguments[0].ToBoolean()), nil
	}
	return jsBoolean(false), nil
}

// Construct implements [[Construct]] for BooleanConstructor.
func (c *BooleanConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	// new Boolean(value) returns a Boolean wrapper object
	val := false
	if callFrame.argumentCount > 0 {
		val = callFrame.arguments[0].ToBoolean()
	}
	structure := globalObject.booleanObjectStructure()
	obj := NewBooleanObject(globalObject.VM(), structure)
	obj.SetInternalValue(jsBoolean(val))
	return NewJSValueObject(&obj.JSObject), nil
}
