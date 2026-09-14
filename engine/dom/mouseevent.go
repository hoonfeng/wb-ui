// Translation of: Source/WebCore/dom/MouseEvent.h
//                  Source/WebCore/dom/MouseEvent.cpp
//                  Source/WebCore/dom/MouseEventInit.h
// Completeness: 75%
// Simplifications:
//   - the MouseRelatedEvent/UIEvent intermediate layers are flattened: MouseEvent embeds
//     baseEvent directly and carries the fields those layers would have held
//   - WindowProxy/detail/platform event plumbing is reduced to plain fields
//   - coalesced/predicted event vectors and input source metadata are omitted

package dom

// MouseButton mirrors WebCore::MouseButton.
type MouseButton int16

const (
	MouseButtonNone    MouseButton = -1
	MouseButtonLeft     MouseButton = 0
	MouseButtonMiddle   MouseButton = 1
	MouseButtonRight    MouseButton = 2
)

// Mouse event type constants.
const (
	EventClick       = "click"
	EventMouseDown   = "mousedown"
	EventMouseUp     = "mouseup"
	EventMouseMove   = "mousemove"
	EventMouseOver   = "mouseover"
	EventMouseOut    = "mouseout"
	EventMouseEnter  = "mouseenter"
	EventMouseLeave  = "mouseleave"
	EventDblClick    = "dblclick"
	EventContextMenu = "contextmenu"
)

// MouseEventInit mirrors WebCore::MouseEventInit.
type MouseEventInit struct {
	EventInit
	ScreenX, ScreenY, ClientX, ClientY   float64
	MovementX, MovementY                 float64
	CtrlKey, AltKey, ShiftKey, MetaKey   bool
	Button                              MouseButton
	Buttons                             uint16
	Detail                              int
	RelatedTarget                       EventTarget
}

// MouseEvent is the Go translation of WebCore::MouseEvent. It embeds baseEvent for the
// common Event machinery and adds the mouse-coordinate, button and modifier fields.
type MouseEvent struct {
	baseEvent
	screenX, screenY, clientX, clientY, movementX, movementY float64
	detail                                                   int
	button                                                   MouseButton
	buttons                                                  uint16
	ctrlKey, altKey, shiftKey, metaKey                       bool
	buttonDown                                               bool
	force                                                    float64
	relatedTarget                                            EventTarget
}

// NewMouseEvent constructs a MouseEvent, mirroring MouseEvent::create(type, canBubble,
// isCancelable, isComposed).
func NewMouseEvent(typ string, canBubble, cancelable, composed bool) *MouseEvent {
	return &MouseEvent{
		baseEvent: newBaseEvent(typ, canBubble, cancelable, composed, true),
	}
}

// NewMouseEventFromInit constructs a MouseEvent from a MouseEventInit dictionary,
// mirroring MouseEvent::create(type, init).
func NewMouseEventFromInit(typ string, init MouseEventInit) *MouseEvent {
	m := &MouseEvent{
		baseEvent:    newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		screenX:      init.ScreenX,
		screenY:      init.ScreenY,
		clientX:      init.ClientX,
		clientY:      init.ClientY,
		movementX:    init.MovementX,
		movementY:    init.MovementY,
		detail:       init.Detail,
		button:       init.Button,
		buttons:      init.Buttons,
		ctrlKey:      init.CtrlKey,
		altKey:       init.AltKey,
		shiftKey:     init.ShiftKey,
		metaKey:      init.MetaKey,
		relatedTarget: init.RelatedTarget,
	}
	return m
}

// initMouseEvent re-initialises the event, mirroring MouseEvent::initMouseEvent().
func (m *MouseEvent) initMouseEvent(typ string, canBubble, cancelable bool, detail int, screenX, screenY, clientX, clientY float64, ctrlKey, altKey, shiftKey, metaKey bool, button MouseButton, relatedTarget EventTarget) {
	m.initEvent(typ, canBubble, cancelable)
	m.detail = detail
	m.screenX, m.screenY, m.clientX, m.clientY = screenX, screenY, clientX, clientY
	m.ctrlKey, m.altKey, m.shiftKey, m.metaKey = ctrlKey, altKey, shiftKey, metaKey
	m.button = button
	m.relatedTarget = relatedTarget
}

// Coordinate and button accessors, mirroring MouseEvent::screenX()/clientX()/button()/...
func (m *MouseEvent) ScreenX() float64      { return m.screenX }
func (m *MouseEvent) ScreenY() float64      { return m.screenY }
func (m *MouseEvent) ClientX() float64      { return m.clientX }
func (m *MouseEvent) ClientY() float64      { return m.clientY }
func (m *MouseEvent) MovementX() float64    { return m.movementX }
func (m *MouseEvent) MovementY() float64    { return m.movementY }
func (m *MouseEvent) Detail() int           { return m.detail }
func (m *MouseEvent) Button() MouseButton   { return m.button }
func (m *MouseEvent) Buttons() uint16       { return m.buttons }
func (m *MouseEvent) ButtonDown() bool      { return m.buttonDown }
func (m *MouseEvent) Force() float64       { return m.force }
func (m *MouseEvent) SetForce(f float64)   { m.force = f }

// Modifier accessors, mirroring UIEventWithKeyState::getModifierState().
func (m *MouseEvent) CtrlKey() bool  { return m.ctrlKey }
func (m *MouseEvent) AltKey() bool   { return m.altKey }
func (m *MouseEvent) ShiftKey() bool { return m.shiftKey }
func (m *MouseEvent) MetaKey() bool  { return m.metaKey }

// GetModifierState reports whether the named modifier is active, mirroring
// MouseEvent::getModifierState(key).
func (m *MouseEvent) GetModifierState(key string) bool {
	switch key {
	case "Control":
		return m.ctrlKey
	case "Alt":
		return m.altKey
	case "Shift":
		return m.shiftKey
	case "Meta":
		return m.metaKey
	}
	return false
}

// RelatedTarget returns the event's related target, mirroring MouseEvent::relatedTarget().
func (m *MouseEvent) RelatedTarget() EventTarget { return m.relatedTarget }

// SetRelatedTarget sets the related target, mirroring MouseEvent::setRelatedTarget().
func (m *MouseEvent) SetRelatedTarget(t EventTarget) { m.relatedTarget = t }
