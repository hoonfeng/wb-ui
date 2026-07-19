// Translation of: Source/JavaScriptCore/runtime/JSArray.h
//                  Source/JavaScriptCore/runtime/JSArray.cpp
//
// JSArray is the JSC array type. It stores indexed elements in a Go slice.

package runtime

// JSArray corresponds to JSC::JSArray. It holds indexed elements.
type JSArray struct {
	JSObject
	elements []JSValue
}

// StructureFlags for JSArray.
const JSArrayStructureFlags uint32 = JSObjectStructureFlags

// NewJSArray creates a new empty JSArray.
func NewJSArray(vm *VM, globalObject *JSGlobalObject) *JSArray {
	_ = vm
	_ = globalObject
	a := &JSArray{
		elements: make([]JSValue, 0, 4),
	}
	a.typ = ArrayType
	a.cellState = DefinitelyWhite
	a.properties = make(map[string]JSValue)
	a.indexingTypeAndMisc = ArrayWithUndecided
	return a
}

// NewJSArrayWithValues creates a JSArray from a slice of JSValues.
func NewJSArrayWithValues(vm *VM, globalObject *JSGlobalObject, values []JSValue) *JSArray {
	_ = vm
	_ = globalObject
	elements := make([]JSValue, len(values))
	copy(elements, values)
	a := &JSArray{elements: elements}
	a.typ = ArrayType
	a.cellState = DefinitelyWhite
	a.properties = make(map[string]JSValue)
	if len(values) > 0 {
		a.indexingTypeAndMisc = ArrayWithContiguous
	} else {
		a.indexingTypeAndMisc = ArrayWithUndecided
	}
	return a
}

// Length returns the array length.
func (a *JSArray) Length() uint32 { return uint32(len(a.elements)) }

// GetIndex returns the element at the given index.
func (a *JSArray) GetIndex(index uint32) JSValue {
	if index < uint32(len(a.elements)) {
		return a.elements[index]
	}
	return JSValueUndefined
}

// PutByIndex sets the element at the given index.
func (a *JSArray) PutByIndex(cell *JSCell, globalObject *JSGlobalObject, index uint32, value JSValue, shouldThrow bool) bool {
	_ = cell
	_ = globalObject
	_ = shouldThrow
	if index >= uint32(len(a.elements)) {
		// Grow
		newElements := make([]JSValue, index+1)
		copy(newElements, a.elements)
		a.elements = newElements
	}
	a.elements[index] = value
	a.indexingTypeAndMisc = ArrayWithContiguous
	return true
}

// Push appends an element and returns the new length.
func (a *JSArray) Push(globalObject *JSGlobalObject, value JSValue) uint32 {
	_ = globalObject
	a.elements = append(a.elements, value)
	a.indexingTypeAndMisc = ArrayWithContiguous
	return uint32(len(a.elements))
}

// Pop removes and returns the last element.
func (a *JSArray) Pop(globalObject *JSGlobalObject) JSValue {
	_ = globalObject
	if len(a.elements) == 0 {
		return JSValueUndefined
	}
	val := a.elements[len(a.elements)-1]
	a.elements = a.elements[:len(a.elements)-1]
	return val
}

// Shift removes and returns the first element.
func (a *JSArray) Shift(globalObject *JSGlobalObject) JSValue {
	_ = globalObject
	if len(a.elements) == 0 {
		return JSValueUndefined
	}
	val := a.elements[0]
	a.elements = a.elements[1:]
	return val
}

// Unshift prepends elements.
func (a *JSArray) Unshift(globalObject *JSGlobalObject, items ...JSValue) uint32 {
	_ = globalObject
	a.elements = append(items, a.elements...)
	a.indexingTypeAndMisc = ArrayWithContiguous
	return uint32(len(a.elements))
}

// Slice returns a sub-array copy [start, end).
func (a *JSArray) Slice(globalObject *JSGlobalObject, start, end int) *JSArray {
	_ = globalObject
	if start < 0 {
		start = 0
	}
	if end > len(a.elements) {
		end = len(a.elements)
	}
	if start >= end {
		return NewJSArray(nil, nil)
	}
	slice := a.elements[start:end]
	result := make([]JSValue, len(slice))
	copy(result, slice)
	return NewJSArrayWithValues(nil, nil, result)
}

// ForEach iterates over the array elements.
func (a *JSArray) ForEach(fn func(JSValue, uint32)) {
	for i, v := range a.elements {
		fn(v, uint32(i))
	}
}

// ToString returns a comma-separated string representation.
func (a *JSArray) ToString(globalObject *JSGlobalObject) string {
	_ = globalObject
	if len(a.elements) == 0 {
		return ""
	}
	result := ""
	for i, v := range a.elements {
		if i > 0 {
			result += ","
		}
		result += v.ToString()
	}
	return result
}

// SetLength sets the array length (truncating or extending).
func (a *JSArray) SetLength(length uint32) {
	if length < uint32(len(a.elements)) {
		a.elements = a.elements[:length]
	} else if length > uint32(len(a.elements)) {
		newElements := make([]JSValue, length)
		copy(newElements, a.elements)
		a.elements = newElements
	}
}
