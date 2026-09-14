// Translation of: Source/WebCore/bindings/js/JSDocumentCustom.cpp (test surface)
//                  Source/WebCore/bindings/js/JSElementCustom.cpp (test surface)
//                  Source/WebCore/bindings/js/JSEventListener.cpp (test surface)
// Tests for the DOM bindings: document/element wrappers, accessor properties, and
// JS-registered event listeners invoked from Go.

package bindings

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
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

func TestDOMAddEventListenerAsMethodCall(t *testing.T) {
	// Simulates Vue 3's patchEvent calling el2.addEventListener(event2, handler, options)
	// as a method call: el2.addEventListener(...)
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("test-el")
	doc.AppendChild(el)

	// Verify addEventListener exists as a function on the element
	mustRun(t, rt, `
		var el2 = document.getElementById("test-el");
		if (typeof el2.addEventListener !== "function") throw new Error("addEventListener not a function: " + typeof el2.addEventListener);
		if (typeof el2.removeEventListener !== "function") throw new Error("removeEventListener not a function");
	`)

	// Test calling through method call pattern — simulating Vue's patchEvent
	mustRun(t, rt, `
		var el2 = document.getElementById("test-el");
		var called = false;
		el2.addEventListener("test", function() { called = true; }, false);
		// Call addEventListener again with different options to verify repeated calls work
		el2.addEventListener("test2", function() {}, {capture:false});
	`)

	// Test calling inside a closure (mimicking Vue's patched event wrapper)
	mustRun(t, rt, `
		var el2 = document.getElementById("test-el");
		var called2 = false;
		// This matches Vue 3's patchEvent pattern:
		// el2.addEventListener(event2, handler, options) inside a closure
		(function(el, evt, handler, opts) {
			el.addEventListener(evt, handler, opts);
		})(el2, "test-closure", function() { called2 = true; }, false);
		if (typeof el2.addEventListener !== "function") throw new Error("FAIL: addEventListener lost after call");
	`)

	// Test removeEventListener too
	mustRun(t, rt, `
		var el2 = document.getElementById("test-el");
		var called3 = false;
		var handler3 = function() { called3 = true; };
		el2.addEventListener("test3", handler3, false);
		el2.removeEventListener("test3", handler3, false);
	`)
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

// TestElementPrototypeStandardLayout 验证 attribute 方法定义在 Element.prototype
// 上（标准 DOM 设计）而非每个实例的自有属性，且实例经原型链继承、instanceof 正常：
//   - Element.prototype.setAttribute 等为函数
//   - el.hasOwnProperty("setAttribute") === false（实例无自有遮蔽）
//   - el.setAttribute 可调用（原型链继承）
//   - el instanceof Element 成立（原型链完整）
// 注：instanceof HTMLElement/SVGElement 因 dom.Element 未区分 HTML/SVG 命名空间
// （tag 名扁平存储），所有元素共用 Element.prototype——属已知架构简化，不在此验证。
func TestElementPrototypeStandardLayout(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("x")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("x");
		console.log(typeof Element.prototype.setAttribute);
		console.log(typeof Element.prototype.getAttribute);
		console.log(typeof Element.prototype.removeAttribute);
		console.log(el.hasOwnProperty("setAttribute"));
		console.log(el instanceof Element);
		el.setAttribute("data-v-test", "1");
		console.log(el.getAttribute("data-v-test"));
	`)
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	want := []string{"function", "function", "function", "false", "true", "1"}
	if len(lines) < len(want) {
		t.Fatalf("output lines = %d, want >= %d: %q", len(lines), len(want), log.String())
	}
	for i, w := range want {
		if got := strings.TrimSpace(lines[i]); got != w {
			t.Fatalf("line %d = %q, want %q", i+1, got, w)
		}
	}
	if got := el.GetAttribute("data-v-test"); got != "1" {
		t.Fatalf("Go DOM data-v-test = %q, want 1", got)
	}
}

// TestElementPrototypeHookCapture 模拟探针/框架在 Element.prototype 上
// monkey-patch setAttribute（如 data-v 属性追踪、测试工具 hook），验证：
//   - patch 后实例调用 el.setAttribute 走的是被替换的方法（可捕获）
//   - 原始方法经 orig.apply(this, ...) 仍能正常工作（不破坏 DOM 写入）
func TestElementPrototypeHookCapture(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("x")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var origSA = Element.prototype.setAttribute;
		window.__slog = [];
		Element.prototype.setAttribute = function(n, v) {
			window.__slog.push(String(n));
			return origSA.apply(this, arguments);
		};
		var el = document.getElementById("x");
		el.setAttribute("data-v-hooktest", "");
		el.setAttribute("class", "item");
		console.log(JSON.stringify(window.__slog));
	`)
	got := strings.TrimSpace(log.String())
	if want := `["data-v-hooktest","class"]`; got != want {
		t.Fatalf("hook capture = %s, want %s", got, want)
	}
	if g := el.GetAttribute("data-v-hooktest"); g != "" {
		t.Fatalf("Go DOM data-v-hooktest = %q, want empty", g)
	}
	if g := el.GetAttribute("class"); g != "item" {
		t.Fatalf("Go DOM class = %q, want item", g)
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
	if _, ok := w.JS.GetByKey("innerHTML"); !ok {
		t.Fatalf("innerHTML accessor should be installed")
	}
	if _, ok := w.JS.GetByKey("innerHTML"); !ok {
		t.Fatalf("innerHTML accessor should be installed")
	}
}

// TestDOMErrorPropagation verifies that a GoCallback registered via DOM bindings
// that returns an error propagates to JS as a catchable exception.
func TestDOMErrorPropagation(t *testing.T) {
	rt, doc, log := newRuntimeWithDoc(t)
	// Register a Go callback on the document that always errors.
	RegisterGoFunction(rt, "thrower", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.Undefined(), &customError{msg: "dom error"}
	})
	_ = doc
	mustRun(t, rt, `
		try {
			go.thrower();
			console.log("no-error");
		} catch(e) {
			console.log("caught:" + e);
		}
	`)
	if got := strings.TrimSpace(log.String()); got != "caught:GoError: dom error" {
		t.Fatalf("got %q, want 'caught:GoError: dom error'", got)
	}
}

// TestInsertTextAtSelectionSyncsSelectionFields 验证 InsertTextAtSelection
// 插入文本后，window.getSelection() 返回的 Selection 单例（selObj）的
// anchorNode/anchorOffset/focusNode/focusOffset 字段被同步到「插入文本
// 之后」的位置——CM6 的 DOMObserver.readSelectionChange 直接读这些字段
// （而非 getRangeAt），不同步则读到旧光标位置 → IME/字符输入后光标
// 不后移（「光标停在插入文字前」根因）。
func TestInsertTextAtSelectionSyncsSelectionFields(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetAttribute("contenteditable", "true")
	doc.AppendChild(el)
	txt := dom.NewText(doc, "func main")
	if err := el.AppendChild(txt); err != nil {
		t.Fatalf("AppendChild: %v", err)
	}

	// JS 侧 collapse 定位光标到 "func" 后（offset=4）
	mustRun(t, rt, `
		var el = document.getElementsByTagName("div")[0];
		var tn = el.firstChild;
		window.getSelection().collapse(tn, 4);
	`)
	if len(sstate.ranges) == 0 {
		t.Fatal("collapse did not populate sstate.ranges")
	}

	// Go 侧在光标处插入 "拼"
	if !InsertTextAtSelection("拼") {
		t.Fatal("InsertTextAtSelection returned false")
	}

	// 文本正确插入："func" 分裂为 "func"+" main"，"拼" 插在中间
	if got := el.TextContent(); got != "func拼 main" {
		t.Fatalf("textContent = %q, want %q", got, "func拼 main")
	}

	// ★ 核心断言：selObj 字段被同步到「插入文本之后」的位置
	if sstate.selObj == nil {
		t.Fatal("sstate.selObj is nil")
	}
	ao := int(sstate.selObj.GetStr("anchorOffset").ToNumber())
	// 分裂后子节点：t="func"(idx0), ins="拼"(idx1), tail=" main"(idx2)
	// updateRangeForInsert 把 offset 设为 ins 之后的索引 = 2
	if ao != 2 {
		t.Fatalf("selObj.anchorOffset = %d, want 2 (after inserted text)", ao)
	}
	an := sstate.selObj.GetStr("anchorNode")
	if an.IsNull() || an.IsUndefined() {
		t.Fatal("selObj.anchorNode is null/undefined (should point to parent element)")
	}
	fo := int(sstate.selObj.GetStr("focusOffset").ToNumber())
	if fo != ao {
		t.Fatalf("selObj.focusOffset = %d, want %d", fo, ao)
	}
}