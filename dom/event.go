// Translation of: Source/WebCore/dom/Event.h
//                  Source/WebCore/dom/Event.cpp
//                  Source/WebCore/dom/EventInit.h
// Completeness: 80%
// Simplifications:
//   - Event is a Go interface satisfied by a baseEvent struct embedded by subclasses
//     (MouseEvent, KeyboardEvent, ...); the C++ virtual dispatch is replaced by
//     interface dispatch plus promoted base methods
//   - event timestamps use Go time.Time instead of WTF::MonotonicTime
//   - isTrusted/IsComposed legacy flags are kept but trusted events are user-created
//     only via constructors in this port
//   - composedPath / EventPath shadow-tree details are omitted
//   - wtf.AtomString is available but event types use native Go strings for ergonomics

package dom

import "time"

// EventPhase mirrors Event::PhaseType.
type EventPhase uint8

// Event phase constants, mirroring Event::NONE/CAPTURING_PHASE/AT_TARGET/BUBBLING_PHASE.
const (
	EventNone           EventPhase = 0
	EventCapturingPhase EventPhase = 1
	EventAtTarget       EventPhase = 2
	EventBubblingPhase  EventPhase = 3
)

// Event is the Go translation of WebCore::Event. It exposes the read-only DOM Event
// attributes (type, target, currentTarget, phase, bubbles, cancelable, ...) and the
// propagation-control methods (StopPropagation, StopImmediatePropagation,
// PreventDefault). Concrete events embed baseEvent to inherit the implementation.
type Event interface {
	// Identity and phase.
	Type() string
	Target() EventTarget
	CurrentTarget() EventTarget
	EventPhase() EventPhase
	TimeStamp() time.Time

	// Flags.
	Bubbles() bool
	Cancelable() bool
	Composed() bool
	IsTrusted() bool
	DefaultPrevented() bool

	// Propagation control, mirroring stopPropagation / stopImmediatePropagation /
	// preventDefault.
	StopPropagation()
	StopImmediatePropagation()
	PreventDefault()

	// Internal query helpers used by the dispatcher.
	PropagationStopped() bool
	ImmediatePropagationStopped() bool
}

// eventInternal exposes the mutation hooks the dispatcher needs to drive an Event
// through the capture/target/bubble phases. It is satisfied by baseEvent (and therefore
// by every concrete event type via embedding) and asserted at dispatch time so that the
// public Event interface stays free of mutation methods.
type eventInternal interface {
	setTarget(EventTarget)
	setCurrentTarget(EventTarget)
	setEventPhase(EventPhase)
	setInPassiveListener(bool)
	resetBeforeDispatch()
	resetAfterDispatch()
}

// EventInit mirrors the WebCore::EventInit dictionary used by the Event(type, init)
// constructor. It is the base of MouseEventInit/KeyboardEventInit/etc.
type EventInit struct {
	Bubbles    bool
	Cancelable bool
	Composed   bool
}

// baseEvent is the shared implementation embedded by every concrete event type. It
// carries the type string, the capture/bubble/cancel flags and the mutable dispatch
// state (target, currentTarget, phase, propagation flags, defaultPrevented). The
// dispatcher drives an event through its phases by calling the eventInternal hooks.
type baseEvent struct {
	typ                       string
	bubbles                   bool
	cancelable                bool
	composed                  bool
	trusted                   bool
	phase                     EventPhase
	target                    EventTarget
	currentTarget             EventTarget
	propagationStopped        bool
	immediatePropagationStopped bool
	defaultPrevented          bool
	defaultHandled            bool
	inPassiveListener         bool
	timeStamp                 time.Time
}

// NewEvent constructs a plain Event, mirroring Event::create(type, canBubble,
// isCancelable, isComposed).
func NewEvent(typ string, canBubble, cancelable, composed bool) Event {
	return &baseEvent{
		typ:        typ,
		bubbles:    canBubble,
		cancelable: cancelable,
		composed:   composed,
		trusted:    false,
		timeStamp:  time.Now(),
	}
}

// NewEventFromInit constructs an Event from an EventInit dictionary, mirroring
// Event::create(type, EventInit).
func NewEventFromInit(typ string, init EventInit) Event {
	return &baseEvent{
		typ:       typ,
		bubbles:   init.Bubbles,
		cancelable: init.Cancelable,
		composed:  init.Composed,
		timeStamp: time.Now(),
	}
}

// newBaseEvent is the constructor used by subclasses (MouseEvent/KeyboardEvent/...) to
// seed their embedded baseEvent with common state. trusted defaults to true for events
// constructed by the runtime.
func newBaseEvent(typ string, canBubble, cancelable, composed bool, trusted bool) baseEvent {
	return baseEvent{
		typ:       typ,
		bubbles:   canBubble,
		cancelable: cancelable,
		composed:  composed,
		trusted:   trusted,
		timeStamp: time.Now(),
	}
}

// Read-only attribute accessors.
func (e *baseEvent) Type() string             { return e.typ }
func (e *baseEvent) Target() EventTarget      { return e.target }
func (e *baseEvent) CurrentTarget() EventTarget { return e.currentTarget }
func (e *baseEvent) EventPhase() EventPhase   { return e.phase }
func (e *baseEvent) TimeStamp() time.Time     { return e.timeStamp }
func (e *baseEvent) Bubbles() bool            { return e.bubbles }
func (e *baseEvent) Cancelable() bool         { return e.cancelable }
func (e *baseEvent) Composed() bool           { return e.composed }
func (e *baseEvent) IsTrusted() bool          { return e.trusted }
func (e *baseEvent) DefaultPrevented() bool   { return e.defaultPrevented }
func (e *baseEvent) PropagationStopped() bool {
	return e.propagationStopped || e.immediatePropagationStopped
}
func (e *baseEvent) ImmediatePropagationStopped() bool { return e.immediatePropagationStopped }

// StopPropagation marks the event so the dispatcher will not visit further targets,
// mirroring Event::stopPropagation(). Listeners on the current target keep firing.
func (e *baseEvent) StopPropagation() { e.propagationStopped = true }

// StopImmediatePropagation marks the event so the dispatcher will not visit further
// targets and will not fire any further listeners on the current target, mirroring
// Event::stopImmediatePropagation().
func (e *baseEvent) StopImmediatePropagation() {
	e.immediatePropagationStopped = true
	e.propagationStopped = true
}

// PreventDefault sets the canceled flag when the event is cancelable and is not being
// dispatched inside a passive listener, mirroring Event::preventDefault() /
// setCanceledFlagIfPossible().
func (e *baseEvent) PreventDefault() {
	if e.cancelable && !e.inPassiveListener {
		e.defaultPrevented = true
	}
}

// eventInternal hooks used by the dispatcher.

func (e *baseEvent) setTarget(t EventTarget)           { e.target = t }
func (e *baseEvent) setCurrentTarget(t EventTarget)    { e.currentTarget = t }
func (e *baseEvent) setEventPhase(p EventPhase)        { e.phase = p }
func (e *baseEvent) setInPassiveListener(v bool)       { e.inPassiveListener = v }

// resetBeforeDispatch clears the per-dispatch mutable state so a reused Event object
// starts a fresh propagation pass, mirroring Event::resetBeforeDispatch().
func (e *baseEvent) resetBeforeDispatch() {
	e.propagationStopped = false
	e.immediatePropagationStopped = false
	e.defaultPrevented = false
	e.defaultHandled = false
	e.inPassiveListener = false
	e.phase = EventNone
}

// resetAfterDispatch restores the post-dispatch state, mirroring Event::resetAfterDispatch().
func (e *baseEvent) resetAfterDispatch() {
	e.currentTarget = nil
	e.inPassiveListener = false
}

// IsBeingDispatched reports whether the dispatch flag is set, mirroring
// Event::isBeingDispatched(). The dispatch flag is true while eventPhase != NONE.
func (e *baseEvent) IsBeingDispatched() bool { return e.phase != EventNone }

// initEvent re-initialises an event, mirroring Event::initEvent().
func (e *baseEvent) initEvent(typ string, canBubble, cancelable bool) {
	e.typ = typ
	e.bubbles = canBubble
	e.cancelable = cancelable
	e.propagationStopped = false
	e.immediatePropagationStopped = false
	e.defaultPrevented = false
}
