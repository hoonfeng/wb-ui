// Tests for the window event types and constants.
// These tests are build-tag agnostic and run on any platform.

package window

import (
	"testing"
)

func TestEventTypeValues(t *testing.T) {
	tests := []struct {
		typ  EventType
		want int
	}{
		{EventMouseButton, 0},
		{EventChar, 1},
		{EventCursorMove, 2},
		{EventKey, 3},
		{EventResize, 4},
		{EventScroll, 5},
		{EventDrop, 6},
		{EventTouch, 7},
	}
	for _, tt := range tests {
		if int(tt.typ) != tt.want {
			t.Errorf("EventType(%d) = %d, want %d", tt.typ, int(tt.typ), tt.want)
		}
	}
}

func TestEventStructDefaults(t *testing.T) {
	e := Event{}
	if e.Type != 0 {
		t.Errorf("default Event.Type = %d, want 0", e.Type)
	}
	if e.DropFiles != nil {
		t.Errorf("default Event.DropFiles = %v, want nil", e.DropFiles)
	}
}

func TestEventDropFiles(t *testing.T) {
	e := Event{
		Type:      EventDrop,
		DropFiles: []string{"file1.txt", "file2.txt"},
	}
	if e.Type != EventDrop {
		t.Errorf("Event.Type = %d, want EventDrop(%d)", e.Type, EventDrop)
	}
	if len(e.DropFiles) != 2 {
		t.Errorf("len(DropFiles) = %d, want 2", len(e.DropFiles))
	}
	if e.DropFiles[0] != "file1.txt" {
		t.Errorf("DropFiles[0] = %q, want file1.txt", e.DropFiles[0])
	}
}

func TestEventResize(t *testing.T) {
	e := Event{
		Type:   EventResize,
		Width:  1024,
		Height: 768,
	}
	if e.Width != 1024 || e.Height != 768 {
		t.Errorf("Resize event = %dx%d, want 1024x768", e.Width, e.Height)
	}
}

func TestEventMouse(t *testing.T) {
	e := Event{
		Type:   EventMouseButton,
		X:      100.5,
		Y:      200.5,
		Button: 0, // left button
		Action: 1, // press
	}
	if e.X != 100.5 || e.Y != 200.5 {
		t.Errorf("Mouse event position = (%v,%v), want (100.5,200.5)", e.X, e.Y)
	}
}

func TestEventScroll(t *testing.T) {
	e := Event{
		Type:    EventScroll,
		ScrollY: -3.0,
	}
	if e.ScrollY != -3.0 {
		t.Errorf("ScrollY = %v, want -3.0", e.ScrollY)
	}
}

func TestEventTouch(t *testing.T) {
	e := Event{
		Type:        EventTouch,
		TouchX:      300.0,
		TouchY:      400.0,
		TouchID:     1,
		TouchAction: 2, // move
	}
	if e.TouchID != 1 {
		t.Errorf("TouchID = %d, want 1", e.TouchID)
	}
	if e.TouchAction != 2 {
		t.Errorf("TouchAction = %d, want 2", e.TouchAction)
	}
}

func TestEventKey(t *testing.T) {
	e := Event{
		Type:   EventKey,
		Key:    65, // A
		Mods:   1,  // shift
		Action: 1,  // press
	}
	if e.Key != 65 {
		t.Errorf("Key = %d, want 65 (A)", e.Key)
	}
	if e.Mods != 1 {
		t.Errorf("Mods = %d, want 1 (shift)", e.Mods)
	}
}
