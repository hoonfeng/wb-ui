// Package event provides cross-platform event type definitions for the wb-ui
// platform layer. It mirrors the role of WebCore/platform/PlatformEvent.h
// and related event types, providing a unified event representation that
// platform/window backends (GLFW, X11, Cocoa) normalize into.
//
// The event types defined here are consumed by the app/ host and the
// DOM EventTarget dispatch path for user interaction events.
//
// Usage:
//
//	ev := event.Event{
//	    Type: event.MouseDown,
//	    X:    120, Y: 340,
//	    Button: 0,
//	}
package event

// EventType enumerates all platform-independent input event types.
type EventType int

const (
	// Mouse events
	MouseDown   EventType = iota
	MouseUp
	MouseMove
	MouseDrag

	// Keyboard events
	KeyDown
	KeyUp

	// Scroll / wheel events
	Wheel

	// Touch events
	TouchBegin
	TouchMove
	TouchEnd
	TouchCancel

	// Window events
	WindowResize
	WindowClose
	WindowFocusIn
	WindowFocusOut

	// Drag & Drop events
	DragEnter   // dragged content entered the window
	DragOver    // dragged content moved within the window
	DragLeave   // dragged content left the window
	Drop        // content was dropped (files or text)
)

// Modifier is a bitmask of modifier keys pressed during an event.
type Modifier uint32

const (
	ModShift Modifier = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
	ModCapsLock
)

// Touch represents a single touch point on a touch screen or trackpad.
type Touch struct {
	ID        int     // unique touch identifier
	X, Y      float64 // position in CSS (logical) pixels
	Force     float64 // 0.0–1.0 pressure sensitivity
	RadiusX   float64 // touch ellipse semi-major axis
	RadiusY   float64 // touch ellipse semi-minor axis
}

// Event is a unified platform input event. All platform window backends
// (GLFW, X11, Cocoa) normalize their native events into this type before
// dispatching to the wb-ui rendering and event system.
type Event struct {
	Type      EventType
	Timestamp float64 // seconds since an unspecified epoch (monotonic)

	// Mouse / touch position
	X, Y    float64
	Button  int    // 0=left, 1=right, 2=middle; -1=none
	Action  int    // 0=release, 1=press

	// Keyboard
	KeyCode   int      // platform-specific key code (GLFW key, X11 keysym, macOS keyCode)
	Modifiers Modifier // bitmask of active modifiers
	Char      rune     // Unicode character for text input

	// Touch (multiple simultaneous touches)
	Touches []Touch

	// Scroll / wheel
	DeltaX, DeltaY float64 // scroll deltas in pixels (or lines * lineHeight)
	PreciseDelta   bool    // true for trackpad smooth scrolling

	// Window resize
	Width  int // new width in CSS pixels
	Height int // new height in CSS pixels

	// Drag & Drop
	DropFiles []string // file paths dropped on the window
	DropText  string   // plain text dropped on the window
}

// IsMouse reports whether the event is a mouse button event.
func (e Event) IsMouse() bool {
	switch e.Type {
	case MouseDown, MouseUp:
		return true
	}
	return false
}

// IsKeyboard reports whether the event is a keyboard event.
func (e Event) IsKeyboard() bool {
	switch e.Type {
	case KeyDown, KeyUp:
		return true
	}
	return false
}

// IsTouch reports whether the event is a touch event.
func (e Event) IsTouch() bool {
	switch e.Type {
	case TouchBegin, TouchMove, TouchEnd, TouchCancel:
		return true
	}
	return false
}

// IsWindow reports whether the event is a window lifecycle event.
func (e Event) IsWindow() bool {
	switch e.Type {
	case WindowResize, WindowClose, WindowFocusIn, WindowFocusOut:
		return true
	}
	return false
}

// Pressed returns true if the event is a press (action == 1).
func (e Event) Pressed() bool { return e.Action == 1 }

// Released returns true if the event is a release (action == 0).
func (e Event) Released() bool { return e.Action == 0 }

// ModifierNames returns the human-readable names of active modifiers.
func (m Modifier) ModifierNames() []string {
	var names []string
	if m&ModShift != 0 {
		names = append(names, "Shift")
	}
	if m&ModCtrl != 0 {
		names = append(names, "Ctrl")
	}
	if m&ModAlt != 0 {
		names = append(names, "Alt")
	}
	if m&ModMeta != 0 {
		names = append(names, "Meta")
	}
	if m&ModCapsLock != 0 {
		names = append(names, "CapsLock")
	}
	return names
}
