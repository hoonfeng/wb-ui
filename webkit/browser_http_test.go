package webkit

// 真实 HTTP 端到端测试：ModeBrowser 用 LoadURL 加载真实页面时，页面里的
// **相对引用**必须按浏览器语义以文档 URL 为基准解析——根相对（/style.css）、
// 文档相对（app.js）、fetch("/api.json")。同时文档 URL 要能被页面脚本读到
// （document.URL / location.href）。
//
// 这几条此前全是坏的，且只有真起 HTTP 服务器才暴露：data: URL 与内存替换
// 都不会经过 URL 解析，file:// 绝对路径也没有基准问题。
//   - 相对 <link>/<script src>：落到「相对当前工作目录读文件」分支 → 读不到
//   - fetch("/x")：原样交给 http.NewRequest → unsupported protocol scheme ""
//   - document.URL：注册时求值的静态快照 → 恒为空串；location.href 写死
//     "about:blank"
//   - 外部资源即便相对解析正确也仍会失败：fetchHTTP 把非 HTML 的
//     Content-Type（text/css、application/javascript）当错误返回空内容

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// httpRequestLog 记录服务器真实收到的请求路径。
type httpRequestLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *httpRequestLog) add(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, path)
}

func (l *httpRequestLog) paths() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.seen))
	copy(out, l.seen)
	return out
}

func (l *httpRequestLog) has(path string) bool {
	for _, p := range l.paths() {
		if p == path {
			return true
		}
	}
	return false
}

// browserHTTPFixture 起一个真实 HTTP 服务器：/index.html 用两类相对引用
// 引外部样式与外部脚本，另有 JSON 接口与两个子目录（用于导航基准测试）。
//
// 注意 Content-Type 都按真实服务器给（text/css、application/javascript、
// application/json）——不是 text/html。
func browserHTTPFixture(t *testing.T) (*httptest.Server, *httpRequestLog) {
	t.Helper()
	log := &httpRequestLog{}
	serve := func(mux *http.ServeMux, path, ctype, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			log.add(r.URL.Path)
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			fmt.Fprint(w, body)
		})
	}
	mux := http.NewServeMux()
	serve(mux, "/index.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><title>HTTP Fixture</title>`+
			`<link rel="stylesheet" href="/style.css"></head><body>`+
			`<div id="box">x</div>`+
			`<script src="app.js"></script>`+
			`</body></html>`)
	serve(mux, "/style.css", "text/css", "#box{width:123px}")
	serve(mux, "/app.js", "application/javascript", `window.__app = "app-js-ran";`)
	serve(mux, "/api.json", "application/json", `{"ok":true,"n":7}`)

	// 导航基准：/a/ 与 /b/ 各有一份 index.html + 同目录 style.css，样式值
	// 不同 → 能区分「按新文档目录解析」与「残留旧基准」。
	serve(mux, "/a/index.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="style.css"></head>`+
			`<body><div id="box">a</div></body></html>`)
	serve(mux, "/a/style.css", "text/css", "#box{width:111px}")
	serve(mux, "/b/index.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="style.css"></head>`+
			`<body><div id="box">b</div></body></html>`)
	serve(mux, "/b/style.css", "text/css", "#box{width:222px}")
	// 重定向入口：/r → /b/index.html（真实站点的 http→https、补尾斜杠、
	// 跳到 /index.html 都是这类跳转）。
	mux.HandleFunc("/r", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		http.Redirect(w, r, "/b/index.html", http.StatusFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, log
}

// browserBoxWidth 触发布局后量 #box 的宽度。
func browserBoxWidth(t *testing.T, wv *WebView) float64 {
	t.Helper()
	wv.EnsureHitTestReady()
	doc := wv.Document()
	if doc == nil {
		t.Fatal("文档缺失")
	}
	box := findBox(wv, doc.GetElementById("box"))
	if box == nil {
		t.Fatal("#box 渲染盒缺失")
	}
	return box.W
}

// TestBrowserModeHTTPPageRelativeReferences：真实页面的三类相对引用 +
// 文档 URL 可读 + 服务器确实收到这些请求。
func TestBrowserModeHTTPPageRelativeReferences(t *testing.T) {
	srv, log := browserHTTPFixture(t)
	pageURL := srv.URL + "/index.html"

	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(pageURL); err != nil {
		t.Fatalf("LoadURL(%s): %v", pageURL, err)
	}

	// 文档 URL：页面脚本必须能读到（document.URL 与 location.href 同源）。
	if got := modeEval(t, wv, `document.URL`); got != pageURL {
		t.Errorf("document.URL = %q, want %q", got, pageURL)
	}
	if got := modeEval(t, wv, `location.href`); got != pageURL {
		t.Errorf("location.href = %q, want %q", got, pageURL)
	}
	if got := modeEval(t, wv, `location.pathname`); got != "/index.html" {
		t.Errorf("location.pathname = %q, want /index.html", got)
	}
	if got := modeEval(t, wv, `location.origin`); !strings.HasPrefix(got, "http://") {
		t.Errorf("location.origin = %q, want http://…", got)
	}

	// 文档相对的外部脚本（src="app.js" → /app.js）。
	if got := modeEval(t, wv, `window.__app`); got != "app-js-ran" {
		t.Errorf(`外部 <script src="app.js"> 未执行：window.__app = %q`, got)
	}
	// 根相对的外部样式（href="/style.css"）。
	if w := browserBoxWidth(t, wv); math.Abs(w-123) > 1 {
		t.Errorf("相对外部样式未生效：#box 宽度 = %.1f, want ≈123", w)
	}

	// fetch 的相对 URL（脚本写 "/api.json"，实际请求 srv.URL + "/api.json"）。
	got := modeSettle(t, wv, `
		window.__probe = "pending";
		fetch("/api.json").then(function (r) {
			return r.text().then(function (t) { window.__probe = "ok:" + r.status + ":" + t; });
		}, function (e) {
			window.__probe = "rejected:" + String(e && e.message || e);
		});
	`)
	if !strings.HasPrefix(got, "ok:200:") || !strings.Contains(got, `{"ok":true,"n":7}`) {
		t.Errorf("相对 URL fetch = %q, want ok:200:{\"ok\":true,\"n\":7}", got)
	}

	for _, p := range []string{"/index.html", "/style.css", "/app.js", "/api.json"} {
		if !log.has(p) {
			t.Errorf("服务器未收到 %s（实际收到：%v）", p, log.paths())
		}
	}
}

// TestBrowserModeHTTPNavigationUpdatesBase：LoadURL 导航到另一个目录后，
// 相对引用必须按**新**文档 URL 解析（基准跟随导航，不残留旧基准）。
func TestBrowserModeHTTPNavigationUpdatesBase(t *testing.T) {
	srv, log := browserHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)

	if err := wv.LoadURL(srv.URL + "/a/index.html"); err != nil {
		t.Fatalf("LoadURL(/a/index.html): %v", err)
	}
	if w := browserBoxWidth(t, wv); math.Abs(w-111) > 1 {
		t.Fatalf("/a/#box 宽度 = %.1f, want ≈111（文档相对样式未按 /a/ 解析）", w)
	}

	if err := wv.LoadURL(srv.URL + "/b/index.html"); err != nil {
		t.Fatalf("LoadURL(/b/index.html): %v", err)
	}
	if w := browserBoxWidth(t, wv); math.Abs(w-222) > 1 {
		t.Fatalf("/b/#box 宽度 = %.1f, want ≈222（相对引用未按新文档 URL 解析）", w)
	}
	if got := modeEval(t, wv, `document.URL`); got != srv.URL+"/b/index.html" {
		t.Errorf("导航后 document.URL = %q, want %q", got, srv.URL+"/b/index.html")
	}
	for _, p := range []string{"/a/index.html", "/a/style.css", "/b/index.html", "/b/style.css"} {
		if !log.has(p) {
			t.Errorf("服务器未收到 %s（实际收到：%v）", p, log.paths())
		}
	}
}

// TestBrowserModeHTTPRedirectUpdatesBase：LoadURL 跟随重定向后，文档基地址
// 必须是**最终 URL**（浏览器语义）——相对引用据此解析。若沿用请求 URL 作为
// 基准，页面里的 "style.css" 会解析到 /style.css（不存在）而不是 /b/style.css。
func TestBrowserModeHTTPRedirectUpdatesBase(t *testing.T) {
	srv, log := browserHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)

	if err := wv.LoadURL(srv.URL + "/r"); err != nil {
		t.Fatalf("LoadURL(/r): %v", err)
	}
	if got := modeEval(t, wv, `document.URL`); got != srv.URL+"/b/index.html" {
		t.Errorf("重定向后 document.URL = %q, want %q", got, srv.URL+"/b/index.html")
	}
	if w := browserBoxWidth(t, wv); math.Abs(w-222) > 1 {
		t.Errorf("重定向后 #box 宽度 = %.1f, want ≈222（相对引用未按最终 URL 解析）", w)
	}
	if !log.has("/b/style.css") {
		t.Errorf("服务器未收到 /b/style.css（实际收到：%v）", log.paths())
	}
	if log.has("/style.css") {
		t.Errorf("相对引用被按请求 URL（/r）解析成了 /style.css：%v", log.paths())
	}
}

// TestToolkitModeHTTPNoNetworkWithRelativeRefs：UI 库模式下同样的相对引用
// （即便宿主给了文档 URL 语义可解析的目标）也不得产生任何网络请求。
func TestToolkitModeHTTPNoNetworkWithRelativeRefs(t *testing.T) {
	// 服务器只用于提供「可被解析到的绝对目标」；断言的是这些请求一个都
	// 不该发出去（srv.URL 本身不参与构造页面）。
	_, log := browserHTTPFixture(t)

	wv := modeWebView(t, ModeToolkit)
	modeMustLoad(t, wv, `<!DOCTYPE html><html><head><link rel="stylesheet" href="/style.css">`+
		`</head><body><div id="box">x</div><script src="app.js"></script></body></html>`)

	if w := browserBoxWidth(t, wv); math.Abs(w-123) <= 1 {
		t.Errorf("UI 库模式不应加载外部样式（#box 宽度 = %.1f）", w)
	}
	got := modeSettle(t, wv, `
		window.__probe = "pending";
		fetch("/api.json").then(function () {
			window.__probe = "resolved";
		}, function (e) {
			window.__probe = "rejected:" + String(e && e.message || e);
		});
	`)
	if !strings.HasPrefix(got, "rejected:") {
		t.Errorf("UI 库模式 fetch 相对 URL = %q, want rejected:…", got)
	}
	if paths := log.paths(); len(paths) != 0 {
		t.Errorf("UI 库模式产生了网络请求：%v", paths)
	}
}
