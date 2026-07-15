// JS bindings for the wb-ui Markdown renderer.
//
// Exposes a wb namespace to JavaScript with a Markdown constructor:
//
//	const md = new wb.Markdown();
//	const frag = md.parse("# Title\n...");   // returns DOM fragment as JS wrapper
//
// This follows the same pattern as editor/bindings.go and bindings/dom.go.

package markdown

import (
	"wb-ui/dom"
	"wb-ui/jsc"
)

// RegisterMarkdownJSBindings registers Markdown bindings under the "wb" namespace
// on the JS global object. After calling this, JS code can use:
//
//	md = new wb.Markdown()
//	md.parse(src)   // returns a JS wrapper around a DocumentFragment
func RegisterMarkdownJSBindings(rt *jsc.Interpreter) {
	// Create or get the wb namespace object.
	wb := ensureWBNamespace(rt)

	// wb.Markdown constructor.
	wb.Set("Markdown", jsc.FunctionValue(jsc.NewNativeFunction("Markdown", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		mdObj := jsc.NewObject(in.ObjectPrototype())
		mdObj.ClassName = "Markdown"

		// parse(src) — returns a DocumentFragment as a JS object.
		mdObj.Set("parse", jsc.FunctionValue(jsc.NewNativeFunction("parse", func(in2 *jsc.Interpreter, _ jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			src := ""
			if len(args2) > 0 {
				src = args2[0].ToString()
			}
			if src == "" {
				return jsc.Null()
			}
			doc := dom.NewDocument()
			fragment := ParseToDOM(src, doc)
			if fragment == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapFragment(in2, fragment))
		}, 1)))

		return jsc.ObjectValue(mdObj)
	}, 0)))
}

// ensureWBNamespace returns the global "wb" object, creating it on first access.
func ensureWBNamespace(rt *jsc.Interpreter) *jsc.JSObject {
	g := rt.GlobalObject()
	if v, ok := g.Get("wb"); ok && v.IsObject() {
		return v.AsObject()
	}
	wb := jsc.NewObject(rt.ObjectPrototype())
	wb.ClassName = "wb"
	g.Set("wb", jsc.ObjectValue(wb))
	return wb
}

// wrapFragment converts a dom.DocumentFragment to a JSObject for the JS side.
func wrapFragment(in *jsc.Interpreter, fragment *dom.DocumentFragment) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.ClassName = "DocumentFragment"
	obj.Internal = fragment
	obj.Set("length", jsc.NumberValue(float64(len(fragment.ChildNodes()))))
	return obj
}
