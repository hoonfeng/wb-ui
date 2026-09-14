// Translation of: Source/WebCore/dom/WheelEvent.h
//                  Source/WebCore/dom/WheelEvent.cpp
// Completeness: 80%
// Simplifications:
//   - WheelEvent embeds MouseEvent (mirroring the C++ inheritance
//     WheelEvent : MouseEvent : MouseRelatedEvent : UIEvent : Event) so all mouse
//     coordinate/button/modifier accessors are promoted to WheelEvent
//   - the deprecated wheelDelta/wheelDeltaX/wheelDeltaY integer fields are kept for
//     compatibility with legacy callers; they are derived from deltaX/Y at construction
//   - PlatformWheelEvent plumbing, webkitDirectionInvertedFromDevice and macOS phase
//     accessors are omitted (no platform adapter in this port)

package dom

// DeltaMode mirrors WebCore::WheelEvent deltaMode constants.
type DeltaMode uint8

// Delta mode constants, mirroring WheelEvent::DOM_DELTA_PIXEL/LINE/PAGE.
const (
	DOMDeltaPixel DeltaMode = 0
	DOMDeltaLine  DeltaMode = 1
	DOMDeltaPage  DeltaMode = 2
)

// Wheel event type constant.
const EventWheel = "wheel"
// TickMultiplier mirrors WheelEvent::TickMultiplier (120) used to derive the legacy
// wheelDelta property values from deltaX/deltaY.
const TickMultiplier = 120

// WheelEventInit mirrors WebCore::WheelEvent::Init.
type WheelEventInit struct {
	MouseEventInit
	DeltaX    float64
	DeltaY    float64
	DeltaZ    float64
	DeltaMode DeltaMode
}

// WheelEvent is the Go translation of WebCore::WheelEvent. It embeds MouseEvent (which in
// turn embeds baseEvent) and adds the deltaX/Y/Z scroll deltas and the deltaMode unit.
type WheelEvent struct {
	MouseEvent
	deltaX    float64
	deltaY    float64
	deltaZ    float64
	deltaMode DeltaMode
}

// NewWheelEvent constructs a WheelEvent, mirroring WheelEvent::create(type, canBubble,
// isCancelable).
func NewWheelEvent(typ string, canBubble, cancelable bool) *WheelEvent {
	return &WheelEvent{
		MouseEvent: *NewMouseEvent(typ, canBubble, cancelable, false),
	}
}

// NewWheelEventFromInit constructs a WheelEvent from a WheelEventInit dictionary,
// mirroring WheelEvent::create(type, init).
func NewWheelEventFromInit(typ string, init WheelEventInit) *WheelEvent {
	w := &WheelEvent{
		MouseEvent: *NewMouseEventFromInit(typ, init.MouseEventInit),
		deltaX:     init.DeltaX,
		deltaY:     init.DeltaY,
		deltaZ:     init.DeltaZ,
		deltaMode:  init.DeltaMode,
	}
	return w
}

// initWheelEvent re-initialises the event, mirroring WheelEvent::initWheelEvent().
func (w *WheelEvent) initWheelEvent(typ string, canBubble, cancelable bool, deltaX, deltaY, deltaZ float64, deltaMode DeltaMode) {
	w.initEvent(typ, canBubble, cancelable)
	w.deltaX = deltaX
	w.deltaY = deltaY
	w.deltaZ = deltaZ
	w.deltaMode = deltaMode
}

// DeltaX returns the horizontal scroll delta (positive = scroll right), mirroring
// WheelEvent::deltaX().
func (w *WheelEvent) DeltaX() float64 { return w.deltaX }

// DeltaY returns the vertical scroll delta (positive = scroll down), mirroring
// WheelEvent::deltaY().
func (w *WheelEvent) DeltaY() float64 { return w.deltaY }

// DeltaZ returns the z-axis scroll delta, mirroring WheelEvent::deltaZ().
func (w *WheelEvent) DeltaZ() float64 { return w.deltaZ }

// DeltaMode returns the unit of the delta values (pixels/lines/pages), mirroring
// WheelEvent::deltaMode().
func (w *WheelEvent) DeltaMode() DeltaMode { return w.deltaMode }

// WheelDeltaX returns the deprecated integer horizontal delta (negative when scrolling
// right), mirroring WheelEvent::wheelDeltaX().
func (w *WheelEvent) WheelDeltaX() int {
	return int(-w.deltaX * TickMultiplier)
}

// WheelDeltaY returns the deprecated integer vertical delta (negative when scrolling
// down), mirroring WheelEvent::wheelDeltaY().
func (w *WheelEvent) WheelDeltaY() int {
	return int(-w.deltaY * TickMultiplier)
}

// WheelDelta returns the non-zero deprecated integer delta, preferring the Y axis,
// mirroring WheelEvent::wheelDelta().
func (w *WheelEvent) WheelDelta() int {
	if w.deltaY != 0 {
		return w.WheelDeltaY()
	}
	return w.WheelDeltaX()
}
