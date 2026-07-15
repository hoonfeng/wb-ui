package page

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wb-ui/jsc"
)

// TestRegisterFetch verifies that the fetch() function is registered and can make
// basic HTTP requests.
//
// Note: The Promise chain does NOT auto-unwrap nested Promises returned from .then()
// callbacks. So fetch(url).then(r => r.text()).then(t => ...) will pass the inner
// Promise (not the resolved text) to the second then. For now we test the Response
// fields directly from the first then.
func TestRegisterFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/echo" {
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"echo":%q}`, string(body))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Hello, fetch! path=%s", r.URL.Path)
	}))
	defer ts.Close()

	// Test basic GET: verify Response fields from the first then.
	out := runFetchJS(t, fmt.Sprintf(`
		fetch("%s/hello").then(function(resp) {
			console.log(resp.status);
			console.log(resp.ok);
			console.log(resp.url);
		});
	`, ts.URL))

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected 3 output lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "200" {
		t.Errorf("status = %q, want '200'", lines[0])
	}
	if lines[1] != "true" {
		t.Errorf("ok = %q, want 'true'", lines[1])
	}

	// Test fetch with missing URL (should reject).
	out2 := runFetchJS(t, `
		fetch().then(
			function(v) { console.log("unexpected-resolve"); },
			function(e) { console.log("rejected"); }
		);
	`)
	if got := strings.TrimSpace(out2); got != "rejected" {
		t.Errorf("fetch() no args = %q, want 'rejected'", got)
	}
}

// TestRegisterXMLHttpRequest verifies that the XMLHttpRequest constructor works
// for basic GET requests and event handling.
func TestRegisterXMLHttpRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "XHR response for %s", r.URL.Path)
	}))
	defer ts.Close()

	out := runXHRJS(t, fmt.Sprintf(`
		var xhr = new XMLHttpRequest();
		xhr.open("GET", "%s/data");
		xhr.onreadystatechange = function() {
			if (xhr.readyState == 4) {
				console.log("status: " + xhr.status);
				console.log("body: " + xhr.responseText);
			}
		};
		xhr.send();
	`, ts.URL))

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 output lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "status: 200" {
		t.Errorf("status = %q, want 'status: 200'", lines[0])
	}
	if !strings.Contains(lines[1], "XHR response") {
		t.Errorf("body = %q, want contains 'XHR response'", lines[1])
	}

	// Test XHR with abort.
	out2 := runXHRJS(t, `
		var xhr2 = new XMLHttpRequest();
		xhr2.open("GET", "http://example.com/");
		xhr2.abort();
		console.log("readyState after abort: " + xhr2.readyState);
	`)
	if got := strings.TrimSpace(out2); got != "readyState after abort: 0" {
		t.Errorf("abort = %q, want 'readyState after abort: 0'", got)
	}
}

// runFetchJS creates a fresh interpreter with fetch() registered, runs script,
// and returns console output.
func runFetchJS(t *testing.T, script string) string {
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

// runXHRJS creates a fresh interpreter with XMLHttpRequest registered, runs script,
// and returns console output.
func runXHRJS(t *testing.T, script string) string {
	t.Helper()
	in := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	in.SetupGlobal(log)
	RegisterXMLHttpRequest(in)
	_, err := in.Run(script)
	if err != nil {
		t.Fatalf("Run(%q) error: %v", script, err)
	}
	return log.String()
}
