package bindings

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// TestScrollEventDispatchedFromGo verifies that a scroll event dispatched from
// Go (the Host.dispatchScrollEvent path in app/host.go) reaches a JS-registered
// 'scroll' listener — the exact wiring Vue's @scroll="onScroll" relies on for
// history lazy-loading (loadMoreMessages).
//
// Regression: the engine updated scroll offsets on wheel/scrollbar interaction
// but never dispatched the scroll DOM event, so el.addEventListener('scroll')
// (Vue @scroll) never fired. A history conversation opened with only the last
// 50 raw JSONL lines (~1 run, tool messages consuming the quota) could never
// page further back — "history only loads the last run".
func TestScrollEventDispatchedFromGo(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("scroll-box")
	doc.AppendChild(el)

	// JS side: Vue-equivalent addEventListener('scroll', handler).
	mustRun(t, rt, `
		var el2 = document.getElementById("scroll-box");
		window.__scrollFired = 0;
		el2.addEventListener("scroll", function(ev) {
			window.__scrollFired++;
			window.__scrollType = ev && ev.type;
		}, false);
	`)

	// Go side: exact code path of Host.dispatchScrollEvent.
	el.DispatchEvent(dom.NewEvent("scroll", false, false, false))

	if got := evalInt(t, rt, "window.__scrollFired"); got != 1 {
		t.Fatalf("__scrollFired after Go dispatch = %d, want 1 (JS scroll listener must fire)", got)
	}
	if got := evalStr(t, rt, "window.__scrollType"); got != "scroll" {
		t.Fatalf("__scrollType = %q, want scroll", got)
	}

	// Repeated dispatch (smooth wheel scrolling fires per frame) must keep working.
	el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
	el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
	if got := evalInt(t, rt, "window.__scrollFired"); got != 3 {
		t.Fatalf("__scrollFired after 3 dispatches = %d, want 3", got)
	}
}

func evalInt(t *testing.T, rt *jsc.Interpreter, expr string) int {
	t.Helper()
	v, err := rt.Run(expr)
	if err != nil {
		t.Fatalf("Run(%q) error: %v", expr, err)
	}
	if n, ok := v.(int64); ok {
		return int(n)
	}
	if n, ok := v.(float64); ok {
		return int(n)
	}
	t.Fatalf("Run(%q) returned %T (%v), want number", expr, v, v)
	return 0
}

func evalStr(t *testing.T, rt *jsc.Interpreter, expr string) string {
	t.Helper()
	v, err := rt.Run(expr)
	if err != nil {
		t.Fatalf("Run(%q) error: %v", expr, err)
	}
	if s, ok := v.(string); ok {
		return s
	}
	t.Fatalf("Run(%q) returned %T (%v), want string", expr, v, v)
	return ""
}
