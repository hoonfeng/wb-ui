// Tests for the Worker bindings（并发脚本执行）：data: 脚本加载、双向消息、
// 克隆语义（无共享对象）、terminate / close、脚本错误、importScripts，以及
// 「worker 忙等时主线程照常推进」这一并发本质。

package bindings

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"wb-ui/jsc"
)

// workerDataURL 把脚本正文包成 data: URL（引擎无需联网即可加载）。
func workerDataURL(src string) string {
	return "data:text/javascript;base64," + base64.StdEncoding.EncodeToString([]byte(src))
}

// jsLit 把 Go 字符串转成 JS 字面量（安全内联进测试脚本）。
func jsLit(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// pumpUntil 驱动主线程事件循环（并跑微任务），直到 cond 为真或超时。
// 返回 cond 是否最终成立。
func pumpUntil(rt *jsc.Interpreter, cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if el := rt.EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		rt.RunJobs()
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// jsNum 求值一个 JS 数值表达式（出错返回 -1）。
func jsNum(rt *jsc.Interpreter, expr string) float64 {
	v, err := rt.Run("(" + expr + ")")
	if err != nil {
		return -1
	}
	switch n := v.(type) {
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case float64:
		return n
	}
	return -1
}

func TestWorkerMessageEventConstructor(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		{
			const ev = new MessageEvent("message", { data: { a: 1 }, origin: "https://example.test", lastEventId: "7" });
			if (ev.type !== "message") throw new Error("type 错误");
			if (ev.data.a !== 1) throw new Error("data 未保留");
			if (ev.origin !== "https://example.test") throw new Error("origin 错误");
			if (ev.lastEventId !== "7") throw new Error("lastEventId 错误");
			if (ev.source !== null) throw new Error("source 应为 null");
			if (!Array.isArray(ev.ports)) throw new Error("ports 应为数组");
			const def = new MessageEvent("ping");
			if (def.type !== "ping") throw new Error("默认构造 type 错误");
			if (def.data !== null) throw new Error("未给 data 时应为 null");
		}
	`)
}

// TestWorkerDataURLRoundTripAndSelfName 覆盖：data: 脚本加载、self.name、
// worker 内 self 指向全局、main → worker → main 的往返。
func TestWorkerDataURLRoundTripAndSelfName(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `
		self.onmessage = function (e) {
			self.postMessage({
				echo: e.data,
				name: self.name,
				hasPost: (typeof self.postMessage === "function"),
				selfIsGlobal: (self === globalThis)
			});
		};
	`
	mustRun(t, rt, `
		window.__got = [];
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`, { name: "echo" });
		window.__w.onmessage = function (e) { window.__got.push(e.data); };
		window.__w.postMessage({ n: 7, s: "hi" });
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__got.length") >= 1 }, 5*time.Second) {
		t.Fatal("worker 未回消息（脚本未执行或主线程消息泵未派发）")
	}
	mustRun(t, rt, `
		{
			const d = window.__got[0];
			if (!d || !d.echo) throw new Error("回信缺少 echo: " + JSON.stringify(d));
			if (d.echo.n !== 7 || d.echo.s !== "hi") throw new Error("回显数据错误: " + JSON.stringify(d.echo));
			if (d.name !== "echo") throw new Error("worker 的 self.name 应为 echo，实际 " + d.name);
			if (d.hasPost !== true) throw new Error("worker 内 self.postMessage 不可用");
			if (d.selfIsGlobal !== true) throw new Error("worker 内 self 应指向全局对象");
		}
	`)
}

// TestWorkerMessagesAreCloned 覆盖结构化克隆语义：两个运行时之间没有共享对象。
func TestWorkerMessagesAreCloned(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `self.onmessage = function (e) { e.data.touched = true; self.postMessage(e.data); };`
	mustRun(t, rt, `
		window.__back = null;
		window.__src = { value: 1 };
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function (e) { window.__back = e.data; };
		window.__w.postMessage(window.__src);
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__back === null ? 0 : 1") == 1 }, 5*time.Second) {
		t.Fatal("worker 未回消息")
	}
	mustRun(t, rt, `
		{
			if (window.__src.touched !== undefined) throw new Error("worker 的修改污染了主线程对象（未克隆）");
			if (window.__back.touched !== true) throw new Error("worker 侧修改未体现在回信对象上");
			if (window.__back.value !== 1) throw new Error("回信对象丢字段");
		}
	`)
}

// TestWorkerTerminateStopsDelivery 覆盖 terminate：之后既不派发消息，也不再处理。
func TestWorkerTerminateStopsDelivery(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `self.onmessage = function (e) { self.postMessage("tick:" + e.data); };`
	mustRun(t, rt, `
		window.__msgs = [];
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function (e) { window.__msgs.push(e.data); };
		window.__w.postMessage(1);
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__msgs.length") >= 1 }, 5*time.Second) {
		t.Fatal("terminate 前置条件未满足：worker 未回第一条消息")
	}
	mustRun(t, rt, `
		window.__w.terminate();
		window.__w.postMessage(2);
	`)
	pumpUntil(rt, func() bool { return false }, 300*time.Millisecond)
	if n := jsNum(rt, "window.__msgs.length"); n != 1 {
		t.Fatalf("terminate 后仍收到消息：期望 1 条，实际 %v", n)
	}
}

// TestWorkerCloseStopsMessageHandling 覆盖 worker 内 self.close()。
func TestWorkerCloseStopsMessageHandling(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `
		self.onmessage = function (e) {
			if (e.data === 1) { self.close(); self.postMessage("closing"); }
			else { self.postMessage("after-close"); }
		};
	`
	mustRun(t, rt, `
		window.__msgs = [];
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function (e) { window.__msgs.push(e.data); };
		window.__w.postMessage(1);
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__msgs.length") >= 1 }, 5*time.Second) {
		t.Fatal("close 前置条件未满足：worker 未回第一条消息")
	}
	mustRun(t, rt, `window.__w.postMessage(2);`)
	pumpUntil(rt, func() bool { return false }, 300*time.Millisecond)
	if n := jsNum(rt, "window.__msgs.length"); n != 1 {
		t.Fatalf("close() 后仍处理消息：期望 1 条，实际 %v", n)
	}
	mustRun(t, rt, `
		{
			if (window.__msgs[0] !== "closing") throw new Error("首条消息内容错误: " + window.__msgs[0]);
		}
	`)
}

// TestWorkerScriptErrorDispatch 覆盖脚本语法错误 → error 事件（ErrorEvent 形状）。
func TestWorkerScriptErrorDispatch(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	bad := workerDataURL("this is (not valid javascript")
	mustRun(t, rt, `
		window.__err = null;
		window.__w = new Worker(`+jsLit(bad)+`);
		window.__w.onerror = function (e) {
			window.__err = { type: e.type, message: e.message, filename: e.filename };
		};
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__err === null ? 0 : 1") == 1 }, 5*time.Second) {
		t.Fatal("worker 脚本错误未派发 error 事件")
	}
	mustRun(t, rt, `
		{
			if (window.__err.type !== "error") throw new Error("事件类型应为 error，实际 " + window.__err.type);
			if (!window.__err.message || window.__err.message.length === 0) throw new Error("message 为空");
			if (window.__err.filename.indexOf("data:") !== 0) throw new Error("filename 应为脚本 URL，实际 " + window.__err.filename);
		}
	`)
}

// TestWorkerUnsupportedURLDispatchesError 覆盖「引擎默认不联网」：非 data: URL
// 且宿主未注入 Fetcher 时，派发 error 事件而不是静默失败。
func TestWorkerUnsupportedURLDispatchesError(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		window.__err = null;
		window.__w = new Worker("https://example.test/worker.js");
		window.__w.onerror = function (e) { window.__err = e.message; };
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__err === null ? 0 : 1") == 1 }, 5*time.Second) {
		t.Fatal("未注入 Fetcher 时未派发 error 事件")
	}
	mustRun(t, rt, `
		{
			if (window.__err.indexOf("no fetcher") < 0) throw new Error("错误文本应说明缺少 Fetcher，实际 " + window.__err);
		}
	`)
}

// TestWorkerImportScripts 覆盖 importScripts（同步加载并执行，data: URL）。
func TestWorkerImportScripts(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	lib := workerDataURL(`self.__libValue = 41; self.bump = function () { return self.__libValue + 1; };`)
	src := `importScripts(` + jsLit(lib) + `);
		self.onmessage = function () { self.postMessage(self.bump()); };`
	mustRun(t, rt, `
		window.__val = null;
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function (e) { window.__val = e.data; };
		window.__w.postMessage("go");
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__val === null ? 0 : 1") == 1 }, 5*time.Second) {
		t.Fatal("importScripts 后未回消息")
	}
	if v := jsNum(rt, "window.__val"); v != 42 {
		t.Fatalf("importScripts 加载的脚本未生效：期望 42，实际 %v", v)
	}
}

// TestWorkerBusyLoopDoesNotBlockMainThread 是「并发」的本质断言：worker 忙等
// 占住自己的线程时，主线程的定时器照常推进。若 worker 跑在页面线程上，主线程
// 的 tick 会被卡住，ticks 不会增长。
func TestWorkerBusyLoopDoesNotBlockMainThread(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `
		self.onmessage = function () {
			const start = Date.now();
			while (Date.now() - start < 400) {}
			self.postMessage("busy-done");
		};
	`
	mustRun(t, rt, `
		window.__ticks = 0;
		window.__busyDone = null;
		window.__timer = setInterval(function () { window.__ticks++; }, 10);
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function (e) { window.__busyDone = e.data; };
		window.__w.postMessage("start");
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__busyDone === null ? 0 : 1") == 1 }, 10*time.Second) {
		t.Fatal("worker 忙等后未回消息")
	}
	ticks := jsNum(rt, "window.__ticks")
	if ticks < 5 {
		t.Fatalf("worker 忙等期间主线程定时器只推进 %v 次（应 >= 5）——worker 占住了主线程", ticks)
	}
	mustRun(t, rt, `
		clearInterval(window.__timer);
		if (window.__busyDone !== "busy-done") throw new Error("忙等后的回信内容错误: " + window.__busyDone);
	`)
}

// TestWorkerPostMessageDataCloneError 覆盖不可克隆值的 DataCloneError。
func TestWorkerPostMessageDataCloneError(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		window.__w = new Worker(`+jsLit(workerDataURL(`self.onmessage = function () {};`))+`);
		window.__name = null;
		try {
			window.__w.postMessage(function () {});
		} catch (e) {
			window.__name = e.name;
		}
		if (window.__name !== "DataCloneError") throw new Error("函数应抛 DataCloneError，实际 " + window.__name);

		const circular = {};
		circular.self = circular;
		let circularName = null;
		try {
			window.__w.postMessage(circular);
		} catch (e) {
			circularName = e.name;
		}
		if (circularName !== "DataCloneError") throw new Error("循环引用应抛 DataCloneError，实际 " + circularName);
	`)
}

// TestWorkerAddEventListenerAndOnMessage 覆盖两种注册方式同时生效。
func TestWorkerAddEventListenerAndOnMessage(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	src := `self.onmessage = function () { self.postMessage("pong"); };`
	mustRun(t, rt, `
		window.__viaProp = 0;
		window.__viaListener = 0;
		window.__w = new Worker(`+jsLit(workerDataURL(src))+`);
		window.__w.onmessage = function () { window.__viaProp++; };
		window.__w.addEventListener("message", function () { window.__viaListener++; });
		window.__w.postMessage("ping");
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__viaListener") >= 1 }, 5*time.Second) {
		t.Fatal("addEventListener 注册的监听器未收到消息")
	}
	mustRun(t, rt, `
		{
			if (window.__viaProp !== 1) throw new Error("onmessage 属性回调未触发: " + window.__viaProp);
			if (window.__viaListener !== 1) throw new Error("监听器回调次数错误: " + window.__viaListener);
		}
	`)
}

// TestWorkerInjectedFetcher 覆盖宿主注入的脚本加载器：非 data: URL 也能加载，
// 且 URL 原样交给宿主（宿主据此走自己的资源管道与拦截规则）。
func TestWorkerInjectedFetcher(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	var asked []string
	SetWorkerScriptFetcher(func(u string) (string, error) {
		asked = append(asked, u)
		if u != "app://worker.js" {
			return "", fmt.Errorf("unexpected url: %s", u)
		}
		return `self.onmessage = function () { self.postMessage("fetched"); };`, nil
	})
	defer SetWorkerScriptFetcher(nil)

	mustRun(t, rt, `
		window.__got = [];
		window.__w = new Worker("app://worker.js");
		window.__w.onmessage = function (e) { window.__got.push(e.data); };
		window.__w.postMessage("go");
	`)
	if !pumpUntil(rt, func() bool { return jsNum(rt, "window.__got.length") >= 1 }, 5*time.Second) {
		t.Fatal("注入 Fetcher 后 worker 未回消息")
	}
	mustRun(t, rt, `
		{
			if (window.__got[0] !== "fetched") throw new Error("回信内容错误: " + window.__got[0]);
		}
	`)
	if len(asked) != 1 || asked[0] != "app://worker.js" {
		t.Fatalf("宿主 Fetcher 收到的 URL 不对: %v", asked)
	}
}
