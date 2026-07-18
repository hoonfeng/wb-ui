// Package bindings implements Go <-> JS <-> DOM bridges for wb-ui.
// Completeness: 70% — adds full style/classList/traversal/event for SPA support.
package bindings

import (
	"fmt"
	"strings"

	"wb-ui/dom"
	"wb-ui/jsc"
)

func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	docObj := wrapDocument(rt, document)
	rt.GlobalObject().Set("document", jsc.ObjectValue(docObj))

	// window / self / globalThis → 全局对象
	g := rt.GlobalObject()
	g.Set("window", jsc.ObjectValue(g))
	g.Set("self", jsc.ObjectValue(g))
	g.Set("globalThis", jsc.ObjectValue(g))

	// window.location 桩
	loc := jsc.NewObject(rt.ObjectPrototype())
	loc.Set("href", jsc.StringValue("about:blank"))
	loc.Set("origin", jsc.StringValue(""))
	loc.Set("hostname", jsc.StringValue(""))
	loc.Set("pathname", jsc.StringValue("/"))
	loc.Set("search", jsc.StringValue(""))
	loc.Set("hash", jsc.StringValue(""))
	loc.Set("protocol", jsc.StringValue("file:"))
	g.Set("location", jsc.ObjectValue(loc))

	// window.navigator 桩
	nav := jsc.NewObject(rt.ObjectPrototype())
	nav.Set("userAgent", jsc.StringValue("wb-ui"))
	nav.Set("platform", jsc.StringValue("Go"))
	nav.Set("language", jsc.StringValue("zh-CN"))
	nav.Set("languages", jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.StringValue("zh-CN"), jsc.StringValue("en")})))
	g.Set("navigator", jsc.ObjectValue(nav))

	// window.screen 桩
	screen := jsc.NewObject(rt.ObjectPrototype())
	screen.Set("width", jsc.NumberValue(1280))
	screen.Set("height", jsc.NumberValue(800))
	g.Set("screen", jsc.ObjectValue(screen))

	// window.console 由 SetupGlobal 设置

	// window.matchMedia 桩
	g.Set("matchMedia", jsc.FunctionValue(jsc.NewNativeFunction("matchMedia",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			mq := jsc.NewObject(in.ObjectPrototype())
			mq.Set("matches", jsc.BooleanValue(false))
			return jsc.ObjectValue(mq)
		}, 1)))

	// setTimeout / setInterval / clearTimeout / clearInterval 桩
	g.Set("setTimeout", jsc.FunctionValue(jsc.NewNativeFunction("setTimeout",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return jsc.NumberValue(0)
		}, 2)))
	g.Set("setInterval", jsc.FunctionValue(jsc.NewNativeFunction("setInterval",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return jsc.NumberValue(0)
		}, 2)))
	g.Set("clearTimeout", jsc.FunctionValue(jsc.NewNativeFunction("clearTimeout",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	g.Set("clearInterval", jsc.FunctionValue(jsc.NewNativeFunction("clearInterval",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))

	// requestAnimationFrame / cancelAnimationFrame 桩
	g.Set("requestAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("requestAnimationFrame",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.NumberValue(0)
		}, 1)))
	g.Set("cancelAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("cancelAnimationFrame",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
}

type ElementWrapper struct {
	JS  *jsc.JSObject
	DOM *dom.Element
}

// ─── Document ──────────────────────────────────────────

func wrapDocument(rt *jsc.Interpreter, doc *dom.Document) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Document"
	obj.Internal = doc

	obj.Set("getElementById", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		if el := doc.GetElementById(arg); el != nil {
			return jsc.ObjectValue(wrapElement(in, el))
		}
		return jsc.Null()
	})))
	obj.Set("createElement", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createTextNode", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapText(in, doc.CreateTextNode(arg)))
	})))
	obj.Set("createComment", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapComment(in, doc.CreateComment(arg)))
	})))
	obj.Set("createDocumentFragment", funcVal(fn0(func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(wrapDocFrag(in, doc.CreateDocumentFragment()))
	})))
	obj.Set("createEvent", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return eventToJS(in, doc.CreateEvent(arg))
	})))
	obj.Set("querySelector", funcVal(fn1(func(in *jsc.Interpreter, sel string) jsc.JSValue {
		if strings.HasPrefix(sel, "#") {
			if el := doc.GetElementById(sel[1:]); el != nil {
				return jsc.ObjectValue(wrapElement(in, el))
			}
		}
		if els := doc.GetElementsByTagName(sel); len(els) > 0 {
			return jsc.ObjectValue(wrapElement(in, els[0]))
		}
		return jsc.Null()
	})))
	obj.Set("getElementsByTagName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByTagName(arg)
		return arrJS(in, els)
	})))
	obj.Set("getElementsByClassName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByClassName(arg)
		return arrJS(in, els)
	})))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(doc)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(doc)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(doc)))

	// Accessors
	obj.SetAccessor("body", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if b := doc.Body(); b != nil { return jsc.ObjectValue(wrapElement(in, b)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("head", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if h := doc.Head(); h != nil { return jsc.ObjectValue(wrapElement(in, h)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("documentElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if de := doc.DocumentElement(); de != nil { return jsc.ObjectValue(wrapElement(in, de)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("title",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(doc.Title()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { doc.SetTitle(v.ToString()) })
	obj.SetAccessor("URL", strAcc(doc.URL()), nil)
	obj.SetAccessor("cookie", strAcc(""), nil)

	return obj
}

// ─── Element ───────────────────────────────────────────

func wrapElement(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Element"
	obj.Internal = el

	// Attributes
	obj.Set("getAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.StringValue(el.GetAttribute(arg))
	})))
	obj.Set("setAttribute", funcVal(fn2(func(_ *jsc.Interpreter, a, b string) jsc.JSValue {
		el.SetAttribute(a, b)
		return jsc.Undefined()
	})))
	obj.Set("hasAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.BooleanValue(el.HasAttribute(arg))
	})))
	obj.Set("removeAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		el.RemoveAttribute(arg)
		return jsc.Undefined()
	})))
	obj.Set("toggleAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		if el.HasAttribute(arg) { el.RemoveAttribute(arg); return jsc.BooleanValue(false) }
		el.SetAttribute(arg, "")
		return jsc.BooleanValue(true)
	})))

	// Node tree
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.AppendChild(n)
		return a
	})))
	obj.Set("removeChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.RemoveChild(n)
		return a
	})))
	obj.Set("insertBefore", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil { return jsc.Null() }
		el.InsertBefore(nc, rc)
		return a0
	})))
	obj.Set("replaceChild", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, oc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil || oc == nil { return jsc.Null() }
		el.ReplaceChild(nc, oc)
		return a1
	})))
	obj.Set("contains", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, _ jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.BooleanValue(false) }
		return jsc.BooleanValue(el.Contains(n))
	})))
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			deep := len(args) > 0 && args[0].ToBoolean()
			switch v := el.CloneNode(deep).(type) {
			case *dom.Element:
				return jsc.ObjectValue(wrapElement(in, v))
			case *dom.Text:
				return jsc.ObjectValue(wrapText(in, v))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("hasChildNodes", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.HasChildNodes())
	})))
	obj.Set("isConnected", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.IsConnected())
	})))

	// classList
	obj.Set("classList", jsc.ObjectValue(makeClassList(rt, el)))

	// style — a live object that reads/writes the style attribute
	obj.Set("style", jsc.ObjectValue(makeStyleObject(rt, el)))

	// Events
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(el)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(el)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(el)))

	// Tree traversal — dynamic getters so they reflect live DOM tree
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return el.ParentNode() }), nil)
	obj.SetAccessor("parentElement", nodeAccFn(rt, func() dom.Node { return el.ParentElement() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return el.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return el.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return el.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return el.LastChild() }), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		n := 0
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*dom.Element); ok { n++ }
		}
		return jsc.NumberValue(float64(n))
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		var els []*dom.Element
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok { els = append(els, e) }
		}
		return arrElem(in, els)
	}), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, el.ChildNodes())
	}), nil)
	// ownerDocument — needed by Vue 3 when checking element's document
	obj.SetAccessor("ownerDocument", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return in.GlobalObject().GetOrZero("document")
	}), nil)

	// Position / dimension stubs (Vue needs these)
	obj.Set("getBoundingClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			r := jsc.NewObject(in.ObjectPrototype())
			r.Set("x", jsc.NumberValue(0))
			r.Set("y", jsc.NumberValue(0))
			r.Set("width", jsc.NumberValue(0))
			r.Set("height", jsc.NumberValue(0))
			r.Set("top", jsc.NumberValue(0))
			r.Set("right", jsc.NumberValue(0))
			r.Set("bottom", jsc.NumberValue(0))
			r.Set("left", jsc.NumberValue(0))
			return jsc.ObjectValue(r)
		}, 0)))
	obj.Set("scrollIntoView", jsc.FunctionValue(jsc.NewNativeFunction("scrollIntoView",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))

	// Accessors for string properties
	obj.SetAccessor("tagName", strAcc(el.TagName()), nil)
	obj.SetAccessor("nodeName", strAcc(el.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(el.NodeType()))
	}), nil)
	obj.SetAccessor("id",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetId()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetId(v.ToString()) })
	obj.SetAccessor("className",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetClassName()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	obj.SetAccessor("innerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetInnerHTML()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetInnerHTML(v.ToString()) })
	obj.SetAccessor("outerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetOuterHTML()) }), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetTextContent(v.ToString()) })

	return obj
}

// ─── classList ──────────────────────────────────────────

func makeClassList(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	cls := jsc.NewObject(rt.ObjectPrototype())
	get := func() []string { return strings.Fields(el.GetClassName()) }
	set := func(c []string) { el.SetClassName(strings.Join(c, " ")) }

	cls.Set("add", jsc.FunctionValue(jsc.NewNativeFunction("add",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { m[a.ToString()] = true }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { delete(m, a.ToString()) }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			for _, c := range get() { if c == n { return jsc.BooleanValue(true) } }
			return jsc.BooleanValue(false)
		}, 1)))
	cls.Set("toggle", jsc.FunctionValue(jsc.NewNativeFunction("toggle",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			cs := get()
			for i, c := range cs {
				if c == n {
					cs = append(cs[:i], cs[i+1:]...)
					set(cs)
					return jsc.BooleanValue(false)
				}
			}
			cs = append(cs, n)
			set(cs)
			return jsc.BooleanValue(true)
		}, 1)))
	return cls
}

// ─── style object ──────────────────────────────────────
// A live CSSStyleDeclaration that reads/writes the element's style attribute.

func makeStyleObject(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	s := jsc.NewObject(rt.ObjectPrototype())
	s.ClassName = "CSSStyleDeclaration"
	s.Internal = el

	// cssText getter/setter
	s.SetAccessor("cssText",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(el.GetAttribute("style"))
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			el.SetAttribute("style", v.ToString())
		})

		s.Set("setProperty", jsc.FunctionValue(jsc.NewNativeFunction("setProperty",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 2 { return jsc.Undefined() }
				cssText := el.GetAttribute("style")
				props := parseStyle(cssText)
				props[args[0].ToString()] = args[1].ToString()
				el.SetAttribute("style", joinStyle(props))
				return jsc.Undefined()
			}, 2)))
		s.Set("removeProperty", jsc.FunctionValue(jsc.NewNativeFunction("removeProperty",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 { return jsc.StringValue("") }
				cssText := el.GetAttribute("style")
				props := parseStyle(cssText)
				old := props[args[0].ToString()]
				delete(props, args[0].ToString())
				el.SetAttribute("style", joinStyle(props))
				return jsc.StringValue(old)
			}, 1)))

	return s
}

// parseStyle parses "color:red;font-size:16px" → map
func parseStyle(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func joinStyle(m map[string]string) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ";")
}

// ─── DocumentFragment ──────────────────────────────────

func wrapDocFrag(rt *jsc.Interpreter, frag *dom.DocumentFragment) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "DocumentFragment"
	obj.Internal = frag
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		frag.AppendChild(n)
		return a
	})))
	return obj
}

// ─── Text / Comment ────────────────────────────────────

func wrapText(rt *jsc.Interpreter, t *dom.Text) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Text"
	obj.Internal = t
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := t.ParentNode(); p != nil { p.RemoveChild(t) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		strAcc(t.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		strAcc(t.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.Length()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(t.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.NodeType()))
	}), nil)
	return obj
}

func wrapComment(rt *jsc.Interpreter, c *dom.Comment) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.ClassName = "Comment"
	obj.Internal = c
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := c.ParentNode(); p != nil { p.RemoveChild(c) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		strAcc(c.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	return obj
}

// ─── Helpers ───────────────────────────────────────────

func unwrapNode(v jsc.JSValue) dom.Node {
	if !v.IsObject() { return nil }
	if n, ok := v.AsObject().Internal.(dom.Node); ok { return n }
	return nil
}

// Accessor helpers
type getterFn = func(*jsc.Interpreter, jsc.JSValue) jsc.JSValue

func getter(fn func(*jsc.Interpreter) jsc.JSValue) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return fn(in) }
}

func strAcc(s string) getterFn {
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.StringValue(s) }
}

func nodeAcc(rt *jsc.Interpreter, n dom.Node) getterFn {
	if n == nil {
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
	}
	switch v := n.(type) {
	case *dom.Element:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapElement(rt, v))
		}
	case *dom.Text:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapText(rt, v))
		}
	}
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
}

// nodeAccFn returns an accessor getter that calls fn() each time it is read,
// so it stays in sync with the live DOM tree.
func nodeAccFn(rt *jsc.Interpreter, fn func() dom.Node) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
		n := fn()
		if n == nil {
			return jsc.Null()
		}
		switch v := n.(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		}
		return jsc.Null()
	}
}

func funcVal(fn *jsc.JSFunction) jsc.JSValue { return jsc.FunctionValue(fn) }

// fn0/fn1/fn2 helpers
func fn0(fn func(in *jsc.Interpreter) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return fn(in)
	}, 0)
}

func fn1(fn func(in *jsc.Interpreter, arg string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, args[0].ToString())
	}, 1)
}

func fn2(fn func(in *jsc.Interpreter, a, b string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Undefined() }
		return fn(in, args[0].ToString(), args[1].ToString())
	}, 2)
}

func fn1Node(fn func(in *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), args[0])
	}, 1)
}

func fn2Node(fn func(in *jsc.Interpreter, n1, n2 dom.Node, a0, a1 jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), unwrapNode(args[1]), args[0], args[1])
	}, 2)
}

// arr helpers
func arrElem(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

func arrNode(in *jsc.Interpreter, nodes []dom.Node) jsc.JSValue {
	return arrayValue(in, len(nodes), func(i int) jsc.JSValue {
		switch v := nodes[i].(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		}
		return jsc.Null()
	})
}

func arrJS(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

func arrayValue(_ *jsc.Interpreter, n int, fn func(int) jsc.JSValue) jsc.JSValue {
	arr := make([]jsc.JSValue, n)
	for i := 0; i < n; i++ { arr[i] = fn(i) }
	return jsc.ObjectValue(jsc.NewArray(nil, arr))
}

// Silence unused import warning
var _ = fmt.Sprintf
