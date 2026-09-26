// Command gouide_desktop_probe 探测「用 wb-ui 构建桌面端」的实战链路。
//
// 用 ModeBrowser（默认模式，桌面端形态）的 WebView 经 LoadURL 加载**真实
// 运行**的 gou-ide 前端（默认 http://127.0.0.1:9097/），驱动事件循环 + 渲染
// 若干帧后报告：
//   - document.title / #app 挂载情况 / 页面自带 window.__pageErrors
//   - UI 区域装配链路诊断（/api/ui-boot 图、/plugins-assets/<id>/client.js、/api/plugins）
//   - UI 运行时状态（window.__pluginRuntime：client 半实例、槽位占用、boot 脚本）
//   - console 输出（含 JS 异常）
//   - 关键区域几何（宽高是否为 0 = 布局塌陷）
//   - 渲染产物 PNG（dev/output/），供人眼目检
//
// 用法（仓库根目录，CGO 环境）：
//
//	cgo_env.bat run ./dev/probes/gouide_desktop_probe -url http://127.0.0.1:9097/
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// jsProbe 收集页面侧可观测状态：任意一项失败都不影响其余项（逐项 try/catch）。
const jsProbe = `(function () {
  function safe(fn, dflt) { try { var v = fn(); return (v === undefined || v === null) ? dflt : v; } catch (e) { return 'ERR:' + String(e && e.message || e); } }
  var out = {};
  out.title = safe(function () { return document.title; }, '');
  out.totalElements = safe(function () { return document.getElementsByTagName('*').length; }, -1);
  out.appChildren = safe(function () { var a = document.getElementById('app'); return a ? a.children.length : -1; }, -1);
  out.bodyText = safe(function () { var t = (document.body && (document.body.innerText || document.body.textContent)) || ''; return t.replace(/\s+/g, ' ').slice(0, 300); }, '');
  out.pageErrors = safe(function () { return (window.__pageErrors || []).slice(0, 12); }, []);
  out.hasVue = safe(function () { return typeof (window.Vue || {}) + '/' + (!!document.getElementById('app') && !!document.getElementById('app').__vue_app__); }, '');
  out.geom = {};
  var sels = ['#app-bg', '#app', '.titlebar', '.app-shell', '.sidebar', '.chat-panel', '.editor-area'];
  for (var i = 0; i < sels.length; i++) {
    out.geom[sels[i]] = safe(function () {
      var e = document.querySelector(sels[i]);
      if (!e) return 'missing';
      var b = e.getBoundingClientRect();
      return [Math.round(b.x), Math.round(b.y), Math.round(b.width), Math.round(b.height)].join(',');
    }, 'missing');
  }
  return JSON.stringify(out);
})()`

// jsDiag 诊断 UI 区域装配链路：boot() 依赖的三个端点在页面内是否可用。
const jsDiag = `window.__diag = 'pending';
(async function () {
  var out = {};
  try { out.core = typeof window.__PAIRCODE_CORE; } catch (e) { out.core = 'ERR'; }
  try {
    var r = await fetch('/api/ui-boot');
    out.uiBootStatus = r.status;
    var g = await r.json();
    out.uiBootRev = String(g.rev || '').slice(0, 16);
    out.uiBootEntries = (g.entries || []).length;
    out.uiBootIds = (g.entries || []).map(function (e) { return e.id + (e.immediately ? '*' : ''); }).join(',');
  } catch (e) { out.uiBootErr = String((e && e.message) || e); }
  try {
    var r2 = await fetch('/plugins-assets/ui-titlebar/client.js');
    out.clientStatus = r2.status;
    var t = await r2.text();
    out.clientLen = t.length;
  } catch (e) { out.clientErr = String((e && e.message) || e); }
  try {
    var r3 = await fetch('/api/plugins');
    var list = await r3.json();
    out.pluginsCount = (list || []).length;
    var running = 0, withClient = 0;
    for (var i = 0; i < (list || []).length; i++) {
      if (list[i] && list[i].state === 'running') running++;
      if (list[i] && list[i].state === 'running' && list[i].hasClient && list[i].clientCode) withClient++;
    }
    out.running = running; out.runningWithClient = withClient;
    out.uiPkgs = (list || []).filter(function (p) { return /^ui-/.test(p.name); }).map(function (p) { return p.name + '=' + p.state + '/c' + ((p.clientCode || '').length); }).join(',');
  } catch (e) { out.pluginsErr = String((e && e.message) || e); }
  window.__diag = JSON.stringify(out);
})();`

// jsDiag2 读页面运行时的 UI 装配状态（plugin-runtime 的调试入口）。
const jsDiag2 = `window.__diag2 = 'pending';
(async function () {
  var out = {};
  try {
    var pr = window.__pluginRuntime;
    out.hasPR = !!pr;
    if (pr) {
      out.instances = pr.instances().map(function (i) { return i.name + ':' + i.status + (i.error ? ('(' + String(i.error).slice(0, 80) + ')') : ''); }).join(' | ');
      out.slots = pr.clientSlots().map(function (s) { return s.slotId + ':' + s.pluginName + ':r' + (s.hasRender ? 1 : 0); }).join(' | ');
      out.owners = ['titlebar', 'activitybar', 'sidebar', 'editor', 'conversation', 'statusbar', 'modals'].map(function (id) { return id + '=' + (pr.getSlotOwner(id) || '-'); }).join(' ');
    }
  } catch (e) { out.prErr = String((e && e.message) || e); }
  try { out.bootScripts = document.querySelectorAll('script[data-boot]').length; } catch (e) {}
  try { out.scriptSrcs = document.querySelectorAll('script[src]').length; } catch (e) {}
  try { out.globals = Object.keys(window).filter(function (k) { return /^(Ui|Tool|Market|Pair)/.test(k); }).join(','); } catch (e) {}
  try {
    var core = window.__PAIRCODE_CORE;
    out.hasApi = !!(core && core.api);
    if (core && core.api && core.api.listPlugins) {
      var list = await core.api.listPlugins();
      out.apiListLen = (list || []).length;
    }
  } catch (e) { out.apiErr = String((e && e.message) || e); }
  window.__diag2 = JSON.stringify(out);
})();`

// pump 驱动事件循环 + 微任务 + 一帧渲染，让 Promise/定时器/图片推进。
func pump(wv *webkit.WebView, rounds int) {
	for i := 0; i < rounds; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		wv.EnsureLayout()
		_, _ = wv.Render()
		time.Sleep(5 * time.Millisecond)
	}
}

func evalStr(wv *webkit.WebView, script string) string {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

func main() {
	url := flag.String("url", "http://127.0.0.1:9097/", "gou-ide 前端地址（ModeBrowser 真实 HTTP）")
	out := flag.String("out", "dev/output/gouide_desktop.png", "PNG 输出路径")
	w := flag.Int("w", 1600, "视口宽")
	h := flag.Int("h", 1000, "视口高")
	frames := flag.Int("frames", 60, "渲染/事件循环轮数")
	inject := flag.String("inject", "", "可选：注入页面的 JS 文件路径（渲染前执行；用于构造待测 DOM 场景）")
	flag.Parse()

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	logger := &jsc.BufferLogger{}
	wv.SetConsoleLogger(logger)

	start := time.Now()
	if err := wv.LoadURL(*url); err != nil {
		fmt.Printf("[FAIL] LoadURL(%s): %v\n", *url, err)
		os.Exit(1)
	}
	fmt.Printf("[OK] LoadURL(%s) 完成，耗时 %v（视口 %dx%d）\n", *url, time.Since(start), *w, *h)

	for _, d := range []struct{ label, js string }{{"boot 链路", jsDiag}, {"运行时状态", jsDiag2}} {
		if _, err := wv.EvalJS(d.js); err != nil {
			fmt.Printf("[WARN] %s 诊断脚本注入失败: %v\n", d.label, err)
		}
	}
	pump(wv, 40)
	fmt.Println("=== UI 区域装配链路诊断 ===")
	fmt.Println(evalStr(wv, `String(window.__diag)`))
	fmt.Println("=== UI 运行时状态 ===")
	fmt.Println(evalStr(wv, `String(window.__diag2)`))

	runDynScriptProbe(wv)

	// 可选注入脚本：在最终渲染前执行，用于构造「待测 DOM」（如 toast 节点），
	// 使引擎侧能对同一结构采样几何 + 出 PNG，与浏览器同视口对照。
	if *inject != "" {
		src, rerr := os.ReadFile(*inject)
		if rerr != nil {
			fmt.Printf("[WARN] 读取注入脚本 %s 失败: %v\n", *inject, rerr)
		} else {
			fmt.Printf("=== 注入脚本 %s（%d 字节）===\n", *inject, len(src))
			fmt.Println("inject  :", evalStr(wv, string(src)))
		}
	}

	pump(wv, *frames)

	fmt.Println("=== 页面侧状态 ===")
	fmt.Println(evalStr(wv, jsProbe))

	runDomDump(wv)

	fmt.Println("=== console 输出 ===")
	if s := strings.TrimSpace(wv.ConsoleOutput()); s != "" {
		fmt.Println(s)
	} else {
		fmt.Println("(无输出)")
	}

	pix, err := wv.Render()
	if err != nil || len(pix) == 0 {
		fmt.Printf("[FAIL] 渲染无像素输出: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Printf("[WARN] mkdir: %v\n", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, *w, *h))
	if len(pix) >= len(img.Pix) {
		copy(img.Pix, pix[:len(img.Pix)])
	}
	f, cerr := os.Create(*out)
	if cerr != nil {
		fmt.Printf("[FAIL] create png: %v\n", cerr)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Printf("[FAIL] encode png: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] PNG 已写出: %s\n", *out)
}
