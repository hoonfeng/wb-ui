// Command gouide_real_e2e 把 gouide 的**真实构建产物**加载进 wb-ui 引擎，验证
// 「Vue 3 壳能否真正挂载」—— 这是「让 gouide 用上 wb-ui」的直接前置条件。
//
// 与既有探针的分工：
//
//	· vuesupport       —— 合成最小 Vue 应用，验证引擎的 API 面与响应式
//	· framework_matrix —— 同规模压测页，验证框架性能可比性
//	· gouide_real_e2e  —— **未裁剪的真实产物**：vite 壳 index.html +
//	  assets/index-*.js + vendor/mermaid.min.js，以及 /plugins-assets/<name>/...
//	  的 10 个 ui-* 区域插件 bundle（源：.pair/plugins/<name>/）
//
// 静态服务路由对齐 gouide 的 cmd/companion/web_server.go：
//
//	/plugins-assets/<name>/<file> → <gouide>/.pair/plugins/<name>/<file>
//	其余                            → <gouide>/cmd/companion/web-ui/dist/<path>
//
// 显式设置 Content-Type（module script 对 MIME 敏感，MIME 不对会被引擎拒绝执行，
// 那会造成「假阴性」—— 误判为引擎不支持而非服务配置问题）。
//
// 用法（wb-ui 仓库根，CGO 环境）：
//
//	go run ./dev/probes/gouide_real_e2e
//	go run ./dev/probes/gouide_real_e2e -gouide /f/syproject/gou-ide -rounds 150
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// ─── gouide 后端 API 的最小桩 ─────────────────────────────────────────────
//
// 真实壳（dist/assets/index-<hash>.js）启动即向后端要「UI 装配清单」，否则不会
// 去加载 /plugins-assets/*.js 的区域 bundle（实测：无桩时只有 2 个 200 请求，
// 页面只挂出一个空壳 #app）。这里复刻 gouide internal/agent/uiboot.go 的
// UIBootGraph 结构：
//
//	GET /api/ui-boot → { rev, entries:[ {id, url, rev, inject, immediately, external} ] }
//
// entry.url 形如 /plugins-assets/<pkgName>/assets/<file>.js?rev=<hash>。
// 只纳入含 dsh.ui 段的包（与 gouide 发现层一致），bundle 缺失的包跳过。

type uiBootEntry struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Rev         string   `json:"rev"`
	Inject      []string `json:"inject"`
	Immediately bool     `json:"immediately"`
	External    []string `json:"external"`
}

type uiBootGraph struct {
	Rev     string        `json:"rev"`
	Entries []uiBootEntry `json:"entries"`
}

// scanUIBootGraph 扫描插件目录装配 boot 图；第二个返回值是跳过说明（报告用）。
func scanUIBootGraph(pluginsDir string) (uiBootGraph, []string) {
	g := uiBootGraph{Rev: "stub-rev", Entries: []uiBootEntry{}}
	var notes []string
	dirents, err := os.ReadDir(pluginsDir)
	if err != nil {
		return g, []string{"插件目录不可读：" + err.Error()}
	}
	type pkgShape struct {
		Name string `json:"name"`
		Dsh  *struct {
			UI *struct {
				Slot        string   `json:"slot"`
				Kind        string   `json:"kind"`
				Inject      []string `json:"inject"`
				Immediately *bool    `json:"immediately"`
			} `json:"ui"`
		} `json:"dsh"`
	}
	for _, de := range dirents {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		name := de.Name()
		pkgDir := filepath.Join(pluginsDir, name)
		raw, err := os.ReadFile(filepath.Join(pkgDir, "package.json"))
		if err != nil {
			continue
		}
		var pkg pkgShape
		if err := json.Unmarshal(raw, &pkg); err != nil || pkg.Dsh == nil || pkg.Dsh.UI == nil {
			continue // 无 dsh.ui 段 → 不进 boot 图
		}
		id := pkg.Name
		if id == "" {
			id = name
		}
		bundle := findBundle(pkgDir, id)
		if bundle == "" {
			notes = append(notes, name+"：有 dsh.ui 但 assets bundle 缺失 → 跳过")
			continue
		}
		imm := true
		if pkg.Dsh.UI.Immediately != nil {
			imm = *pkg.Dsh.UI.Immediately
		}
		inj := pkg.Dsh.UI.Inject
		if inj == nil {
			inj = []string{}
		}
		g.Entries = append(g.Entries, uiBootEntry{
			ID:          id,
			URL:         "/plugins-assets/" + id + "/" + bundle + "?rev=" + fileRev(filepath.Join(pkgDir, filepath.FromSlash(bundle))),
			Rev:         fileRev(filepath.Join(pkgDir, filepath.FromSlash(bundle))),
			Inject:      inj,
			Immediately: imm,
			External:    []string{"@paircode/core"},
		})
	}
	sort.Slice(g.Entries, func(i, j int) bool { return g.Entries[i].ID < g.Entries[j].ID })
	return g, notes
}

// findBundle 在 <pkg>/assets 下找区域 bundle（优先与包名同名者）。
func findBundle(pkgDir, id string) string {
	ents, err := os.ReadDir(filepath.Join(pkgDir, "assets"))
	if err != nil {
		return ""
	}
	fallback := ""
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		p := "assets/" + e.Name()
		if strings.TrimSuffix(e.Name(), ".js") == id {
			return p
		}
		if fallback == "" {
			fallback = p
		}
	}
	return fallback
}

// fileRev 内容摘要（cache-busting 锚，语义同 gouide 的 rev）。
func fileRev(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "0"
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:8])
}

// ─── 请求日志（哪些资源没喂到，是排障关键证据）────────────────────────────

type reqLog struct {
	mu    sync.Mutex
	ok    int
	miss  int
	items []map[string]any
}

func (l *reqLog) add(path string, status, size int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if status == 200 {
		l.ok++
	} else {
		l.miss++
	}
	if len(l.items) < 400 {
		l.items = append(l.items, map[string]any{"path": path, "status": status, "size": size})
	}
}

func (l *reqLog) missing() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, it := range l.items {
		if it["status"] != 200 {
			out = append(out, it)
		}
	}
	return out
}

// ─── 静态服务 ─────────────────────────────────────────────────────────────

// mimeByExt 显式映射：module script 要求 JavaScript MIME，若交给系统 mime 表
// （Windows 上取注册表）可能得到 text/plain → 引擎拒绝执行 → 假阴性。
var mimeByExt = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".map":   "application/json; charset=utf-8",
}

func serveFile(w http.ResponseWriter, r *http.Request, fp string, rl *reqLog) {
	b, err := os.ReadFile(fp)
	if err != nil {
		rl.add(r.URL.Path, 404, 0)
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(fp))
	ct := mimeByExt[ext]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(200)
	n, _ := w.Write(b)
	rl.add(r.URL.Path, 200, n)
}

// startServer 起本地静态服务（随机端口），路由语义对齐 gouide web_server。
// 返回装配好的 boot 图与跳过说明，供报告记录。
func startServer(gouideRoot string, rl *reqLog) (*http.Server, string, uiBootGraph, []string, error) {
	distDir := filepath.Join(gouideRoot, "cmd", "companion", "web-ui", "dist")
	pluginsDir := filepath.Join(gouideRoot, ".pair", "plugins")
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return nil, "", uiBootGraph{}, nil, fmt.Errorf("未找到壳产物 %s/index.html：%w", distDir, err)
	}

	mux := http.NewServeMux()

	// 后端 API 桩：壳要先拿到 /api/ui-boot 的装配清单，才会去加载区域 bundle。
	boot, bootNotes := scanUIBootGraph(pluginsDir)
	mux.HandleFunc("/api/ui-boot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		b, _ := json.Marshal(boot)
		w.WriteHeader(200)
		_, _ = w.Write(b)
		rl.add(r.URL.Path, 200, len(b))
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// 其余接口给空对象桩（/api/ui-assembly、/api/settings、/api/health、
		// /api/plugins/client-state|client-events 等），避免壳因 404 中断装配。
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		body := "{}"
		switch r.URL.Path {
		case "/api/health":
			body = `{"ok":true}`
		case "/api/ui-assembly":
			// 真实装配状态：直接回传 gouide 的 .pair/ui-assembly.json
			// （slotOwner / slotUIEnabled / slotOverlay 决定哪些区域槽位启用）。
			if b, err := os.ReadFile(filepath.Join(gouideRoot, ".pair", "ui-assembly.json")); err == nil {
				body = string(b)
			}
		case "/api/toolsets":
			// ListAllToolsetsPublic 返回**裸数组**（[]ToolsetMeta）；给对象会让壳
			// 对 undefined 调 .filter() → "工具集列表加载失败"。
			body = `[]`
		case "/api/toolsets/active":
			// ConvToolsetActiveInfo：会话生效集合（未选 → 默认集合「基础」）。
			body = `{"selected":"","effective":"基础","isDefault":true,"converged":false,"defaultName":"基础"}`
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(body))
		rl.add(r.URL.Path, 200, len(body))
	})

	mux.HandleFunc("/plugins-assets/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/plugins-assets/")
		serveFile(w, r, filepath.Join(pluginsDir, filepath.FromSlash(rest)), rl)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		serveFile(w, r, filepath.Join(distDir, filepath.FromSlash(p)), rl)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", uiBootGraph{}, nil, err
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return srv, fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port), boot, bootNotes, nil
}

// ─── 页面侧脚本 ───────────────────────────────────────────────────────────

// jsInjectErrors 经 BeforePageScripts 在页面脚本前注入（DOM 已就绪、脚本未跑）。
const jsInjectErrors = `(function () {
  if (window.__e2eErrors) return 'already';
  var E = window.__e2eErrors = { errors: [], rejections: [], consoleError: [], consoleWarn: [] };
  window.addEventListener('error', function (e) {
    E.errors.push({
      msg: String((e && e.message) || ''),
      src: String((e && e.filename) || ''),
      line: (e && e.lineno) || 0,
      stack: String((e && e.error && e.error.stack) || '')
    });
  });
  window.addEventListener('unhandledrejection', function (e) {
    var r = e && e.reason;
    E.rejections.push(String((r && (r.stack || r.message)) || r || e));
  });
  var ce = window.console && console.error;
  if (ce) {
    console.error = function () { try { E.consoleError.push(Array.prototype.join.call(arguments, ' ')); } catch (x) {} return ce.apply(console, arguments); };
  }
  var cw = window.console && console.warn;
  if (cw) {
    console.warn = function () { try { E.consoleWarn.push(Array.prototype.join.call(arguments, ' ')); } catch (x) {} return cw.apply(console, arguments); };
  }
  // 缺失 API 访问计数（q7 证据）：把当前未定义的候选全局装 getter **只计数**，
  // 访问仍返回 undefined（语义不变），用于判定「真实运行路径是否真的碰过它」。
  var HITS = window.__missingApiHits = {};
  var CAND = ['Intl', 'WeakRef', 'customElements', 'reportError', 'WebAssembly', 'BroadcastChannel',
              'FinalizationRegistry', 'TextEncoder', 'TextDecoder', 'PerformanceObserver',
              'IntersectionObserver', 'ResizeObserver', 'queueMicrotask', 'structuredClone',
              'requestIdleCallback', 'matchMedia', 'getComputedStyle'];
  for (var mi = 0; mi < CAND.length; mi++) {
    (function (nm) {
      if (typeof window[nm] !== 'undefined') { return; }
      HITS[nm] = 0;
      try {
        Object.defineProperty(window, nm, {
          configurable: true,
          get: function () { HITS[nm]++; return undefined; },
          set: function () {}
        });
      } catch (e) { HITS[nm] = -1; }
    })(CAND[mi]);
  }
  return 'installed';
})()`

// jsCollect 读回挂载证据：根容器 DOM、关键区域节点、缺失 API、未捕获错误。
const jsCollect = `(function () {
  function safe(fn, dflt) {
    try { var v = fn(); return (v === undefined || v === null) ? dflt : v; }
    catch (e) { return 'ERR:' + String((e && e.message) || e); }
  }
  var appEl = document.getElementById('app');
  var o = {};
  o.docTitle = document.title;
  o.readyState = document.readyState;
  o.url = String(location.href || '');
  o.appExists = !!appEl;
  o.appChildCount = safe(function () { return appEl.children.length; }, -1);
  o.appInnerLen = safe(function () { return (appEl.innerHTML || '').length; }, -1);
  o.appBgExists = !!document.getElementById('app-bg');
  o.totalElements = safe(function () { return document.getElementsByTagName('*').length; }, -1);
  o.bodyTextLen = safe(function () { return ((document.body && document.body.textContent) || '').length; }, -1);
  o.appVueKeys = safe(function () {
    return Object.getOwnPropertyNames(appEl).filter(function (k) {
      return k.indexOf('__vue') === 0 || k.indexOf('__v_') === 0;
    }).join(',');
  }, '');
  o.htmlHead = safe(function () { return (appEl.innerHTML || '').slice(0, 400); }, '');

  // 关键区域节点：gouide 的 10 个 ui-* 插件渲染出的 DOM 特征
  var sels = ['#app-bg', '.app', '.titlebar', '.statusbar', '.activitybar', '.sidebar',
    '.editor', '.right-panel', '.modal-root', '.quick-exec', '[data-ui]', '#app > *'];
  o.selectors = {};
  for (var i = 0; i < sels.length; i++) {
    var s = sels[i];
    o.selectors[s] = safe(function () { return document.querySelectorAll(s).length; }, -1);
  }

  // 真实产物的区域承载：壳渲染 .app-root grid + .plugin-slot-host / .plugin-area-<slot>。
  // 逐个统计数量与内容长度，区分「槽位已装配（有内容）」与「空壳」。
  var slotSels = ['.app-root', '.plugin-slot-host', '.plugin-area-titlebar', '.plugin-area-activitybar',
    '.plugin-area-editor', '.plugin-area-sidebar', '.plugin-area-statusbar', '.plugin-area-conversation',
    '.plugin-area-modals', '.plugin-area-quick-exec'];
  o.slots = {};
  for (var si = 0; si < slotSels.length; si++) {
    var ss = slotSels[si];
    o.slots[ss] = safe(function () {
      var el = document.querySelector(ss);
      if (!el) { return { count: 0, innerLen: 0, text: '' }; }
      return {
        count: document.querySelectorAll(ss).length,
        innerLen: (el.innerHTML || '').length,
        text: ((el.textContent || '').replace(/\s+/g, ' ')).trim().slice(0, 70)
      };
    }, null);
  }

  // 引擎 API 面（Vue 3.5 源码 0 引用的项也列出，用于判定「是否阻塞」）
  var apis = ['Intl', 'WeakRef', 'customElements', 'reportError', 'ResizeObserver',
    'IntersectionObserver', 'MutationObserver', 'queueMicrotask', 'MessageChannel', 'setImmediate',
    'structuredClone', 'requestIdleCallback', 'getComputedStyle', 'matchMedia', 'TextEncoder',
    'WebAssembly', 'fetch', 'Proxy', 'Reflect', 'WeakMap', 'FinalizationRegistry', 'BroadcastChannel'];
  o.apis = {};
  for (var j = 0; j < apis.length; j++) { o.apis[apis[j]] = typeof window[apis[j]]; }

  o.e2eErrors = window.__e2eErrors || null;
  o.missingApiHits = safe(function () { return window.__missingApiHits || null; }, null);

  // 诊断：壳的核心全局 + 已加载的区域 bundle 全局（判断剩余区域为何未装配）
  o.core = safe(function () {
    var c = window.__PAIRCODE_CORE;
    if (!c) { return null; }
    return { type: typeof c, keys: Object.keys(c).slice(0, 60).join(',') };
  }, null);
  o.scripts = safe(function () {
    var out = [], ss = document.getElementsByTagName('script');
    for (var i = 0; i < ss.length; i++) { out.push(String(ss[i].src || '(inline)')); }
    return out;
  }, []);
  o.bundleGlobals = safe(function () {
    var names = ['UiEditor', 'UiSidebar', 'UiTitlebar', 'UiStatusbar', 'UiActivitybar', 'UiModals', 'UiRightPanel'];
    var out = {};
    for (var i = 0; i < names.length; i++) { out[names[i]] = typeof window[names[i]]; }
    return out;
  }, null);
  // 11 个槽位宿主的真实 class + 内容长度：区分「槽位宿主未创建」与「选择器名不符」
  o.slotHosts = safe(function () {
    var out = [], hs = document.querySelectorAll('.plugin-slot-host');
    for (var i = 0; i < hs.length; i++) {
      out.push({ cls: String(hs[i].className || ''), innerLen: (hs[i].innerHTML || '').length });
    }
    return out;
  }, []);
  return JSON.stringify(o);
})()`

// ─── WebView 驱动 ─────────────────────────────────────────────────────────

// perfOneResult 保存 -interaction 单交互模式的结果，并写入报告 JSON ——
// 保证每个性能数字都能在 out/ 下追溯到落盘产物（而非只存在于 stdout）。
var perfOneResult string

func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

func evalStr(wv *webkit.WebView, js string) (string, error) {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

func main() {
	gouide := flag.String("gouide", "", "gouide 仓库根（缺省自动探测 ../gou-ide 等）")
	rounds := flag.Int("rounds", 150, "驱动轮数（每轮含事件循环 + 布局 + 渲染）")
	w := flag.Int("w", 1280, "视口宽")
	h := flag.Int("h", 860, "视口高")
	out := flag.String("out", "out/gouide-real-e2e.json", "JSON 报告输出路径")
	pngOut := flag.String("png", "screenshots/gouide-real-e2e.png", "渲染截图输出路径")
	holdSec := flag.Float64("hold", 0, "装载后再等 N 秒（给异步插件加载留时间）")
	cpuProfile := flag.String("cpuprofile", "", "CPU profile 路径（**只覆盖** -interaction 指定的交互）")
	interaction := flag.String("interaction", "", "只采样该交互：panelSwitch | sessionListUpdate")
	iterations := flag.Int("iterations", 60, "采样时该交互的迭代次数")
	flag.Parse()

	root := *gouide
	if root == "" {
		for _, c := range []string{"../gou-ide", "F:/syproject/gou-ide", "gou-ide"} {
			if _, err := os.Stat(filepath.Join(c, "cmd", "companion", "web-ui", "dist", "index.html")); err == nil {
				root = c
				break
			}
		}
	}
	if root == "" {
		fmt.Println("[FAIL] 未找到 gouide 产物（用 -gouide 指定仓库根）")
		os.Exit(1)
	}
	fmt.Printf("[OK] gouide 根 %s\n", root)

	rl := &reqLog{}
	srv, base, boot, bootNotes, err := startServer(root, rl)
	if err != nil {
		fmt.Printf("[FAIL] 静态服务：%v\n", err)
		os.Exit(1)
	}
	defer func() { _ = srv.Close() }()
	fmt.Printf("[OK] 静态服务 %s（dist + /plugins-assets → .pair/plugins；/api/ui-boot 桩 %d 个 entry）\n", base, len(boot.Entries))
	for _, n := range bootNotes {
		fmt.Printf("     跳过：%s\n", n)
	}

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	// 页面脚本执行前注入错误采集器
	webkit.BeforePageScripts = func(rt *jsc.Interpreter) {
		if _, err := rt.RunJS(jsInjectErrors); err != nil {
			fmt.Printf("[WARN] 错误采集器注入失败：%v\n", err)
		}
	}

	t0 := time.Now()
	if err := wv.LoadURL(base); err != nil {
		fmt.Printf("[FAIL] LoadURL(%s)：%v\n", base, err)
		os.Exit(1)
	}
	loadMs := float64(time.Since(t0).Microseconds()) / 1000
	fmt.Printf("[OK] LoadURL 完成 %.1fms\n", loadMs)

	firstPaintMs := 0.0
	for i := 0; i < *rounds; i++ {
		pumpFrame(wv)
		// 冷启动到首帧：主容器首次出现挂载内容（此刻已完成一帧渲染）
		if firstPaintMs == 0 {
			s, perr := evalStr(wv, `(function(){var a=document.getElementById('app');return (a&&a.children.length>0)?String((a.innerHTML||'').length):'0';})()`)
			if perr == nil {
				if t := strings.TrimSpace(s); t != "0" && t != "" {
					firstPaintMs = float64(time.Since(t0).Microseconds()) / 1000
					fmt.Printf("[OK] 冷启动到首帧 %.1fms（#app innerHTML=%s 字符）\n", firstPaintMs, t)
				}
			}
		}
	}
	if *holdSec > 0 {
		deadline := time.Now().Add(time.Duration(*holdSec * float64(time.Second)))
		for time.Now().Before(deadline) {
			pumpFrame(wv)
		}
	}
	driveMs := float64(time.Since(t0).Microseconds()) / 1000
	fmt.Printf("[OK] 驱动 %d 轮完成，累计 %.1fms\n", *rounds, driveMs)

	raw, err := evalStr(wv, jsCollect)
	if err != nil {
		fmt.Printf("[FAIL] 采集：%v\n", err)
		os.Exit(1)
	}
	var coll map[string]any
	if err := json.Unmarshal([]byte(raw), &coll); err != nil {
		fmt.Printf("[FAIL] 采集 JSON 解析：%v（原始：%s）\n", err, truncate(raw, 400))
		os.Exit(1)
	}

	// ─── 性能埋点：3 类真实交互（面板切换 / 编辑器滚动 / 会话列表更新）───
	// （线 1）单交互采样模式：只跑 -interaction 指定交互并采 CPU profile，
	// 使样本**全部**落在「交互 + 同步布局」上，不被其它交互稀释。
	if *interaction != "" {
		var pf *os.File
		if *cpuProfile != "" {
			var perr error
			pf, perr = os.Create(*cpuProfile)
			if perr != nil {
				fmt.Printf("[FAIL] profile 文件创建：%v\n", perr)
				os.Exit(1)
			}
			if serr := pprof.StartCPUProfile(pf); serr != nil {
				fmt.Printf("[FAIL] StartCPUProfile：%v\n", serr)
				os.Exit(1)
			}
			fmt.Printf("[profile] 采样 %s ×%d → %s\n", *interaction, *iterations, *cpuProfile)
		} else {
			// 未指定 profile 路径 → 只测耗时（用于 A/B 对照，如 GOGC 调参）
			fmt.Printf("[perf] 只测耗时 %s ×%d（未采样）\n", *interaction, *iterations)
		}
		if s, eerr := evalStr(wv, jsPerfOne(*interaction, *iterations)); eerr != nil {
			fmt.Printf("[FAIL] 交互执行：%v\n", eerr)
		} else {
			fmt.Printf("[perf] 交互执行：%s\n", s)
		}
		// 驱动若干帧：让交互引发的异步链（promise / 微任务 / 定时器 / 网络回调）
		// 有机会推进，随后读回的 promise 状态才是最终态（pending 即「永不结算」的硬证据）。
		for i := 0; i < 60; i++ {
			pumpFrame(wv)
		}
		if pf != nil {
			pprof.StopCPUProfile()
			_ = pf.Close()
		}
		if oneRaw, oerr := evalStr(wv, `JSON.stringify(window.__perfOne || null)`); oerr == nil {
			perfOneResult = oneRaw
			fmt.Printf("[perf] 单交互结果 %s\n", oneRaw)
		}
	}

	perfT0 := time.Now()
	if s, perr := evalStr(wv, jsPerf); perr != nil {
		fmt.Printf("[WARN] 交互埋点注入失败：%v\n", perr)
	} else {
		fmt.Println("[perf] 交互埋点:", s)
	}
	for i := 0; i < 30; i++ {
		pumpFrame(wv)
	}
	perfRaw, _ := evalStr(wv, jsPerfCollect)
	var perfWrap map[string]any
	if err := json.Unmarshal([]byte(perfRaw), &perfWrap); err != nil {
		perfWrap = map[string]any{"parseErr": err.Error(), "raw": truncate(perfRaw, 300)}
	}
	perfHarnessMs := float64(time.Since(perfT0).Microseconds()) / 1000
	fmt.Printf("[perf] 埋点完成（%.1fms）\n", perfHarnessMs)

	// 截图
	pngOK := false
	if pix, rerr := wv.Render(); rerr == nil && len(pix) > 0 {
		if err := os.MkdirAll(filepath.Dir(*pngOut), 0o755); err == nil {
			if werr := os.WriteFile(*pngOut, pix, 0o644); werr == nil {
				pngOK = true
				fmt.Printf("[png] %s（%.0fKB）\n", *pngOut, float64(len(pix))/1024)
			}
		}
	}
	if !pngOK {
		fmt.Println("[WARN] 截图失败")
	}

	// 判定：根容器有子节点且 innerHTML 非空 = 挂载成功
	childCount, _ := coll["appChildCount"].(float64)
	innerLen, _ := coll["appInnerLen"].(float64)
	mounted := childCount > 0 && innerLen > 0

	consoleOut := strings.TrimSpace(wv.ConsoleOutput())
	missingPaths := rl.missing()

	// 「性能满足」的量化验收线（依据见 basis，参照系为 39763 元素的合成压测页）
	acceptance := map[string]any{
		"coldStartToFirstPaintMs": map[string]any{
			"target": 1500,
			"basis": "参照系：39763 元素的 React 合成压测页首屏 3753ms、Vue dev 合成页 9535ms / prod 10245ms；" +
				"真实产物稳态 DOM 仅约 4 百元素（小两个数量级）且壳为构建产物，故取 ≤1500ms（约 React 合成页的 40%）",
		},
		"panelSwitchPerOpMs": map[string]any{
			"target": 50,
			"basis":  "浏览器「长任务」阈值 50ms（超过即可感知卡顿）；面板切换 = 单次 DOM patch + 一次同步布局",
		},
		"editorScrollPerOpMs": map[string]any{
			"target": 16.7,
			"basis":  "60fps 单帧预算（高频连续交互需达帧率）",
		},
		"sessionListUpdatePerOpMs": map[string]any{
			"target": 50,
			"basis":  "同长任务阈值；会话列表更新含 Vue 响应式 patch + 布局",
		},
	}

	report := map[string]any{
		"generatedAt":  time.Now().Format(time.RFC3339),
		"gouideRoot":   root,
		"baseURL":      base,
		"viewport":     map[string]int{"w": *w, "h": *h},
		"rounds":       *rounds,
		"loadMs":       loadMs,
		"driveTotalMs": driveMs,
		"mounted":      mounted,
		"perf": map[string]any{
			"coldStartToFirstPaintMs": firstPaintMs,
			"bundleLoadMs":            loadMs,
			"domCount":                coll["totalElements"],
			"interactionHarnessMs":    perfHarnessMs,
			"interactions":            perfWrap,
		},
		"acceptance":     acceptance,
		"oneInteraction": perfOneResult,
		"dom":            coll,
		"serverStubs": map[string]any{
			"uiBootEntries": len(boot.Entries),
			"uiBootGraph":   boot,
			"skipped":       bootNotes,
		},
		"http": map[string]any{
			"ok": rl.ok, "miss": rl.miss, "missing": missingPaths, "all": rl.items,
		},
		"console": consoleOut,
		"screenshot": map[string]any{
			"path": *pngOut, "ok": pngOK,
		},
	}

	if *out != "" {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err == nil {
			b, _ := json.MarshalIndent(report, "", "  ")
			if werr := os.WriteFile(*out, append(b, '\n'), 0o644); werr != nil {
				fmt.Printf("[WARN] 写报告失败：%v\n", werr)
			} else {
				fmt.Printf("[OK] 报告 %s\n", *out)
			}
		}
	}

	// ─── 控制台摘要（人读）──────────────────────────────────────────────
	fmt.Println("=== 判定 ===")
	fmt.Printf("  挂载成功         : %v（#app children=%v, innerHTML=%v 字符）\n", mounted, coll["appChildCount"], coll["appInnerLen"])
	fmt.Printf("  页面元素总数     : %v\n", coll["totalElements"])
	fmt.Printf("  document.title   : %v\n", coll["docTitle"])
	fmt.Printf("  #app Vue 内部标记: %v\n", coll["appVueKeys"])
	if sels, ok := coll["selectors"].(map[string]any); ok {
		keys := make([]string, 0, len(sels))
		for k := range sels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Println("  关键区域节点：")
		for _, k := range keys {
			fmt.Printf("    %-16s %v\n", k, sels[k])
		}
	}
	if e, ok := coll["e2eErrors"].(map[string]any); ok {
		fmt.Println("  未捕获错误：")
		fmt.Printf("    error 事件      : %v\n", e["errors"])
		fmt.Printf("    unhandledreject : %v\n", e["rejections"])
		fmt.Printf("    console.error   : %v\n", e["consoleError"])
	}
	fmt.Printf("  HTTP: 200×%d, 非 200×%d\n", rl.ok, rl.miss)
	if len(missingPaths) > 0 {
		fmt.Println("  缺失资源（前 15 条）：")
		for i, m := range missingPaths {
			if i >= 15 {
				break
			}
			fmt.Printf("    %v → %v\n", m["path"], m["status"])
		}
	}
	if apis, ok := coll["apis"].(map[string]any); ok {
		var undef []string
		for k, v := range apis {
			if v == "undefined" {
				undef = append(undef, k)
			}
		}
		sort.Strings(undef)
		fmt.Printf("  undefined API    : %v\n", undef)
	}
	fmt.Println("=== console 输出 ===")
	if consoleOut == "" {
		fmt.Println("(无输出)")
	} else {
		fmt.Println(truncate(consoleOut, 3000))
	}
}

// jsPerf 真实交互性能埋点：先探测交互入口（可点击元素 / 可滚动容器），
// 再对 3 类典型 IDE 交互计时。每类给两组数：
//
//	op           —— 交互本身（含引擎内部的样式/布局失效标记）
//	opWithLayout —— 交互后再读一次布局属性（真实交互通常紧接布局读取）
//
// 每类首轮做一次「预热」不计入：插件/组件首次渲染与懒初始化属冷启动成本，
// 不应混入稳态交互耗时（冷启动另有 coldStartToFirstPaintMs 指标）。
const jsPerf = `(function () {
  function safe(fn, dflt) {
    try { var v = fn(); return (v === undefined || v === null) ? dflt : v; }
    catch (e) { return 'ERR:' + String((e && e.message) || e); }
  }
  var now = function () { return performance.now(); };
  var P = { env: {}, interactions: {} };
  window.__perf = P;

  function mesure(n, fn) {
    var t0 = now();
    for (var i = 0; i < n; i++) { fn(i); }
    var t1 = now();
    return { n: n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2) };
  }
  function click(el) {
    if (!el) { return false; }
    try {
      if (typeof el.click === 'function') { el.click(); return true; }
      if (el.dispatchEvent) { el.dispatchEvent(new Event('click', { bubbles: true })); return true; }
    } catch (e) {}
    return false;
  }

  P.env.scrollers = safe(function () {
    var out = [], all = document.getElementsByTagName('*');
    for (var i = 0; i < all.length && out.length < 12; i++) {
      var e = all[i];
      if (e.scrollHeight > e.clientHeight + 8 && e.clientHeight > 24) {
        out.push({ cls: String(e.className || '').slice(0, 48), scrollH: e.scrollHeight, clientH: e.clientHeight });
      }
    }
    return out;
  }, null);
  P.env.clickables = safe(function () {
    var hosts = ['.plugin-area-activitybar', '.plugin-area-titlebar', '.plugin-area-sidebar', '.plugin-area-conversation'];
    var out = [];
    for (var h = 0; h < hosts.length; h++) {
      var host = document.querySelector(hosts[h]);
      if (!host) { out.push({ host: hosts[h], found: false }); continue; }
      var cand = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="tab"], [class*="icon"], [class*="btn"]');
      out.push({
        host: hosts[h], found: true, innerLen: (host.innerHTML || '').length, candidates: cand.length,
        sample: Array.prototype.slice.call(cand, 0, 5).map(function (e) {
          return { tag: e.tagName, cls: String(e.className || '').slice(0, 34),
                   txt: ((e.textContent || '').replace(/\s+/g, ' ')).trim().slice(0, 20) };
        })
      });
    }
    return out;
  }, null);

  // ── 交互 ①：顶部/侧栏面板切换 ──
  P.interactions.panelSwitch = safe(function () {
    var host = document.querySelector('.plugin-area-activitybar');
    if (!host) { return { err: 'no activitybar host' }; }
    var cand = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="tab"], [class*="icon"], [class*="btn"]');
    if (!cand.length) { cand = host.children; }
    if (!cand.length) { return { err: 'no candidates in activitybar' }; }
    var pick = function (i) { return cand[i % cand.length]; };
    click(pick(0)); void host.offsetHeight;
    var op = mesure(12, function (i) { click(pick(i + 1)); });
    var opLayout = mesure(12, function (i) { click(pick(i + 1)); void host.offsetHeight; });
    return { candidates: cand.length, op: op, opWithLayout: opLayout };
  }, null);

  // ── 交互 ②：编辑器区域滚动 ──
  P.interactions.editorScroll = safe(function () {
    var host = document.querySelector('.plugin-area-editor') || document.querySelector('.editor-container');
    var scroller = null, note = '';
    if (host) {
      if (host.scrollHeight > host.clientHeight + 8 && host.clientHeight > 24) { scroller = host; }
      var cand = host.querySelectorAll('*');
      for (var i = 0; !scroller && i < cand.length; i++) {
        if (cand[i].scrollHeight > cand[i].clientHeight + 8 && cand[i].clientHeight > 24) { scroller = cand[i]; }
      }
    }
    if (!scroller) {
      // 编辑器为空（真实产物启动时未打开文件）→ 回落到页面最大的可滚动容器，
      // 保证「滚动 + 布局读取」这条路径仍被度量到（原因记入 note）。
      var all = document.getElementsByTagName('*'), best = null;
      for (var j = 0; j < all.length; j++) {
        var e = all[j];
        if (e.scrollHeight > e.clientHeight + 8 && e.clientHeight > 24) {
          if (!best || (e.scrollHeight - e.clientHeight) > (best.scrollHeight - best.clientHeight)) { best = e; }
        }
      }
      scroller = best;
      note = 'fallback：editor 为空（未打开文件）→ 改用页面最大可滚动容器';
    }
    if (!scroller) { return { err: 'no scrollable container on page' }; }
    var step = Math.max(1, Math.floor((scroller.scrollHeight - scroller.clientHeight) / 12));
    scroller.scrollTop = 0; void scroller.offsetHeight;
    var op = mesure(12, function (i) { scroller.scrollTop = (i % 12) * step; });
    var opLayout = mesure(12, function (i) { scroller.scrollTop = (i % 12) * step; void scroller.offsetHeight; });
    return { cls: String(scroller.className || '').slice(0, 40), scrollH: scroller.scrollHeight,
             clientH: scroller.clientHeight, note: note, op: op, opWithLayout: opLayout };
  }, null);

  // ── 交互 ③：对话/会话列表更新 ──
  P.interactions.sessionListUpdate = safe(function () {
    var host = document.querySelector('.plugin-area-conversation') || document.querySelector('.conversation-container');
    if (!host) { return { err: 'no conversation host' }; }
    var items = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="session"], [class*="row"], li');
    if (!items.length) { return { err: 'no session items', innerLen: (host.innerHTML || '').length }; }
    click(items[0]); void host.offsetHeight;
    var op = mesure(10, function (i) { click(items[i % items.length]); });
    var opLayout = mesure(10, function (i) { click(items[i % items.length]); void host.offsetHeight; });
    return { candidates: items.length, op: op, opWithLayout: opLayout };
  }, null);

  return 'started';
})()`

// jsPerfOne 生成「只跑指定交互 N 次」的脚本，供单独 CPU 采样用（profile 仅覆盖该交互）。
// 每次交互后立即读一次布局属性 —— 与真实交互「改完就读布局」的语义一致。
func jsPerfOne(name string, iters int) string {
	return `(function () {
  var now = function () { return performance.now(); };
  function click(el) {
    if (!el) { return false; }
    try {
      if (typeof el.click === 'function') { el.click(); return true; }
      if (el.dispatchEvent) { el.dispatchEvent(new Event('click', { bubbles: true })); return true; }
    } catch (e) {}
    return false;
  }
  var name = '` + name + `', iters = ` + itoa(iters) + `;
  var host = null, pick = null;
  if (name === 'panelSwitch') {
    host = document.querySelector('.plugin-area-activitybar');
    if (!host) { return 'ERR:no-activitybar'; }
    var cand = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="tab"], [class*="icon"], [class*="btn"]');
    if (!cand.length) { cand = host.children; }
    pick = function (i) { return cand[i % cand.length]; };
  } else if (name === 'sessionListUpdate') {
    host = document.querySelector('.plugin-area-conversation') || document.querySelector('.conversation-container');
    if (!host) { return 'ERR:no-conversation'; }
    var items = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="session"], [class*="row"], li');
    if (!items.length) { return 'ERR:no-items'; }
    pick = function (i) { return items[i % items.length]; };
  } else {
    if (name === 'sessionListSplit') {
      // B-1：同一序列内分解「首交互 1 次」与「稳态 N 次」——
      // 首交互含首次点击触发的完整重建（预热效应），稳态则不含。
      host = document.querySelector('.plugin-area-conversation') || document.querySelector('.conversation-container');
      if (!host) { return 'ERR:no-conversation'; }
      var itemsS = host.querySelectorAll('button, [role="button"], [class*="item"], [class*="session"], [class*="row"], li');
      if (!itemsS.length) { return 'ERR:no-items'; }
      var tF = now();
      click(itemsS[0]); void host.offsetHeight;
      var firstMs = now() - tF;
      var tS = now();
      for (var si = 1; si <= iters; si++) { click(itemsS[si % itemsS.length]); void host.offsetHeight; }
      var steadyTotal = now() - tS;
      window.__perfOne = { name: name, candidates: itemsS.length,
                           firstMs: +firstMs.toFixed(3),
                           steadyIters: iters, steadyTotalMs: +steadyTotal.toFixed(2),
                           steadyPerOpMs: +(steadyTotal / iters).toFixed(3) };
      return 'done';
    } else if (name === 'editorInput') {
      // C：输入路径（等价负载）—— 向注入的文档节点写 textContent + 强制布局，
      // 语义等同「编辑器每敲一键触发文本变更 → 重排」。
      var edhI = document.querySelector('.plugin-area-editor') || document.querySelector('.editor-container');
      var targetI = '';
      if (edhI && edhI.clientHeight > 24) { host = edhI; targetI = 'editor-container(active)'; }
      if (!host) {
        var allI = document.getElementsByTagName('*'), bestI = null;
        for (var qi = 0; qi < allI.length; qi++) {
          var eqi = allI[qi];
          if (eqi.scrollHeight > eqi.clientHeight + 8 && eqi.clientHeight > 24) {
            if (!bestI || (eqi.scrollHeight - eqi.clientHeight) > (bestI.scrollHeight - bestI.clientHeight)) { bestI = eqi; }
          }
        }
        host = bestI; targetI = bestI ? ('real-scrollable:' + String(bestI.className || '').slice(0, 40)) : '';
      }
      // 兜底：单独跑本交互时页面上可能没有任何可滚动容器（market-panel 只有点击后才出现），
      // 此时挂到 body（自带尺寸）—— 保证测量不依赖面板状态。
      if (!host) { host = document.body; targetI = 'body(fallback)'; }
      var docI = document.createElement('div');
      docI.className = 'code seeded-doc';
      docI.style.height = '200px';
      docI.style.overflow = 'auto';
      var lineI = document.createElement('div');
      lineI.className = 'line';
      var spI = document.createElement('span');
      spI.className = 't-ident';
      spI.textContent = 'init';
      lineI.appendChild(spI);
      docI.appendChild(lineI);
      host.appendChild(docI);
      var opI = mesure(iters, function (i) {
        spI.textContent = 'const v' + i + ' = ' + ((i * 7919) % 9973) + ';';
        void docI.offsetHeight;
      });
      window.__perfOne = { name: name, target: targetI, seeded: true, op: opI,
                           note: '输入路径等价负载：写 textContent + 强制布局' };
      return 'done';
    } else if (name === 'modalOpen') {
      // C：命令面板/模态打开路径 —— 点 titlebar 菜单按钮（打开下拉/模态）+ 强制布局。
      var tbM = document.querySelector('.plugin-area-titlebar');
      var btnsM = tbM ? tbM.querySelectorAll('button') : [];
      if (!btnsM.length) { return 'ERR:no-titlebar-buttons'; }
      var hostM = document.querySelector('.plugin-area-modals') || document.querySelector('.modals-host') || tbM;
      click(btnsM[0]); void hostM.offsetHeight;
      var opM = mesure(iters, function (i) { click(btnsM[i % btnsM.length]); void hostM.offsetHeight; });
      window.__perfOne = { name: name, candidates: btnsM.length, op: opM,
                           modalInnerLen: (hostM.innerHTML || '').length };
      return 'done';
    } else if (name === 'editorActivate') {
      // 驱动真实「打开文件」链路的第一步：调用壳的 loadFileTree（coreActions），
      // 由它去请求后端文件列表 → 文件树出现条目 → 后续点击条目打开文件。
      var rA = { steps: [], err: null, tree: null, opened: null };
      try {
        var C3 = window.__PAIRCODE_CORE;
        if (C3 && C3.actions && typeof C3.actions.loadFileTree === 'function') {
          // 跟踪 promise 最终状态（对象引用 → 后续读回可拿到已更新的 state）。
          // 这是「打开文件链路是否可达」的**硬证据**：pending=永不结算（依赖未接通的通道）、
          // rejected=精确报错、resolved=拿到了数据。
          // 三步链：① 读工作区列表 → ② 切换工作区 → ③ 加载文件树。
          // 每步记录**精确返回值**（这是「链路是否可达」的硬证据，替代推断）。
          var stA = { state: 'pending', tries: [] };
          var WS = 'F:/syproject/gou-ide';
          try {
            if (typeof C3.actions.loadWsList === 'function') {
              try {
                var r1 = C3.actions.loadWsList();
                stA.tries.push('loadWsList → ' + ((r1 && typeof r1.then === 'function') ? 'promise' : String(r1).slice(0, 80)));
              } catch (e) { stA.tries.push('loadWsList threw:' + String((e && e.message) || e)); }
            } else { stA.tries.push('no loadWsList'); }
            if (typeof C3.actions.switchWorkspace === 'function') {
              try {
                var r2 = C3.actions.switchWorkspace(WS);
                stA.tries.push('switchWorkspace(' + WS + ') → ' + ((r2 && typeof r2.then === 'function') ? 'promise' : String(r2).slice(0, 80)));
              } catch (e) { stA.tries.push('switchWorkspace threw:' + String((e && e.message) || e)); }
            } else { stA.tries.push('no switchWorkspace'); }
            var pr = C3.actions.loadFileTree(WS);
            if (pr && typeof pr.then === 'function') {
              stA.tries.push('loadFileTree(' + WS + ') → promise');
              pr.then(function (v) {
                stA.state = 'resolved:' + String(JSON.stringify(v)).slice(0, 300);
                stA.tries.push('loadFileTree resolved');
              }, function (e) {
                stA.state = 'rejected:' + String((e && (e.stack || e.message)) || e);
                stA.tries.push('loadFileTree rejected');
              });
            } else { stA.tries.push('loadFileTree → ' + typeof pr); stA.state = 'not-promise'; }
          } catch (e) { stA.state = 'throw:' + String((e && e.stack) || e); }
          rA.promise = stA;
        } else {
          rA.steps.push('no loadFileTree action');
        }
      } catch (e) { rA.err = String((e && e.stack) || e); }
      var tl = document.querySelector('.plugin-area-sidebar') || document.querySelector('.explorer');
      rA.tree = tl ? { cls: String(tl.className), innerLen: (tl.innerHTML || '').length,
                       text: ((tl.textContent || '').replace(/\s+/g, ' ')).trim().slice(0, 160) } : null;
      window.__perfOne = { name: name, r: rA };
      return 'done';
    } else if (name === 'editorProbe') {
      // 编辑器激活入口探测（证据驱动，先探清可调用面再动手）：
      //   ① 可用全局与 __PAIRCODE_CORE 的 actions/api（含 fs）
      //   ② 编辑器容器的真实尺寸（clientH/scrollH —— 判定面板是否已激活可见）
      //   ③ Vue 组件链的 setupState/ctx 键名（定位 openFiles/fileContents/activeFile）
      // prod 构建下 el.__vueParentComponent 可能不存在，故同时尝试 el.__vnode.component。
      var r = { globals: {}, core: null, coreActions: null, coreApi: null, coreApiFs: null,
                uiEditor: null, host: null, hasVueComp: false, vueChain: [], err: null };
      try {
        var gs = ['UiEditor', 'monaco', 'CodeMirror', 'cm', '__PAIRCODE_CORE', 'paircodeHost'];
        for (var g = 0; g < gs.length; g++) { r.globals[gs[g]] = typeof window[gs[g]]; }
        var CORE = window.__PAIRCODE_CORE;
        if (CORE) {
          r.core = Object.keys(CORE).slice(0, 40);
          r.coreActions = (CORE.actions && typeof CORE.actions === 'object') ? Object.keys(CORE.actions).slice(0, 60) : null;
          r.coreApi = (CORE.api && typeof CORE.api === 'object') ? Object.keys(CORE.api).slice(0, 60) : null;
          if (CORE.api && CORE.api.fs && typeof CORE.api.fs === 'object') { r.coreApiFs = Object.keys(CORE.api.fs).slice(0, 30); }
        }
        if (window.UiEditor && typeof window.UiEditor === 'object') { r.uiEditor = Object.keys(window.UiEditor).slice(0, 60); }
        var elE = document.querySelector('.editor-container') || document.querySelector('.plugin-area-editor');
        if (elE) {
          r.host = { cls: String(elE.className), innerLen: (elE.innerHTML || '').length,
                     clientH: elE.clientHeight, scrollH: elE.scrollHeight, children: elE.children.length };
          var vc = elE.__vueParentComponent || (elE.__vnode && elE.__vnode.component) || null;
          r.hasVueComp = !!vc;
          var cur = vc, depth = 0;
          while (cur && depth < 10) {
            r.vueChain.push({
              d: depth,
              name: (cur.type && (cur.type.name || cur.type.__name)) || '?',
              setup: cur.setupState ? Object.keys(cur.setupState).slice(0, 30) : [],
              ctx: cur.ctx ? Object.keys(cur.ctx).slice(0, 30) : []
            });
            cur = cur.parent; depth++;
          }
        }
      } catch (e) { r.err = String((e && e.stack) || e); }
      window.__perfOne = { name: name, probe: r };
      return 'done';
    } else if (name === 'editorScrollSeeded') {
      // 真实产物启动态未打开文件 → 编辑器为空（不可滚动，滚动性能是盲区）。
      // 这里注入**等价的长文档 DOM**（200 行，形态对齐 idepage 压测页代码视图：
      // .line + .ln + token span），使「长文档滚动 + 强制布局」路径可度量。
      // 标注 seeded=true —— 非真实 openFile 路径，是等价负载。
      // 目标容器选择（记录 target 以便追溯）：
      //   ① 编辑器容器若已激活可见（clientHeight>24）→ 直接用它；
      //   ② 否则回落到**页面真实可滚动容器**（如 market-panel）—— 保证 scrollH>0、
      //      滚动是真实发生的（而非注入到高度为 0 的隐藏容器里）。
      var edh = document.querySelector('.plugin-area-editor') || document.querySelector('.editor-container');
      var target = '';
      if (edh && edh.clientHeight > 24) { host = edh; target = 'editor-container(active)'; }
      if (!host) {
        var allQ = document.getElementsByTagName('*'), bestQ = null;
        for (var q = 0; q < allQ.length; q++) {
          var eq = allQ[q];
          if (eq.scrollHeight > eq.clientHeight + 8 && eq.clientHeight > 24) {
            if (!bestQ || (eq.scrollHeight - eq.clientHeight) > (bestQ.scrollHeight - bestQ.clientHeight)) { bestQ = eq; }
          }
        }
        host = bestQ;
        target = bestQ ? ('real-scrollable:' + String(bestQ.className || '').slice(0, 40)) : '';
      }
      if (!host) { host = document.body; target = 'body(fallback)'; }
      var doc = document.createElement('div');
      doc.className = 'code seeded-doc';
      doc.style.overflow = 'auto';
      doc.style.height = '400px';
      var frag = document.createDocumentFragment();
      for (var i = 0; i < 200; i++) {
        var line = document.createElement('div');
        line.className = 'line';
        var ln = document.createElement('span');
        ln.className = 'ln';
        ln.textContent = String(i + 1);
        line.appendChild(ln);
        var sp = document.createElement('span');
        sp.className = 'tok t-ident';
        sp.textContent = 'const value' + i + ' = compute(' + i + ');';
        line.appendChild(sp);
        frag.appendChild(line);
      }
      doc.appendChild(frag);
      host.appendChild(doc);
      var maxScroll = Math.max(1, doc.scrollHeight - doc.clientHeight);
      var step = Math.max(1, Math.floor(maxScroll / 12));
      doc.scrollTop = 0; void doc.offsetHeight;
      var t0s = now();
      for (var s = 0; s < iters; s++) { doc.scrollTop = (s % 12) * step; void doc.offsetHeight; }
      var tot = now() - t0s;
      window.__perfOne = { name: name, iters: iters, totalMs: +tot.toFixed(2),
                           perOpMs: +(tot / iters).toFixed(3), scrollH: doc.scrollHeight,
                           clientH: doc.clientHeight, seeded: true, target: target,
                           note: '注入 200 行等价长文档到**真实可滚动容器**（编辑器未激活时的等价负载）；target 见字段' };
      return 'done';
    }
    return 'ERR:unknown-interaction:' + name;
  }
  click(pick(0)); void host.offsetHeight;
  var t0 = now();
  for (var i = 0; i < iters; i++) {
    click(pick(i + 1));
    void host.offsetHeight;
  }
  var total = now() - t0;
  window.__perfOne = { name: name, iters: iters, totalMs: +total.toFixed(2), perOpMs: +(total / iters).toFixed(3) };
  return 'done';
})()`
}

func itoa(n int) string { return strconv.Itoa(n) }

// jsPerfCollect 读回埋点结果，并确认交互后 DOM 仍正常。
const jsPerfCollect = `(function () {
  try {
    var p = window.__perf || null;
    var appEl = document.getElementById('app');
    return JSON.stringify({
      perf: p,
      after: {
        appInnerLen: appEl ? (appEl.innerHTML || '').length : -1,
        totalElements: document.getElementsByTagName('*').length,
        title: document.title
      }
    });
  } catch (e) { return 'null'; }
})()`

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…（截断）"
}
