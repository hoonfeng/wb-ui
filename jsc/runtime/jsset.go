// Translation of: Source/JavaScriptCore/runtime/JSSet.h
//                  Source/JavaScriptCore/runtime/JSSet.cpp
//
// JSSet implements the ES Set object with ordered value storage.

package runtime

import "fmt"

// JSSet corresponds to JSC::JSSet.
// Stores unique values with insertion-order iteration.
type JSSet struct {
	JSObject
	data   map[string]bool // value → presence
	values []JSValue       // ordered values for iteration
}

// NewJSSet creates a new empty Set.
func NewJSSet(vm *VM, structure *Structure) *JSSet {
	s := &JSSet{
		data:   make(map[string]bool),
		values: make([]JSValue, 0),
	}
	s.structureID = structure.structureID
	s.typ = FinalObjectType
	s.cellState = DefinitelyWhite
	s.properties = make(map[string]JSValue)
	_ = vm
	return s
}

// Add adds a value to the set (no-op if already present).
func (s *JSSet) Add(value JSValue) {
	keyStr := jsValueToSetKey(value)
	if _, exists := s.data[keyStr]; !exists {
		s.data[keyStr] = true
		s.values = append(s.values, value)
	}
}

// Has returns true if the value exists in the set.
func (s *JSSet) Has(value JSValue) bool {
	keyStr := jsValueToSetKey(value)
	_, ok := s.data[keyStr]
	return ok
}

// Delete removes a value from the set, returning true if it existed.
func (s *JSSet) Delete(value JSValue) bool {
	keyStr := jsValueToSetKey(value)
	if _, ok := s.data[keyStr]; ok {
		delete(s.data, keyStr)
		for i, v := range s.values {
			if jsValueToSetKey(v) == keyStr {
				s.values = append(s.values[:i], s.values[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

// Clear removes all values from the set.
func (s *JSSet) Clear() {
	s.data = make(map[string]bool)
	s.values = make([]JSValue, 0)
}

// Size returns the number of values.
func (s *JSSet) Size() int {
	return len(s.values)
}

// Values returns all values for iteration.
func (s *JSSet) Values() []JSValue {
	return s.values
}

// jsValueToSetKey converts a JSValue to a string set key.
func jsValueToSetKey(v JSValue) string {
	if v.IsString() {
		return v.ToString()
	}
	if v.IsNumber() {
		return fmt.Sprintf("__num_%v__", v.ToNumber())
	}
	return v.ToString()
}
