// Translation of: Source/JavaScriptCore/runtime/JSGlobalObject.h
//                  Source/JavaScriptCore/runtime/JSGlobalObject.cpp
//                  Source/JavaScriptCore/runtime/ConsoleObject.cpp
//                  Source/JavaScriptCore/runtime/MathObject.cpp
//                  Source/JavaScriptCore/runtime/JSONObject.cpp
// Completeness: 50%
// Simplifications:
//   - global object is a single plain JSObject with prototype-linked sub-objects.
//   - console output is routed through a Go Logger interface (default: testing log).
//   - JSON.stringify supports primitives, arrays, plain objects (no replacer/space).
//   - constructors (Array/Object/String) are thin native functions; no Symbol/Map/Set.

package jsc

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// Logger is the Go-side sink for console.log/error/warn/info output. Implementations
// route the formatted message to the host (e.g. a test buffer or os.Stdout).
type Logger interface {
	Log(level string, msg string)
}

// DefaultLogger writes console output to standard output via fmt.Print.
type DefaultLogger struct{}

// Log implements Logger by printing the level-tagged message.
func (DefaultLogger) Log(level string, msg string) {
	if level == "" {
		fmt.Println(msg)
		return
	}
	fmt.Printf("%s: %s\n", level, msg)
}

// BufferLogger records console output in memory for test assertions.
type BufferLogger struct {
	Lines []string
}

// Log appends the formatted line to the buffer.
func (b *BufferLogger) Log(level string, msg string) {
	if level == "" {
		b.Lines = append(b.Lines, msg)
		return
	}
	b.Lines = append(b.Lines, level+": "+msg)
}

// String returns the joined console output.
func (b *BufferLogger) String() string { return strings.Join(b.Lines, "\n") }

// SetupGlobal installs the built-in globals (console, Math, JSON, constructors) onto
// the interpreter's global object. It must be called before running scripts that use
// them. The optional logger receives console output.
func (in *Interpreter) SetupGlobal(logger Logger) {
	if logger == nil {
		logger = DefaultLogger{}
	}
	g := in.global
	in.installConsole(g, logger)
	in.installMath(g)
	in.installJSON(g)
	in.installConstructors(g)
	in.installGlobals(g)
}

// installConsole creates the console object with log/error/warn/info.
func (in *Interpreter) installConsole(g *JSObject, logger Logger) {
	console := NewObject(in.objectProto)
	console.ClassName = "Console"
	makeFn := func(name, level string) *JSFunction {
		return NewNativeFunction(name, func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = formatForConsole(a)
			}
			logger.Log(level, strings.Join(parts, " "))
			return Undefined()
		}, 0)
	}
	console.Set("log", FunctionValue(makeFn("log", "")))
	console.Set("info", FunctionValue(makeFn("info", "info")))
	console.Set("warn", FunctionValue(makeFn("warn", "warn")))
	console.Set("error", FunctionValue(makeFn("error", "error")))
	console.Set("debug", FunctionValue(makeFn("debug", "debug")))
	g.Set("console", ObjectValue(console))
}

// formatForConsole renders a JSValue for console output. Objects are pretty-printed.
func formatForConsole(v JSValue) string {
	switch v.tag {
	case TagObject:
		return prettyPrint(v, 0, make(map[*JSObject]bool))
	case TagFunction:
		return "function " + v.fn.Name + "() { [native code] }"
	default:
		return v.ToString()
	}
}

// prettyPrint renders an object/array as a nested structure (for console.log).
func prettyPrint(v JSValue, depth int, seen map[*JSObject]bool) string {
	if !v.IsObject() {
		return v.ToString()
	}
	o := v.object
	if seen[o] {
		return "[Circular]"
	}
	seen[o] = true
	defer delete(seen, o)
	indent := strings.Repeat("  ", depth+1)
	closing := strings.Repeat("  ", depth)
	if o.IsArray {
		if len(o.Elements) == 0 {
			return "[]"
		}
		parts := make([]string, len(o.Elements))
		for i, e := range o.Elements {
			parts[i] = prettyPrint(e, depth+1, seen)
		}
		return "[\n" + indent + strings.Join(parts, ",\n"+indent) + "\n" + closing + "]"
	}
	keys := make([]string, 0, len(o.Properties))
	for k := range o.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		val := o.Properties[k]
		parts = append(parts, k+": "+prettyPrint(val, depth+1, seen))
	}
	return "{\n" + indent + strings.Join(parts, ",\n"+indent) + "\n" + closing + "}"
}

// installMath creates the Math object with the standard methods.
func (in *Interpreter) installMath(g *JSObject) {
	mathObj := NewObject(in.objectProto)
	mathObj.ClassName = "Math"
	mathObj.Set("PI", NumberValue(math.Pi))
	mathObj.Set("E", NumberValue(math.E))
	add := func(name string, fn NativeFunc, n int) {
		mathObj.Set(name, FunctionValue(NewNativeFunction(name, fn, n)))
	}
	add("floor", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Floor(args[0].ToNumber()))
	}, 1)
	add("ceil", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Ceil(args[0].ToNumber()))
	}, 1)
	add("round", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Round(args[0].ToNumber()))
	}, 1)
	add("abs", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Abs(args[0].ToNumber()))
	}, 1)
	add("sqrt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Sqrt(args[0].ToNumber()))
	}, 1)
	add("pow", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Pow(args[0].ToNumber(), args[1].ToNumber()))
	}, 2)
	add("min", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.Inf(1))
		}
		m := args[0].ToNumber()
		for _, a := range args[1:] {
			n := a.ToNumber()
			if math.IsNaN(n) {
				return NumberValue(math.NaN())
			}
			if n < m {
				m = n
			}
		}
		return NumberValue(m)
	}, 2)
	add("max", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.Inf(-1))
		}
		m := args[0].ToNumber()
		for _, a := range args[1:] {
			n := a.ToNumber()
			if math.IsNaN(n) {
				return NumberValue(math.NaN())
			}
			if n > m {
				m = n
			}
		}
		return NumberValue(m)
	}, 2)
	add("random", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return NumberValue(rand.Float64())
	}, 0)
	add("trunc", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Trunc(args[0].ToNumber()))
	}, 1)
	add("log", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Log(args[0].ToNumber()))
	}, 1)
	g.Set("Math", ObjectValue(mathObj))
}

// installJSON creates the JSON object with parse/stringify.
func (in *Interpreter) installJSON(g *JSObject) {
	jsonObj := NewObject(in.objectProto)
	jsonObj.ClassName = "JSON"
	jsonObj.Set("parse", FunctionValue(NewNativeFunction("parse", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return Undefined()
		}
		var raw interface{}
		if err := json.Unmarshal([]byte(args[0].ToString()), &raw); err != nil {
			return Undefined()
		}
		return jsonToValue(in, raw)
	}, 1)))
	jsonObj.Set("stringify", FunctionValue(NewNativeFunction("stringify", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return Undefined()
		}
		indent := ""
		if len(args) >= 3 {
			if args[2].IsNumber() {
				n := int(args[2].AsNumber())
				if n > 0 {
					indent = strings.Repeat(" ", n)
				}
			} else if args[2].IsString() {
				indent = args[2].AsString()
			}
		}
		out := valueToJSON(args[0], indent, "")
		if out == "null" && !args[0].IsNull() && !args[0].IsObject() && !args[0].IsArray() {
			return Undefined()
		}
		return StringValue(out)
	}, 3)))
	g.Set("JSON", ObjectValue(jsonObj))
}

// jsonToValue converts a decoded Go interface{} into a JSValue.
func jsonToValue(in *Interpreter, raw interface{}) JSValue {
	switch v := raw.(type) {
	case nil:
		return Null()
	case bool:
		return BooleanValue(v)
	case float64:
		return NumberValue(v)
	case string:
		return StringValue(v)
	case []interface{}:
		elems := make([]JSValue, len(v))
		for i, e := range v {
			elems[i] = jsonToValue(in, e)
		}
		return ObjectValue(NewArray(in.arrayProto, elems))
	case map[string]interface{}:
		obj := NewObject(in.objectProto)
		for k, val := range v {
			obj.Set(k, jsonToValue(in, val))
		}
		return ObjectValue(obj)
	}
	return Undefined()
}

// valueToJSON renders a JSValue as a JSON string. indent is the per-level indent (empty
// for compact output); current is the accumulated indent for the current level.
func valueToJSON(v JSValue, indent, current string) string {
	switch v.tag {
	case TagUndefined:
		return "null"
	case TagNull:
		return "null"
	case TagBoolean:
		if v.boolean {
			return "true"
		}
		return "false"
	case TagNumber:
		if math.IsInf(v.number, 0) || math.IsNaN(v.number) {
			return "null"
		}
		return strconv.FormatFloat(v.number, 'g', -1, 64)
	case TagString:
		b, _ := json.Marshal(v.str)
		return string(b)
	case TagSymbol:
		return "null"
	case TagObject:
		o := v.object
		if o.IsArray {
			if len(o.Elements) == 0 {
				return "[]"
			}
			next := current + indent
			parts := make([]string, len(o.Elements))
			for i, e := range o.Elements {
				parts[i] = next + valueToJSON(e, indent, next)
			}
			if indent == "" {
				return "[" + strings.Join(parts, ",") + "]"
			}
			return "[\n" + strings.Join(parts, ",\n") + "\n" + current + "]"
		}
		keys := make([]string, 0, len(o.Properties))
		for k := range o.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return "{}"
		}
		next := current + indent
		sep := ": "
		if indent == "" {
			sep = ":"
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			kb, _ := json.Marshal(k)
			parts = append(parts, next+string(kb)+sep+valueToJSON(o.Properties[k], indent, next))
		}
		if indent == "" {
			return "{" + strings.Join(parts, ",") + "}"
		}
		return "{\n" + strings.Join(parts, ",\n") + "\n" + current + "}"
	case TagFunction:
		return "undefined"
	}
	return "null"
}

// installConstructors registers Array/Object/String/Number/Boolean as callable globals.
func (in *Interpreter) installConstructors(g *JSObject) {
	// Ensure Map and Set prototypes are created first.
	mapProto := in.MapPrototype()
	setProto := in.SetPrototype()

	// Map constructor.
	mapCtor := NewNativeFunction("Map", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		obj := NewObject(mapProto)
		obj.ClassName = "Map"
		obj.Internal = newMapStorage()
		return ObjectValue(obj)
	}, 0)
	mapCtor.properties.Prototype = in.functionProto
	mapCtor.properties.Set("prototype", ObjectValue(mapProto))
	g.Set("Map", FunctionValue(mapCtor))

	// Set constructor.
	setCtor := NewNativeFunction("Set", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		obj := NewObject(setProto)
		obj.ClassName = "Set"
		obj.Internal = newSetStorage()
		return ObjectValue(obj)
	}, 0)
	setCtor.properties.Prototype = in.functionProto
	setCtor.properties.Set("prototype", ObjectValue(setProto))
	g.Set("Set", FunctionValue(setCtor))

	// WeakMap constructor.
	weakMapProto := in.WeakMapPrototype()
	weakMapCtor := NewNativeFunction("WeakMap", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		obj := NewObject(weakMapProto)
		obj.ClassName = "WeakMap"
		obj.Internal = newMapStorage()
		return ObjectValue(obj)
	}, 0)
	weakMapCtor.properties.Prototype = in.functionProto
	weakMapCtor.properties.Set("prototype", ObjectValue(weakMapProto))
	g.Set("WeakMap", FunctionValue(weakMapCtor))

	// WeakSet constructor.
	weakSetProto := in.WeakSetPrototype()
	weakSetCtor := NewNativeFunction("WeakSet", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		obj := NewObject(weakSetProto)
		obj.ClassName = "WeakSet"
		obj.Internal = newSetStorage()
		return ObjectValue(obj)
	}, 0)
	weakSetCtor.properties.Prototype = in.functionProto
	weakSetCtor.properties.Set("prototype", ObjectValue(weakSetProto))
	g.Set("WeakSet", FunctionValue(weakSetCtor))

	// Promise constructor.
	promiseProto := in.PromisePrototype()
	promiseCtor := in.PromiseConstructor()
	promiseCtor.properties.Prototype = in.functionProto
	promiseCtor.properties.Set("prototype", ObjectValue(promiseProto))
	// Static methods.
	promiseCtor.properties.Set("resolve", FunctionValue(in.staticResolve()))
	promiseCtor.properties.Set("reject", FunctionValue(in.staticReject()))
	promiseCtor.properties.Set("all", FunctionValue(in.staticAll()))
	promiseCtor.properties.Set("race", FunctionValue(in.staticRace()))
	g.Set("Promise", FunctionValue(promiseCtor))

	arrayCtor := NewNativeFunction("Array", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		// Array(len) or Array(elem, elem, ...). For the subset, treat a single number
		// argument as length, otherwise collect elements.
		if len(args) == 1 && args[0].IsNumber() {
			n := int(args[0].AsNumber())
			if n < 0 {
				n = 0
			}
			return ObjectValue(NewArray(in.arrayProto, make([]JSValue, n)))
		}
		return ObjectValue(NewArray(in.arrayProto, args))
	}, 1)
	arrayCtor.properties.Prototype = in.functionProto
	arrayCtor.properties.Set("prototype", ObjectValue(in.arrayProto))
	g.Set("Array", FunctionValue(arrayCtor))
	// Array.isArray / Array.from
	arrayCtor.properties.Set("isArray", FunctionValue(NewNativeFunction("isArray",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if len(args) == 0 { return BooleanValue(false) }
			return BooleanValue(args[0].IsArray())
		}, 1)))
	arrayCtor.properties.Set("from", FunctionValue(NewNativeFunction("from",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if len(args) == 0 { return ObjectValue(NewArray(in.arrayProto, nil)) }
			if !args[0].IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			src := args[0].AsObject()
			var elems []JSValue
			if src.IsArray { elems = append(elems, src.Elements...) } else
			if l, ok := src.Properties["length"]; ok && l.IsNumber() {
				for i := 0; i < int(l.AsNumber()); i++ {
					if e, ok := src.Properties[strconv.Itoa(i)]; ok { elems = append(elems, e) }
				}
			}
			return ObjectValue(NewArray(in.arrayProto, elems))
		}, 1)))

	objectCtor := NewNativeFunction("Object", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			return ObjectValue(NewObject(in.objectProto))
		}
		return args[0]
	}, 1)
	objectCtor.properties.Prototype = in.functionProto
	// Set Object.prototype to the shared objectProto
	objectCtor.properties.Set("prototype", ObjectValue(in.objectProto))
	g.Set("Object", FunctionValue(objectCtor))

	stringCtor := NewNativeFunction("String", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return StringValue("")
		}
		return StringValue(args[0].ToString())
	}, 1)
	stringCtor.properties.Prototype = in.functionProto
	stringProto := in.StringPrototype()
	stringCtor.properties.Set("prototype", ObjectValue(stringProto))
	g.Set("String", FunctionValue(stringCtor))

	numberCtor := NewNativeFunction("Number", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(0)
		}
		return NumberValue(args[0].ToNumber())
	}, 1)
	numberCtor.properties.Prototype = in.functionProto
	numProto := NewObject(in.objectProto)
	numProto.ClassName = "Number"
	numProto.Set("toString", FunctionValue(NewNativeFunction("toString",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			n := 0.0
			if this.IsNumber() { n = this.AsNumber() } else if this.IsObject() { n = this.ToNumber() }
			radix := 10
			if len(args) > 0 && args[0].IsNumber() { radix = int(args[0].AsNumber()) }
			if radix == 10 { return StringValue(fmt.Sprintf("%g", n)) }
			return StringValue(strconv.FormatInt(int64(n), radix))
		}, 1)))
	numProto.Set("toFixed", FunctionValue(NewNativeFunction("toFixed",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			n := 0.0
			if this.IsNumber() { n = this.AsNumber() }
			digits := 0
			if len(args) > 0 && args[0].IsNumber() { digits = int(args[0].AsNumber()) }
			return StringValue(strconv.FormatFloat(n, 'f', digits, 64))
		}, 1)))
	numberCtor.properties.Set("prototype", ObjectValue(numProto))
	g.Set("Number", FunctionValue(numberCtor))
	// Number.isNaN / isFinite / isInteger
	numberCtor.properties.Set("isNaN", FunctionValue(NewNativeFunction("isNaN",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if len(args) == 0 { return BooleanValue(true) }
			return BooleanValue(math.IsNaN(args[0].ToNumber()))
		}, 1)))
	numberCtor.properties.Set("isFinite", FunctionValue(NewNativeFunction("isFinite",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if len(args) == 0 { return BooleanValue(false) }
			n := args[0].ToNumber()
			return BooleanValue(!math.IsNaN(n) && !math.IsInf(n, 0))
		}, 1)))
	numberCtor.properties.Set("isInteger", FunctionValue(NewNativeFunction("isInteger",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if len(args) == 0 { return BooleanValue(false) }
			n := args[0].ToNumber()
			return BooleanValue(!math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n))
		}, 1)))

	booleanCtor := NewNativeFunction("Boolean", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(args[0].ToBoolean())
	}, 1)
	booleanCtor.properties.Prototype = in.functionProto
	boolProto := NewObject(in.objectProto)
	boolProto.ClassName = "Boolean"
	boolProto.Set("toString", FunctionValue(NewNativeFunction("toString",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			b := false
			if this.IsBoolean() { b = this.AsBoolean() }
			return StringValue(fmt.Sprintf("%t", b))
		}, 0)))
	booleanCtor.properties.Set("prototype", ObjectValue(boolProto))
	g.Set("Boolean", FunctionValue(booleanCtor))

	// Object.keys / Object.values (static helpers).
	objStatic := NewObject(in.objectProto)
	objStatic.Set("keys", FunctionValue(NewNativeFunction("keys", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || !args[0].IsObject() {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		o := args[0].AsObject()
		// Proxy support: call [[OwnPropertyKeys]] via handler's ownKeys trap
		if IsProxy(args[0]) {
			pkeys := proxyOwnKeys(in, args[0])
			if pkeys == nil {
				return ObjectValue(NewArray(in.arrayProto, nil))
			}
			keys := make([]JSValue, len(pkeys))
			for i, k := range pkeys {
				keys[i] = StringValue(k)
			}
			return ObjectValue(NewArray(in.arrayProto, keys))
		}
		var keys []JSValue
		if o.IsArray {
			for i := range o.Elements {
				keys = append(keys, StringValue(strconv.Itoa(i)))
			}
		} else {
			for k := range o.Properties {
				keys = append(keys, StringValue(k))
			}
		}
		return ObjectValue(NewArray(in.arrayProto, keys))
	}, 1)))
	objStatic.Set("values", FunctionValue(NewNativeFunction("values", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || !args[0].IsObject() {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		o := args[0].AsObject()
		var vals []JSValue
		if o.IsArray {
			vals = append(vals, o.Elements...)
		} else {
			for _, v := range o.Properties {
				vals = append(vals, v)
			}
		}
		return ObjectValue(NewArray(in.arrayProto, vals))
	}, 1)))
	objStatic.Set("entries", FunctionValue(NewNativeFunction("entries", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || !args[0].IsObject() {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		o := args[0].AsObject()
		var pairs []JSValue
		if o.IsArray {
			for i, e := range o.Elements {
				pairs = append(pairs, ObjectValue(NewArray(in.arrayProto, []JSValue{StringValue(strconv.Itoa(i)), e})))
			}
		} else {
			keys := make([]string, 0, len(o.Properties))
			for k := range o.Properties {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				pairs = append(pairs, ObjectValue(NewArray(in.arrayProto, []JSValue{StringValue(k), o.Properties[k]})))
			}
		}
		return ObjectValue(NewArray(in.arrayProto, pairs))
	}, 1)))
	objStatic.Set("assign", FunctionValue(NewNativeFunction("assign", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 { return ObjectValue(NewObject(in.objectProto)) }
		target := args[0]
		if !target.IsObject() { return target }
		targetObj := target.AsObject()
		for i := 1; i < len(args); i++ {
			if !args[i].IsObject() { continue }
			for k, v := range args[i].AsObject().Properties {
				targetObj.Set(k, v)
			}
		}
		return target
	}, 2)))
	objStatic.Set("create", FunctionValue(NewNativeFunction("create", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		proto := in.objectProto
		if len(args) > 0 && args[0].IsObject() {
			proto = args[0].AsObject()
		}
		return ObjectValue(&JSObject{Properties: make(map[string]JSValue), Prototype: proto, ClassName: "Object"})
	}, 1)))
	objStatic.Set("defineProperty", FunctionValue(NewNativeFunction("defineProperty", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 || !args[0].IsObject() { return args[0] }
		// Proxy support: call defineProperty trap
		if IsProxy(args[0]) {
			proxyDefineProperty(in, args[0], args[1].ToString(), args[2])
			return args[0]
		}
		obj := args[0].AsObject()
		key := args[1].ToString()
		if len(args) > 2 && args[2].IsObject() {
			desc := args[2].AsObject()
			if v, ok := desc.Properties["value"]; ok {
				obj.Properties[key] = v
			}
			if fn, ok := desc.Properties["get"]; ok && fn.IsFunction() {
				getter := fn
				obj.SetAccessor(key, func(_ *Interpreter, _ JSValue) JSValue {
					r, _ := in.Call(getter, ObjectValue(obj))
					return r
				}, nil)
			}
			if fn, ok := desc.Properties["set"]; ok && fn.IsFunction() {
				setter := fn
				obj.SetAccessor(key, nil, func(_ *Interpreter, _ JSValue, v JSValue) {
					in.Call(setter, ObjectValue(obj), v)
				})
			}
		}
		return args[0]
	}, 3)))
	objStatic.Set("freeze", FunctionValue(NewNativeFunction("freeze", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) > 0 { return args[0] }
		return Undefined()
	}, 1)))
	objStatic.Set("getOwnPropertyDescriptor", FunctionValue(NewNativeFunction("getOwnPropertyDescriptor", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) < 2 || !args[0].IsObject() { return Undefined() }
		// Proxy support: call getOwnPropertyDescriptor trap
		if IsProxy(args[0]) {
			res, ok := proxyGetOwnPropertyDescriptor(in, args[0], args[1].ToString())
			if ok {
				return res
			}
			return Undefined()
		}
		obj := args[0].AsObject()
		key := args[1].ToString()
		if v, ok := obj.Properties[key]; ok {
			desc := NewObject(in.objectProto)
			desc.Set("value", v)
			desc.Set("writable", BooleanValue(true))
			desc.Set("enumerable", BooleanValue(true))
			desc.Set("configurable", BooleanValue(true))
			return ObjectValue(desc)
		}
		return Undefined()
	}, 2)))
	g.Set("Object_static", ObjectValue(objStatic))
	// Override bare 'Object' reference resolution: Object.keys etc. attach to the ctor.
	objectCtor.properties.Set("keys", objStatic.GetOrZero("keys"))
	objectCtor.properties.Set("values", objStatic.GetOrZero("values"))
	objectCtor.properties.Set("entries", objStatic.GetOrZero("entries"))
	objectCtor.properties.Set("assign", objStatic.GetOrZero("assign"))
	objectCtor.properties.Set("create", objStatic.GetOrZero("create"))
	objectCtor.properties.Set("defineProperty", objStatic.GetOrZero("defineProperty"))
	objectCtor.properties.Set("freeze", objStatic.GetOrZero("freeze"))
	objectCtor.properties.Set("getOwnPropertyDescriptor", objStatic.GetOrZero("getOwnPropertyDescriptor"))

	// Symbol constructor.
	symCtor := in.SymbolConstructor()
	g.Set("Symbol", FunctionValue(symCtor))

	// Proxy constructor.
	g.Set("Proxy", FunctionValue(in.ProxyConstructor()))

	// Reflect object.
	g.Set("Reflect", ObjectValue(in.ReflectObject()))

	// ─── Error constructors ────────────────────────────
	makeError := func(name string) *JSFunction {
		return NewNativeFunction(name, func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			msg := ""
			if len(args) > 0 { msg = args[0].ToString() }
			err := NewObject(in.objectProto)
			err.Set("name", StringValue(name))
			err.Set("message", StringValue(msg))
			return ObjectValue(err)
		}, 1)
	}
	errorCtor := makeError("Error")
	errorCtor.properties.Set("prototype", ObjectValue(NewObject(in.objectProto)))
	g.Set("Error", FunctionValue(errorCtor))
	g.Set("TypeError", FunctionValue(makeError("TypeError")))
	g.Set("RangeError", FunctionValue(makeError("RangeError")))
	g.Set("SyntaxError", FunctionValue(makeError("SyntaxError")))
	g.Set("ReferenceError", FunctionValue(makeError("ReferenceError")))

	// ─── RegExp constructor ────────────────────────────
	regCtor := NewNativeFunction("RegExp", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		pat := ""
		flags := ""
		if len(args) > 0 { pat = args[0].ToString() }
		if len(args) > 1 { flags = args[1].ToString() }
		re := NewObject(in.objectProto)
		re.Set("source", StringValue(pat))
		re.Set("flags", StringValue(flags))
		re.Set("lastIndex", NumberValue(0))
		re.ClassName = "RegExp"
		return ObjectValue(re)
	}, 2)
	regCtor.properties.Set("prototype", ObjectValue(NewObject(in.objectProto)))
	g.Set("RegExp", FunctionValue(regCtor))

	// ─── Date constructor ──────────────────────────────
	dateCtor := NewNativeFunction("Date", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		d := NewObject(in.objectProto)
		d.ClassName = "Date"
		if len(args) == 1 && args[0].IsNumber() {
			d.Set("value", NumberValue(args[0].AsNumber()))
		} else if len(args) > 0 {
			// Simplified: parse first string arg only
			d.Set("value", StringValue(args[0].ToString()))
		} else {
			d.Set("value", StringValue("(date)"))
		}
		return ObjectValue(d)
	}, 7)
	dateCtor.properties.Set("prototype", ObjectValue(NewObject(in.objectProto)))
	g.Set("Date", FunctionValue(dateCtor))

	// ─── Function.prototype methods ────────────────────
	fnProto := in.functionProto
	fnProto.Set("call", FunctionValue(NewNativeFunction("call",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsFunction() { return Undefined() }
			thisArg := Undefined()
			if len(args) > 0 { thisArg = args[0] }
			callArgs := args[1:]
			r, _ := in.Call(this, thisArg, callArgs...)
			return r
		}, 1)))
	fnProto.Set("apply", FunctionValue(NewNativeFunction("apply",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsFunction() { return Undefined() }
			thisArg := Undefined()
			if len(args) > 0 { thisArg = args[0] }
			var callArgs []JSValue
			if len(args) > 1 && args[1].IsObject() && args[1].AsObject().IsArray {
				callArgs = args[1].AsObject().Elements
			}
			r, _ := in.Call(this, thisArg, callArgs...)
			return r
		}, 2)))
	fnProto.Set("bind", FunctionValue(NewNativeFunction("bind",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsFunction() { return Undefined() }
			fn := this
			thisArg := Undefined()
			if len(args) > 0 { thisArg = args[0] }
			boundArgs := args[1:]
			return FunctionValue(NewNativeFunction("bound", func(in *Interpreter, _ JSValue, callArgs []JSValue) JSValue {
				fullArgs := append(boundArgs, callArgs...)
				r, _ := in.Call(fn, thisArg, fullArgs...)
				return r
			}, 0))
		}, 1)))

	// ─── Array.prototype methods ────────────────────────
	arrProto := in.arrayProto
	arrProto.Set("toString", FunctionValue(NewNativeFunction("toString",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return StringValue("") }
			arr := this.AsObject()
			if !arr.IsArray { return StringValue("") }
			var parts []string
			for _, e := range arr.Elements {
				if e.IsUndefined() || e.IsNull() { parts = append(parts, "") } else
				if e.IsString() { parts = append(parts, e.AsString()) } else
				if e.IsNumber() { parts = append(parts, fmt.Sprintf("%g", e.AsNumber())) } else
				if e.IsBoolean() { parts = append(parts, fmt.Sprintf("%t", e.AsBoolean())) } else
				{ parts = append(parts, e.ToString()) }
			}
			return StringValue(strings.Join(parts, ","))
		}, 0)))
	arrProto.Set("push", FunctionValue(NewNativeFunction("push",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return NumberValue(0) }
			arr := this.AsObject()
			arr.Elements = append(arr.Elements, args...)
			return NumberValue(float64(len(arr.Elements)))
		}, 1)))
	arrProto.Set("pop", FunctionValue(NewNativeFunction("pop",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return Undefined() }
			arr := this.AsObject()
			if len(arr.Elements) == 0 { return Undefined() }
			last := arr.Elements[len(arr.Elements)-1]
			arr.Elements = arr.Elements[:len(arr.Elements)-1]
			return last
		}, 0)))
	arrProto.Set("indexOf", FunctionValue(NewNativeFunction("indexOf",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return NumberValue(-1) }
			arr := this.AsObject()
			if !arr.IsArray { return NumberValue(-1) }
			if len(args) == 0 { return NumberValue(-1) }
			fromIdx := 0
			if len(args) > 1 && args[1].IsNumber() { fromIdx = int(args[1].AsNumber()) }
			for i := fromIdx; i < len(arr.Elements); i++ {
				if arr.Elements[i].StrictEquals(args[0]) { return NumberValue(float64(i)) }
			}
			return NumberValue(-1)
		}, 1)))
	arrProto.Set("forEach", FunctionValue(NewNativeFunction("forEach",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return Undefined() }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return Undefined() }
			fn := args[0]
			for i, e := range arr.Elements {
				in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
			}
			return Undefined()
		}, 1)))
	arrProto.Set("map", FunctionValue(NewNativeFunction("map",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() {
				return ObjectValue(NewArray(in.arrayProto, nil))
			}
			fn := args[0]
			var result []JSValue
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				result = append(result, r)
			}
			return ObjectValue(NewArray(in.arrayProto, result))
		}, 1)))
	arrProto.Set("filter", FunctionValue(NewNativeFunction("filter",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() {
				return ObjectValue(NewArray(in.arrayProto, nil))
			}
			fn := args[0]
			var result []JSValue
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				if r.IsBoolean() && r.AsBoolean() { result = append(result, e) }
			}
			return ObjectValue(NewArray(in.arrayProto, result))
		}, 1)))
	arrProto.Set("some", FunctionValue(NewNativeFunction("some",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return BooleanValue(false) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return BooleanValue(false) }
			fn := args[0]
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				if r.IsBoolean() && r.AsBoolean() { return BooleanValue(true) }
			}
			return BooleanValue(false)
		}, 1)))
	arrProto.Set("every", FunctionValue(NewNativeFunction("every",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return BooleanValue(false) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return BooleanValue(false) }
			fn := args[0]
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				if !r.IsBoolean() || !r.AsBoolean() { return BooleanValue(false) }
			}
			return BooleanValue(true)
		}, 1)))
	arrProto.Set("includes", FunctionValue(NewNativeFunction("includes",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return BooleanValue(false) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 { return BooleanValue(false) }
			for _, e := range arr.Elements {
				if e.StrictEquals(args[0]) { return BooleanValue(true) }
			}
			return BooleanValue(false)
		}, 1)))
	arrProto.Set("find", FunctionValue(NewNativeFunction("find",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return Undefined() }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return Undefined() }
			fn := args[0]
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				if r.IsBoolean() && r.AsBoolean() { return e }
			}
			return Undefined()
		}, 1)))
	arrProto.Set("reduce", FunctionValue(NewNativeFunction("reduce",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return Undefined() }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return Undefined() }
			fn := args[0]
			start := 0
			acc := Undefined()
			hasInit := false
			if len(args) > 1 { acc = args[1]; hasInit = true }
			if !hasInit {
				if len(arr.Elements) == 0 { return Undefined() }
				acc = arr.Elements[0]
				start = 1
			}
			for i := start; i < len(arr.Elements); i++ {
				r, _ := in.Call(fn, Undefined(), acc, arr.Elements[i], NumberValue(float64(i)), ObjectValue(arr))
				acc = r
			}
			return acc
		}, 1)))
	arrProto.Set("concat", FunctionValue(NewNativeFunction("concat",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			var result []JSValue
			result = append(result, arr.Elements...)
			for _, a := range args {
				if a.IsArray() {
					result = append(result, a.AsObject().Elements...)
				} else {
					result = append(result, a)
				}
			}
			return ObjectValue(NewArray(in.arrayProto, result))
		}, 1)))
	arrProto.Set("slice", FunctionValue(NewNativeFunction("slice",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			if !arr.IsArray { return ObjectValue(NewArray(in.arrayProto, nil)) }
			start, end := 0, len(arr.Elements)
			if len(args) > 0 && args[0].IsNumber() { start = int(args[0].AsNumber()) }
			if len(args) > 1 && args[1].IsNumber() { end = int(args[1].AsNumber()) }
			if start < 0 { start = len(arr.Elements) + start; if start < 0 { start = 0 } }
			if end < 0 { end = len(arr.Elements) + end }
			if start > len(arr.Elements) { start = len(arr.Elements) }
			if end > len(arr.Elements) { end = len(arr.Elements) }
			if start > end { start = end }
			var result []JSValue
			result = append(result, arr.Elements[start:end]...)
			return ObjectValue(NewArray(in.arrayProto, result))
		}, 2)))
	arrProto.Set("splice", FunctionValue(NewNativeFunction("splice",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			if !arr.IsArray { return ObjectValue(NewArray(in.arrayProto, nil)) }
			start := 0
			if len(args) > 0 && args[0].IsNumber() { start = int(args[0].AsNumber()) }
			deleteCount := len(arr.Elements) - start
			if len(args) > 1 && args[1].IsNumber() { deleteCount = int(args[1].AsNumber()) }
			if start < 0 { start = len(arr.Elements) + start; if start < 0 { start = 0 } }
			if deleteCount < 0 { deleteCount = 0 }
			if start > len(arr.Elements) { start = len(arr.Elements) }
			if deleteCount > len(arr.Elements)-start { deleteCount = len(arr.Elements) - start }
			var removed []JSValue
			removed = append(removed, arr.Elements[start:start+deleteCount]...)
			var newElems []JSValue
			newElems = append(newElems, arr.Elements[:start]...)
			if len(args) > 2 { newElems = append(newElems, args[2:]...) }
			newElems = append(newElems, arr.Elements[start+deleteCount:]...)
			arr.Elements = newElems
			return ObjectValue(NewArray(in.arrayProto, removed))
		}, 2)))
	// ─── Additional Array.prototype methods ─────────────
	arrProto.Set("findIndex", FunctionValue(NewNativeFunction("findIndex",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return NumberValue(-1) }
			arr := this.AsObject()
			if !arr.IsArray || len(args) == 0 || !args[0].IsFunction() { return NumberValue(-1) }
			fn := args[0]
			for i, e := range arr.Elements {
				r, _ := in.Call(fn, Undefined(), e, NumberValue(float64(i)), ObjectValue(arr))
				if r.IsBoolean() && r.AsBoolean() { return NumberValue(float64(i)) }
			}
			return NumberValue(-1)
		}, 1)))
	arrProto.Set("fill", FunctionValue(NewNativeFunction("fill",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return this }
			arr := this.AsObject()
			if !arr.IsArray { return this }
			val := Undefined()
			if len(args) > 0 { val = args[0] }
			start := 0
			if len(args) > 1 && args[1].IsNumber() { start = int(args[1].AsNumber()) }
			end := len(arr.Elements)
			if len(args) > 2 && args[2].IsNumber() { end = int(args[2].AsNumber()) }
			if start < 0 { start = len(arr.Elements) + start; if start < 0 { start = 0 } }
			if end < 0 { end = len(arr.Elements) + end }
			if start > len(arr.Elements) { start = len(arr.Elements) }
			if end > len(arr.Elements) { end = len(arr.Elements) }
			for i := start; i < end; i++ { arr.Elements[i] = val }
			return this
		}, 1)))
	arrProto.Set("flat", FunctionValue(NewNativeFunction("flat",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return ObjectValue(NewArray(in.arrayProto, nil)) }
			arr := this.AsObject()
			if !arr.IsArray { return ObjectValue(NewArray(in.arrayProto, nil)) }
			depth := 1
			if len(args) > 0 && args[0].IsNumber() { depth = int(args[0].AsNumber()) }
			var flatten func(elems []JSValue, d int) []JSValue
			flatten = func(elems []JSValue, d int) []JSValue {
				var result []JSValue
				for _, e := range elems {
					if d > 0 && e.IsObject() && e.AsObject() != nil && e.AsObject().IsArray {
						result = append(result, flatten(e.AsObject().Elements, d-1)...)
					} else {
						result = append(result, e)
					}
				}
				return result
			}
			return ObjectValue(NewArray(in.arrayProto, flatten(arr.Elements, depth)))
		}, 1)))
	arrProto.Set("sort", FunctionValue(NewNativeFunction("sort",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return this }
			arr := this.AsObject()
			if !arr.IsArray { return this }
			// Simple insertion sort (stable, works for any comparator)
			for i := 1; i < len(arr.Elements); i++ {
				for j := i; j > 0; j-- {
					a, b := arr.Elements[j-1], arr.Elements[j]
					swap := false
					if len(args) > 0 && args[0].IsFunction() {
						r, _ := in.Call(args[0], Undefined(), a, b)
						if r.IsNumber() && r.AsNumber() > 0 { swap = true }
					} else {
						sa, sb := a.ToString(), b.ToString()
						if sa > sb { swap = true }
					}
					if swap {
						arr.Elements[j-1], arr.Elements[j] = b, a
					}
				}
			}
			return this
		}, 1)))
	arrProto.Set("reverse", FunctionValue(NewNativeFunction("reverse",
		func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() { return this }
			arr := this.AsObject()
			if !arr.IsArray { return this }
			for i, j := 0, len(arr.Elements)-1; i < j; i, j = i+1, j-1 {
				arr.Elements[i], arr.Elements[j] = arr.Elements[j], arr.Elements[i]
			}
			return this
		}, 0)))
}

// GetOrZero returns the property or undefined.
func (o *JSObject) GetOrZero(key string) JSValue {
	if v, ok := o.Get(key); ok {
		return v
	}
	return Undefined()
}

// installGlobals sets up primitive global constants and helpers.
func (in *Interpreter) installGlobals(g *JSObject) {
	g.Set("undefined", Undefined())
	g.Set("NaN", NumberValue(math.NaN()))
	g.Set("Infinity", NumberValue(math.Inf(1)))
	// parseInt / parseFloat
	g.Set("parseInt", FunctionValue(NewNativeFunction("parseInt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		s := strings.TrimSpace(args[0].ToString())
		radix := 10
		if len(args) > 1 && !args[1].IsUndefined() {
			radix = int(args[1].ToInt32())
		}
		if radix == 0 {
			radix = 10
		}
		// Strip sign and detect hex prefix.
		neg := false
		if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
			neg = s[0] == '-'
			s = s[1:]
		}
		if radix == 16 && strings.HasPrefix(strings.ToLower(s), "0x") {
			s = s[2:]
		}
		// Consume valid digits for the radix.
		i := 0
		for i < len(s) {
			c := s[i]
			digit := -1
			switch {
			case c >= '0' && c <= '9':
				digit = int(c - '0')
			case c >= 'a' && c <= 'z':
				digit = int(c-'a') + 10
			case c >= 'A' && c <= 'Z':
				digit = int(c-'A') + 10
			}
			if digit < 0 || digit >= radix {
				break
			}
			i++
		}
		if i == 0 {
			return NumberValue(math.NaN())
		}
		n, err := strconv.ParseInt(s[:i], radix, 64)
		if err != nil {
			return NumberValue(math.NaN())
		}
		if neg {
			n = -n
		}
		return NumberValue(float64(n))
	}, 2)))
	g.Set("parseFloat", FunctionValue(NewNativeFunction("parseFloat", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(math.NaN())
		}
		s := strings.TrimSpace(args[0].ToString())
		// Find the longest numeric prefix.
		end := 0
		seenDot := false
		seenE := false
		if end < len(s) && (s[end] == '+' || s[end] == '-') {
			end++
		}
		for end < len(s) {
			c := s[end]
			if c >= '0' && c <= '9' {
				end++
				continue
			}
			if c == '.' && !seenDot && !seenE {
				seenDot = true
				end++
				continue
			}
			if (c == 'e' || c == 'E') && !seenE {
				seenE = true
				end++
				if end < len(s) && (s[end] == '+' || s[end] == '-') {
					end++
				}
				continue
			}
			break
		}
		if end == 0 {
			return NumberValue(math.NaN())
		}
		n, err := strconv.ParseFloat(s[:end], 64)
		if err != nil {
			return NumberValue(math.NaN())
		}
		return NumberValue(n)
	}, 1)))
	// isNaN / isFinite
	g.Set("isNaN", FunctionValue(NewNativeFunction("isNaN", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return BooleanValue(true)
		}
		return BooleanValue(math.IsNaN(args[0].ToNumber()))
	}, 1)))
	g.Set("isFinite", FunctionValue(NewNativeFunction("isFinite", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return BooleanValue(false)
		}
		n := args[0].ToNumber()
		return BooleanValue(!math.IsNaN(n) && !math.IsInf(n, 0))
	}, 1)))
}

// Eval parses and runs source in the global environment, returning the result.
func (in *Interpreter) Eval(src string) (JSValue, error) {
	return in.Run(src)
}
