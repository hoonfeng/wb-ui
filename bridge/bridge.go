// Package bridge implements the Go ↔ JS API bridge for wb-ui.
// It allows Go functions to be registered as API endpoints that front-end code
// can call via standard fetch() / XMLHttpRequest without modification.
//
// In GUI mode (wb-ui), fetch calls matching registered API routes are intercepted
// and routed to Go functions directly, bypassing HTTP.
// In web mode, routes are not registered and fetch falls through to real HTTP.
//
// Usage (Go side):
//
//	bridge.Register("/api/users", func(args []jsc.JSValue) (jsc.JSValue, error) {
//	    users := getUsersFromDB()
//	    return bindings.ToJSValue(users), nil
//	})
//
// Or for standard http.HandlerFunc-style handlers (recommended):
//
//	bridge.RegisterHTTP("GET", "/api/users", func(w http.ResponseWriter, r *http.Request) {
//	    json.NewEncoder(w).Encode(users)
//	})
//
// Usage (JS side — same code in both modes):
//
//	const res = await fetch('/api/users')
//	const users = await res.json()
package bridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"wb-ui/bindings"
	"wb-ui/jsc"
)

// Route represents a registered API endpoint.
type Route struct {
	Method  string             // HTTP method, e.g. "GET"/"POST". "" = any.
	Pattern string             // URL pattern, e.g. "/api/users" or "/api/*"
	JSName  string             // name exposed on window.go, e.g. "api_users"
	Handler bindings.GoCallback // Go callback function
}

var globalRoutes []Route

// Register registers a Go function as an API endpoint. pattern supports:
//   - "/api/users"   — exact match
//   - "/api/*"       — prefix wildcard (matches "/api/users", "/api/posts", etc.)
//
// The handler is automatically exposed as window.go.<jsName> for direct
// JS access, and also as a route that can be intercepted by the bridge SDK.
func Register(pattern string, handler bindings.GoCallback) {
	registerRoute("", pattern, handler)
}

// RegisterHTTP registers a standard http.HandlerFunc as an API endpoint.
// method: "GET"/"POST"/"PUT"/"DELETE" (empty = any).
// pattern supports exact ("/api/users"), prefix ("/api/*") and trailing-slash
// prefix ("/api/conversations/") forms.
//
// The handler receives a real *http.Request (URL path, query, body and headers
// all populated from the JS fetch() call) and writes to a synthetic
// http.ResponseWriter. This makes GUI-mode API calls indistinguishable from
// real HTTP on the Go side — no custom request/response parsing needed.
//
// The handler is also exposed as window.go.<jsName> and matches fetch()
// calls via the native fetch() intercept (page.RegisterFetch).
func RegisterHTTP(method, pattern string, handler http.HandlerFunc) {
	if handler == nil {
		return
	}
	registerRoute(method, pattern, func(args []jsc.JSValue) (jsc.JSValue, error) {
		return dispatchHTTP(args, handler)
	})
}

func registerRoute(method, pattern string, handler bindings.GoCallback) {
	jsName := patternToJSName(pattern)
	globalRoutes = append(globalRoutes, Route{
		Method:  strings.ToUpper(method),
		Pattern: pattern,
		JSName:  jsName,
		Handler: handler,
	})
}

// dispatchHTTP adapts a JS fetch() call (args[0]=url, args[1]=options) into a
// *http.Request + synthetic ResponseWriter and invokes the handler.
// Returns a JS object {"status": N, "body": "..."} so the caller can build a
// proper fetch() Response with the right status code.
func dispatchHTTP(args []jsc.JSValue, handler http.HandlerFunc) (jsc.JSValue, error) {
	urlStr := ""
	if len(args) > 0 && args[0].IsString() {
		urlStr = args[0].ToString()
	}
	method := "GET"
	var bodyStr string
	headers := http.Header{}
	if len(args) >= 2 && args[1].IsObject() {
		o := args[1].AsObject()
		if o != nil {
			if m, ok := o.GetByKey("method"); ok && !m.IsUndefined() && !m.IsNull() {
				method = m.ToString()
			}
			if b, ok := o.GetByKey("body"); ok && !b.IsUndefined() && !b.IsNull() {
				bodyStr = b.ToString()
			}
			if h, ok := o.GetByKey("headers"); ok && h.IsObject() {
				hObj := h.AsObject()
				if hObj != nil {
					for _, key := range hObj.Keys() {
						if val, ok2 := hObj.GetByKey(key); ok2 {
							headers.Set(key, val.ToString())
						}
					}
				}
			}
		}
	}

	// Build the request: strip origin, keep path+query.
	path := urlStr
	if i := strings.Index(path, "://"); i >= 0 {
		rest := path[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			path = rest[j:]
		} else {
			path = "/"
		}
	}
	var rawQuery string
	if q := strings.IndexByte(path, '?'); q >= 0 {
		rawQuery = path[q+1:]
		path = path[:q]
	}
	req, err := http.NewRequest(method, path, strings.NewReader(bodyStr))
	if err != nil {
		return jsc.Undefined(), fmt.Errorf("bridge: bad request: %w", err)
	}
	if rawQuery != "" {
		req.URL.RawQuery = rawQuery
	}
	req.Header = headers
	if bodyStr != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	vw := &virtualWriter{status: http.StatusOK}
	handler(vw, req)

	obj := jsc.NewObject(nil)
	obj.Set("status", jsc.NumberValue(float64(vw.status)))
	obj.Set("body", jsc.StringValue(vw.buf.String()))
	return jsc.ObjectValue(obj), nil
}

// virtualWriter collects handler output without a real network connection.
type virtualWriter struct {
	status int
	buf    strings.Builder
	hdr    http.Header
}

func (v *virtualWriter) Header() http.Header {
	if v.hdr == nil {
		v.hdr = http.Header{}
	}
	return v.hdr
}
func (v *virtualWriter) Write(b []byte) (int, error) { return v.buf.Write(b) }
func (v *virtualWriter) WriteHeader(status int)      { v.status = status }

// Match finds the first registered route matching the given URL.
// Returns nil if no route matches.
func Match(url string) *Route {
	for i := range globalRoutes {
		r := &globalRoutes[i]
		if matchPattern(r.Pattern, url) {
			return r
		}
	}
	return nil
}

// MatchMethod finds the first registered route matching method + URL.
// A route with empty Method matches any method.
func MatchMethod(method, url string) *Route {
	method = strings.ToUpper(method)
	for i := range globalRoutes {
		r := &globalRoutes[i]
		if r.Method != "" && r.Method != method {
			continue
		}
		if matchPattern(r.Pattern, url) {
			return r
		}
	}
	return nil
}

// InjectAll registers all routes on the given JS interpreter as window.go.*
// functions. Call this before the application JS executes.
func InjectAll(rt *jsc.Interpreter) {
	for i := range globalRoutes {
		r := &globalRoutes[i]
		bindings.RegisterGoFunction(rt, r.JSName, r.Handler)
	}
}

// InjectSDK returns the JS SDK script that should be injected into the page
// before any application code runs. The SDK wraps fetch() to intercept
// registered routes in GUI mode. In web mode, it's a no-op.
func InjectSDK() string {
	routesJSON, _ := json.Marshal(buildRouteMap())
	return fmt.Sprintf(bridgeSDKTemplate, string(routesJSON))
}

// RoutesJSON returns the route table as a JSON string for injection into
// the JS environment.
func RoutesJSON() string {
	data, _ := json.Marshal(buildRouteMap())
	return string(data)
}

// --- internals ---

func patternToJSName(pattern string) string {
	s := strings.TrimPrefix(pattern, "/")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "*", "_wildcard")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

// matchPattern matches a URL against a route pattern. Supports:
//   - exact:  pattern == url
//   - prefix: pattern ends with "/*" → HasPrefix
//   - trailing slash: pattern ends with "/" → HasPrefix (e.g. /api/conversations/)
//
// Query strings and origin (http://host) are stripped from url before matching.
func matchPattern(pattern, url string) bool {
	// Strip query string.
	if q := strings.IndexByte(url, '?'); q >= 0 {
		url = url[:q]
	}
	// Strip origin.
	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			url = rest[j:]
		} else {
			url = "/"
		}
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(url, prefix)
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(url, pattern)
	}
	return url == pattern
}

// buildRouteMap builds a map[urlPrefix]jsName for the SDK.
func buildRouteMap() map[string]string {
	m := make(map[string]string)
	for _, r := range globalRoutes {
		m[r.Pattern] = r.JSName
	}
	return m
}

// bridgeSDKTemplate is the JS SDK injected into GUI-mode pages.
// It wraps the global fetch() to intercept API calls matching registered routes.
const bridgeSDKTemplate = `(function(global) {
	// --- wb-ui Bridge SDK ---
	// In GUI mode: intercepts fetch() for registered API routes,
	// routing them directly to Go functions.
	// In web mode: ROUTES is empty/absent, SDK is a no-op.

	var ROUTES = %s;
	if (!ROUTES || Object.keys(ROUTES).length === 0) return;

	var _fetch = global.fetch;

	// Simple Response polyfill for environments without native Response.
	function Response(body, init) {
		this._body = body;
		this.status = (init && init.status) || 200;
		this.statusText = (init && init.statusText) || 'OK';
		this.ok = this.status >= 200 && this.status < 300;
		this._headers = (init && init.headers) || {};
	}
	Response.prototype.text = function() {
		return Promise.resolve(this._body);
	};
	Response.prototype.json = function() {
		var self = this;
		return Promise.resolve().then(function() {
			return JSON.parse(self._body);
		});
	};

	function matchRoute(url) {
		// Normalize: strip origin (http://host) and query string so patterns
		// like "/api/users" match the absolute URLs api.js builds via
		// new URL('/api/...', location.origin).toString().
		var nu = String(url);
		if (nu.indexOf('://') >= 0) {
			try {
				var _u = new URL(nu);
				nu = _u.pathname;
			} catch(e) {
				nu = nu.substring(nu.indexOf('://') + 3);
				var s = nu.indexOf('/');
				nu = s >= 0 ? nu.substring(s) : '/';
			}
		}
		var q = nu.indexOf('?');
		if (q >= 0) nu = nu.substring(0, q);
		for (var pattern in ROUTES) {
			if (pattern.endsWith('/*')) {
				var prefix = pattern.slice(0, -2);
				if (nu.indexOf(prefix) === 0) return ROUTES[pattern];
			} else if (pattern.slice(-1) === '/') {
				if (nu.indexOf(pattern) === 0) return ROUTES[pattern];
			} else if (nu === pattern) {
				return ROUTES[pattern];
			}
		}
		return null;
	}

	global.fetch = function(url, options) {
		var jsName = matchRoute(url);
		if (jsName && global.go && global.go[jsName]) {
			try {
				// Two-layer direct call: JS → native jsc function (window.go.*)
				// → Go handler. No URL parsing, no JSON re-encoding, no HTTP.
				// RegisterHTTP handlers expect [url, options]; result is a
				// string, or an object {"status": N, "body": "..."}.
				var result = global.go[jsName](url, options || {});
				var responseBody;
				var status = 200;
				if (typeof result === 'string') {
					responseBody = result;
				} else if (result !== null && result !== undefined && typeof result === 'object' && typeof result.body !== 'undefined') {
					status = result.status || 200;
					responseBody = result.body;
				} else if (result !== null && result !== undefined) {
					try {
						responseBody = JSON.stringify(result);
					} catch(e) {
						responseBody = String(result);
					}
				} else {
					responseBody = '{}';
				}
				return Promise.resolve(new Response(responseBody, {
					status: status,
					statusText: status >= 400 ? 'Error' : 'OK',
					headers: { 'Content-Type': 'application/json' }
				}));
			} catch(e) {
				return Promise.reject(e);
			}
		}
		if (_fetch) return _fetch(url, options);
		return Promise.reject(new Error('fetch not available'));
	};
})(typeof globalThis !== 'undefined' ? globalThis : this);
`
