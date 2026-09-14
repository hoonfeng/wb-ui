package webkit

// 图片（`<img src>`）与 CSS `@import` 的真实 HTTP 端到端回归。
//
// 这两条此前**根本没有 URL 加载通道**：
//   - `<img src>` 由渲染层自己处理——http(s) 走渲染层内置 httpGet（绕开宿主
//     ResourceResolver 与运行模式：UI 库模式下会真的联网），相对引用被当成
//     宿主进程工作目录下的文件（真实页面里的相对图片必然加载失败）；
//   - `@import` 因 `style.Resolver.StyleSheetLoader` 从未被接线，resolveImports
//     第一行就返回——页面里所有 @import 静默跳过；且相对 @import 的基准应是
//     **样式表 URL**，不是文档 URL。
//
// 这里真起服务器验证：加载、URL 基准、解码尺寸、绘制像素、UI 库模式拒绝，
// 以及「全局图片缓存不得旁路模式门禁」这条特有回归。

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wb-ui/rendering"
)

// mediaHTTPFixture：图片与 @import 的 fixture。
//
// 目录刻意错开（文档在 /imp/page/，样式表在 /imp/css/），这样「相对 @import
// 按样式表 URL 解析」与「按文档 URL 解析」会落到不同路径：
// 正确 → /imp/css/theme.css；错误 → /imp/page/theme.css（404）。
func mediaHTTPFixture(t *testing.T) (*httptest.Server, *httpRequestLog) {
	t.Helper()
	log := &httpRequestLog{}
	mux := http.NewServeMux()
	serve := func(path, ctype, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			log.add(r.URL.Path)
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			fmt.Fprint(w, body)
		})
	}

	// 图片页：`<img>` 用**文档相对**引用（src="logo.png" → /logo.png），
	// 尺寸由内联样式给出（不依赖 HTML width/height 属性的布局支持）。
	serve("/img.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}</style>`+
			`</head><body><img id="pic" src="logo.png"></body></html>`)
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(redPNG4x4(t))
	})
	// 两个不同目录下的**同名**相对引用 logo.png（内容不同：红 / 蓝）：
	// 用来锁定「图片 URL 必须先按文档基准规范化再进缓存」，否则第二个文档
	// 会命中第一个文档留下的缓存（跨文档串味）。
	imgPage := func() string {
		return `<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}` +
			`</style></head><body><img id="pic" src="logo.png"></body></html>`
	}
	serve("/x/page.html", "text/html; charset=utf-8", imgPage())
	serve("/y/page.html", "text/html; charset=utf-8", imgPage())
	mux.HandleFunc("/x/logo.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{R: 255, A: 255}))
	})
	mux.HandleFunc("/y/logo.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{B: 255, A: 255}))
	})
	// background-image 通道：与 <img> 共用 loadBackgroundImage，但**不**附着
	// 解码图到 RenderBox（断言看渲染像素）。
	serve("/bg.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><style>`+
			`#bg{width:24px;height:24px;background-image:url(logo.png);background-size:100% 100%}`+
			`</style></head><body><div id="bg"></div></body></html>`)

	// 外部样式表里的相对 url()：CSS 规范要求以**样式表自身 URL** 为基准
	// （CSS Values 3 §4.4 / CSS Syntax §5.4），与文档 URL 无关。
	// ★ 两个目录必须在**不同深度**上：文档 /cssurl/page/index.html、
	//   样式表 /cssurl/theme/css/site.css，样式表里 `url(../assets/bg.png)`：
	//   样式表基准 → /cssurl/theme/assets/bg.png（红，正确）
	//   文档基准   → /cssurl/assets/bg.png（蓝，错误的那份也真实存在）
	//   同深度（/cssurl/theme/）时两种基准会算出同一个 URL，测试就抓不到回归
	//   ——这条是反向验证暴露出来的 fixture 缺陷。
	serve("/cssurl/page/index.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="../theme/css/site.css">`+
			`</head><body><div id="bg"></div></body></html>`)
	serve("/cssurl/theme/css/site.css", "text/css",
		`#bg{width:24px;height:24px;background-image:url(../assets/bg.png);background-size:100% 100%}`)
	mux.HandleFunc("/cssurl/theme/assets/bg.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{R: 255, A: 255}))
	})
	mux.HandleFunc("/cssurl/assets/bg.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{B: 255, A: 255}))
	})

	// 同一条 `url(…)` 写在两处的基准对照：内联（style 属性 / 内联 <style>）
	// 相对**文档**，外部样式表相对**样式表自身**。两个文件名各自只存在于
	// 一个目录里，"正确路径被请求 + 错误路径未被请求" 四条断言因此能各自
	// 指认基准，而不只是看到 404。
	serve("/cssurl/page/inline.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="../theme/inline.css">`+
			`</head><body>`+
			`<div id="a" style="width:24px;height:24px;background-size:100% 100%;`+
			`background-image:url(inline-only.png)"></div>`+
			`<div id="b"></div></body></html>`)
	serve("/cssurl/theme/inline.css", "text/css",
		`#b{width:24px;height:24px;background-size:100% 100%;background-image:url(sheet-only.png)}`)
	mux.HandleFunc("/cssurl/page/inline-only.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{G: 255, A: 255}))
	})
	mux.HandleFunc("/cssurl/theme/sheet-only.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{R: 255, G: 255, A: 255}))
	})

	// @import 页：文档在 /imp/page/，样式表在 /imp/css/，样式表里再嵌套
	// 一层 `../shared/base.css`（验证逐级基准拼接）。
	serve("/imp/page/index.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="../css/main.css">`+
			`</head><body><div id="box">x</div><div id="nested">y</div></body></html>`)
	// main.css 里的 `#nested{width:666px}` 与 base.css（经嵌套 @import 加载）
	// 的 `#nested{width:555px}` 争同一属性：按 CSS 规范的源序，@import 的
	// 规则排在导入语句所在表之前 → 666 必须胜出。若实现把导入表追加在
	// 「当前表之后」，就会得到 555（这条同时说明两个表确实都被加载了）。
	serve("/imp/css/main.css", "text/css", `@import "theme.css";#nested{width:666px}`)
	serve("/imp/css/theme.css", "text/css", `@import "../shared/base.css";#box{width:444px}`)
	serve("/imp/shared/base.css", "text/css", `#nested{width:555px}`)
	// 故意在**文档**同级也放一份 theme.css：若相对 @import 被按文档 URL
	// 解析，就会拿到这份（999px）——断言因此能明确指认「基准错了」，
	// 而不是只看到 404 造成的「没生效」。
	serve("/imp/page/theme.css", "text/css", `#box{width:999px}`)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, log
}

// redPNG4x4 生成 4x4 纯红 PNG：像素断言用（不依赖硬编码 base64）。
func redPNG4x4(t *testing.T) []byte {
	t.Helper()
	return solidPNG4x4(t, color.RGBA{R: 255, A: 255})
}

// solidPNG4x4 生成 4x4 纯色 PNG（像素断言用，不依赖硬编码 base64）。
func solidPNG4x4(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// waitBoxImage 反复渲染若干帧，直到 #id 的渲染盒附着了已解码的图片
// （图片加载是异步的：首帧发起请求，后续帧命中缓存才画得出来）。
// 返回 nil 表示 frames 帧内没有附着（测试据此报错/断言"未加载"）。
func waitBoxImage(t *testing.T, wv *WebView, id string, frames int) *rendering.RenderBox {
	t.Helper()
	doc := wv.Document()
	if doc == nil {
		t.Fatal("文档缺失")
	}
	el := doc.GetElementById(id)
	if el == nil {
		t.Fatalf("#%s 元素缺失", id)
	}
	for i := 0; i < frames; i++ {
		wv.EnsureHitTestReady()
		if rv := wv.RenderView(); rv != nil {
			if box := rv.FindRenderBoxForNode(el); box != nil {
				if img := box.DecodedImage(); img != nil && img.Loaded() {
					return box
				}
			}
		}
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	return nil
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

// TestBrowserModeHTTPImageLoadsAndPaints：文档相对 `<img src="logo.png">`
// 必须经真实网络加载 → 解码（固有尺寸 4x4）→ 画到元素位置（红色像素）。
func TestBrowserModeHTTPImageLoadsAndPaints(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/img.html"); err != nil {
		t.Fatalf("LoadURL(/img.html): %v", err)
	}

	box := waitBoxImage(t, wv, "pic", 80)
	if box == nil {
		t.Fatalf("<img src=\"logo.png\"> 未加载出解码图（服务器收到：%v）", log.paths())
	}
	img := box.DecodedImage()
	if gotW, gotH := float64(img.Width()), float64(img.Height()); gotW != 4 || gotH != 4 {
		t.Errorf("解码图固有尺寸 = %.0fx%.0f, want 4x4", gotW, gotH)
	}
	if !log.has("/logo.png") {
		t.Errorf("服务器未收到 /logo.png（实际收到：%v）", log.paths())
	}

	// 绘制验证：渲染一帧，取 <img> 盒中心像素。
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cx := int(box.AbsoluteX() + box.Width()/2)
	cy := int(box.AbsoluteY() + box.Height()/2)
	r, g, b, a := pixelAt(pix, wv.Width(), cx, cy)
	if !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("<img> 位置 (%d,%d) 像素 = rgba(%d,%d,%d,%d), want 红", cx, cy, r, g, b, a)
	}
}

// TestBrowserModeCSSImportLoadsAndBaseIsStyleSheetURL：外部样式表里的
// `@import`（含嵌套）必须加载，且相对引用以**样式表 URL** 为基准解析——
// 按文档 URL 会去请求 /imp/page/theme.css（不存在）而不是 /imp/css/theme.css。
func TestBrowserModeCSSImportLoadsAndBaseIsStyleSheetURL(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/imp/page/index.html"); err != nil {
		t.Fatalf("LoadURL(/imp/page/index.html): %v", err)
	}

	browserBoxWidth(t, wv) // 触发布局
	doc := wv.Document()
	for _, tc := range []struct {
		id    string
		width float64
		what  string
	}{
		{"box", 444, "样式表同级的 @import（main.css → theme.css）"},
		{"nested", 666, "嵌套 @import 的源序（@import 规则先于本表规则）"},
	} {
		box := findBox(wv, doc.GetElementById(tc.id))
		if box == nil {
			t.Fatalf("#%s 渲染盒缺失", tc.id)
		}
		if math.Abs(box.W-tc.width) > 1 {
			t.Errorf("%s 未生效：#%s 宽度 = %.1f, want ≈%.0f（服务器收到：%v）",
				tc.what, tc.id, box.W, tc.width, log.paths())
		}
	}

	for _, p := range []string{"/imp/css/main.css", "/imp/css/theme.css", "/imp/shared/base.css"} {
		if !log.has(p) {
			t.Errorf("服务器未收到 %s（实际收到：%v）", p, log.paths())
		}
	}
	if log.has("/imp/page/theme.css") {
		t.Errorf("相对 @import 被按**文档** URL 解析（拿到了 /imp/page/theme.css 而非样式表同级的 /imp/css/theme.css）：%v",
			log.paths())
	}
}

// TestToolkitModeRejectsImageURLsEvenWhenCached：UI 库模式下 http(s) 图片
// 引用必须被拒绝——**即便**另一个（浏览器模式）WebView 已经把同一 URL 取回
// 并留在进程级全局图片缓存里。只查缓存会让模式门禁被缓存旁路，图片照样
// 显示出来（实现期间实测到过这一条）。
func TestToolkitModeRejectsImageURLsEvenWhenCached(t *testing.T) {
	srv, log := mediaHTTPFixture(t)

	// 1) 浏览器模式先把 /logo.png 取回（此后全局缓存里有这个绝对 URL）。
	br := modeWebView(t, ModeBrowser)
	if err := br.LoadURL(srv.URL + "/img.html"); err != nil {
		t.Fatalf("LoadURL(/img.html): %v", err)
	}
	if waitBoxImage(t, br, "pic", 80) == nil {
		t.Fatalf("浏览器模式未能加载图片（服务器收到：%v）", log.paths())
	}

	// 2) UI 库模式引用同一绝对 URL：不得显示、不得发请求。
	tk := modeWebView(t, ModeToolkit)
	modeMustLoad(t, tk, `<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}`+
		`</style></head><body><img id="pic" src="`+srv.URL+`/logo.png"></body></html>`)
	before := len(log.paths())
	if box := waitBoxImage(t, tk, "pic", 8); box != nil {
		t.Errorf("UI 库模式下 <img src=\"%s/logo.png\"> 显示了图片（解码图已附着）", srv.URL)
	}
	if paths := log.paths(); len(paths) != before {
		t.Errorf("UI 库模式产生了网络请求：%v", paths[before:])
	}

	// 3) 相对引用同样拒绝（无网络可发，且不应读出宿主工作目录里的同名文件）。
	modeMustLoad(t, tk, `<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}`+
		`</style></head><body><img id="pic" src="logo.png"></body></html>`)
	if box := waitBoxImage(t, tk, "pic", 8); box != nil {
		t.Errorf("UI 库模式下相对 <img src=\"logo.png\"> 显示了图片")
	}
}

// TestBrowserModeHTTPBackgroundImage：CSS background-image 与 `<img>` 共用
// 同一条图片接线（loadBackgroundImage）——相对引用同样按文档 URL 解析、同样
// 经宿主策略取字节。背景图不附着解码图到 RenderBox，因此断言看渲染像素。
func TestBrowserModeHTTPBackgroundImage(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/bg.html"); err != nil {
		t.Fatalf("LoadURL(/bg.html): %v", err)
	}

	// 页面只有 #bg（body margin 8px）→ 元素占 (8,8)-(32,32)，取中心。
	const x, y = 20, 20
	var r, g, b, a uint8
	for i := 0; i < 80; i++ {
		pix, err := wv.Render()
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		r, g, b, a = pixelAt(pix, wv.Width(), x, y)
		if r > 200 && g < 80 && b < 80 && a > 200 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("background-image 位置 (%d,%d) 像素 = rgba(%d,%d,%d,%d), want 红（服务器收到：%v）",
			x, y, r, g, b, a, log.paths())
	}
	if !log.has("/logo.png") {
		t.Errorf("服务器未收到 /logo.png（实际收到：%v）", log.paths())
	}
}

// TestBrowserModeStylesheetURLBaseForCSSURLs：外部样式表里的相对 url() 必须
// 相对**样式表自身 URL** 解析（CSS Values 3 §4.4），不是文档 URL。文档在
// /cssurl/page/、样式表在 /cssurl/theme/css/，`url(../assets/bg.png)` 因此：
//
//	正确基准（样式表）→ /cssurl/theme/assets/bg.png（红）
//	错误基准（文档）  → /cssurl/assets/bg.png（蓝，刻意真实存在）
//
// 三路证据：请求路径、**是否**请求了错误路径、以及画出来的像素颜色。
func TestBrowserModeStylesheetURLBaseForCSSURLs(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/cssurl/page/index.html"); err != nil {
		t.Fatalf("LoadURL(/cssurl/page/index.html): %v", err)
	}

	// #bg 是 body 第一个子元素（body margin 8px）→ 占 (8,8)-(32,32)，取中心。
	const x, y = 20, 20
	var r, g, b, a uint8
	for i := 0; i < 80; i++ {
		pix, err := wv.Render()
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		r, g, b, a = pixelAt(pix, wv.Width(), x, y)
		if r > 200 || b > 200 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !log.has("/cssurl/theme/css/site.css") {
		t.Fatalf("外部样式表未加载（服务器收到：%v）", log.paths())
	}
	if !log.has("/cssurl/theme/assets/bg.png") {
		t.Errorf("服务器未收到样式表同级的 /cssurl/theme/assets/bg.png（实际收到：%v）", log.paths())
	}
	if log.has("/cssurl/assets/bg.png") {
		t.Errorf("相对 url() 被按**文档** URL 解析：请求了 /cssurl/assets/bg.png（应相对样式表 → /cssurl/theme/assets/bg.png）")
	}
	if !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("#bg 中心 (%d,%d) 像素 = rgba(%d,%d,%d,%d), want 红（= 样式表同级图片；服务器收到：%v）",
			x, y, r, g, b, a, log.paths())
	}
}

// TestBrowserModeDocumentBaseForInlineURLs：`style` 属性（内联声明）里的相对
// url() 按**文档** URL 解析——它是文档的一部分，不是某张样式表的一部分；
// 同一页面里外部样式表的相对 url() 按**样式表** URL 解析。互不串用。
//
// 两个文件名各自只存在一个目录，四条断言（两条正、两条负）因此能分别指认
// 两条通道的基准，而不是笼统地"404 了"。
func TestBrowserModeDocumentBaseForInlineURLs(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/cssurl/page/inline.html"); err != nil {
		t.Fatalf("LoadURL(/cssurl/page/inline.html): %v", err)
	}
	wv.EnsureHitTestReady()
	for i := 0; i < 40; i++ {
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !log.has("/cssurl/page/inline-only.png") {
		t.Errorf("内联 style 属性里的相对 url() 未按文档 URL 解析：未收到 /cssurl/page/inline-only.png（实际：%v）",
			log.paths())
	}
	if !log.has("/cssurl/theme/sheet-only.png") {
		t.Errorf("外部样式表里的相对 url() 未按样式表 URL 解析：未收到 /cssurl/theme/sheet-only.png（实际：%v）",
			log.paths())
	}
	if log.has("/cssurl/theme/inline-only.png") {
		t.Errorf("内联声明被当成样式表内的声明（按样式表基准解析了内联 url()）")
	}
	if log.has("/cssurl/page/sheet-only.png") {
		t.Errorf("外部样式表里的 url() 被按文档基准解析（请求了 /cssurl/page/sheet-only.png）")
	}
}

// TestBrowserModeHTTPImageNoCrossDocumentCacheBleed：两个不同目录下的**同名**
// 相对引用（/x/logo.png 红、/y/logo.png 蓝）必须各自加载自己的那份。
//
// 图片缓存（rendering.backgroundImageCache）是进程级全局的、按键索引：若
// 页面写下的相对引用**不先按文档 URL 规范化**就进缓存，两个文档会共用
// "logo.png" 这一个键——第二个文档显示第一个文档的图（跨文档串味）。
// 这条是 imageLoader.ResolveURL 存在的实际理由，也是它的回归测试。
func TestBrowserModeHTTPImageNoCrossDocumentCacheBleed(t *testing.T) {
	srv, log := mediaHTTPFixture(t)
	wv := modeWebView(t, ModeBrowser)

	// 文档一：/x/page.html → /x/logo.png（红）
	if err := wv.LoadURL(srv.URL + "/x/page.html"); err != nil {
		t.Fatalf("LoadURL(/x/page.html): %v", err)
	}
	if waitBoxImage(t, wv, "pic", 80) == nil {
		t.Fatalf("/x/logo.png 未加载（服务器收到：%v）", log.paths())
	}
	if r, g, b, a := renderBoxCenterPixel(t, wv, "pic"); !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("/x/logo.png 像素 = rgba(%d,%d,%d,%d), want 红", r, g, b, a)
	}

	// 文档二：/y/page.html 里同样写 src="logo.png" → 必须是 /y/logo.png（蓝）
	if err := wv.LoadURL(srv.URL + "/y/page.html"); err != nil {
		t.Fatalf("LoadURL(/y/page.html): %v", err)
	}
	if waitBoxImage(t, wv, "pic", 80) == nil {
		t.Fatalf("/y/logo.png 未加载（服务器收到：%v）", log.paths())
	}
	if r, g, b, a := renderBoxCenterPixel(t, wv, "pic"); !(b > 200 && r < 80 && g < 80 && a > 200) {
		t.Errorf("/y/logo.png 像素 = rgba(%d,%d,%d,%d), want 蓝（疑似命中 /x 的缓存）", r, g, b, a)
	}
	if !log.has("/y/logo.png") {
		t.Errorf("服务器未收到 /y/logo.png（实际收到：%v）", log.paths())
	}
}

// renderBoxCenterPixel 渲染一帧并取 #id 盒中心像素（RGBA8888）。
func renderBoxCenterPixel(t *testing.T, wv *WebView, id string) (r, g, b, a uint8) {
	t.Helper()
	doc := wv.Document()
	if doc == nil {
		t.Fatal("文档缺失")
	}
	box := findBox(wv, doc.GetElementById(id))
	if box == nil {
		t.Fatalf("#%s 渲染盒缺失", id)
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cx, cy := box.Center()
	return pixelAt(pix, wv.Width(), int(cx), int(cy))
}
