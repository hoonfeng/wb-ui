// Translation of: Source/JavaScriptCore/runtime/ReflectObject.cpp
// Completeness: 50%
// Simplifications:
//   - Reflect methods are static methods on the Reflect object; no receiver
//     forwarding beyond what Reflect.get/set already support.
//   - Reflect.get/set use getProperty/setProperty on target directly (no proxy
//     trap re-triggering — that's by design per the spec).

package jsc

// ReflectConstructor builds and returns the Reflect object with static methods.
func (in *Interpreter) ReflectObject() *JSObject {
	ref := NewObject(in.objectProto)
	ref.ClassName = "Reflect"

	// Reflect.get(target, prop, receiver)
	ref.Set("get", FunctionValue(NewNativeFunction("get", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return Undefined()
		}
		target := args[0]
		prop := args[1]
		propStr := prop.ToString()
		var recv JSValue
		if len(args) >= 3 {
			recv = args[2]
		} else {
			recv = target
		}
		if target.IsObject() {
			// For array numeric indices, use getIndex instead of getProperty
			if target.IsObject() && target.AsObject().IsArray {
				if prop.IsNumber() {
					idx := int(prop.AsNumber())
					if v, ok := target.AsObject().GetIndex(idx); ok {
						return v
					}
					return Undefined()
				}
				// Also handle numeric string keys
				if n, ok := numericIndex(propStr); ok && n >= 0 {
					if v, ok := target.AsObject().GetIndex(n); ok {
						return v
					}
					return Undefined()
				}
			}
			return in.getProperty(target, propStr, recv)
		}
		return Undefined()
	}, 2)))

	// Reflect.set(target, prop, value, receiver)
	ref.Set("set", FunctionValue(NewNativeFunction("set", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 3 {
			return BooleanValue(false)
		}
		target := args[0]
		prop := args[1]
		value := args[2]
		propStr := prop.ToString()
		receiver := target
		if len(args) >= 4 {
			receiver = args[3]
		}
		if target.IsObject() {
			o := target.AsObject()
			// For array numeric indices, use SetIndex
			if o.IsArray {
				if prop.IsNumber() {
					idx := int(prop.AsNumber())
					o.SetIndex(idx, value)
					if idx >= len(o.Elements) {
						o.Set("length", NumberValue(float64(idx+1)))
					}
					return BooleanValue(true)
				}
				if n, ok := numericIndex(propStr); ok && n >= 0 {
					o.SetIndex(n, value)
					if n >= len(o.Elements) {
						o.Set("length", NumberValue(float64(n+1)))
					}
					return BooleanValue(true)
				}
			}
			// Walk prototype chain to find accessor with setter
			cur := o
			for cur != nil {
				if a := cur.Accessor(propStr); a != nil {
					if a.Setter != nil {
						a.Setter(in, receiver, value)
						return BooleanValue(true)
					}
					// Accessor without setter → return false per spec
					return BooleanValue(false)
				}
				if _, ok := cur.Properties[propStr]; ok {
					break
				}
				cur = cur.Prototype
			}
			// No accessor found: set directly on target
			o.Set(propStr, value)
			return BooleanValue(true)
		}
		return BooleanValue(false)
	}, 3)))

	// Reflect.has(target, prop)
	ref.Set("has", FunctionValue(NewNativeFunction("has", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return BooleanValue(false)
		}
		target := args[0]
		prop := args[1].ToString()
		if target.IsObject() {
			_, ok := target.AsObject().Get(prop)
			return BooleanValue(ok)
		}
		return BooleanValue(false)
	}, 2)))

	// Reflect.deleteProperty(target, prop)
	ref.Set("deleteProperty", FunctionValue(NewNativeFunction("deleteProperty", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return BooleanValue(false)
		}
		target := args[0]
		prop := args[1].ToString()
		if target.IsObject() {
			return BooleanValue(target.AsObject().Delete(prop))
		}
		return BooleanValue(true)
	}, 2)))

	// Reflect.apply(target, thisArg, args)
	ref.Set("apply", FunctionValue(NewNativeFunction("apply", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 3 {
			return Undefined()
		}
		target := args[0]
		thisArg := args[1]
		argsArg := args[2]
		var callArgs []JSValue
		if argsArg.IsObject() && argsArg.AsObject().IsArray {
			callArgs = argsArg.AsObject().Elements
		}
		if target.IsCallable() {
			res, exc := in.callValue(target, thisArg, callArgs)
			if exc != nil {
				return Undefined()
			}
			return res
		}
		return Undefined()
	}, 3)))

	// Reflect.construct(target, args)
	ref.Set("construct", FunctionValue(NewNativeFunction("construct", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return Undefined()
		}
		target := args[0]
		argsArg := args[1]
		var callArgs []JSValue
		if argsArg.IsObject() && argsArg.AsObject().IsArray {
			callArgs = argsArg.AsObject().Elements
		}
		if target.IsCallable() {
			res, exc := in.construct(target, callArgs)
			if exc != nil {
				return Undefined()
			}
			return res
		}
		return Undefined()
	}, 2)))

	return ref
}
