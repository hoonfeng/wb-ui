package webkit

// 外部资源的内存缓存（浏览器 memory cache 语义）与「按用途的 MIME 检查」
// （nosniff）的真实回归。
//
// 引擎此前：每次引用都重新取内容（同一 CSS 被多个 `<link>` 引用、样式重扫、
// UI 库模式下宿主 ResourceResolver 被反复调用），且 MIME 从不拒绝——浏览器则
// 会缓存，并在 `X-Content-Type-Options: nosniff` 下按类型拒绝。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func countPath(log *httpRequestLog, path string) int {
	n := 0
	for _, p := range log.paths() {
		if p == path {
			n++
		}
	}
	return n
}

// TestResourceCacheFetchesOnce：同一 URL 在一次页面会话里只取一次
// （多个 `<link>` 指向同一 CSS、重复引用同一脚本）。
func TestResourceCacheFetchesOnce(t *testing.T) {
	log := &httpRequestLog{}
	mux := http.NewServeMux()
	mux.HandleFunc("/twice.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head>`+
			`<link rel="stylesheet" href="dup.css">`+
			`<link rel="stylesheet" href="dup.css">`+
			`</head><body></body></html>`)
	})
	mux.HandleFunc("/dup.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, `body{margin:0}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/twice.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	if n := countPath(log, "/dup.css"); n != 1 {
		t.Errorf("同一 CSS 被取回 %d 次（请求序列 %v），want 1（浏览器 memory cache 语义）", n, log.paths())
	}

	// ClearResourceCache 后重新取（宿主内容变化时的强制刷新入口）。
	wv.ClearResourceCache()
	if err := wv.LoadURL(srv.URL + "/twice.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	if n := countPath(log, "/dup.css"); n != 2 {
		t.Errorf("清缓存后重取次数 = %d, want 2", n)
	}
}

// TestResourceCacheCoversResolver：UI 库模式下宿主 ResourceResolver 的结果也
// 缓存（样式重扫不再重复打扰宿主），换 resolver 时自动失效。
func TestResourceCacheCoversResolver(t *testing.T) {
	wv := modeWebView(t, ModeToolkit)
	calls := map[string]int{}
	var body string
	wv.SetResourceResolver(func(ref string) (string, bool) {
		calls[ref]++
		if ref == "app://ui/app.css" {
			return body, true
		}
		return "", false
	})
	html := `<html><head><link rel="stylesheet" href="app.css">` +
		`<link rel="stylesheet" href="app.css"></head><body><div id="box"></div></body></html>`
	body = `#box{position:absolute;left:0;top:0;width:20px;height:20px;background:#ff0000}`
	if err := wv.LoadHTMLWithBaseURL(html, "app://ui/index.html"); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	if got := calls["app://ui/app.css"]; got != 1 {
		t.Errorf("resolver 被调用 %d 次, want 1（同一引用应命中缓存）", got)
	}

	// 换 resolver → 缓存失效（旧内容不再被使用）。
	body = `#box{position:absolute;left:0;top:0;width:20px;height:20px;background:#0000ff}`
	wv.SetResourceResolver(func(ref string) (string, bool) {
		calls[ref]++
		if ref == "app://ui/app.css" {
			return body, true
		}
		return "", false
	})
	if err := wv.LoadHTMLWithBaseURL(html, "app://ui/index.html"); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	if got := calls["app://ui/app.css"]; got != 2 {
		t.Errorf("换 resolver 后调用次数 = %d, want 2（SetResourceResolver 应清空缓存）", got)
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	r, g, b, a := pixelAt(pix, wv.Width(), 5, 5)
	if !(b > 200 && r < 80 && a > 200) {
		t.Errorf("换 resolver 后 (5,5) = rgba(%d,%d,%d,%d), want 蓝（旧缓存内容仍在生效？）", r, g, b, a)
	}
}

// TestMIMENosniffRejectsWrongType：带 `X-Content-Type-Options: nosniff` 时，
// `<link rel=stylesheet>` / `<script src>` 按 MIME 拒绝（浏览器语义）；
// 不带 nosniff 时宽松接受。
func TestMIMENosniffRejectsWrongType(t *testing.T) {
	const rule = `#box{position:absolute;left:0;top:0;width:30px;height:30px;background:%s}`
	mux := http.NewServeMux()
	serve := func(path, ctype string, nosniff bool, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			if nosniff {
				w.Header().Set("X-Content-Type-Options", "nosniff")
			}
			fmt.Fprint(w, body)
		})
	}
	serve("/strict.css", "text/plain", true, fmt.Sprintf(rule, "#ff0000"))
	serve("/loose.css", "text/plain", false, fmt.Sprintf(rule, "#0000ff"))
	serve("/strict.html", "text/html", false,
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="strict.css"></head>`+
			`<body><div id="box"></div></body></html>`)
	serve("/loose.html", "text/html", false,
		`<!DOCTYPE html><html><head><link rel="stylesheet" href="loose.css"></head>`+
			`<body><div id="box"></div></body></html>`)
	serve("/nosniff.js", "text/plain", true, `window.__ran = 1;`)
	serve("/plain.js", "text/plain", false, `window.__ran = 2;`)
	serve("/script-nosniff.html", "text/html", false,
		`<!DOCTYPE html><html><head><script src="nosniff.js"></script></head><body>x</body></html>`)
	serve("/script-plain.html", "text/html", false,
		`<!DOCTYPE html><html><head><script src="plain.js"></script></head><body>x</body></html>`)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// ① nosniff 的 text/plain CSS：不采用（红块不存在）。
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/strict.html"); err != nil {
		t.Fatalf("LoadURL(strict.html): %v", err)
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if r, g, b, a := pixelAt(pix, wv.Width(), 5, 5); r > 200 && g < 80 && b < 80 && a > 200 {
		t.Errorf("nosniff 的 text/plain 样式表被采用了（(5,5) = rgba(%d,%d,%d,%d) 是红色）", r, g, b, a)
	}

	// ② 同样内容、无 nosniff：宽松接受（浏览器 quirk 兼容行为）。
	wv2 := modeWebView(t, ModeBrowser)
	if err := wv2.LoadURL(srv.URL + "/loose.html"); err != nil {
		t.Fatalf("LoadURL(loose.html): %v", err)
	}
	pix2, err := wv2.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if r, g, b, a := pixelAt(pix2, wv2.Width(), 5, 5); !(b > 200 && r < 80 && a > 200) {
		t.Errorf("无 nosniff 的 text/plain 样式表未被采用（(5,5) = rgba(%d,%d,%d,%d), want 蓝）", r, g, b, a)
	}

	// ③ nosniff 的 text/plain 脚本：不执行；无 nosniff 时执行。
	wv3 := modeWebView(t, ModeBrowser)
	if err := wv3.LoadURL(srv.URL + "/script-nosniff.html"); err != nil {
		t.Fatalf("LoadURL(script-nosniff.html): %v", err)
	}
	if got := evalNavStr(t, wv3, `typeof window.__ran`); got != "undefined" {
		t.Errorf("nosniff 的 text/plain 脚本执行了（window.__ran = %s）", got)
	}
	wv4 := modeWebView(t, ModeBrowser)
	if err := wv4.LoadURL(srv.URL + "/script-plain.html"); err != nil {
		t.Fatalf("LoadURL(script-plain.html): %v", err)
	}
	if got := evalNavStr(t, wv4, `String(window.__ran)`); got != "2" {
		t.Errorf("无 nosniff 的 text/plain 脚本未执行（window.__ran = %s）", got)
	}
}

// TestImageIgnoresMIME：图片不看 MIME（解码成功即采用）——即使响应带
// nosniff 且类型是 text/plain，只要字节是合法 PNG 就照画（浏览器行为）。
func TestImageIgnoresMIME(t *testing.T) {
	png := redPNG4x4(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/img-mime.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}</style>`+
			`</head><body><img id="pic" src="pic.png"></body></html>`)
	})
	mux.HandleFunc("/pic.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(png)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/img-mime.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	box := waitBoxImage(t, wv, "pic", 80)
	if box == nil {
		t.Fatalf("text/plain + nosniff 的 PNG 未被采用（图片不应看 MIME）")
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cx := int(box.AbsoluteX() + box.Width()/2)
	cy := int(box.AbsoluteY() + box.Height()/2)
	if r, g, b, a := pixelAt(pix, wv.Width(), cx, cy); !(r > 200 && g < 80 && a > 200) {
		t.Errorf("图片位置 (%d,%d) = rgba(%d,%d,%d,%d), want 红", cx, cy, r, g, b, a)
	}
}

// TestResourceCacheRespectsNoStore：`Cache-Control: no-store` 的响应不缓存
// （浏览器语义）——两次引用都要重新取。
func TestResourceCacheRespectsNoStore(t *testing.T) {
	log := &httpRequestLog{}
	mux := http.NewServeMux()
	mux.HandleFunc("/nostore.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><link rel="stylesheet" href="ns.css">`+
			`<link rel="stylesheet" href="ns.css"></head><body></body></html>`)
	})
	mux.HandleFunc("/ns.css", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, `body{margin:0}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/nostore.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	// no-store 的响应不写缓存：同页面的两次引用都要真的取（>=2）。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && countPath(log, "/ns.css") < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if n := countPath(log, "/ns.css"); n < 2 {
		t.Errorf("Cache-Control: no-store 的响应被缓存了（只取回 %d 次，请求序列 %v）", n, log.paths())
	}
	if n := countPath(log, "/ns.css"); n > 3 {
		t.Errorf("no-store 资源取回 %d 次（重复请求过多）：%v", n, strings.Join(log.paths(), " "))
	}
}
