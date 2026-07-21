/**
 * wb-bridge.js — Unified API Bridge for wb-ui GUI + Web
 *
 * This SDK provides a unified API layer that works in both environments:
 * - GUI mode (wb-ui engine): fetch() is automatically intercepted for registered
 *   Go function routes. Use fetch('/api/users') as usual.
 * - Web mode: fetch() goes to the real HTTP server as usual.
 *
 * Additionally, this SDK provides helpers:
 * - wb.call(name, args) — call a named API endpoint
 * - wb.register(pattern, handler) — register a route in GUI mode
 * - wb.isGUI — true if running in wb-ui engine
 *
 * Usage:
 *   <script src="wb-bridge.js"></script>
 *   <script>
 *     // Standard fetch works in both modes
 *     const res = await fetch('/api/users')
 *     const users = await res.json()
 *
 *     // Or use the wb.call helper
 *     const users = await wb.call('users')
 *   </script>
 */
(function(global) {
  'use strict';

  var WB = {
    isGUI: false,
    _routes: {},
    _handlers: {}
  };

  // Detect wb-ui GUI mode
  if (global.__WB_BRIDGE__ || (global.go && typeof global.go === 'object')) {
    WB.isGUI = true;
  }

  // --- Route registration (GUI mode only) ---
  // In GUI mode, register a named API endpoint backed by a Go function
  // that was registered via bridge.Register(pattern, handler).
  WB.register = function(name, pattern) {
    if (!WB.isGUI) {
      console.warn('wb-bridge: register() is a no-op in web mode');
      return;
    }
    WB._routes[name] = pattern;
  };

  // --- Unified API call ---
  // wb.call('users', { page: 1 }) calls the registered API.
  // In GUI mode: calls the Go function via go.<name>.
  // In web mode: does fetch('/api/' + name, { body: JSON.stringify(args) }).
  WB.call = function(name, args) {
    args = args || {};
    if (WB.isGUI && global.go && global.go[name]) {
      // Direct Go function call in GUI mode
      try {
        var result = global.go[name](args);
        return Promise.resolve(result);
      } catch (e) {
        return Promise.reject(e);
      }
    }
    // Web mode or fallback: HTTP fetch
    var url = '/api/' + name;
    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(args)
    }).then(function(res) {
      if (!res.ok) throw new Error('API ' + name + ' returned ' + res.status);
      return res.json();
    });
  };

  // --- Auto-wrap registered go functions for fetch interception ---
  // In GUI mode, if the go namespace has registered functions, wrap fetch
  // to intercept calls to /api/{name} patterns.
  if (WB.isGUI && global.go && global.fetch) {
    var _fetch = global.fetch;
    var goKeys = [];
    try {
      for (var k in global.go) {
        if (typeof global.go[k] === 'function') goKeys.push(k);
      }
    } catch(e) {}

    if (goKeys.length > 0) {
      global.fetch = function(url, options) {
        // Check if url matches any go function name pattern
        var matchedName = null;
        for (var i = 0; i < goKeys.length; i++) {
          var name = goKeys[i];
          // Try exact match: /api/name
          if (url === '/api/' + name || url === '/' + name) {
            matchedName = name;
            break;
          }
          // Try pattern-based match
          var pattern = WB._routes[name];
          if (pattern && url.indexOf(pattern) === 0) {
            matchedName = name;
            break;
          }
        }

        if (matchedName && global.go[matchedName]) {
          var body = {};
          try {
            if (options && options.body) {
              body = JSON.parse(options.body);
            }
          } catch(e) {}
          var result = global.go[matchedName](body);
          // Build synthetic Response
          var responseBody = (typeof result === 'string')
            ? result
            : JSON.stringify(result != null ? result : {});
          return Promise.resolve(new Response(responseBody, {
            status: 200,
            statusText: 'OK',
            headers: { 'Content-Type': 'application/json' }
          }));
        }

        // Fall through to original fetch (HTTP)
        if (_fetch) return _fetch.call(global, url, options);
        return Promise.reject(new Error('fetch not available'));
      };
    }
  }

  // Expose
  global.wb = WB;
})(typeof window !== 'undefined' ? window : globalThis);
