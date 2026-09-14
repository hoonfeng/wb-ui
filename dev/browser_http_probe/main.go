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
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"wb-ui/webkit"
)

const probeHTML = `<!DOCTYPE html>
<html><head>
<title>HTTP Probe</title>
<link rel="stylesheet" href="/style.css">
</head><body>
<div id="box">x</div>
<img id="pic" src="pic.png" width="24" height="24">
<script>window.__hello = "script-ran";</script>
<script src="app.js"></script>
</body></html>`

// redPNG 是 4x4 纯红 PNG（服务端实时生成）：探针用它做「请求 → 解码 →
// 固有尺寸 → 绘制像素」的端到端断言，比"收到请求"更接近用户可见结果。
var redPNG = mkRedPNG()

// bluePNG 是 4x4 纯蓝 PNG：给「错误基准」那一侧的图片用，于是断言不只
// 「正确路径被请求」，还能验证**画出来的颜色**来自正确的那份文件。
var bluePNG = mkBluePNG()

func mkBluePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mkRedPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

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

// countPath 数某个路径被请求了几次：资源缓存与 MIME 断言的判据都是
// 「同一个 URL 到底打扰了服务器几次」——比"收到过请求"更严格。
func (l *reqLog) countPath(p string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.seen {
		if s == p {
			n++
		}
	}
	return n
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
		fmt.Fprint(w, "#box{width:123px}#pic{width:24px;height:24px}")
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
		w.Write(redPNG)
	})
	// @import 场景一（同级）：/import.html 的样式表里只有一条 @import。
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
	// @import 场景二（基准）：页面在 /sub/，样式表在 /sub/css/，而样式表里的
	// @import 是**相对样式表 URL** 的引用 → 必须请求 /sub/css/theme.css。
	// 若按文档 URL 解析就变成 /sub/theme.css（404）→ 样式不生效 → 断言失败。
	mux.HandleFunc("/sub/page.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><link rel="stylesheet" href="css/main.css">`+
			`</head><body><div id="box">x</div></body></html>`)
	})
	mux.HandleFunc("/sub/css/main.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, `@import "theme.css";`)
	})
	mux.HandleFunc("/sub/css/theme.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:444px}")
	})
	// <base href>：页面里所有引用都是相对路径，只有认了 <base href="/assets/">
	// 才会请求 /assets/…；忽略它就落到 /base/…（服务器没有这些端点 → 外部
	// 样式/脚本/图片全部失效）。这是真实站点把静态资源放子目录的常规手段。
	mux.HandleFunc("/base/page.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><base href="/assets/">`+
			`<link rel="stylesheet" href="theme.css">`+
			`<script src="app.js"></script></head>`+
			`<body><div id="box">x</div><img id="pic" src="pic.png" width="4" height="4"></body></html>`)
	})
	mux.HandleFunc("/assets/theme.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:246px}")
	})
	mux.HandleFunc("/assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__baseApp = "base-app-js-ran";`)
	})
	mux.HandleFunc("/assets/pic.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(redPNG)
	})
	// 资源缓存：同一个 URL 挂两个 <link>、同一张图挂两个 <img>——浏览器只向
	// 服务器取一次（内存缓存），引擎此前每个引用都重新取。
	//
	// 这里特意用 /cached.* 这些**没被任何其他页面引用过**的 URL：图片的解码
	// 结果是进程级缓存（rendering.backgroundImageCache），若复用 /pic.png 就会
	// 命中前一个文档的解码图、连字节都不再取——那样测到的是跨文档缓存，
	// 而不是本探针要测的「同一文档内重复引用只取一次」。
	mux.HandleFunc("/cached.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:123px}")
	})
	mux.HandleFunc("/cached.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(redPNG)
	})
	mux.HandleFunc("/cache.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head>`+
			`<link rel="stylesheet" href="/cached.css">`+
			`<link rel="stylesheet" href="/cached.css"></head><body>`+
			`<div id="box">x</div>`+
			`<img id="pica" src="/cached.png" width="4" height="4">`+
			`<img id="picb" src="/cached.png" width="4" height="4"></body></html>`)
	})
	// MIME 检查：/plain.css 回 text/plain 但无 nosniff → 浏览器宽松接受；
	// /nosniff.css 回 text/plain **且**带 X-Content-Type-Options: nosniff →
	// 浏览器拒绝把 text/plain 当样式表用（正是 nosniff 的用途）。
	mux.HandleFunc("/plain.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><link rel="stylesheet" href="/plain.css"></head>`+
			`<body><div id="box">x</div></body></html>`)
	})
	mux.HandleFunc("/plain.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "#box{width:321px}")
	})
	mux.HandleFunc("/nosniff.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><link rel="stylesheet" href="/nosniff.css"></head>`+
			`<body><div id="box">x</div></body></html>`)
	})
	mux.HandleFunc("/nosniff.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fmt.Fprint(w, "#box{width:180px}")
	})
	// 装配期脚本导航：页面脚本在装配中就 location.replace → 排队到装配结束
	// 执行、最终文档是 /index.html。
	mux.HandleFunc("/nav.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>NAV</title>`+
			`<script>location.replace("/index.html")</script></head>`+
			`<body><div id="who">NAV</div></body></html>`)
	})
	// html/body 背景传播：页面只写 body 背景、body 盒只有一行高，画布底部
	// 仍应被染成该色（CSS-BACKGROUNDS-3 §2.11.2 的背景传播规则）。
	mux.HandleFunc("/bg.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><style>`+
			`body{background:rgb(18,52,86);margin:0}</style></head>`+
			`<body><div id="who">BG</div></body></html>`)
	})
	// 外部样式表里相对 url() 的基准：必须是**样式表自身 URL**（CSS Values 3
	// §4.4），不是文档 URL。文档 /cssurl/page/index.html、样式表
	// /cssurl/theme/css/site.css——★ 两者深度必须不同：同深度
	// （/cssurl/theme/）时"样式表基准"与"文档基准"会算出同一个 URL，断言就
	// 抓不到回归（这正是反向验证暴露过的 fixture 陷阱）。
	// 样式表里 `url(../assets/bg.png)`：
	//   样式表基准 → /cssurl/theme/assets/bg.png（红，正确那份）
	//   文档基准   → /cssurl/assets/bg.png（蓝，错误那份也真实存在）
	mux.HandleFunc("/cssurl/page/index.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head>`+
			`<style>body{margin:0;background:#fff}</style>`+
			`<link rel="stylesheet" href="../theme/css/site.css"></head>`+
			`<body><div id="bg"></div></body></html>`)
	})
	mux.HandleFunc("/cssurl/theme/css/site.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, `#bg{width:24px;height:24px;margin:0;`+
			`background-image:url(../assets/bg.png);background-size:100% 100%}`)
	})
	mux.HandleFunc("/cssurl/theme/assets/bg.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(redPNG)
	})
	mux.HandleFunc("/cssurl/assets/bg.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(bluePNG)
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

	// ── <img src>：URL 加载 → 解码 → 固有尺寸 → 绘制像素 ──────────
	// 页面里写的是**相对引用** `src="pic.png"`（文档在根目录 → /pic.png）：
	// 这一条同时覆盖「相对引用按文档 URL 解析」与「图片走网络加载通道」。
	img := waitForImage(br, "pic", 60)
	check(logHas(log, "/pic.png"), "<img src=\"pic.png\"> 触发资源请求 /pic.png：%v", logHas(log, "/pic.png"))
	check(img.loaded, "<img> 已附着解码图：%v（尺寸 %.0fx%.0f）", img.loaded, img.w, img.h)
	check(img.w == 4 && img.h == 4, "解码图固有尺寸 = %.0fx%.0f（want 4x4）", img.w, img.h)
	check(img.red, "<img> 位置渲染像素 = %s（want 红）", img.pixel)

	// ── CSS @import：外部样式表里的 @import 生效 ──────────────────
	// 场景一（同级）：/import.css 里 @import "imported.css" → /imported.css
	imp1 := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	imp1.Resize(300, 200)
	if err := imp1.LoadURL(srv.URL + "/import.html"); err != nil {
		check(false, "LoadURL(/import.html): %v", err)
	}
	w1 := boxWidth(imp1, "box")
	check(logHas(log, "/imported.css"), "@import \"imported.css\" 触发资源请求：%v", logHas(log, "/imported.css"))
	check(math.Abs(w1-333) <= 1, "#box 宽度 = %.1f（@import 的样式生效，want 333）", w1)
	imp1.Destroy()

	// 场景二（基准）：/sub/page.html ← /sub/css/main.css ← @import "theme.css"
	// 相对 @import 必须相对**样式表 URL** 解析 → /sub/css/theme.css。
	imp2 := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	imp2.Resize(300, 200)
	if err := imp2.LoadURL(srv.URL + "/sub/page.html"); err != nil {
		check(false, "LoadURL(/sub/page.html): %v", err)
	}
	w2 := boxWidth(imp2, "box")
	check(logHas(log, "/sub/css/theme.css"), "子目录 @import 请求 /sub/css/theme.css：%v", logHas(log, "/sub/css/theme.css"))
	check(!logHas(log, "/sub/theme.css"), "没有按文档 URL 误解析到 /sub/theme.css：%v", !logHas(log, "/sub/theme.css"))
	check(math.Abs(w2-444) <= 1, "#box 宽度 = %.1f（嵌套 @import 生效，want 444）", w2)
	imp2.Destroy()

	// ── <base href>：相对引用的基准可被文档改写 ──────────────────
	bw := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	bw.Resize(600, 400)
	if err := bw.LoadURL(srv.URL + "/base/page.html"); err != nil {
		check(false, "LoadURL(/base/page.html): %v", err)
	}
	check(evalStr(bw, "document.baseURI") == srv.URL+"/assets/",
		"document.baseURI = %q（want %q）", evalStr(bw, "document.baseURI"), srv.URL+"/assets/")
	check(logHas(log, "/assets/theme.css"),
		"<base> 下的 <link href=\"theme.css\"> 请求 /assets/theme.css：%v", logHas(log, "/assets/theme.css"))
	check(logHas(log, "/assets/app.js"),
		"<base> 下的 <script src=\"app.js\"> 请求 /assets/app.js：%v", logHas(log, "/assets/app.js"))
	check(!logHas(log, "/base/theme.css"),
		"没有按文档 URL 误解析到 /base/theme.css：%v", !logHas(log, "/base/theme.css"))
	check(evalStr(bw, "window.__baseApp") == "base-app-js-ran",
		"<base> 下的外部脚本已执行：window.__baseApp = %q", evalStr(bw, "window.__baseApp"))
	check(math.Abs(boxWidth(bw, "box")-246) <= 1,
		"<base> 下的外部样式已生效：#box 宽度 = %.1f（want 246）", boxWidth(bw, "box"))
	bimg := waitForImage(bw, "pic", 40)
	check(logHas(log, "/assets/pic.png"),
		"<base> 下的 <img src=\"pic.png\"> 请求 /assets/pic.png：%v", logHas(log, "/assets/pic.png"))
	check(bimg.loaded && bimg.red, "<base> 下的图片已绘制：loaded=%v pixel=%s", bimg.loaded, bimg.pixel)
	bw.Destroy()

	// ── 资源缓存：同一 URL 只向服务器取一次 ──────────────────────
	cssBefore, pngBefore := log.countPath("/cached.css"), log.countPath("/cached.png")
	cw := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	cw.Resize(600, 400)
	if err := cw.LoadURL(srv.URL + "/cache.html"); err != nil {
		check(false, "LoadURL(/cache.html): %v", err)
	}
	cp1, cp2 := waitForImage(cw, "pica", 40), waitForImage(cw, "picb", 40)
	check(cp1.red && cp2.red, "两个 <img> 都绘制出图片：pica=%s picb=%s", cp1.pixel, cp2.pixel)
	check(log.countPath("/cached.css")-cssBefore == 1,
		"两个 <link> 指向同一 URL → 服务器收到 /cached.css 请求 %d 次（期望 1，内存缓存）",
		log.countPath("/cached.css")-cssBefore)
	check(log.countPath("/cached.png")-pngBefore == 1,
		"两个 <img> 指向同一 URL → 服务器收到 /cached.png 请求 %d 次（期望 1，内存缓存）",
		log.countPath("/cached.png")-pngBefore)
	check(math.Abs(boxWidth(cw, "box")-123) <= 1,
		"缓存命中的样式表仍然生效：#box 宽度 = %.1f（want 123）", boxWidth(cw, "box"))
	cw.Destroy()

	// ── MIME 检查：nosniff 时按类型拒绝（无 nosniff 则宽松接受）────
	pw := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	pw.Resize(600, 400)
	if err := pw.LoadURL(srv.URL + "/plain.html"); err != nil {
		check(false, "LoadURL(/plain.html): %v", err)
	}
	check(math.Abs(boxWidth(pw, "box")-321) <= 1,
		"无 nosniff 的 text/plain 样式表被宽松接受：#box 宽度 = %.1f（want 321）", boxWidth(pw, "box"))
	pw.Destroy()

	ns := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	ns.Resize(600, 400)
	if err := ns.LoadURL(srv.URL + "/nosniff.html"); err != nil {
		check(false, "LoadURL(/nosniff.html): %v", err)
	}
	check(math.Abs(boxWidth(ns, "box")-180) > 1,
		"带 nosniff 的 text/plain 样式表被拒绝：#box 宽度 = %.1f（容器宽 → 样式未生效）", boxWidth(ns, "box"))
	ns.Destroy()

	// ── location 导航（装配期脚本发起）与 html/body 背景传播 ──────
	nv := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	nv.Resize(600, 400)
	if err := nv.LoadURL(srv.URL + "/nav.html"); err != nil {
		check(false, "LoadURL(/nav.html): %v", err)
	}
	check(evalStr(nv, "document.URL") == pageURL,
		"装配期 location.replace(\"/index.html\") 已换文档：document.URL = %q（want %q）",
		evalStr(nv, "document.URL"), pageURL)
	check(evalStr(nv, "document.title") == "HTTP Probe",
		"导航后文档是 index.html：title = %q", evalStr(nv, "document.title"))
	check(evalStr(nv, "history.length") == "1",
		"location.replace 替换当前条目：history.length = %q（want 1）", evalStr(nv, "history.length"))
	nv.Destroy()

	bgc := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	bgc.Resize(240, 160)
	if err := bgc.LoadURL(srv.URL + "/bg.html"); err != nil {
		check(false, "LoadURL(/bg.html): %v", err)
	}
	bpix, berr := bgc.Render()
	bpr, bpg, bpb, bpa := pixelAt(bpix, 240, 120, 150)
	if berr != nil {
		check(false, "Render(/bg.html): %v", berr)
	} else {
		check(bpr == 18 && bpg == 52 && bpb == 86 && bpa == 255,
			"body 背景传播到画布：画布底部像素 = rgba(%d,%d,%d,%d)（want 18,52,86,255）", bpr, bpg, bpb, bpa)
	}
	bgc.Destroy()

	// ── 外部样式表里相对 url() 的基准 = 样式表自身 URL ─────────
	fmt.Println("── 样式表内 url() 的基准 ──")
	cssw := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	cssw.Resize(240, 160)
	if err := cssw.LoadURL(srv.URL + "/cssurl/page/index.html"); err != nil {
		check(false, "LoadURL(/cssurl/page/index.html): %v", err)
	}
	// ★ 先渲染等待，**再**断言：外部样式表与背景图都是异步取回的，断言写在
	// 等待之前就会看到"还没请求"的假失败（探针第一版正是这个顺序问题）。
	// 固定渲染若干帧、不用「像素变红就 break」：那个条件会被上一个 WebView
	// 留在共享底层缓冲里的残留像素误判（首帧即"红"→ 立刻退出）。页面自带
	// 白底（body{background:#fff}，经背景传播铺满画布）压掉残留，红因此
	// 只可能来自样式表引用的那张图。
	for i := 0; i < 30; i++ {
		if _, err := cssw.Render(); err != nil {
			check(false, "Render(/cssurl/page/index.html): %v", err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	check(logHas(log, "/cssurl/theme/css/site.css"),
		"外部样式表 /cssurl/theme/css/site.css 已加载：%v", logHas(log, "/cssurl/theme/css/site.css"))
	check(logHas(log, "/cssurl/theme/assets/bg.png"),
		"样式表里的 url(../assets/bg.png) 按**样式表**基准请求 /cssurl/theme/assets/bg.png：%v（服务器共收到：%s）",
		logHas(log, "/cssurl/theme/assets/bg.png"), log.dump())
	check(!logHas(log, "/cssurl/assets/bg.png"),
		"没有按**文档**基准误请求 /cssurl/assets/bg.png：%v", !logHas(log, "/cssurl/assets/bg.png"))
	pix, _ := cssw.Render()
	cpr, cpg, cpb, cpa := pixelAt(pix, 240, 12, 12)
	check(cpr > 200 && cpg < 80 && cpb < 80 && cpa > 200,
		"背景图取自样式表同级文件：(12,12) 像素 = rgba(%d,%d,%d,%d)（want 红；蓝表示落到了文档基准那份）",
		cpr, cpg, cpb, cpa)
	cssw.Destroy()

	// ── 用途二：UI 框架（同样的动作不得产生网络）──────────────
	fmt.Println("\n── ModeToolkit（UI 框架）──")
	tk := webkit.NewWebViewWithMode(webkit.ModeToolkit)
	defer tk.Destroy()
	tk.Resize(600, 400)

	err = tk.LoadURL(pageURL)
	check(errors.Is(err, webkit.ErrModeNotSupported), "LoadURL err = %v（期望 ErrModeNotSupported）", err)

	before := log.count()
	// 工具包页面额外挂两张**绝对 http 图片**（相对引用 + 真实 URL）：UI 库
	// 模式下图片加载必须同样被模式门禁挡住——渲染层不得绕过模式去联网。
	tkHTML := strings.Replace(probeHTML, `<img id="pic" src="pic.png"`,
		`<img id="pic" src="pic.png"><img id="remote" src="`+srv.URL+`/pic.png" width="8" height="8"`,
		1)
	if err := tk.LoadHTML(tkHTML); err != nil {
		check(false, "LoadHTML: %v", err)
	}
	tkW := boxWidth(tk, "box")
	check(math.Abs(tkW-123) > 1, "#box 宽度 = %.1f（外部样式未加载）", tkW)
	tkGot := settle(tk, fetchScript, 8)
	check(strings.HasPrefix(tkGot, "rejected:"), "fetch('/api.json') = %q（期望 rejected）", tkGot)
	tkImg := waitForImage(tk, "remote", 10)
	check(!tkImg.loaded, "UI 框架模式下 <img src=\"%s/pic.png\"> 未加载：%v（期望 true）", srv.URL, !tkImg.loaded)
	check(log.count() == before, "UI 框架模式新增网络请求 = %d（期望 0）", log.count()-before)

	// 能力面：UI 框架模式下 Worker 是**真删除**——`in` 与 typeof 同时为假，
	// 靠 `"Worker" in window` 做 feature detect 的库不会误判；浏览器模式里
	// 它必须存在（否则两边一样，这条断言就失去意义）。
	check(evalStr(tk, `("Worker" in window)`) == "false",
		"UI 框架模式：\"Worker\" in window = %v（期望 false）", evalStr(tk, `("Worker" in window)`))
	check(evalStr(br, `("Worker" in window)`) == "true",
		"浏览器模式：\"Worker\" in window = %v（期望 true）", evalStr(br, `("Worker" in window)`))

	// browserHits 是「图片/@import 断言之前」的计数（用于对照探针输出），
	// 结论行给出包含全部能力在内的最终计数。
	fmt.Printf("\n结论：浏览器模式共发出 %d 个真实请求（图片/@import 断言前 %d 个）；UI 框架模式发出 0 个。\n",
		log.count(), browserHits)
	if failures > 0 {
		fmt.Printf("失败 %d 项\n", failures)
		os.Exit(1)
	}
	fmt.Println("全部检查通过。")
}

// imgProbe 是 <img> 的端到端观测结果：是否附着解码图、固有尺寸、以及该
// 元素位置渲染输出中心的像素（证明图片真的被画出来了，而不只是被下载）。
type imgProbe struct {
	loaded bool
	w, h   float64
	pixel  string
	red    bool
}

// waitForImage 反复渲染若干帧直到 <img id> 附着了解码图（图片加载是异步
// goroutine：第一帧发起请求，之后的帧命中缓存才画得出来），返回该元素的
// 固有尺寸与渲染像素。frames 帧内未加载则返回零值（loaded=false）。
func waitForImage(wv *webkit.WebView, id string, frames int) imgProbe {
	var pr imgProbe
	for i := 0; i < frames; i++ {
		pix, err := wv.Render()
		if err != nil {
			continue
		}
		doc := wv.Document()
		rv := wv.RenderView()
		if doc == nil || rv == nil {
			continue
		}
		el := doc.GetElementById(id)
		if el == nil {
			continue
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			continue
		}
		img := box.DecodedImage()
		if img == nil || !img.Loaded() {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		pr.loaded = true
		pr.w, pr.h = float64(img.Width()), float64(img.Height())
		cx := int(box.X() + box.Width()/2)
		cy := int(box.Y() + box.Height()/2)
		r, g, b, a := pixelAt(pix, wv.Width(), cx, cy)
		pr.pixel = fmt.Sprintf("rgba(%d,%d,%d,%d) @ %d,%d", r, g, b, a, cx, cy)
		pr.red = r > 200 && g < 80 && b < 80 && a > 200
		return pr
	}
	return pr
}

// pixelAt 读 Render() 输出（RGBA8888，1 像素 4 字节）中 (x,y) 的分量。
func pixelAt(pix []byte, w, x, y int) (r, g, b, a uint8) {
	if w <= 0 || x < 0 || y < 0 {
		return
	}
	i := (y*w + x) * 4
	if i+3 >= len(pix) {
		return
	}
	return pix[i], pix[i+1], pix[i+2], pix[i+3]
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
