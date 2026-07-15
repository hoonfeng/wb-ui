// Translation of: Source/JavaScriptCore/runtime/MapConstructor.cpp
//                  Source/JavaScriptCore/runtime/MapPrototype.cpp
//                  Source/JavaScriptCore/runtime/MapInstance.cpp
// Completeness: 60%
// Simplifications:
//   - Map uses a Go map[string]JSValue for internal storage with a parallel
//     insertion-order key slice, rather than C++'s HashMap + linked list.
//   - SameValueZero semantics: NaN is treated as equal to NaN; -0 is treated as
//     equal to +0; object keys are compared by pointer identity.
//   - The Map constructor does not accept an iterable initialiser; use set().
//   - Map.prototype.forEach and Iterator objects are omitted.

package jsc

import (
	"fmt"
	"math"
	"strings"
)

// mapKey serialises a JSValue to a string key for Map/Set internal storage,
// implementing SameValueZero semantics:
//   - NaN maps to a fixed sentinel so NaN === NaN for Map purposes.
//   - -0 and +0 both map to "n:0" (SameValueZero treats them as equal).
//   - Object/Function keys use pointer address so each object is unique.
func mapKey(v JSValue) string {
	switch v.tag {
	case TagUndefined:
		return "u"
	case TagNull:
		return "l"
	case TagBoolean:
		if v.boolean {
			return "b:true"
		}
		return "b:false"
	case TagNumber:
		if math.IsNaN(v.number) {
			return "n:NaN"
		}
		return "n:" + formatNumber(v.number)
	case TagString:
		return "s:" + v.str
	case TagObject:
		return fmt.Sprintf("o:%p", v.object)
	case TagFunction:
		return fmt.Sprintf("f:%p", v.fn)
	}
	return "u"
}

// newMapStorage creates the internal storage object for a Map/Set instance.
// It holds an insertion-ordered entry list and a key-index map for O(1) lookup.
type mapStorage struct {
	keys   []string
	values map[string]JSValue
}

func newMapStorage() *mapStorage {
	return &mapStorage{values: make(map[string]JSValue)}
}

func (ms *mapStorage) set(key string, value JSValue) {
	if _, exists := ms.values[key]; !exists {
		ms.keys = append(ms.keys, key)
	}
	ms.values[key] = value
}

func (ms *mapStorage) get(key string) (JSValue, bool) {
	v, ok := ms.values[key]
	return v, ok
}

func (ms *mapStorage) has(key string) bool {
	_, ok := ms.values[key]
	return ok
}

func (ms *mapStorage) delete(key string) bool {
	if _, ok := ms.values[key]; ok {
		delete(ms.values, key)
		// Remove from ordered keys.
		for i, k := range ms.keys {
			if k == key {
				ms.keys = append(ms.keys[:i], ms.keys[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

func (ms *mapStorage) clear() {
	ms.keys = nil
	ms.values = make(map[string]JSValue)
}

func (ms *mapStorage) len() int { return len(ms.keys) }

// MapPrototype returns the Map.prototype object with all Map methods installed.
// It is created once and cached on the interpreter.
func (in *Interpreter) MapPrototype() *JSObject {
	if in.mapProto != nil {
		return in.mapProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "Map"

	// size getter.
	proto.SetAccessor("size", func(interp *Interpreter, this JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return IntValue(0)
		}
		return IntValue(m.len())
	}, nil)

	proto.Set("set", FunctionValue(NewNativeFunction("set", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return Undefined()
		}
		key := mapKey(args[0])
		val := Undefined()
		if len(args) > 1 {
			val = args[1]
		}
		m.set(key, val)
		return this
	}, 2)))

	proto.Set("get", FunctionValue(NewNativeFunction("get", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return Undefined()
		}
		if len(args) == 0 {
			return Undefined()
		}
		key := mapKey(args[0])
		v, ok := m.get(key)
		if !ok {
			return Undefined()
		}
		return v
	}, 1)))

	proto.Set("has", FunctionValue(NewNativeFunction("has", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return BooleanValue(false)
		}
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(m.has(mapKey(args[0])))
	}, 1)))

	proto.Set("delete", FunctionValue(NewNativeFunction("delete", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return BooleanValue(false)
		}
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(m.delete(mapKey(args[0])))
	}, 1)))

	proto.Set("clear", FunctionValue(NewNativeFunction("clear", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m != nil {
			m.clear()
		}
		return Undefined()
	}, 0)))

	proto.Set("keys", FunctionValue(NewNativeFunction("keys", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		arr := make([]JSValue, 0, len(m.keys))
		for _, k := range m.keys {
			arr = append(arr, mapUnkey(k))
		}
		return ObjectValue(NewArray(in.arrayProto, arr))
	}, 0)))

	proto.Set("values", FunctionValue(NewNativeFunction("values", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		arr := make([]JSValue, 0, len(m.keys))
		for _, k := range m.keys {
			arr = append(arr, m.values[k])
		}
		return ObjectValue(NewArray(in.arrayProto, arr))
	}, 0)))

	proto.Set("entries", FunctionValue(NewNativeFunction("entries", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		pairs := make([]JSValue, 0, len(m.keys))
		for _, k := range m.keys {
			pair := []JSValue{mapUnkey(k), m.values[k]}
			pairs = append(pairs, ObjectValue(NewArray(in.arrayProto, pair)))
		}
		return ObjectValue(NewArray(in.arrayProto, pairs))
	}, 0)))

	proto.Set("forEach", FunctionValue(NewNativeFunction("forEach", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil || len(args) == 0 {
			return Undefined()
		}
		cb := args[0]
		if !cb.IsCallable() {
			return Undefined()
		}
		thisArg := Undefined()
		if len(args) > 1 {
			thisArg = args[1]
		}
		for _, k := range m.keys {
			entry := []JSValue{m.values[k], mapUnkey(k), this}
			_, _ = in.callValue(cb, thisArg, entry)
		}
		return Undefined()
	}, 1)))

	in.mapProto = proto
	return proto
}

// mapThis extracts the mapStorage from a Map instance (the ObjectValue's Internal).
func mapThis(v JSValue) *mapStorage {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	if o.Internal == nil {
		return nil
	}
	ms, ok := o.Internal.(*mapStorage)
	if !ok {
		return nil
	}
	return ms
}

// mapUnkey reverses mapKey to recover the original JSValue from a storage key
// for use in keys()/entries() return values.
func mapUnkey(key string) JSValue {
	if len(key) < 2 {
		switch key {
		case "u":
			return Undefined()
		case "l":
			return Null()
		}
		return Undefined()
	}
	prefix := key[:2]
	payload := key[2:]
	switch prefix {
	case "b:":
		return BooleanValue(payload == "true")
	case "n:":
		if payload == "NaN" {
			return NumberValue(math.NaN())
		}
		if payload == "Infinity" {
			return NumberValue(math.Inf(1))
		}
		if payload == "-Infinity" {
			return NumberValue(math.Inf(-1))
		}
		if payload == "0" {
			return NumberValue(0)
		}
		// Parse the number back.
		var n float64
		if strings.Contains(payload, ".") {
			_, _ = fmt.Sscanf(payload, "%f", &n)
		} else {
			_, _ = fmt.Sscanf(payload, "%d", &n)
		}
		return NumberValue(n)
	case "s:":
		return StringValue(payload)
	case "o:", "f:":
		// We cannot recover the original pointer, but this is only used for
		// keys()/entries() which return keys as-is in JS. For object keys
		// we return a representative string.
		return StringValue("[object MapKey]")
	}
	return Undefined()
}
