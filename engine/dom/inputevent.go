// Translation of: Source/WebCore/dom/InputEvent.h
//                  Source/WebCore/dom/InputEvent.cpp
//                  Source/WebCore/dom/InputEvent.idl
// Completeness: 80%
//
// InputEvent is fired when the user modifies the value of an <input>,
// <textarea>, or contentEditable element. It carries the inputType string
// (e.g. "insertText", "deleteContentBackward") describing the kind of edit,
// the data payload (the inserted/deleted text), and whether the change
// occurred during IME composition.

package dom

// InputEventInit mirrors WebCore::InputEvent::Init (the EventInit-derived
// dictionary used by the InputEvent(type, init) constructor).
type InputEventInit struct {
	EventInit
	InputType   string
	Data        string
	IsComposing bool
}

// InputEvent is the Go translation of WebCore::InputEvent. It is dispatched
// after the DOM has been updated to reflect a user edit. The event type
// string is always "input" (it does not bubble in WebKit but the spec says
// it does; this port follows the spec and makes it bubble).
type InputEvent struct {
	baseEvent
	inputType   string
	data        string
	isComposing bool
}

// Input event type constants, mirroring the event type strings used by WebCore.
const (
	EventInput  = "input"
	EventChange = "change"
)

// NewInputEvent constructs an InputEvent, mirroring InputEvent::create(type,
// canBubble, cancelable, view, inputType, data, isComposing).
func NewInputEvent(inputType, data string, isComposing bool) *InputEvent {
	return &InputEvent{
		baseEvent:   newBaseEvent(EventInput, true, false, true, true),
		inputType:   inputType,
		data:        data,
		isComposing: isComposing,
	}
}

// NewInputEventFromInit constructs an InputEvent from an InputEventInit
// dictionary, mirroring InputEvent::create(type, init).
func NewInputEventFromInit(typ string, init InputEventInit) *InputEvent {
	return &InputEvent{
		baseEvent:   newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		inputType:   init.InputType,
		data:        init.Data,
		isComposing: init.IsComposing,
	}
}

// InputType returns the kind of edit that triggered the event, mirroring
// InputEvent::inputType(). Common values include:
//   - "insertText"            : text was inserted
//   - "insertFromPaste"       : text was pasted
//   - "insertCompositionText" : IME composition text was inserted
//   - "deleteContentBackward" : backspace
//   - "deleteContentForward"  : forward delete
//   - "deleteByCut"            : cut to clipboard
func (e *InputEvent) InputType() string { return e.inputType }

// Data returns the data payload for the edit, mirroring InputEvent::data().
// For insertText this is the inserted text; for deleteContentBackward it is
// the deleted text; for insertFromPaste it is the pasted text.
func (e *InputEvent) Data() string { return e.data }

// IsComposing reports whether the event was fired during IME composition,
// mirroring InputEvent::isComposing().
func (e *InputEvent) IsComposing() bool { return e.isComposing }

// initInputEvent re-initialises the event, mirroring
// InputEvent::initInputEvent(). Has no effect if the event is being dispatched.
func (e *InputEvent) initInputEvent(typ string, canBubble, cancelable bool, inputType, data string, isComposing bool) {
	if e.IsBeingDispatched() {
		return
	}
	e.initEvent(typ, canBubble, cancelable)
	e.inputType = inputType
	e.data = data
	e.isComposing = isComposing
}
