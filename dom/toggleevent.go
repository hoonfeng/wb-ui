// Translation of: Source/WebCore/dom/ToggleEvent.h
//                  Source/WebCore/dom/ToggleEvent.cpp
//                  Source/WebCore/dom/ToggleEvent.idl
// Completeness: 90%
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
//
// `source` (the element that initiated the toggle) is part of the IDL; see
// Source() for which transitions can produce a non-null value in this port.

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
	Source   *Element
}

// ToggleEvent is the Go translation of WebCore::ToggleEvent.
type ToggleEvent struct {
	baseEvent
	oldState string
	newState string
	source   *Element
}

// NewToggleEvent constructs a ToggleEvent, mirroring
// ToggleEvent::create(type, canBubble, cancelable, oldState, newState, source).
// source is the element that initiated the toggle, or nil (the common case —
// see Source()).
func NewToggleEvent(typ string, canBubble, cancelable bool, oldState, newState string, source *Element) *ToggleEvent {
	return &ToggleEvent{
		baseEvent: newBaseEvent(typ, canBubble, cancelable, false, true),
		oldState:  oldState,
		newState:  newState,
		source:    source,
	}
}

// NewToggleEventFromInit constructs a ToggleEvent from a ToggleEventInit
// dictionary, mirroring ToggleEvent::create(type, init).
//
// The dictionary members are copied verbatim: the IDL declares
// `DOMString oldState = ""` / `DOMString newState = ""`, so a dictionary that
// omits them yields the *empty string*, not "closed" — `new
// ToggleEvent("toggle").oldState === ""` in a browser. Normalising here would
// make script-constructed events disagree with the IDL default.
func NewToggleEventFromInit(typ string, init ToggleEventInit) *ToggleEvent {
	return &ToggleEvent{
		baseEvent: newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		oldState:  init.OldState,
		newState:  init.NewState,
		source:    init.Source,
	}
}

// OldState returns the state the element was in before the transition
// ("open" or "closed"), mirroring ToggleEvent::oldState().
func (e *ToggleEvent) OldState() string { return e.oldState }

// NewState returns the state the element moves to ("open" or "closed"),
// mirroring ToggleEvent::newState().
func (e *ToggleEvent) NewState() string { return e.newState }

// Source returns the element that initiated the state transition, or nil when
// there is no such element — mirroring ToggleEvent::source() (IDL type
// `Element?`, so nil is exposed to script as null, never undefined).
//
// Which transitions can have a source at all (HTML spec):
//   - <dialog>: every close path passes null — close() and requestClose() call
//     "close the dialog with result and null", form submission with
//     method=dialog calls "close the dialog subject with result and null", and
//     the close watcher reads the dialog's "request close source element",
//     which only requestClose() ever writes (with null). show()/showModal()
//     have no caller modelling either.
//   - <details>: the details toggle task initialises only oldState/newState.
//   - popover: the invoker (popovertarget / command element) is the one case
//     where source is non-null — showPopover()/hidePopover()/togglePopover()
//     called by script pass no source, while clicking a popover invoker does
//     (see the popover package). For every other transition the field is null,
//     so that `e.source === null` (and MDN's `event.source === undefined`
//     feature detection) still behaves like a browser.
func (e *ToggleEvent) Source() *Element { return e.source }
