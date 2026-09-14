// Tests for ToggleEvent (HTML §4.11.4 / §4.11.6): the toggle / beforetoggle
// events of <dialog> and <details> carry oldState / newState so a single handler
// can tell "opening" from "closing", and beforetoggle is cancelable (cancelling
// it aborts the transition).

package bindings

import (
	"testing"
)

func TestDialogToggleEventStates(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)

	mustRun(t, rt, `
		{
			window.__ev = [];
			const d = document.getElementById("dlg");
			const rec = (kind) => function (e) {
				window.__ev.push(kind + ":" + e.oldState + ">" + e.newState +
					":" + (e.cancelable ? "cancelable" : "fixed"));
			};
			d.addEventListener("beforetoggle", rec("beforetoggle"));
			d.addEventListener("toggle", rec("toggle"));
			d.addEventListener("close", rec("close"));
			d.showModal();
			if (d.open !== true) throw new Error("showModal 应打开");
		}
	`)
	// beforetoggle 是同步的（打开算法在第一步就派发），toggle 是异步的。
	mustRun(t, rt, `
		{
			const want = "beforetoggle:closed>open:cancelable";
			if (window.__ev.join(",") !== want) {
				throw new Error("同步阶段事件错误: " + window.__ev.join(","));
			}
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			const want = "beforetoggle:closed>open:cancelable,toggle:closed>open:fixed";
			if (window.__ev.join(",") !== want) {
				throw new Error("打开的事件序列错误: " + window.__ev.join(","));
			}
		}
	`)

	mustRun(t, rt, `{ document.getElementById("dlg").close(); }`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			const want = "beforetoggle:closed>open:cancelable,toggle:closed>open:fixed," +
				"beforetoggle:open>closed:cancelable,toggle:open>closed:fixed," +
				"close:undefined>undefined:fixed"; // close 是普通 Event，不带状态
			if (window.__ev.join(",") !== want) {
				throw new Error("关闭的事件序列错误: " + window.__ev.join(","));
			}
		}
	`)
}

// TestDialogBeforeToggleCancelable 取消 beforetoggle（e.preventDefault()）必须
// 中止打开/关闭——页面用这个机制做「未保存则不关闭」的拦截。
func TestDialogBeforeToggleCancelable(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)

	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			window.__toggles = 0;
			window.__block = function (e) { e.preventDefault(); };
			d.addEventListener("beforetoggle", window.__block);
			d.addEventListener("toggle", function () { window.__toggles++; });
			d.showModal();
			if (d.open !== false) throw new Error("被取消的 showModal 不应打开 dialog");
			if (d.matches(":modal")) throw new Error("被取消后不应处于模态状态");
			d.show();
			if (d.open !== false) throw new Error("被取消的 show() 不应打开 dialog");
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			if (window.__toggles !== 0) throw new Error("取消后不应派发 toggle");
		}
	`)

	// 移除拦截后能正常打开；关闭同样可以被拦截。
	mustRun(t, rt, `
		{
			const d = document.getElementById("dlg");
			d.removeEventListener("beforetoggle", window.__block);
			d.showModal();
			if (d.open !== true || !d.matches(":modal")) throw new Error("正常状态应能打开");
			// 重新装上拦截：close() 也应被拦下（beforetoggle 对关闭同样可取消）。
			d.addEventListener("beforetoggle", window.__block);
			d.close();
			if (d.open !== true) throw new Error("被取消的 close 不应关闭");
			if (!d.matches(":modal")) throw new Error("被取消的 close 应保持模态状态");
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `{ if (window.__toggles !== 1) throw new Error("被取消的 close 不应派发 toggle，实际 " + window.__toggles); }`)
}

// TestDetailsToggleEventStates <details> 的 toggle 也带状态（属性反射路径）。
func TestDetailsToggleEventStates(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	det := doc.CreateElement("details")
	det.SetId("det")
	body.AppendChild(det)

	mustRun(t, rt, `
		{
			window.__ev = [];
			const d = document.getElementById("det");
			d.addEventListener("toggle", function (e) {
				window.__ev.push(e.oldState + ">" + e.newState + ":" + (d.open ? "open" : "closed"));
			});
			d.open = true;
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `{ document.getElementById("det").open = false; }`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			const want = "closed>open:open,open>closed:closed";
			if (window.__ev.join(",") !== want) {
				throw new Error("details toggle 状态错误: " + window.__ev.join(","));
			}
		}
	`)
}

// TestToggleEventSourceIsNull 覆盖 ToggleEvent.source（IDL 类型 Element?，HTML
// §4.11.4 / §4.11.6）：<dialog> 与 <details> 的 toggle / beforetoggle 都把
// source 初始化为 null——规范的每一条打开/关闭路径都传 null（close()、
// requestClose()、form method=dialog 提交、close watcher 读的 request close
// source element 槽只被 requestClose() 写过且写 null、details toggle 任务只
// 初始化 oldState/newState）。脚本必须看到 `e.source === null`（不是
// undefined）：MDN 用 `event.source === undefined` 做特性检测，缺字段会被误判成
// 「浏览器不支持」。非 null 的 source 只出现在 popover 的 invoker 场景，本端口
// 尚无 Popover API（见 docs/TECH_DEBT.md）。
func TestToggleEventSourceIsNull(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	dlg := doc.CreateElement("dialog")
	dlg.SetId("dlg")
	body.AppendChild(dlg)
	det := doc.CreateElement("details")
	det.SetId("det")
	body.AppendChild(det)

	mustRun(t, rt, `
		{
			window.__src = [];
			const probe = (e) => e.source === null ? "null"
				: (e.source === undefined ? "undefined" : "element");
			const d = document.getElementById("dlg");
			d.addEventListener("beforetoggle", (e) => { window.__src.push("beforetoggle:" + probe(e)); });
			d.addEventListener("toggle", (e) => { window.__src.push("toggle:" + probe(e)); });
			const t = document.getElementById("det");
			t.addEventListener("toggle", (e) => { window.__src.push("details:" + probe(e)); });
			d.showModal();
		}
	`)
	mustRun(t, rt, `
		{
			if (window.__src.join(",") !== "beforetoggle:null") {
				throw new Error("dialog beforetoggle 的 source 应为 null: " + window.__src.join(","));
			}
		}
	`)
	drainEventLoop(rt)

	mustRun(t, rt, `{ document.getElementById("dlg").close(); }`)
	drainEventLoop(rt)
	mustRun(t, rt, `{ document.getElementById("det").open = true; }`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
			const want = "beforetoggle:null,toggle:null,beforetoggle:null,toggle:null,details:null";
			if (window.__src.join(",") !== want) {
				throw new Error("source 应为 null 的事件序列: " + window.__src.join(","));
			}
		}
	`)
}
