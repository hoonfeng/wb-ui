// Tests for the <dialog> JS interface: open reflection, show() / showModal() /
// close(), returnValue, the :modal / :open pseudo-class link, and the async
// toggle / close events.

package bindings

import (
	"testing"

	"wb-ui/engine/dom"
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

	// open 属性设置器：**纯反射**（HTML §4.11.6）——设置/移除属性本身不改变
	// 模态状态、不派发事件。由 showModal() 打开的 dialog 在 open 属性被移除后
	// 仍处于模态（文档仍被阻塞、:modal 仍匹配、::backdrop 仍在），只是不再显示；
	// 规范因此建议作者用 close() 而不是移除属性。残留的模态状态只能靠
	// close()（元素在文档中时）或把元素从文档移除来清理。
	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			d.close(); // 上一块用 show() 打开过：先复位再验证 open 设置器
			if (d.matches(":modal")) throw new Error("前置状态错误：close 后不应为模态");
			d.showModal();
			if (!d.matches(":modal")) throw new Error("前置状态错误");
			d.open = false;
			if (d.open !== false) throw new Error("open = false 后 open 应为 false");
			if (d.hasAttribute("open")) throw new Error("open = false 应移除 open 属性");
			if (!d.matches(":closed")) throw new Error("无 open 属性时应匹配 :closed");
			if (!d.matches(":modal")) throw new Error("移除 open 属性不退出模态状态（规范），应仍匹配 :modal");
			d.open = true;
			if (d.open !== true || !d.hasAttribute("open")) throw new Error("open = true 应设置 open 属性");
			if (!d.matches(":open")) throw new Error("有 open 属性时应匹配 :open");
			// open 设置器不进入/退出模态状态：仍是模态（也说明它没被重置）。
			if (!d.matches(":modal")) throw new Error("open = true 不应重置模态状态");
			d.close();
			if (d.open !== false) throw new Error("close 后 open 应为 false");
			if (d.matches(":modal")) throw new Error("close 后应退出模态状态");
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
			// HTML §4.11.6 的关闭算法：先排队 toggle，再排队 close（同一批
			// 任务内按序派发）——所以是 toggle 在前、close 在后。
			if (window.__ev.join(",") !== "toggle:open,toggle:closed,close") {
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

	// open 属性被切换时排队派发 toggle（HTML §4.11.4 的 details toggle 事件
	// 任务）：异步、不可取消、打开与关闭都派发。
	drainEventLoop(rt) // 先清掉上面两块切换 open 排下的 toggle（那时还没监听器）
	mustRun(t, rt, `
		{
			window.__det = [];
			const d = document.getElementById("det");
			d.addEventListener("toggle", function () { window.__det.push(d.open ? "open" : "closed"); });
			d.open = true;
			if (window.__det.length !== 0) throw new Error("toggle 不应同步派发，实际 " + window.__det);
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__det.join(",") !== "open") throw new Error("打开的事件序列错误: " + window.__det.join(","));
			document.getElementById("det").open = false;
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__det.join(",") !== "open,closed") throw new Error("关闭的事件序列错误: " + window.__det.join(","));
		}
	`)
}
