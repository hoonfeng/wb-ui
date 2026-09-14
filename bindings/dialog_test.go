// Tests for the <dialog> JS interface: open reflection, show() / showModal() /
// close(), returnValue, the :modal / :open pseudo-class link, and the async
// toggle / close events.

package bindings

import (
	"testing"

	"wb-ui/dom"
)

func TestDialogShowModalClose(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)

	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			if (typeof d.show !== "function") throw new Error("show 不是函数: " + typeof d.show);
			if (typeof d.showModal !== "function") throw new Error("showModal 不是函数: " + typeof d.showModal);
			if (typeof d.close !== "function") throw new Error("close 不是函数: " + typeof d.close);
			if (!("showModal" in d)) throw new Error("'showModal' in dialog 应为 true");
			if (d.open !== false) throw new Error("初始 open 应为 false");
			if (d.returnValue !== "") throw new Error("初始 returnValue 应为空串");
			if (d.matches(":modal")) throw new Error("未打开时不应匹配 :modal");
			if (d.matches(":open")) throw new Error("未打开时不应匹配 :open");

			d.showModal();
			if (d.open !== true) throw new Error("showModal 后 open 应为 true");
			if (!d.hasAttribute("open")) throw new Error("showModal 应设置 open 属性");
			if (!d.matches(":modal")) throw new Error("showModal 打开的 dialog 应匹配 :modal");
			if (!d.matches(":open")) throw new Error("打开的 dialog 应匹配 :open");
			if (document.querySelectorAll(":modal").length !== 1) throw new Error(":modal 应只匹配 1 个元素");

			d.close("ok");
			if (d.open !== false) throw new Error("close 后 open 应为 false");
			if (d.hasAttribute("open")) throw new Error("close 应移除 open 属性");
			if (d.matches(":modal")) throw new Error("close 后不应匹配 :modal");
			if (d.matches(":open")) throw new Error("close 后不应匹配 :open");
			if (d.returnValue !== "ok") throw new Error("close('ok') 后 returnValue 应为 ok，实际 " + d.returnValue);
		}
	`)

	// show() 是非模态：不匹配 :modal，但仍匹配 :open。
	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			d.show();
			if (d.open !== true) throw new Error("show 后 open 应为 true");
			if (!d.matches(":open")) throw new Error("show 打开的 dialog 应匹配 :open");
			if (d.matches(":modal")) throw new Error("show 打开的是非模态 dialog，不应匹配 :modal");
			if (document.querySelectorAll(":modal").length !== 0) throw new Error("非模态 dialog 不应被 :modal 命中");
		}
	`)

	// 已打开时 show()/showModal() 按规范抛错（InvalidStateError；本引擎用
	// TypeError 兜底）。
	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			let err = null;
			try { d.showModal(); } catch (e) { err = e && e.name; }
			if (err !== "TypeError") throw new Error("已打开时 showModal() 应抛错，实际 " + err);
		}
	`)

	// open 属性设置器：置 true/false 与 show/close 等效（关闭时同样清模态状态）。
	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			d.close(); // 上一块用 show() 打开过：先复位再验证 open 设置器
			d.showModal();
			if (!d.matches(":modal")) throw new Error("前置状态错误");
			d.open = false;
			if (d.open !== false) throw new Error("open = false 后 open 应为 false");
			if (d.hasAttribute("open")) throw new Error("open = false 应移除 open 属性");
			if (d.matches(":modal")) throw new Error("open = false 后不应匹配 :modal");
			d.open = true;
			if (d.open !== true || !d.hasAttribute("open")) throw new Error("open = true 应设置 open 属性");
		}
	`)
}

func TestDialogToggleAndCloseEvents(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)

	mustRun(t, rt, `
		{
			window.__ev = [];
			const d = document.getElementById("dlg");
			d.addEventListener("toggle", function () { window.__ev.push("toggle:" + (d.open ? "open" : "closed")); });
			d.addEventListener("close", function () { window.__ev.push("close"); });
			d.showModal();
			if (window.__ev.length !== 0) throw new Error("toggle 不应同步派发，实际 " + window.__ev);
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `{ if (window.__ev.join(",") !== "toggle:open") throw new Error("打开的事件序列错误: " + window.__ev.join(",")); }`)

	mustRun(t, rt, `{ document.getElementById("dlg").close(); }`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__ev.join(",") !== "toggle:open,close,toggle:closed") {
				throw new Error("关闭的事件序列错误: " + window.__ev.join(","));
			}
		}
	`)
}

// TestDialogInvalidatesStyle 状态迁移必须走样式失效链（:modal / dialog[open]
// 的 UA 定位都依赖它）。
func TestDialogInvalidatesStyle(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)

	var invalidated int
	prev := OnClassChanged
	OnClassChanged = func(el *dom.Element) {
		if el == dlg {
			invalidated++
		}
	}
	defer func() { OnClassChanged = prev }()

	mustRun(t, rt, `{ document.getElementById("dlg").showModal(); }`)
	if invalidated != 1 {
		t.Fatalf("showModal 后 OnClassChanged 调用次数 = %d，want 1", invalidated)
	}
	mustRun(t, rt, `{ document.getElementById("dlg").close(); }`)
	if invalidated != 2 {
		t.Fatalf("close 后 OnClassChanged 调用次数 = %d，want 2", invalidated)
	}
}

// TestDetailsOpenProperty <details> 与 <dialog> 共用 open 属性反射。
func TestDetailsOpenProperty(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	det := doc.CreateElement("details")
	det.SetId("det")
	body.AppendChild(det)

	mustRun(t, rt, `
		{
			const d = document.getElementById("det");
			if (d.open !== false) throw new Error("初始 open 应为 false");
			if (d.matches(":open")) throw new Error("未打开时不应匹配 :open");
			d.open = true;
			if (d.open !== true || !d.hasAttribute("open")) throw new Error("open = true 应设置 open 属性");
			if (!d.matches(":open")) throw new Error("打开的 details 应匹配 :open");
			d.open = false;
			if (d.hasAttribute("open")) throw new Error("open = false 应移除 open 属性");
			if (!d.matches(":closed")) throw new Error("关闭的 details 应匹配 :closed");
		}
	`)
}
