// Translation of: Source/WebCore/bindings/js/JSDOMBinding.h (test surface)
// Tests for the Go->JS bridge: ToJSValue, RegisterGoFunction, RegisterGoObject,
// CallJSFunction.
//
// Filename note: see go2js.go. This file is named go2js_test.go (not go_to_js_test.go)
// for the same GOOS=js build-constraint reason.

package bindings

import (
	"strings"
	"testing"

	"wb-ui/jsc"
)

func TestToJSValuePrimitives(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want jsc.JSValue
	}{
		{"nil", nil, jsc.Null()},
		{"bool true", true, jsc.BooleanValue(true)},
		{"bool false", false, jsc.BooleanValue(false)},
		{"int", 42, jsc.NumberValue(42)},
		{"int64", int64(-7), jsc.NumberValue(-7)},
		{"float64", 3.14, jsc.NumberValue(3.14)},
		{"string", "hi", jsc.StringValue("hi")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ToJSValue(c.in)
			if !got.StrictEquals(c.want) {
				t.Fatalf("ToJSValue(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestToJSValueSliceAndMap(t *testing.T) {
	in := []any{1.0, "two", true}
	got := ToJSValue(in)
	if !got.IsObject() {
		t.Fatalf("slice should become object, got %v", got)
	}
	o := got.AsObject()
	if !o.IsArray {
		t.Fatalf("expected array, got plain object")
	}
	if got := o.Elements[0]; !got.StrictEquals(jsc.NumberValue(1)) {
		t.Fatalf("elem 0 = %v, want 1", got)
	}
	if got := o.Elements[1]; !got.StrictEquals(jsc.StringValue("two")) {
		t.Fatalf("elem 1 = %v, want 'two'", got)
	}

	m := map[string]any{"name": "Alice", "age": 30}
	mv := ToJSValue(m)
	if !mv.IsObject() {
		t.Fatalf("map should become object")
	}
	name, ok := mv.AsObject().Get("name")
	if !ok || !name.StrictEquals(jsc.StringValue("Alice")) {
		t.Fatalf("name = %v, want Alice", name)
	}
}

func TestToJSValueStruct(t *testing.T) {
	type point struct{ X, Y int }
	p := point{X: 1, Y: 2}
	v := ToJSValue(p)
	if !v.IsObject() {
		t.Fatalf("struct should become object")
	}
	x, _ := v.AsObject().Get("X")
	if !x.StrictEquals(jsc.NumberValue(1)) {
		t.Fatalf("X = %v, want 1", x)
	}
}

func TestRegisterGoFunctionToUpper(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	RegisterGoFunction(rt, "ToUpper", func(args []jsc.JSValue) (jsc.JSValue, error) {
		if len(args) == 0 {
			return jsc.StringValue(""), nil
		}
		return jsc.StringValue(strings.ToUpper(args[0].ToString())), nil
	})
	if _, err := rt.Run(`console.log(go.ToUpper("hello"));`); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got := strings.TrimSpace(log.String()); got != "HELLO" {
		t.Fatalf("got %q, want HELLO", log.String())
	}
}

func TestRegisterGoObject(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	RegisterGoObject(rt, "math2", map[string]any{
		"double": func(args []jsc.JSValue) (jsc.JSValue, error) {
			if len(args) == 0 {
				return jsc.NumberValue(0), nil
			}
			return jsc.NumberValue(args[0].ToNumber() * 2), nil
		},
		"pi":     3.14,
		"nested": map[string]any{"greeting": "hi"},
	})
	if _, err := rt.Run(`
		console.log(math2.double(21));
		console.log(math2.pi);
		console.log(math2.nested.greeting);
	`); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	want := []string{"42", "3.14", "hi"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines: %v", len(lines), lines)
	}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestCallJSFunction(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	if _, err := rt.Run(`function add(a, b) { return a + b; }`); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	v, err := CallJSFunction(rt, "add", 3, 4)
	if err != nil {
		t.Fatalf("CallJSFunction error: %v", err)
	}
	if !v.IsNumber() || v.AsNumber() != 7 {
		t.Fatalf("add(3,4) = %v, want 7", v)
	}
}

func TestCallJSFunctionUndefined(t *testing.T) {
	rt := jsc.NewInterpreter()
	rt.SetupGlobal(nil)
	if _, err := CallJSFunction(rt, "nope", 1); err == nil {
		t.Fatalf("expected error for unknown function")
	}
}

// TestGoCallbackErrorThrows verifies that a GoCallback returning an error is caught
// as a JS exception via try/catch.
func TestGoCallbackErrorThrows(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	// Register a Go function that always errors.
	RegisterGoFunction(rt, "fail", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.Undefined(), &customError{msg: "something went wrong"}
	})
	if _, err := rt.Run(`
		try {
			go.fail();
			console.log("no-error");
		} catch(e) {
			console.log("caught:" + e);
		}
	`); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got := log.String(); got != "caught:Error: something went wrong" {
		t.Fatalf("got %q, want 'caught:Error: something went wrong'", got)
	}
}

// TestGoCallbackErrorPropagates verifies that an error from a nested Go callback
// propagates correctly through the call stack.
func TestGoCallbackErrorPropagates(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	// Inner: always fails.
	RegisterGoFunction(rt, "inner", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.Undefined(), &customError{msg: "inner fail"}
	})
	// Outer: calls inner.
	if _, err := rt.Run(`
		try {
			go.inner();
			console.log("no-error");
		} catch(e) {
			console.log("caught:" + e);
		}
	`); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got := log.String(); got != "caught:Error: inner fail" {
		t.Fatalf("got %q, want 'caught:Error: inner fail'", got)
	}
}

// customError is a simple error type for test assertions.
type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }
