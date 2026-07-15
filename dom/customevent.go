// Translation of: Source/WebCore/dom/CustomEvent.h
//                  Source/WebCore/dom/CustomEvent.cpp
// Completeness: 80%
// Simplifications:
//   - CustomEvent embeds baseEvent directly, following the same pattern as FocusEvent
//     and MouseEvent in this port
//   - the detail field is interface{} rather than a JSValue reference; callers are
//     expected to store any Go value that represents the event's payload
//   - initCustomEvent / init-dictionary constructor follow the DOM spec pattern

package dom

// CustomEvent is the Go translation of WebCore::CustomEvent. It carries an arbitrary
// detail value alongside the standard Event machinery, allowing application code to
// dispatch events with custom payloads. CustomEvent embeds baseEvent to inherit the
// standard Event interface implementation.
type CustomEvent struct {
	baseEvent
	detail interface{}
}

// NewCustomEvent constructs a CustomEvent with the given type and an initialisation
// map. Supported init keys:
//
//	"bubbles"    (bool, default false)
//	"cancelable" (bool, default false)
//	"composed"   (bool, default false)
//	"detail"     (interface{}, default nil)
//
// This mirrors the DOM spec's CustomEvent(type, init) constructor.
func NewCustomEvent(typ string, init map[string]interface{}) *CustomEvent {
	ev := &CustomEvent{
		baseEvent: newBaseEvent(typ, false, false, false, false),
	}
	if init == nil {
		return ev
	}
	if v, ok := init["bubbles"]; ok {
		if b, ok := v.(bool); ok {
			ev.bubbles = b
		}
	}
	if v, ok := init["cancelable"]; ok {
		if b, ok := v.(bool); ok {
			ev.cancelable = b
		}
	}
	if v, ok := init["composed"]; ok {
		if b, ok := v.(bool); ok {
			ev.composed = b
		}
	}
	if v, ok := init["detail"]; ok {
		ev.detail = v
	}
	return ev
}

// Detail returns the custom detail value, mirroring CustomEvent::detail().
func (e *CustomEvent) Detail() interface{} { return e.detail }

// initCustomEvent re-initialises the event, mirroring CustomEvent::initCustomEvent().
// It follows the legacy DOM 0 pattern: type, canBubble, cancelable, detail.
func (e *CustomEvent) initCustomEvent(typ string, canBubble, cancelable bool, detail interface{}) {
	e.initEvent(typ, canBubble, cancelable)
	e.detail = detail
}
