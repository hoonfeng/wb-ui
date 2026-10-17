// Command frontend_matrix 是「任意前端项目直接加载」的端到端验收探针。
//
// 与 browser_api_probe（浏览器 API 面）互补：本探针只关心**项目形态**——
// 把真实前端项目的几种典型产物结构起在本地 HTTP 服务器上，用引擎的
// ModeBrowser 加载，断言「页面是否真的跑起来了」：
//
//  1. 静态多页站：相对路径 CSS / JS / 图片、a 链接；
//
//  2. SPA：history.pushState 路由 + popstate + 首次直达深链；
//
//  3. Vite 产物：type=module + 相对 ESM 图 + vendor 分包 + modulepreload
//     + 动态 import + CSS @import / @font-face / background url；
//
//  4. Webpack 产物：经典 defer 脚本 + 运行时动态插入 chunk（JSONP 回调）；
//
//  5. ESM 语义：default/named/namespace/export*/live binding（见 webkit 单测，
//     这里只验构建产物形态）；
//
//  6. 媒体：img srcset / picture / SVG；
//
//  7. data: 模块导入、blob: URL（createObjectURL + fetch + img）；
//
//  8. 跨源 fetch（CORS）：无 ACAO 应 reject、有 ACAO 应成功；
//
//  9. WebSocket 构造（引擎侧 stub 语义）。
//
//     go run ./dev/probes/frontend_matrix
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/platform/graphics"
	"wb-ui/webkit"
)

var failures int

func check(ok bool, format string, args ...any) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
		failures++
	}
	fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(format, args...))
}

func evalStr(wv *webkit.WebView, js string) string {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

func pump(wv *webkit.WebView, frames int) {
	for i := 0; i < frames; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		_, _ = wv.Render()
		if strings.HasPrefix(strings.TrimSpace(evalStr(wv, `String(window.__done)`)), "true") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// pumpAsync 先固定驱动 25 帧（让异步子资源 —— 样式表/图片/字体 —— 完成），
// 再驱动到 window.__done 为 true（最多 400 帧）。
//
// 固定帧数不能省：页面脚本常在首帧就把 __done 置 true，此时 <link> 的异步
// 样式表可能还在路上，断言 getComputedStyle 会读到空样式（探针误报）。
func pumpAsync(wv *webkit.WebView) {
	pumpRaw(wv, 25)
	pump(wv, 400)
}

// pumpRaw 无条件驱动 N 帧（不因 __done 提前退出）。
func pumpRaw(wv *webkit.WebView, frames int) {
	for i := 0; i < frames; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		_, _ = wv.Render()
		time.Sleep(5 * time.Millisecond)
	}
}

type reqLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *reqLog) add(p string) {
	l.mu.Lock()
	l.seen = append(l.seen, p)
	l.mu.Unlock()
}

func (l *reqLog) has(p string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.seen {
		if s == p {
			return true
		}
	}
	return false
}

func (l *reqLog) dump() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.seen, " ")
}

// reset 清空请求轨迹（按场景隔离断言：上一个场景的请求不能把「外部资源
// 已被堵住」的断言喂成误报）。
func (l *reqLog) reset() {
	l.mu.Lock()
	l.seen = nil
	l.mu.Unlock()
}

func mkPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

var greenPNG = mkPNG(8, 8, color.RGBA{G: 200, A: 255})
var bluePNG = mkPNG(16, 16, color.RGBA{B: 200, A: 255})

// ── fixtures ────────────────────────────────────────────────────────────────

// staticSite 是「纯静态多页站」形态：相对路径（子目录里的页面引用 ../css）。
var staticSite = map[string]string{
	"/static/index.html": `<!DOCTYPE html><html><head>
<link rel="stylesheet" href="css/site.css">
</head><body><div id="app">static</div>
<img id="logo" src="img/logo.png" width="8" height="8">
<script src="js/app.js"></script>
<a id="to-about" href="about.html">about</a>
</body></html>`,
	"/static/about.html": `<!DOCTYPE html><html><head><title>About</title></head><body>
<div id="page">about-page</div>
<script src="js/app.js"></script></body></html>`,
	"/static/css/site.css": `body { background-color: rgb(10, 20, 30); }
#app { color: rgb(200, 100, 50); }`,
	"/static/js/app.js": `window.__staticJs = (window.__staticJs || "") + "js;";
if (document.getElementById("logo")) { window.__logo = document.getElementById("logo").src; }
window.__done = true;`,
	"/static/img/logo.png": "PNG",
}

// spaSite 是「SPA + history 路由」形态（模拟 Vite dev / CRA 的入口）。
var spaSite = map[string]string{
	"/spa/index.html": `<!DOCTYPE html><html><head><title>SPA</title></head><body>
<nav><a id="home" href="/spa/">home</a><a id="about" href="/spa/about">about</a></nav>
<div id="view">boot</div>
<script>
window.__routes = [];
function render(path) {
  document.getElementById("view").textContent = "route:" + path;
  window.__routes.push(path);
}
document.getElementById("about").addEventListener("click", function (e) {
  e.preventDefault();
  history.pushState({ page: "about" }, "", "/spa/about");
  render(location.pathname);
});
window.addEventListener("popstate", function () { render(location.pathname); });
window.__spa = { boot: true, path: location.pathname, ready: true };
window.__done = true;
</script></body></html>`,
	// 服务器端 SPA fallback：深链返回入口 HTML（真实 dev server 的行为）。
	"/spa/about": "FALLBACK",
}

// viteSite 是「Vite 构建产物」形态：ESM 入口 + vendor 分包 + 动态 import +
// modulepreload + CSS（@import / @font-face / background url）。
var viteSite = map[string]string{
	"/vite/index.html": `<!DOCTYPE html><html><head>
<title>Vite App</title>
<link rel="modulepreload" href="/vite/assets/vendor-xyz.js">
<link rel="stylesheet" href="/vite/assets/index-abc.css">
</head><body><div id="app">boot</div><div class="card" id="card"></div>
<script type="module" crossorigin src="/vite/assets/index-abc.js"></script>
</body></html>`,
	"/vite/assets/index-abc.js": `
import { a as vendor } from "./vendor-xyz.js";
const app = document.getElementById("app");
if (app) { app.textContent = "vite:" + vendor; }
window.__vite = { vendor: vendor, url: import.meta.url };
import("./lazy-def.js").then(function (m) {
  window.__vite.lazy = m.default;
  window.__vite.done = true;
  window.__done = true;
}, function (e) {
  window.__vite.lazy = "ERR:" + e;
  window.__done = true;
});`,
	"/vite/assets/vendor-xyz.js": `const a = "vendor-ok"; export { a };`,
	"/vite/assets/lazy-def.js":   `export default "lazy-ok";`,
	"/vite/assets/index-abc.css": `@import url("./base-abc.css");`,
	"/vite/assets/base-abc.css": `
body { background-color: rgb(5, 6, 7); }
#app { color: rgb(1, 2, 3); font-family: ProbeFont, sans-serif; }
@font-face { font-family: "ProbeFont"; src: url("./probe.woff2") format("woff2"); }
.card { background-image: url("./bg-card.png"); width: 20px; height: 20px; }`,
	"/vite/assets/probe.woff2": "FAKEFONT",
	"/vite/assets/bg-card.png": "PNG",
}

// webpackSite 是「Webpack 产物」形态：defer 脚本 + 运行时插入异步 chunk
// （webpack 的 JSONP 加载器语义：chunk 脚本到达后回调全局数组）。
var webpackSite = map[string]string{
	"/webpack/index.html": `<!DOCTYPE html><html><head><title>WP</title></head><body>
<div id="app">boot</div>
<script defer src="/webpack/main.js"></script>
</body></html>`,
	"/webpack/main.js": `
document.getElementById("app").textContent = "webpack:main";
window.__wp = { main: true, chunks: [] };
var chunk = document.createElement("script");
chunk.src = "/webpack/chunk-0.js";
chunk.onload = function () {
  window.__wp.chunks.push("loaded");
  window.__wp.value = window.__wpChunkValue || "missing";
  window.__done = true;
};
document.head.appendChild(chunk);`,
	"/webpack/chunk-0.js": `window.__wpChunkValue = "chunk-ok";`,
}

// mediaSite 是「响应式图片 / SVG」形态。
var mediaSite = map[string]string{
	"/media/index.html": `<!DOCTYPE html><html><head><title>media</title></head><body>
<picture><source srcset="/media/blue.png 2x"><img id="pic" src="/media/green.png" width="8" height="8"></picture>
<img id="set" srcset="/media/blue.png 2x, /media/green.png 1x" src="/media/green.png" width="8" height="8">
<svg id="icon" width="10" height="10"><rect width="10" height="10" fill="red"/></svg>
<script>
window.__media = { loaded: [], errors: [] };
['pic','set'].forEach(function (id) {
  var el = document.getElementById(id);
  el.addEventListener('load', function () { window.__media.loaded.push(id); });
  el.addEventListener('error', function () { window.__media.errors.push(id); });
});
window.__done = true;
</script></body></html>`,
	"/media/green.png": "PNG",
	"/media/blue.png":  "PNG",
}

// blobSite 覆盖 blob: URL（createObjectURL）+ data: 模块导入。
var blobSite = map[string]string{
	"/blob/index.html": `<!DOCTYPE html><html><head><title>blob</title></head><body>
<div id="out">boot</div>
<script type="module">
window.__blob = { hasCreate: typeof URL.createObjectURL === "function" };
if (window.__blob.hasCreate) {
  var u = URL.createObjectURL(new Blob(["blob-body"], { type: "text/plain" }));
  window.__blob.url = u;
  fetch(u).then(function (r) { return r.text(); }).then(function (t) {
    window.__blob.text = t;
    window.__blob.done = true;
    window.__done = true;
  }, function (e) {
    window.__blob.err = String(e);
    window.__done = true;
  });
}
import("data:text/javascript,export default 42").then(function (m) {
  window.__blob.dataModule = m.default;
}, function (e) { window.__blob.dataErr = String(e); });
</script></body></html>`,
}

// wsSite 覆盖 WebSocket 构造（引擎侧 stub：不联网，但语法完整）。
var wsSite = map[string]string{
	"/ws/index.html": `<!DOCTYPE html><html><head><title>ws</title></head><body>
<script>
window.__ws = { ok: false };
try {
  var ws = new WebSocket("ws://127.0.0.1:9/none");
  window.__ws.ok = true;
  window.__ws.readyState = ws.readyState;
  ws.onopen = function () { window.__ws.opened = true; };
  ws.close();
} catch (e) { window.__ws.err = String(e); }
window.__done = true;
</script></body></html>`,
}

func main() {
	log := &reqLog{}
	// ★ @font-face 端到端必须用**真实** woff2 容器：假字节会让「解码器坏了」
	//   也照样绿。这里先过一遍解码器（非空且产出 sfnt 魔数），再把同一份
	//   字节挂到 Vite 产物的字体 URL 上。
	woff2Fixture, err := os.ReadFile("engine/platform/graphics/testdata/ahem.woff2")
	if err != nil {
		fmt.Printf("读取 woff2 fixture 失败（请在 wb-ui 根目录运行）：%v\n", err)
		os.Exit(1)
	}
	sfnt, derr := graphics.DecodeEmbeddedFont(woff2Fixture)
	if derr != nil {
		fmt.Printf("woff2 fixture 解码失败：%v\n", derr)
		os.Exit(1)
	}
	check(len(sfnt) > 12 && (string(sfnt[:4]) == "\x00\x01\x00\x00" || string(sfnt[:4]) == "OTTO"),
		"woff2 解码器产出合法 sfnt（%d → %d 字节）", len(woff2Fixture), len(sfnt))
	viteSite["/vite/assets/probe.woff2"] = string(woff2Fixture)
	mux := http.NewServeMux()
	serve := func(files map[string]string) {
		for p, body := range files {
			path, content := p, body
			handler := func(w http.ResponseWriter, r *http.Request) {
				log.add(r.URL.Path)
				switch {
				case strings.HasSuffix(path, ".js"):
					w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
				case strings.HasSuffix(path, ".css"):
					w.Header().Set("Content-Type", "text/css; charset=utf-8")
				case strings.HasSuffix(path, ".png"):
					w.Header().Set("Content-Type", "image/png")
				case strings.HasSuffix(path, ".woff2"):
					w.Header().Set("Content-Type", "font/woff2")
				default:
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
				}
				if content == "PNG" {
					if strings.Contains(path, "blue") {
						w.Write(bluePNG)
						return
					}
					w.Write(greenPNG)
					return
				}
				if content == "FALLBACK" {
					// SPA fallback：深链返回入口（真实 dev server 的行为）。
					fmt.Fprint(w, spaSite["/spa/index.html"])
					return
				}
				fmt.Fprint(w, content)
			}
			mux.HandleFunc(path, handler)
			// 真实静态服务器的目录索引语义：/dir/ 与 /dir 都映射到 /dir/index.html。
			if strings.HasSuffix(path, "/index.html") {
				dir := strings.TrimSuffix(path, "index.html")
				mux.HandleFunc(dir, handler)
				mux.HandleFunc(strings.TrimSuffix(dir, "/"), handler)
			}
		}
	}
	serve(staticSite)
	serve(spaSite)
	serve(viteSite)
	serve(webpackSite)
	serve(mediaSite)
	serve(blobSite)
	serve(wsSite)

	// 跨源服务器（CORS）：一个允许任意源，一个不允许。
	corsOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			// 预检：允许 POST + content-type（覆盖非简单请求场景）。
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "cors-body")
	}))
	defer corsOK.Close()
	corsDeny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "denied-body")
	}))
	defer corsDeny.Close()

	srv := httptest.NewServer(mux)
	defer srv.Close()
	fmt.Printf("本地服务器：%s\n跨源（允许）：%s\n跨源（拒绝）：%s\n\n", srv.URL, corsOK.URL, corsDeny.URL)

	// ★ CORS 严格断言用的两个源（第 7 场景）：
	//   corsThirdPartyACAO：ACAO 设为**第三方源**（corsDeny 的源，不是文档源、也不是 *），
	//     并带 ACAC:true——credentials 模式下只查 ACAC 不查源的话就会误放行；
	//   corsDocOriginACAO：ACAO 设为文档源（对照组，证明不是一律拒绝）。
	var corsSawOrigin string
	corsThirdPartyACAO := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corsSawOrigin = r.Header.Get("Origin")
		w.Header().Set("Access-Control-Allow-Origin", corsDeny.URL)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "third-party-body")
	}))
	defer corsThirdPartyACAO.Close()
	corsDocOriginACAO := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", srv.URL)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "doc-origin-body")
	}))
	defer corsDocOriginACAO.Close()

	load := func(name, path string) *webkit.WebView {
		wv := webkit.NewWebViewWithMode(webkit.ModeBrowser)
		if err := wv.LoadURL(srv.URL + path); err != nil {
			fmt.Printf("LoadURL(%s) 失败: %v\n", path, err)
			os.Exit(1)
		}
		pumpAsync(wv)
		pump(wv, 10)
		return wv
	}

	// ── 1. 静态多页站 ─────────────────────────────────────────────
	fmt.Println("── 1. 静态多页站 ──")
	{
		wv := load("static", "/static/index.html")
		check(evalStr(wv, `String(document.getElementById("app").textContent)`) == "static",
			"静态页渲染：div#app=static")
		check(evalStr(wv, `String(window.__staticJs)`) == "js;", "相对路径 JS 执行（js/app.js）")
		check(strings.Contains(evalStr(wv, `String(window.__logo)`), "/static/img/logo.png"),
			"相对路径图片解析：%s", evalStr(wv, `String(window.__logo)`))
		check(strings.Contains(evalStr(wv, `getComputedStyle(document.body).backgroundColor`), "10, 20, 30"),
			"相对路径 CSS 生效：body 背景=%s", evalStr(wv, `getComputedStyle(document.body).backgroundColor`))
		fmt.Printf("     css diag: sheets=%s rules=%s appColor=%s bodyBg=%s\n",
			evalStr(wv, `String(document.styleSheets ? document.styleSheets.length : "no-styleSheets")`),
			evalStr(wv, `String(document.styleSheets && document.styleSheets[0] && document.styleSheets[0].cssRules ? document.styleSheets[0].cssRules.length : "n/a")`),
			evalStr(wv, `getComputedStyle(document.getElementById("app")).color`),
			evalStr(wv, `getComputedStyle(document.body).backgroundColor`))
		wv.Destroy()

		// 多页导航：链接 href 解析 + 直接加载第二页。
		w2 := load("static2", "/static/about.html")
		check(evalStr(w2, `String(document.getElementById("page").textContent)`) == "about-page",
			"多页站第二页可加载（直接 URL）")
		w2.Destroy()
	}

	// ── 2. SPA + history 路由 ────────────────────────────────────
	fmt.Println("\n── 2. SPA + history 路由 ──")
	{
		wv := load("spa", "/spa/")
		check(evalStr(wv, `String(window.__spa.boot)`) == "true", "SPA 入口脚本执行")
		// 点击路由链接 → pushState + 视图更新
		evalStr(wv, `document.getElementById("about").click()`)
		pump(wv, 10)
		check(evalStr(wv, `String(location.pathname)`) == "/spa/about",
			"pushState 更新 location.pathname（实际 %s）", evalStr(wv, `String(location.pathname)`))
		check(evalStr(wv, `String(document.getElementById("view").textContent)`) == "route:/spa/about",
			"路由视图更新（实际 %s）", evalStr(wv, `String(document.getElementById("view").textContent)`))
		// 深链直达（服务器 fallback）
		w2 := load("spa-deep", "/spa/about")
		check(evalStr(w2, `String(window.__spa.path)`) == "/spa/about",
			"SPA 深链直达（服务器 fallback）：path=%s", evalStr(w2, `String(window.__spa.path)`))
		w2.Destroy()
		wv.Destroy()
	}

	// ── 3. Vite 产物（ESM + CSS 资源）────────────────────────────
	fmt.Println("\n── 3. Vite 构建产物 ──")
	{
		wv := load("vite", "/vite/index.html")
		got := evalStr(wv, `JSON.stringify(window.__vite)`)
		fmt.Printf("     window.__vite = %s\n", got)
		check(strings.Contains(got, `"vendor":"vendor-ok"`), "ESM 入口 + vendor 分包导入")
		check(strings.Contains(got, `"lazy":"lazy-ok"`), "动态 import 懒加载 chunk")
		check(strings.Contains(evalStr(wv, `String(document.getElementById("app").textContent)`), "vite:vendor-ok"),
			"Vite 入口脚本渲染 DOM")
		check(log.has("/vite/assets/base-abc.css"), "CSS @import 加载（base-abc.css）")
		check(log.has("/vite/assets/probe.woff2"), "@font-face 字体文件请求")
		check(log.has("/vite/assets/bg-card.png"), "CSS background-image 请求")
		check(strings.Contains(evalStr(wv, `getComputedStyle(document.getElementById("app")).color`), "1, 2, 3"),
			"@import 进来的样式参与层叠（#app 颜色=%s）", evalStr(wv, `getComputedStyle(document.getElementById("app")).color`))
		check(strings.Contains(evalStr(wv, `getComputedStyle(document.getElementById("app")).fontFamily`), "ProbeFont"),
			"@font-face 规则进入层叠（字体族=%s）", evalStr(wv, `getComputedStyle(document.getElementById("app")).fontFamily`))
		wv.Destroy()
	}

	// ── 4. Webpack 产物 ─────────────────────────────────────────
	fmt.Println("\n── 4. Webpack 构建产物 ──")
	{
		wv := load("webpack", "/webpack/index.html")
		got := evalStr(wv, `JSON.stringify(window.__wp)`)
		fmt.Printf("     window.__wp = %s\n", got)
		check(strings.Contains(evalStr(wv, `String(document.getElementById("app").textContent)`), "webpack:main"),
			"defer 脚本执行（main.js）")
		check(strings.Contains(got, `"chunk-ok"`), "运行时插入异步 chunk 并执行")
		wv.Destroy()
	}

	// ── 5. 响应式图片 / SVG ─────────────────────────────────────
	fmt.Println("\n── 5. 响应式图片 / SVG ──")
	{
		wv := load("media", "/media/index.html")
		got := evalStr(wv, `JSON.stringify(window.__media)`)
		fmt.Printf("     window.__media = %s\n", got)
		check(strings.Contains(got, `"pic"`), "picture/source 中的 img 触发 load")
		check(log.has("/media/green.png"), "srcset 默认候选加载（green.png）")
		wv.Destroy()
	}

	// ── 6. blob: URL + data: 模块 ───────────────────────────────
	fmt.Println("\n── 6. blob: URL / data: 模块 ──")
	{
		wv := load("blob", "/blob/index.html")
		got := evalStr(wv, `JSON.stringify(window.__blob)`)
		fmt.Printf("     window.__blob = %s\n", got)
		check(strings.Contains(got, `"hasCreate":true`), "URL.createObjectURL 存在")
		check(strings.Contains(got, `"text":"blob-body"`), "fetch(blobURL) 读到 blob 内容")
		check(strings.Contains(got, `"dataModule":42`), "data: 模块 import 可用")
		wv.Destroy()
	}

	// ── 7. 跨源 fetch（CORS）────────────────────────────────────
	fmt.Println("\n── 7. 跨源 fetch（CORS）──")
	{
		page := `<!DOCTYPE html><html><body><script>
window.__cors = {};
fetch("` + corsOK.URL + `/ok").then(function (r) { return r.text(); }).then(function (t) {
  window.__cors.ok = t;
  // 拒绝：无 ACAO → fetch 必须 reject（内容不可读）。
  return fetch("` + corsDeny.URL + `/deny").then(function (r) { return r.text(); },
     function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.deny = t;
  // 非简单请求：先 OPTIONS 预检，通过后再发 POST。
  return fetch("` + corsOK.URL + `/pre", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: "{}"
    }).then(function (r) { return r.text(); }, function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.preflight = t;
  // credentials: include + ACAO:* → 必须 reject（浏览器语义）。
  return fetch("` + corsOK.URL + `/ok", { credentials: "include" })
    .then(function (r) { return "ok:" + r.status; }, function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.credentials = t;
  // ★ credentials + ACAO 指向**第三方源**（带 ACAC:true）：必须 reject——
  //   credentials 模式下 ACAO 必须精确等于文档源，不能是任何其他源。
  return fetch("` + corsThirdPartyACAO.URL + `/cred", { credentials: "include" })
    .then(function (r) { return "ok:" + r.status; }, function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.credentialsThirdParty = t;
  // no-cors：opaque 响应（status 0 / type opaque / 内容不可读）。
  return fetch("` + corsOK.URL + `/ok", { mode: "no-cors" })
    .then(function (r) { return "status:" + r.status + ":type:" + r.type; },
          function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.noCors = t;
  // 同源：文档自己的服务器（无 ACAO）——不受 CORS 限制。
  return fetch("/cors/same-origin")
    .then(function (r) { return r.text(); }, function (e) { return "rejected:" + e; });
}).then(function (t) {
  window.__cors.sameOrigin = t;
  window.__done = true;
});
</script></body></html>`
		mux.HandleFunc("/cors/index.html", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, page)
		})
		mux.HandleFunc("/cors/same-origin", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "same-origin-body")
		})
		wv := load("cors", "/cors/index.html")
		got := evalStr(wv, `JSON.stringify(window.__cors)`)
		fmt.Printf("     window.__cors = %s\n", got)
		check(strings.Contains(got, `"ok":"cors-body"`), "跨源 fetch（ACAO:*）成功")
		check(strings.Contains(got, `"deny":"rejected`) && strings.Contains(got, "Access-Control-Allow-Origin"),
			"跨源 fetch（无 ACAO）被拒绝（错误信息指出缺少 ACAO）")
		check(strings.Contains(got, `"preflight":"cors-body"`), "非简单请求：OPTIONS 预检通过后成功")
		check(strings.Contains(got, `"credentials":"rejected`), "credentials:include + ACAO:* 被拒绝")
		check(strings.Contains(got, `"credentialsThirdParty":"rejected`),
			"credentials:include + ACAO 指向第三方源被拒绝（必须精确等于文档源）")
		check(strings.Contains(got, `"noCors":"status:0:type:opaque"`), "mode:no-cors 返回 opaque 响应")
		check(strings.Contains(got, `"sameOrigin":"same-origin-body"`), "同源请求不受 CORS 限制")
		wv.Destroy()
	}

	// ── 7b. 源与解析基准分离（<base href> 指向异源）──────────────
	fmt.Println("\n── 7b. 源与解析基准分离 ──")
	{
		// 页面用 <base href> 把**解析基准**指到异源；fetch 用绝对 URL，因此这里
		// 只考察 Origin 的计算。正确行为：Origin 取文档 URL（本文档服务器的源），
		// 而不是 baseURI —— 否则页面写一行 <base href> 就能伪造源绕过 CORS。
		basePage := `<!DOCTYPE html><html><head>
<base href="` + corsDeny.URL + `/">` + `</head><body><script>
window.__corsBase = {};
fetch("` + corsThirdPartyACAO.URL + `/x").then(function (r) { return r.text(); })
  .then(function (t) { window.__corsBase.thirdParty = "leaked:" + t; },
        function (e) { window.__corsBase.thirdParty = "rejected:" + e; })
  .then(function () {
    return fetch("` + corsDocOriginACAO.URL + `/y")
      .then(function (r) { return r.text(); }, function (e) { return "rejected:" + e; });
  }).then(function (t) {
    window.__corsBase.docOrigin = t;
    window.__done = true;
  });
</script></body></html>`
		mux.HandleFunc("/cors/base.html", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, basePage)
		})
		corsSawOrigin = ""
		wv := load("cors-base", "/cors/base.html")
		got := evalStr(wv, `JSON.stringify(window.__corsBase)`)
		fmt.Printf("     window.__corsBase = %s（服务器收到 Origin=%q）\n", got, corsSawOrigin)
		check(corsSawOrigin == srv.URL,
			"<base href> 指向异源时 Origin 仍取文档源（实际 "+corsSawOrigin+"，期望 "+srv.URL+"）")
		check(strings.Contains(got, `"thirdParty":"rejected`),
			"ACAO 指向 <base href> 源（第三方）必须 reject（不能因 baseURI 伪造源而放行）")
		check(strings.Contains(got, `"docOrigin":"doc-origin-body"`),
			"ACAO=文档源时通过（对照组）")
		wv.Destroy()
	}

	// ── 8. WebSocket 构造 ───────────────────────────────────────
	fmt.Println("\n── 8. WebSocket ──")
	{
		wv := load("ws", "/ws/index.html")
		got := evalStr(wv, `JSON.stringify(window.__ws)`)
		fmt.Printf("     window.__ws = %s\n", got)
		check(strings.Contains(got, `"ok":true`), "new WebSocket(...) 不抛异常（引擎 stub）")
		wv.Destroy()
	}

	// ── 9. ModeToolkit 宿主堵漏 ─────────────────────────────────
	// UI 库模式必须：外部资源（http CSS/图片/字体）一律拒绝且**不发请求**，
	// 页面自产内容（blob:）照常可用，fetch 无桥路由时拒绝联网。
	fmt.Println("\n── 9. ModeToolkit 宿主堵漏 ──")
	{
		log.reset()
		wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
		html := `<!DOCTYPE html><html><head>
<link rel="stylesheet" href="` + srv.URL + `/static/css/site.css">
<style>@font-face { font-family: "Ext"; src: url("` + srv.URL + `/vite/assets/probe.woff2"); }</style>
</head><body><img id="img" src="` + srv.URL + `/static/img/logo.png">
<div id="out">toolkit-ok</div>
<script>
window.__tk = {};
var u = URL.createObjectURL(new Blob(["tk-blob"], { type: "text/plain" }));
fetch(u).then(function (r) { return r.text(); }).then(function (t) {
  window.__tk.blob = t;
  return fetch("` + srv.URL + `/static/js/app.js").then(
    function () { window.__tk.net = "leaked"; },
    function (e) { window.__tk.net = String(e); });
}).then(function () {
  window.__done = true;
});
</script></body></html>`
		if err := wv.LoadHTML(html); err != nil {
			fmt.Printf("Toolkit LoadHTML 失败: %v\n", err)
			failures++
		}
		pumpAsync(wv)
		got := evalStr(wv, `JSON.stringify(window.__tk)`)
		fmt.Printf("     window.__tk = %s\n", got)
		check(evalStr(wv, `String(document.getElementById("out").textContent)`) == "toolkit-ok",
			"Toolkit：页面仍装配/渲染（被拒资源不中断装配）")
		check(strings.Contains(got, `"blob":"tk-blob"`),
			"Toolkit：页面自产 blob 可读取（fetch(blob:) 不联网）")
		check(strings.Contains(got, "UI 库模式") && !strings.Contains(got, "leaked"),
			"Toolkit：fetch 外部 URL 被拒（无桥路由不联网）")
		check(!log.has("/static/css/site.css"), "Toolkit：外部 CSS 被拒（未发出网络请求）")
		check(!log.has("/static/img/logo.png"), "Toolkit：外部图片被拒（未发出网络请求）")
		check(!log.has("/vite/assets/probe.woff2"), "Toolkit：外部字体被拒（未发出网络请求）")
		check(!log.has("/static/js/app.js"), "Toolkit：外部脚本被拒（未发出网络请求）")
		wv.Destroy()
	}

	fmt.Printf("\n请求轨迹（前 2000 字符）：\n%s\n", truncate(log.dump(), 2000))
	if failures > 0 {
		fmt.Printf("\n结果：%d 项失败\n", failures)
		os.Exit(1)
	}
	fmt.Println("\n结果：全部通过")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var _ = json.Marshal
