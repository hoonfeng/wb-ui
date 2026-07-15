package dom

import "testing"

// TestCustomEventConstructor verifies that NewCustomEvent creates an event with the
// expected type and default flag values.
func TestCustomEventConstructor(t *testing.T) {
	ev := NewCustomEvent("myevent", nil)
	if ev.Type() != "myevent" {
		t.Errorf("Type = %q, want %q", ev.Type(), "myevent")
	}
	if ev.Detail() != nil {
		t.Errorf("Detail = %v, want nil", ev.Detail())
	}
	if ev.Bubbles() {
		t.Errorf("Bubbles should be false by default")
	}
	if ev.Cancelable() {
		t.Errorf("Cancelable should be false by default")
	}
	if ev.Composed() {
		t.Errorf("Composed should be false by default")
	}
	if ev.IsTrusted() {
		t.Errorf("user-created CustomEvent should not be trusted")
	}
}

// TestCustomEventDetail verifies the detail payload storage and retrieval.
func TestCustomEventDetail(t *testing.T) {
	// String detail.
	ev := NewCustomEvent("msg", map[string]interface{}{"detail": "hello"})
	if ev.Detail() != "hello" {
		t.Errorf("Detail = %v, want %q", ev.Detail(), "hello")
	}

	// Integer detail.
	ev2 := NewCustomEvent("count", map[string]interface{}{"detail": 42})
	if ev2.Detail() != 42 {
		t.Errorf("Detail = %v, want 42", ev2.Detail())
	}

	// Struct detail.
	type payload struct{ X, Y int }
	p := payload{X: 10, Y: 20}
	ev3 := NewCustomEvent("move", map[string]interface{}{"detail": p})
	got, ok := ev3.Detail().(payload)
	if !ok || got != p {
		t.Errorf("Detail = %v, want %v", ev3.Detail(), p)
	}

	// nil detail (explicit).
	ev4 := NewCustomEvent("empty", map[string]interface{}{"detail": nil})
	if ev4.Detail() != nil {
		t.Errorf("Detail = %v, want nil", ev4.Detail())
	}
}

// TestCustomEventFlags verifies that bubbles, cancelable and composed flags are
// correctly inherited from the init map.
func TestCustomEventFlags(t *testing.T) {
	ev := NewCustomEvent("test", map[string]interface{}{
		"bubbles":    true,
		"cancelable": true,
		"composed":   true,
	})
	if !ev.Bubbles() {
		t.Errorf("Bubbles should be true")
	}
	if !ev.Cancelable() {
		t.Errorf("Cancelable should be true")
	}
	if !ev.Composed() {
		t.Errorf("Composed should be true")
	}
}

// TestCustomEventDispatch verifies that a CustomEvent can be dispatched through the
// event system and that its custom detail is accessible to listeners.
func TestCustomEventDispatch(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	_ = d.AppendChild(el)

	var capturedDetail interface{}
	el.AddEventListener("custom", EventListenerFunc(func(e Event) {
		if ce, ok := e.(*CustomEvent); ok {
			capturedDetail = ce.Detail()
		}
	}))

	ev := NewCustomEvent("custom", map[string]interface{}{
		"detail": "payload",
	})
	_ = el.DispatchEvent(ev)
	if capturedDetail != "payload" {
		t.Errorf("listener received detail = %v, want %q", capturedDetail, "payload")
	}
}

// TestCustomEventInitCustomEvent verifies the legacy initCustomEvent method.
func TestCustomEventInitCustomEvent(t *testing.T) {
	ev := NewCustomEvent("init", nil)
	ev.initCustomEvent("updated", true, true, 99)
	if ev.Type() != "updated" {
		t.Errorf("Type = %q, want %q", ev.Type(), "updated")
	}
	if !ev.Bubbles() {
		t.Errorf("Bubbles should be true after initCustomEvent")
	}
	if !ev.Cancelable() {
		t.Errorf("Cancelable should be true after initCustomEvent")
	}
	if ev.Detail() != 99 {
		t.Errorf("Detail = %v, want 99", ev.Detail())
	}
}
