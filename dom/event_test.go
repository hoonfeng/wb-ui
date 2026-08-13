package dom

import "testing"

// TestEventConstructor covers NewEvent and the read-only attributes.
func TestEventConstructor(t *testing.T) {
	ev := NewEvent("click", true, false, false)
	if ev.Type() != "click" {
		t.Errorf("Type = %q, want %q", ev.Type(), "click")
	}
	if !ev.Bubbles() {
		t.Errorf("Bubbles = false, want true")
	}
	if ev.Cancelable() {
		t.Errorf("Cancelable = true, want false")
	}
	if ev.IsTrusted() {
		t.Errorf("user-created event should not be trusted")
	}
	if ev.DefaultPrevented() {
		t.Errorf("fresh event should not be DefaultPrevented")
	}
}

// TestAddRemoveListener checks registration and removal return values.
func TestAddRemoveListener(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	called := false
	cb := EventListenerFunc(func(Event) { called = true })
	if !el.AddEventListener("click", cb) {
		t.Errorf("first AddEventListener should return true")
	}
	if el.AddEventListener("click", cb) {
		t.Errorf("duplicate AddEventListener should return false")
	}
	if !el.HasEventListener("click") {
		t.Errorf("HasEventListener should be true after add")
	}
	if !el.RemoveEventListener("click", cb) {
		t.Errorf("RemoveEventListener of registered listener should return true")
	}
	if el.HasEventListener("click") {
		t.Errorf("HasEventListener should be false after remove")
	}
	if el.RemoveEventListener("click", cb) {
		t.Errorf("RemoveEventListener of unregistered listener should return false")
	}
	_ = el.DispatchEvent(NewEvent("click", false, false, false))
	if called {
		t.Errorf("removed listener should not be called")
	}
}

// TestDispatchEventTargetPhase verifies that listeners on the target fire on dispatch.
func TestDispatchEventTargetPhase(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	_ = d.AppendChild(el)
	calls := 0
	el.AddEventListener("click", EventListenerFunc(func(e Event) {
		calls++
		if e.EventPhase() != EventAtTarget {
			t.Errorf("phase = %d, want %d (AtTarget)", e.EventPhase(), EventAtTarget)
		}
		if e.Target() != el {
			t.Errorf("Target = %v, want el", e.Target())
		}
		if e.CurrentTarget() != el {
			t.Errorf("CurrentTarget = %v, want el", e.CurrentTarget())
		}
	}))
	_ = el.DispatchEvent(NewEvent("click", false, false, false))
	if calls != 1 {
		t.Errorf("listener called %d times, want 1", calls)
	}
}

// TestEventBubbling checks that bubble-phase listeners on ancestors fire in order.
func TestEventBubbling(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	mid := d.CreateElement("mid")
	_ = root.AppendChild(mid)
	leaf := d.CreateElement("leaf")
	_ = mid.AppendChild(leaf)
	var order []string
	addBubble := func(n Node, name string) {
		n.AddEventListener("test", EventListenerFunc(func(e Event) {
			if e.EventPhase() == EventAtTarget || e.EventPhase() == EventBubblingPhase {
				order = append(order, name)
			}
		}))
	}
	addBubble(root, "root")
	addBubble(mid, "mid")
	addBubble(leaf, "leaf")
	_ = leaf.DispatchEvent(NewEvent("test", true, false, false))
	want := []string{"leaf", "mid", "root"}
	if len(order) != len(want) {
		t.Fatalf("bubble order len = %d, want %d (%v)", len(order), len(want), order)
	}
	for i, name := range want {
		if order[i] != name {
			t.Errorf("order[%d] = %q, want %q (full: %v)", i, order[i], name, order)
		}
	}
}

// TestEventCapture checks that capture-phase listeners fire from the root down to the
// target's parent, before the target and bubble listeners.
func TestEventCapture(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	mid := d.CreateElement("mid")
	_ = root.AppendChild(mid)
	leaf := d.CreateElement("leaf")
	_ = mid.AppendChild(leaf)
	var order []string
	addCapture := func(n Node, name string) {
		n.AddEventListener("test", EventListenerFunc(func(e Event) {
			if e.EventPhase() == EventCapturingPhase {
				order = append(order, name)
			}
		}), true)
	}
	addBubble := func(n Node, name string) {
		n.AddEventListener("test", EventListenerFunc(func(e Event) {
			if e.EventPhase() == EventAtTarget || e.EventPhase() == EventBubblingPhase {
				order = append(order, name)
			}
		}))
	}
	addCapture(root, "cap-root")
	addCapture(mid, "cap-mid")
	addBubble(leaf, "leaf")
	addBubble(mid, "mid")
	addBubble(root, "root")
	_ = leaf.DispatchEvent(NewEvent("test", true, false, false))
	want := []string{"cap-root", "cap-mid", "leaf", "mid", "root"}
	if len(order) != len(want) {
		t.Fatalf("order len = %d, want %d (%v)", len(order), len(want), order)
	}
	for i, name := range want {
		if order[i] != name {
			t.Errorf("order[%d] = %q, want %q (full: %v)", i, order[i], name, order)
		}
	}
}

// TestStopPropagation verifies that StopPropagation halts progression to further targets.
func TestStopPropagation(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	leaf := d.CreateElement("leaf")
	_ = root.AppendChild(leaf)
	rootCalls := 0
	leaf.AddEventListener("test", EventListenerFunc(func(e Event) {
		e.StopPropagation()
	}))
	root.AddEventListener("test", EventListenerFunc(func(Event) {
		rootCalls++
	}))
	_ = leaf.DispatchEvent(NewEvent("test", true, false, false))
	if rootCalls != 0 {
		t.Errorf("root called %d times, want 0 after StopPropagation", rootCalls)
	}
}

// TestStopImmediatePropagation verifies that StopImmediatePropagation halts further
// listeners on the current target.
func TestStopImmediatePropagation(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	calls := 0
	el.AddEventListener("test", EventListenerFunc(func(e Event) {
		calls++
		e.StopImmediatePropagation()
	}))
	el.AddEventListener("test", EventListenerFunc(func(Event) {
		calls++
	}))
	_ = el.DispatchEvent(NewEvent("test", false, false, false))
	if calls != 1 {
		t.Errorf("listeners called %d times, want 1", calls)
	}
}

// TestPreventDefault verifies that preventDefault only works on cancelable events and
// causes DispatchEvent to return false.
func TestPreventDefault(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	el.AddEventListener("test", EventListenerFunc(func(e Event) {
		e.PreventDefault()
	}))
	if !el.DispatchEvent(NewEvent("test", false, false, false)) {
		t.Errorf("non-cancelable event should not be canceled by preventDefault")
	}
	if el.DispatchEvent(NewEvent("test", false, true, false)) {
		// bubbles=false cancelable=true; listener prevents default.
	}
	if el.DispatchEvent(NewEvent("test", false, true, false)) {
		t.Errorf("cancelable event with preventDefault should return false from DispatchEvent")
	}
}

// TestOnceListener verifies that a once listener is removed after firing.
func TestOnceListener(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	calls := 0
	el.AddEventListenerWithOptions("test", EventListenerFunc(func(Event) {
		calls++
	}), AddEventListenerOptions{Once: true})
	_ = el.DispatchEvent(NewEvent("test", false, false, false))
	_ = el.DispatchEvent(NewEvent("test", false, false, false))
	if calls != 1 {
		t.Errorf("once listener called %d times, want 1", calls)
	}
	if el.HasEventListener("test") {
		t.Errorf("once listener should be removed after firing")
	}
}

// TestPassiveListener verifies that preventDefault is a no-op inside a passive listener.
func TestPassiveListener(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	el.AddEventListenerWithOptions("test", EventListenerFunc(func(e Event) {
		e.PreventDefault()
	}), AddEventListenerOptions{Passive: true})
	if !el.DispatchEvent(NewEvent("test", false, true, false)) {
		t.Errorf("preventDefault in passive listener should not cancel the event")
	}
}

// TestMouseEventFields covers MouseEvent construction and field accessors.
func TestMouseEventFields(t *testing.T) {
	m := NewMouseEvent("click", true, true, false)
	m.initMouseEvent("click", true, true, 2, 10, 20, 30, 40, true, false, true, false, MouseButtonLeft, nil)
	if m.Detail() != 2 {
		t.Errorf("Detail = %d, want 2", m.Detail())
	}
	if m.ScreenX() != 10 || m.ScreenY() != 20 {
		t.Errorf("Screen = (%v, %v), want (10, 20)", m.ScreenX(), m.ScreenY())
	}
	if m.ClientX() != 30 || m.ClientY() != 40 {
		t.Errorf("Client = (%v, %v), want (30, 40)", m.ClientX(), m.ClientY())
	}
	if !m.CtrlKey() || m.AltKey() {
		t.Errorf("Ctrl=%v Alt=%v, want true/false", m.CtrlKey(), m.AltKey())
	}
	if !m.ShiftKey() || m.MetaKey() {
		t.Errorf("Shift=%v Meta=%v, want true/false", m.ShiftKey(), m.MetaKey())
	}
	if m.Button() != MouseButtonLeft {
		t.Errorf("Button = %d, want %d", m.Button(), MouseButtonLeft)
	}
	if !m.GetModifierState("Control") {
		t.Errorf("GetModifierState(Control) should be true")
	}
	if m.GetModifierState("Alt") {
		t.Errorf("GetModifierState(Alt) should be false")
	}
}

// TestMouseEventFromInit covers the init-dictionary constructor.
func TestMouseEventFromInit(t *testing.T) {
	m := NewMouseEventFromInit("click", MouseEventInit{
		EventInit: EventInit{Bubbles: true, Cancelable: true},
		ScreenX:   100, ScreenY: 200,
		Button:    MouseButtonRight,
		CtrlKey:   true,
	})
	if m.ScreenX() != 100 || m.ScreenY() != 200 {
		t.Errorf("Screen = (%v, %v), want (100, 200)", m.ScreenX(), m.ScreenY())
	}
	if m.Button() != MouseButtonRight {
		t.Errorf("Button = %d, want %d", m.Button(), MouseButtonRight)
	}
	if !m.CtrlKey() {
		t.Errorf("CtrlKey should be true")
	}
	if !m.Bubbles() {
		t.Errorf("Bubbles should be true from init")
	}
}

// TestKeyboardEventFields covers KeyboardEvent construction and accessors.
func TestKeyboardEventFields(t *testing.T) {
	k := NewKeyboardEvent("keydown", true, true, false)
	k.initKeyboardEvent("keydown", true, true, "a", "KeyA", DOMKeyLocationStandard, true, false, true, false)
	if k.Key() != "a" {
		t.Errorf("Key = %q, want %q", k.Key(), "a")
	}
	if k.Code() != "KeyA" {
		t.Errorf("Code = %q, want %q", k.Code(), "KeyA")
	}
	if k.Location() != DOMKeyLocationStandard {
		t.Errorf("Location = %d, want %d", k.Location(), DOMKeyLocationStandard)
	}
	if !k.CtrlKey() {
		t.Errorf("CtrlKey should be true")
	}
	if !k.ShiftKey() {
		t.Errorf("ShiftKey should be true")
	}
	if !k.GetModifierState("Control") {
		t.Errorf("GetModifierState(Control) should be true")
	}
}

// TestKeyboardEventFromInit covers the init-dictionary constructor.
func TestKeyboardEventFromInit(t *testing.T) {
	k := NewKeyboardEventFromInit("keyup", KeyboardEventInit{
		EventInit: EventInit{Bubbles: true},
		Key:       "Enter",
		Code:      "Enter",
		Location:  DOMKeyLocationNumpad,
		Repeat:     true,
	})
	if k.Key() != "Enter" {
		t.Errorf("Key = %q, want %q", k.Key(), "Enter")
	}
	if k.Location() != DOMKeyLocationNumpad {
		t.Errorf("Location = %d, want %d", k.Location(), DOMKeyLocationNumpad)
	}
	if !k.Repeat() {
		t.Errorf("Repeat should be true")
	}
}

// TestWheelEventFields covers WheelEvent construction and accessors.
func TestWheelEventFields(t *testing.T) {
	w := NewWheelEvent("wheel", true, true)
	w.initWheelEvent("wheel", true, true, 0, 120, 0, DOMDeltaPixel)
	if w.DeltaX() != 0 {
		t.Errorf("DeltaX = %v, want 0", w.DeltaX())
	}
	if w.DeltaY() != 120 {
		t.Errorf("DeltaY = %v, want 120", w.DeltaY())
	}
	if w.DeltaMode() != DOMDeltaPixel {
		t.Errorf("DeltaMode = %d, want %d", w.DeltaMode(), DOMDeltaPixel)
	}
	// WheelEvent inherits from MouseEvent so it should report mouse-button defaults.
	if w.Button() != 0 {
		t.Errorf("WheelEvent.Button = %d, want 0 (inherited default)", w.Button())
	}
	// Legacy wheelDelta should be -120 * TickMultiplier for deltaY=120.
	if got := w.WheelDeltaY(); got != -120*TickMultiplier {
		t.Errorf("WheelDeltaY = %d, want %d", got, -120*TickMultiplier)
	}
}

// TestWheelEventFromInit covers the init-dictionary constructor.
func TestWheelEventFromInit(t *testing.T) {
	w := NewWheelEventFromInit("wheel", WheelEventInit{
		MouseEventInit: MouseEventInit{
			EventInit: EventInit{Bubbles: true},
			ClientX:   5, ClientY: 10,
		},
		DeltaX:    10,
		DeltaY:    20,
		DeltaMode: DOMDeltaLine,
	})
	if w.DeltaX() != 10 || w.DeltaY() != 20 {
		t.Errorf("Delta = (%v, %v), want (10, 20)", w.DeltaX(), w.DeltaY())
	}
	if w.DeltaMode() != DOMDeltaLine {
		t.Errorf("DeltaMode = %d, want %d", w.DeltaMode(), DOMDeltaLine)
	}
	// Inherited MouseEvent fields.
	if w.ClientX() != 5 || w.ClientY() != 10 {
		t.Errorf("Client = (%v, %v), want (5, 10)", w.ClientX(), w.ClientY())
	}
}

// TestFocusEventFields covers FocusEvent construction and accessors.
func TestFocusEventFields(t *testing.T) {
	d := NewDocument()
	target := d.CreateElement("input")
	f := NewFocusEvent("focus", false, false)
	if f.Type() != "focus" {
		t.Errorf("Type = %q, want %q", f.Type(), "focus")
	}
	f.SetRelatedTarget(target)
	if f.RelatedTarget() != target {
		t.Errorf("RelatedTarget = %v, want target", f.RelatedTarget())
	}
}

// TestFocusEventFromInit covers the init-dictionary constructor.
func TestFocusEventFromInit(t *testing.T) {
	d := NewDocument()
	target := d.CreateElement("input")
	f := NewFocusEventFromInit("blur", FocusEventInit{
		EventInit:     EventInit{Bubbles: false, Cancelable: false},
		RelatedTarget: target,
	})
	if f.Type() != "blur" {
		t.Errorf("Type = %q, want %q", f.Type(), "blur")
	}
	if f.RelatedTarget() != target {
		t.Errorf("RelatedTarget = %v, want target", f.RelatedTarget())
	}
}

// TestRemoveAllEventListeners verifies that all listeners are cleared.
func TestRemoveAllEventListeners(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	calls := 0
	el.AddEventListener("a", EventListenerFunc(func(Event) { calls++ }))
	el.AddEventListener("b", EventListenerFunc(func(Event) { calls++ }))
	el.RemoveAllEventListeners()
	_ = el.DispatchEvent(NewEvent("a", false, false, false))
	_ = el.DispatchEvent(NewEvent("b", false, false, false))
	if calls != 0 {
		t.Errorf("listeners called %d times after RemoveAll, want 0", calls)
	}
}

// TestEvent_IsTrusted verifies that user-created events have isTrusted=false and that
// the trusted flag is not modified during dispatch.
func TestEvent_IsTrusted(t *testing.T) {
	// NewEvent creates an untrusted event.
	ev := NewEvent("click", false, false, false)
	if ev.IsTrusted() {
		t.Errorf("NewEvent should not be trusted")
	}

	// NewMouseEvent creates a trusted event (mirroring browser-constructed events).
	m := NewMouseEvent("click", false, false, false)
	if !m.IsTrusted() {
		t.Errorf("NewMouseEvent should be trusted")
	}

	// NewEventFromInit creates an untrusted event.
	ev2 := NewEventFromInit("test", EventInit{Bubbles: true, Cancelable: true})
	if ev2.IsTrusted() {
		t.Errorf("NewEventFromInit should not be trusted")
	}

	// NewCustomEvent creates an untrusted event.
	ce := NewCustomEvent("app-event", nil)
	if ce.IsTrusted() {
		t.Errorf("NewCustomEvent should not be trusted")
	}

	// DispatchEvent should not alter the isTrusted flag.
	d := NewDocument()
	el := d.CreateElement("div")
	_ = d.AppendChild(el)

	var trustedDuringDispatch bool
	el.AddEventListener("trust-test", EventListenerFunc(func(e Event) {
		trustedDuringDispatch = e.IsTrusted()
	}))

	untrusted := NewEvent("trust-test", true, false, false)
	_ = el.DispatchEvent(untrusted)
	if trustedDuringDispatch {
		t.Errorf("untrusted event should remain untrusted during dispatch")
	}
	if untrusted.IsTrusted() {
		t.Errorf("untrusted event should remain untrusted after dispatch")
	}
}

// TestEvent_ComposedFlag verifies the Composed() flag on different event types.
func TestEvent_ComposedFlag(t *testing.T) {
	// Default composed=false.
	ev := NewEvent("test", true, true, false)
	if ev.Composed() {
		t.Errorf("NewEvent with composed=false should return false")
	}

	// Explicit composed=true.
	ev2 := NewEvent("test", true, true, true)
	if !ev2.Composed() {
		t.Errorf("NewEvent with composed=true should return true")
	}

	// MouseEvent defaults to composed=true (mirroring WebKit behavior).
	m := NewMouseEvent("click", true, true, true)
	if !m.Composed() {
		t.Errorf("NewMouseEvent should have composed=true")
	}

	// FocusEvent defaults to composed=false.
	f := NewFocusEvent("focus", false, false)
	if f.Composed() {
		t.Errorf("NewFocusEvent should have composed=false by default")
	}

	// CustomEvent with composed flag in init.
	ce := NewCustomEvent("test", map[string]interface{}{"composed": true})
	if !ce.Composed() {
		t.Errorf("CustomEvent with composed=true should return true")
	}

	ce2 := NewCustomEvent("test", map[string]interface{}{"composed": false})
	if ce2.Composed() {
		t.Errorf("CustomEvent with composed=false should return false")
	}
}

// TestEvent_PropagationChain verifies the full capture→target→bubble propagation
// order, including StopPropagation and StopImmediatePropagation effects.
//
// Note: composedPath is a known limitation of this port (intentionally omitted per
// event.go comments); no tests are written for it.
func TestEvent_PropagationChain(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	mid := d.CreateElement("mid")
	_ = root.AppendChild(mid)
	leaf := d.CreateElement("leaf")
	_ = mid.AppendChild(leaf)

	// Record the full propagation order with listener name and phase.
	var order []string
	type phaseInfo struct {
		name  string
		phase EventPhase
	}
	var phases []phaseInfo

	addBoth := func(n Node, name string) {
		n.AddEventListener("test", EventListenerFunc(func(e Event) {
			order = append(order, "cap-"+name)
			phases = append(phases, phaseInfo{"cap-" + name, e.EventPhase()})
		}), true)
		n.AddEventListener("test", EventListenerFunc(func(e Event) {
			order = append(order, "bub-"+name)
			phases = append(phases, phaseInfo{"bub-" + name, e.EventPhase()})
		}))
	}
	addBoth(root, "root")
	addBoth(mid, "mid")
	addBoth(leaf, "leaf")

	_ = leaf.DispatchEvent(NewEvent("test", true, false, false))

	// Expected: capture root→mid, then target leaf (cap+cap, bub+bub),
	// then bubble mid→root.
	want := []string{"cap-root", "cap-mid", "cap-leaf", "bub-leaf", "bub-mid", "bub-root"}
	if len(order) != len(want) {
		t.Fatalf("propagation order len = %d, want %d\ngot:  %v\nwant: %v",
			len(order), len(want), order, want)
	}
	for i, name := range want {
		if order[i] != name {
			t.Fatalf("order[%d] = %q, want %q\nfull: %v", i, order[i], name, order)
		}
	}

	// Verify phases.
	for _, p := range phases {
		switch {
		case len(p.name) > 4 && p.name[:4] == "cap-":
			if p.phase != EventCapturingPhase && p.phase != EventAtTarget {
				t.Errorf("%s phase = %d, want %d (capture or at-target)",
					p.name, p.phase, EventCapturingPhase)
			}
		case len(p.name) > 4 && p.name[:4] == "bub-":
			if p.phase != EventBubblingPhase && p.phase != EventAtTarget {
				t.Errorf("%s phase = %d, want %d (bubble or at-target)",
					p.name, p.phase, EventBubblingPhase)
			}
		}
	}

	// --- Sub-test: StopPropagation in capture phase ---
	// When StopPropagation is called in a capture listener, subsequent capture
	// listeners, the target phase, and the bubble phase are all skipped.
	t.Run("StopPropagationInCapture", func(t *testing.T) {
		d2 := NewDocument()
		r := d2.CreateElement("r")
		_ = d2.AppendChild(r)
		m := d2.CreateElement("m")
		_ = r.AppendChild(m)
		l := d2.CreateElement("l")
		_ = m.AppendChild(l)

		var capOrder []string
		r.AddEventListener("s", EventListenerFunc(func(e Event) {
			capOrder = append(capOrder, "cap-r")
		}), true)
		m.AddEventListener("s", EventListenerFunc(func(e Event) {
			capOrder = append(capOrder, "cap-m")
			e.StopPropagation()
		}), true)
		// Target and bubble listeners should NOT fire.
		l.AddEventListener("s", EventListenerFunc(func(Event) {
			capOrder = append(capOrder, "tgt-l")
		}))
		r.AddEventListener("s", EventListenerFunc(func(Event) {
			capOrder = append(capOrder, "bub-r")
		}))

		_ = l.DispatchEvent(NewEvent("s", true, false, false))
		wantCap := []string{"cap-r", "cap-m"}
		if len(capOrder) != len(wantCap) {
			t.Fatalf("order len = %d, want %d\ngot:  %v\nwant: %v",
				len(capOrder), len(wantCap), capOrder, wantCap)
		}
		for i, name := range wantCap {
			if capOrder[i] != name {
				t.Errorf("order[%d] = %q, want %q", i, capOrder[i], name)
			}
		}
	})

	// --- Sub-test: StopImmediatePropagation in capture ---
	// Stops further listeners on the current target AND prevents remaining
	// targets from being visited.
	t.Run("StopImmediatePropagationInCapture", func(t *testing.T) {
		d3 := NewDocument()
		r := d3.CreateElement("r")
		_ = d3.AppendChild(r)
		m := d3.CreateElement("m")
		_ = r.AppendChild(m)
		l := d3.CreateElement("l")
		_ = m.AppendChild(l)

		var order3 []string
		r.AddEventListener("s", EventListenerFunc(func(Event) {
			order3 = append(order3, "cap-r")
		}), true)
		m.AddEventListener("s", EventListenerFunc(func(e Event) {
			order3 = append(order3, "cap-m-1")
			e.StopImmediatePropagation()
		}), true)
		// This second listener on mid should NOT fire.
		m.AddEventListener("s", EventListenerFunc(func(Event) {
			order3 = append(order3, "cap-m-2")
		}), true)
		l.AddEventListener("s", EventListenerFunc(func(Event) {
			order3 = append(order3, "tgt-l")
		}))

		_ = l.DispatchEvent(NewEvent("s", true, false, false))
		want3 := []string{"cap-r", "cap-m-1"}
		if len(order3) != len(want3) {
			t.Fatalf("order len = %d, want %d\ngot:  %v\nwant: %v",
				len(order3), len(want3), order3, want3)
		}
		for i, name := range want3 {
			if order3[i] != name {
				t.Errorf("order[%d] = %q, want %q", i, order3[i], name)
			}
		}
	})

	// --- Sub-test: StopPropagation in bubble phase ---
	// Upstream bubble listeners (towards the root) are skipped.
	t.Run("StopPropagationInBubble", func(t *testing.T) {
		d4 := NewDocument()
		r := d4.CreateElement("r")
		_ = d4.AppendChild(r)
		m := d4.CreateElement("m")
		_ = r.AppendChild(m)
		l := d4.CreateElement("l")
		_ = m.AppendChild(l)

		var order4 []string
		l.AddEventListener("s", EventListenerFunc(func(Event) {
			order4 = append(order4, "tgt-l")
		}))
		m.AddEventListener("s", EventListenerFunc(func(e Event) {
			order4 = append(order4, "bub-m")
			e.StopPropagation()
		}))
		r.AddEventListener("s", EventListenerFunc(func(Event) {
			order4 = append(order4, "bub-r") // should NOT fire
		}))

		_ = l.DispatchEvent(NewEvent("s", true, false, false))
		want4 := []string{"tgt-l", "bub-m"}
		if len(order4) != len(want4) {
			t.Fatalf("order len = %d, want %d\ngot:  %v\nwant: %v",
				len(order4), len(want4), order4, want4)
		}
		for i, name := range want4 {
			if order4[i] != name {
				t.Errorf("order[%d] = %q, want %q", i, order4[i], name)
			}
		}
	})

	// --- Sub-test: StopImmediatePropagation in bubble phase ---
	// Stops further listeners on the current target AND prevents upstream
	// bubble listeners from firing.
	t.Run("StopImmediatePropagationInBubble", func(t *testing.T) {
		d5 := NewDocument()
		r := d5.CreateElement("r")
		_ = d5.AppendChild(r)
		l := d5.CreateElement("l")
		_ = r.AppendChild(l)

		var order5 []string
		l.AddEventListener("s", EventListenerFunc(func(e Event) {
			order5 = append(order5, "tgt-1")
		}))
		l.AddEventListener("s", EventListenerFunc(func(e Event) {
			order5 = append(order5, "tgt-2")
			e.StopImmediatePropagation()
		}))
		// This third listener on target should NOT fire.
		l.AddEventListener("s", EventListenerFunc(func(Event) {
			order5 = append(order5, "tgt-3")
		}))
		r.AddEventListener("s", EventListenerFunc(func(Event) {
			order5 = append(order5, "bub-r") // should NOT fire
		}))

		_ = l.DispatchEvent(NewEvent("s", true, false, false))
		want5 := []string{"tgt-1", "tgt-2"}
		if len(order5) != len(want5) {
			t.Fatalf("order len = %d, want %d\ngot:  %v\nwant: %v",
				len(order5), len(want5), order5, want5)
		}
		for i, name := range want5 {
			if order5[i] != name {
				t.Errorf("order[%d] = %q, want %q", i, order5[i], name)
			}
		}
	})
}

// TestEvent_ComposedPath verifies that ComposedPath returns the full propagation path
// (target → parents → document) when dispatched, and nil when no target is set.
func TestEvent_ComposedPath(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	leaf := d.CreateElement("leaf")
	_ = root.AppendChild(leaf)

	// Event with target set via dispatch: path is [leaf, root, document].
	var captured []EventTarget
	leaf.AddEventListener("test", EventListenerFunc(func(e Event) {
		captured = e.ComposedPath()
	}), false)
	_ = leaf.DispatchEvent(NewEvent("test", false, false, false))
	if len(captured) != 3 {
		t.Fatalf("ComposedPath() length = %d, want 3 ([leaf, root, document])", len(captured))
	}
	if captured[0] != leaf {
		t.Errorf("ComposedPath()[0] is not the target element")
	}
	if captured[1] != root {
		t.Errorf("ComposedPath()[1] is not the parent element")
	}

	// Event with no target.
	e := NewEvent("not-dispatched", false, false, false)
	if p := e.ComposedPath(); p != nil {
		t.Fatalf("ComposedPath() = %v, want nil for non-dispatched event", p)
	}
}

// TestEvent_ComposedPathShadow verifies that a composed event's path crosses the
// shadow boundary (shadow-root child → host), while a non-composed event's path
// stops at the shadow root (does not leak into the host's light-DOM tree).
func TestEvent_ComposedPathShadow(t *testing.T) {
	d := NewDocument()
	host := d.CreateElement("xwidget")
	_ = d.AppendChild(host)
	sr, _ := host.AttachShadow("open")
	btn := d.CreateElement("button")
	_ = sr.AppendChild(btn)

	// composed=true: path is [btn, host, document].
	var composedPath []EventTarget
	btn.AddEventListener("test", EventListenerFunc(func(e Event) {
		composedPath = e.ComposedPath()
	}), false)
	_ = btn.DispatchEvent(NewEvent("test", false, false, true))
	if len(composedPath) != 3 {
		t.Fatalf("composed ComposedPath() length = %d, want 3 ([btn, host, document])", len(composedPath))
	}
	if composedPath[1] != host {
		t.Errorf("composed ComposedPath()[1] = %v, want host", composedPath[1])
	}

	// composed=false: path stops at the shadow root — [btn].
	var nonComposedPath []EventTarget
	btn.AddEventListener("test2", EventListenerFunc(func(e Event) {
		nonComposedPath = e.ComposedPath()
	}), false)
	_ = btn.DispatchEvent(NewEvent("test2", false, false, false))
	if len(nonComposedPath) != 1 {
		t.Fatalf("non-composed ComposedPath() length = %d, want 1 ([btn])", len(nonComposedPath))
	}
}

// TestEvent_ComposedPathSlot verifies that a composed event dispatched on a
// slot-assigned light-DOM node includes the <slot> node in its path (flattened tree:
// assigned node → slot → host → document).
func TestEvent_ComposedPathSlot(t *testing.T) {
	d := NewDocument()
	host := d.CreateElement("xwidget")
	_ = d.AppendChild(host)
	item := d.CreateElement("span")
	_ = host.AppendChild(item)
	sr, _ := host.AttachShadow("open")
	slot := d.CreateElement("slot")
	_ = sr.AppendChild(slot)

	if got := item.AssignedSlot(); got != slot {
		t.Fatalf("AssignedSlot() = %v, want slot", got)
	}

	var composedPath []EventTarget
	item.AddEventListener("test", EventListenerFunc(func(e Event) {
		composedPath = e.ComposedPath()
	}), false)
	_ = item.DispatchEvent(NewEvent("test", false, false, true))
	if len(composedPath) != 4 {
		t.Fatalf("composed ComposedPath() length = %d, want 4 ([item, slot, host, document])", len(composedPath))
	}
	if composedPath[1] != slot {
		t.Errorf("ComposedPath()[1] = %v, want slot", composedPath[1])
	}
	if composedPath[2] != host {
		t.Errorf("ComposedPath()[2] = %v, want host", composedPath[2])
	}
}

// TestEvent_Retargeting verifies DOM §2.8 event retargeting: a listener on a shadow
// host (or its light-DOM ancestor) observes the host as the event target, while a
// listener inside the shadow tree observes the real target.
func TestEvent_Retargeting(t *testing.T) {
	d := NewDocument()
	host := d.CreateElement("xwidget")
	_ = d.AppendChild(host)
	sr, _ := host.AttachShadow("open")
	btn := d.CreateElement("button")
	_ = sr.AppendChild(btn)

	var innerTarget, hostTarget, docTarget EventTarget
	btn.AddEventListener("test", EventListenerFunc(func(e Event) {
		innerTarget = e.Target()
	}), false)
	host.AddEventListener("test", EventListenerFunc(func(e Event) {
		hostTarget = e.Target()
	}), false)
	d.AddEventListener("test", EventListenerFunc(func(e Event) {
		docTarget = e.Target()
	}), false)

	ev := NewEvent("test", true, false, true)
	_ = btn.DispatchEvent(ev)

	if innerTarget != btn {
		t.Errorf("inner listener target = %v, want btn", innerTarget)
	}
	if hostTarget != host {
		t.Errorf("host listener target = %v, want host (retargeted)", hostTarget)
	}
	if docTarget != host {
		t.Errorf("document listener target = %v, want host (retargeted)", docTarget)
	}
	// After dispatch the target attribute is restored to the real target.
	if ev.Target() != btn {
		t.Errorf("post-dispatch target = %v, want btn", ev.Target())
	}
}

// TestEvent_RelatedTargetRetargeting verifies DOM §2.8 last paragraph: a non-null
// relatedTarget (MouseEvent/FocusEvent) is retargeted the same way as the target. A
// listener on the shadow host observes the host as relatedTarget, while a listener
// inside the shadow tree observes the real related target.
func TestEvent_RelatedTargetRetargeting(t *testing.T) {
	d := NewDocument()
	host := d.CreateElement("xwidget")
	_ = d.AppendChild(host)
	sr, _ := host.AttachShadow("open")
	a := d.CreateElement("button")
	_ = sr.AppendChild(a)
	b := d.CreateElement("button")
	_ = sr.AppendChild(b)

	var innerRT, hostRT, docRT EventTarget
	a.AddEventListener("mouseover", EventListenerFunc(func(e Event) {
		innerRT = e.(*MouseEvent).RelatedTarget()
	}), false)
	host.AddEventListener("mouseover", EventListenerFunc(func(e Event) {
		hostRT = e.(*MouseEvent).RelatedTarget()
	}), false)
	d.AddEventListener("mouseover", EventListenerFunc(func(e Event) {
		docRT = e.(*MouseEvent).RelatedTarget()
	}), false)

	ev := NewMouseEventFromInit(EventMouseOver, MouseEventInit{
		EventInit:     EventInit{Bubbles: true, Composed: true},
		RelatedTarget: b,
	})
	_ = a.DispatchEvent(ev)

	if innerRT != b {
		t.Errorf("inner listener relatedTarget = %v, want b (real)", innerRT)
	}
	if hostRT != host {
		t.Errorf("host listener relatedTarget = %v, want host (retargeted)", hostRT)
	}
	if docRT != host {
		t.Errorf("document listener relatedTarget = %v, want host (retargeted)", docRT)
	}
	// After dispatch the relatedTarget is restored to the real target.
	if ev.RelatedTarget() != b {
		t.Errorf("post-dispatch relatedTarget = %v, want b", ev.RelatedTarget())
	}
}
