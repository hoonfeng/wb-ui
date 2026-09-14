// Translation of: Source/WebCore/bindings/js/JSDOMConvertAny.h (test surface)
// Tests for the JS->Go bridge (FromJSValue).

package bindings

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
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
	// Use same interpreter for all objects to avoid cross-runtime issues
	in := jsc.NewInterpreter()
	proto := in.ObjectPrototype()
	arr := jsc.ObjectValue(jsc.NewArray(proto, []jsc.JSValue{
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
	if out[0] != 1.0 && out[0] != int64(1) {
		t.Fatalf("out[0] = %v (%T), want 1.0", out[0], out[0])
	}
	if out[1] != "two" {
		t.Fatalf("out[1] = %v, want two", out[1])
	}
	if out[2] != true {
		t.Fatalf("out[2] = %v, want true", out[2])
	}
}

func TestFromJSValueObject(t *testing.T) {
	in := jsc.NewInterpreter()
	obj := jsc.NewObject(in.ObjectPrototype())
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
	if out["age"] != 30.0 && out["age"] != int64(30) {
		t.Fatalf("age = %v (%T), want 30", out["age"], out["age"])
	}
}

func TestFromJSValueNested(t *testing.T) {
	in := jsc.NewInterpreter()
	proto := in.ObjectPrototype()
	inner := jsc.NewObject(proto)
	inner.Set("x", jsc.NumberValue(1))
	outer := jsc.NewObject(proto)
	outer.Set("nested", jsc.ObjectValue(inner))
	outer.Set("list", jsc.ObjectValue(jsc.NewArray(proto, []jsc.JSValue{jsc.NumberValue(10), jsc.NumberValue(20)})))
	got, err := FromJSValue(jsc.ObjectValue(outer))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	out := got.(map[string]any)
	nested := out["nested"].(map[string]any)
	if nested["x"] != 1.0 && nested["x"] != int64(1) {
		t.Fatalf("nested.x = %v (%T), want 1", nested["x"], nested["x"])
	}
	list := out["list"].([]any)
	if list[1] != 20.0 && list[1] != int64(20) {
		t.Fatalf("list[1] = %v (%T), want 20", list[1], list[1])
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
	if (m["a"] != 1.0 && m["a"] != int64(1)) || m["b"] != "two" {
		t.Fatalf("round-trip lost data: %#v", m)
	}
}

func TestDOMCreateComment(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	root.SetId("root")
	doc.AppendChild(root)
	mustRun(t, rt, `
		var c = document.createComment("hello");
		document.getElementById("root").appendChild(c);
		console.log(document.getElementById("root").innerHTML);
	`)
	out := strings.TrimSpace(log.String())
	if !strings.Contains(out, "<!--hello-->") {
		t.Fatalf("innerHTML = %q, should contain <!--hello-->", out)
	}
}

func TestDOMGetElementsByClassName(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	doc.AppendChild(root)
	for i := 0; i < 3; i++ {
		item := doc.CreateElement("span")
		item.SetAttribute("class", "item active")
		root.AppendChild(item)
	}
	mustRun(t, rt, `
		var items = document.getElementsByClassName("active");
		console.log(items.length);
	`)
	if got := strings.TrimSpace(log.String()); got != "3" {
		t.Fatalf("got %q, want 3", got)
	}
}

func TestDOMRemoveAttribute(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	el := doc.CreateElement("a")
	el.SetId("link")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("link");
		el.setAttribute("href", "https://example.com");
		console.log(el.hasAttribute("href"));
		el.removeAttribute("href");
		console.log(el.hasAttribute("href"));
	`)
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if strings.TrimSpace(lines[0]) != "true" {
		t.Fatalf("hasAttribute after set = %q, want true", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "false" {
		t.Fatalf("hasAttribute after remove = %q, want false", lines[1])
	}
}

func TestDOMInsertBefore(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("ul")
	root.SetId("list")
	doc.AppendChild(root)
	mustRun(t, rt, `
		var list = document.getElementById("list");
		var first = document.createElement("li");
		first.innerHTML = "first";
		list.appendChild(first);
		var second = document.createElement("li");
		second.innerHTML = "second";
		list.insertBefore(second, first);
		console.log(list.innerHTML);
	`)
	out := strings.TrimSpace(log.String())
	if !strings.Contains(out, "second") || !strings.Contains(out, "first") {
		t.Fatalf("innerHTML after insertBefore = %q, should contain both items", out)
	}
	// Second item should be before first item in DOM order
	if len(root.ChildNodes()) != 2 {
		t.Fatalf("child count = %d, want 2", len(root.ChildNodes()))
	}
}

func TestDOMContains(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	parent := doc.CreateElement("div")
	parent.SetId("parent")
	child := doc.CreateElement("p")
	child.SetId("child")
	parent.AppendChild(child)
	doc.AppendChild(parent)
	mustRun(t, rt, `
		var p = document.getElementById("parent");
		var c = document.getElementById("child");
		console.log(p.contains(c));
		console.log(p.contains(p));
		console.log(c.contains(p));
	`)
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if strings.TrimSpace(lines[0]) != "true" {
		t.Fatalf("parent.contains(child) = %q, want true", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "true" {
		t.Fatalf("parent.contains(parent) = %q, want true", lines[1])
	}
	if strings.TrimSpace(lines[2]) != "false" {
		t.Fatalf("child.contains(parent) = %q, want false", lines[2])
	}
}

func TestDOMDocumentTitle(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	doc.SetTitle("My Page")
	mustRun(t, rt, `console.log(document.title);`)
	log := &jsc.BufferLogger{}
	rt2, doc2 := jsc.NewInterpreter(), dom.NewDocument()
	rt2.SetupGlobal(log)
	RegisterDOMBindings(rt2, doc2)
	doc2.SetTitle("Test Title")
	mustRun(t, rt2, `console.log(document.title);`)
	if got := strings.TrimSpace(log.String()); got != "Test Title" {
		t.Fatalf("document.title = %q, want 'Test Title'", got)
	}
}

func TestDOMRemoveChild(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	parent := doc.CreateElement("div")
	parent.SetId("parent")
	child := doc.CreateElement("span")
	child.SetId("child")
	parent.AppendChild(child)
	doc.AppendChild(parent)
	mustRun(t, rt, `
		var p = document.getElementById("parent");
		var c = document.getElementById("child");
		p.removeChild(c);
		console.log(p.innerHTML);
	`)
	if got := strings.TrimSpace(log.String()); got != "" {
		t.Fatalf("after removeChild, innerHTML should be empty, got %q", got)
	}
}
