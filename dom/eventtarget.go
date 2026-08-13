// Translation of: Source/WebCore/dom/EventTarget.h
//                  Source/WebCore/dom/EventTarget.cpp
//                  Source/WebCore/dom/EventListenerMap.cpp
//                  Source/WebCore/dom/EventDispatcher.cpp
// Completeness: 80%
// Simplifications:
//   - EventTarget is a Go interface; the only concrete implementation in this port is
//     nodeBase (every EventTarget is a Node), so listener storage lives on nodeBase
//   - the full capture/target/bubble dispatch flow is implemented here in
//     nodeBase.DispatchEvent, replacing EventDispatcher::dispatchEvent
//   - EventListener is an interface (HandleEvent) plus an EventListenerFunc adapter
//   - capture/bubble listeners are stored in one map keyed by type with a capture flag
//   - the legacy "onevent" attribute listeners are not modelled separately

package dom

import "reflect"

// EventTarget is the Go translation of WebCore::EventTarget. It is the contract for an
// object that can receive events. In this port the only concrete EventTargets are DOM
// nodes (via nodeBase), so AddEventListener/RemoveEventListener/DispatchEvent are
// implemented once on *nodeBase and promoted to every node type.
type EventTarget interface {
	// AddEventListener registers listener for the given event type, mirroring
	// EventTarget::addEventListener. The optional capture flag selects the capturing
	// phase; omitted means bubbling. Returns false if an identical listener was
	// already registered.
	AddEventListener(eventType string, listener EventListener, capture ...bool) bool
	// AddEventListenerWithOptions registers a listener with the full options object
	// (capture/passive/once).
	AddEventListenerWithOptions(eventType string, listener EventListener, options AddEventListenerOptions) bool
	// RemoveEventListener unregisters a listener, mirroring EventTarget::removeEventListener.
	RemoveEventListener(eventType string, listener EventListener, capture ...bool) bool
	// RemoveAllEventListeners clears every listener, mirroring EventTarget::removeAllEventListeners.
	RemoveAllEventListeners()
	// HasEventListener reports whether any listener is registered for eventType.
	HasEventListener(eventType string) bool
	// DispatchEvent runs the capture/target/bubble dispatch and returns whether no
	// listener called preventDefault, mirroring EventTarget::dispatchEvent.
	DispatchEvent(Event) bool
}

// EventListener is the Go translation of WebCore::EventListener. Implementations handle
// a single dispatched event.
type EventListener interface {
	HandleEvent(Event)
}

// EventListenerFunc adapts a plain function to the EventListener interface, mirroring
// the JS "event handler function" form of addEventListener.
type EventListenerFunc func(Event)

// HandleEvent calls f(event).
func (f EventListenerFunc) HandleEvent(e Event) { f(e) }

// EventListenerOptions mirrors WebCore::EventListenerOptions.
type EventListenerOptions struct {
	Capture bool
}

// AddEventListenerOptions mirrors WebCore::AddEventListenerOptions.
type AddEventListenerOptions struct {
	Capture bool
	Passive bool
	Once   bool
}

// registeredListener is the internal record kept for one (type, listener, capture)
// triple. The Once flag schedules removal after the first invocation; Removed is set
// when the listener has been logically unregistered (e.g. fired Once) and is filtered
// out during the next compaction.
type registeredListener struct {
	callback EventListener
	capture  bool
	passive  bool
	once     bool
	removed  bool
}

// AddEventListener registers listener for eventType with the bubbling phase (or
// capturing when capture is true), mirroring EventTarget::addEventListener(type,
// listener, useCapture).
func (b *nodeBase) AddEventListener(eventType string, listener EventListener, capture ...bool) bool {
	c := false
	if len(capture) > 0 {
		c = capture[0]
	}
	return b.AddEventListenerWithOptions(eventType, listener, AddEventListenerOptions{Capture: c})
}

// AddEventListenerWithOptions registers a listener with the full options object,
// mirroring EventTarget::addEventListener(type, listener, options). A duplicate (same
// callback and capture flag) is rejected to match the DOM spec.
func (b *nodeBase) AddEventListenerWithOptions(eventType string, listener EventListener, options AddEventListenerOptions) bool {
	if listener == nil {
		return false
	}
	if b.listeners == nil {
		b.listeners = map[string][]*registeredListener{}
	}
	for _, l := range b.listeners[eventType] {
		if listenerEquals(l.callback, listener) && l.capture == options.Capture {
			return false
		}
	}
	b.listeners[eventType] = append(b.listeners[eventType], &registeredListener{
		callback: listener,
		capture:  options.Capture,
		passive:  options.Passive,
		once:     options.Once,
	})
	return true
}

// RemoveEventListener unregisters a listener, mirroring EventTarget::removeEventListener.
func (b *nodeBase) RemoveEventListener(eventType string, listener EventListener, capture ...bool) bool {
	c := false
	if len(capture) > 0 {
		c = capture[0]
	}
	if b.listeners == nil {
		return false
	}
	list := b.listeners[eventType]
	for _, l := range list {
		if listenerEquals(l.callback, listener) && l.capture == c && !l.removed {
			l.removed = true
			return true
		}
	}
	return false
}

// listenerEquals reports whether two EventListener values reference the same underlying
// callback. Go does not permit == on interface values whose dynamic type is a function,
// map or slice (it panics), so the comparison uses reflect to obtain the underlying
// pointer for those kinds and falls back to direct equality for comparable kinds. Two
// distinct function literals that happen to do the same thing are treated as distinct,
// matching how WebKit compares the JS callback identity.
func listenerEquals(a, b EventListener) bool {
	if a == nil || b == nil {
		return a == b
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Kind() != vb.Kind() {
		return false
	}
	switch va.Kind() {
	case reflect.Func, reflect.Ptr, reflect.Chan, reflect.Map, reflect.Slice, reflect.UnsafePointer:
		return va.Pointer() == vb.Pointer()
	default:
		return a == b
	}
}

// RemoveAllEventListeners clears every registered listener, mirroring
// EventTarget::removeAllEventListeners.
func (b *nodeBase) RemoveAllEventListeners() {
	b.listeners = nil
}

// HasEventListener reports whether any listener is registered for eventType, mirroring
// EventTarget::hasEventListeners.
func (b *nodeBase) HasEventListener(eventType string) bool {
	if b.listeners == nil {
		return false
	}
	for _, l := range b.listeners[eventType] {
		if !l.removed {
			return true
		}
	}
	return false
}

// DispatchEvent runs the full capture/target/bubble propagation algorithm and returns
// whether no listener called preventDefault. It mirrors
// EventDispatcher::dispatchEvent combined with EventTarget::dispatchEvent.
//
// The propagation path is [target, parent, ..., root]. The capture phase visits the
// path from the root down to the target's parent; the target phase fires both capture
// and bubble listeners on the target; the bubble phase (only when Bubbles) walks back
// up to the root. StopPropagation halts progression to further targets;
// StopImmediatePropagation also halts further listeners on the current target. If the
// event was not canceled the target's defaultEventHandler is invoked.
func (b *nodeBase) DispatchEvent(event Event) bool {
	if event == nil {
		return true
	}
	ev, ok := event.(eventInternal)
	if !ok {
		// No internal hooks: just fire listeners on this target.
		fireEventListeners(b.self, event, false)
		return !event.DefaultPrevented()
	}
	// Build propagation path: index 0 is the target, last index is the root. The path
	// crosses shadow boundaries (a shadow-root child's composed parent is the shadow
	// host) and includes slot nodes when the event is composed; a non-composed event
	// stops at its shadow root.
	path := buildEventPath(Node(b.self), event.Composed())
	ev.setTarget(b.self)
	ev.setPath(path)
	ev.resetBeforeDispatch()

	// Save the un-retargeted relatedTarget (if any) so it can be restored after
	// dispatch; DOM §2.8 retargets relatedTarget alongside target per currentTarget.
	var rtEv relatedTargetProvider
	var origRelated EventTarget
	if rtp, ok := event.(relatedTargetProvider); ok {
		rtEv = rtp
		origRelated = rtp.RelatedTarget()
	}

	// setCurrent applies event retargeting (DOM §2.8): the target a listener observes
	// is retargeted to the shadow host when the listener sits on or above that host in
	// the composed tree, so shadow-internal targets do not leak past the boundary.
	setCurrent := func(et EventTarget) {
		ev.setCurrentTarget(et)
		if cn, ok := et.(Node); ok {
			ev.setTarget(retargetedTarget(Node(b.self), cn))
			if rtEv != nil && origRelated != nil {
				if rn, ok := origRelated.(Node); ok {
					rtEv.SetRelatedTarget(retargetedTarget(rn, cn))
				}
			}
		} else {
			ev.setTarget(et)
		}
	}

	// Capture phase: root -> target's parent.
	if len(path) > 1 {
		ev.setEventPhase(EventCapturingPhase)
		for i := len(path) - 1; i >= 1; i-- {
			if event.PropagationStopped() {
				break
			}
			setCurrent(path[i])
			fireEventListeners(path[i], event, true)
		}
	}

	// Target phase: fire capture then bubble listeners on the target.
	if !event.PropagationStopped() {
		setCurrent(path[0])
		ev.setEventPhase(EventAtTarget)
		fireEventListeners(path[0], event, true)
		if !event.ImmediatePropagationStopped() {
			fireEventListeners(path[0], event, false)
		}
	}

	// Bubble phase: target's parent -> root.
	if event.Bubbles() && !event.PropagationStopped() {
		ev.setEventPhase(EventBubblingPhase)
		for i := 1; i < len(path); i++ {
			if event.PropagationStopped() {
				break
			}
			setCurrent(path[i])
			fireEventListeners(path[i], event, false)
		}
	}

	ev.setEventPhase(EventNone)
	ev.setCurrentTarget(nil)
	ev.setTarget(b.self) // restore the un-retargeted target once dispatch ends
	if rtEv != nil {
		rtEv.SetRelatedTarget(origRelated) // restore the un-retargeted relatedTarget
	}
	ev.resetAfterDispatch()

	// Default action: if not canceled, give the target a chance to perform its default
	// behavior, mirroring Node::defaultEventHandler.
	if !event.DefaultPrevented() {
		if dh, ok := path[0].(defaultActionHandler); ok {
			dh.defaultEventHandler(event)
		}
	}
	return !event.DefaultPrevented()
}

// defaultActionHandler is the internal hook a target implements to opt into default
// behavior (e.g. an anchor navigating on click). The default nodeBase implementation
// is a no-op.
type defaultActionHandler interface {
	defaultEventHandler(Event)
}

// relatedTargetProvider is satisfied by events carrying a relatedTarget (MouseEvent,
// FocusEvent) that must be retargeted alongside the target (DOM §2.8 last paragraph:
// "for events whose relatedTarget is non-null, that value is also retargeted in the
// same way as the target"). The dispatcher retargets it per-currentTarget so a
// shadow-internal related target does not leak past the shadow boundary, then restores
// the original value once dispatch ends.
type relatedTargetProvider interface {
	RelatedTarget() EventTarget
	SetRelatedTarget(EventTarget)
}

// eventPathParent returns n's next node in a composed event's propagation path
// (DOM §5.3 flattened tree): a slot-assigned light-DOM node's next step is the
// <slot> that assigns it (not its light-DOM parent), so the slot node appears in the
// composed path between the assigned node and the shadow host. Every other node uses
// ComposedParent, which crosses the shadow boundary.
func eventPathParent(n Node) Node {
	if el, ok := n.(*Element); ok {
		if slot := el.AssignedSlot(); slot != nil {
			return slot
		}
	}
	return ComposedParent(n)
}

// buildEventPath constructs the composed propagation path for an event dispatched at
// target: the list of targets from target up to the document root. For a composed
// event the walk crosses shadow boundaries and includes slot nodes (via
// eventPathParent); for a non-composed event it follows the raw parent chain and
// stops at the shadow root, never leaking into the host's light-DOM tree.
func buildEventPath(target Node, composed bool) []EventTarget {
	var path []EventTarget
	for n := target; n != nil; {
		path = append(path, n)
		var next Node
		if composed {
			next = eventPathParent(n)
		} else {
			next = n.ParentNode()
		}
		if next == nil {
			break
		}
		if !composed {
			if _, isSR := next.(*ShadowRoot); isSR {
				break
			}
		}
		n = next
	}
	return path
}

// retargetedTarget returns the target a listener on `current` observes for an event
// whose real target is `target` (DOM §2.8 event retargeting). When the real target
// lives inside a shadow tree and `current` sits on or above that tree's host in the
// composed tree, the target is retargeted — layer by layer for nested shadow trees —
// to the innermost shadow host that `current` is still inside or above.
func retargetedTarget(target Node, current Node) Node {
	rt := target
	for {
		sr := ContainingShadowRoot(rt)
		if sr == nil {
			break
		}
		host := sr.Host()
		if host == nil || !isComposedInclusiveAncestor(current, host) {
			break
		}
		rt = host
	}
	return rt
}

// isComposedInclusiveAncestor reports whether ancestor is node itself or one of its
// composed-tree ancestors (walking ComposedParent across shadow boundaries).
func isComposedInclusiveAncestor(ancestor Node, node Node) bool {
	for n := node; n != nil; n = ComposedParent(n) {
		if n == ancestor {
			return true
		}
	}
	return false
}

// nodeBase satisfies defaultActionHandler with a no-op so DispatchEvent can call it
// uniformly.
func (b *nodeBase) defaultEventHandler(Event) {}

// fireEventListeners invokes the listeners of phase (capture when capture is true,
// bubble otherwise) registered on target for the event's type. It honours the Passive
// flag (preventDefault is ignored inside passive listeners) and the Once flag (the
// listener is removed after it fires). StopImmediatePropagation aborts the loop.
func fireEventListeners(target EventTarget, event Event, capture bool) {
	nb := nodeBaseOfEventTarget(target)
	if nb == nil || nb.listeners == nil {
		return
	}
	typ := event.Type()
	list := nb.listeners[typ]
	if len(list) == 0 {
		return
	}
	ev, _ := event.(eventInternal)
	// Snapshot the listener slice so registration/removal during iteration cannot
	// corrupt the loop; the Once-removed listeners are filtered out afterwards.
	snap := make([]*registeredListener, len(list))
	copy(snap, list)
	for _, l := range snap {
		if l.removed {
			continue
		}
		if l.capture != capture {
			continue
		}
		if ev != nil {
			ev.setInPassiveListener(l.passive)
		}
		l.callback.HandleEvent(event)
		if ev != nil {
			ev.setInPassiveListener(false)
		}
		if l.once {
			l.removed = true
		}
		if event.ImmediatePropagationStopped() {
			break
		}
	}
	// Compact the stored slice, dropping removed listeners.
	if compact := compactListeners(list); compact != nil {
		nb.listeners[typ] = compact
	} else if nb.listeners[typ] != nil {
		delete(nb.listeners, typ)
	}
}

// compactListeners returns a slice containing only the non-removed listeners, or nil
// if none remain.
func compactListeners(list []*registeredListener) []*registeredListener {
	out := make([]*registeredListener, 0, len(list))
	for _, l := range list {
		if !l.removed {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// nodeBaseOfEventTarget returns the *nodeBase for an EventTarget. In this port every
// EventTarget is a Node, so the assertion falls back to Node.
func nodeBaseOfEventTarget(et EventTarget) *nodeBase {
	if et == nil {
		return nil
	}
	if n, ok := et.(Node); ok {
		return nodeBaseOf(n)
	}
	return nil
}
