// ErrorConstructor corresponds to JSC::ErrorConstructor (runtime/ErrorConstructor.h/.cpp)
package runtime

// ErrorConstructor corresponds to JSC::ErrorConstructor.
type ErrorConstructor struct {
	InternalFunction
}

// NewErrorConstructor creates a new ErrorConstructor.
func NewErrorConstructor(vm *VM, structure *Structure, errorPrototype *ErrorPrototype) *ErrorConstructor {
	c := &ErrorConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, errorPrototype)
	return c
}

// FinishCreation completes ErrorConstructor initialization.
func (c *ErrorConstructor) FinishCreation(vm *VM, errorPrototype *ErrorPrototype) {
	c.InternalFunction.FinishCreation(vm, 1, "Error")
	// Set prototype property
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&errorPrototype.ErrorPrototypeBase.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
}

// Call implements [[Call]] for ErrorConstructor.
func (c *ErrorConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	// When called as a function, Error() returns a new Error object
	return c.Construct(globalObject, callFrame)
}

// Construct implements [[Construct]] for ErrorConstructor.
func (c *ErrorConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	message := ""
	if callFrame.argumentCount > 0 {
		message = callFrame.arguments[0].ToString()
	}
	err := NewErrorInstance(globalObject.VM(), nil, ErrorTypeError)
	err.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("message"), NewJSValueString(message), PropertyAttributeDontEnum)
	return NewJSValueObject(&err.JSNonFinalObject.JSObject), nil
}
