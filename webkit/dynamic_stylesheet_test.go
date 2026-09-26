package webkit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 页面：内联脚本在运行时创建 <link rel=stylesheet> 并 appendChild —— 这是
// gou-ide 插件包 CSS 的装载方式（client.js：createElement('link') + rel/href
// + appendChild）。此前引擎只在装配期处理 <link>，运行时插入的样式表永不生效。
const dynStylePage = `<!DOCTYPE html><html><head><title>dynstyle</title></head><body>
<div id="box">x</div>
<script>
  var l = document.createElement('link');
  l.rel = 'stylesheet';
  l.href = '/style.css';
  l.onload = function () { window.__linkLoad = 'yes'; };
  document.head.appendChild(l);
  window.__rel = String(l.getAttribute('rel'));
  window.__href = String(l.getAttribute('href'));
</script>
</body></html>`

func TestDynamicStylesheetInsertedAtRuntime(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dynStylePage)
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "#box{width:123px}")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := NewWebView()
	defer wv.Destroy()
	if err := wv.LoadURL(srv.URL + "/index.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	t.Logf("link 属性: rel=%q href=%q",
		dynEval(t, wv, `String(window.__rel)`), dynEval(t, wv, `String(window.__href)`))
	for i := 0; i < 30; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	var w float64
	if doc, rv := wv.Document(), wv.RenderView(); doc != nil && rv != nil {
		if el := doc.GetElementById("box"); el != nil {
			if b := rv.FindRenderBoxForNode(el); b != nil {
				w = b.Width()
			}
		}
	}
	if w != 123 {
		t.Fatalf("动态样式表未生效：#box 宽度=%v（期望 123）", w)
	}
	if got := dynEval(t, wv, `String(window.__linkLoad)`); got != "yes" {
		t.Fatalf("link load 事件未按浏览器语义派发：%q", got)
	}
}
