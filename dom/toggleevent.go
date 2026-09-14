// Translation of: Source/WebCore/dom/ToggleEvent.h
//                  Source/WebCore/dom/ToggleEvent.cpp
//                  Source/WebCore/dom/ToggleEvent.idl
// Completeness: 80%
//
// ToggleEvent is fired on state transitions of elements that have an
// "open/closed" state machine — <details> and <dialog> (HTML §4.11.4 /
// §4.11.6). Before this existed the port dispatched a plain Event for
// "toggle", so `e.newState` / `e.oldState` (how pages tell "opening" from
// "closing" while sharing one handler) were undefined.
//
// The event is cancelable only for the "beforetoggle" variant: the open/close
// algorithms fire it in a cancelable phase and abort when a listener calls
// preventDefault().

package dom

// ToggleState constants, mirroring the ToggleEvent's oldState/newState values.
const (
	ToggleStateOpen   = "open"
	ToggleStateClosed = "closed"
)

// ToggleEventInit mirrors WebCore::ToggleEvent::Init (the EventInit-derived
// dictionary used by the ToggleEvent(type, init) constructor).
type ToggleEventInit struct {
	EventInit
	OldState string
	NewState string
}

// ToggleEvent is the Go translation of WebCore::ToggleEvent.
type ToggleEvent struct {
	baseEvent
	oldState string
	newState string
}

// NewToggleEvent constructs a ToggleEvent, mirroring
// ToggleEvent::create(type, canBubble, cancelable, oldState, newState).
func NewToggleEvent(typ string, canBubble, cancelable bool, oldState, newState string) *ToggleEvent {
	return &ToggleEvent{
		baseEvent: newBaseEvent(typ, canBubble, cancelable, false, true),
		oldState:  oldState,
		newState:  newState,
	}
}

// NewToggleEventFromInit constructs a ToggleEvent from a ToggleEventInit
// dictionary, mirroring ToggleEvent::create(type, init). An absent state
// defaults to "closed", matching the IDL default.
func NewToggleEventFromInit(typ string, init ToggleEventInit) *ToggleEvent {
	old, new := init.OldState, init.NewState
	if old == "" {
		old = ToggleStateClosed
	}
	if new == "" {
		new = ToggleStateClosed
	}
	return &ToggleEvent{
		baseEvent: newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		oldState:  old,
		newState:  new,
	}
}

// OldState returns the state the element was in before the transition
// ("open" or "closed"), mirroring ToggleEvent::oldState().
func (e *ToggleEvent) OldState() string { return e.oldState }

// NewState returns the state the element moves to ("open" or "closed"),
// mirroring ToggleEvent::newState().
func (e *ToggleEvent) NewState() string { return e.newState }
