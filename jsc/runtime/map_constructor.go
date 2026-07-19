// MapConstructor corresponds to JSC::MapConstructor (runtime/MapConstructor.h)
package runtime

// MapConstructor corresponds to JSC::MapConstructor.
type MapConstructor struct {
	InternalFunction
}

// NewMapConstructor creates a new MapConstructor.
func NewMapConstructor(vm *VM, structure *Structure, mapPrototype *MapPrototype) *MapConstructor {
	c := &MapConstructor{}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, mapPrototype)
	return c
}

// FinishCreation completes MapConstructor initialization.
func (c *MapConstructor) FinishCreation(vm *VM, mapPrototype *MapPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "Map")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&mapPrototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	_ = vm
}

// Call implements [[Call]] for MapConstructor.
func (c *MapConstructor) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	_ = callFrame
	// Map() must be called as a constructor
	globalObject.VM().ThrowException(globalObject, "TypeError: Map constructor cannot be called without 'new'")
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for MapConstructor.
func (c *MapConstructor) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = callFrame
	m := &JSMap{}
	m.structureID = 0
	m.typ = ObjectType
	m.cellState = DefinitelyWhite
	m.properties = make(map[string]JSValue)
	m.data = make(map[string]JSValue)
	_ = globalObject
	return NewJSValueObject(&m.JSObject), nil
}
