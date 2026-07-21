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
// Usage (JS side — same code in both modes):
//
//	const res = await fetch('/api/users')
//	const users = await res.json()
package bridge

import (
	"encoding/json"
	"fmt"
	"strings"

	"wb-ui/bindings"
	"wb-ui/jsc"
)

// Route represents a registered API endpoint.
type Route struct {
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
	jsName := patternToJSName(pattern)
	globalRoutes = append(globalRoutes, Route{
		Pattern: pattern,
		JSName:  jsName,
		Handler: handler,
	})
}

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

func matchPattern(pattern, url string) bool {
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(url, prefix)
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
		for (var pattern in ROUTES) {
			if (pattern.endsWith('/*')) {
				var prefix = pattern.slice(0, -2);
				if (url.indexOf(prefix) === 0) return ROUTES[pattern];
			} else if (url === pattern) {
				return ROUTES[pattern];
			}
		}
		return null;
	}

	global.fetch = function(url, options) {
		var jsName = matchRoute(url);
		if (jsName && global.go && global.go[jsName]) {
			try {
				var body = {};
				if (options && options.body) {
					try { body = JSON.parse(options.body); }
					catch(e) {}
				}
				var result = global.go[jsName](body);
				// result can be: JSON string, Go-backed object, or plain object
				var responseBody;
				if (typeof result === 'string') {
					responseBody = result;
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
					status: 200,
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
