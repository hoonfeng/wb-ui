package webkit

// `<base href>` / `document.baseURI` / `LoadHTMLWithBaseURL` 的真实 HTTP 回归。
//
// 引擎此前完全不认 `<base>`：所有相对引用一律按文档 URL 解析。
// `<base href="/assets/">` 是真实站点把静态资源挪到子目录的常规手段（也常用于
// SPA 的 history 路由），不认它 = 页面里的相对 CSS/JS/图片全部 404。
//
// fixture 故意在**文档同级**也放一份同名资源（内容不同），这样「基准用错」会
// 请求到那一条、而不是只表现为 404——测试能指认错误来源。

import (
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"
)

func baseURLFixture(t *testing.T) (*httptest.Server, *httpRequestLog) {
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

	// 页面：`<base href="/base/">` → 相对引用一律落在 /base/ 下。
	// 第二个 `<base>`（href 不同）必须被忽略：规范只认**第一个**带 href 的
	// base 元素（HTML §4.2.3）。
	serve("/page.html", "text/html; charset=utf-8",
		`<!DOCTYPE html><html><head>`+
			`<base href="/base/"><base href="/ignored/">`+
			`<link rel="stylesheet" href="a.css">`+
			`</head><body><img id="pic" src="p.png" style="width:24px;height:24px"></body></html>`)
	serve("/base/a.css", "text/css", `body{margin:0}`)
	mux.HandleFunc("/base/p.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(redPNG4x4(t))
	})
	// 陷阱：文档同级放同名资源（颜色不同），按文档 URL 解析（错误实现）会命中
	// 这两个，而不是 404。
	serve("/a.css", "text/css", `body{margin:99px}`)
	mux.HandleFunc("/p.png", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(solidPNG4x4(t, color.RGBA{B: 255, A: 255}))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, log
}

// TestBaseHrefIsDocumentBase：`<base href>` 决定所有相对引用的解析基准。
// 反向验证：把 refreshBaseHref 改为不缓存（或让 documentBaseURL 直接返回
// documentURL）后，本测试立刻失败——服务器会收到 /a.css 与 /p.png。
func TestBaseHrefIsDocumentBase(t *testing.T) {
	srv, log := baseURLFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/page.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}

	// 1) 相对样式表按 base 解析。
	if !log.has("/base/a.css") {
		t.Errorf("<link href=\"a.css\"> 未按 <base href> 解析：服务器收到 %v", log.paths())
	}
	if log.has("/a.css") {
		t.Errorf("相对引用被按**文档 URL** 解析了（请求到 /a.css）：%v", log.paths())
	}

	// 2) document.baseURI 反映 base（document.URL 仍是真实地址）。
	wantBase := srv.URL + "/base/"
	if val, err := wv.EvalJS("document.baseURI"); err != nil {
		t.Fatalf("EvalJS(baseURI): %v", err)
	} else if got := val.ToString(); got != wantBase {
		t.Errorf("document.baseURI = %q, want %q", got, wantBase)
	}
	if val, err := wv.EvalJS("document.URL"); err != nil {
		t.Fatalf("EvalJS(URL): %v", err)
	} else if got := val.ToString(); got != srv.URL+"/page.html" {
		t.Errorf("document.URL = %q, want %q（base 不应改写文档 URL）", got, srv.URL+"/page.html")
	}
	// node.baseURI 与 document.baseURI 一致（HTML 元素）。
	if val, err := wv.EvalJS(`document.getElementById("pic").baseURI`); err != nil {
		t.Fatalf("EvalJS(el.baseURI): %v", err)
	} else if got := val.ToString(); got != wantBase {
		t.Errorf("element.baseURI = %q, want %q", got, wantBase)
	}

	// 3) 图片同样按 base 解析并画出（像素级）。
	box := waitBoxImage(t, wv, "pic", 80)
	if box == nil {
		t.Fatalf("<img src=\"p.png\"> 未加载（服务器收到 %v）", log.paths())
	}
	if !log.has("/base/p.png") {
		t.Errorf("<img> 未按 <base href> 解析：%v", log.paths())
	}
	if log.has("/p.png") {
		t.Errorf("<img> 被按文档 URL 解析了：%v", log.paths())
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cx := int(box.AbsoluteX() + box.Width()/2)
	cy := int(box.AbsoluteY() + box.Height()/2)
	r, g, b, a := pixelAt(pix, wv.Width(), cx, cy)
	// /base/p.png 是红图；/p.png 是蓝图——像素颜色能区分取了哪一个基准。
	if !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("图片位置 (%d,%d) = rgba(%d,%d,%d,%d), want 红（/base/p.png）", cx, cy, r, g, b, a)
	}
}

// TestBaseHrefDynamicInsertTakesEffect：脚本插入 `<base>` 后 document.baseURI
// 必须立即变化（浏览器语义：base 元素的变化即时生效，不需要重新加载）。
func TestBaseHrefDynamicInsertTakesEffect(t *testing.T) {
	wv := modeWebView(t, ModeToolkit)
	if err := wv.LoadHTMLWithBaseURL(`<html><head></head><body>x</body></html>`, "app://ui/page.html"); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	if val, err := wv.EvalJS("document.baseURI"); err != nil {
		t.Fatalf("EvalJS: %v", err)
	} else if got := val.ToString(); got != "app://ui/page.html" {
		t.Fatalf("初始 document.baseURI = %q, want app://ui/page.html", got)
	}
	// 插入 <base href="assets/"> → baseURI 按当前文档 URL 解析为 app://ui/assets/
	if _, err := wv.EvalJS(`(function(){
		var b = document.createElement("base");
		b.setAttribute("href", "assets/");
		document.head.appendChild(b);
		return 1;
	})()`); err != nil {
		t.Fatalf("插入 <base>: %v", err)
	}
	if val, err := wv.EvalJS("document.baseURI"); err != nil {
		t.Fatalf("EvalJS: %v", err)
	} else if got := val.ToString(); got != "app://ui/assets/" {
		t.Errorf("插入 <base> 后 document.baseURI = %q, want app://ui/assets/", got)
	}
	// 移除后又回到文档 URL。
	if _, err := wv.EvalJS(`document.querySelector("base").remove()`); err != nil {
		t.Fatalf("移除 <base>: %v", err)
	}
	if val, err := wv.EvalJS("document.baseURI"); err != nil {
		t.Fatalf("EvalJS: %v", err)
	} else if got := val.ToString(); got != "app://ui/page.html" {
		t.Errorf("移除 <base> 后 document.baseURI = %q, want app://ui/page.html", got)
	}
}

// TestLoadHTMLWithBaseURLResolvesRelativeRefs：宿主自供内容 + 显式基准
// （UI 库模式的典型用法：模板里写相对引用，宿主用 ResourceResolver 提供）。
// 这也是「LoadHTML 没有基准」的解法：LoadHTMLWithBaseURL 不取内容、不联网，
// 只给文档一个 document.baseURI。
func TestLoadHTMLWithBaseURLResolvesRelativeRefs(t *testing.T) {
	wv := modeWebView(t, ModeToolkit)
	var seen []string
	wv.SetResourceResolver(func(ref string) (string, bool) {
		seen = append(seen, ref)
		if ref == "app://ui/styles/app.css" {
			return "#box{position:absolute;left:0;top:0;width:10px;height:10px;background:#0000ff}", true
		}
		return "", false
	})
	if err := wv.LoadHTMLWithBaseURL(
		`<html><head><link rel="stylesheet" href="styles/app.css"></head>`+
			`<body><div id="box"></div></body></html>`, "app://ui/index.html"); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	found := false
	for _, s := range seen {
		if s == "app://ui/styles/app.css" {
			found = true
		}
	}
	if !found {
		t.Errorf("相对样式表未按 baseURL 解析成绝对引用；resolver 收到 %v", seen)
	}
	// 样式真的生效（蓝块画出来）。
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	r, g, b, a := pixelAt(pix, wv.Width(), 5, 5)
	if !(b > 200 && r < 80 && g < 80 && a > 200) {
		t.Errorf("(5,5) = rgba(%d,%d,%d,%d), want 蓝（baseURL 解析出的样式未生效）", r, g, b, a)
	}
}
