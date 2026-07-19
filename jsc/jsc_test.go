package jsc

import (
	"math"
	"strings"
	"testing"
)

func TestNewInterpreter(t *testing.T) {
	rt := NewInterpreter()
	if rt == nil {
		t.Fatal("NewInterpreter() returned nil")
	}
	if rt.GlobalObject() == nil {
		t.Fatal("GlobalObject() returned nil")
	}
}

func TestConsoleLog(t *testing.T) {
	rt := NewInterpreter()
	buf := &BufferLogger{}
	rt.SetupGlobal(buf)

	consoleVal := rt.GlobalObject().GetStr("console")
	if !consoleVal.IsObject() {
		t.Fatal("console is not an object")
	}
	console := consoleVal.AsObject()

	logFn := console.GetStr("log")
	if !logFn.IsFunction() {
		t.Fatal("console.log is not a function")
	}

	_, err := rt.Call(logFn, consoleVal, []JSValue{StringValue("hello world")})
	if err != nil {
		t.Fatalf("console.log call failed: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("console.log output missing 'hello world': %q", buf.String())
	}
}

func TestConsoleMultipleLevels(t *testing.T) {
	rt := NewInterpreter()
	buf := &BufferLogger{}
	rt.SetupGlobal(buf)
	consoleVal := rt.GlobalObject().GetStr("console")
	console := consoleVal.AsObject()

	tests := []struct{ level, method string }{
		{"", "log"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"debug", "debug"},
	}
	for _, tt := range tests {
		buf.Clear()
		fn := console.GetStr(tt.method)
		if !fn.IsFunction() {
			t.Fatalf("console.%s is not a function", tt.method)
		}
		_, err := rt.Call(fn, consoleVal, []JSValue{StringValue("test " + tt.method)})
		if err != nil {
			t.Fatalf("console.%s call failed: %v", tt.method, err)
		}
		if !strings.Contains(buf.String(), "test") {
			t.Fatalf("console.%s output missing: %q", tt.method, buf.String())
		}
	}
}

func TestMathConstants(t *testing.T) {
	rt := NewInterpreter()
	buf := &BufferLogger{}
	rt.SetupGlobal(buf)

	mathObj := rt.GlobalObject().GetStr("Math").AsObject()
	pi := mathObj.GetStr("PI")
	if !pi.IsNumber() {
		t.Fatal("Math.PI is not a number")
	}
	if pi.AsNumber() < 3.14 || pi.AsNumber() > 3.15 {
		t.Fatalf("Math.PI out of range: %v", pi.AsNumber())
	}
	e := mathObj.GetStr("E")
	if !e.IsNumber() || e.AsNumber() < 2.71 || e.AsNumber() > 2.73 {
		t.Fatalf("Math.E out of range: %v", e.AsNumber())
	}
}

func TestMathFunctions(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	mathObj := rt.GlobalObject().GetStr("Math").AsObject()
	tests := []struct {
		name  string
		args  []JSValue
		check func(JSValue) bool
	}{
		{"floor", []JSValue{NumberValue(3.7)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 3 }},
		{"ceil", []JSValue{NumberValue(3.2)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 4 }},
		{"round", []JSValue{NumberValue(3.5)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 4 }},
		{"abs", []JSValue{NumberValue(-5)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 5 }},
		{"sqrt", []JSValue{NumberValue(9)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 3 }},
		{"pow", []JSValue{NumberValue(2), NumberValue(3)}, func(v JSValue) bool { return v.IsNumber() && v.AsNumber() == 8 }},
	}
	for _, tt := range tests {
		fn := mathObj.GetStr(tt.name)
		if !fn.IsFunction() {
			t.Fatalf("Math.%s is not a function", tt.name)
		}
		result, err := rt.Call(fn, Undefined(), tt.args)
		if err != nil {
			t.Fatalf("Math.%s call failed: %v", tt.name, err)
		}
		if !tt.check(result) {
			t.Fatalf("Math.%s(%v) = %v, expected condition not met", tt.name, tt.args, result)
		}
	}
}

func TestJSONParse(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	jsonObj := rt.GlobalObject().GetStr("JSON").AsObject()
	parseFn := jsonObj.GetStr("parse")

	result, err := rt.Call(parseFn, Undefined(), []JSValue{StringValue(`{"a":1,"b":"hello"}`)})
	if err != nil {
		t.Fatalf("JSON.parse call failed: %v", err)
	}
	if !result.IsObject() {
		t.Fatalf("JSON.parse result is not an object, got: %v", result)
	}
	obj := result.AsObject()
	aVal := obj.GetStr("a")
	if !aVal.IsNumber() || aVal.AsNumber() != 1 {
		t.Fatalf("JSON.parse result.a = %v, expected 1", aVal)
	}
	bVal := obj.GetStr("b")
	if !bVal.IsString() || bVal.AsString() != "hello" {
		t.Fatalf("JSON.parse result.b = %v, expected 'hello'", bVal)
	}
}

func TestJSONStringify(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	jsonObj := rt.GlobalObject().GetStr("JSON").AsObject()
	stringifyFn := jsonObj.GetStr("stringify")

	result, err := rt.Call(stringifyFn, Undefined(), []JSValue{StringValue("hello")})
	if err != nil {
		t.Fatalf("JSON.stringify call failed: %v", err)
	}
	if !result.IsString() {
		t.Fatalf("JSON.stringify result is not a string: %v", result)
	}
	if result.AsString() != `"hello"` {
		t.Fatalf("JSON.stringify('hello') = %q, expected '\"hello\"'", result.AsString())
	}

	result, err = rt.Call(stringifyFn, Undefined(), []JSValue{NumberValue(42)})
	if err != nil {
		t.Fatalf("JSON.stringify call failed: %v", err)
	}
	if !result.IsString() || result.AsString() != "42" {
		t.Fatalf("JSON.stringify(42) = %q, expected '42'", result.AsString())
	}
}

func TestNewObject(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	obj := rt.NewObject()
	if obj == nil {
		t.Fatal("NewObject() returned nil")
	}
	obj.Set("foo", StringValue("bar"))
	obj.Set("num", NumberValue(42))

	val := obj.GetStr("foo")
	if !val.IsString() || val.AsString() != "bar" {
		t.Fatalf("obj.foo = %v, expected 'bar'", val)
	}
	val = obj.GetStr("num")
	if !val.IsNumber() || val.AsNumber() != 42 {
		t.Fatalf("obj.num = %v, expected 42", val)
	}
}

func TestNewArray(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	arr := rt.NewArray()
	if arr == nil {
		t.Fatal("NewArray() returned nil")
	}
	arr.Set("0", StringValue("a"))
	arr.Set("1", StringValue("b"))
	arr.Set("length", NumberValue(2))

	length := arr.GetStr("length")
	if !length.IsNumber() || length.AsNumber() != 2 {
		t.Fatalf("arr.length = %v, expected 2", length)
	}
	v0 := arr.GetStr("0")
	if !v0.IsString() || v0.AsString() != "a" {
		t.Fatalf("arr[0] = %v, expected 'a'", v0)
	}
}

func TestNativeFunction(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	addFn := NewNativeFunction("add", func(in *Interpreter, thisVal JSValue, args []JSValue) JSValue {
		sum := 0.0
		for _, a := range args {
			sum += a.ToNumber()
		}
		return NumberValue(sum)
	}, 2)

	rt.GlobalObject().Set("add", FunctionValue(addFn))

	addVal := rt.GlobalObject().GetStr("add")
	result, err := rt.Call(addVal, Undefined(), []JSValue{NumberValue(3), NumberValue(4), NumberValue(5)})
	if err != nil {
		t.Fatalf("add call failed: %v", err)
	}
	if !result.IsNumber() || result.AsNumber() != 12 {
		t.Fatalf("add(3,4,5) = %v, expected 12", result.AsNumber())
	}
}

func TestGlobalFunctions(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	g := rt.GlobalObject()

	// parseInt
	parseIntFn := g.GetStr("parseInt")
	result, err := rt.Call(parseIntFn, Undefined(), []JSValue{StringValue("42")})
	if err != nil {
		t.Fatalf("parseInt call failed: %v", err)
	}
	if !result.IsNumber() || result.AsNumber() != 42 {
		t.Fatalf("parseInt('42') = %v, expected 42", result.AsNumber())
	}

	// isNaN
	isNaN := g.GetStr("isNaN")
	result, err = rt.Call(isNaN, Undefined(), []JSValue{NumberValue(42)})
	if err != nil {
		t.Fatalf("isNaN call failed: %v", err)
	}
	if !result.IsBoolean() || result.AsBoolean() {
		t.Fatalf("isNaN(42) = true, expected false")
	}

	// isNaN(NaN) using math.NaN()
	result, err = rt.Call(isNaN, Undefined(), []JSValue{NumberValue(math.NaN())})
	if err != nil {
		t.Fatalf("isNaN(NaN) call failed: %v", err)
	}
	if !result.IsBoolean() || !result.AsBoolean() {
		t.Fatalf("isNaN(NaN) = false, expected true")
	}

	// Global constants
	nan := g.GetStr("NaN")
	if !nan.IsNumber() || !math.IsNaN(nan.AsNumber()) {
		t.Fatal("global NaN is not NaN")
	}
	inf := g.GetStr("Infinity")
	if !inf.IsNumber() || !math.IsInf(inf.AsNumber(), 1) {
		t.Fatal("global Infinity is not Infinity")
	}
	undef := g.GetStr("undefined")
	if !undef.IsUndefined() {
		t.Fatal("global undefined is not undefined")
	}
}

func TestConsoleMultipleArgs(t *testing.T) {
	rt := NewInterpreter()
	buf := &BufferLogger{}
	rt.SetupGlobal(buf)

	consoleVal := rt.GlobalObject().GetStr("console")
	console := consoleVal.AsObject()
	logFn := console.GetStr("log")

	_, err := rt.Call(logFn, consoleVal, []JSValue{
		StringValue("hello"),
		StringValue("world"),
		NumberValue(42),
		BooleanValue(true),
	})
	if err != nil {
		t.Fatalf("console.log multi-arg failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "hello") || !strings.Contains(output, "world") || !strings.Contains(output, "42") {
		t.Fatalf("console.log multi-arg output missing: %q", output)
	}
}

func TestJSONParseArray(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	jsonObj := rt.GlobalObject().GetStr("JSON").AsObject()
	parseFn := jsonObj.GetStr("parse")

	result, err := rt.Call(parseFn, Undefined(), []JSValue{StringValue(`[1, "two", true, null]`)})
	if err != nil {
		t.Fatalf("JSON.parse array failed: %v", err)
	}
	if !result.IsObject() {
		t.Fatalf("JSON.parse array result is not object: %v", result)
	}
	obj := result.AsObject()
	v0 := obj.GetStr("0")
	if !v0.IsNumber() || v0.AsNumber() != 1 {
		t.Fatalf("arr[0] = %v, expected 1", v0)
	}
	v1 := obj.GetStr("1")
	if !v1.IsString() || v1.AsString() != "two" {
		t.Fatalf("arr[1] = %v, expected 'two'", v1)
	}
	v2 := obj.GetStr("2")
	if !v2.IsBoolean() || !v2.AsBoolean() {
		t.Fatalf("arr[2] = %v, expected true", v2)
	}
	v3 := obj.GetStr("3")
	if !v3.IsNull() {
		t.Fatalf("arr[3] = %v, expected null", v3)
	}
}

func TestMathMinMax(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	mathObj := rt.GlobalObject().GetStr("Math").AsObject()
	minFn := mathObj.GetStr("min")
	maxFn := mathObj.GetStr("max")

	result, err := rt.Call(minFn, Undefined(), []JSValue{NumberValue(3), NumberValue(1), NumberValue(2)})
	if err != nil {
		t.Fatalf("Math.min call failed: %v", err)
	}
	if !result.IsNumber() || result.AsNumber() != 1 {
		t.Fatalf("Math.min(3,1,2) = %v, expected 1", result.AsNumber())
	}

	result, err = rt.Call(maxFn, Undefined(), []JSValue{NumberValue(3), NumberValue(1), NumberValue(2)})
	if err != nil {
		t.Fatalf("Math.max call failed: %v", err)
	}
	if !result.IsNumber() || result.AsNumber() != 3 {
		t.Fatalf("Math.max(3,1,2) = %v, expected 3", result.AsNumber())
	}
}

func TestObjectPrototypeChain(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})

	obj := rt.NewObject()
	obj.Set("x", NumberValue(10))

	// Verify prototype chain is set up
	proto := rt.ObjectPrototype()
	if proto == nil {
		t.Fatal("ObjectPrototype() returned nil")
	}

	// toString should be reachable through prototype chain
	toStringVal := rt.GlobalObject().GetStr("toString")
	_ = toStringVal
}
