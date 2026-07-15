// Translation of: Source/WebCore/bindings/js/JSDOMBinding.h
//                  Source/WebCore/bindings/js/JSDOMBinding.cpp
//                  Source/WebCore/bindings/js/JSDOMConvertAny.h
// Completeness: 65%
// Simplifications:
//   - no WeakMap for DOM nodes
//   - native functions are Go closures, not native codegen
//   - ToJSValue returns Undefined for unsupported Go types instead of throwing
//   - Go callback errors are swallowed (no JS throw mechanism is wired in this port)
//
// Filename note: this file implements the Go->JS bridge (the "go_to_js" surface). It is
// named go2js.go rather than go_to_js.go because Go applies an implicit GOOS=js build
// constraint to any file whose name ends in _js.go (and _js_test.go), which would make
// the file invisible on every other platform and break `go build ./...`. Using an
// underscore-free name avoids that constraint while preserving the descriptive intent.

package bindings

import (
	"reflect"

	"wb-ui/jsc"
)

// GoCallback is the signature for Go functions exposed to JS. The function receives
// the JS arguments as JSValues and returns a JSValue plus an optional error. When the
// error is non-nil the bridge returns undefined to JS (the interpreter has no native
// throw path).
type GoCallback func(args []jsc.JSValue) (jsc.JSValue, error)

// ToJSValue converts a Go value to a jsc.JSValue, mirroring toJS(exec, value) in
// JSDOMBinding. Supported kinds:
//   - bool, int/int32/int64/uint/uint32/uint64, float32/float64, string
//   - nil (any typed or untyped nil) -> null
//   - []any -> JS array (indexed)
//   - map[string]any -> JS plain object
//   - GoCallback -> JS function (native)
//   - struct (exported fields become properties)
//
// Unsupported values yield jsc.Undefined(). Slices/arrays created here use a nil
// prototype, so they expose indexing and .length but not Array.prototype methods; for
// fully-featured arrays build them through the interpreter.
func ToJSValue(v any) jsc.JSValue {
	if v == nil {
		return jsc.Null()
	}
	switch val := v.(type) {
	case bool:
		return jsc.BooleanValue(val)
	case int:
		return jsc.NumberValue(float64(val))
	case int32:
		return jsc.NumberValue(float64(val))
	case int64:
		return jsc.NumberValue(float64(val))
	case uint:
		return jsc.NumberValue(float64(val))
	case uint32:
		return jsc.NumberValue(float64(val))
	case uint64:
		return jsc.NumberValue(float64(val))
	case float32:
		return jsc.NumberValue(float64(val))
	case float64:
		return jsc.NumberValue(val)
	case string:
		return jsc.StringValue(val)
	case []any:
		elems := make([]jsc.JSValue, len(val))
		for i, e := range val {
			elems[i] = ToJSValue(e)
		}
		return jsc.ObjectValue(jsc.NewArray(nil, elems))
	case map[string]any:
		obj := jsc.NewObject(nil)
		for k, e := range val {
			obj.Set(k, ToJSValue(e))
		}
		return jsc.ObjectValue(obj)
	case GoCallback:
		return jsc.FunctionValue(nativeFromCallback(val))
	case jsc.JSValue:
		return val
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Func:
		// Only function values whose signature matches GoCallback are bridged to a
		// native JS function; a func of any other signature is not callable from JS in
		// this port.
		if cb, ok := asGoCallback(v); ok {
			return jsc.FunctionValue(nativeFromCallback(cb))
		}
		return jsc.Undefined()
	case reflect.Struct:
		obj := jsc.NewObject(nil)
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if !f.IsExported() {
				continue
			}
			obj.Set(f.Name, ToJSValue(rv.Field(i).Interface()))
		}
		return jsc.ObjectValue(obj)
	case reflect.Ptr:
		if rv.IsNil() {
			return jsc.Null()
		}
		return ToJSValue(rv.Elem().Interface())
	case reflect.Slice:
		n := rv.Len()
		elems := make([]jsc.JSValue, n)
		for i := 0; i < n; i++ {
			elems[i] = ToJSValue(rv.Index(i).Interface())
		}
		return jsc.ObjectValue(jsc.NewArray(nil, elems))
	}
	return jsc.Undefined()
}

// nativeFromCallback wraps a GoCallback as a JSFunction whose Native adapter invokes
// the callback. A non-nil error is swallowed and the call returns undefined.
func nativeFromCallback(cb GoCallback) *jsc.JSFunction {
	return jsc.NewNativeFunction("goFunction", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		v, err := cb(args)
		if err != nil {
			return jsc.Undefined()
		}
		return v
	}, 0)
}

// asGoCallback reports whether v is a Go function whose signature matches GoCallback
// (either the named GoCallback type itself, or an unnamed function literal with the
// same signature). When it matches it returns the value converted to GoCallback so it
// can be wrapped as a native JS function. Go's type switch only matches the exact
// dynamic type, so a plain `func([]jsc.JSValue) (jsc.JSValue, error)` literal is not
// caught by `case GoCallback:`; this helper uses reflect to bridge that gap.
func asGoCallback(v any) (GoCallback, bool) {
	if cb, ok := v.(GoCallback); ok {
		return cb, true
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Func {
		return nil, false
	}
	gcbType := reflect.TypeOf((*GoCallback)(nil)).Elem()
	if rv.Type().ConvertibleTo(gcbType) {
		return rv.Convert(gcbType).Interface().(GoCallback), true
	}
	return nil, false
}

// RegisterGoFunction exposes a Go function as a method on the JS global "go" object,
// mirroring how WebKit installs host functions on the global object. After registration
// the function is callable from JS as go.<name>(...). The "go" namespace object is
// created lazily on first registration.
func RegisterGoFunction(rt *jsc.Interpreter, name string, fn GoCallback) {
	goObj := ensureGoNamespace(rt)
	goObj.Set(name, jsc.FunctionValue(nativeFromCallback(fn)))
}

// RegisterGoObject exposes a Go map[string]any as a named JS global object. Nested
// maps become nested objects; GoCallback values become callable functions; other values
// are converted via ToJSValue. This mirrors registration of a host-defined interface
// object on the global scope.
func RegisterGoObject(rt *jsc.Interpreter, name string, obj map[string]any) {
	jsObj := convertMapToJSObject(rt, obj)
	rt.GlobalObject().Set(name, jsc.ObjectValue(jsObj))
}

// convertMapToJSObject builds a JSObject from a Go map, recursing into nested maps and
// wrapping GoCallback values (including matching unnamed function literals) as native
// functions.
func convertMapToJSObject(rt *jsc.Interpreter, obj map[string]any) *jsc.JSObject {
	jsObj := jsc.NewObject(rt.ObjectPrototype())
	jsObj.ClassName = "GoObject"
	for k, v := range obj {
		if cb, ok := asGoCallback(v); ok {
			jsObj.Set(k, jsc.FunctionValue(nativeFromCallback(cb)))
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			jsObj.Set(k, jsc.ObjectValue(convertMapToJSObject(rt, val)))
		default:
			jsObj.Set(k, ToJSValue(v))
		}
	}
	return jsObj
}

// ensureGoNamespace returns the global "go" object, creating it (with Object.prototype)
// on first access.
func ensureGoNamespace(rt *jsc.Interpreter) *jsc.JSObject {
	g := rt.GlobalObject()
	if v, ok := g.Get("go"); ok && v.IsObject() {
		return v.AsObject()
	}
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Go"
	g.Set("go", jsc.ObjectValue(obj))
	return obj
}

// CallJSFunction invokes a global JS function by name with Go arguments, mirroring
// JSC::call(exec, function, ...). Arguments are converted via ToJSValue. It is the
// Go->JS counterpart of RegisterGoFunction.
func CallJSFunction(rt *jsc.Interpreter, name string, args ...any) (jsc.JSValue, error) {
	jsArgs := make([]jsc.JSValue, len(args))
	for i, a := range args {
		jsArgs[i] = ToJSValue(a)
	}
	return rt.CallFunction(name, jsArgs...)
}
