// Translation of: Source/JavaScriptCore/runtime/JSWeakSet.h
//
// JSWeakSet implements the ES WeakSet object (values are objects, held weakly).

package runtime

import "fmt"

// JSWeakSet corresponds to JSC::JSWeakSet.
// Values are objects (referenced weakly in spirit).
// Note: simplified implementation with strong references.
type JSWeakSet struct {
	JSObject
	data map[string]bool
}

// NewJSWeakSet creates a new empty WeakSet.
func NewJSWeakSet(vm *VM, structure *Structure) *JSWeakSet {
	ws := &JSWeakSet{
		data: make(map[string]bool),
	}
	ws.structureID = structure.structureID
	ws.typ = FinalObjectType
	ws.cellState = DefinitelyWhite
	ws.properties = make(map[string]JSValue)
	_ = vm
	return ws
}

// Add adds a value to the WeakSet.
func (ws *JSWeakSet) Add(value JSValue) {
	keyStr := jsValueToWeakSetKey(value)
	ws.data[keyStr] = true
}

// Has returns true if the value exists.
func (ws *JSWeakSet) Has(value JSValue) bool {
	keyStr := jsValueToWeakSetKey(value)
	_, ok := ws.data[keyStr]
	return ok
}

// Delete removes a value, returning true if it existed.
func (ws *JSWeakSet) Delete(value JSValue) bool {
	keyStr := jsValueToWeakSetKey(value)
	if _, ok := ws.data[keyStr]; ok {
		delete(ws.data, keyStr)
		return true
	}
	return false
}

// jsValueToWeakSetKey converts a JSValue to a set key string.
func jsValueToWeakSetKey(v JSValue) string {
	if v.IsObject() {
		return fmt.Sprintf("__obj_%p__", v.payload)
	}
	return v.ToString()
}
