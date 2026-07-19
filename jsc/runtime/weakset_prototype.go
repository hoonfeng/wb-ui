// WeakSetPrototype corresponds to JSC::WeakSetPrototype (runtime/WeakSetPrototype.h)
package runtime

// WeakSetPrototype corresponds to JSC::WeakSetPrototype.
type WeakSetPrototype struct {
	JSNonFinalObject
}

// NewWeakSetPrototype creates a new WeakSetPrototype.
func NewWeakSetPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *WeakSetPrototype {
	p := &WeakSetPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes WeakSetPrototype initialization.
func (p *WeakSetPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.putDirectWithoutTransition(vm, NewPropertyName("has"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("add"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("delete"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("WeakSet"), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
}
