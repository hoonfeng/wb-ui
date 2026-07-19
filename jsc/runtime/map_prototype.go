// MapPrototype corresponds to JSC::MapPrototype (runtime/MapPrototype.h)
package runtime

// MapPrototype corresponds to JSC::MapPrototype.
type MapPrototype struct {
	JSNonFinalObject
}

// NewMapPrototype creates a new MapPrototype.
func NewMapPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *MapPrototype {
	p := &MapPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes MapPrototype initialization.
func (p *MapPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.putDirectWithoutTransition(vm, NewPropertyName("get"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("set"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("has"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("delete"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("clear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("forEach"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("size"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("keys"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("values"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("entries"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Map"), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
}
