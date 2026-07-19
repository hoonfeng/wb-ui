// SetPrototype corresponds to JSC::SetPrototype (runtime/SetPrototype.h)
package runtime

// SetPrototype corresponds to JSC::SetPrototype.
type SetPrototype struct {
	JSNonFinalObject
}

// NewSetPrototype creates a new SetPrototype.
func NewSetPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *SetPrototype {
	p := &SetPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes SetPrototype initialization.
func (p *SetPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.putDirectWithoutTransition(vm, NewPropertyName("has"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("add"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("delete"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("clear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("forEach"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("size"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("keys"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("values"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("entries"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Set"), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
}
