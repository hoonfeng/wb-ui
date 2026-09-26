package webkit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 插件侧（gou-ide api.js）全靠 fetch(...).then(r => r.json()) 拿数据。
// 此前根因表记录「fetch().json() 把响应体当 JS 代码 Run 并字符串化」——本用例
// 钉死其正确行为：json() 必须返回**真正的 JS 对象**（可读字段、可读嵌套、类型正确）。
const fetchJSONPage = `<!DOCTYPE html><html><head><title>fetchjson</title></head><body>
<script>
  window.__json = 'pending';
  window.__type = '';
  fetch('/api/data').then(function (r) { return r.json(); }).then(function (j) {
    window.__type = typeof j + '/' + (j && j.nested ? typeof j.nested : 'noNested');
    window.__json = j.name + '/' + j.count + '/' + j.nested.ok + '/keys=' + Object.keys(j).length;
  }).catch(function (e) { window.__json = 'ERR:' + e; });
</script>
</body></html>`

func TestFetchJSONResponseIsParsed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, fetchJSONPage)
	})
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"gou","count":3,"nested":{"ok":true}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := NewWebView()
	defer wv.Destroy()
	if err := wv.LoadURL(srv.URL + "/index.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	var got, typ string
	for i := 0; i < 40; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
		got = dynEval(t, wv, `String(window.__json)`)
		typ = dynEval(t, wv, `String(window.__type)`)
		if got != "pending" {
			break
		}
	}
	t.Logf("json 解析结果 = %q, 类型 = %q", got, typ)
	if got != "gou/3/true/keys=3" {
		t.Fatalf("fetch().json() 未正确解析：%q（期望 gou/3/true/keys=3）", got)
	}
	if typ != "object/object" {
		t.Fatalf("json() 返回类型错误：%q（期望 object/object —— 必须是真 JS 对象而非字符串）", typ)
	}
}
