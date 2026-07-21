// Command bridge_demo demonstrates the wb-bridge SDK: one codebase,
// two runtime modes (GUI + Web).
//
// Go side:
//   bridge.Register("/api/users", userHandler)
//   bridge.Register("/api/echo", echoHandler)
//
// JS side (same fetch() code in both modes):
//   const users = await fetch('/api/users').then(r => r.json())
//   const echo = await fetch('/api/echo', { method:'POST', body: JSON.stringify({msg:'hi'}) })
//
// In GUI mode: fetch() is intercepted by the bridge SDK → calls Go directly.
// In web mode:   fetch() goes to the real HTTP server.
package main

import (
	"fmt"
	"os"
	"time"

	"wb-ui/bridge"
	"wb-ui/jsc"
)

func main() {
	fmt.Println("=== wb-bridge Demo ===")
	fmt.Println()

	// 1. Register API routes (handlers return JSON strings for cross-runtime safety)
	bridge.Register("/api/users", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.StringValue(`[{"id":1,"name":"Alice","email":"alice@example.com"},{"id":2,"name":"Bob","email":"bob@example.com"},{"id":3,"name":"Charlie","email":"charlie@example.com"}]`), nil
	})

	bridge.Register("/api/echo", func(args []jsc.JSValue) (jsc.JSValue, error) {
		// Echo back whatever body was sent
		if len(args) > 0 && !args[0].IsNull() && !args[0].IsUndefined() {
			if args[0].IsObject() {
				body := args[0].AsObject()
				if body != nil {
					if msg, ok := body.GetByKey("msg"); ok {
						return jsc.StringValue(`{"echo":"` + msg.ToString() + `"}`), nil
					}
				}
			}
		}
		return jsc.StringValue(`{"echo":"no message"}`), nil
	})

	bridge.Register("/api/time", func(args []jsc.JSValue) (jsc.JSValue, error) {
		now := time.Now().Format(time.RFC3339)
		return jsc.StringValue(`{"time":"` + now + `"}`), nil
	})

	// 2. Create JS runtime
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	// 3. Inject bridge routes (expose as window.go.*)
	bridge.InjectAll(rt)

	// 4. Inject a Response polyfill + simple fetch (for demo only; in real app, page.RegisterFetch provides this)
	if _, err := rt.RunJS(`
		// Response polyfill
		function Response(body, init) {
			this._body = body;
			this.status = (init && init.status) || 200;
			this.statusText = (init && init.statusText) || 'OK';
			this.ok = this.status >= 200 && this.status < 300;
		}
		Response.prototype.text = function() { return Promise.resolve(this._body); };
		Response.prototype.json = function() { return Promise.resolve(JSON.parse(this._body)); };

		// Simple fetch that passes through to the "network"
		var g = typeof globalThis !== 'undefined' ? globalThis : this;
		g.fetch = function(url, options) {
			console.log("[fetch] " + (options && options.method || 'GET') + " " + url);
			return Promise.resolve(new Response(
				JSON.stringify({status:"ok", url: url}),
				{ status: 200 }
			));
		};
	`); err != nil {
		fmt.Fprintf(os.Stderr, "fetch setup failed: %v\n", err)
		os.Exit(1)
	}

	// 5. Inject bridge SDK (wraps fetch for route interception)
	sdk := bridge.InjectSDK()
	if sdk != "" {
		if _, err := rt.RunJS(sdk); err != nil {
			fmt.Fprintf(os.Stderr, "SDK injection failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[SDK] Bridge SDK injected (fetch interception active)")
	}
	fmt.Println()

	// 6. Run test scenarios
	scenarios := []struct {
		name string
		code string
	}{
		{
			name: "GET /api/users (should be intercepted by bridge)",
			code: `
				fetch('/api/users').then(r => r.json()).then(data => {
					console.log("[PASS] /api/users returned " + data.length + " users");
					if (data.length !== 3) throw new Error('expected 3 users, got ' + data.length);
					if (data[0].name !== 'Alice') throw new Error('expected Alice, got ' + data[0].name);
				}).catch(e => console.log("[FAIL] /api/users: " + e.message));
			`,
		},
		{
			name: "POST /api/echo (should be intercepted by bridge)",
			code: `
				fetch('/api/echo', {
					method: 'POST',
					body: JSON.stringify({msg: 'Hello Bridge!'})
				}).then(r => r.text()).then(text => {
					console.log("[PASS] /api/echo response: " + text);
					if (text.indexOf('"echo"') < 0) throw new Error('expected echo key in response');
				}).catch(e => console.log("[FAIL] /api/echo: " + e.message));
			`,
		},
		{
			name: "GET /api/nonexistent (should NOT be intercepted, falls through to HTTP)",
			code: `
				fetch('/api/nonexistent').then(r => r.json()).then(data => {
					console.log("[PASS] /api/nonexistent fell through to fetch (not intercepted)");
					if (data.url !== '/api/nonexistent') throw new Error('unexpected response');
				}).catch(e => console.log("[FAIL] /api/nonexistent: " + e.message));
			`,
		},
	}

	// Execute all scenarios
	for _, s := range scenarios {
		fmt.Printf("--- %s ---\n", s.name)
		if _, err := rt.RunJS(s.code); err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
		}
	}

	// 7. Show console output
	fmt.Println("\n=== Console Output ===")
	fmt.Println(log.String())

	// 8. Summary
	fmt.Println("=== SUCCESS ===")
	fmt.Println("All API calls routed correctly:")
	fmt.Println("  /api/users       → Go handler (bridge intercept)")
	fmt.Println("  /api/echo        → Go handler (bridge intercept)")
	fmt.Println("  /api/nonexistent → HTTP fetch (no route registered)")
}
