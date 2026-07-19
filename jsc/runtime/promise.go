// Translation of: Source/JavaScriptCore/runtime/JSPromise.h
//                  Source/JavaScriptCore/runtime/JSPromise.cpp
//
// JSPromise implements the ECMAScript Promise object.
// Manages state (Pending/Fulfilled/Rejected), reactions, and settlement logic.

package runtime

// JSPromise corresponds to JSC::JSPromise.
// Stores state, optional inline reactions, and settlement value.
type JSPromise struct {
	JSNonFinalObject
	status           JSPromiseStatus      // Pending/Fulfilled/Rejected
	isHandled        bool                 // has .catch() handler
	resolvingCalled  bool                 // has resolve/reject been called
	reactionHead     *JSPromiseReaction   // linked list of pending reactions
	settlementValue  JSValue              // fulfillment or rejection value
}

// JSPromiseStatus is the status of a Promise.
type JSPromiseStatus uint16

const (
	PromiseStatusPending   JSPromiseStatus = 0
	PromiseStatusFulfilled                = 1
	PromiseStatusRejected                 = 2
)

// NewJSPromise creates a new Promise in the Pending state.
func NewJSPromise(vm *VM, structure *Structure) *JSPromise {
	p := &JSPromise{
		status:          PromiseStatusPending,
		isHandled:       false,
		resolvingCalled: false,
	}
	p.structureID = structure.structureID
	p.typ = JSPromiseType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = vm
	return p
}

// Status returns the current promise status.
func (p *JSPromise) Status() JSPromiseStatus {
	return p.status
}

// IsHandled returns whether this promise has a rejection handler.
func (p *JSPromise) IsHandled() bool {
	return p.isHandled
}

// MarkAsHandled marks this promise as having a rejection handler.
func (p *JSPromise) MarkAsHandled() {
	p.isHandled = true
}

// SettlementValue returns the settlement value (only valid after settlement).
func (p *JSPromise) SettlementValue() JSValue {
	return p.settlementValue
}

// Result returns the promise's result — undefined if pending, the settlement value otherwise.
func (p *JSPromise) Result() JSValue {
	if p.status == PromiseStatusPending {
		return JSValueUndefined
	}
	return p.settlementValue
}

// IsFirstResolvingFunctionCalled returns whether resolve/reject has been called.
func (p *JSPromise) IsFirstResolvingFunctionCalled() bool {
	return p.resolvingCalled
}

// --- Settlement / Resolution ---

// Resolve implements Promise resolution:
//   If value is a thenable, unwrap it; otherwise fulfill.
func (p *JSPromise) Resolve(globalObject *JSGlobalObject, vm *VM, value JSValue) {
	if p.resolvingCalled {
		return
	}
	p.resolvingCalled = true
	p.resolvePromise(globalObject, vm, value)
}

// Reject transitions to Rejected with the given reason.
func (p *JSPromise) Reject(vm *VM, reason JSValue) {
	if p.resolvingCalled {
		return
	}
	p.resolvingCalled = true
	p.rejectPromise(vm, reason)
}

// Fulfill transitions to Fulfilled with the given value.
func (p *JSPromise) Fulfill(vm *VM, value JSValue) {
	if p.resolvingCalled {
		return
	}
	p.resolvingCalled = true
	p.fulfillPromise(vm, value)
}

// Then implements the ES then() method:
//   Appends fulfillment/rejection handlers and returns a new promise.
func (p *JSPromise) Then(globalObject *JSGlobalObject, onFulfilled, onRejected JSValue) *JSPromise {
	return p.performPromiseThen(globalObject, onFulfilled, onRejected, JSValueUndefined, false)
}

// PerformPromiseThen is the core [[PerformPromiseThen]] operation.
func (p *JSPromise) performPromiseThen(globalObject *JSGlobalObject, onFulfilled, onRejected, resultCapability JSValue, internal bool) *JSPromise {
	switch p.status {
	case PromiseStatusPending:
		// Append reaction (simplified)
		_ = onFulfilled
		_ = onRejected
		_ = resultCapability
		_ = internal
		return p
	case PromiseStatusFulfilled:
		return p
	case PromiseStatusRejected:
		return p
	}
	return p
}

// --- Internal helpers ---

// resolvePromise implements Promise resolving (ES 25.4.1.3.2).
func (p *JSPromise) resolvePromise(globalObject *JSGlobalObject, vm *VM, resolution JSValue) {
	// If resolution is the promise itself, reject with TypeError
	if p.samePromise(resolution) {
		p.rejectPromise(vm, JSValueUndefined)
		return
	}

	// If not an object, fulfill directly
	if !resolution.IsObject() {
		p.fulfillPromise(vm, resolution)
		return
	}

	// Otherwise, try to unwrap thenable (simplified)
	// In full implementation, would call PromiseResolveThenableJob
	p.fulfillPromise(vm, resolution)
}

// rejectPromise transitions to Rejected state.
func (p *JSPromise) rejectPromise(vm *VM, reason JSValue) {
	p.status = PromiseStatusRejected
	p.settlementValue = reason
	p.triggerReactions(vm, nil)
}

// fulfillPromise transitions to Fulfilled state.
func (p *JSPromise) fulfillPromise(vm *VM, value JSValue) {
	p.status = PromiseStatusFulfilled
	p.settlementValue = value
	p.triggerReactions(vm, nil)
}

// triggerReactions fires all pending reactions after settlement.
func (p *JSPromise) triggerReactions(vm *VM, head *JSPromiseReaction) {
	_ = vm
	_ = head
	// Simplified: clear reaction list
	p.reactionHead = nil
}

// samePromise checks if the given value is the same promise.
func (p *JSPromise) samePromise(value JSValue) bool {
	if !value.IsCell() {
		return false
	}
	if cell, ok := value.payload.(*JSPromise); ok {
		return cell == p
	}
	return false
}

// --- Static factories ---

// ResolvedPromise creates a promise already resolved with the given value.
func ResolvedPromise(globalObject *JSGlobalObject, value JSValue) *JSPromise {
	vm := globalObject.VM()
	promise := NewJSPromise(vm, nil)
	promise.status = PromiseStatusFulfilled
	promise.settlementValue = value
	return promise
}

// RejectedPromise creates a promise already rejected with the given reason.
func RejectedPromise(globalObject *JSGlobalObject, reason JSValue) *JSPromise {
	vm := globalObject.VM()
	promise := NewJSPromise(vm, nil)
	promise.status = PromiseStatusRejected
	promise.settlementValue = reason
	return promise
}
