// Translation of: Source/JavaScriptCore/runtime/SetIteratorPrototype.h
package runtime

// SetIteratorPrototype corresponds to JSC::SetIteratorPrototype.
type SetIteratorPrototype struct {
	JSNonFinalObject
}

// NewSetIteratorPrototype creates %SetIteratorPrototype%.
func NewSetIteratorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *SetIteratorPrototype {
	p := &SetIteratorPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers .next() and @@toStringTag.
func (p *SetIteratorPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	_ = globalObject
	p.putDirectWithoutTransition(vm, NewPropertyName("next"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Set Iterator"), PropertyAttributeDontEnum)
	_ = vm
}

// setIteratorProtoFuncNext implements %SetIteratorPrototype%.next().
func setIteratorProtoFuncNext(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	thisVal := callFrame.ThisValue()
	if it, ok := thisVal.payload.(*JSSetIterator); ok {
		result := it.Next(globalObject)
		return NewJSValueObject(result)
	}
	globalObject.VM().ThrowException(globalObject, "TypeError: %SetIteratorPrototype%.next called on non-set-iterator")
	return JSValueUndefined
}
