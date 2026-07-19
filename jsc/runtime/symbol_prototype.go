// SymbolPrototype corresponds to JSC::SymbolPrototype (runtime/SymbolPrototype.h)
package runtime

// SymbolPrototype corresponds to JSC::SymbolPrototype.
type SymbolPrototype struct {
	JSNonFinalObject
}

// NewSymbolPrototype creates a new SymbolPrototype.
func NewSymbolPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *SymbolPrototype {
	p := &SymbolPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = globalObject
	p.FinishCreation(vm)
	return p
}

// FinishCreation completes SymbolPrototype initialization.
func (p *SymbolPrototype) FinishCreation(vm *VM) {
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Symbol"), PropertyAttributeDontEnum)
	_ = vm
}

// SymbolToStringTag is the well-known Symbol.toStringTag property name.
const SymbolToStringTag = "Symbol.toStringTag"
