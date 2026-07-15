// Translation of: Source/WebCore/bindings/js/JSDocumentCustom.cpp (test surface)
//                  Source/WebCore/bindings/js/JSElementCustom.cpp (test surface)
//                  Source/WebCore/bindings/js/JSEventListener.cpp (test surface)
// Tests for the DOM bindings: document/element wrappers, accessor properties, and
// JS-registered event listeners invoked from Go.

package bindings

import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/jsc"
)

// newRuntimeWithDoc builds an interpreter with console logging and registers DOM
// bindings for a fresh document. It returns the runtime, the document and the log.
func newRuntimeWithDoc(t *testing.T) (*jsc.Interpreter, *dom.Document, *jsc.BufferLogger) {
	t.Helper()
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)
	doc := dom.NewDocument()
	RegisterDOMBindings(rt, doc)
	return rt, doc, log
}

func mustRun(t *testing.T, rt *jsc.Interpreter, src string) {
	t.Helper()
	if _, err := rt.Run(src); err != nil {
		t.Fatalf("Run(%q) error: %v", src, err)
	}
}

func TestDOMGetElementByIdReturnsWrapper(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("foo")
	doc.AppendChild(el)
	mustRun(t, rt, `console.log(document.getElementById("foo").tagName);`)
	if got := strings.TrimSpace(log.String()); got != "DIV" {
		t.Fatalf("got %q, want DIV", got)
	}
}

func TestDOMGetElementByIdMissingReturnsNull(t *testing.T) {
	rt, _, log := newRuntimeWithDoc(t)
	mustRun(t, rt, `console.log(document.getElementById("nope") === null);`)
	if got := strings.TrimSpace(log.String()); got != "true" {
		t.Fatalf("got %q, want true", got)
	}
}

func TestDOMInnerHTMLSetterUpdatesGoDOM(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("foo")
	doc.AppendChild(el)
	mustRun(t, rt, `document.getElementById("foo").innerHTML = "<b>hi</b>";`)
	if got := el.GetInnerHTML(); got != "<b>hi</b>" {
		t.Fatalf("Go DOM innerHTML = %q, want <b>hi</b>", got)
	}
	// Reading back from JS returns the same value.
	log := &jsc.BufferLogger{}
	// Swap the logger so we can inspect a fresh console.log.
	rt2, doc2 := jsc.NewInterpreter(), dom.NewDocument()
	rt2.SetupGlobal(log)
	RegisterDOMBindings(rt2, doc2)
	el2 := doc2.CreateElement("p")
	el2.SetId("p")
	doc2.AppendChild(el2)
	el2.SetInnerHTML("<i>x</i>")
	mustRun(t, rt2, `console.log(document.getElementById("p").innerHTML);`)
	if got := strings.TrimSpace(log.String()); got != "<i>x</i>" {
		t.Fatalf("JS read innerHTML = %q, want <i>x</i>", got)
	}
}

func TestDOMSetAttributeAndReadBack(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	el := doc.CreateElement("a")
	el.SetId("link")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("link");
		el.setAttribute("href", "https://example.com");
		console.log(el.getAttribute("href"));
		console.log(el.hasAttribute("href"));
	`)
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if strings.TrimSpace(lines[0]) != "https://example.com" {
		t.Fatalf("href = %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "true" {
		t.Fatalf("hasAttribute = %q, want true", lines[1])
	}
	if got := el.GetAttribute("href"); got != "https://example.com" {
		t.Fatalf("Go DOM href = %q", got)
	}
}

func TestDOMCreateElementAppendChild(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("section")
	root.SetId("root")
	doc.AppendChild(root)
	mustRun(t, rt, `
		var root = document.getElementById("root");
		var child = document.createElement("span");
		child.setAttribute("class", "item");
		root.appendChild(child);
		console.log(root.outerHTML);
	`)
	out := strings.TrimSpace(log.String())
	// Expected: <section id="root"><span class="item"></span></section>
	if !strings.Contains(out, "<section") || !strings.Contains(out, `id="root"`) {
		t.Fatalf("outerHTML missing section root: %q", out)
	}
	if !strings.Contains(out, `<span class="item">`) {
		t.Fatalf("outerHTML missing span child: %q", out)
	}
	// Confirm on the Go side too.
	if len(root.ChildNodes()) != 1 {
		t.Fatalf("Go DOM child count = %d, want 1", len(root.ChildNodes()))
	}
}

func TestDOMIdAndClassNameAccessors(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("x")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("x");
		el.className = "btn primary";
		el.id = "y";
	`)
	if got := el.GetClassName(); got != "btn primary" {
		t.Fatalf("className = %q", got)
	}
	if got := el.GetId(); got != "y" {
		t.Fatalf("id = %q, want y", got)
	}
}

func TestDOMCreateTextNode(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("p")
	root.SetId("p")
	doc.AppendChild(root)
	mustRun(t, rt, `
		var t = document.createTextNode("hello");
		document.getElementById("p").appendChild(t);
		console.log(document.getElementById("p").textContent);
	`)
	if got := strings.TrimSpace(log.String()); got != "hello" {
		t.Fatalf("textContent = %q, want hello", got)
	}
	if got := root.TextContent(); got != "hello" {
		t.Fatalf("Go DOM textContent = %q", got)
	}
}

func TestDOMAddEventListenerInvokedFromGo(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	btn := doc.CreateElement("button")
	btn.SetId("btn")
	doc.AppendChild(btn)
	// Register a click handler from JS that logs the event type.
	mustRun(t, rt, `
		document.getElementById("btn").addEventListener("click", function(e) {
			console.log("clicked:" + e.type);
		});
	`)
	// Dispatch the event from Go; the JS handler should run and log.
	ev := dom.NewMouseEvent("click", true, true, false)
	btn.DispatchEvent(ev)
	if got := strings.TrimSpace(log.String()); got != "clicked:click" {
		t.Fatalf("got %q, want 'clicked:click'", got)
	}
}

func TestDOMAddEventListenerMouseEventFields(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	btn := doc.CreateElement("button")
	btn.SetId("btn")
	doc.AppendChild(btn)
	mustRun(t, rt, `
		document.getElementById("btn").addEventListener("mousedown", function(e) {
			console.log(e.clientX + "," + e.clientY);
		});
	`)
	me := dom.NewMouseEventFromInit("mousedown", dom.MouseEventInit{
		EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
		ClientX:   12, ClientY: 34,
	})
	btn.DispatchEvent(me)
	if got := strings.TrimSpace(log.String()); got != "12,34" {
		t.Fatalf("got %q, want 12,34", got)
	}
}

func TestDOMDispatchEventFromJS(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	btn := doc.CreateElement("button")
	btn.SetId("btn")
	doc.AppendChild(btn)
	mustRun(t, rt, `
		var got = false;
		document.getElementById("btn").addEventListener("ping", function(e) {
			got = e.bubbles;
		});
		document.getElementById("btn").dispatchEvent({ type: "ping", bubbles: true });
		console.log(got);
	`)
	if got := strings.TrimSpace(log.String()); got != "true" {
		t.Fatalf("got %q, want true", got)
	}
}

func TestDOMRemoveEventListener(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	btn := doc.CreateElement("button")
	btn.SetId("btn")
	doc.AppendChild(btn)
	mustRun(t, rt, `
		var handler = function(e) { console.log("hit"); };
		var el = document.getElementById("btn");
		el.addEventListener("x", handler);
		el.removeEventListener("x", handler);
		el.dispatchEvent({ type: "x", bubbles: false });
	`)
	if got := strings.TrimSpace(log.String()); got != "" {
		t.Fatalf("handler should not fire after removeEventListener; got %q", got)
	}
}

func TestDOMGetElementsByTagName(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	root := doc.CreateElement("ul")
	doc.AppendChild(root)
	for i := 0; i < 3; i++ {
		li := doc.CreateElement("li")
		root.AppendChild(li)
	}
	mustRun(t, rt, `
		var items = document.getElementsByTagName("li");
		console.log(items.length);
	`)
	if got := strings.TrimSpace(log.String()); got != "3" {
		t.Fatalf("got %q, want 3", got)
	}
}

func TestElementWrapperStructExposesBothHalves(t *testing.T) {
	rt := jsc.NewInterpreter()
	rt.SetupGlobal(nil)
	doc := dom.NewDocument()
	RegisterDOMBindings(rt, doc)
	el := doc.CreateElement("div")
	w := &ElementWrapper{DOM: el, JS: wrapElement(rt, el)}
	if w.DOM == nil || w.JS == nil {
		t.Fatalf("wrapper halves should be set")
	}
	if w.JS.Accessor("innerHTML") == nil {
		t.Fatalf("innerHTML accessor should be installed")
	}
}
