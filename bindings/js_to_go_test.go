// Translation of: Source/WebCore/bindings/js/JSDOMConvertAny.h (test surface)
// Tests for the JS->Go bridge (FromJSValue).

package bindings

import (
	"testing"

	"wb-ui/jsc"
)

func TestFromJSValuePrimitives(t *testing.T) {
	cases := []struct {
		name string
		in   jsc.JSValue
		want any
	}{
		{"undefined", jsc.Undefined(), nil},
		{"null", jsc.Null(), nil},
		{"true", jsc.BooleanValue(true), true},
		{"false", jsc.BooleanValue(false), false},
		{"number", jsc.NumberValue(3.5), 3.5},
		{"string", jsc.StringValue("hi"), "hi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := FromJSValue(c.in)
			if err != nil {
				t.Fatalf("FromJSValue error: %v", err)
			}
			if got != c.want {
				t.Fatalf("FromJSValue(%v) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestFromJSValueArray(t *testing.T) {
	arr := jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{
		jsc.NumberValue(1), jsc.StringValue("two"), jsc.BooleanValue(true),
	}))
	got, err := FromJSValue(arr)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	out, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0] != 1.0 {
		t.Fatalf("out[0] = %v, want 1.0", out[0])
	}
	if out[1] != "two" {
		t.Fatalf("out[1] = %v, want two", out[1])
	}
	if out[2] != true {
		t.Fatalf("out[2] = %v, want true", out[2])
	}
}

func TestFromJSValueObject(t *testing.T) {
	obj := jsc.NewObject(nil)
	obj.Set("name", jsc.StringValue("Alice"))
	obj.Set("age", jsc.NumberValue(30))
	got, err := FromJSValue(jsc.ObjectValue(obj))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	out, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if out["name"] != "Alice" {
		t.Fatalf("name = %v, want Alice", out["name"])
	}
	if out["age"] != 30.0 {
		t.Fatalf("age = %v, want 30", out["age"])
	}
}

func TestFromJSValueNested(t *testing.T) {
	inner := jsc.NewObject(nil)
	inner.Set("x", jsc.NumberValue(1))
	outer := jsc.NewObject(nil)
	outer.Set("nested", jsc.ObjectValue(inner))
	outer.Set("list", jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.NumberValue(10), jsc.NumberValue(20)})))
	got, err := FromJSValue(jsc.ObjectValue(outer))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	out := got.(map[string]any)
	nested := out["nested"].(map[string]any)
	if nested["x"] != 1.0 {
		t.Fatalf("nested.x = %v, want 1", nested["x"])
	}
	list := out["list"].([]any)
	if list[1] != 20.0 {
		t.Fatalf("list[1] = %v, want 20", list[1])
	}
}

func TestFromJSValueFunctionReturnsNil(t *testing.T) {
	fn := jsc.FunctionValue(jsc.NewNativeFunction("f",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0))
	got, err := FromJSValue(fn)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFromJSValueRoundTrip(t *testing.T) {
	original := map[string]any{"a": 1.0, "b": "two"}
	jsv := ToJSValue(original)
	back, err := FromJSValue(jsv)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := back.(map[string]any)
	if m["a"] != 1.0 || m["b"] != "two" {
		t.Fatalf("round-trip lost data: %#v", m)
	}
}
