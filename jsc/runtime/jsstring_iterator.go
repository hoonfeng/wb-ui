// Translation of: Source/JavaScriptCore/runtime/JSStringIterator.h
//
// JSStringIterator iterates over the code points of a string.

package runtime

// JSStringIterator corresponds to JSC::JSStringIterator.
// Iterates over a string's characters (as code points).
type JSStringIterator struct {
	JSObject
	iteratedString string // the string being iterated
	nextIndex      int    // current position (in runes)
}

// NewJSStringIterator creates a new string iterator.
func NewJSStringIterator(vm *VM, structure *Structure, str string) *JSStringIterator {
	it := &JSStringIterator{
		iteratedString: str,
		nextIndex:      0,
	}
	it.structureID = structure.structureID
	it.typ = FinalObjectType
	it.cellState = DefinitelyWhite
	it.properties = make(map[string]JSValue)
	_ = vm
	return it
}

// Next returns the next code point from the string.
func (it *JSStringIterator) Next(globalObject *JSGlobalObject) *JSObject {
	resultObj := constructEmptyObject(globalObject.VM(), nil)

	runes := []rune(it.iteratedString)
	if it.nextIndex >= len(runes) {
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), JSValueUndefined, 0)
		resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueTrue, 0)
		return resultObj
	}

	idx := it.nextIndex
	it.nextIndex++
	ch := runes[idx]
	value := NewJSValueString(string(ch))

	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("value"), value, 0)
	resultObj.putDirectWithoutTransition(globalObject.VM(), NewPropertyName("done"), JSValueFalse, 0)
	return resultObj
}
