// Translation of: Source/JavaScriptCore/runtime/JSMapIterator.h
//
// JSMapIterator is the iterator for Map objects (Map.prototype[Symbol.iterator]).

package runtime

import "fmt"

// JSMapIterator corresponds to JSC::JSMapIterator.
// Iterates over a Map's entries, keys, or values.
type JSMapIterator struct {
	JSObject
	iteratedMap   *JSMap        // the map being iterated
	iterationKind IterationKind // Keys / Values / Entries
	nextIndex     int           // current position in iteration
}

// NewJSMapIterator creates a new map iterator.
func NewJSMapIterator(vm *VM, structure *Structure, mapObj *JSMap, kind IterationKind) *JSMapIterator {
	it := &JSMapIterator{
		iteratedMap:   mapObj,
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

// Next returns the next value from the map iterator.
func (it *JSMapIterator) Next(globalObject *JSGlobalObject) *JSObject {
	resultObj := constructEmptyObject(globalObject.VM(), nil)

	if it.iteratedMap == nil || it.nextIndex >= len(it.iteratedMap.keys) {
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), JSValueUndefined, 0)
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueTrue, 0)
		return resultObj
	}

	idx := it.nextIndex
	it.nextIndex++
	keyVal := it.iteratedMap.keys[idx]

	// Convert key JSValue to string for Get access
	keyStr := keyVal.ToString()
	if keyVal.IsNumber() {
		keyStr = fmt.Sprintf("%v", keyVal.ToNumber())
	}

	var value JSValue
	switch it.iterationKind {
	case IterationKindKeys:
		value = keyVal
	case IterationKindValues:
		if v, ok := it.iteratedMap.data[keyStr]; ok {
			value = v
		} else {
			value = JSValueUndefined
		}
	case IterationKindEntries:
		mapVal := JSValueUndefined
		if v, ok := it.iteratedMap.data[keyStr]; ok {
			mapVal = v
		}
		entryArr := NewJSArray(globalObject.VM(), globalObject)
		entryArr.elements = append(entryArr.elements, keyVal, mapVal)
		value = NewJSValueObject(&entryArr.JSObject)
	}

	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), value, 0)
	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueFalse, 0)
	return resultObj
}
