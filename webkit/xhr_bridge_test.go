package webkit

import (
	"fmt"
	"net/http"
	"testing"

	"wb-ui/bridge"
)

// TestWebViewXHRBridgeIntegration 验证**装配级**行为：浏览器模式的 WebView 里，
// 前端用裸 XMLHttpRequest 调宿主注册的 bridge 路由，必须像 fetch 一样拿到数据
// （onload 触发、status/responseText 就位、JSON 可解析）。
//
// 这是宿主 PairCode 插件面板的真实调用形态：修复前 XHR 既不走 bridge 也不触发
// onload，面板永远停在「加载插件…」（前端当时改用 fetch 绕开，该代偿已按
// 「引擎的缺口应由引擎修」的原则撤销）。
func TestWebViewXHRBridgeIntegration(t *testing.T) {
	bridge.RegisterHTTP("GET", "/api/wv-xhr-plugins", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"name":"ui-sidebar"},{"name":"tool-web"}]`)
	})

	wv := modeWebView(t, ModeBrowser)
	modeMustLoad(t, wv, modeMinimalHTML)

	got := modeSettle(t, wv, `
		window.__probe = "pending"
		var x = new XMLHttpRequest()
		x.open("GET", "/api/wv-xhr-plugins", true)
		x.timeout = 8000
		x.onload = function() {
			try {
				var list = JSON.parse(x.responseText)
				window.__probe = "load:" + x.status + ":" + list.length + ":" + list[0].name
			} catch (e) { window.__probe = "parse-error:" + x.responseText }
		}
		x.onerror = function() { window.__probe = "error:" + x.status }
		x.ontimeout = function() { window.__probe = "timeout" }
		x.send()
	`)

	if want := "load:200:2:ui-sidebar"; got != want {
		t.Fatalf("WebView 内 XHR+bridge 结果 = %q，期望 %q", got, want)
	}
}
