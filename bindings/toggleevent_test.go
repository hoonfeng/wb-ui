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
