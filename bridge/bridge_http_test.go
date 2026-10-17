package bridge

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"wb-ui/engine/js/jsc"
)

// TestRegisterHTTP_DirectCall verifies that a handler registered via
// RegisterHTTP can be invoked through the native bridge path (the same
// route the page.RegisterFetch interceptor uses): args = [url, options],
// handler returns {"status": N, "body": "..."}.
func TestRegisterHTTP_DirectCall(t *testing.T) {
	// Reset global routes (package-level state — tests run serially).
	old := globalRoutes
	globalRoutes = nil
	defer func() { globalRoutes = old }()

	RegisterHTTP("GET", "/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"name": "alice"})
	})
	RegisterHTTP("POST", "/api/users", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]string{"got": body["name"]})
	})

	in := jsc.NewInterpreter()
	_ = in

	// GET → 201 + JSON body
	urlVal := jsc.StringValue("http://localhost:9090/api/users?x=1")
	opts := jsc.NewObject(in.ObjectPrototype())
	opts.Set("method", jsc.StringValue("GET"))
	args := []jsc.JSValue{urlVal, jsc.ObjectValue(opts)}
	res, err := MatchMethod("GET", "http://localhost:9090/api/users?x=1").Handler(args)
	if err != nil {
		t.Fatalf("GET handler error: %v", err)
	}
	o := res.AsObject()
	if st, _ := o.GetByKey("status"); st.ToNumber() != 201 {
		t.Errorf("GET status = %v, want 201", st.ToNumber())
	}
	body := ""
	if b, _ := o.GetByKey("body"); b.IsString() {
		body = b.ToString()
	}
	if body != `{"name":"alice"}`+"\n" && body != `{"name":"alice"}` {
		t.Errorf("GET body = %q", body)
	}

	// POST → method-aware routing
	opts2 := jsc.NewObject(in.ObjectPrototype())
	opts2.Set("method", jsc.StringValue("POST"))
	opts2.Set("body", jsc.StringValue(`{"name":"bob"}`))
	args2 := []jsc.JSValue{jsc.StringValue("/api/users"), jsc.ObjectValue(opts2)}
	res2, err := MatchMethod("POST", "/api/users").Handler(args2)
	if err != nil {
		t.Fatalf("POST handler error: %v", err)
	}
	o2 := res2.AsObject()
	if st, _ := o2.GetByKey("status"); st.ToNumber() != 200 {
		t.Errorf("POST status = %v, want 200", st.ToNumber())
	}
	b2 := ""
	if b, _ := o2.GetByKey("body"); b.IsString() {
		b2 = b.ToString()
	}
	if b2 != `{"got":"bob"}`+"\n" && b2 != `{"got":"bob"}` {
		t.Errorf("POST body = %q", b2)
	}

	// Query preservation: handler reads r.URL.Query()
	RegisterHTTP("GET", "/api/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"q": r.URL.Query().Get("q")})
	})
	args3 := []jsc.JSValue{jsc.StringValue("/api/search?q=hello"), jsc.Null()}
	res3, err := MatchMethod("GET", "/api/search?q=hello").Handler(args3)
	if err != nil {
		t.Fatalf("search handler error: %v", err)
	}
	o3 := res3.AsObject()
	b3 := ""
	if b, _ := o3.GetByKey("body"); b.IsString() {
		b3 = b.ToString()
	}
	if b3 != `{"q":"hello"}`+"\n" && b3 != `{"q":"hello"}` {
		t.Errorf("search body = %q (query lost)", b3)
	}

	// Prefix pattern with trailing slash
	RegisterHTTP("GET", "/api/conversations/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": r.URL.Path})
	})
	args4 := []jsc.JSValue{jsc.StringValue("/api/conversations/conv_1/messages"), jsc.Null()}
	res4, err := MatchMethod("GET", "/api/conversations/conv_1/messages").Handler(args4)
	if err != nil {
		t.Fatalf("conv handler error: %v", err)
	}
	o4 := res4.AsObject()
	b4 := ""
	if b, _ := o4.GetByKey("body"); b.IsString() {
		b4 = b.ToString()
	}
	if b4 != `{"id":"/api/conversations/conv_1/messages"}`+"\n" && b4 != `{"id":"/api/conversations/conv_1/messages"}` {
		t.Errorf("conv body = %q (prefix match failed)", b4)
	}
}

// TestRegisterHTTP_ResponseHeaders 验证 dispatchHTTP 把 handler 写进
// ResponseWriter 的响应头**原样回传**（2026-09-28 修复的缺口：此前只回传
// status/body，virtualWriter.hdr 捕获的头被丢弃 → 页面侧只能凭空造
// Content-Type、getResponseHeader 恒 null）。
//
// 契约：单值头 → 字符串；同名多值（Header().Add 多次）→ 字符串数组（保序、
// 不合并）；handler 一个头都没写 → headers 是空对象（不是 undefined/null）。
func TestRegisterHTTP_ResponseHeaders(t *testing.T) {
	old := globalRoutes
	globalRoutes = nil
	defer func() { globalRoutes = old }()

	RegisterHTTP("GET", "/api/hdr-probe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("X-Total-Count", "3")
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		io.WriteString(w, "a,b\n1,2\n")
	})
	RegisterHTTP("GET", "/api/no-hdr-probe", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "plain")
	})

	args := []jsc.JSValue{jsc.StringValue("/api/hdr-probe"), jsc.Null()}
	res, err := MatchMethod("GET", "/api/hdr-probe").Handler(args)
	if err != nil {
		t.Fatalf("hdr handler error: %v", err)
	}
	o := res.AsObject()
	if o == nil {
		t.Fatal("dispatchHTTP 返回值不是对象")
	}

	// 向后兼容：status / body 语义不变。
	if st, ok := o.GetByKey("status"); !ok || st.ToNumber() != 200 {
		t.Errorf("status 丢失或被改变：%v", st)
	}
	if b, ok := o.GetByKey("body"); !ok || b.ToString() != "a,b\n1,2\n" {
		t.Errorf("body = %q, want %q", b.ToString(), "a,b\n1,2\n")
	}

	hv, ok := o.GetByKey("headers")
	if !ok || !hv.IsObject() {
		t.Fatalf("返回值缺少 headers 对象：%v", hv)
	}
	hObj := hv.AsObject()
	if hObj == nil {
		t.Fatal("headers 不是可读对象")
	}
	if v, ok := hObj.GetByKey("Content-Type"); !ok || v.ToString() != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", v.ToString())
	}
	if v, ok := hObj.GetByKey("X-Total-Count"); !ok || v.ToString() != "3" {
		t.Errorf("X-Total-Count = %q, want 3", v.ToString())
	}

	// 多值头必须是数组且保序，不能被压成单值。
	multi, ok := hObj.GetByKey("X-Multi")
	if !ok || !multi.IsObject() {
		t.Fatalf("X-Multi 应为多值数组，实际 %v", multi)
	}
	arr := multi.AsObject()
	var got []string
	for i := 0; ; i++ {
		item, ok := arr.GetByKey(strconv.Itoa(i))
		if !ok {
			break
		}
		got = append(got, item.ToString())
	}
	if strings.Join(got, "|") != "a|b" {
		t.Errorf("X-Multi = %v, want [a b]", got)
	}

	// 未调用 w.Header() 的 handler → headers 为空对象（不是缺失/undefined）。
	res2, err := MatchMethod("GET", "/api/no-hdr-probe").Handler(
		[]jsc.JSValue{jsc.StringValue("/api/no-hdr-probe"), jsc.Null()})
	if err != nil {
		t.Fatalf("no-hdr handler error: %v", err)
	}
	o2 := res2.AsObject()
	if o2 == nil {
		t.Fatal("dispatchHTTP 返回值不是对象（无响应头用例）")
	}
	hv2, ok := o2.GetByKey("headers")
	if !ok || !hv2.IsObject() {
		t.Fatalf("无响应头时应回传空对象，实际 %v", hv2)
	}
	if keys := hv2.AsObject().Keys(); len(keys) != 0 {
		t.Errorf("无响应头时 headers 应为空，实际键：%v", keys)
	}
}
