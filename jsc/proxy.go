// Translation of: Source/JavaScriptCore/runtime/ProxyObject.cpp
//                  Source/JavaScriptCore/runtime/ProxyObject.h
// Completeness: 50%
// Simplifications:
//   - Proxy traps are called synchronously; no revocation, no handler invariants.
//   - Proxy is a plain JSObject with proxyData in Internal; the interpreter checks
//     IsProxy before get/set/call/construct operations.
//   - No revocable proxy, no handler stacking.

package jsc

// proxyData stores the target and handler for a Proxy instance.
type proxyData struct {
	target  JSValue
	handler JSObject
}

// IsProxy reports whether v is a Proxy object.
func IsProxy(v JSValue) bool {
	if !v.IsObject() {
		return false
	}
	_, ok := v.AsObject().Internal.(*proxyData)
	return ok
}

// proxyTarget returns the proxy's target value, or nil if not a proxy.
func proxyTarget(v JSValue) JSValue {
	if !IsProxy(v) {
		return JSValue{}
	}
	return v.AsObject().Internal.(*proxyData).target
}

// proxyHandler returns the proxy's handler object, or nil if not a proxy.
func proxyHandler(v JSValue) *JSObject {
	if !IsProxy(v) {
		return nil
	}
	pd := v.AsObject().Internal.(*proxyData)
	return &pd.handler
}

// callProxyTrap calls a named trap on the handler. Returns the result value.
// If the trap is not defined, returns (JSValue{}, false) so the caller can
// fall through to the default behaviour.
func callProxyTrap(in *Interpreter, handler *JSObject, trapName string, args []JSValue) (JSValue, bool) {
	trap, ok := handler.Get(trapName)
	if !ok || !trap.IsCallable() {
		return JSValue{}, false
	}
	res, exc := in.callValue(trap, ObjectValue(handler), args)
	if exc != nil {
		return Undefined(), true // error
	}
	return res, true
}

// proxyGet implements the [[Get]] internal method for proxies.
// Optional receiver overrides the `this` passed to the get trap (defaults to proxy).
func proxyGet(in *Interpreter, proxy JSValue, prop string, receiver ...JSValue) JSValue {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	recv := proxy
	if len(receiver) > 0 {
		recv = receiver[0]
	}
	if res, ok := callProxyTrap(in, handler, "get", []JSValue{pd.target, StringValue(prop), recv}); ok {
		return res
	}
	// Default: forward to target with receiver binding for accessors.
	return in.getProperty(pd.target, prop, recv)
}

// proxySet implements the [[Set]] internal method for proxies.
func proxySet(in *Interpreter, proxy JSValue, prop string, value JSValue) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "set", []JSValue{pd.target, StringValue(prop), value, proxy}); ok {
		return res.ToBoolean()
	}
	// Default: forward to target with receiver binding for accessors.
	if pd.target.IsObject() {
		targetObj := pd.target.AsObject()
		// Walk prototype chain to find accessor
		cur := targetObj
		for cur != nil {
			if a := cur.Accessor(prop); a != nil {
				if a.Setter != nil {
					a.Setter(in, proxy, value)
					return true
				}
				return false // accessor without setter
			}
			if _, ok := cur.Properties[prop]; ok {
				break
			}
			cur = cur.Prototype
		}
		targetObj.Set(prop, value)
		return true
	}
	return false
}

// proxyHas implements the [[HasProperty]] internal method for proxies.
func proxyHas(in *Interpreter, proxy JSValue, prop string) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "has", []JSValue{pd.target, StringValue(prop)}); ok {
		return res.ToBoolean()
	}
	// Default: forward to target.
	if pd.target.IsObject() {
		_, ok := pd.target.AsObject().Get(prop)
		return ok
	}
	return false
}

// proxyDeleteProperty implements the [[Delete]] internal method for proxies.
func proxyDeleteProperty(in *Interpreter, proxy JSValue, prop string) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "deleteProperty", []JSValue{pd.target, StringValue(prop)}); ok {
		return res.ToBoolean()
	}
	// Default: forward to target.
	if pd.target.IsObject() {
		return pd.target.AsObject().Delete(prop)
	}
	return true
}

// proxyApply implements the [[Call]] internal method for proxies.
func proxyApply(in *Interpreter, proxy JSValue, thisArg JSValue, args []JSValue) (JSValue, *jsException) {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	res, ok := callProxyTrap(in, handler, "apply", []JSValue{pd.target, thisArg, ObjectValue(newArgsArray(in, args))})
	if ok {
		return res, nil
	}
	// Default: forward to target.
	if pd.target.IsCallable() {
		return in.callValue(pd.target, thisArg, args)
	}
	return Undefined(), &jsException{value: StringValue("TypeError: proxy target is not callable")}
}

// proxyConstruct implements the [[Construct]] internal method for proxies.
func proxyConstruct(in *Interpreter, proxy JSValue, args []JSValue) (JSValue, *jsException) {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	res, ok := callProxyTrap(in, handler, "construct", []JSValue{pd.target, ObjectValue(newArgsArray(in, args))})
	if ok {
		return res, nil
	}
	// Default: forward to target.
	return in.construct(pd.target, args)
}

// proxyOwnKeys implements [[OwnPropertyKeys]] for proxies.
func proxyOwnKeys(in *Interpreter, proxy JSValue) []string {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "ownKeys", []JSValue{pd.target}); ok {
		if res.IsObject() {
			// Convert array-like result to []string
			var keys []string
			arr := res.AsObject()
			if arr.IsArray {
				for _, elem := range arr.Elements {
					keys = append(keys, elem.String())
				}
			}
			return keys
		}
	}
	// Default: forward to target
	if pd.target.IsObject() {
		var keys []string
		target := pd.target.AsObject()
		for k := range target.Properties {
			keys = append(keys, k)
		}
		return keys
	}
	return nil
}

// proxyGetOwnPropertyDescriptor implements [[GetOwnProperty]] for proxies.
func proxyGetOwnPropertyDescriptor(in *Interpreter, proxy JSValue, prop string) (JSValue, bool) {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "getOwnPropertyDescriptor", []JSValue{pd.target, StringValue(prop)}); ok {
		return res, true
	}
	// Default: no property descriptor
	return Undefined(), false
}

// proxyDefineProperty implements [[DefineOwnProperty]] for proxies.
func proxyDefineProperty(in *Interpreter, proxy JSValue, prop string, desc JSValue) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "defineProperty", []JSValue{pd.target, StringValue(prop), desc}); ok {
		return res.ToBoolean()
	}
	// Default: forward to target
	if pd.target.IsObject() {
		pd.target.AsObject().Set(prop, desc)
		return true
	}
	return false
}

// proxyGetPrototypeOf implements [[GetPrototypeOf]] for proxies.
func proxyGetPrototypeOf(in *Interpreter, proxy JSValue) *JSObject {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "getPrototypeOf", []JSValue{pd.target}); ok {
		if res.IsObject() {
			return res.AsObject()
		}
	}
	// Default
	if pd.target.IsObject() {
		return pd.target.AsObject().Prototype
	}
	return nil
}

// proxySetPrototypeOf implements [[SetPrototypeOf]] for proxies.
func proxySetPrototypeOf(in *Interpreter, proxy JSValue, proto *JSObject) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	protoVal := Undefined()
	if proto != nil {
		protoVal = ObjectValue(proto)
	}
	if res, ok := callProxyTrap(in, handler, "setPrototypeOf", []JSValue{pd.target, protoVal}); ok {
		return res.ToBoolean()
	}
	// Default
	if pd.target.IsObject() {
		pd.target.AsObject().Prototype = proto
		return true
	}
	return false
}

// proxyPreventExtensions implements [[PreventExtensions]] for proxies.
func proxyPreventExtensions(in *Interpreter, proxy JSValue) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "preventExtensions", []JSValue{pd.target}); ok {
		return res.ToBoolean()
	}
	return false
}

// proxyIsExtensible implements [[IsExtensible]] for proxies.
func proxyIsExtensible(in *Interpreter, proxy JSValue) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "isExtensible", []JSValue{pd.target}); ok {
		return res.ToBoolean()
	}
	return true
}

// newArgsArray creates a JS array from a Go []JSValue.
func newArgsArray(in *Interpreter, args []JSValue) *JSObject {
	elems := make([]JSValue, len(args))
	copy(elems, args)
	return NewArray(in.arrayProto, elems)
}

// ProxyConstructor returns the Proxy constructor function.
func (in *Interpreter) ProxyConstructor() *JSFunction {
	return NewNativeFunction("Proxy", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return Undefined()
		}
		target := args[0]
		handler := args[1]
		if !handler.IsObject() {
			// TypeError would be more appropriate, but return undefined for simplicity.
			return Undefined()
		}
		obj := NewObject(in.objectProto)
		obj.ClassName = "Proxy"
		obj.Internal = &proxyData{
			target:  target,
			handler: *handler.AsObject(),
		}
		return ObjectValue(obj)
	}, 2)
}
