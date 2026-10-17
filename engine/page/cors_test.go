package page

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wb-ui/engine/js/jsc"
)

// ── 单元：CORS 判定内核 ────────────────────────────────────────────

func TestCanonicalOrigin(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"http://Example.COM:80/a", "http://example.com", true},
		{"https://example.com:443/a", "https://example.com", true},
		{"http://example.com:8080/a", "http://example.com:8080", true},
		{"file:///C:/x.html", "", false},
		{"data:text/html,x", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := canonicalOrigin(c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("canonicalOrigin(%q) = (%q, %v)，期望 (%q, %v)", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestCORSRequestNeedsCheck(t *testing.T) {
	// 同源：不校验。
	if r := newCORSRequest("http://a.test/x.html", "http://a.test/api", "GET", http.Header{}, false, corsModeCORS); r.needsCheck() {
		t.Fatal("同源请求不应走 CORS 校验")
	}
	// 默认端口归一化后仍是同源。
	if r := newCORSRequest("http://a.test:80/x.html", "http://a.test/api", "GET", http.Header{}, false, corsModeCORS); r.needsCheck() {
		t.Fatal("默认端口应归一化为同源")
	}
	// 跨源：校验。
	if r := newCORSRequest("http://a.test/x.html", "http://b.test/api", "GET", http.Header{}, false, corsModeCORS); !r.needsCheck() {
		t.Fatal("跨源请求必须走 CORS 校验")
	}
	// 不透明源（file:/data:）：不校验（见 cors.go 已知差异）。
	if r := newCORSRequest("file:///C:/x.html", "http://b.test/api", "GET", http.Header{}, false, corsModeCORS); r.needsCheck() {
		t.Fatal("file 文档的跨源请求不校验（宿主本地上下文）")
	}
}

func TestCORSRequestIsSimple(t *testing.T) {
	mk := func(method string, hdr http.Header) *corsRequest {
		return newCORSRequest("http://a.test/x", "http://b.test/api", method, hdr, false, corsModeCORS)
	}
	if !mk("GET", http.Header{}).isSimple() {
		t.Error("GET 无头应为简单请求")
	}
	if !mk("POST", http.Header{"Content-Type": []string{"text/plain"}}).isSimple() {
		t.Error("POST + text/plain 应为简单请求")
	}
	if mk("PUT", http.Header{}).isSimple() {
		t.Error("PUT 不是简单请求")
	}
	if mk("POST", http.Header{"Content-Type": []string{"application/json"}}).isSimple() {
		t.Error("POST + application/json 不是简单请求")
	}
	if mk("GET", http.Header{"X-Token": []string{"t"}}).isSimple() {
		t.Error("自定义请求头不是简单请求")
	}
}

func TestCORSValidateResponseHeaders(t *testing.T) {
	cases := []struct {
		name        string
		acao        string
		acac        string
		credentials bool
		wantErr     bool
	}{
		{"wildcard", "*", "", false, false},
		{"exact", "http://a.test", "", false, false},
		{"mismatch", "http://evil.test", "", false, true},
		{"missing", "", "", false, true},
		{"credentials-wildcard", "*", "true", true, true},
		{"credentials-exact", "http://a.test", "true", true, false},
		// ★ credentials 模式下 ACAO 必须精确等于文档源：指向第三方源即使带
		//   ACAC:true 也必须拒绝（修复前这里会放行）。
		{"credentials-mismatch", "http://evil.test", "true", true, true},
		{"credentials-exact-case-insensitive", "HTTP://A.TEST", "true", true, false},
		{"credentials-no-acac", "http://a.test", "", true, true},
	}
	for _, c := range cases {
		r := newCORSRequest("http://a.test/x", "http://b.test/api", "GET", http.Header{}, c.credentials, corsModeCORS)
		h := http.Header{}
		if c.acao != "" {
			h.Set("Access-Control-Allow-Origin", c.acao)
		}
		if c.acac != "" {
			h.Set("Access-Control-Allow-Credentials", c.acac)
		}
		err := r.validateResponseHeaders(h)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v，期望 wantErr=%v", c.name, err, c.wantErr)
		}
	}
}

// TestCORSPreflightTokenMatching 守预检头的 token 精确匹配：
//
//   - ACAM/ACAH 都是逗号分隔的 token 列表，必须逐 token 比较；
//   - 子串匹配会把 `POSTX` 当作允许 `POST`、`content-type-evil` 当作允许
//     `content-type`——安全边界直接失效；
//   - ACAH 的 `*` 是通配。
func TestCORSPreflightTokenMatching(t *testing.T) {
	mkResp := func(extra map[string]string) *http.Response {
		h := http.Header{}
		h.Set("Access-Control-Allow-Origin", "http://a.test") // 文档源精确匹配
		for k, v := range extra {
			h.Set(k, v)
		}
		return &http.Response{Header: h}
	}
	cases := []struct {
		name       string
		method     string
		reqHeaders http.Header
		resp       map[string]string
		wantErr    bool
	}{
		{
			name:    "acam 子串不算允许",
			method:  "POST",
			resp:    map[string]string{"Access-Control-Allow-Methods": "POSTX"},
			wantErr: true,
		},
		{
			name:    "acam 逗号列表精确命中",
			method:  "POST",
			resp:    map[string]string{"Access-Control-Allow-Methods": "GET, POST"},
			wantErr: false,
		},
		{
			name:    "acam 大小写不敏感",
			method:  "POST",
			resp:    map[string]string{"Access-Control-Allow-Methods": "get, post"},
			wantErr: false,
		},
		{
			name:       "acah 子串不算允许",
			method:     "GET",
			reqHeaders: http.Header{"Content-Type": {"application/json"}},
			resp: map[string]string{
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "content-type-evil",
			},
			wantErr: true,
		},
		{
			name:       "acah 精确命中",
			method:     "GET",
			reqHeaders: http.Header{"Content-Type": {"application/json"}},
			resp: map[string]string{
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "content-type",
			},
			wantErr: false,
		},
		{
			name:       "acah 通配 *",
			method:     "GET",
			reqHeaders: http.Header{"Content-Type": {"application/json"}},
			resp: map[string]string{
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "*",
			},
			wantErr: false,
		},
	}
	for _, c := range cases {
		r := newCORSRequest("http://a.test/x", "http://b.test/api", c.method, c.reqHeaders, false, corsModeCORS)
		err := r.validatePreflightResponse(mkResp(c.resp))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v，期望 wantErr=%v", c.name, err, c.wantErr)
		}
	}
}

// ── 端到端：JS fetch / XHR 跨源行为 ───────────────────────────────

// runFetchJSWithURLs 在指定**文档 URL** 与**解析基准**下运行脚本。
//
// 两者必须分开：CORS 的源取文档 URL（location.href），相对引用解析取
// document.baseURI（含 `<base href>`）。只设一个会让另一侧静默退化。
func runFetchJSWithURLs(t *testing.T, docURL, base, script string) string {
	t.Helper()
	in := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	in.SetupGlobal(log)
	RegisterFetch(in)
	SetDocumentURLProvider(in, func() string { return docURL })
	SetDocumentBaseProvider(in, func() string { return base })
	t.Cleanup(func() {
		ClearDocumentBaseProvider(in)
		ClearDocumentURLProvider(in)
	})
	if _, err := in.Run(script); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	return log.String()
}

// runFetchJSWithBase 是「文档 URL == baseURI」的常用情形包装。
func runFetchJSWithBase(t *testing.T, base, script string) string {
	return runFetchJSWithURLs(t, base, base, script)
}

func runXHRJSWithURLs(t *testing.T, docURL, base, script string) string {
	t.Helper()
	in := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	in.SetupGlobal(log)
	RegisterXMLHttpRequest(in)
	SetDocumentURLProvider(in, func() string { return docURL })
	SetDocumentBaseProvider(in, func() string { return base })
	t.Cleanup(func() {
		ClearDocumentBaseProvider(in)
		ClearDocumentURLProvider(in)
	})
	if _, err := in.Run(script); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	return log.String()
}

func runXHRJSWithBase(t *testing.T, base, script string) string {
	return runXHRJSWithURLs(t, base, base, script)
}

// TestCORSOriginUsesDocumentURLNotBase 守**源与解析基准分离**：
//
// 文档 URL = http://a.test/x.html、`<base href>` 指向另一个源（evil）时，
// 跨源请求的 Origin 必须是**文档源**，服务器返回 ACAO=evil 必须 reject——
// 否则页面只要写一行 `<base href="http://evil.test/">` 就能绕过 CORS。
func TestCORSOriginUsesDocumentURLNotBase(t *testing.T) {
	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer doc.Close()
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer evil.Close()
	docOrigin := "http://127.0.0.1:" + portOf(t, doc.URL)
	evilOrigin := "http://127.0.0.1:" + portOf(t, evil.URL)

	var sawOrigin string
	// 服务器只认 evil（即 `<base href>` 指向的源）——引擎若用 baseURI 算源就会误判为匹配。
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawOrigin = r.Header.Get("Origin")
		w.Header().Set("Access-Control-Allow-Origin", evilOrigin)
		fmt.Fprint(w, "leaked-body")
	}))
	defer api.Close()

	docURL := doc.URL + "/x.html"
	base := evil.URL + "/" // 模拟 <base href="http://evil.test/">
	out := runFetchJSWithURLs(t, docURL, base, fmt.Sprintf(`
		fetch("%s/data").then(function (r) {
			console.log("unexpected:" + r.status);
			return r.text();
		}).then(function (t) {
			console.log("leaked:" + t);
		}, function (e) {
			console.log("rejected:" + e);
		});
	`, api.URL))
	if sawOrigin != docOrigin {
		t.Fatalf("Origin 必须取文档 URL（%s），不能取 baseURI（%s），实际 %q", docOrigin, evilOrigin, sawOrigin)
	}
	if !strings.Contains(out, "rejected") || strings.Contains(out, "leaked-body") {
		t.Fatalf("ACAO=%s（baseURI 源）与文档源不匹配，必须 reject，输出：%q", evilOrigin, out)
	}

	// 对照组：ACAO=文档源 → 成功（证明不是一律拒绝）。
	apiOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", docOrigin)
		fmt.Fprint(w, "doc-origin-body")
	}))
	defer apiOK.Close()
	out = runFetchJSWithURLs(t, docURL, base, fmt.Sprintf(`
		fetch("%s/data").then(function (r) {
			console.log("status=" + r.status);
			return r.text();
		}).then(function (t) {
			console.log("body=" + t);
		}, function (e) {
			console.log("rejected:" + e);
		});
	`, apiOK.URL))
	if !strings.Contains(out, "doc-origin-body") {
		t.Fatalf("ACAO=文档源时应成功，输出：%q", out)
	}
}

func TestFetchCrossOriginCORS(t *testing.T) {
	var sawOrigin, sawPreflight string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawOrigin = r.Header.Get("Origin")
		if r.Method == http.MethodOptions {
			sawPreflight = r.Header.Get("Access-Control-Request-Method") + "|" +
				r.Header.Get("Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/allow" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			fmt.Fprint(w, "allow-body")
			return
		}
		fmt.Fprint(w, "deny-body")
	}))
	defer api.Close()

	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer doc.Close()
	base := doc.URL + "/index.html"

	// 允许：ACAO:* → 成功读到内容，且请求带 Origin。
	out := runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/allow").then(function (r) {
			console.log("status=" + r.status + " type=" + r.type);
			return r.text();
		}).then(function (t) {
			console.log("body=" + t);
		}, function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "status=200") || !strings.Contains(out, "allow-body") {
		t.Fatalf("ACAO:* 跨源请求应成功，输出：%q", out)
	}
	if sawOrigin != "http://127.0.0.1:"+portOf(t, doc.URL) {
		t.Errorf("跨源请求应带 Origin=%s，实际 %q", base, sawOrigin)
	}

	// 拒绝：无 ACAO → fetch reject，内容不可读。
	out = runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/deny").then(function (r) {
			console.log("unexpected:" + r.text());
		}, function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "rejected") || strings.Contains(out, "deny-body") {
		t.Fatalf("无 ACAO 的跨源响应必须 reject，输出：%q", out)
	}
	if !strings.Contains(out, "Access-Control-Allow-Origin") {
		t.Errorf("错误信息应指出缺少 ACAO，输出：%q", out)
	}

	// 非简单请求：预检通过 → 成功；预检头正确。
	out = runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/allow", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" })
			.then(function (r) { console.log("status=" + r.status + " body=" + r.text()); },
			      function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "status=200") {
		t.Fatalf("预检通过的跨源 POST 应成功，输出：%q", out)
	}
	if !strings.Contains(sawPreflight, "POST|content-type") {
		t.Errorf("预检应带 Access-Control-Request-Method/Headers，实际 %q", sawPreflight)
	}

	// credentials: include + ACAO:* → 拒绝（浏览器语义）。
	out = runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/allow", { credentials: "include" }).then(function (r) {
			console.log("unexpected:" + r.status);
		}, function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "rejected") {
		t.Fatalf("credentials:include 下 ACAO:* 必须 reject，输出：%q", out)
	}

	// mode: no-cors → opaque（status 0、body 空、type opaque）。
	out = runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/allow", { mode: "no-cors" }).then(function (r) {
			console.log("status=" + r.status + " type=" + r.type);
		}, function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "status=0") || !strings.Contains(out, "type=opaque") {
		t.Fatalf("no-cors 应返回 opaque 响应，输出：%q", out)
	}

	// mode: same-origin → 跨源直接失败。
	out = runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/allow", { mode: "same-origin" }).then(function (r) {
			console.log("unexpected:" + r.status);
		}, function (e) { console.log("rejected:" + e); });
	`, api.URL))
	if !strings.Contains(out, "rejected") {
		t.Fatalf("mode=same-origin 的跨源请求必须 reject，输出：%q", out)
	}

	// 同源（文档与请求同源）：无 ACAO 也应成功。
	sameDoc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "same-origin-body")
	}))
	defer sameDoc.Close()
	out = runFetchJSWithBase(t, sameDoc.URL+"/index.html", fmt.Sprintf(`
		fetch("%s/data").then(function (r) {
			console.log("status=" + r.status);
			return r.text();
		}).then(function (t) {
			console.log("body=" + t);
		}, function (e) { console.log("rejected:" + e); });
	`, sameDoc.URL))
	if !strings.Contains(out, "same-origin-body") || strings.Contains(out, "rejected") {
		t.Fatalf("同源请求不应受 CORS 限制，输出：%q", out)
	}
}

func TestXHRCrossOriginCORS(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/allow" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			fmt.Fprint(w, "xhr-allow")
			return
		}
		fmt.Fprint(w, "xhr-deny")
	}))
	defer api.Close()
	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer doc.Close()
	base := doc.URL + "/index.html"

	out := runXHRJSWithBase(t, base, fmt.Sprintf(`
		var x = new XMLHttpRequest();
		x.open("GET", "%s/allow");
		x.onload = function () { console.log("load:" + x.status + ":" + x.responseText); };
		x.onerror = function () { console.log("error:" + x.statusText); };
		x.send();
	`, api.URL))
	if !strings.Contains(out, "load:200:xhr-allow") {
		t.Fatalf("XHR 跨源（ACAO:*）应成功，输出：%q", out)
	}

	out = runXHRJSWithBase(t, base, fmt.Sprintf(`
		var x = new XMLHttpRequest();
		x.open("GET", "%s/deny");
		x.onload = function () { console.log("unexpected:" + x.responseText); };
		x.onerror = function () { console.log("error:blocked"); };
		x.send();
	`, api.URL))
	if !strings.Contains(out, "error:blocked") || strings.Contains(out, "xhr-deny") {
		t.Fatalf("XHR 跨源（无 ACAO）必须 error，输出：%q", out)
	}
}

// portOf 从 httptest URL 里取端口（用于构造期望的 Origin）。
func portOf(t *testing.T, raw string) string {
	t.Helper()
	i := strings.LastIndexByte(raw, ':')
	if i < 0 {
		t.Fatalf("no port in %q", raw)
	}
	return raw[i+1:]
}

// TestCORSPreflightWildcardRequiresNoCredentials 守预检 `*` 通配的凭证约束：
//
//   - Fetch 标准：Access-Control-Allow-Headers / Allow-Methods 的 `*` 只在
//     **非凭证**请求（credentials mode 不是 include）下作通配；
//   - credentials 模式下 `*` 是字面 token，不匹配任何方法/头名（不能借通配
//     放大带凭证请求的权限）；
//   - 非凭证请求仍正常享受 `*` 通配（不能因修复凭证侧而过度拒绝）。
func TestCORSPreflightWildcardRequiresNoCredentials(t *testing.T) {
	mkResp := func(credentials bool) *http.Response {
		h := http.Header{}
		h.Set("Access-Control-Allow-Origin", "http://a.test") // 与文档源精确一致
		if credentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		h.Set("Access-Control-Allow-Methods", "*")
		h.Set("Access-Control-Allow-Headers", "*")
		return &http.Response{Header: h}
	}
	reqHeaders := http.Header{"X-Token": {"t"}} // 非 safelisted 头，必须走 ACAH 判定

	// a) ACAH:`*` + 非 safelisted 头：非凭证 → 通过；凭证 → reject。
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "GET", reqHeaders, false, corsModeCORS).
		validatePreflightResponse(mkResp(false)); err != nil {
		t.Fatalf("非凭证预检 ACAH:* 应放行任意非 safelisted 头，err=%v", err)
	}
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "GET", reqHeaders, true, corsModeCORS).
		validatePreflightResponse(mkResp(true)); err == nil {
		t.Fatal("credentials 模式下 ACAH:* 不能放行非 safelisted 头")
	}
	// 对照：凭证模式明确列出头名 → 通过（证明不是一律拒绝）。
	h := mkResp(true)
	h.Header.Set("Access-Control-Allow-Methods", "GET") // 凭证下 ACAM:* 无效，须显式列出
	h.Header.Set("Access-Control-Allow-Headers", "x-token")
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "GET", reqHeaders, true, corsModeCORS).
		validatePreflightResponse(h); err != nil {
		t.Fatalf("credentials 模式下明确列出的头应通过，err=%v", err)
	}

	// b) ACAM:`*` + POST：非凭证 → 通过；凭证 → reject。
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "POST", http.Header{}, false, corsModeCORS).
		validatePreflightResponse(mkResp(false)); err != nil {
		t.Fatalf("非凭证预检 ACAM:* 应放行任意方法，err=%v", err)
	}
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "POST", http.Header{}, true, corsModeCORS).
		validatePreflightResponse(mkResp(true)); err == nil {
		t.Fatal("credentials 模式下 ACAM:* 不能放行 POST")
	}
	// 对照：凭证模式明确列出方法 → 通过。
	h = mkResp(true)
	h.Header.Set("Access-Control-Allow-Methods", "GET, POST")
	h.Header.Set("Access-Control-Allow-Headers", "*") // 本用例无自定义头，ACAH 不参与判定
	if err := newCORSRequest("http://a.test/x", "http://b.test/api", "POST", http.Header{}, true, corsModeCORS).
		validatePreflightResponse(h); err != nil {
		t.Fatalf("credentials 模式下明确列出的方法应通过，err=%v", err)
	}
}

// TestCORSResponseHeaderExposure 守跨源响应头可读性（Access-Control-Expose-Headers）：
//
//   - 跨源响应默认只暴露 CORS-safelisted 响应头（content-type 等）；
//   - 自定义头（X-Internal）只有 ACEH 列出才可读；
//   - Set-Cookie 恒不可见（即使 ACEH 显式列出）；
//   - fetch 与 XHR 必须走同一份实现（cors.go filterResponseHeaders）。
func TestCORSResponseHeaderExposure(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Internal", "secret")
		w.Header().Set("Set-Cookie", "sid=abc; Path=/")
		switch r.URL.Path {
		case "/exposed":
			w.Header().Set("Access-Control-Expose-Headers", "X-Internal")
		case "/exposed-cookie":
			// Set-Cookie 是 forbidden response header：写进 ACEH 也不可读。
			w.Header().Set("Access-Control-Expose-Headers", "X-Internal, Set-Cookie")
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer api.Close()
	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer doc.Close()
	base := doc.URL + "/index.html"

	fetchScript := func(path string) string {
		return fmt.Sprintf(`
			fetch("%s%s").then(function (r) {
				var xi = r.headers.get("x-internal");
				var sc = r.headers.get("set-cookie");
				console.log("ct=" + r.headers.get("content-type"));
				console.log("xi=" + (xi || "<null>"));
				console.log("sc=" + (sc || "<null>"));
			}, function (e) { console.log("rejected:" + e); });
		`, api.URL, path)
	}
	// 无 ACEH：自定义头与 Set-Cookie 都不可读，content-type 仍可见。
	out := runFetchJSWithBase(t, base, fetchScript("/plain"))
	if !strings.Contains(out, "ct=application/json") {
		t.Fatalf("跨源响应 content-type 应可读，输出：%q", out)
	}
	if !strings.Contains(out, "xi=<null>") || !strings.Contains(out, "sc=<null>") {
		t.Fatalf("无 ACEH 时 x-internal / set-cookie 必须不可读，输出：%q", out)
	}
	// ACEH: X-Internal → x-internal 可读，set-cookie 仍不可读。
	out = runFetchJSWithBase(t, base, fetchScript("/exposed"))
	if !strings.Contains(out, "xi=secret") {
		t.Fatalf("ACEH 列出 X-Internal 后应可读，输出：%q", out)
	}
	if !strings.Contains(out, "sc=<null>") {
		t.Fatalf("Set-Cookie 必须恒不可读，输出：%q", out)
	}
	// ACEH 显式列出 Set-Cookie → 仍不可读。
	out = runFetchJSWithBase(t, base, fetchScript("/exposed-cookie"))
	if !strings.Contains(out, "sc=<null>") {
		t.Fatalf("Set-Cookie 即使写进 ACEH 也不可读，输出：%q", out)
	}

	xhrScript := func(path string) string {
		return fmt.Sprintf(`
			var x = new XMLHttpRequest();
			x.open("GET", "%s%s");
			x.onload = function () {
				var xi = x.getResponseHeader("X-Internal");
				var sc = x.getResponseHeader("Set-Cookie");
				// 序列化已保证小写，这里直接用小写 needle 匹配即可；
				// 若引擎返回大写头名，下面的 all-xi 断言会失败（不再用 toLowerCase 掩盖差异）。
				var all = x.getAllResponseHeaders();
				console.log("ct=" + x.getResponseHeader("Content-Type"));
				console.log("xi=" + (xi || "<null>"));
				console.log("sc=" + (sc || "<null>"));
				console.log("all-xi=" + (all.indexOf("x-internal") >= 0));
				console.log("all-sc=" + (all.indexOf("set-cookie") >= 0));
			};
			x.onerror = function () { console.log("error:" + x.status); };
			x.send();
		`, api.URL, path)
	}
	// XHR 无 ACEH：getResponseHeader / getAllResponseHeaders 都看不到自定义头。
	out = runXHRJSWithBase(t, base, xhrScript("/plain"))
	if !strings.Contains(out, "ct=application/json") {
		t.Fatalf("XHR 跨源 content-type 应可读，输出：%q", out)
	}
	if !strings.Contains(out, "xi=<null>") || !strings.Contains(out, "sc=<null>") ||
		!strings.Contains(out, "all-xi=false") || !strings.Contains(out, "all-sc=false") {
		t.Fatalf("XHR 无 ACEH 时自定义头 / Set-Cookie 必须不可读，输出：%q", out)
	}
	// XHR 有 ACEH：getResponseHeader 与 getAllResponseHeaders 都能看到 X-Internal。
	out = runXHRJSWithBase(t, base, xhrScript("/exposed"))
	if !strings.Contains(out, "xi=secret") || !strings.Contains(out, "all-xi=true") {
		t.Fatalf("XHR 在 ACEH 列出后应可读 X-Internal，输出：%q", out)
	}
	if !strings.Contains(out, "sc=<null>") || !strings.Contains(out, "all-sc=false") {
		t.Fatalf("XHR 的 Set-Cookie 必须恒不可读，输出：%q", out)
	}
}

// TestSameOriginResponseHeadersNotFiltered 同时覆盖两件事：
//
//  1. 回归保护：同源请求不走跨源可读性白名单，自定义头（X-Internal）照常可读；
//  2. 收紧保护：Set-Cookie / Set-Cookie2 是 Fetch 的 forbidden response
//     header，同源同样不可读（与浏览器一致），getAllResponseHeaders 里也
//     不出现；
//  3. getAllResponseHeaders 的浏览器序列化：头名小写、按名字典序、每行
//     `name: value`、整体以 `\r\n` 结尾。
func TestSameOriginResponseHeadersNotFiltered(t *testing.T) {
	same := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Internal", "secret")
		w.Header().Set("X-Alpha", "one")
		w.Header().Set("Set-Cookie", "sid=abc; Path=/")
		w.Header().Set("Set-Cookie2", "legacy=1")
		fmt.Fprint(w, "ok")
	}))
	defer same.Close()
	base := same.URL + "/index.html"

	out := runFetchJSWithBase(t, base, fmt.Sprintf(`
		fetch("%s/data").then(function (r) {
			var xi = r.headers.get("x-internal");
			var sc = r.headers.get("set-cookie");
			var sc2 = r.headers.get("set-cookie2");
			console.log("xi=" + (xi || "<null>"));
			console.log("sc=" + (sc || "<null>"));
			console.log("sc2=" + (sc2 || "<null>"));
		}, function (e) { console.log("rejected:" + e); });
	`, same.URL))
	if !strings.Contains(out, "xi=secret") {
		t.Fatalf("同源 fetch 自定义响应头应可读，输出：%q", out)
	}
	if !strings.Contains(out, "sc=<null>") || !strings.Contains(out, "sc2=<null>") {
		t.Fatalf("同源 fetch 的 Set-Cookie / Set-Cookie2 必须不可读，输出：%q", out)
	}

	out = runXHRJSWithBase(t, base, fmt.Sprintf(`
		function hasCRLFEnd(s) {
			return s.length >= 2 && s.charCodeAt(s.length - 2) === 13 && s.charCodeAt(s.length - 1) === 10;
		}
		function linesOf(s) {
			var out = [];
			var cur = "";
			for (var i = 0; i < s.length; i++) {
				if (s.charCodeAt(i) === 13 && s.charCodeAt(i + 1) === 10) { out.push(cur); cur = ""; i++; }
				else { cur += s.charAt(i); }
			}
			if (cur !== "") { out.push(cur); }
			return out;
		}
		var x = new XMLHttpRequest();
		x.open("GET", "%s/data");
		x.onload = function () {
			var xi = x.getResponseHeader("X-Internal");
			var sc = x.getResponseHeader("Set-Cookie");
			var sc2 = x.getResponseHeader("Set-Cookie2");
			var all = x.getAllResponseHeaders();
			var ls = linesOf(all);
			var okForm = true, okLower = true, okSorted = true, prev = "";
			for (var i = 0; i < ls.length; i++) {
				var idx = ls[i].indexOf(": ");
				if (idx <= 0 || ls[i].slice(idx, idx + 2) !== ": ") { okForm = false; continue; }
				var nm = ls[i].slice(0, idx);
				if (nm !== nm.toLowerCase()) { okLower = false; }
				if (prev !== "" && !(prev < nm)) { okSorted = false; }
				prev = nm;
			}
			console.log("xi=" + (xi || "<null>"));
			console.log("sc=" + (sc || "<null>"));
			console.log("sc2=" + (sc2 || "<null>"));
			console.log("all-sc=" + (all.indexOf("set-cookie") >= 0));
			console.log("lines=" + ls.length);
			console.log("ends-crlf=" + hasCRLFEnd(all));
			console.log("lower=" + okLower);
			console.log("form=" + okForm);
			console.log("sorted=" + okSorted);
			console.log("names=" + ls.length);
		};
		x.onerror = function () { console.log("error:" + x.status); };
		x.send();
	`, same.URL))
	for _, want := range []string{
		"xi=secret",
		"sc=<null>",
		"sc2=<null>",
		"all-sc=false",
		"ends-crlf=true",
		"lower=true",
		"form=true",
		"sorted=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("同源 XHR 响应头断言 %q 未满足，输出：%q", want, out)
		}
	}
	// 至少要有 content-type / x-internal 这类可见头，避免空串误判格式全绿。
	if strings.Contains(out, "lines=0") {
		t.Fatalf("同源 getAllResponseHeaders 不应为空，输出：%q", out)
	}
}
