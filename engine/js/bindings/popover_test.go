// Popover API（HTML §6.12）在 JS 层的端到端测试：IDL 表面、枚举属性反射、
// showPopover/hidePopover/togglePopover 的状态迁移与异常、beforetoggle/toggle
// 事件（顺序、oldState/newState/source、可取消）、auto popover 之间的互斥，
// 以及 invoker（popovertarget / commandfor）属性的反射与激活。
//
// 为什么必须走真实脚本（newRuntimeWithDoc + mustRun）：bindings 层只做
// 「IDL ↔ 算法」的翻译，Go 侧直接调 popover.Show 覆盖不到三件最容易出错的事
// ——属性访问器的注册、参数解析（options.source / options.force）、异常值的
// 构造（DOMException 等价对象的 name）。这些正是本文件要锁住的内容。
package bindings

import (
	"testing"

	"wb-ui/engine/popover"
)

// withoutQueueTask 让 popover 的 toggle 事件在本测试内同步派发。本层没有 JS
// 事件循环（QueueTask 由 webkit 按元素归属注入），同步路径才可断言「调用后
// 立刻能看到事件序列」；同时也隔离其它测试可能留下的注入。
func withoutQueueTask(t *testing.T) {
	t.Helper()
	prev := popover.QueueTask
	popover.QueueTask = nil
	t.Cleanup(func() { popover.QueueTask = prev })
}

func TestPopoverIDLSurface(t *testing.T) {
	withoutQueueTask(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	p := doc.CreateElement("div")
	p.SetId("p")
	body.AppendChild(p)

	btn := doc.CreateElement("button")
	btn.SetId("btn")
	body.AppendChild(btn)

	mustRun(t, rt, `
	{
		const p = document.getElementById("p");
		const btn = document.getElementById("btn");

		// 属性/方法必须已注册（in 运算符走 elemKnownProps 表，未登记的属性
		// 即使存在访问器也会让特性检测失败）。
		for (const k of ["popover", "showPopover", "hidePopover", "togglePopover"]) {
			if (!(k in p)) throw new Error("元素上缺少 " + k);
		}
		for (const k of ["popoverTargetElement", "popoverTargetAction",
		                 "command", "commandForElement"]) {
			if (!(k in btn)) throw new Error("button 上缺少 " + k);
		}
		if (typeof p.showPopover !== "function") throw new Error("showPopover 不是函数");
		if (typeof p.hidePopover !== "function") throw new Error("hidePopover 不是函数");
		if (typeof p.togglePopover !== "function") throw new Error("togglePopover 不是函数");
		if (typeof window.ToggleEvent !== "function") throw new Error("window.ToggleEvent 缺失");

		// popover 是「limited to only known values」的枚举属性反射：
		// 缺省值默认 No Popover（null）、空值默认 auto、无效值默认 manual。
		if (p.popover !== null) throw new Error("无属性时 popover 应为 null，得到 " + p.popover);
		p.setAttribute("popover", "");
		if (p.popover !== "auto") throw new Error("popover='' 应为 auto，得到 " + p.popover);
		p.setAttribute("popover", "HiNt");
		if (p.popover !== "hint") throw new Error("popover=HiNt 应为 hint，得到 " + p.popover);
		p.setAttribute("popover", "bogus");
		if (p.popover !== "manual") throw new Error("无效值应为 manual，得到 " + p.popover);
		p.removeAttribute("popover");
		if (p.popover !== null) throw new Error("移除属性后应为 null，得到 " + p.popover);

		// setter 是纯反射（浏览器实测：属性值原样保留，规范化只在 getter）。
		p.popover = "aUtO";
		if (p.getAttribute("popover") !== "aUtO") {
			throw new Error("setter 应原样写入属性，得到 " + p.getAttribute("popover"));
		}
		if (p.popover !== "auto") throw new Error("getter 应规范化，得到 " + p.popover);
		p.popover = "";
		if (p.getAttribute("popover") !== "") {
			throw new Error("popover='' 应写入空串，得到 " + p.getAttribute("popover"));
		}
		p.removeAttribute("popover");

		// ToggleEvent 构造器：属性初值按 IDL 是空串 / null。
		const ev = new ToggleEvent("toggle");
		if (ev.type !== "toggle") throw new Error("ToggleEvent.type 应为 toggle");
		if (ev.oldState !== "" || ev.newState !== "") {
			throw new Error("ToggleEvent 状态初值应为空串，得到 " + ev.oldState + "/" + ev.newState);
		}
		if (ev.source !== null) throw new Error("ToggleEvent.source 初值应为 null");
		if (typeof ev.preventDefault !== "function") throw new Error("ToggleEvent 缺少 preventDefault");

		// 自己构造的 ToggleEvent 经 dispatchEvent 派发时三个字段不能丢。
		window.__got = null;
		p.addEventListener("toggle", function once(e) {
			window.__got = e.oldState + ">" + e.newState + "/" + (e.source ? e.source.id : "null");
		});
		p.dispatchEvent(new ToggleEvent("toggle", {oldState: "closed", newState: "open", source: btn}));
		if (window.__got !== "closed>open/btn") {
			throw new Error("脚本构造的 ToggleEvent 字段丢失：" + window.__got);
		}
	}
	`)
}

func TestPopoverShowHideToggle(t *testing.T) {
	withoutQueueTask(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	plain := doc.CreateElement("div")
	plain.SetId("plain")
	body.AppendChild(plain)

	p := doc.CreateElement("div")
	p.SetId("p")
	p.SetAttribute("popover", "")
	body.AppendChild(p)

	mustRun(t, rt, `
	{
		const plain = document.getElementById("plain");
		const p = document.getElementById("p");

		// No Popover 状态的元素 → NotSupportedError（规范 check popover validity）。
		let name = null;
		try { plain.showPopover(); } catch (e) { name = e && e.name; }
		if (name !== "NotSupportedError") throw new Error("期望 NotSupportedError，得到 " + name);

		// 未连接的元素 → InvalidStateError（同理，不是静默返回）。
		const orphan = document.createElement("div");
		orphan.setAttribute("popover", "");
		name = null;
		try { orphan.showPopover(); } catch (e) { name = e && e.name; }
		if (name !== "InvalidStateError") {
			throw new Error("未连接元素期望 InvalidStateError，得到 " + name);
		}

		// 显示 / 隐藏 + :popover-open 的匹配结果（样式失效链路的判据）。
		if (p.matches(":popover-open")) throw new Error("初始不应匹配 :popover-open");
		p.showPopover();
		if (!p.matches(":popover-open")) throw new Error("showPopover 后应匹配 :popover-open");
		p.showPopover(); // 已显示 → check popover validity 静默返回 false
		if (!p.matches(":popover-open")) throw new Error("重复 showPopover 不应改变状态");
		p.hidePopover();
		if (p.matches(":popover-open")) throw new Error("hidePopover 后不应匹配 :popover-open");
		p.hidePopover(); // 已隐藏 → 同样静默

		// togglePopover 的返回值与 force 语义（boolean 与 {force} 两种形态）。
		if (p.togglePopover() !== true) throw new Error("togglePopover() 应返回 true");
		if (p.togglePopover() !== false) throw new Error("再次 togglePopover() 应返回 false");
		if (p.togglePopover(true) !== true) throw new Error("togglePopover(true) 应返回 true");
		if (p.togglePopover(true) !== true) {
			throw new Error("已显示时 togglePopover(true) 应返回 true（静默）");
		}
		if (p.togglePopover(false) !== false) throw new Error("togglePopover(false) 应返回 false");
		if (p.togglePopover({force: true}) !== true) throw new Error("togglePopover({force:true}) 应返回 true");
		if (p.togglePopover({force: false}) !== false) {
			throw new Error("togglePopover({force:false}) 应返回 false");
		}

		// 三个方法在 No Popover 状态下的异常一致。
		name = null;
		try { plain.togglePopover(); } catch (e) { name = e && e.name; }
		if (name !== "NotSupportedError") throw new Error("togglePopover 期望 NotSupportedError，得到 " + name);
		name = null;
		try { plain.hidePopover(); } catch (e) { name = e && e.name; }
		if (name !== "NotSupportedError") throw new Error("hidePopover 期望 NotSupportedError，得到 " + name);
	}
	`)
}

func TestPopoverEventsAndInvoker(t *testing.T) {
	withoutQueueTask(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	p := doc.CreateElement("div")
	p.SetId("p")
	body.AppendChild(p)

	p2 := doc.CreateElement("div")
	p2.SetId("p2")
	body.AppendChild(p2)

	p3 := doc.CreateElement("div")
	p3.SetId("p3")
	body.AppendChild(p3)

	btn := doc.CreateElement("button")
	btn.SetId("btn")
	body.AppendChild(btn)

	mustRun(t, rt, `
	{
		const p = document.getElementById("p");
		const p2 = document.getElementById("p2");
		const p3 = document.getElementById("p3");
		const btn = document.getElementById("btn");
		p.setAttribute("popover", "auto");
		p2.setAttribute("popover", "auto");
		p3.setAttribute("popover", "auto");

		window.__log = [];
		const record = (prefix) => (e) => {
			window.__log.push(prefix + ":" + e.oldState + ">" + e.newState + "/" +
				(e.source ? e.source.id : "null"));
		};
		p.addEventListener("beforetoggle", record("before"));
		p.addEventListener("toggle", record("toggle"));

		// 打开：beforetoggle 在前，字段 closed>open，脚本调用没有 source。
		p.showPopover();
		const wantOpen = "before:closed>open/null|toggle:closed>open/null";
		if (window.__log.join("|") !== wantOpen) {
			throw new Error("打开事件序列 = " + window.__log.join("|") + "，期望 " + wantOpen);
		}

		// auto 互斥：打开第二个 auto popover 会先关闭第一个（hide 也派发
		// beforetoggle，oldState/newState 正好相反）。
		window.__log = [];
		p2.showPopover();
		if (p.matches(":popover-open")) throw new Error("打开第二个 auto popover 应关闭第一个");
		const wantHide = "before:open>closed/null|toggle:open>closed/null";
		if (window.__log.join("|") !== wantHide) {
			throw new Error("关闭事件序列 = " + window.__log.join("|") + "，期望 " + wantHide);
		}

		// beforetoggle 可取消：preventDefault 后既不进入 showing、也不派发 toggle。
		let canceled = 0;
		let toggles = 0;
		p3.addEventListener("beforetoggle", (e) => { canceled++; e.preventDefault(); });
		p3.addEventListener("toggle", () => { toggles++; });
		p3.showPopover();
		if (canceled !== 1) throw new Error("beforetoggle 派发次数 = " + canceled + "，期望 1");
		if (toggles !== 0) throw new Error("beforetoggle 被取消后不应派发 toggle");
		if (p3.matches(":popover-open")) throw new Error("beforetoggle 被取消后不应打开");

		// ── invoker 属性反射 ──
		if (btn.popoverTargetElement !== null) throw new Error("未设置 popovertarget 时应为 null");
		btn.setAttribute("popovertarget", "p2");
		const t = btn.popoverTargetElement;
		if (t === null || t.id !== "p2") throw new Error("popoverTargetElement 应反射到 #p2");
		if (btn.popoverTargetAction !== "toggle") {
			throw new Error("popoverTargetAction 缺省应为 toggle，得到 " + btn.popoverTargetAction);
		}
		btn.setAttribute("popovertargetaction", "show");
		if (btn.popoverTargetAction !== "show") throw new Error("popoverTargetAction 应为 show");
		btn.setAttribute("popovertargetaction", "bogus");
		if (btn.popoverTargetAction !== "toggle") {
			throw new Error("无效 popovertargetaction 应回退 toggle");
		}
		btn.popoverTargetElement = document.getElementById("p");
		if (btn.getAttribute("popovertarget") !== "p") {
			throw new Error("popoverTargetElement setter 应写入属性");
		}
		btn.popoverTargetElement = null;
		if (btn.hasAttribute("popovertarget")) {
			throw new Error("popoverTargetElement = null 应移除属性");
		}

		// command / commandForElement（button 专有；command 的 getter 做
		// Custom/Known/Unknown 区分，setter 是纯反射）。
		btn.setAttribute("command", "show-popover");
		if (btn.command !== "show-popover") throw new Error("command 应返回已知关键字");
		btn.setAttribute("command", "bogus");
		if (btn.command !== "") throw new Error("未知 command 应返回空串，得到 " + btn.command);
		btn.setAttribute("command", "--custom-thing");
		if (btn.command !== "--custom-thing") throw new Error("自定义命令应原样返回");
		btn.command = "hide-popover";
		if (btn.getAttribute("command") !== "hide-popover") throw new Error("command setter 应反射");
		btn.commandForElement = document.getElementById("p2");
		const cf = btn.commandForElement;
		if (cf === null || cf.id !== "p2") throw new Error("commandForElement 应反射到 #p2");
		btn.removeAttribute("commandfor");
		if (btn.commandForElement !== null) throw new Error("移除 commandfor 后应为 null");
		btn.removeAttribute("command");

		// ── 准备 invoker 激活 ──
		// p2 在 auto 互斥后处于显示状态，先关掉，好让下面的 show 生效。
		if (p2.matches(":popover-open")) p2.hidePopover();
		window.__src = "none";
		p.addEventListener("toggle", (e) => { window.__src = e.source ? e.source.id : "null"; });
		btn.setAttribute("popovertarget", "p");
		btn.setAttribute("popovertargetaction", "show");
	}
	`)

	// invoker 激活：宿主在真实点击的默认行为阶段调用 popover.RunActivation
	// （webkit/app 走的就是这条），source 必须是 invoker 本身 —— 这是本引擎里
	// ToggleEvent.source 唯一非 null 的场景。
	if !popover.RunActivation(btn, btn) {
		t.Fatal("RunActivation 应返回 true（点击被 popover 消费）")
	}
	mustRun(t, rt, `
	{
		const p = document.getElementById("p");
		if (!p.matches(":popover-open")) throw new Error("invoker 激活后 popover 应处于显示状态");
		if (window.__src !== "btn") {
			throw new Error("ToggleEvent.source 应是 invoker 本身，得到 " + window.__src);
		}
	}
	`)

	// 再次激活（action=show 且已显示）→ 消费但不改变状态。
	if !popover.RunActivation(btn, btn) {
		t.Fatal("已显示时 RunActivation 仍应返回 true")
	}
	mustRun(t, rt, `
	{
		const p = document.getElementById("p");
		if (!p.matches(":popover-open")) throw new Error("show 命令不应关闭已显示的 popover");
	}
	`)
}
