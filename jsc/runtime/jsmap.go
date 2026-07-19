// Translation of: Source/JavaScriptCore/runtime/JSMap.h
//                  Source/JavaScriptCore/runtime/JSMap.cpp
//
// JSMap implements the ES Map object with ordered key-value storage.

package runtime

import "fmt"

// JSMap corresponds to JSC::JSMap.
// Stores key-value pairs with insertion-order iteration.
type JSMap struct {
	JSObject
	data   map[string]JSValue // key → value storage
	keys   []JSValue          // ordered keys for iteration
}

// NewJSMap creates a new empty Map.
func NewJSMap(vm *VM, structure *Structure) *JSMap {
	m := &JSMap{
		data: make(map[string]JSValue),
		keys: make([]JSValue, 0),
	}
	m.structureID = structure.structureID
	m.typ = FinalObjectType
	m.cellState = DefinitelyWhite
	m.properties = make(map[string]JSValue)
	_ = vm
	return m
}

// Get returns the value for the given key, or undefined if not found.
func (m *JSMap) Get(key JSValue) JSValue {
	keyStr := jsValueToMapKey(key)
	if v, ok := m.data[keyStr]; ok {
		return v
	}
	return JSValueUndefined
}

// Set sets the value for the given key, preserving insertion order.
func (m *JSMap) Set(key JSValue) {
	keyStr := jsValueToMapKey(key)
	if _, exists := m.data[keyStr]; !exists {
		m.keys = append(m.keys, key)
	}
	m.data[keyStr] = key
}

// Has returns true if the key exists in the map.
func (m *JSMap) Has(key JSValue) bool {
	keyStr := jsValueToMapKey(key)
	_, ok := m.data[keyStr]
	return ok
}

// Delete removes a key from the map, returning true if it existed.
func (m *JSMap) Delete(key JSValue) bool {
	keyStr := jsValueToMapKey(key)
	if _, ok := m.data[keyStr]; ok {
		delete(m.data, keyStr)
		// Remove from keys slice (linear scan)
		for i, k := range m.keys {
			if jsValueToMapKey(k) == keyStr {
				m.keys = append(m.keys[:i], m.keys[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

// Clear removes all entries from the map.
func (m *JSMap) Clear() {
	m.data = make(map[string]JSValue)
	m.keys = make([]JSValue, 0)
}

// Size returns the number of entries.
func (m *JSMap) Size() int {
	return len(m.keys)
}

// jsValueToMapKey converts a JSValue to a string map key.
func jsValueToMapKey(v JSValue) string {
	if v.IsString() {
		return v.ToString()
	}
	if v.IsNumber() {
		return fmt.Sprintf("__num_%v__", v.ToNumber())
	}
	if v.IsSymbol() {
		return v.ToString()
	}
	return v.ToString()
}
