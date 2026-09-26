package page

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wb-ui/bridge"
)

// ─── 2026-09-27：XHR 事件投递 + bridge 路由拦截（宿主 PairCode 实测缺口）───
//
// 背景：wb-ui 的 XHR 此前**只** fire onreadystatechange，从不触发
// onload / onerror / ontimeout，也从不查宿主注册的 bridge 路由。于是前端最
// 标准的写法 `xhr.onload = function(){ resolve(...) }` 在 wb-ui 里永远停在
// pending（宿主现象：插件面板卡在「加载插件…」）。bridge 包文档承诺
// "front-end code can call via standard fetch() / XMLHttpRequest without
// modification" —— 下面五条用例把该承诺钉死。

// TestXHRBridgeRouteIntercept 验证 XHR 与 fetch 一样命中 bridge 路由（绕开真实
// HTTP）。路径故意用相对形式：桌面壳里没有本地 HTTP 服务，走真实网络必然失败。
func TestXHRBridgeRouteIntercept(t *testing.T) {
	bridge.RegisterHTTP("GET", "/api/xhr-bridge-probe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"where":"bridge","path":%q}`, r.URL.Path)
	})

	out := runXHRJS(t, `
		var xhr = new XMLHttpRequest();
		xhr.open("GET", "/api/xhr-bridge-probe");
		xhr.onload = function() { console.log("load:" + xhr.status + ":" + xhr.responseText); };
		xhr.onerror = function() { console.log("error:" + xhr.status); };
		xhr.send();
	`)

	got := strings.TrimSpace(out)
	if !strings.HasPrefix(got, "load:200:") {
		t.Fatalf("XHR 未命中 bridge 路由（期望 load:200:…），实际：%q", got)
	}
	if !strings.Contains(got, `"where":"bridge"`) {
		t.Errorf("bridge 响应体不符：%q", got)
	}
}

// TestXHRAsyncEventCallbacks 验证 onload / onloadend / addEventListener("load")
// 三条通路都会触发（浏览器语义）。
func TestXHRAsyncEventCallbacks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload-ok")
	}))
	defer ts.Close()

	out := runXHRJS(t, fmt.Sprintf(`
		var xhr = new XMLHttpRequest();
		xhr.open("GET", %q);
		xhr.onload = function() { console.log("onload:" + xhr.status + ":" + xhr.responseText); };
		xhr.onloadend = function() { console.log("onloadend:" + xhr.readyState); };
		xhr.addEventListener("load", function() { console.log("listener-load:" + xhr.readyState); });
		xhr.send();
	`, ts.URL+"/evt"))

	for _, want := range []string{"onload:200:payload-ok", "onloadend:4", "listener-load:4"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q\n实际输出：\n%s", want, out)
		}
	}
}

// TestXHRErrorEvent 验证网络失败触发 onerror 且 readyState=4 / status=0
// （否则调用方 Promise 永不 settle）。
func TestXHRErrorEvent(t *testing.T) {
	out := runXHRJS(t, `
		var xhr = new XMLHttpRequest();
		xhr.open("GET", "http://127.0.0.1:1/refused");
		xhr.onerror = function() { console.log("onerror:" + xhr.readyState + ":" + xhr.status); };
		xhr.onload = function() { console.log("onload-unexpected"); };
		xhr.send();
	`)
	if got := strings.TrimSpace(out); got != "onerror:4:0" {
		t.Fatalf("onerror 语义不符，期望 \"onerror:4:0\"，实际 %q", got)
	}
}

// TestXHRTimeoutEvent 验证 timeout 属性真正生效（此前是死属性：设了也被忽略），
// 超时触发 ontimeout 且**不**触发 onload。
func TestXHRTimeoutEvent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		fmt.Fprint(w, "too-late")
	}))
	defer ts.Close()

	start := time.Now()
	out := runXHRJS(t, fmt.Sprintf(`
		var xhr = new XMLHttpRequest();
		xhr.open("GET", %q);
		xhr.timeout = 40;
		xhr.ontimeout = function() { console.log("ontimeout:" + xhr.readyState); };
		xhr.onload = function() { console.log("onload-unexpected"); };
		xhr.send();
	`, ts.URL+"/slow"))
	elapsed := time.Since(start)

	if got := strings.TrimSpace(out); got != "ontimeout:4" {
		t.Fatalf("ontimeout 未触发，期望 \"ontimeout:4\"，实际 %q", got)
	}
	// timeout=40ms 必须真正截断请求（服务器要 400ms 才响应）。
	if elapsed > 300*time.Millisecond {
		t.Errorf("timeout 未生效：耗时 %v（应 ≈40ms，说明仍在等服务器响应）", elapsed)
	}
}

// TestXHRHostCallShape 是本次修复的**直接验收场景**：把宿主 PairCode 前端
// （plugins-src/ui-app/src/components/PluginPanel.vue 的 fetchPluginsJSON）的
// 调用形态原样搬过来 —— 裸 XHR + onload/onerror/ontimeout + Promise 包装 +
// status 判定 + JSON.parse。修复前该 Promise 永不 settle，宿主表现为插件面板
// 一直停在「加载插件…」；当时前端为绕开此缺陷改用了 fetch（该代偿现已按
// 「引擎的缺口应由引擎修」的原则撤销）。
func TestXHRHostCallShape(t *testing.T) {
	bridge.RegisterHTTP("GET", "/api/plugins", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"name":"ui-sidebar"},{"name":"tool-web"}]`)
	})

	out := runXHRJS(t, `
		function fetchPluginsJSON() {
			return new Promise(function(resolve, reject) {
				var x = new XMLHttpRequest()
				x.open('GET', '/api/plugins', true)
				x.timeout = 8000
				x.onload = function() {
					if (x.status >= 200 && x.status < 300) {
						try { resolve(JSON.parse(x.responseText)) } catch (e) { reject(e) }
					} else { reject(new Error('HTTP ' + x.status)) }
				}
				x.onerror = function() { reject(new Error('network error')) }
				x.ontimeout = function() { reject(new Error('timeout')) }
				x.send()
			})
		}
		fetchPluginsJSON().then(
			function(list) { console.log("resolved:" + list.length + ":" + list[0].name) },
			function(e) { console.log("rejected:" + e.message) }
		)
	`)

	if got := strings.TrimSpace(out); got != "resolved:2:ui-sidebar" {
		t.Fatalf("宿主调用形态未 resolve（期望 \"resolved:2:ui-sidebar\"），实际 %q", got)
	}
}
