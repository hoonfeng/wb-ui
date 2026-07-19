// Package jsc implements a JavaScript runtime translated from
// Source/JavaScriptCore. This root package re-exports key types from
// the sub-packages (runtime/, interpreter/, bytecode/, parser/, etc.)
// to maintain backward compatibility with the old flat-package API.
package jsc

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"wb-ui/jsc/interpreter"
	"wb-ui/jsc/runtime"
)

// ---------------------------------------------------------------------------
// Type aliases — these are the same types used throughout jsc/runtime/.
// ---------------------------------------------------------------------------

type (
	JSValue  = runtime.JSValue
	JSObject = runtime.JSObject
	JSFunction = runtime.JSFunction
)

// Pre-defined JSValues.
var (
	Undefined = func() JSValue { return runtime.JSValueUndefined }
	Null      = func() JSValue { return runtime.JSValueNull }
)

// Value constructors.
func ObjectValue(obj *JSObject) JSValue   { return runtime.NewJSValueObject(obj) }
func StringValue(s string) JSValue        { return runtime.NewJSValueString(s) }
func NumberValue(n float64) JSValue       { return runtime.NewJSValueNumber(n) }
func BooleanValue(b bool) JSValue         { return runtime.NewJSValueBool(b) }
func FunctionValue(fn *JSFunction) JSValue { return runtime.NewJSValueObject(&fn.JSObject) }
func SymbolValue(sym *runtime.Symbol) JSValue { _ = sym; return runtime.JSValueUndefined }

// ---------------------------------------------------------------------------
// NativeFunc is the signature for Go-native JS callback functions.
// ---------------------------------------------------------------------------

// NativeFunc is a Go callback used with NewNativeFunction. It receives the
// interpreter (for access to the global object and other context), the this
// value, and the argument list. It returns a JSValue.
type NativeFunc func(rt *Interpreter, thisVal JSValue, args []JSValue) JSValue

// ---------------------------------------------------------------------------
// Logger interface (console output sink).
// ---------------------------------------------------------------------------

// Logger receives console.log/error/warn output.
type Logger interface {
	Log(level string, msg string)
}

// DefaultLogger writes console output to stdout.
type DefaultLogger struct{}

func (DefaultLogger) Log(level string, msg string) {
	if level == "" {
		fmt.Println(msg)
		return
	}
	fmt.Printf("%s: %s\n", level, msg)
}

// BufferLogger records output in memory (useful in tests).
type BufferLogger struct {
	Lines []string
}

func (b *BufferLogger) Log(level string, msg string) {
	if level == "" {
		b.Lines = append(b.Lines, msg)
		return
	}
	b.Lines = append(b.Lines, level+": "+msg)
}

func (b *BufferLogger) Clear()     { b.Lines = nil }
func (b *BufferLogger) String() string { return strings.Join(b.Lines, "\n") }

// ---------------------------------------------------------------------------
// Interpreter — the main JavaScript runtime handle.
// ---------------------------------------------------------------------------

// Interpreter wraps the low-level interpreter and VM, providing the same
// API that wb-ui consumers (bindings/, page/, webkit/) expect.
type Interpreter struct {
	vm          *runtime.VM
	interp      *interpreter.Interpreter
	globalObj   *runtime.JSGlobalObject

	// Cached prototypes and constructors.
	objectPrototype    *runtime.JSObject
	arrayPrototype     *runtime.JSObject
	functionPrototype  *runtime.JSObject
	stringPrototype    *runtime.JSObject
	numberPrototype    *runtime.JSObject
	booleanPrototype   *runtime.JSObject

	// Cached constructors.
	objectConstructor   *runtime.JSObject
	arrayConstructor    *runtime.JSObject
	functionConstructor *runtime.JSObject
	stringConstructor   *runtime.JSObject
	numberConstructor   *runtime.JSObject
	booleanConstructor  *runtime.JSObject
	dateConstructor     *runtime.JSObject
	regexpConstructor   *runtime.JSObject
	errorConstructor    *runtime.JSObject
	symbolConstructor   *runtime.JSObject
	promiseConstructor  *runtime.JSObject
	mapConstructor      *runtime.JSObject
	setConstructor      *runtime.JSObject
	weakMapConstructor  *runtime.JSObject
	weakSetConstructor  *runtime.JSObject
}

// NewInterpreter creates a new JavaScript interpreter with a default VM and
// global object.
func NewInterpreter() *Interpreter {
	vm := runtime.NewVM()
	globalObj := runtime.NewJSGlobalObject(vm, nil)
	vm.GlobalObject = globalObj

	// Create interpreter.
	interp := interpreter.NewInterpreter(vm)

	// Create the interpreter wrapper.
	rt := &Interpreter{
		vm:        vm,
		interp:    interp,
		globalObj: globalObj,
	}

	// Set up core prototypes.
	rt.setupCoreObjects()

	return rt
}

// setupCoreObjects creates the root prototype chain.
func (rt *Interpreter) setupCoreObjects() {
	// The base object prototype: a plain JSObject with Object.prototype
	baseProto := runtime.NewJSObject(rt.vm, nil)
	rt.objectPrototype = baseProto

	// Array prototype inherits from object prototype.
	arrProto := runtime.NewJSObject(rt.vm, nil)
	arrProto.Set("length", runtime.NewJSValueNumber(0))
	rt.arrayPrototype = arrProto

	// Function prototype inherits from object prototype.
	rt.functionPrototype = runtime.NewJSObject(rt.vm, nil)

	// String, Number, Boolean prototypes.
	rt.stringPrototype = runtime.NewJSObject(rt.vm, nil)
	rt.numberPrototype = runtime.NewJSObject(rt.vm, nil)
	rt.booleanPrototype = runtime.NewJSObject(rt.vm, nil)

	// Set up the global object.
	rt.globalObj.Set("Object", ObjectValue(rt.objectPrototype))
	rt.globalObj.Set("Array", ObjectValue(rt.arrayPrototype))
}

// GlobalObject returns the JS global object (window/globalThis).
func (rt *Interpreter) GlobalObject() *runtime.JSObject {
	return &rt.globalObj.JSObject
}

// ObjectPrototype returns Object.prototype (the prototype used when
// creating plain objects via NewObject).
func (rt *Interpreter) ObjectPrototype() *runtime.JSObject {
	return rt.objectPrototype
}

// ArrayPrototype returns Array.prototype.
func (rt *Interpreter) ArrayPrototype() *runtime.JSObject {
	return rt.arrayPrototype
}

// FunctionPrototype returns Function.prototype.
func (rt *Interpreter) FunctionPrototype() *runtime.JSObject {
	return rt.functionPrototype
}

// StringPrototype returns String.prototype.
func (rt *Interpreter) StringPrototype() *runtime.JSObject {
	return rt.stringPrototype
}

// NumberPrototype returns Number.prototype.
func (rt *Interpreter) NumberPrototype() *runtime.JSObject {
	return rt.numberPrototype
}

// BooleanPrototype returns Boolean.prototype.
func (rt *Interpreter) BooleanPrototype() *runtime.JSObject {
	return rt.booleanPrototype
}

// VM returns the underlying VM.
func (rt *Interpreter) VM() *runtime.VM { return rt.vm }

// ---------------------------------------------------------------------------
// Call / Run — execute JavaScript code.
// ---------------------------------------------------------------------------

// Call invokes a JS function value with the given this-value and arguments.
func (rt *Interpreter) Call(fn JSValue, thisVal JSValue, args []JSValue) (JSValue, error) {
	if !fn.IsObject() {
		return runtime.JSValueUndefined, fmt.Errorf("jsc: call on non-object")
	}
	obj := fn.GetObject()
	if obj == nil {
		return runtime.JSValueUndefined, fmt.Errorf("jsc: call on nil object")
	}
	fun, ok := any(obj).(*runtime.JSFunction)
	if !ok || fun == nil {
		// Try calling via prototype method.
		callMethod := obj.Get(rt.globalObj, runtime.NewPropertyName("call"))
		if callMethod.IsFunction() {
			return rt.Call(callMethod, runtime.NewJSValueObject(obj), append([]JSValue{thisVal}, args...))
		}
		return runtime.JSValueUndefined, fmt.Errorf("jsc: value is not callable")
	}
	return fun.Call(rt.globalObj, thisVal, args)
}

// Run executes JavaScript source code and returns the result.
func (rt *Interpreter) Run(src string) (JSValue, error) {
	_ = src
	// Skeleton — full parser/bytecode compilation not wired yet.
	// For now, return undefined.
	return runtime.JSValueUndefined, nil
}

// RunScript is an alias for Run.
func (rt *Interpreter) RunScript(src string) (JSValue, error) {
	return rt.Run(src)
}

// Eval evaluates a JavaScript expression.
func (rt *Interpreter) Eval(src string) (JSValue, error) {
	return rt.Run(src)
}

// ---------------------------------------------------------------------------
// Promise helpers (skeleton — used by page/fetcher.go).
// ---------------------------------------------------------------------------

// ResolvePromise creates a resolved promise with the given value.
func (rt *Interpreter) ResolvePromise(val JSValue) JSValue {
	_ = val
	return runtime.JSValueUndefined
}

// RejectPromise creates a rejected promise with the given error value.
func (rt *Interpreter) RejectPromise(err JSValue) JSValue {
	_ = err
	return runtime.JSValueUndefined
}

// ---------------------------------------------------------------------------
// SetupGlobal — installs built-in objects (console, Math, JSON, constructors).
// ---------------------------------------------------------------------------

// SetupGlobal installs the standard built-in objects onto the global scope.
// It must be called before running user scripts that use console/Math/JSON.
func (rt *Interpreter) SetupGlobal(logger Logger) {
	if logger == nil {
		logger = DefaultLogger{}
	}
	g := rt.GlobalObject()
	rt.installConsole(g, logger)
	rt.installMath(g)
	rt.installJSON(g)
	rt.installConstructors(g)
	rt.installGlobals(g)
}

// installConsole sets up the console object.
func (rt *Interpreter) installConsole(g *runtime.JSObject, logger Logger) {
	console := rt.NewObject()
	makeFn := func(name, level string) *JSFunction {
		return NewNativeFunction(name, func(in *Interpreter, thisVal JSValue, args []JSValue) JSValue {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = formatForConsole(a)
			}
			logger.Log(level, strings.Join(parts, " "))
			return runtime.JSValueUndefined
		}, 0)
	}
	console.Set("log", FunctionValue(makeFn("log", "")))
	console.Set("info", FunctionValue(makeFn("info", "info")))
	console.Set("warn", FunctionValue(makeFn("warn", "warn")))
	console.Set("error", FunctionValue(makeFn("error", "error")))
	console.Set("debug", FunctionValue(makeFn("debug", "debug")))
	g.Set("console", ObjectValue(console))
}

// formatForConsole renders a JSValue as a string for console output.
func formatForConsole(v JSValue) string {
	switch {
	case v.IsObject():
		return prettyPrint(v, 0, make(map[uintptr]bool))
	case v.IsString():
		return v.AsString()
	case v.IsNumber():
		return fmt.Sprintf("%v", v.AsNumber())
	case v.IsBoolean():
		return fmt.Sprintf("%v", v.AsBoolean())
	case v.IsUndefined():
		return "undefined"
	case v.IsNull():
		return "null"
	default:
		return v.String()
	}
}

// prettyPrint renders a JS object for console.log with indentation.
func prettyPrint(v JSValue, depth int, seen map[uintptr]bool) string {
	if !v.IsObject() {
		return formatForConsole(v)
	}
	obj := v.AsObject()
	if obj == nil {
		return "null"
	}
	_ = seen
	_ = obj
	_ = depth
	// Cycle detection and property enumeration
	return "[object]"
}

// installMath sets up the Math object.
func (rt *Interpreter) installMath(g *runtime.JSObject) {
	mathObj := rt.NewObject()
	add := func(name string, fn NativeFunc, n int) {
		mathObj.Set(name, FunctionValue(NewNativeFunction(name, fn, n)))
	}
	mathObj.Set("PI", NumberValue(math.Pi))
	mathObj.Set("E", NumberValue(math.E))
	add("floor", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Floor(args[0].ToNumber()))
	}, 1)
	add("ceil", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Ceil(args[0].ToNumber()))
	}, 1)
	add("round", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Round(args[0].ToNumber()))
	}, 1)
	add("abs", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Abs(args[0].ToNumber()))
	}, 1)
	add("sqrt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Sqrt(args[0].ToNumber()))
	}, 1)
	add("pow", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 { return NumberValue(math.NaN()) }
		return NumberValue(math.Pow(args[0].ToNumber(), args[1].ToNumber()))
	}, 2)
	add("min", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.Inf(1)) }
		m := args[0].ToNumber()
		for _, a := range args[1:] {
			n := a.ToNumber()
			if math.IsNaN(n) { return NumberValue(math.NaN()) }
			if n < m { m = n }
		}
		return NumberValue(m)
	}, 2)
	add("max", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.Inf(-1)) }
		m := args[0].ToNumber()
		for _, a := range args[1:] {
			n := a.ToNumber()
			if math.IsNaN(n) { return NumberValue(math.NaN()) }
			if n > m { m = n }
		}
		return NumberValue(m)
	}, 2)
	add("random", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return NumberValue(rand.Float64())
	}, 0)
	add("trunc", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Trunc(args[0].ToNumber()))
	}, 1)
	add("log", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		return NumberValue(math.Log(args[0].ToNumber()))
	}, 1)
	add("max", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.Inf(-1)) }
		m := args[0].ToNumber()
		for _, a := range args[1:] {
			n := a.ToNumber()
			if math.IsNaN(n) { return NumberValue(math.NaN()) }
			if n > m { m = n }
		}
		return NumberValue(m)
	}, 2)
	g.Set("Math", ObjectValue(mathObj))
}

// installJSON sets up the JSON object with parse/stringify.
func (rt *Interpreter) installJSON(g *runtime.JSObject) {
	jsonObj := rt.NewObject()
	jsonObj.Set("parse", FunctionValue(NewNativeFunction("parse", func(in *Interpreter, thisVal JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return runtime.JSValueUndefined }
		var raw interface{}
		if err := json.Unmarshal([]byte(args[0].ToString()), &raw); err != nil {
			return runtime.JSValueUndefined
		}
		return jsonToValue(in, raw)
	}, 1)))
	jsonObj.Set("stringify", FunctionValue(NewNativeFunction("stringify", func(in *Interpreter, thisVal JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return runtime.JSValueUndefined }
		indent := ""
		if len(args) >= 3 {
			if args[2].IsNumber() {
				n := int(args[2].AsNumber())
				if n > 0 { indent = strings.Repeat(" ", n) }
			} else if args[2].IsString() {
				indent = args[2].AsString()
			}
		}
		out := valueToJSON(args[0], indent, "")
		if out == "null" && !args[0].IsNull() && !args[0].IsObject() {
			return runtime.JSValueUndefined
		}
		return StringValue(out)
	}, 3)))
	g.Set("JSON", ObjectValue(jsonObj))
}

// jsonToValue converts a decoded Go interface{} to a JSValue.
func jsonToValue(in *Interpreter, raw interface{}) JSValue {
	switch v := raw.(type) {
	case nil:
		return runtime.JSValueNull
	case bool:
		return runtime.NewJSValueBool(v)
	case float64:
		return runtime.NewJSValueNumber(v)
	case string:
		return runtime.NewJSValueString(v)
	case []interface{}:
		arr := in.NewArray()
		for i, e := range v {
			arr.Set(fmt.Sprintf("%d", i), jsonToValue(in, e))
		}
		return ObjectValue(arr)
	case map[string]interface{}:
		obj := in.NewObject()
		for k, e := range v {
			obj.Set(k, jsonToValue(in, e))
		}
		return ObjectValue(obj)
	default:
		return runtime.JSValueUndefined
	}
}

// valueToJSON converts a JSValue to its JSON string representation.
func valueToJSON(v JSValue, indent, prefix string) string {
	switch {
	case v.IsUndefined():
		return "null"
	case v.IsNull():
		return "null"
	case v.IsBoolean():
		if v.AsBoolean() { return "true" }
		return "false"
	case v.IsNumber():
		n := v.AsNumber()
		if math.IsInf(n, 0) || math.IsNaN(n) { return "null" }
		return fmt.Sprintf("%v", n)
	case v.IsString():
		b, _ := json.Marshal(v.AsString())
		return string(b)
	case v.IsObject():
		obj := v.AsObject()
		if obj == nil { return "null" }
		// Cannot access properties map directly from this package
		// Return simple representation
		return "{}"
	default:
		return "null"
	}
}

// installConstructors sets up built-in constructors on the global object.
func (rt *Interpreter) installConstructors(g *runtime.JSObject) {
	// Skeleton — full constructors from jsc/runtime are available but
	// not yet wired through this facade.
	_ = g
}

// installGlobals sets up global functions (parseInt, parseFloat, isNaN, etc.).
func (rt *Interpreter) installGlobals(g *runtime.JSObject) {
	add := func(name string, fn NativeFunc, n int) {
		g.Set(name, FunctionValue(NewNativeFunction(name, fn, n)))
	}
	add("parseInt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		s := strings.TrimSpace(args[0].ToString())
		if len(args) > 1 && args[1].IsNumber() {
			r := int(args[1].AsNumber())
			if r >= 2 && r <= 36 {
				// Use current radix (simplified parseInt)
				_ = r
			}
		}
		// Basic parseInt
		var val float64
		if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
			return NumberValue(math.NaN())
		}
		return NumberValue(val)
	}, 2)
	add("parseFloat", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return NumberValue(math.NaN()) }
		var val float64
		if _, err := fmt.Sscanf(strings.TrimSpace(args[0].ToString()), "%f", &val); err != nil {
			return NumberValue(math.NaN())
		}
		return NumberValue(val)
	}, 1)
	add("isNaN", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return BooleanValue(true) }
		return BooleanValue(math.IsNaN(args[0].ToNumber()))
	}, 1)
	add("isFinite", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return BooleanValue(false) }
		n := args[0].ToNumber()
		return BooleanValue(!math.IsInf(n, 0) && !math.IsNaN(n))
	}, 1)
	add("encodeURI", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return StringValue("undefined") }
		return StringValue(encodeURI(args[0].ToString()))
	}, 1)
	add("decodeURI", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return StringValue("undefined") }
		// Simplified — real decodeURI is more complex
		return StringValue(args[0].ToString())
	}, 1)
	add("encodeURIComponent", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return StringValue("undefined") }
		return StringValue(encodeURIComponent(args[0].ToString()))
	}, 1)
	add("decodeURIComponent", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return StringValue("undefined") }
		// Simplified
		return StringValue(args[0].ToString())
	}, 1)
	// Date constructor
	dateFn := NewNativeFunction("Date", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(time.Now().Format(time.RFC3339))
	}, 7)
	g.Set("Date", FunctionValue(dateFn))
	g.Set("NaN", NumberValue(math.NaN()))
	g.Set("Infinity", NumberValue(math.Inf(1)))
	g.Set("undefined", runtime.JSValueUndefined)
}

// encodeURI percent-encodes a URI.
func encodeURI(s string) string {
	keep := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~:/?#[]@!$&'()*+,;="
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(keep, r) {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

// encodeURIComponent percent-encodes a URI component.
func encodeURIComponent(s string) string {
	keep := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(keep, r) {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Object / Array / Function constructors.
// ---------------------------------------------------------------------------

// NewObject creates a new plain JSObject with a null prototype.
func (rt *Interpreter) NewObject() *runtime.JSObject {
	return runtime.NewJSObjectWithPrototype(rt.vm, rt.globalObj, ObjectValue(rt.objectPrototype))
}

// NewArray creates a new JSArray (backed by a plain JSObject with length property).
func (rt *Interpreter) NewArray() *runtime.JSObject {
	arr := runtime.NewJSObjectWithPrototype(rt.vm, rt.globalObj, ObjectValue(rt.arrayPrototype))
	arr.Set("length", runtime.NewJSValueNumber(0))
	return arr
}

// NewNativeFunction creates a new JSFunction backed by a Go callback.
// The function is callable from JS and can be passed to Set() as a property value.
func NewNativeFunction(name string, fn NativeFunc, length int) *JSFunction {
	f := runtime.NewJSFunction(nil, nil, name, length, func(globalObject *runtime.JSGlobalObject, thisVal JSValue, args []JSValue) (JSValue, error) {
		// Create a new Interpreter context for this call.
		rt := &Interpreter{
			vm:        globalObject.VM(),
			interp:    nil,
			globalObj: globalObject,
		}
		rt.objectPrototype = &runtime.JSObject{}
		rt.objectPrototype.Set("", runtime.JSValueUndefined)
		return fn(rt, thisVal, args), nil
	})
	return f
}

// ---------------------------------------------------------------------------
// Convenience wrappers for runtime type checks.
// ---------------------------------------------------------------------------

// IsObject checks if v is an object.
func IsObject(v JSValue) bool { return v.IsObject() }

// IsString checks if v is a string.
func IsString(v JSValue) bool { return v.IsString() }

// IsNumber checks if v is a number.
func IsNumber(v JSValue) bool { return v.IsNumber() }

// IsBoolean checks if v is a boolean.
func IsBoolean(v JSValue) bool { return v.IsBoolean() }

// IsUndefined checks if v is undefined.
func IsUndefined(v JSValue) bool { return v.IsUndefined() }

// IsNull checks if v is null.
func IsNull(v JSValue) bool { return v.IsNull() }

// IsFunction checks if v is a function.
func IsFunction(v JSValue) bool { return v.IsFunction() }

// ToString converts v to a string using ToString().
func ToString(v JSValue) string { return v.ToString() }

// ToNumber converts v to a number using ToNumber().
func ToNumber(v JSValue) float64 { return v.ToNumber() }

// ToBoolean converts v to a boolean using ToBoolean().
func ToBoolean(v JSValue) bool { return v.ToBoolean() }

// ---------------------------------------------------------------------------
// Package-level convenience constructors (for backward compatibility).
// ---------------------------------------------------------------------------

// defaultVM is a lazily-initialized VM used by package-level convenience
// functions when no interpreter context is available.
var defaultVM = runtime.NewVM()

// defaultGlobalObject is a lazily-initialized global object.
var defaultGlobalObject = func() *runtime.JSGlobalObject {
	g := runtime.NewJSGlobalObject(defaultVM, nil)
	defaultVM.GlobalObject = g
	return g
}()

// NewObject creates a new plain JSObject based on the given prototype.
func NewObject(prototype *JSObject) *JSObject {
	if prototype == nil {
		return runtime.NewJSObjectWithPrototype(defaultVM, defaultGlobalObject, runtime.JSValueNull)
	}
	obj := runtime.NewJSObjectWithPrototype(defaultVM, defaultGlobalObject, ObjectValue(prototype))
	return obj
}

// NewArray creates a new array with the given prototype and element values.
func NewArray(prototype *JSObject, items []JSValue) *JSObject {
	var arr *runtime.JSObject
	if prototype == nil {
		arr = runtime.NewJSObjectWithPrototype(defaultVM, defaultGlobalObject, runtime.JSValueNull)
	} else {
		arr = runtime.NewJSObjectWithPrototype(defaultVM, defaultGlobalObject, ObjectValue(prototype))
	}
	arr.Set("length", runtime.NewJSValueNumber(float64(len(items))))
	for i, item := range items {
		arr.Set(fmt.Sprintf("%d", i), item)
	}
	return arr
}

// ---------------------------------------------------------------------------
// Extensions to runtime.JSObject — package-level wrapper functions.
// ---------------------------------------------------------------------------

// GetOrZero returns a property value by string key, or JSValueUndefined if
// the property does not exist.
func GetOrZero(obj *JSObject, key string) JSValue {
	return obj.GetStr(key)
}

// Get returns a property value by string key and a boolean indicating whether
// the property exists.
func Get(obj *JSObject, key string) (JSValue, bool) {
	return obj.GetByKey(key)
}
