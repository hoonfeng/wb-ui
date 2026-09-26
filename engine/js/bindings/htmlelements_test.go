package bindings

import (
	"testing"
)

// TestHTMLElementConstructorsExist 验证 HTML 规范 §4 的每个元素接口都有全局构造器。
//
// 失败模式回顾：构造器缺失时 `x instanceof HTMLIFrameElement` **抛 TypeError**
//（右操作数 undefined），而不是返回 false —— React 19 commit 阶段的
// getActiveElementDeep 正因此中断，页面静默空白。
func TestHTMLElementConstructorsExist(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	for _, itf := range htmlElementInterfaces {
		mustRun(t, rt, `if (typeof `+itf.name+` !== "function") throw new Error("missing global ctor: `+itf.name+`");`)
		// React 通过 window 取构造器（containerInfo.HTMLIFrameElement，containerInfo 为 window）。
		mustRun(t, rt, `if (typeof window.`+itf.name+` !== "function") throw new Error("missing window.`+itf.name+`");`)
	}
	// 媒体元素接口（media_element.go 注册）同样必须存在。
	for _, n := range []string{"HTMLMediaElement", "HTMLVideoElement", "HTMLAudioElement"} {
		mustRun(t, rt, `if (typeof window.`+n+` !== "function") throw new Error("missing window.`+n+`");`)
	}
}

// TestReactGetActiveElementDeepPattern 精确复现 React 19 的判定式。
// 修复前：`el instanceof window.HTMLIFrameElement` 抛
// TypeError: Value is not an object: undefined。
func TestReactGetActiveElementDeepPattern(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var containerInfo = window;
		var el = document.createElement("div");
		var isFrame;
		try {
			isFrame = el instanceof containerInfo.HTMLIFrameElement;
		} catch (e) {
			throw new Error("React getActiveElementDeep 路径抛错: " + e.name + ": " + e.message);
		}
		if (isFrame !== false) throw new Error("div 不应是 HTMLIFrameElement，得到 " + isFrame);

		var f = document.createElement("iframe");
		if (!(f instanceof containerInfo.HTMLIFrameElement)) throw new Error("iframe 未 instanceof HTMLIFrameElement");
	`)
}

// TestElementInstanceOfChain 验证元素实例的完整原型链。
func TestElementInstanceOfChain(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var div = document.createElement("div");
		var chain = {
			"HTMLDivElement": div instanceof HTMLDivElement,
			"HTMLElement":    div instanceof HTMLElement,
			"Element":        div instanceof Element,
			"Node":           div instanceof Node,
			"EventTarget":    typeof EventTarget === "function" ? (div instanceof EventTarget) : true
		};
		for (var k in chain) {
			if (chain[k] !== true) throw new Error("原型链断裂: div instanceof " + k + " = " + chain[k]);
		}
		if (div instanceof HTMLIFrameElement) throw new Error("div instanceof HTMLIFrameElement 应为 false");
		if (div instanceof HTMLInputElement) throw new Error("div instanceof HTMLInputElement 应为 false");

		if (div.constructor !== HTMLDivElement) throw new Error("constructor 不是 HTMLDivElement");
		if (div.constructor.name !== "HTMLDivElement") {
			throw new Error("constructor.name = " + div.constructor.name + "（期望 HTMLDivElement）");
		}
	`)
}

// TestTagToInterfaceDispatch 表驱动：每个标签落到正确的接口原型。
func TestTagToInterfaceDispatch(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	cases := []struct{ tag, iface string }{
		{"html", "HTMLHtmlElement"},
		{"head", "HTMLHeadElement"},
		{"body", "HTMLBodyElement"},
		{"div", "HTMLDivElement"},
		{"span", "HTMLSpanElement"},
		{"p", "HTMLParagraphElement"},
		{"h1", "HTMLHeadingElement"},
		{"h6", "HTMLHeadingElement"},
		{"a", "HTMLAnchorElement"},
		{"img", "HTMLImageElement"},
		{"iframe", "HTMLIFrameElement"},
		{"canvas", "HTMLCanvasElement"},
		{"script", "HTMLScriptElement"},
		{"style", "HTMLStyleElement"},
		{"template", "HTMLTemplateElement"},
		{"input", "HTMLInputElement"},
		{"textarea", "HTMLTextAreaElement"},
		{"select", "HTMLSelectElement"},
		{"option", "HTMLOptionElement"},
		{"form", "HTMLFormElement"},
		{"button", "HTMLButtonElement"},
		{"label", "HTMLLabelElement"},
		{"table", "HTMLTableElement"},
		{"thead", "HTMLTableSectionElement"},
		{"tbody", "HTMLTableSectionElement"},
		{"tr", "HTMLTableRowElement"},
		{"td", "HTMLTableCellElement"},
		{"th", "HTMLTableCellElement"},
		{"col", "HTMLTableColElement"},
		{"ul", "HTMLUListElement"},
		{"li", "HTMLLIElement"},
		{"details", "HTMLDetailsElement"},
		{"dialog", "HTMLDialogElement"},
		{"slot", "HTMLSlotElement"},
		{"svg", "SVGElement"},
		{"circle", "SVGElement"},
	}
	for _, c := range cases {
		mustRun(t, rt, `var e = document.createElement("`+c.tag+`"); `+
			`if (!(e instanceof `+c.iface+`)) throw new Error("<`+c.tag+`> 未 instanceof `+c.iface+`");`)
	}
}

// TestUnknownAndCustomElements 未知标签/自定义元素的兜底语义（浏览器行为）。
func TestUnknownAndCustomElements(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		// 非自定义的未知标签 → HTMLUnknownElement
		var foo = document.createElement("foo");
		if (!(foo instanceof HTMLUnknownElement)) throw new Error("<foo> 未 instanceof HTMLUnknownElement");
		if (!(foo instanceof HTMLElement)) throw new Error("<foo> 未 instanceof HTMLElement");

		// 自定义元素（名称含 '-'）→ HTMLElement（浏览器里不是 HTMLUnknownElement）
		var w = document.createElement("my-widget");
		if (!(w instanceof HTMLElement)) throw new Error("<my-widget> 未 instanceof HTMLElement");
	`)
}

// TestMediaElementsUnaffected 回归：媒体元素原型链未被 HTML 接口分派覆盖。
func TestMediaElementsUnaffected(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var v = document.createElement("video");
		if (!(v instanceof HTMLVideoElement)) throw new Error("video 未 instanceof HTMLVideoElement");
		if (!(v instanceof HTMLMediaElement)) throw new Error("video 未 instanceof HTMLMediaElement");
		if (!(v instanceof HTMLElement)) throw new Error("video 未 instanceof HTMLElement");
		if (v instanceof HTMLDivElement) throw new Error("video instanceof HTMLDivElement 应为 false");

		var a = document.createElement("audio");
		if (!(a instanceof HTMLAudioElement)) throw new Error("audio 未 instanceof HTMLAudioElement");
	`)
}

// TestElementPropertiesStillWork 回归：改原型分派后元素方法与惰性属性仍可达。
func TestElementPropertiesStillWork(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("probe")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("probe");
		if (!el) throw new Error("getElementById 返回空");
		el.setAttribute("data-x", "1");
		if (el.getAttribute("data-x") !== "1") throw new Error("setAttribute/getAttribute 失效");
		el.appendChild(document.createTextNode("t"));
		if (el.childNodes.length !== 1) throw new Error("appendChild 失效");
		if (el.nodeType !== 1) throw new Error("nodeType 失效");
		if (typeof el.addEventListener !== "function") throw new Error("addEventListener 失效");
	`)
}

// TestDiagConstructorChain 临时诊断：查清 div.constructor 为何不是 HTMLDivElement。
func TestDiagConstructorChain(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	_, err := rt.Run(`
		(function(){
			var div = document.createElement("div");
			var out = {
				isInstance: div instanceof HTMLDivElement,
				ctorType: typeof div.constructor,
				ctorName: (div.constructor && div.constructor.name) || "(none)",
				protoIsDivProto: Object.getPrototypeOf(div) === HTMLDivElement.prototype,
				protoCtorIsCtor: HTMLDivElement.prototype.constructor === HTMLDivElement,
				hasOwnCtor: Object.prototype.hasOwnProperty.call(HTMLDivElement.prototype, "constructor"),
				ownNames: Object.getOwnPropertyNames(HTMLDivElement.prototype).join(","),
				protoOfProto: Object.getPrototypeOf(HTMLDivElement.prototype) === HTMLElement.prototype,
				ctorIsGlobal: HTMLDivElement === window.HTMLDivElement
			};
			throw new Error("DIAG:" + JSON.stringify(out));
		})()
	`)
	t.Logf("诊断: %v", err)
}
