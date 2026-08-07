package bridge

import (
	"encoding/json"
	"net/http"
	"testing"

	"wb-ui/jsc"
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
