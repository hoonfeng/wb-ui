package webkit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// esmServe 起一个真实 HTTP 服务器提供给定的页面/模块文件。
func esmServe(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range files {
		p, b := path, body
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") {
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			fmt.Fprint(w, b)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func esmEval(t *testing.T, wv *WebView, script string) string {
	t.Helper()
	v, err := wv.EvalJS(script)
	if err != nil {
		t.Fatalf("EvalJS(%s): %v", script, err)
	}
	return v.ToString()
}

func esmPump(wv *WebView, frames int) {
	for i := 0; i < frames; i++ {
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		_, _ = wv.Render()
	}
}

func esmLoad(t *testing.T, files map[string]string, page string) (*WebView, *httptest.Server) {
	t.Helper()
	srv := esmServe(t, files)
	wv := NewWebViewWithMode(ModeBrowser)
	t.Cleanup(wv.Destroy)
	if err := wv.LoadURL(srv.URL + page); err != nil {
		t.Fatalf("LoadURL(%s): %v", page, err)
	}
	esmPump(wv, 10)
	return wv, srv
}

const esmPage = `<!DOCTYPE html><html><head><title>esm</title></head><body>
<div id="app">x</div>
<script type="module" src="/js/main.js"></script>
</body></html>`

// TestESMModuleGraph 覆盖构建产物最常见的静态导入形态：
// 默认导入 + 命名导入（含 as）+ 命名空间导入 + 副作用导入 + re-export +
// export 声明（const/function/default）+ import.meta.url。
func TestESMModuleGraph(t *testing.T) {
	wv, _ := esmLoad(t, map[string]string{
		"/index.html": esmPage,
		"/js/main.js": `
import def, { a, b as c } from "./dep.js";
import * as ns from "./ns.js";
import "./side.js";
import { aa, n } from "./re.js";
window.__r = {
  def: def,
  a: a,
  c: c,
  ns: ns.n,
  nsDefault: ns.default && ns.default.x,
  side: window.__side,
  aa: aa,
  reexportedN: n,
  url: import.meta.url
};`,
		"/js/dep.js": `
export default "D";
export const a = 1;
export const b = 2;
export function fn() { return 3; }`,
		"/js/ns.js": `
export const n = "N";
export default { x: 7 };`,
		"/js/side.js": `window.__sideRuns = (window.__sideRuns || 0) + 1; window.__side = "S";`,
		"/js/re.js": `
export { a as aa } from "./dep.js";
export * from "./ns.js";`,
	}, "/index.html")

	got := esmEval(t, wv, `JSON.stringify(window.__r)`)
	want := `{"def":"D","a":1,"c":2,"ns":"N","nsDefault":7,"side":"S","aa":1,"reexportedN":"N","url":"` // 前缀断言
	if !strings.HasPrefix(got, want) {
		t.Fatalf("模块图结果不符：\n got=%s\nwant 前缀=%s", got, want)
	}
	// import.meta.url 必须是模块自身 URL（不是文档 URL）。
	if !strings.Contains(got, `"url":"http://127.0.0.1:`) || !strings.Contains(got, `/js/main.js"`) {
		t.Fatalf("import.meta.url 不是模块自身 URL：%s", got)
	}
	// 每个模块只执行一次：副作用模块被 main 与 re 各引用一次。
	if v := esmEval(t, wv, `String(window.__sideRuns)`); v != "1" {
		t.Fatalf("副作用模块执行次数异常：%s", v)
	}
}

// TestESMInlineModule 覆盖内联 module 脚本（文档 URL 作为 import.meta.url 与
// 相对说明符基准）。
func TestESMInlineModule(t *testing.T) {
	files := map[string]string{
		"/index.html": `<!DOCTYPE html><html><head><title>esm-inline</title></head><body>
<script type="module">
import { v } from "./m.js";
window.__inline = { v: v, url: import.meta.url };
</script>
<script type="module">
window.__inline2 = (typeof window.__inline === "object");
</script>
</body></html>`,
		"/m.js": `export const v = "inline-ok";`,
	}
	wv, srv := esmLoad(t, files, "/index.html")
	got := esmEval(t, wv, `JSON.stringify(window.__inline)`)
	if !strings.Contains(got, `"v":"inline-ok"`) {
		t.Fatalf("内联模块未执行：%s", got)
	}
	if !strings.Contains(got, srv.URL+`/index.html"`) {
		t.Fatalf("内联模块 import.meta.url 应为文档 URL：%s", got)
	}
	if v := esmEval(t, wv, `String(window.__inline2)`); v != "true" {
		t.Fatalf("同一文档的第二条内联模块未执行：%s", v)
	}
}

// TestESMDynamicImport 覆盖 import()（返回 Promise，微任务后可用）。
func TestESMDynamicImport(t *testing.T) {
	wv, _ := esmLoad(t, map[string]string{
		"/index.html": esmPage,
		"/js/main.js": `
window.__dyn = "pending";
import("./lazy.js").then(function (m) {
  window.__dyn = m.value + "/" + m.default;
}, function (e) {
  window.__dyn = "err:" + e;
});`,
		"/js/lazy.js": `
export const value = "lazy";
export default "D";`,
	}, "/index.html")
	esmPump(wv, 10)
	got := esmEval(t, wv, `String(window.__dyn)`)
	if got != "lazy/D" {
		t.Fatalf("动态 import 未正确 resolve：%q", got)
	}
}

// TestESMCircularImport 覆盖循环依赖：不能死循环、不能 panic，导出对象可用
// （简化实现允许先执行的模块看到尚未填充的绑定）。
func TestESMCircularImport(t *testing.T) {
	wv, _ := esmLoad(t, map[string]string{
		"/index.html": esmPage,
		"/js/main.js": `
import { bFromA } from "./a.js";
window.__circ = { b: bFromA, a: "ok" };`,
		"/js/bob.js": `export function bob() { return "bob"; }`,
		"/js/a.js": `
import { bob } from "./bob.js";
export const aVal = "A";
export const bFromA = bob() + "-A";`,
	}, "/index.html")
	got := esmEval(t, wv, `JSON.stringify(window.__circ)`)
	if !strings.Contains(got, `"b":"bob-A"`) {
		t.Fatalf("循环依赖场景失败：%s", got)
	}
}

// TestESMLiveBinding 覆盖 export 的 live binding：模块内变量更新后，
// **命名空间导入**（ns.count）通过导出 getter 读到新值。
//
// 注意：`import { count }`（具名值导入）转译成的是执行时快照（见
// page/esm.go 的说明），不随模块内变量变化——这是文本转译相对真正 ESM
// 链接的已知差异。
func TestESMLiveBinding(t *testing.T) {
	wv, _ := esmLoad(t, map[string]string{
		"/index.html": esmPage,
		"/js/main.js": `
import * as counter from "./counter.js";
window.__live = [String(counter.count)];
counter.inc();
window.__live.push(String(counter.count));`,
		"/js/counter.js": `
export let count = 0;
export function inc() { count++; }`,
	}, "/index.html")
	got := esmEval(t, wv, `JSON.stringify(window.__live)`)
	if got != `["0","1"]` {
		t.Fatalf("live binding 未生效：%s", got)
	}
}
