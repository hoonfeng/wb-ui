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
