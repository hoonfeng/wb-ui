// NumberConstructor corresponds to JSC::NumberConstructor (runtime/NumberConstructor.h)
package runtime

// NumberConstructor corresponds to JSC::NumberConstructor.
type NumberConstructor struct {
	InternalFunction
}

// NewNumberConstructor creates a new NumberConstructor.
func NewNumberConstructor(vm *VM, structure *Structure, numberPrototype *NumberPrototype) *NumberConstructor {
	c := &NumberConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, numberPrototype)
	return c
}

// FinishCreation completes NumberConstructor initialization.
func (c *NumberConstructor) FinishCreation(vm *VM, numberPrototype *NumberPrototype) {
	c.InternalFunction.FinishCreation(vm, 1, "Number")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&numberPrototype.NumberObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	// Static properties
	c.putDirectWithoutTransition(vm, NewPropertyName("MAX_VALUE"), jsNumber(1.7976931348623157e+308), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("MIN_VALUE"), jsNumber(5e-324), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("NaN"), NewJSValueNumber(mathNaN()), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("NEGATIVE_INFINITY"), jsNumber(mathInfNeg()), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("POSITIVE_INFINITY"), jsNumber(mathInf()), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("EPSILON"), jsNumber(2.220446049250313e-16), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("MAX_SAFE_INTEGER"), jsNumber(9007199254740991), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("MIN_SAFE_INTEGER"), jsNumber(-9007199254740991), PropertyAttributeDontEnum)
	_ = vm
}

// Call implements [[Call]] for NumberConstructor.
func (c *NumberConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	if callFrame.argumentCount > 0 {
		return NewJSValueNumber(callFrame.arguments[0].ToNumber()), nil
	}
	return NewJSValueNumber(0), nil
}

// Construct implements [[Construct]] for NumberConstructor.
func (c *NumberConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	val := 0.0
	if callFrame.argumentCount > 0 {
		val = callFrame.arguments[0].ToNumber()
	}
	structure := globalObject.numberObjectStructure()
	obj := NewNumberObject(globalObject.VM(), structure)
	obj.SetInternalValue(jsNumber(val))
	return NewJSValueObject(&obj.JSObject), nil
}
