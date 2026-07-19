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

// SymbolSpecies is the well-known Symbol.species property name used for
// @@species accessor in ArrayConstructor and other built-in constructors.
const SymbolSpecies = "Symbol.species"

// SymbolIterator is the well-known Symbol.iterator property name.
const SymbolIterator = "Symbol.iterator"

// SymbolUnscopables is the well-known Symbol.unscopables property name.
const SymbolUnscopables = "Symbol.unscopables"

// SymbolHasInstance is the well-known Symbol.hasInstance property name.
const SymbolHasInstance = "Symbol.hasInstance"

// SymbolToPrimitive is the well-known Symbol.toPrimitive property name.
const SymbolToPrimitive = "Symbol.toPrimitive"

// SymbolMatch is the well-known Symbol.match property name.
const SymbolMatch = "Symbol.match"

// SymbolMatchAll is the well-known Symbol.matchAll property name.
const SymbolMatchAll = "Symbol.matchAll"

// SymbolReplace is the well-known Symbol.replace property name.
const SymbolReplace = "Symbol.replace"

// SymbolSplit is the well-known Symbol.split property name.
const SymbolSplit = "Symbol.split"

// SymbolSearch is the well-known Symbol.search property name.
const SymbolSearch = "Symbol.search"
