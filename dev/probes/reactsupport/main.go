// Command reactsupport 诊断「React 19 在 wb-ui 引擎里静默挂起」的具体缺失项。
//
// 背景：idepage_wbui 探针加载同一 React 压测页时，bundle.js 确实执行了
// （window.__bench 被创建、reactMountCallMs 已写入、无 fatal），
// 但 createRoot().render() 之后再无任何 DOM（domCount=9），且 console 无报错
// —— 典型的「调度器没被驱动」而非「脚本报错」。
//
// 本探针逐项探测：① API 能力面 ② 异步驱动是否真的会推进（微任务/Promise/
// setTimeout/rAF/MessageChannel/MutationObserver），并检查 root 容器上是否留下了
// React 的内部标记（__reactContainer$xxx），以区分「React 没启动」与「启动了但没提交」。
//
// 用法（仓库根，CGO 环境）：
//
//	go run ./dev/probes/reactsupport -url http://127.0.0.1:8099/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// jsDiag 启动探测：先记 API 面，再逐个挂异步回调（结果稍后读回）。
const jsDiag = `(function () {
  var d = { api: {}, async: {}, dom: {} };
  d.api.MessageChannel = typeof MessageChannel;
  d.api.MessagePort = typeof MessagePort;
  d.api.requestAnimationFrame = typeof requestAnimationFrame;
  d.api.cancelAnimationFrame = typeof cancelAnimationFrame;
  d.api.queueMicrotask = typeof queueMicrotask;
  d.api.MutationObserver = typeof MutationObserver;
  d.api.IntersectionObserver = typeof IntersectionObserver;
  d.api.ResizeObserver = typeof ResizeObserver;
  d.api.requestIdleCallback = typeof requestIdleCallback;
  d.api.PerformanceObserver = typeof PerformanceObserver;
  d.api.EventTarget = typeof EventTarget;
  d.api.CustomEvent = typeof CustomEvent;
  d.api.WeakRef = typeof WeakRef;
  d.api.structuredClone = typeof structuredClone;
  d.api.setImmediate = typeof setImmediate;
  d.api.setTimeout = typeof setTimeout;
  d.api.setInterval = typeof setInterval;
  d.api.clearImmediate = typeof clearImmediate;
  d.api.process = typeof process;
  d.api.globalThis = typeof globalThis;
  d.api.window_setImmediate = typeof (window && window.setImmediate);
  d.api.window_MessageChannel = typeof (window && window.MessageChannel);
  d.api.fetch = typeof fetch;
  d.api.Promise = typeof Promise;
  d.api.Symbol = typeof Symbol;
  d.api.Proxy = typeof Proxy;
  d.api.Reflect = typeof Reflect;
  d.api.WeakMap = typeof WeakMap;
  d.api.queueMicrotaskOnWindow = typeof (window && window.queueMicrotask);
  d.api.performanceNow = (typeof performance !== 'undefined') ? typeof performance.now : 'no-performance';
  d.api.messageChannelProto = (typeof MessageChannel === 'function') ?
      Object.getOwnPropertyNames(MessageChannel.prototype).join(',') : '';

  d.async.micro = false; d.async.promise = false; d.async.timeout = false;
  d.async.raf = false; d.async.mc = false; d.async.mo = false;

  try { queueMicrotask(function () { d.async.micro = true; }); }
  catch (e) { d.async.microErr = String(e); }

  try { Promise.resolve().then(function () { d.async.promise = true; }); }
  catch (e) { d.async.promiseErr = String(e); }

  try { setTimeout(function () { d.async.timeout = true; }, 0); }
  catch (e) { d.async.timeoutErr = String(e); }

  d.async.immediate = false;
  try {
    if (typeof setImmediate === 'function') {
      setImmediate(function () { d.async.immediate = true; });
    }
  } catch (e) { d.async.immediateErr = String(e && e.stack || e); }

  try {
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(function () { d.async.raf = true; });
    }
  } catch (e) { d.async.rafErr = String(e && e.stack || e); }

  try {
    if (typeof MessageChannel === 'function') {
      var ch = new MessageChannel();
      ch.port1.onmessage = function () { d.async.mc = true; };
      if (typeof ch.port1.start === 'function') { ch.port1.start(); }
      ch.port2.postMessage('ping');
    }
  } catch (e) { d.async.mcErr = String(e && e.stack || e); }

  try {
    var rootEl = document.getElementById('root');
    if (typeof MutationObserver === 'function' && rootEl) {
      var mo = new MutationObserver(function () { d.async.mo = true; });
      mo.observe(rootEl, { childList: true, subtree: true });
      var x = document.createElement('div');
      x.id = 'mo-probe';
      rootEl.appendChild(x);
      d.dom.moInjected = true;
    }
  } catch (e) { d.async.moErr = String(e && e.stack || e); }

  window.__diag = d;
  return 'started';
})()`

// jsCollect 读回探测结果 + DOM/React 内部标记。
const jsCollect = `(function () {
  function safe(fn, dflt) {
    try { var v = fn(); return (v === undefined || v === null) ? dflt : v; }
    catch (e) { return 'ERR:' + String(e && e.message || e); }
  }
  var d = window.__diag || {};
  d.dom = d.dom || {};
  var rootEl = document.getElementById('root');
  d.dom.rootExists = !!rootEl;
  d.dom.rootChildren = safe(function () { return rootEl.children.length; }, -1);
  d.dom.rootInnerLen = safe(function () { return (rootEl.innerHTML || '').length; }, -1);
  d.dom.rootReactKeys = safe(function () {
    return Object.getOwnPropertyNames(rootEl).filter(function (k) { return k.indexOf('__react') === 0; });
  }, []);
  d.dom.totalElements = safe(function () { return document.getElementsByTagName('*').length; }, -1);
  d.dom.scripts = safe(function () { return document.getElementsByTagName('script').length; }, -1);
  d.dom.scriptSrcs = safe(function () {
    var out = [], ss = document.getElementsByTagName('script');
    for (var i = 0; i < ss.length; i++) {
      out.push(String(ss[i].src || '(inline)').replace(/^https?:\/\/[^/]+/, ''));
    }
    return out;
  }, []);
  d.dom.readyState = safe(function () { return document.readyState; }, '');
  d.dom.title = safe(function () { return document.title; }, '');
  d.dom.bodyTextLen = safe(function () { return (document.body && document.body.textContent || '').length; }, -1);
  d.dom.benchType = safe(function () { return typeof window.__bench; }, '');
  if (window.__bench) {
    d.dom.benchUA = safe(function () { return window.__bench.ua; }, '');
    d.dom.benchMount = safe(function () { return window.__bench.reactMountCallMs; }, null);
    d.dom.benchFatal = safe(function () { return window.__bench.fatal || ''; }, '');
    d.dom.benchReady = safe(function () { return window.__bench.ready; }, null);
    d.dom.benchRafAvail = safe(function () { return window.__bench.rafAvailable; }, null);
  }
  return JSON.stringify(d);
})()`

func evalStr(wv *webkit.WebView, js string) string {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8099/", "压测页地址")
	rounds := flag.Int("rounds", 60, "驱动轮数")
	w := flag.Int("w", 1280, "视口宽")
	h := flag.Int("h", 860, "视口高")
	minimal := flag.Bool("minimal", false, "最小 React 复现模式：读 window.__minCheck()")
	flag.Parse()

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	start := time.Now()
	if err := wv.LoadURL(*url); err != nil {
		fmt.Printf("[FAIL] LoadURL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] LoadURL %v\n", time.Since(start))

	if *minimal {
		for i := 0; i < *rounds; i++ {
			pumpFrame(wv)
		}
		fmt.Println("=== minimal 结果 ===")
		out := evalStr(wv, `(window.__minCheck ? window.__minCheck() : '(no __minCheck)')`)
		if pretty, err := prettyJSON(out); err == nil {
			fmt.Println(pretty)
		} else {
			fmt.Println(out)
		}
		fmt.Println("=== window.__min ===")
		fmt.Println(evalStr(wv, `JSON.stringify(window.__min || null)`))
		fmt.Println("=== __minErrors ===")
		fmt.Println(evalStr(wv, `JSON.stringify(window.__minErrors || null)`))
		fmt.Println("=== 异步原语 hook 计数 ===")
		fmt.Println(evalStr(wv, `JSON.stringify(window.__hooks || null)`))
		fmt.Println("=== polyfill 内异常 ===")
		fmt.Println(evalStr(wv, `String(window.__polyErr || '(none)')`))
		fmt.Println("=== console 输出 ===")
		if s := strings.TrimSpace(wv.ConsoleOutput()); s != "" {
			fmt.Println(s)
		} else {
			fmt.Println("(无输出)")
		}
		return
	}

	fmt.Println("[diag] 启动探测:", evalStr(wv, jsDiag))
	// 给异步驱动充足机会（含渲染帧）
	for i := 0; i < *rounds; i++ {
		pumpFrame(wv)
	}
	out := evalStr(wv, jsCollect)
	fmt.Println("=== 探测结果 ===")
	if pretty, err := prettyJSON(out); err == nil {
		fmt.Println(pretty)
	} else {
		fmt.Println(out)
	}

	fmt.Println("=== console 输出 ===")
	if s := strings.TrimSpace(wv.ConsoleOutput()); s != "" {
		fmt.Println(s)
	} else {
		fmt.Println("(无输出)")
	}
}

func prettyJSON(s string) (string, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}
