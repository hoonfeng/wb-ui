package webkit

// location 导航（assign/replace/href/reload）与 window.history 的真实回归。
//
// 引擎此前：`location.assign/replace/reload` 是返回 undefined 的空桩
// （静默无效）、`location.href` 只有 getter（赋值被丢弃），且宿主的
// LoadHTML/LoadURL 完全不进历史栈（history.length 恒为 1、back() 永远无操作）。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func navigationFixture(t *testing.T) (*httptest.Server, *httpRequestLog) {
	t.Helper()
	log := &httpRequestLog{}
	mux := http.NewServeMux()
	serve := func(path, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			log.add(r.URL.Path)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, body)
		})
	}
	page := func(name string) string {
		return `<!DOCTYPE html><html><head><title>` + name + `</title></head>` +
			`<body><div id="who">` + name + `</div></body></html>`
	}
	serve("/a.html", page("A"))
	serve("/b.html", page("B"))
	// 页面脚本在装配期间就发起导航（浏览器里会在当前脚本跑完后换文档）：
	// 引擎必须排队，而不是在装配中途重入换文档。
	serve("/auto.html", `<!DOCTYPE html><html><head><title>AUTO</title></head>`+
		`<body><div id="who">AUTO</div><script>location.replace("b.html")</script></body></html>`)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, log
}

func evalNavStr(t *testing.T, wv *WebView, script string) string {
	t.Helper()
	v, err := wv.EvalJS(script)
	if err != nil {
		t.Fatalf("EvalJS(%q): %v", script, err)
	}
	return v.ToString()
}

// TestLocationAssignNavigates：`location.href = …` 与 `location.assign(…)`
// 真的换文档（相对引用按文档基准解析）。反向验证：把 location.href 的 setter
// 去掉（或让 assign 回到空桩）后，本测试立刻失败（文档仍是 A）。
func TestLocationAssignNavigates(t *testing.T) {
	srv, log := navigationFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/a.html"); err != nil {
		t.Fatalf("LoadURL(a.html): %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "A" {
		t.Fatalf("初始文档 = %q, want A", got)
	}

	// location.href 赋值（相对路径 /b.html 按文档基准解析）。
	if _, err := wv.EvalJS(`location.href = "/b.html"`); err != nil {
		t.Fatalf("location.href 赋值: %v", err)
	}
	if got, want := evalNavStr(t, wv, `document.URL`), srv.URL+"/b.html"; got != want {
		t.Errorf("href 赋值后 document.URL = %q, want %q", got, want)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "B" {
		t.Errorf("href 赋值后文档内容 = %q, want B（文档没有被替换）", got)
	}
	if !log.has("/b.html") {
		t.Errorf("服务器未收到 /b.html：%v", log.paths())
	}

	// location.assign 等价 href 赋值。
	if _, err := wv.EvalJS(`location.assign("/a.html")`); err != nil {
		t.Fatalf("location.assign: %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "A" {
		t.Errorf("assign 后文档内容 = %q, want A", got)
	}
}

// TestHistoryRecordsHostNavigation：宿主导航进历史栈——length 反映文档数，
// back() 跨文档回到上一个文档（且不新增条目）。
func TestHistoryRecordsHostNavigation(t *testing.T) {
	srv, log := navigationFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/a.html"); err != nil {
		t.Fatalf("LoadURL(a.html): %v", err)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "1" {
		t.Errorf("初始 history.length = %q, want 1", got)
	}
	if _, err := wv.EvalJS(`location.href = "/b.html"`); err != nil {
		t.Fatalf("location.href 赋值: %v", err)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "2" {
		t.Errorf("导航后 history.length = %q, want 2", got)
	}

	// back() → 跨文档遍历回 a.html（浏览器里这种遍历不派发 popstate，只换文档）。
	before := len(log.paths())
	if _, err := wv.EvalJS(`history.back()`); err != nil {
		t.Fatalf("history.back: %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "A" {
		t.Errorf("back() 后文档内容 = %q, want A（历史遍历没有换文档）", got)
	}
	if got, want := evalNavStr(t, wv, `document.URL`), srv.URL+"/a.html"; got != want {
		t.Errorf("back() 后 document.URL = %q, want %q", got, want)
	}
	if !log.has("/a.html") || len(log.paths()) < before+1 {
		t.Errorf("back() 未重新请求 a.html：%v", log.paths())
	}
	// 遍历不新增条目：length 仍是 2（曾是 3 就说明 back 也 push 了新条目）。
	if got := evalNavStr(t, wv, `history.length`); got != "2" {
		t.Errorf("back() 后 history.length = %q, want 2（历史遍历不应追加条目）", got)
	}

	// forward() 回到 b.html。
	if _, err := wv.EvalJS(`history.forward()`); err != nil {
		t.Fatalf("history.forward: %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "B" {
		t.Errorf("forward() 后文档内容 = %q, want B", got)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "2" {
		t.Errorf("forward() 后 history.length = %q, want 2", got)
	}
}

// TestLocationReplaceAndReload：replace 不新增条目（替换当前条目）、reload
// 重新装配当前文档。
func TestLocationReplaceAndReload(t *testing.T) {
	srv, log := navigationFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/a.html"); err != nil {
		t.Fatalf("LoadURL(a.html): %v", err)
	}
	if _, err := wv.EvalJS(`location.replace("/b.html")`); err != nil {
		t.Fatalf("location.replace: %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "B" {
		t.Errorf("replace 后文档内容 = %q, want B", got)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "1" {
		t.Errorf("replace 后 history.length = %q, want 1（replace 替换当前条目）", got)
	}

	// reload：重新请求当前 URL，条目数不变。
	n := 0
	for _, p := range log.paths() {
		if p == "/b.html" {
			n++
		}
	}
	if _, err := wv.EvalJS(`location.reload()`); err != nil {
		t.Fatalf("location.reload: %v", err)
	}
	n2 := 0
	for _, p := range log.paths() {
		if p == "/b.html" {
			n2++
		}
	}
	if n2 <= n {
		t.Errorf("reload 未重新请求 /b.html（请求序列 %v）", log.paths())
	}
	if got := evalNavStr(t, wv, `history.length`); got != "1" {
		t.Errorf("reload 后 history.length = %q, want 1", got)
	}
}

// TestScriptNavigationDuringAssemblyIsQueued：页面脚本在装配期间发起的导航
// 排队到装配结束后执行（不重入换文档），最终文档是跳转目标。
func TestScriptNavigationDuringAssemblyIsQueued(t *testing.T) {
	srv, _ := navigationFixture(t)
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadURL(srv.URL + "/auto.html"); err != nil {
		t.Fatalf("LoadURL(auto.html): %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "B" {
		t.Errorf("脚本跳转后文档内容 = %q, want B（装配期间的导航没有生效）", got)
	}
	// location.replace 的语义在排队执行后仍成立：只有一条历史条目。
	if got := evalNavStr(t, wv, `history.length`); got != "1" {
		t.Errorf("history.length = %q, want 1（replace 不应追加条目）", got)
	}
}

// TestToolkitModeRejectsLocationNavigation：UI 库模式下 location 导航被模式
// 门禁拒绝——文档不被替换（Go 侧构建的 UI 树与事件绑定不会悬空），宿主能通过
// 回调感知。
func TestToolkitModeRejectsLocationNavigation(t *testing.T) {
	srv, _ := navigationFixture(t)
	wv := modeWebView(t, ModeToolkit)
	var blocked []string
	wv.SetOnNavigationBlocked(func(url string) { blocked = append(blocked, url) })
	if err := wv.LoadURL(srv.URL + "/a.html"); err == nil {
		t.Fatalf("UI 库模式下 LoadURL 应当被拒绝")
	}
	if err := wv.LoadHTML(`<html><body><div id="who">TOOLKIT</div></body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if _, err := wv.EvalJS(`location.assign("/b.html")`); err != nil {
		t.Fatalf("location.assign: %v", err)
	}
	if got := evalNavStr(t, wv, `document.getElementById("who").textContent`); got != "TOOLKIT" {
		t.Errorf("UI 库模式下文档被替换了：%q", got)
	}
	if len(blocked) != 1 || !strings.HasSuffix(blocked[0], "/b.html") {
		t.Errorf("导航被拒回调 = %v, want 一条 /b.html", blocked)
	}
}
