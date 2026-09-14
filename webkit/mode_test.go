package webkit

// 运行模式（Mode）测试：默认值与锁定语义、两种模式的 JS 能力面、
// UI 库模式的网络门禁/外部资源门禁/子框架装配差异。
//
// 模式 = 装配策略（见 mode.go / docs/MODES.md）：ModeBrowser 是历史
// 行为（回归基准），ModeToolkit 裁剪浏览器专属的「外部输入」。

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wb-ui/bridge"
	"wb-ui/engine/page"
)

const modeMinimalHTML = `<!DOCTYPE html><html><head></head><body><div id="box">x</div></body></html>`

// modeWebView 创建指定模式的 WebView 并在测试结束销毁。
func modeWebView(t *testing.T, mode Mode) *WebView {
	t.Helper()
	wv := NewWebViewWithMode(mode)
	t.Cleanup(func() { wv.Destroy() })
	wv.Resize(400, 300)
	return wv
}

func modeMustLoad(t *testing.T, wv *WebView, src string) {
	t.Helper()
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
}

// modeEval 执行 JS 返回字符串结果。
func modeEval(t *testing.T, wv *WebView, script string) string {
	t.Helper()
	v, err := wv.EvalJS(script)
	if err != nil {
		t.Fatalf("EvalJS(%q): %v", script, err)
	}
	// jsc.JSValue 的 ToString 已处理 undefined/null/零值（返回 ""）。
	return v.ToString()
}

// modeSettle 执行脚本（脚本把结果写进 window.__probe），驱动事件循环
// 直到探针不再是 "pending"（引擎的 promise 可能是同步 settle 的，也可能
// 需要一次事件循环；两种实现都要能测）。
func modeSettle(t *testing.T, wv *WebView, script string) string {
	t.Helper()
	if _, err := wv.EvalJS(script); err != nil {
		t.Fatalf("EvalJS(%q): %v", script, err)
	}
	for i := 0; i < 8; i++ {
		got := modeEval(t, wv, `String(window.__probe)`)
		if got != "pending" {
			return got
		}
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
	}
	return modeEval(t, wv, `String(window.__probe)`)
}

// ── 模式基本语义 ─────────────────────────────────────────

func TestModeDefaultIsBrowser(t *testing.T) {
	wv := NewWebView()
	t.Cleanup(func() { wv.Destroy() })
	if got := wv.Mode(); got != ModeBrowser {
		t.Fatalf("NewWebView().Mode() = %v, want ModeBrowser", got)
	}
	if ModeBrowser.String() != "browser" || ModeToolkit.String() != "toolkit" {
		t.Fatalf("Mode.String(): %q / %q", ModeBrowser.String(), ModeToolkit.String())
	}
}

func TestModeLockedAfterLoad(t *testing.T) {
	wv := modeWebView(t, ModeBrowser)
	// 装配前可自由设置（含改回）。
	if err := wv.SetMode(ModeToolkit); err != nil {
		t.Fatalf("SetMode(toolkit) before load: %v", err)
	}
	if wv.Mode() != ModeToolkit {
		t.Fatal("装配前 SetMode 未生效")
	}
	if err := wv.SetMode(ModeBrowser); err != nil {
		t.Fatalf("SetMode(browser) before load: %v", err)
	}

	modeMustLoad(t, wv, modeMinimalHTML)
	// 装配后：切换被拒，同值是 no-op。
	if err := wv.SetMode(ModeToolkit); !errors.Is(err, ErrModeLocked) {
		t.Fatalf("装配后切换模式 err = %v, want ErrModeLocked", err)
	}
	if wv.Mode() != ModeBrowser {
		t.Fatalf("被拒的 SetMode 改变了模式：%v", wv.Mode())
	}
	if err := wv.SetMode(ModeBrowser); err != nil {
		t.Fatalf("同值 SetMode 应为 no-op：%v", err)
	}
}

// ── JS 能力面 ────────────────────────────────────────────

func modeProbeAPISurface(t *testing.T, mode Mode) map[string]string {
	t.Helper()
	wv := modeWebView(t, mode)
	modeMustLoad(t, wv, modeMinimalHTML)
	raw := modeEval(t, wv, `JSON.stringify({
		fetch: typeof fetch,
		xhr: typeof XMLHttpRequest,
		worker: typeof Worker,
		ws: typeof WebSocket,
		workerIn: ("Worker" in window) ? "true" : "false",
		wsIn: ("WebSocket" in window) ? "true" : "false",
		xhrIn: ("XMLHttpRequest" in window) ? "true" : "false",
		document: typeof document,
		window: typeof window,
		querySelector: typeof document.querySelector,
		matchMedia: typeof matchMedia
	})`)
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("能力面探测结果不是 JSON：%q (%v)", raw, err)
	}
	return m
}

// TestModeBrowserAPISurface：浏览器模式必须保留全部浏览器能力（回归）。
func TestModeBrowserAPISurface(t *testing.T) {
	m := modeProbeAPISurface(t, ModeBrowser)
	for _, k := range []string{"fetch", "xhr", "worker", "ws"} {
		if m[k] == "undefined" || m[k] == "" {
			t.Errorf("ModeBrowser 下 %s = %q，应存在", k, m[k])
		}
	}
	if m["document"] != "object" || m["querySelector"] != "function" {
		t.Errorf("ModeBrowser 下 DOM 绑定缺失：document=%q querySelector=%q", m["document"], m["querySelector"])
	}
	// 浏览器模式下 `in` 判定与实际能力一致（属性真的存在）。
	for _, k := range []string{"workerIn", "wsIn", "xhrIn"} {
		if m[k] != "true" {
			t.Errorf("ModeBrowser 下 %s = %q，want true", k, m[k])
		}
	}
}

// TestModeToolkitAPISurface：UI 库模式保留 DOM/fetch（宿主桥），
// 隐藏 XHR/Worker/WebSocket（浏览器专属）。
func TestModeToolkitAPISurface(t *testing.T) {
	m := modeProbeAPISurface(t, ModeToolkit)
	if m["fetch"] != "function" {
		t.Errorf("ModeToolkit 下 fetch = %q，应保留（宿主桥路由取数据用）", m["fetch"])
	}
	for _, k := range []string{"xhr", "worker", "ws"} {
		if m[k] != "undefined" {
			t.Errorf("ModeToolkit 下 %s = %q，应为 undefined", k, m[k])
		}
	}
	// ★ 真删除：`"Worker" in window` 必须是 false（浏览器里该 API 不存在时正是
	//   如此）。置 undefined 只能让 typeof 判定正确，靠 in 做 feature detect
	//   的库仍会误判「有 Worker」。
	for _, k := range []string{"workerIn", "wsIn", "xhrIn"} {
		if m[k] != "false" {
			t.Errorf("ModeToolkit 下 %s = %q，want false（全局应被真删除，不是置 undefined）", k, m[k])
		}
	}
	if m["document"] != "object" || m["querySelector"] != "function" {
		t.Errorf("ModeToolkit 下 DOM 绑定缺失：document=%q querySelector=%q", m["document"], m["querySelector"])
	}
}

// ── 网络门禁 ─────────────────────────────────────────────

// TestToolkitModeFetchRoutesOnly：UI 库模式下 fetch 命中宿主桥路由，
// 未命中路由时 reject（不发起真实网络请求）。
func TestToolkitModeFetchRoutesOnly(t *testing.T) {
	bridge.RegisterHTTP("GET", "/api/mode-probe", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"probe":1}`))
	})
	wv := modeWebView(t, ModeToolkit)
	modeMustLoad(t, wv, modeMinimalHTML)

	// 1) 宿主桥路由：可用
	got := modeSettle(t, wv, `
		window.__probe = "pending";
		fetch("/api/mode-probe").then(function (resp) {
			window.__probe = "route:" + resp.status;
		}, function (e) {
			window.__probe = "route-err:" + String(e && e.message || e);
		});
	`)
	if got != "route:200" {
		t.Errorf("UI 库模式桥路由结果 = %q, want route:200", got)
	}

	// 2) 非路由 URL：拒绝（且错误信息说明是模式禁用）
	got = modeSettle(t, wv, `
		window.__probe = "pending";
		fetch("https://mode-probe.invalid/x").then(function () {
			window.__probe = "resolved";
		}, function (e) {
			window.__probe = "rejected:" + String(e && e.message || e);
		});
	`)
	if !strings.HasPrefix(got, "rejected:") {
		t.Fatalf("UI 库模式外部 fetch = %q, want rejected:…", got)
	}
	if !strings.Contains(got, "UI 库模式") {
		t.Errorf("拒绝原因未说明模式：%q", got)
	}
}

// ── 外部资源门禁 ─────────────────────────────────────────

// modeExternalCSSWidth 用真实存在的本地 CSS 文件量出 #box 的宽度。
// 返回 (宽度, 是否找到盒)。
func modeExternalCSSWidth(t *testing.T, mode Mode, resolver ResourceResolver) float64 {
	t.Helper()
	dir := t.TempDir()
	cssPath := filepath.Join(dir, "mode-theme.css")
	if err := os.WriteFile(cssPath, []byte("#box{width:123px}"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	href := "file:///" + filepath.ToSlash(cssPath)
	src := `<!DOCTYPE html><html><head><link rel="stylesheet" href="` + href + `"></head>` +
		`<body><div id="box">x</div></body></html>`

	wv := modeWebView(t, mode)
	if resolver != nil {
		wv.SetResourceResolver(resolver)
	}
	modeMustLoad(t, wv, src)
	wv.EnsureHitTestReady()
	box := findBox(wv, wv.Document().GetElementById("box"))
	if box == nil {
		t.Fatal("#box 渲染盒缺失")
	}
	return box.W
}

func TestBrowserModeLoadsExternalCSS(t *testing.T) {
	if w := modeExternalCSSWidth(t, ModeBrowser, nil); math.Abs(w-123) > 1 {
		t.Fatalf("#box 宽度 = %.1f，want ≈123（浏览器模式应加载外部 CSS）", w)
	}
}

// TestToolkitModeBlocksExternalCSS：UI 库模式拒绝 http(s)/file 外部资源
// ——同样的 HTML，样式不生效（宽度回落到块级默认 = 容器宽）。
func TestToolkitModeBlocksExternalCSS(t *testing.T) {
	w := modeExternalCSSWidth(t, ModeToolkit, nil)
	if math.Abs(w-123) <= 1 {
		t.Fatalf("#box 宽度 = %.1f：UI 库模式不应加载 file:// 外部样式", w)
	}
}

// TestToolkitModeResourceResolver：外部引用（web 方式）可由宿主资源
// 解析器（基础方式）供应 —— 这就是「某些作为基础、某些以 web 方式提供」
// 的接线：页面照旧写 <link>，内容来自 Go（内存/内嵌）。
func TestToolkitModeResourceResolver(t *testing.T) {
	const logical = "app://mode-theme.css"

	// 1) resolver 全部拒绝（ok=false）→ 外部引用仍被模式门禁挡住
	if w := modeExternalCSSWidth(t, ModeToolkit, func(string) (string, bool) { return "", false }); math.Abs(w-123) <= 1 {
		t.Fatalf("#box 宽度 = %.1f：resolver 未命中时不应加载外部资源", w)
	}

	// 2) resolver 命中逻辑名 → 样式生效（内容来自 Go，不触碰文件系统）
	calls := 0
	wv := modeWebView(t, ModeToolkit)
	wv.SetResourceResolver(func(ref string) (string, bool) {
		if ref == logical {
			calls++
			return "#box{width:123px}", true
		}
		return "", false
	})
	modeMustLoad(t, wv, `<!DOCTYPE html><html><head><link rel="stylesheet" href="`+logical+`"></head>`+
		`<body><div id="box">x</div></body></html>`)
	wv.EnsureHitTestReady()
	box := findBox(wv, wv.Document().GetElementById("box"))
	if box == nil {
		t.Fatal("#box 渲染盒缺失")
	}
	if math.Abs(box.W-123) > 1 {
		t.Fatalf("经 resolver 的样式未生效：#box 宽度 = %.1f, want ≈123", box.W)
	}
	if calls != 1 {
		t.Errorf("resolver 调用次数 = %d, want 1", calls)
	}
}

// ── 子框架装配 ───────────────────────────────────────────

func TestModeSubframeAssembly(t *testing.T) {
	src := `<!DOCTYPE html><html><body>` +
		`<iframe id="f" src="data:text/html,<p>child</p>"></iframe>` +
		`</body></html>`

	br := modeWebView(t, ModeBrowser)
	modeMustLoad(t, br, src)
	el := br.Document().GetElementById("f")
	if el == nil {
		t.Fatal("#f 未解析")
	}
	if page.IFrameFrame(el) == nil {
		t.Error("ModeBrowser：iframe 子文档未装配")
	}

	tk := modeWebView(t, ModeToolkit)
	modeMustLoad(t, tk, src)
	el2 := tk.Document().GetElementById("f")
	if el2 == nil {
		t.Fatal("toolkit #f 未解析")
	}
	if page.IFrameFrame(el2) != nil {
		t.Error("ModeToolkit：iframe 子文档不应装配")
	}
}

// ── 导航门禁 ─────────────────────────────────────────────

func TestModeNavigationGate(t *testing.T) {
	tk := modeWebView(t, ModeToolkit)
	if err := tk.LoadURL("data:text/html,<p id=x>hi</p>"); !errors.Is(err, ErrModeNotSupported) {
		t.Fatalf("ModeToolkit LoadURL err = %v, want ErrModeNotSupported", err)
	}

	br := modeWebView(t, ModeBrowser)
	if err := br.LoadURL("data:text/html,<p id=x>hi</p>"); err != nil {
		t.Fatalf("ModeBrowser LoadURL: %v", err)
	}
	if br.Document() == nil || br.Document().GetElementById("x") == nil {
		t.Fatal("ModeBrowser 导航后文档未更新")
	}
}
