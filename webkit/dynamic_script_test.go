package webkit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 页面：内联脚本在运行时创建 <script src> 并 appendChild —— 这是 gou-ide
// 区域包 client 半装载 bundle 的方式（见 /plugins-assets/ui-titlebar/client.js）。
const dynScriptPage = `<!DOCTYPE html><html><head><title>dyn</title></head><body>
<div id="app">x</div>
<script>
  window.__events = [];
  window.__srcAttr = '';
  var s = document.createElement('script');
  s.src = '/app.js';
  s.onload = function () { window.__events.push('load'); window.__loaded = 'yes'; };
  s.onerror = function () { window.__events.push('error'); };
  document.head.appendChild(s);
  window.__inserted = 'yes';
  try { window.__srcAttr = String(s.getAttribute('src')); } catch (e) { window.__srcAttr = 'ERR:' + e; }
  try { window.__connected = String(s.isConnected); } catch (e) { window.__connected = 'ERR:' + e; }
</script>
</body></html>`

func dynEval(t *testing.T, wv *WebView, script string) string {
	t.Helper()
	v, err := wv.EvalJS(script)
	if err != nil {
		t.Fatalf("EvalJS(%s): %v", script, err)
	}
	return v.ToString()
}

func TestDynamicScriptInsertedAtRuntime(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dynScriptPage)
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__app = 'ran';`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := NewWebView()
	defer wv.Destroy()
	if err := wv.LoadURL(srv.URL + "/index.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	t.Logf("插入状态: inserted=%s srcAttr=%q isConnected=%s",
		dynEval(t, wv, `String(window.__inserted)`),
		dynEval(t, wv, `String(window.__srcAttr)`),
		dynEval(t, wv, `String(window.__connected)`))
	t.Logf("队列长度(插入后): %d", len(wv.dynScripts))
	for i := 0; i < 30; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	t.Logf("队列长度(30 帧后): %d", len(wv.dynScripts))
	got := dynEval(t, wv, `String(window.__app)`)
	events := dynEval(t, wv, `JSON.stringify(window.__events)`)
	if got != "ran" {
		t.Fatalf("动态 <script src> 未执行：window.__app=%q events=%s", got, events)
	}
	if events != "[\"load\"]" {
		t.Fatalf("load 事件语义不符：events=%s（期望 [\"load\"]）", events)
	}
}
