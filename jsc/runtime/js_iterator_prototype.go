// Translation of: Source/JavaScriptCore/runtime/JSIteratorPrototype.h
//
// JSIteratorPrototype is the base prototype for all JS iterators.
// It provides the @@iterator method so all iterators are iterable.

package runtime

// JSIteratorPrototype corresponds to JSC::JSIteratorPrototype.
// All iterator prototype objects inherit from this.
type JSIteratorPrototype struct {
	JSNonFinalObject
}

// NewJSIteratorPrototype creates the %IteratorPrototype%.
func NewJSIteratorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *JSIteratorPrototype {
	p := &JSIteratorPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers @@iterator on %IteratorPrototype%.
func (p *JSIteratorPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	_ = globalObject
	_ = vm
	// @@iterator[Symbol.iterator]() — returns `this`
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolIterator), NewJSValueObject(nil), PropertyAttributeDontEnum)
}

// iteratorProtoFuncIterator implements %IteratorPrototype%[@@iterator]():
//   Returns `this` (the iterator itself).
func iteratorProtoFuncIterator(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = globalObject
	return callFrame.ThisValue()
}
