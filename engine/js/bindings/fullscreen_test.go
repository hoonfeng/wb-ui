// Tests for the Fullscreen API bindings (HTML §4.11.6): Element.requestFullscreen,
// document.exitFullscreen, document.fullscreenElement / fullscreenEnabled, the
// :fullscreen pseudo-class match, and fullscreenchange event ordering.

package bindings

import (
	"testing"

	"wb-ui/engine/dom"
)

// newHTMLBodyFixture 建立真实的 html > body 骨架并返回 body。测试夹具必须这样
// 搭：document.querySelectorAll 从 documentElement 向下遍历，若直接把元素挂到
// document 下，documentElement 会变成第一个元素，同级元素便不在它的子树里。
func newHTMLBodyFixture(doc *dom.Document) *dom.Element {
	html := doc.CreateElement("html")
	doc.AppendChild(html)
	body := doc.CreateElement("body")
	html.AppendChild(body)
	return body
}

func TestFullscreenRequestAndExit(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	box := doc.CreateElement("div")
	box.SetId("box")
	body.AppendChild(box)
	inner := doc.CreateElement("span")
	inner.SetId("inner")
	box.AppendChild(inner)
	sib := doc.CreateElement("div")
	sib.SetId("sib")
	body.AppendChild(sib)

	// 无全屏元素时的静态契约。
	mustRun(t, rt, `
		{
			if (document.fullscreenElement !== null) throw new Error("初始 fullscreenElement 应为 null");
			if (document.fullscreenEnabled !== true) throw new Error("fullscreenEnabled 应为 true");
			if (typeof document.exitFullscreen !== "function") throw new Error("document.exitFullscreen 缺失");
			const box = document.getElementById("box");
			if (typeof box.requestFullscreen !== "function") throw new Error("Element.requestFullscreen 缺失");
			if (!("requestFullscreen" in box)) throw new Error("'requestFullscreen' in box 应为 true");
			if (box.matches(":fullscreen")) throw new Error("未全屏时不应匹配 :fullscreen");
		}
	`)

	// 进入全屏：Promise、同步状态、:fullscreen 匹配、事件顺序（元素先 document 后）。
	mustRun(t, rt, `
		{
			window.__order = [];
			window.__box = document.getElementById("box");
			window.__inner = document.getElementById("inner");
			window.__sib = document.getElementById("sib");
			window.__box.addEventListener("fullscreenchange", function () {
				window.__order.push("element:" + (document.fullscreenElement ? document.fullscreenElement.id : "null"));
			});
			document.addEventListener("fullscreenchange", function () {
				window.__order.push("document:" + (document.fullscreenElement ? document.fullscreenElement.id : "null"));
			});
			window.__p = window.__box.requestFullscreen();
			if (!window.__p || typeof window.__p.then !== "function") {
				throw new Error("requestFullscreen 应返回 Promise，实际 " + typeof window.__p);
			}
			// 规范：全屏元素（及其 top layer 状态）是同步更新的，事件才是异步的。
			if (document.fullscreenElement !== window.__box) throw new Error("fullscreenElement 应同步指向 box");
			if (!document.fullscreenElement.tagName || document.fullscreenElement.tagName.toLowerCase() !== "div") {
				throw new Error("fullscreenElement.tagName 应为 div");
			}
			if (window.__order.length !== 0) throw new Error("fullscreenchange 不应同步派发，实际 " + window.__order);
			if (!window.__box.matches(":fullscreen")) throw new Error("全屏元素应匹配 :fullscreen");
			if (window.__sib.matches(":fullscreen")) throw new Error("兄弟元素不应匹配 :fullscreen");
			if (document.querySelectorAll(":fullscreen").length !== 1) {
				throw new Error("document.querySelectorAll(':fullscreen') 长度应为 1");
			}
			if (document.querySelector(":fullscreen") !== window.__box) {
				throw new Error("querySelector(':fullscreen') 应返回 box");
			}
		}
	`)

	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__order.join(",") !== "element:box,document:box") {
				throw new Error("fullscreenchange 顺序错误: " + window.__order.join(","));
			}
		}
	`)

	// Promise resolve 之后状态仍是全屏（.then 回调应被排入微任务队列）。
	mustRun(t, rt, `{ window.__resolved = false; window.__p.then(function () { window.__resolved = true; }); }`)
	rt.RunJobs()
	drainEventLoop(rt)
	mustRun(t, rt, `{ if (!window.__resolved) throw new Error("requestFullscreen 的 Promise 未 resolve"); }`)

	// 退出全屏：状态清空、事件顺序仍是元素先 document 后（元素为旧的 fullscreen element）。
	mustRun(t, rt, `
		{
			window.__order = [];
			const p2 = document.exitFullscreen();
			if (!p2 || typeof p2.then !== "function") throw new Error("exitFullscreen 应返回 Promise");
			if (document.fullscreenElement !== null) throw new Error("exitFullscreen 后 fullscreenElement 应为 null");
			if (window.__box.matches(":fullscreen")) throw new Error("退出后不应再匹配 :fullscreen");
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__order.join(",") !== "element:null,document:null") {
				throw new Error("退出时事件顺序错误: " + window.__order.join(","));
			}
		}
	`)

	// 没有全屏元素时 exitFullscreen() 必须 reject TypeError。
	mustRun(t, rt, `{ window.__rejected = null; document.exitFullscreen().catch(function (e) { window.__rejected = e && e.name; }); }`)
	rt.RunJobs()
	drainEventLoop(rt)
	mustRun(t, rt, `{ if (window.__rejected !== "TypeError") throw new Error("空状态 exitFullscreen 应 reject TypeError，实际 " + window.__rejected); }`)
}

// TestFullscreenSwitchElement 覆盖「已在全屏时再请求另一个元素」：状态切到新
// 元素，旧元素不再匹配 :fullscreen，事件派发到新元素与 document。
func TestFullscreenSwitchElement(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	box := doc.CreateElement("div")
	box.SetId("box")
	body.AppendChild(box)
	other := doc.CreateElement("div")
	other.SetId("other")
	body.AppendChild(other)

	mustRun(t, rt, `
		{
			window.__box = document.getElementById("box");
			window.__other = document.getElementById("other");
			window.__order = [];
			window.__other.addEventListener("fullscreenchange", function () {
				window.__order.push("element:" + (document.fullscreenElement ? document.fullscreenElement.id : "null"));
			});
			document.addEventListener("fullscreenchange", function () {
				window.__order.push("document:" + (document.fullscreenElement ? document.fullscreenElement.id : "null"));
			});
			window.__box.requestFullscreen();
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			window.__order = [];
			window.__other.requestFullscreen();
			if (document.fullscreenElement !== window.__other) throw new Error("fullscreenElement 应切到 other");
			if (window.__box.matches(":fullscreen")) throw new Error("旧全屏元素不应再匹配 :fullscreen");
			if (!window.__other.matches(":fullscreen")) throw new Error("新全屏元素应匹配 :fullscreen");
			const fs = document.querySelectorAll(":fullscreen");
			if (fs.length !== 1) {
				throw new Error("同一时刻只能有一个全屏元素，实际 len=" + fs.length + " ids=" +
					Array.prototype.map.call(fs, function (e) { return e.id; }).join("|"));
			}
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__order.join(",") !== "element:other,document:other") {
				throw new Error("切换全屏元素时事件顺序错误: " + window.__order.join(","));
			}
		}
	`)
}

// TestFullscreenRepeatedRequest 重复请求同一元素不应重复派发事件（状态未变）。
func TestFullscreenRepeatedRequest(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	box := doc.CreateElement("div")
	box.SetId("box")
	body.AppendChild(box)

	mustRun(t, rt, `
		{
			window.__count = 0;
			window.__box = document.getElementById("box");
			document.addEventListener("fullscreenchange", function () { window.__count++; });
			window.__box.requestFullscreen();
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			const first = window.__count;
			window.__box.requestFullscreen();
			if (window.__count !== first) throw new Error("重复请求不应同步再派发事件");
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__count !== 1) throw new Error("同一元素重复请求应只派发 1 次 fullscreenchange，实际 " + window.__count);
		}
	`)
}

// TestFullscreenDisabledWhenNotInDocument 元素不在文档中时 requestFullscreen
// 必须 reject TypeError（可判定的失败条件）。
func TestFullscreenDisabledWhenNotInDocument(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)

	mustRun(t, rt, `
		{
			const orphan = document.createElement("div");
			window.__rejected = null;
			orphan.requestFullscreen().catch(function (e) { window.__rejected = e && e.name; });
		}
	`)
	rt.RunJobs()
	drainEventLoop(rt)
	mustRun(t, rt, `{ if (window.__rejected !== "TypeError") throw new Error("游离元素应 reject TypeError，实际 " + window.__rejected); }`)
}

// TestFullscreenHookFires 宿主钩子按状态变化触发（样式失效链 + 宿主回调）。
func TestFullscreenHookFires(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	box := doc.CreateElement("div")
	box.SetId("box")
	body.AppendChild(box)

	var styleInvalidated, hostNotified int
	prevStyle, prevHost := OnClassChanged, OnFullscreenChanged
	OnClassChanged = func(el *dom.Element) {
		if el == box {
			styleInvalidated++
		}
	}
	OnFullscreenChanged = func(el *dom.Element) {
		if el == box {
			hostNotified++
		}
	}
	defer func() { OnClassChanged, OnFullscreenChanged = prevStyle, prevHost }()

	mustRun(t, rt, `{ document.getElementById("box").requestFullscreen(); }`)
	if styleInvalidated != 1 {
		t.Fatalf("OnClassChanged 调用次数 = %d，want 1", styleInvalidated)
	}
	if hostNotified != 1 {
		t.Fatalf("OnFullscreenChanged 调用次数 = %d，want 1", hostNotified)
	}

	styleInvalidated, hostNotified = 0, 0
	mustRun(t, rt, `{ document.exitFullscreen(); }`)
	if styleInvalidated != 1 {
		t.Fatalf("退出时 OnClassChanged 调用次数 = %d，want 1", styleInvalidated)
	}
	if hostNotified != 1 {
		t.Fatalf("退出时 OnFullscreenChanged 调用次数 = %d，want 1", hostNotified)
	}
}
