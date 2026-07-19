// Translation of: Source/JavaScriptCore/runtime/JSWeakMap.h
//
// JSWeakMap implements the ES WeakMap object (keys are objects, held weakly).

package runtime

import "fmt"

// JSWeakMap corresponds to JSC::JSWeakMap.
// Keys are objects (referenced weakly in spirit), values can be any JS type.
// Note: Go GC doesn't support weak references natively, so this is a simplified
// implementation that holds strong references.
type JSWeakMap struct {
	JSObject
	data map[string]JSValue
}

// NewJSWeakMap creates a new empty WeakMap.
func NewJSWeakMap(vm *VM, structure *Structure) *JSWeakMap {
	wm := &JSWeakMap{
		data: make(map[string]JSValue),
	}
	wm.structureID = structure.structureID
	wm.typ = FinalObjectType
	wm.cellState = DefinitelyWhite
	wm.properties = make(map[string]JSValue)
	_ = vm
	return wm
}

// Get returns the value for the given key, or undefined.
func (wm *JSWeakMap) Get(key JSValue) JSValue {
	keyStr := jsValueToWeakMapKey(key)
	if v, ok := wm.data[keyStr]; ok {
		return v
	}
	return JSValueUndefined
}

// Set sets the value for the given key.
func (wm *JSWeakMap) Set(key JSValue, value JSValue) {
	keyStr := jsValueToWeakMapKey(key)
	wm.data[keyStr] = value
}

// Has returns true if the key exists.
func (wm *JSWeakMap) Has(key JSValue) bool {
	keyStr := jsValueToWeakMapKey(key)
	_, ok := wm.data[keyStr]
	return ok
}

// Delete removes a key, returning true if it existed.
func (wm *JSWeakMap) Delete(key JSValue) bool {
	keyStr := jsValueToWeakMapKey(key)
	if _, ok := wm.data[keyStr]; ok {
		delete(wm.data, keyStr)
		return true
	}
	return false
}

// jsValueToWeakMapKey converts a JSValue to a map key string.
// In ES, WeakMap keys must be objects.
func jsValueToWeakMapKey(v JSValue) string {
	if v.IsObject() {
		return fmt.Sprintf("__obj_%p__", v.payload)
	}
	return v.ToString()
}
