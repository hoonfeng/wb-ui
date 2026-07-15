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
	g.Set("Array", FunctionValue(arrayCtor))

	objectCtor := NewNativeFunction("Object", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			return ObjectValue(NewObject(in.objectProto))
		}
		return args[0]
	}, 1)
	objectCtor.properties.Prototype = in.functionProto
	g.Set("Object", FunctionValue(objectCtor))

	stringCtor := NewNativeFunction("String", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return StringValue("")
		}
		return StringValue(args[0].ToString())
	}, 1)
	stringCtor.properties.Prototype = in.functionProto
	g.Set("String", FunctionValue(stringCtor))

	numberCtor := NewNativeFunction("Number", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return NumberValue(0)
		}
		return NumberValue(args[0].ToNumber())
	}, 1)
	numberCtor.properties.Prototype = in.functionProto
	g.Set("Number", FunctionValue(numberCtor))

	booleanCtor := NewNativeFunction("Boolean", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(args[0].ToBoolean())
	}, 1)
	booleanCtor.properties.Prototype = in.functionProto
	g.Set("Boolean", FunctionValue(booleanCtor))

	// Object.keys / Object.values (static helpers).
	objStatic := NewObject(in.objectProto)
	objStatic.Set("keys", FunctionValue(NewNativeFunction("keys", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if len(args) == 0 || !args[0].IsObject() {
			return ObjectValue(NewArray(in.arrayProto, nil))
		}
		o := args[0].AsObject()
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
	g.Set("Object_static", ObjectValue(objStatic))
	// Override bare 'Object' reference resolution: Object.keys etc. attach to the ctor.
	objectCtor.properties.Set("keys", objStatic.GetOrZero("keys"))
	objectCtor.properties.Set("values", objStatic.GetOrZero("values"))
	objectCtor.properties.Set("entries", objStatic.GetOrZero("entries"))
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
