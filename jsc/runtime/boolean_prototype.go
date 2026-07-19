// BooleanPrototype corresponds to JSC::BooleanPrototype (runtime/BooleanPrototype.h)
package runtime

// BooleanPrototype corresponds to JSC::BooleanPrototype.
type BooleanPrototype struct {
	BooleanObject
}

// NewBooleanPrototype creates a new BooleanPrototype.
func NewBooleanPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *BooleanPrototype {
	p := &BooleanPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = globalObject
	p.FinishCreation(vm)
	return p
}

// FinishCreation completes BooleanPrototype initialization.
func (p *BooleanPrototype) FinishCreation(vm *VM) {
	// Add toString and valueOf methods
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
}
