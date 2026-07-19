// Translation of: Source/JavaScriptCore/runtime/JSArrayIterator.h
//
// JSArrayIterator is the iterator object returned by Array.prototype.keys(),
// Array.prototype.values(), and Array.prototype.entries().

package runtime

import "strconv"

// JSArrayIterator corresponds to JSC::JSArrayIterator.
// Stores iteration state: current index, iterated object, and iteration kind.
type JSArrayIterator struct {
	JSObject
	currentIndex    int            // current position in the array
	iteratedObject  *JSObject      // the array (or other iterable) being iterated
	iterationKind   IterationKind  // Keys / Values / Entries
}

// NewJSArrayIterator creates a new array iterator.
func NewJSArrayIterator(vm *VM, structure *Structure, iteratedObj *JSObject, kind IterationKind) *JSArrayIterator {
	it := &JSArrayIterator{
		currentIndex:   0,
		iteratedObject: iteratedObj,
		iterationKind:  kind,
	}
	it.structureID = structure.structureID
	it.typ = FinalObjectType
	it.cellState = DefinitelyWhite
	it.properties = make(map[string]JSValue)
	_ = vm
	return it
}

// Next advances the iterator and returns the next value.
// Returns {value: <nextValue>, done: true/false} as a JSObject.
func (it *JSArrayIterator) Next(globalObject *JSGlobalObject) *JSObject {
	resultObj := constructEmptyObject(globalObject.VM(), nil)

	// Check if iterated object exists and has length
	if it.iteratedObject == nil {
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), JSValueUndefined, 0)
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueTrue, 0)
		return resultObj
	}

	// Get length of iterated object
	lengthVal := it.iteratedObject.Get(globalObject, NewPropertyName("length"))
	length := 0
	if lengthVal.IsNumber() {
		length = int(lengthVal.ToNumber())
	}

	// Check if we've reached the end
	if it.currentIndex >= length {
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), JSValueUndefined, 0)
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueTrue, 0)
		return resultObj
	}

	// Get the current value based on iteration kind
	var value JSValue
	idx := it.currentIndex
	it.currentIndex++

	switch it.iterationKind {
	case IterationKindKeys:
		value = jsNumber(float64(idx))
	case IterationKindValues:
		value = it.iteratedObject.Get(globalObject, NewPropertyName(strconv.Itoa(idx)))
	case IterationKindEntries:
		elem := it.iteratedObject.Get(globalObject, NewPropertyName(strconv.Itoa(idx)))
		entryArr := NewJSArray(globalObject.VM(), globalObject)
		entryArr.elements = append(entryArr.elements, jsNumber(float64(idx)))
		entryArr.elements = append(entryArr.elements, elem)
		value = NewJSValueObject(&entryArr.JSObject)
	}

	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), value, 0)
	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueFalse, 0)
	return resultObj
}
