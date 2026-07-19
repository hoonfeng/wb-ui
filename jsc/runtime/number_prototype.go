// NumberPrototype corresponds to JSC::NumberPrototype (runtime/NumberPrototype.h)
package runtime

// NumberPrototype corresponds to JSC::NumberPrototype.
type NumberPrototype struct {
	NumberObject
}

// NewNumberPrototype creates a new NumberPrototype.
func NewNumberPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *NumberPrototype {
	p := &NumberPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = globalObject
	p.FinishCreation(vm)
	return p
}

// FinishCreation completes NumberPrototype initialization.
func (p *NumberPrototype) FinishCreation(vm *VM) {
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toFixed"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toExponential"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toPrecision"), NewJSValueObject(nil), PropertyAttributeDontEnum)
}
