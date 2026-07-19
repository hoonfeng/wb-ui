// Translation of: Source/JavaScriptCore/runtime/JSSetIterator.h
//
// JSSetIterator is the iterator for Set objects (Set.prototype[Symbol.iterator]).

package runtime

// JSSetIterator corresponds to JSC::JSSetIterator.
// Iterates over a Set's values.
type JSSetIterator struct {
	JSObject
	iteratedSet   *JSSet        // the set being iterated
	iterationKind IterationKind // Keys / Values / Entries
	nextIndex     int           // current position in iteration
}

// NewJSSetIterator creates a new set iterator.
func NewJSSetIterator(vm *VM, structure *Structure, setObj *JSSet, kind IterationKind) *JSSetIterator {
	it := &JSSetIterator{
		iteratedSet:   setObj,
		iterationKind: kind,
		nextIndex:     0,
	}
	it.structureID = structure.structureID
	it.typ = FinalObjectType
	it.cellState = DefinitelyWhite
	it.properties = make(map[string]JSValue)
	_ = vm
	return it
}

// Next returns the next value from the set iterator.
func (it *JSSetIterator) Next(globalObject *JSGlobalObject) *JSObject {
	resultObj := constructEmptyObject(globalObject.VM(), nil)

	if it.iteratedSet == nil || it.nextIndex >= len(it.iteratedSet.values) {
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), JSValueUndefined, 0)
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueTrue, 0)
		return resultObj
	}

	idx := it.nextIndex
	it.nextIndex++

	val := it.iteratedSet.values[idx]
	var value JSValue
	switch it.iterationKind {
	case IterationKindKeys:
		value = val
	case IterationKindValues:
		value = val
	case IterationKindEntries:
		entryArr := NewJSArray(globalObject.VM(), globalObject)
		entryArr.elements = append(entryArr.elements, val, val)
		value = NewJSValueObject(&entryArr.JSObject)
	}

	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), value, 0)
	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueFalse, 0)
	return resultObj
}
