// Translation of: Source/WebCore/dom/CompositionEvent.h
//                  Source/WebCore/dom/CompositionEvent.cpp
//                  Source/WebCore/dom/CompositionEvent.idl
// Completeness: 85%
//
// CompositionEvent is the DOM event fired during IME (Input Method Editor)
// composition. It carries the composition string (data) and is dispatched at
// three stages: compositionstart, compositionupdate, and compositionend.
//
// In WebKit, CompositionEvent derives from UIEvent; this port flattens the
// UIEvent layer and embeds baseEvent directly (matching KeyboardEvent's
// approach), carrying only the composition `data` string.

package dom

// CompositionEventInit mirrors WebCore::CompositionEvent::Init (the
// EventInit-derived dictionary used by the CompositionEvent(type, init)
// constructor).
type CompositionEventInit struct {
	EventInit
	Data string
}

// CompositionEvent is the Go translation of WebCore::CompositionEvent. It is
// dispatched during IME composition to notify the page of composition string
// changes. The event type string is one of:
//   - "compositionstart"  : composition began (data = initial composition text)
//   - "compositionupdate"  : composition string changed (data = new text)
//   - "compositionend"     : composition ended (data = final committed text)
type CompositionEvent struct {
	baseEvent
	data string
}

// Composition event type constants, mirroring the event type strings used by
// WebCore. These are the values returned by event.type.
const (
	EventCompositionStart  = "compositionstart"
	EventCompositionUpdate = "compositionupdate"
	EventCompositionEnd    = "compositionend"
)

// NewCompositionEvent constructs a CompositionEvent, mirroring
// CompositionEvent::create(type, view, data). The event bubbles and is
// cancelable and composed, matching the WebKit constructor defaults.
func NewCompositionEvent(typ string, data string) *CompositionEvent {
	return &CompositionEvent{
		baseEvent: newBaseEvent(typ, true, true, true, true),
		data:      data,
	}
}

// NewCompositionEventFromInit constructs a CompositionEvent from an
// CompositionEventInit dictionary, mirroring CompositionEvent::create(type, init).
func NewCompositionEventFromInit(typ string, init CompositionEventInit) *CompositionEvent {
	return &CompositionEvent{
		baseEvent: newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		data:      init.Data,
	}
}

// Data returns the composition string, mirroring CompositionEvent::data().
// For compositionstart/compositionupdate this is the current composition
// text; for compositionend it is the final committed text (possibly empty
// if the composition was cancelled).
func (e *CompositionEvent) Data() string { return e.data }

// initCompositionEvent re-initialises the event, mirroring
// CompositionEvent::initCompositionEvent(). Has no effect if the event is
// currently being dispatched.
func (e *CompositionEvent) initCompositionEvent(typ string, canBubble, cancelable bool, data string) {
	if e.IsBeingDispatched() {
		return
	}
	e.initEvent(typ, canBubble, cancelable)
	e.data = data
}
