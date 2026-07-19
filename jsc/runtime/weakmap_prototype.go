// WeakMapPrototype corresponds to JSC::WeakMapPrototype (runtime/WeakMapPrototype.h)
package runtime

// WeakMapPrototype corresponds to JSC::WeakMapPrototype.
type WeakMapPrototype struct {
	JSNonFinalObject
}

// NewWeakMapPrototype creates a new WeakMapPrototype.
func NewWeakMapPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *WeakMapPrototype {
	p := &WeakMapPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes WeakMapPrototype initialization.
func (p *WeakMapPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.putDirectWithoutTransition(vm, NewPropertyName("get"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("set"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("has"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("delete"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("WeakMap"), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
}
