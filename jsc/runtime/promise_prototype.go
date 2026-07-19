// JSPromisePrototype corresponds to JSC::JSPromisePrototype (runtime/JSPromisePrototype.h)
package runtime

// JSPromisePrototype corresponds to JSC::JSPromisePrototype.
type JSPromisePrototype struct {
	JSNonFinalObject
}

// NewJSPromisePrototype creates a new JSPromisePrototype.
func NewJSPromisePrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *JSPromisePrototype {
	p := &JSPromisePrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation completes JSPromisePrototype initialization.
func (p *JSPromisePrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	// Instance methods
	p.putDirectWithoutTransition(vm, NewPropertyName("then"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("catch"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("finally"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
	// Symbol.toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Promise"), PropertyAttributeDontEnum)
}
