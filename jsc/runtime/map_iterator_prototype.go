// Translation of: Source/JavaScriptCore/runtime/MapIteratorPrototype.h
package runtime

// MapIteratorPrototype corresponds to JSC::MapIteratorPrototype.
type MapIteratorPrototype struct {
	JSNonFinalObject
}

// NewMapIteratorPrototype creates %MapIteratorPrototype%.
func NewMapIteratorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *MapIteratorPrototype {
	p := &MapIteratorPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers .next() and @@toStringTag.
func (p *MapIteratorPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	_ = globalObject
	p.putDirectWithoutTransition(vm, NewPropertyName("next"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Map Iterator"), PropertyAttributeDontEnum)
	_ = vm
}

// mapIteratorProtoFuncNext implements %MapIteratorPrototype%.next().
func mapIteratorProtoFuncNext(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	thisVal := callFrame.ThisValue()
	if it, ok := thisVal.payload.(*JSMapIterator); ok {
		result := it.Next(globalObject)
		return NewJSValueObject(result)
	}
	globalObject.VM().ThrowException(globalObject, "TypeError: %MapIteratorPrototype%.next called on non-map-iterator")
	return JSValueUndefined
}
