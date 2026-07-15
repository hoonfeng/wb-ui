// Translation of: Source/JavaScriptCore/runtime/SetConstructor.cpp
//                  Source/JavaScriptCore/runtime/SetPrototype.cpp
//                  Source/JavaScriptCore/runtime/SetInstance.cpp
// Completeness: 60%
// Simplifications:
//   - Set uses a Go map[string]bool for existence with a parallel insertion-order
//     value slice, rather than C++'s HashMap + linked list.
//   - SameValueZero semantics: NaN is treated as equal to NaN; object values
//     are compared by pointer identity.
//   - The Set constructor does not accept an iterable initialiser; use add().
//   - Set.prototype.forEach and Iterator objects are omitted.

package jsc

// setStorage holds the internal data for a Set instance.
type setStorage struct {
	keys   []string
	values map[string]bool
}

func newSetStorage() *setStorage {
	return &setStorage{values: make(map[string]bool)}
}

func (ss *setStorage) add(key string) {
	if !ss.has(key) {
		ss.keys = append(ss.keys, key)
	}
	ss.values[key] = true
}

func (ss *setStorage) has(key string) bool {
	_, ok := ss.values[key]
	return ok
}

func (ss *setStorage) delete(key string) bool {
	if _, ok := ss.values[key]; ok {
		delete(ss.values, key)
		for i, k := range ss.keys {
			if k == key {
				ss.keys = append(ss.keys[:i], ss.keys[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

func (ss *setStorage) clear() {
	ss.keys = nil
	ss.values = make(map[string]bool)
}

func (ss *setStorage) len() int { return len(ss.keys) }

// setThis extracts the setStorage from a Set instance.
func setThis(v JSValue) *setStorage {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	if o.Internal == nil {
		return nil
	}
	ss, ok := o.Internal.(*setStorage)
	if !ok {
		return nil
	}
	return ss
}

// SetPrototype returns the Set.prototype object with all Set methods installed.
func (in *Interpreter) SetPrototype() *JSObject {
	if in.setProto != nil {
		return in.setProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "Set"

	// size getter.
	proto.SetAccessor("size", func(interp *Interpreter, this JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return IntValue(0)
		}
		return IntValue(s.len())
	}, nil)

	proto.Set("add", FunctionValue(NewNativeFunction("add", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return this
		}
		val := Undefined()
		if len(args) > 0 {
			val = args[0]
		}
		s.add(mapKey(val))
		return this
	}, 1)))

	proto.Set("has", FunctionValue(NewNativeFunction("has", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return BooleanValue(false)
		}
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(s.has(mapKey(args[0])))
	}, 1)))

	proto.Set("delete", FunctionValue(NewNativeFunction("delete", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return BooleanValue(false)
		}
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(s.delete(mapKey(args[0])))
	}, 1)))

	proto.Set("clear", FunctionValue(NewNativeFunction("clear", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s != nil {
			s.clear()
		}
		return Undefined()
	}, 0)))

	proto.Set("values", FunctionValue(NewNativeFunction("values", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		arr := make([]JSValue, 0, len(s.keys))
		for _, k := range s.keys {
			arr = append(arr, mapUnkey(k))
		}
		return ObjectValue(NewArray(in.arrayProto, arr))
	}, 0)))

	// Set.prototype.keys is an alias for values (per the spec).
	proto.Set("keys", FunctionValue(NewNativeFunction("keys", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		arr := make([]JSValue, 0, len(s.keys))
		for _, k := range s.keys {
			arr = append(arr, mapUnkey(k))
		}
		return ObjectValue(NewArray(in.arrayProto, arr))
	}, 0)))

	proto.Set("entries", FunctionValue(NewNativeFunction("entries", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		pairs := make([]JSValue, 0, len(s.keys))
		for _, k := range s.keys {
			v := mapUnkey(k)
			pairs = append(pairs, ObjectValue(NewArray(in.arrayProto, []JSValue{v, v})))
		}
		return ObjectValue(NewArray(in.arrayProto, pairs))
	}, 0)))

	proto.Set("forEach", FunctionValue(NewNativeFunction("forEach", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil || len(args) == 0 {
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
		for _, k := range s.keys {
			v := mapUnkey(k)
			_, _ = in.callValue(cb, thisArg, []JSValue{v, v, this})
		}
		return Undefined()
	}, 1)))

	in.setProto = proto
	return proto
}
