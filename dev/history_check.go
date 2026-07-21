package main

import (
	"fmt"
	"os"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/jsc"
)

func main() {
	pass, fail := 0, 0
	assert := func(name string, ok bool, detail string) {
		if ok { pass++; fmt.Printf("  PASS: %s\n", name) 
		} else { fail++; fmt.Printf("  FAIL: %s — %s\n", name, detail) }
	}

	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	doc := dom.NewDocument()
	bindings.RegisterDOMBindings(rt, doc)

	fmt.Println("=== History API Tests ===\n")

	// Test 1: Initial state
	assert("Initial pathname is /",
		runBool(rt, `window.location.pathname === '/'`),
		runStr(rt, `window.location.pathname`))
	assert("Initial history.length is 1",
		runBool(rt, `window.history.length === 1`),
		runStr(rt, `String(window.history.length)`))
	assert("Initial history.state is null",
		runBool(rt, `window.history.state === null`),
		runStr(rt, `String(window.history.state)`))

	// Test 2: pushState updates location
	rt.Run(`window.history.pushState({page:1}, '', '/page1')`)
	assert("pushState updates pathname to /page1",
		runBool(rt, `window.location.pathname === '/page1'`),
		runStr(rt, `window.location.pathname`))
	assert("history.length becomes 2 after pushState",
		runBool(rt, `window.history.length === 2`),
		runStr(rt, `String(window.history.length)`))
	assert("history.state reflects pushed state",
		runBool(rt, `String(window.history.state).indexOf('page') >= 0`),
		runStr(rt, `String(window.history.state)`))

	// Test 3: register popstate listener
	rt.Run(`window._popFired = false; window.addEventListener('popstate', function(e){ window._popFired = true; window._popState = e.state; console.log('popstate fired:', e.state); })`)

	// Test 4: history.back fires popstate
	rt.Run(`window.history.back()`)
	assert("history.back fires popstate",
		runBool(rt, `window._popFired === true`),
		"popstate should have been dispatched")
	assert("after back, pathname returns to /",
		runBool(rt, `window.location.pathname === '/'`),
		runStr(rt, `window.location.pathname`))

	// Test 5: history.forward fires popstate
	rt.Run(`window._popFired = false; window.history.forward()`)
	assert("history.forward fires popstate",
		runBool(rt, `window._popFired === true`),
		"popstate should have been dispatched")
	assert("after forward, pathname is back to /page1",
		runBool(rt, `window.location.pathname === '/page1'`),
		runStr(rt, `window.location.pathname`))

	// Test 6: pushState doesn't fire popstate (browser spec)
	rt.Run(`window._popFired = false; window.history.pushState({page:2}, '', '/page2')`)
	assert("pushState does NOT fire popstate",
		runBool(rt, `window._popFired === false`),
		"pushState must NOT trigger popstate per spec")

	// Test 7: replaceState updates current entry
	rt.Run(`window.history.pushState({page:3}, '', '/page3'); window.history.replaceState({page:9}, '', '/page9')`)
	assert("replaceState updates pathname",
		runBool(rt, `window.location.pathname === '/page9'`),
		runStr(rt, `window.location.pathname`))
	assert("replaceState keeps length same",
		runBool(rt, `window.history.length === 4`),
		runStr(rt, `String(window.history.length)`))

	// Test 8: removeEventListener
	rt.Run(`
		window._popFired = false;
		function handler(){ window._popFired = true; }
		window.addEventListener('popstate', handler);
		window.removeEventListener('popstate', handler);
		window.history.back();
	`)
	assert("removeEventListener works (no double fire)",
		runBool(rt, `window._popFired === false`),
		"popstate should not have fired after removal")

	fmt.Println("\n=== Console ===")
	for _, line := range splitLines(log.String()) {
		fmt.Printf("  %s\n", line)
	}

	fmt.Printf("\n=== %d PASS, %d FAIL ===\n", pass, fail)
	if fail > 0 { os.Exit(1) }
}

func runBool(rt *jsc.Interpreter, code string) bool {
	v, err := rt.RunJS(code)
	if err != nil { return false }
	return v.ToBoolean()
}

func runStr(rt *jsc.Interpreter, code string) string {
	v, err := rt.RunJS(code)
	if err != nil { return "ERR:" + err.Error() }
	return v.ToString()
}

func splitLines(s string) []string {
	var lines []string
	for _, l := range []string{} {
		_ = l
	}
	// Simple split
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if t := s[start:i]; t != "" { lines = append(lines, t) }
			start = i + 1
		}
	}
	if start < len(s) {
		if t := s[start:]; t != "" { lines = append(lines, t) }
	}
	return lines
}
