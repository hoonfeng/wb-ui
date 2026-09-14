// Command browser_http_probe 是「嵌入浏览器」用途的真实网络端到端探针。
//
// webkit/mode_test.go 覆盖了模式的能力面与门禁，但其中的 LoadURL 只用
// data: URL、外部样式只用 file://。这个探针补上最关键的一条路径：真起一个
// HTTP 服务器，用 ModeBrowser 经 LoadURL 加载真实页面，验证 HTML / 外部
// CSS / 内联脚本 / fetch 全部走真实 HTTP 栈（并记录服务器实际收到的请求），
// 再用 ModeToolkit 复验同样的动作不产生任何网络请求。
//
//	go run ./dev/browser_http_probe
package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"

	"wb-ui/webkit"
)

const probeHTML = `<!DOCTYPE html>
<html><head>
<title>HTTP Probe</title>
<link rel="stylesheet" href="/style.css">
</head><body>
<div id="box">x</div>
<img id="pic" src="/pic.png" width="10" height="10">
<script>window.__hello = "script-ran";</script>
<script src="app.js"></script>
</body></html>`

// onePixelPNG 是 1x1 的 PNG（用于探测 <img src> 是否触发资源加载）。
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

// fetchScript 用相对 URL 请求同源 JSON —— 同时验证 document URL 已设置
// （相对 URL 解析）与 fetch 的真实网络路径。
const fetchScript = `
	window.__probe = "pending";
	fetch("/api.json").then(function (r) {
		return r.text().then(function (t) { window.__probe = "ok:" + r.status + ":" + t; });
	}, function (e) {
		window.__probe = "rejected:" + String(e && e.message || e);
	});
`

// reqLog 记录服务器真实收到的请求（证明「网络真的发出去了 / 真的没发」）。
type reqLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *reqLog) add(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, p)
}

func (l *reqLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.seen)
}

func (l *reqLog) dump() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.seen, " ")
}

var failures int

func check(ok bool, format string, args ...any) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
		failures++
	}
	fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(format, args...))
}

func evalStr(wv *webkit.WebView, script string) string {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

// settle 驱动事件循环直到 window.__probe 不再是 pending。
func settle(wv *webkit.WebView, script string, tries int) string {
	if _, err := wv.EvalJS(script); err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	for i := 0; i < tries; i++ {
		if got := evalStr(wv, `String(window.__probe)`); got != "pending" {
			return got
		}
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
	}
	return evalStr(wv, `String(window.__probe)`)
}

func boxWidth(wv *webkit.WebView, id string) float64 {
	wv.EnsureHitTestReady()
	doc := wv.Document()
	if doc == nil {
		return math.NaN()
	}
	el := doc.GetElementById(id)
	if el == nil {
		return math.NaN()
	}
	rv := wv.RenderView()
	if rv == nil {
		return math.NaN()
	}
	b := rv.FindRenderBoxForNode(el)
	if b == nil {
		return math.NaN()
	}
	return b.Width()
}

func main() {
	log := &reqLog{}
	mux := http.NewServeMux()
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, probeHTML)
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:123px}")
	})
	mux.HandleFunc("/api.json", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"n":7}`)
	})
	// 文档相对引用（src="app.js"，页面在根目录 → /app.js）与标准的
	// JS Content-Type —— 真实服务器对 .css/.js 都不会回 text/html。
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__app = "app-js-ran";`)
	})
	mux.HandleFunc("/pic.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		b, _ := base64.StdEncoding.DecodeString(onePixelPNG)
		w.Write(b)
	})
	// @import 探测：/import.html 的样式表里只有一条 @import。
	mux.HandleFunc("/import.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><link rel="stylesheet" href="/import.css">`+
			`</head><body><div id="box">x</div></body></html>`)
	})
	mux.HandleFunc("/import.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, `@import "imported.css";`)
	})
	mux.HandleFunc("/imported.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:333px}")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pageURL := srv.URL + "/index.html"
	fmt.Printf("本地 HTTP 服务器：%s\n\n", srv.URL)

	// ── 用途一：嵌入浏览器（真实 HTTP 栈）────────────────────
	fmt.Println("── ModeBrowser（嵌入浏览器）──")
	br := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	defer br.Destroy()
	br.Resize(600, 400)

	err := br.LoadURL(pageURL)
	check(err == nil, "LoadURL(%s) err=%v", pageURL, err)
	check(evalStr(br, "document.title") == "HTTP Probe",
		"document.title = %q", evalStr(br, "document.title"))
	check(evalStr(br, `window.__hello`) == "script-ran",
		"内联 <script> 执行：window.__hello = %q", evalStr(br, `window.__hello`))
	check(evalStr(br, `window.__app`) == "app-js-ran",
		"外部 <script src=\"app.js\"> 执行：window.__app = %q", evalStr(br, `window.__app`))
	check(evalStr(br, `document.URL`) == pageURL,
		"document.URL = %q（want %q）", evalStr(br, `document.URL`), pageURL)
	check(evalStr(br, `location.href`) == pageURL,
		"location.href = %q（want %q）", evalStr(br, `location.href`), pageURL)

	w := boxWidth(br, "box")
	check(math.Abs(w-123) <= 1, "#box 宽度 = %.1f（外部 CSS 经 HTTP 生效）", w)

	got := settle(br, fetchScript, 8)
	check(strings.HasPrefix(got, "ok:200:"), "fetch('/api.json') = %q", got)
	check(strings.Contains(got, `{"ok":true,"n":7}`), "fetch 响应正文来自服务器：%q", got)

	browserHits := log.count()
	fmt.Printf("       服务器收到请求 %d 个：%s\n", browserHits, log.dump())
	// 事实项（不作断言）：引擎没有 <img src> 的 URL 加载通道——图片由宿主
	// 解码后经 RenderBox.SetDecodedImage 注入（见 docs/MODES.md 边界）。
	fmt.Printf("       <img src=\"/pic.png\"> 触发的资源请求：%v（引擎不加载 <img> URL，宿主注入解码图）\n",
		logHas(log, "/pic.png"))

	// 事实项：@import（见 docs/MODES.md 边界）。
	importProbe := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	importProbe.Resize(300, 200)
	_ = importProbe.LoadURL(srv.URL + "/import.html")
	fmt.Printf("       CSS @import 触发的资源请求：%v\n", logHas(log, "/imported.css"))
	importProbe.Destroy()

	// ── 用途二：UI 框架（同样的动作不得产生网络）──────────────
	fmt.Println("\n── ModeToolkit（UI 框架）──")
	tk := webkit.NewWebViewWithMode(webkit.ModeToolkit)
	defer tk.Destroy()
	tk.Resize(600, 400)

	err = tk.LoadURL(pageURL)
	check(errors.Is(err, webkit.ErrModeNotSupported), "LoadURL err = %v（期望 ErrModeNotSupported）", err)

	before := log.count()
	if err := tk.LoadHTML(probeHTML); err != nil {
		check(false, "LoadHTML: %v", err)
	}
	tkW := boxWidth(tk, "box")
	check(math.Abs(tkW-123) > 1, "#box 宽度 = %.1f（外部样式未加载）", tkW)
	tkGot := settle(tk, fetchScript, 8)
	check(strings.HasPrefix(tkGot, "rejected:"), "fetch('/api.json') = %q（期望 rejected）", tkGot)
	check(log.count() == before, "UI 框架模式新增网络请求 = %d（期望 0）", log.count()-before)

	fmt.Printf("\n结论：浏览器模式发出 %d 个真实请求；UI 框架模式发出 0 个。\n", browserHits)
	if failures > 0 {
		fmt.Printf("失败 %d 项\n", failures)
		os.Exit(1)
	}
	fmt.Println("全部检查通过。")
}

func logHas(l *reqLog, p string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.seen {
		if s == p {
			return true
		}
	}
	return false
}
