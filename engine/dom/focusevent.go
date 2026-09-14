// Translation of: Source/WebCore/dom/FocusEvent.h
//                  Source/WebCore/dom/FocusEvent.cpp
// Completeness: 80%
// Simplifications:
//   - FocusEvent embeds baseEvent directly (the UIEvent intermediate layer is flattened
//     in this port, matching the simplification used by MouseEvent/KeyboardEvent)
//   - the FocusEventInit dictionary carries only the relatedTarget field on top of
//     EventInit (detail is omitted alongside the UIEvent simplification)
//   - the FocusEvent sub-types (FocusInEvent/FocusOutEvent in WebKit) are represented by
//     the event "type" string ("focusin"/"focusout") rather than separate Go types

package dom

// Focus event type constants.
const (
	EventFocus    = "focus"
	EventBlur     = "blur"
	EventFocusIn  = "focusin"
	EventFocusOut = "focusout"
)

// FocusEventInit mirrors WebCore::FocusEventInit.
type FocusEventInit struct {
	EventInit
	RelatedTarget EventTarget
}

// FocusEvent is the Go translation of WebCore::FocusEvent. It adds a relatedTarget
// (the element losing or receiving focus) on top of the base Event state.
type FocusEvent struct {
	baseEvent
	relatedTarget EventTarget
}

// NewFocusEvent constructs a FocusEvent, mirroring FocusEvent::create(type, canBubble,
// isCancelable).
func NewFocusEvent(typ string, canBubble, cancelable bool) *FocusEvent {
	return &FocusEvent{
		baseEvent: newBaseEvent(typ, canBubble, cancelable, false, true),
	}
}

// NewFocusEventFromInit constructs a FocusEvent from a FocusEventInit dictionary,
// mirroring FocusEvent::create(type, init).
func NewFocusEventFromInit(typ string, init FocusEventInit) *FocusEvent {
	return &FocusEvent{
		baseEvent:     newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		relatedTarget: init.RelatedTarget,
	}
}

// initFocusEvent re-initialises the event, mirroring FocusEvent::initFocusEvent().
func (f *FocusEvent) initFocusEvent(typ string, canBubble, cancelable bool, relatedTarget EventTarget) {
	f.initEvent(typ, canBubble, cancelable)
	f.relatedTarget = relatedTarget
}

// RelatedTarget returns the related target (the element losing or receiving focus),
// mirroring FocusEvent::relatedTarget().
func (f *FocusEvent) RelatedTarget() EventTarget { return f.relatedTarget }

// SetRelatedTarget sets the related target, mirroring FocusEvent::setRelatedTarget().
func (f *FocusEvent) SetRelatedTarget(t EventTarget) { f.relatedTarget = t }
