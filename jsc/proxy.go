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
func proxyGet(in *Interpreter, proxy JSValue, prop string) JSValue {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "get", []JSValue{pd.target, StringValue(prop), proxy}); ok {
		return res
	}
	// Default: forward to target.
	return in.getProperty(pd.target, prop)
}

// proxySet implements the [[Set]] internal method for proxies.
func proxySet(in *Interpreter, proxy JSValue, prop string, value JSValue) bool {
	pd := proxy.AsObject().Internal.(*proxyData)
	handler := &pd.handler
	if res, ok := callProxyTrap(in, handler, "set", []JSValue{pd.target, StringValue(prop), value, proxy}); ok {
		return res.ToBoolean()
	}
	// Default: forward to target.
	if pd.target.IsObject() {
		targetObj := pd.target.AsObject()
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
