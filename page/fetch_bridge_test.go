package page

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"wb-ui/bridge"
	"wb-ui/jsc"
)

// TestFetchBridgeIntercept verifies the full two-layer chain:
//   JS fetch('/api/x') → native fetch() → bridge.MatchMethod → Go http.Handler
//   (registered via bridge.RegisterHTTP) → synthetic Response with status/body.
//
// This is the path gou-ide desktop mode relies on: no JS interceptor, no
// go.bridge_call, no simulated HTTP dispatch — the bridge IS the interceptor.
func TestFetchBridgeIntercept(t *testing.T) {
	// Register routes (package-global state; tests run serially).
	bridge.RegisterHTTP("GET", "/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"name": "alice"})
	})
	bridge.RegisterHTTP("GET", "/api/conversations/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Path, "q": r.URL.Query().Get("ws")})
	})
	bridge.RegisterHTTP("POST", "/api/users", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]string{"got": body["name"]})
	})

	out := runFetchBridgeJS(t, `
		fetch("http://localhost:9090/api/users").then(function(resp) {
			console.log("status=" + resp.status);
			console.log("ok=" + resp.ok);
			resp.json().then(function(v) {
				console.log("json=" + JSON.stringify(v));
			}, function(e) {
				console.log("json-err=" + String(e && e.message || e));
			});
		}, function(e) {
			console.log("fetch-err=" + String(e && e.message || e));
		});
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "status=201" {
		t.Errorf("line0 = %q, want 'status=201'", lines[0])
	}
	if lines[1] != "ok=true" {
		t.Errorf("line1 = %q, want 'ok=true'", lines[1])
	}
	if !strings.Contains(lines[2], `"name":"alice"`) {
		t.Errorf("line2 = %q, want contains alice", lines[2])
	}

	// Prefix route with query.
	out2 := runFetchBridgeJS(t, `
		fetch("http://localhost:9090/api/conversations/conv_1/messages?ws=root").then(function(resp) {
			resp.json().then(function(v) {
				console.log(JSON.stringify(v));
			});
		});
	`)
	l2 := strings.TrimSpace(out2)
	if !strings.Contains(l2, `"path":"/api/conversations/conv_1/messages"`) || !strings.Contains(l2, `"q":"root"`) {
		t.Errorf("prefix+query result = %q", l2)
	}

	// POST with JSON body.
	out3 := runFetchBridgeJS(t, `
		fetch("http://localhost:9090/api/users", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ name: "bob" })
		}).then(function(resp) {
			resp.json().then(function(v) {
				console.log(JSON.stringify(v));
			});
		});
	`)
	l3 := strings.TrimSpace(out3)
	if !strings.Contains(l3, `"got":"bob"`) {
		t.Errorf("POST result = %q", l3)
	}
}

// runFetchBridgeJS creates a fresh interpreter with fetch() registered, runs
// script, and returns console output. Routes registered by the caller are
// matched via the global route table.
func runFetchBridgeJS(t *testing.T, script string) string {
	t.Helper()
	in := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	in.SetupGlobal(log)
	RegisterFetch(in)
	_, err := in.Run(script)
	if err != nil {
		t.Fatalf("Run(%q) error: %v", script, err)
	}
	return log.String()
}
