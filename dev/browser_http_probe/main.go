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
