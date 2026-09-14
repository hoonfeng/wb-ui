// Tests for the IME event types and constants.
// These tests are build-tag agnostic and run on any platform.

package ime

import (
	"testing"
)

func TestEventKindValues(t *testing.T) {
	tests := []struct {
		kind EventKind
		want int
	}{
		{EventCompositionUpdate, 0},
		{EventCharInput, 1},
		{EventCompositionEnd, 2},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("EventKind(%d) = %d, want %d", tt.kind, int(tt.kind), tt.want)
		}
	}
}

func TestIMECompositionEvent(t *testing.T) {
	e := Event{
		Kind:        EventCompositionUpdate,
		Composition: "你好",
		CursorPos:   2,
	}
	if e.Kind != EventCompositionUpdate {
		t.Errorf("Kind = %d, want EventCompositionUpdate(0)", e.Kind)
	}
	if e.Composition != "你好" {
		t.Errorf("Composition = %q, want 你好", e.Composition)
	}
	if e.CursorPos != 2 {
		t.Errorf("CursorPos = %d, want 2", e.CursorPos)
	}
}

func TestIMECharEvent(t *testing.T) {
	e := Event{
		Kind: EventCharInput,
		Char: 'a',
	}
	if e.Kind != EventCharInput {
		t.Errorf("Kind = %d, want EventCharInput(1)", e.Kind)
	}
	if e.Char != 'a' {
		t.Errorf("Char = %c, want 'a'", e.Char)
	}
}

func TestIMECompositionEndEvent(t *testing.T) {
	e := Event{
		Kind:        EventCompositionEnd,
		Composition: "confirmed",
	}
	if e.Kind != EventCompositionEnd {
		t.Errorf("Kind = %d, want EventCompositionEnd(2)", e.Kind)
	}
	if e.Composition != "confirmed" {
		t.Errorf("Composition = %q, want confirmed", e.Composition)
	}
}

func TestIMEEventDefaults(t *testing.T) {
	e := Event{}
	if e.Kind != 0 {
		t.Errorf("default Kind = %d, want 0", e.Kind)
	}
	if e.Composition != "" {
		t.Errorf("default Composition = %q, want empty", e.Composition)
	}
	if e.Char != 0 {
		t.Errorf("default Char = %c, want 0", e.Char)
	}
}

func TestIMEUnicodeChar(t *testing.T) {
	e := Event{
		Kind: EventCharInput,
		Char: '中',
	}
	if e.Char != '中' {
		t.Errorf("Char = %c, want '中'", e.Char)
	}
}

// verify Handler interface is satisfied by the concrete platform implementations
func TestHandlerInterfaceCompiles(t *testing.T) {
	// This is a compile-time check that the Handler interface methods are sound.
	var _ Handler = (*noopHandler)(nil)
}

type noopHandler struct{}

func (n *noopHandler) Init(hwnd uintptr)            {}
func (n *noopHandler) PopEvents() []Event           { return nil }
func (n *noopHandler) SetCompositionPos(x, y int32) {}
func (n *noopHandler) SetEnabled(enabled bool)       {}
func (n *noopHandler) IsComposing() bool             { return false }
