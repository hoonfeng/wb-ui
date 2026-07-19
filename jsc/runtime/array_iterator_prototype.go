// Translation of: Source/JavaScriptCore/runtime/ArrayIteratorPrototype.h
//                  Source/JavaScriptCore/runtime/ArrayIteratorPrototype.cpp
//
// ArrayIteratorPrototype provides the .next() method for array iterators.

package runtime

// ArrayIteratorPrototype corresponds to JSC::ArrayIteratorPrototype.
type ArrayIteratorPrototype struct {
	JSNonFinalObject
}

// NewArrayIteratorPrototype creates %ArrayIteratorPrototype%.
func NewArrayIteratorPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *ArrayIteratorPrototype {
	p := &ArrayIteratorPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers the .next() method.
func (p *ArrayIteratorPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	_ = globalObject
	// ArrayIteratorPrototype.next()
	p.putDirectWithoutTransition(vm, NewPropertyName("next"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	// @@toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Array Iterator"), PropertyAttributeDontEnum)
	_ = vm
}

// arrayIteratorProtoFuncNext implements %ArrayIteratorPrototype%.next():
//   Returns {value: ..., done: true/false}.
func arrayIteratorProtoFuncNext(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	thisVal := callFrame.ThisValue()

	// Extract the JSArrayIterator from `this`
	if it, ok := thisVal.payload.(*JSArrayIterator); ok {
		result := it.Next(globalObject)
		return NewJSValueObject(result)
	}

	globalObject.VM().ThrowException(globalObject, "TypeError: %ArrayIteratorPrototype%.next called on non-array-iterator")
	return JSValueUndefined
}
