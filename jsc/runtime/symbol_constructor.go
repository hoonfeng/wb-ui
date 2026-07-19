// SymbolConstructor corresponds to JSC::SymbolConstructor (runtime/SymbolConstructor.h)
package runtime

// SymbolConstructor corresponds to JSC::SymbolConstructor.
type SymbolConstructor struct {
	InternalFunction
}

// NewSymbolConstructor creates a new SymbolConstructor.
func NewSymbolConstructor(vm *VM, structure *Structure, prototype *SymbolPrototype) *SymbolConstructor {
	c := &SymbolConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, prototype)
	return c
}

// FinishCreation completes SymbolConstructor initialization.
func (c *SymbolConstructor) FinishCreation(vm *VM, prototype *SymbolPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "Symbol")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&prototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	// Well-known symbols
	c.putDirectWithoutTransition(vm, NewPropertyName("iterator"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("asyncIterator"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("match"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("replace"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("search"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("split"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("hasInstance"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("isConcatSpreadable"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("unscopables"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("species"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("toPrimitive"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("toStringTag"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("matchAll"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	_ = vm
}

// Call implements [[Call]] for SymbolConstructor.
func (c *SymbolConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	desc := ""
	if callFrame.argumentCount > 0 {
		desc = callFrame.arguments[0].ToString()
	}
	return NewJSValueString("Symbol(" + desc + ")"), nil
}

// Construct is not allowed for Symbol (throws TypeError).
func (c *SymbolConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, "TypeError: Symbol is not a constructor")
	return JSValueUndefined, nil
}
