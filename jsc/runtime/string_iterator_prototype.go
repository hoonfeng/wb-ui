// Translation of: Source/JavaScriptCore/runtime/StringIteratorPrototype.h
package runtime

// StringIteratorPrototype corresponds to JSC::StringIteratorPrototype.
type StringIteratorPrototype struct {
	JSNonFinalObject
}

// NewStringIteratorPrototype creates %StringIteratorPrototype%.
func NewStringIteratorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *StringIteratorPrototype {
	p := &StringIteratorPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers .next() and @@toStringTag.
func (p *StringIteratorPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	_ = globalObject
	p.putDirectWithoutTransition(vm, NewPropertyName("next"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("String Iterator"), PropertyAttributeDontEnum)
	_ = vm
}

// stringIteratorProtoFuncNext implements %StringIteratorPrototype%.next().
func stringIteratorProtoFuncNext(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	thisVal := callFrame.ThisValue()
	if it, ok := thisVal.payload.(*JSStringIterator); ok {
		result := it.Next(globalObject)
		return NewJSValueObject(result)
	}
	globalObject.VM().ThrowException(globalObject, "TypeError: %StringIteratorPrototype%.next called on non-string-iterator")
	return JSValueUndefined
}
