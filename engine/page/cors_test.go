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
