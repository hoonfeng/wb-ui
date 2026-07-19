// ErrorPrototypeBase and ErrorPrototype correspond to JSC::ErrorPrototypeBase and JSC::ErrorPrototype
// (runtime/ErrorPrototype.h/.cpp)
package runtime

// ErrorPrototypeBase is the superclass for ErrorPrototype, NativeErrorPrototype, and AggregateErrorPrototype.
type ErrorPrototypeBase struct {
	JSNonFinalObject
}

// NewErrorPrototypeBase creates a new ErrorPrototypeBase.
func NewErrorPrototypeBase(vm *VM, structure *Structure) *ErrorPrototypeBase {
	proto := &ErrorPrototypeBase{}
	proto.structureID = structure.structureID
	proto.typ = ObjectType
	proto.cellState = DefinitelyWhite
	proto.properties = make(map[string]JSValue)
	_ = vm
	return proto
}

// FinishCreation completes ErrorPrototypeBase initialization.
func (b *ErrorPrototypeBase) FinishCreation(vm *VM, name string) {
	_ = name
	// In JSC, this sets the toString function and the error name
	b.putDirectWithoutTransition(vm, NewPropertyName("name"), NewJSValueString(name), PropertyAttributeDontEnum)
	b.putDirectWithoutTransition(vm, NewPropertyName("message"), NewJSValueString(""), PropertyAttributeDontEnum)
}

// ErrorPrototype corresponds to JSC::ErrorPrototype.
type ErrorPrototype struct {
	ErrorPrototypeBase
}

// NewErrorPrototype creates a new ErrorPrototype.
func NewErrorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *ErrorPrototype {
	proto := &ErrorPrototype{}
	proto.structureID = structure.structureID
	proto.typ = ObjectType
	proto.cellState = DefinitelyWhite
	proto.properties = make(map[string]JSValue)
	_ = globalObject
	proto.FinishCreation(vm, "Error")
	return proto
}

// FinishCreation completes ErrorPrototype initialization.
func (p *ErrorPrototype) FinishCreation(vm *VM, name string) {
	p.ErrorPrototypeBase.FinishCreation(vm, name)
	// Add toString method
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
}
