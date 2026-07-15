// Translation of: Source/WebCore/bindings/js/JSDOMBinding.h
//                  Source/WebCore/bindings/js/JSDocumentCustom.cpp
//                  Source/WebCore/bindings/js/JSElementCustom.cpp
//                  Source/WebCore/bindings/js/JSEventTargetCustom.cpp
//                  Source/WebCore/bindings/js/JSNodeCustom.cpp
// Completeness: 55%
// Simplifications:
//   - DOM nodes are wrapped one-shot (no wrapper cache / WeakMap): each call to
//     document.getElementById returns a fresh JSObject wrapper around the same node.
//   - innerHTML/outerHTML/id/className/textContent/tagName are JS accessor properties
//     so assignment routes back into Go (e.g. el.innerHTML = "..." calls SetInnerHTML).
//   - element methods are own properties of each wrapper (no prototype chain), keeping
//     the bridge self-contained and avoiding a per-class prototype object.
//   - appendChild unwraps the child JS wrapper by reading JSObject.Internal.

package bindings

import (
	"wb-ui/dom"
	"wb-ui/jsc"
)

// RegisterDOMBindings installs a JS "document" object whose methods route to the Go
// dom.Document, mirroring how WebKit's JSDocument exposes the document to scripts.
// After registration JS code can call document.getElementById/createElement/etc. and
// operate on the returned element wrappers.
func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	docObj := wrapDocument(rt, document)
	rt.GlobalObject().Set("document", jsc.ObjectValue(docObj))
}

// ElementWrapper pairs a JS wrapper object with its underlying dom.Element. The JS
// wrapper is what JS code sees; the DOM pointer is stashed on JSObject.Internal so
// that host methods receiving a wrapper (e.g. appendChild's argument) can recover the
// Go node. The struct is exported for callers that want to keep both halves together.
type ElementWrapper struct {
	JS  *jsc.JSObject
	DOM *dom.Element
}

// wrapDocument builds the JS object representing a dom.Document. It exposes
// getElementById, createElement, createTextNode, addEventListener, dispatchEvent and
// accessor properties for title.
func wrapDocument(rt *jsc.Interpreter, doc *dom.Document) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Document"
	obj.Internal = doc
	obj.Set("getElementById", jsc.FunctionValue(jsc.NewNativeFunction("getElementById",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			el := doc.GetElementById(args[0].ToString())
			if el == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapElement(in, el))
		}, 1)))
	obj.Set("createElement", jsc.FunctionValue(jsc.NewNativeFunction("createElement",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			el := doc.CreateElement(args[0].ToString())
			return jsc.ObjectValue(wrapElement(in, el))
		}, 1)))
	obj.Set("createTextNode", jsc.FunctionValue(jsc.NewNativeFunction("createTextNode",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			t := doc.CreateTextNode(args[0].ToString())
			return jsc.ObjectValue(wrapText(in, t))
		}, 1)))
	obj.Set("createComment", jsc.FunctionValue(jsc.NewNativeFunction("createComment",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			c := doc.CreateComment(args[0].ToString())
			return jsc.ObjectValue(wrapComment(in, c))
		}, 1)))
	obj.Set("getElementsByTagName", jsc.FunctionValue(jsc.NewNativeFunction("getElementsByTagName",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), nil))
			}
			els := doc.GetElementsByTagName(args[0].ToString())
			elems := make([]jsc.JSValue, len(els))
			for i, e := range els {
				elems[i] = jsc.ObjectValue(wrapElement(in, e))
			}
			return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), elems))
		}, 1)))
	obj.Set("getElementsByClassName", jsc.FunctionValue(jsc.NewNativeFunction("getElementsByClassName",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), nil))
			}
			els := doc.GetElementsByClassName(args[0].ToString())
			elems := make([]jsc.JSValue, len(els))
			for i, e := range els {
				elems[i] = jsc.ObjectValue(wrapElement(in, e))
			}
			return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), elems))
		}, 1)))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(doc)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(doc)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(doc)))
	obj.SetAccessor("title",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(doc.Title()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { doc.SetTitle(v.ToString()) },
	)
	obj.SetAccessor("url",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(doc.URL()) },
		nil,
	)
	return obj
}

// wrapElement builds the JS wrapper for a dom.Element. Methods (getAttribute,
// setAttribute, appendChild, addEventListener, ...) are installed as own properties.
// Accessor properties (tagName/id/className/innerHTML/outerHTML/textContent) route
// reads/writes back into Go.
func wrapElement(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Element"
	obj.Internal = el

	obj.Set("getAttribute", jsc.FunctionValue(jsc.NewNativeFunction("getAttribute",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.StringValue("")
			}
			return jsc.StringValue(el.GetAttribute(args[0].ToString()))
		}, 1)))
	obj.Set("setAttribute", jsc.FunctionValue(jsc.NewNativeFunction("setAttribute",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 {
				return jsc.Undefined()
			}
			el.SetAttribute(args[0].ToString(), args[1].ToString())
			return jsc.Undefined()
		}, 2)))
	obj.Set("hasAttribute", jsc.FunctionValue(jsc.NewNativeFunction("hasAttribute",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(el.HasAttribute(args[0].ToString()))
		}, 1)))
	obj.Set("removeAttribute", jsc.FunctionValue(jsc.NewNativeFunction("removeAttribute",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) > 0 {
				el.RemoveAttribute(args[0].ToString())
			}
			return jsc.Undefined()
		}, 1)))
	obj.Set("getAttributeNames", jsc.FunctionValue(jsc.NewNativeFunction("getAttributeNames",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			names := el.AttributeNames()
			elems := make([]jsc.JSValue, len(names))
			for i, n := range names {
				elems[i] = jsc.StringValue(n)
			}
			return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), elems))
		}, 0)))
	obj.Set("appendChild", jsc.FunctionValue(jsc.NewNativeFunction("appendChild",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			child := unwrapNode(args[0])
			if child == nil {
				return jsc.Null()
			}
			if err := el.AppendChild(child); err != nil {
				return jsc.Null()
			}
			return args[0]
		}, 1)))
	obj.Set("removeChild", jsc.FunctionValue(jsc.NewNativeFunction("removeChild",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			child := unwrapNode(args[0])
			if child == nil {
				return jsc.Null()
			}
			if err := el.RemoveChild(child); err != nil {
				return jsc.Null()
			}
			return args[0]
		}, 1)))
	obj.Set("insertBefore", jsc.FunctionValue(jsc.NewNativeFunction("insertBefore",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 {
				return jsc.Null()
			}
			newChild := unwrapNode(args[0])
			if newChild == nil {
				return jsc.Null()
			}
			var refChild dom.Node
			if len(args) >= 2 {
				refChild = unwrapNode(args[1])
			}
			if err := el.InsertBefore(newChild, refChild); err != nil {
				return jsc.Null()
			}
			return args[0]
		}, 2)))
	obj.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			other := unwrapNode(args[0])
			if other == nil {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(el.Contains(other))
		}, 1)))
	obj.Set("getElementById", jsc.FunctionValue(jsc.NewNativeFunction("getElementById",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			el := el.GetElementById(args[0].ToString())
			if el == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapElement(in, el))
		}, 1)))
	obj.Set("getElementsByTagName", jsc.FunctionValue(jsc.NewNativeFunction("getElementsByTagName",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), nil))
			}
			els := el.GetElementsByTagName(args[0].ToString())
			elems := make([]jsc.JSValue, len(els))
			for i, e := range els {
				elems[i] = jsc.ObjectValue(wrapElement(in, e))
			}
			return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), elems))
		}, 1)))
	obj.Set("getElementsByClassName", jsc.FunctionValue(jsc.NewNativeFunction("getElementsByClassName",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), nil))
			}
			els := el.GetElementsByClassName(args[0].ToString())
			elems := make([]jsc.JSValue, len(els))
			for i, e := range els {
				elems[i] = jsc.ObjectValue(wrapElement(in, e))
			}
			return jsc.ObjectValue(jsc.NewArray(in.ArrayPrototype(), elems))
		}, 1)))
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			deep := false
			if len(args) > 0 {
				deep = args[0].ToBoolean()
			}
			c := el.CloneNode(deep)
			if ce, ok := c.(*dom.Element); ok {
				return jsc.ObjectValue(wrapElement(in, ce))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(el)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(el)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(el)))

	// Accessor properties. Writes route back into Go so JS assignments like
	// `element.innerHTML = "<b>hi</b>"` mutate the real DOM.
	obj.SetAccessor("tagName",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.TagName()) },
		nil)
	obj.SetAccessor("nodeName",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.NodeName()) },
		nil)
	obj.SetAccessor("id",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.GetId()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { el.SetId(v.ToString()) })
	obj.SetAccessor("className",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.GetClassName()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	obj.SetAccessor("innerHTML",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.GetInnerHTML()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { _ = el.SetInnerHTML(v.ToString()) })
	obj.SetAccessor("outerHTML",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.GetOuterHTML()) },
		nil)
	obj.SetAccessor("textContent",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(el.TextContent()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { _ = el.SetTextContent(v.ToString()) })
	return obj
}

// wrapText builds the JS wrapper for a dom.Text node, exposing data/textContent as
// accessor properties and appendData/insertData/deleteData as methods.
func wrapText(rt *jsc.Interpreter, t *dom.Text) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Text"
	obj.Internal = t
	obj.Set("appendData", jsc.FunctionValue(jsc.NewNativeFunction("appendData",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) > 0 {
				t.AppendData(args[0].ToString())
			}
			return jsc.Undefined()
		}, 1)))
	obj.Set("insertData", jsc.FunctionValue(jsc.NewNativeFunction("insertData",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 2 {
				t.InsertData(int(args[0].ToNumber()), args[1].ToString())
			}
			return jsc.Undefined()
		}, 2)))
	obj.Set("deleteData", jsc.FunctionValue(jsc.NewNativeFunction("deleteData",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 2 {
				t.DeleteData(int(args[0].ToNumber()), int(args[1].ToNumber()))
			}
			return jsc.Undefined()
		}, 2)))
	obj.Set("splitText", jsc.FunctionValue(jsc.NewNativeFunction("splitText",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			off := 0
			if len(args) > 0 {
				off = int(args[0].ToNumber())
			}
			nt, err := t.SplitText(off)
			if err != nil || nt == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapText(in, nt))
		}, 1)))
	obj.SetAccessor("data",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(t.Data()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(t.Data()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("length",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.NumberValue(float64(t.Length())) },
		nil)
	obj.SetAccessor("nodeName",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(t.NodeName()) },
		nil)
	return obj
}

// wrapComment builds the JS wrapper for a dom.Comment node.
func wrapComment(rt *jsc.Interpreter, c *dom.Comment) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Comment"
	obj.Internal = c
	obj.SetAccessor("data",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(c.Data()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		func(in *jsc.Interpreter, this jsc.JSValue) jsc.JSValue { return jsc.StringValue(c.Data()) },
		func(in *jsc.Interpreter, this jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	return obj
}

// unwrapNode recovers the dom.Node stored on a JS wrapper's Internal slot. Returns nil
// for non-object values or wrappers that do not wrap a node.
func unwrapNode(v jsc.JSValue) dom.Node {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	if n, ok := o.Internal.(dom.Node); ok {
		return n
	}
	return nil
}
