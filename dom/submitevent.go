// Translation of: Source/WebCore/dom/SubmitEvent.h
//                  Source/WebCore/dom/SubmitEvent.cpp
//                  Source/WebCore/dom/SubmitEvent.idl
// Completeness: 80%
//
// SubmitEvent is fired when a form is submitted. It carries a reference to
// the submitter element (the submit button that triggered the submission, or
// nil if the submission was triggered programmatically).

package dom

// SubmitEventInit mirrors WebCore::SubmitEvent::Init (the EventInit-derived
// dictionary used by the SubmitEvent(type, init) constructor).
type SubmitEventInit struct {
	EventInit
	Submitter *Element
}

// SubmitEvent is the Go translation of WebCore::SubmitEvent. It is dispatched
// when a form's submit algorithm runs. The event type string is "submit".
type SubmitEvent struct {
	baseEvent
	submitter *Element
}

// Submit event type constants, mirroring the event type strings used by WebCore.
const (
	EventSubmit = "submit"
	EventReset  = "reset"
)

// NewSubmitEvent constructs a SubmitEvent, mirroring SubmitEvent::create(submitter).
// The event bubbles and is cancelable, matching the spec.
func NewSubmitEvent(submitter *Element) *SubmitEvent {
	return &SubmitEvent{
		baseEvent: newBaseEvent(EventSubmit, true, true, true, true),
		submitter: submitter,
	}
}

// NewSubmitEventFromInit constructs a SubmitEvent from a SubmitEventInit
// dictionary, mirroring SubmitEvent::create(type, init).
func NewSubmitEventFromInit(typ string, init SubmitEventInit) *SubmitEvent {
	return &SubmitEvent{
		baseEvent: newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		submitter: init.Submitter,
	}
}

// Submitter returns the element that triggered the form submission (typically
// a submit button or input[type=submit]), or nil if the submission was
// triggered programmatically. Mirrors SubmitEvent::submitter().
func (e *SubmitEvent) Submitter() *Element { return e.submitter }
