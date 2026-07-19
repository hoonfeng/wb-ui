// Translation of: Source/JavaScriptCore/runtime/JSPromise.h
//                  Source/JavaScriptCore/runtime/JSPromise.cpp
//
// JSPromise implements the ECMAScript Promise object.
// Manages state (Pending/Fulfilled/Rejected), reactions, and settlement logic.
//
// JSPromise stores its state in two fields:
//   packedCell: *JSCell (may be nil) — payload cell pointer
//   packedFlags: uint16 — flags (see Flags layout below)
//   slot: JSValue — settlement value or handler/context

package runtime

// JSPromise corresponds to JSC::JSPromise.
type JSPromise struct {
	JSNonFinalObject
	packedCell  *JSCell // payload cell pointer (reaction or result promise)
	packedFlags uint16  // flags (status + inline reaction info)
	slot        JSValue // settlement value, handler, or context
}

// --- Flags layout (upper 16 bits of m_packed) ---
//   bits 0-1:   Status
//   bit 2:      isHandled
//   bit 3:      isFirstResolvingFunctionCalled
//   bits 4-5:   InlineReactionKind (4 values, 2 bits)
//   bits 6-13:  InternalMicrotask (only when kind == InternalMicrotask)
//   bits 14-15: reserved

// JSPromiseStatus is the status of a Promise.
type JSPromiseStatus uint16

const (
	PromiseStatusPending   JSPromiseStatus = 0
	PromiseStatusFulfilled JSPromiseStatus = 1
	PromiseStatusRejected  JSPromiseStatus = 2
)

// InlineReactionKind corresponds to JSC::JSPromise::InlineReactionKind.
type InlineReactionKind uint8

const (
	InlineReactionNone              InlineReactionKind = 0
	InlineReactionInternalMicrotask InlineReactionKind = 1
	InlineReactionFulfillHandler    InlineReactionKind = 2
	InlineReactionRejectHandler     InlineReactionKind = 3
)

// Flag masks
const (
	promiseStateMask                          uint16 = 0b0000000000000011
	promiseIsHandledFlag                      uint16 = 0b0000000000000100
	promiseIsFirstResolvingFunctionCalledFlag uint16 = 0b0000000000001000
	promiseInlineReactionKindMask             uint16 = 0b0000000000110000
	promiseInlineReactionMicrotaskMask        uint16 = 0b0011111111000000
	promiseInlineReactionKindShift            uint   = 4
	promiseInlineReactionMicrotaskShift       uint   = 6
)

// --- Factory methods ---

// NewJSPromise creates a new Promise in the Pending state.
func NewJSPromise(vm *VM, structure *Structure) *JSPromise {
	p := &JSPromise{
		packedFlags: uint16(PromiseStatusPending),
	}
	p.structureID = structure.structureID
	p.typ = JSPromiseType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = vm
	return p
}

// --- Flag accessors ---

func (p *JSPromise) flags() uint16 {
	return p.packedFlags
}

// Status returns the current promise status.
func (p *JSPromise) Status() JSPromiseStatus {
	return JSPromiseStatus(p.packedFlags & promiseStateMask)
}

// IsHandled returns whether this promise has a rejection handler.
func (p *JSPromise) IsHandled() bool {
	return p.packedFlags&promiseIsHandledFlag != 0
}

// MarkAsHandled marks this promise as having a rejection handler.
func (p *JSPromise) MarkAsHandled() {
	p.packedFlags = p.packedFlags | promiseIsHandledFlag
}

// IsFirstResolvingFunctionCalled returns whether resolve/reject has been called.
func (p *JSPromise) IsFirstResolvingFunctionCalled() bool {
	return p.packedFlags&promiseIsFirstResolvingFunctionCalledFlag != 0
}

// SettlementValue returns the settlement value (only valid after settlement).
func (p *JSPromise) SettlementValue() JSValue {
	return p.slot
}

// Result returns the promise's result — undefined if pending, the settlement value otherwise.
func (p *JSPromise) Result() JSValue {
	if p.Status() == PromiseStatusPending {
		return JSValueUndefined
	}
	return p.slot
}

// --- Inline reaction accessors ---

func (p *JSPromise) inlineReactionKind() InlineReactionKind {
	return InlineReactionKind((p.packedFlags & promiseInlineReactionKindMask) >> promiseInlineReactionKindShift)
}

func (p *JSPromise) hasInlineReaction() bool {
	return p.inlineReactionKind() != InlineReactionNone
}

func (p *JSPromise) hasInlineHandlerReaction() bool {
	kind := p.inlineReactionKind()
	return kind == InlineReactionFulfillHandler || kind == InlineReactionRejectHandler
}

func (p *JSPromise) inlineReactionMicrotask() InternalMicrotask {
	return InternalMicrotask((p.packedFlags & promiseInlineReactionMicrotaskMask) >> promiseInlineReactionMicrotaskShift)
}

func (p *JSPromise) payloadCell() *JSCell {
	return p.packedCell
}

// --- Setters ---

func (p *JSPromise) setFlags(newFlags uint16) {
	p.packedFlags = newFlags
}

func (p *JSPromise) setPackedCell(vm *VM, newFlags uint16, cell *JSCell) {
	p.packedCell = cell
	p.packedFlags = newFlags
	_ = vm
}

func (p *JSPromise) setSlot(vm *VM, value JSValue) {
	p.slot = value
	_ = vm
}

func (p *JSPromise) clearSlot() {
	p.slot = JSValueUndefined
}

// --- Public API ---

// Resolve implements Promise resolution.
func (p *JSPromise) Resolve(globalObject *JSGlobalObject, vm *VM, value JSValue) {
	if !p.IsFirstResolvingFunctionCalled() {
		p.setFlags(p.flags() | promiseIsFirstResolvingFunctionCalledFlag)
		p.resolvePromise(globalObject, vm, value)
	}
}

// Reject transitions to Rejected with the given reason.
func (p *JSPromise) Reject(vm *VM, reason JSValue) {
	if !p.IsFirstResolvingFunctionCalled() {
		p.setFlags(p.flags() | promiseIsFirstResolvingFunctionCalledFlag)
		p.rejectPromise(vm, reason)
	}
}

// RejectAsHandled rejects and marks as handled.
func (p *JSPromise) RejectAsHandled(vm *VM, value JSValue) {
	if !p.IsFirstResolvingFunctionCalled() {
		p.MarkAsHandled()
		p.Reject(vm, value)
	}
}

// Fulfill transitions to Fulfilled with the given value.
func (p *JSPromise) Fulfill(vm *VM, value JSValue) {
	if !p.IsFirstResolvingFunctionCalled() {
		p.setFlags(p.flags() | promiseIsFirstResolvingFunctionCalledFlag)
		p.fulfillPromise(vm, value)
	}
}

// PipeFrom pipes settlement from another promise to this one via internal microtask.
func (p *JSPromise) PipeFrom(vm *VM, from *JSPromise) {
	if p.IsFirstResolvingFunctionCalled() {
		return
	}
	p.setFlags(p.flags() | promiseIsFirstResolvingFunctionCalledFlag)
	from.PerformPromiseThenWithInternalMicrotask(vm, InternalMicrotaskPromiseFulfillWithoutHandlerJob, nil, JSValueUndefined)
}

// RejectWithCaughtException rejects with the current thrown exception.
func (p *JSPromise) RejectWithCaughtException(vm *VM) *JSPromise {
	p.rejectPromise(vm, JSValueUndefined)
	return p
}

// Then implements the ES then() method.
func (p *JSPromise) Then(globalObject *JSGlobalObject, onFulfilled, onRejected JSValue) *JSPromise {
	vm := globalObject.VM()
	resultPromise := NewJSPromise(vm, nil)
	resultCapability := NewJSValueObject(&resultPromise.JSNonFinalObject.JSObject)
	p.PerformPromiseThen(vm, globalObject, onFulfilled, onRejected, resultCapability)
	return resultPromise
}

// PerformPromiseThen is the core [[PerformPromiseThen]] operation.
func (p *JSPromise) PerformPromiseThen(vm *VM, globalObject *JSGlobalObject, onFulfilled, onRejected, promiseOrCapability JSValue) {
	fulfilledCallable := onFulfilled.IsCallable()
	rejectedCallable := onRejected.IsCallable()

	switch p.Status() {
	case PromiseStatusPending:
		onlyFulfill := fulfilledCallable && !rejectedCallable
		onlyReject := !fulfilledCallable && rejectedCallable

		if !p.hasInlineReaction() && p.packedCell == nil {
			if (onlyFulfill || onlyReject) && (promiseOrCapability.IsCell()) {
				if resultPromise, ok := promiseOrCapability.payload.(*JSPromise); ok {
					if onlyFulfill {
						p.setInlineHandlerReaction(vm, InlineReactionFulfillHandler, resultPromise, onFulfilled)
					} else {
						p.setInlineHandlerReaction(vm, InlineReactionRejectHandler, resultPromise, onRejected)
					}
					break
				}
			}
		}

		existing := p.reactionHead(vm)
		var reaction *JSPromiseReaction
		if onlyFulfill {
			reaction = &NewJSSlimPromiseReaction(vm, promiseOrCapability, onFulfilled, true, existing).JSPromiseReaction
		} else if onlyReject {
			reaction = &NewJSSlimPromiseReaction(vm, promiseOrCapability, onRejected, false, existing).JSPromiseReaction
		} else if fulfilledCallable {
			reaction = &NewJSFullPromiseReaction(vm, promiseOrCapability, onFulfilled, onRejected, JSValueUndefined, existing).JSPromiseReaction
		} else {
			reaction = &NewJSSlimPromiseReactionWithMicrotask(vm, promiseOrCapability, InternalMicrotaskPromiseResolveWithoutHandlerJob, JSValueUndefined, existing).JSPromiseReaction
		}
		p.setPackedCell(vm, p.flags()|promiseIsHandledFlag, &reaction.JSCell)
		break

	case PromiseStatusRejected:
		settled := p.SettlementValue()
		if !p.IsHandled() {
			// TODO: PromiseRejectionTracker(Handle) would be called here
		}
		if rejectedCallable {
			// TODO: globalObject.QueueMicrotask(vm, PromiseReactionJob, ...)
			_ = settled
			_ = globalObject
		} else {
			// TODO: globalObject.QueueMicrotask(vm, PromiseResolveWithoutHandlerJob, ...)
		}
		p.MarkAsHandled()
		break

	case PromiseStatusFulfilled:
		settled := p.SettlementValue()
		if fulfilledCallable {
			// TODO: globalObject.QueueMicrotask(vm, PromiseReactionJob, ...)
			_ = settled
			_ = globalObject
		} else {
			// TODO: globalObject.QueueMicrotask(vm, PromiseResolveWithoutHandlerJob, ...)
		}
		break
	}
}

// PerformPromiseThenWithInternalMicrotask implements performPromiseThen with an internal microtask.
func (p *JSPromise) PerformPromiseThenWithInternalMicrotask(vm *VM, task InternalMicrotask, cell *JSCell, context JSValue) {
	cellValue := JSValueUndefined
	if cell != nil {
		cellValue = NewJSValue(cell)
	}

	switch p.Status() {
	case PromiseStatusPending:
		if !p.hasInlineReaction() && p.packedCell == nil {
			p.setInlineMicrotaskReaction(vm, task, cell, context)
			break
		}
		existing := p.reactionHead(vm)
		reaction := &NewJSSlimPromiseReactionWithMicrotask(vm, cellValue, task, context, existing).JSPromiseReaction
		p.setPackedCell(vm, p.flags()|promiseIsHandledFlag, &reaction.JSCell)
		break

	case PromiseStatusRejected:
		settled := p.SettlementValue()
		if !p.IsHandled() {
			// TODO: PromiseRejectionTracker
		}
		// TODO: globalObject.QueueMicrotask(vm, task, status, cellValue, settled, context)
		_ = settled
		p.MarkAsHandled()
		break

	case PromiseStatusFulfilled:
		settled := p.SettlementValue()
		// TODO: globalObject.QueueMicrotask(vm, task, status, cellValue, settled, context)
		_ = settled
		break
	}
}

// PerformPromiseThenExported is the exported version for C API usage.
func (p *JSPromise) PerformPromiseThenExported(vm *VM, globalObject *JSGlobalObject, onFulfilled, onRejected, promiseOrCapability JSValue) {
	p.PerformPromiseThen(vm, globalObject, onFulfilled, onRejected, promiseOrCapability)
}

// --- Inline reaction helpers ---

func (p *JSPromise) setInlineMicrotaskReaction(vm *VM, task InternalMicrotask, cell *JSCell, context JSValue) {
	p.setSlot(vm, context)
	newFlags := p.flags() |
		promiseIsHandledFlag |
		(uint16(InlineReactionInternalMicrotask) << promiseInlineReactionKindShift) |
		(uint16(task) << promiseInlineReactionMicrotaskShift)
	p.setPackedCell(vm, newFlags, cell)
}

func (p *JSPromise) setInlineHandlerReaction(vm *VM, kind InlineReactionKind, resultPromise *JSPromise, handler JSValue) {
	p.setSlot(vm, handler)
	newFlags := p.flags() |
		promiseIsHandledFlag |
		(uint16(kind) << promiseInlineReactionKindShift)
	p.setPackedCell(vm, newFlags, &resultPromise.JSNonFinalObject.JSObject.JSCell)
}

// spillInlineReaction converts an inline reaction to a heap-allocated reaction.
func (p *JSPromise) spillInlineReaction(vm *VM) *JSPromiseReaction {
	kind := p.inlineReactionKind()
	var reaction *JSPromiseReaction
	switch kind {
	case InlineReactionInternalMicrotask:
		task := p.inlineReactionMicrotask()
		context := p.slot
		cell := p.packedCell
		cellValue := JSValueUndefined
		if cell != nil {
			cellValue = NewJSValue(cell)
		}
		r := NewJSSlimPromiseReactionWithMicrotask(vm, cellValue, task, context, nil)
		reaction = &r.JSPromiseReaction

	case InlineReactionFulfillHandler, InlineReactionRejectHandler:
		resultPromise := p.inlineHandlerResultPromise()
		handler := p.slot
		isFulfill := kind == InlineReactionFulfillHandler
		resultVal := JSValueUndefined
		if resultPromise != nil {
			resultVal = NewJSValueObject(&resultPromise.JSNonFinalObject.JSObject)
		}
		r := NewJSSlimPromiseReaction(vm, resultVal, handler, isFulfill, nil)
		reaction = &r.JSPromiseReaction

	default:
		return nil
	}
	p.clearSlot()
	newFlags := p.flags() & ^(promiseInlineReactionKindMask | promiseInlineReactionMicrotaskMask)
	p.setPackedCell(vm, newFlags, &reaction.JSCell)
	return reaction
}

// reactionHead returns the head reaction (spilling inline if necessary).
func (p *JSPromise) reactionHead(vm *VM) *JSPromiseReaction {
	if p.hasInlineReaction() {
		return p.spillInlineReaction(vm)
	}
	if p.packedCell != nil {
		// Type-assert packedCell to JSPromiseReaction
		if r, ok := interface{}(p.packedCell).(*JSPromiseReaction); ok {
			return r
		}
	}
	return nil
}

func (p *JSPromise) inlineHandlerResultPromise() *JSPromise {
	if p.packedCell != nil {
		if rp, ok := interface{}(p.packedCell).(*JSPromise); ok {
			return rp
		}
	}
	return nil
}

// --- Settlement logic ---

// rejectPromise transitions to Rejected state.
func (p *JSPromise) rejectPromise(vm *VM, argument JSValue) {
	globalObject := vm.GlobalObject
	currentFlags := p.flags()
	kind := InlineReactionKind((currentFlags & promiseInlineReactionKindMask) >> promiseInlineReactionKindShift)

	switch kind {
	case InlineReactionInternalMicrotask:
		p.settleInlineInternalMicrotask(vm, globalObject, PromiseStatusRejected, argument, currentFlags)
		return
	case InlineReactionFulfillHandler, InlineReactionRejectHandler:
		p.settleInlineHandler(vm, globalObject, PromiseStatusRejected, argument, currentFlags)
		return
	case InlineReactionNone:
		var reactions *JSPromiseReaction
		if p.packedCell != nil {
			if r, ok := interface{}(p.packedCell).(*JSPromiseReaction); ok {
				reactions = r
			}
		}
		settledFlags := currentFlags | uint16(PromiseStatusRejected)
		p.setSlot(vm, argument)
		p.setPackedCell(vm, settledFlags, nil)

		if !p.IsHandled() {
			// TODO: PromiseRejectionTracker(Reject) would be called
			_ = globalObject
		}

		if reactions == nil {
			return
		}
		triggerPromiseReactions(vm, globalObject, PromiseStatusRejected, reactions, argument)
		return
	}
}

// fulfillPromise transitions to Fulfilled state.
func (p *JSPromise) fulfillPromise(vm *VM, argument JSValue) {
	globalObject := vm.GlobalObject
	currentFlags := p.flags()
	kind := InlineReactionKind((currentFlags & promiseInlineReactionKindMask) >> promiseInlineReactionKindShift)

	switch kind {
	case InlineReactionInternalMicrotask:
		p.settleInlineInternalMicrotask(vm, globalObject, PromiseStatusFulfilled, argument, currentFlags)
		return
	case InlineReactionFulfillHandler, InlineReactionRejectHandler:
		p.settleInlineHandler(vm, globalObject, PromiseStatusFulfilled, argument, currentFlags)
		return
	case InlineReactionNone:
		var reactions *JSPromiseReaction
		if p.packedCell != nil {
			if r, ok := interface{}(p.packedCell).(*JSPromiseReaction); ok {
				reactions = r
			}
		}
		settledFlags := currentFlags | uint16(PromiseStatusFulfilled)
		p.setSlot(vm, argument)
		p.setPackedCell(vm, settledFlags, nil)

		if reactions == nil {
			return
		}
		triggerPromiseReactions(vm, globalObject, PromiseStatusFulfilled, reactions, argument)
		return
	}
}

// resolvePromise implements Promise resolving (ES 25.4.1.3.2).
func (p *JSPromise) resolvePromise(globalObject *JSGlobalObject, vm *VM, resolution JSValue) {
	// If resolution is the promise itself, reject with TypeError
	if resolution.IsCell() {
		if promise, ok := resolution.payload.(*JSPromise); ok && promise == p {
			p.rejectPromise(vm, JSValueUndefined) // Would create TypeError in full impl
			return
		}
	}

	// If not an object, fulfill directly
	if !resolution.IsObject() {
		p.fulfillPromise(vm, resolution)
		return
	}

	resolutionObj := resolution.ToObject(globalObject)
	if resolutionObj == nil {
		p.fulfillPromise(vm, resolution)
		return
	}

	// If it's a JSPromise and isThenFastAndNonObservable, fast path
	if promise, ok := resolution.payload.(*JSPromise); ok {
		if promise.isThenFastAndNonObservable() {
			// TODO: globalObject.QueueMicrotask(vm, PromiseResolveThenableJobFast, ...)
			_ = promise
			return
		}
	}

	// Check if definitely non-thenable
	if isDefinitelyNonThenable(resolutionObj, globalObject) {
		p.fulfillPromise(vm, resolution)
		return
	}

	// Get .then property
	then := resolutionObj.Get(globalObject, NewPropertyName("then"))
	if then.IsCallable() {
		// TODO: globalObject.QueueMicrotask(vm, PromiseResolveThenableJob, ...)
		_ = then
	} else {
		p.fulfillPromise(vm, resolution)
	}
}

// --- Inline reaction settlement ---

func (p *JSPromise) settleInlineInternalMicrotask(vm *VM, globalObject *JSGlobalObject, newStatus JSPromiseStatus, argument JSValue, flagsSnapshot uint16) {
	task := InternalMicrotask((flagsSnapshot & promiseInlineReactionMicrotaskMask) >> promiseInlineReactionMicrotaskShift)
	context := p.slot
	cell := p.packedCell
	cellValue := JSValueUndefined
	if cell != nil {
		cellValue = NewJSValue(cell)
	}
	settledFlags := (flagsSnapshot & ^(promiseInlineReactionKindMask | promiseInlineReactionMicrotaskMask)) | uint16(newStatus)
	p.setSlot(vm, argument)
	p.setPackedCell(vm, settledFlags, nil)
	// TODO: globalObject.QueueMicrotask(vm, task, uint8(newStatus), cellValue, argument, context)
	_ = cellValue
	_ = globalObject
	_ = task
	_ = context
}

func (p *JSPromise) settleInlineHandler(vm *VM, globalObject *JSGlobalObject, newStatus JSPromiseStatus, argument JSValue, flagsSnapshot uint16) {
	kind := InlineReactionKind((flagsSnapshot & promiseInlineReactionKindMask) >> promiseInlineReactionKindShift)
	settledIsFulfilled := newStatus == PromiseStatusFulfilled
	handlerIsFulfill := kind == InlineReactionFulfillHandler
	resultPromise := p.inlineHandlerResultPromise()
	handler := p.slot
	settledFlags := (flagsSnapshot & ^(promiseInlineReactionKindMask | promiseInlineReactionMicrotaskMask)) | uint16(newStatus)
	p.setSlot(vm, argument)
	p.setPackedCell(vm, settledFlags, nil)
	resultVal := JSValueUndefined
	if resultPromise != nil {
		resultVal = NewJSValueObject(&resultPromise.JSNonFinalObject.JSObject)
	}
	// TODO: QueueMicrotask with PromiseReactionJob or PromiseResolveWithoutHandlerJob
	_ = settledIsFulfilled
	_ = handlerIsFulfill
	_ = handler
	_ = resultVal
	_ = globalObject
}

// triggerPromiseReactions fires all pending reactions after settlement.
func triggerPromiseReactions(vm *VM, globalObject *JSGlobalObject, status JSPromiseStatus, head *JSPromiseReaction, argument JSValue) {
	isResolved := status == PromiseStatusFulfilled

	// Single reaction fast path
	if head.Next() == nil {
		// TODO: queue microtask
		_ = isResolved
		_ = argument
		_ = globalObject
		_ = vm
		return
	}

	// Reverse the singly-linked list
	var previous *JSPromiseReaction
	current := head
	for current != nil {
		next := current.Next()
		current.SetNext(next) // simplified: no write barrier
		previous = current
		current = next
	}
	head = previous

	// Queue all reactions in forward order
	for current = head; current != nil; current = current.Next() {
		// TODO: queue microtask for each reaction
		_ = current
	}
}

// --- Static factories ---

// ResolvedPromise creates a promise already resolved with the given value (static).
func ResolvedPromise(globalObject *JSGlobalObject, value JSValue) *JSPromise {
	vm := globalObject.VM()
	promise := NewJSPromise(vm, nil)
	promise.Resolve(globalObject, vm, value)
	return promise
}

// RejectedPromise creates a promise already rejected with the given reason (static).
func RejectedPromise(globalObject *JSGlobalObject, reason JSValue) *JSPromise {
	vm := globalObject.VM()
	promise := NewJSPromise(vm, nil)
	promise.rejectPromise(vm, reason)
	return promise
}

// PromiseResolve implements Promise.resolve.
func PromiseResolve(globalObject *JSGlobalObject, constructor *JSObject, argument JSValue) *JSObject {
	vm := globalObject.VM()

	if argument.IsCell() {
		if promise, ok := argument.payload.(*JSPromise); ok {
			if promise.isThenFastAndNonObservable() {
				_ = constructor
				return &promise.JSNonFinalObject.JSObject
			}
			// check .constructor
			ctor := promise.Get(globalObject, NewPropertyName("constructor"))
			if ctor == NewJSValueObject(constructor) {
				return &promise.JSNonFinalObject.JSObject
			}
		}
	}

	promise := NewJSPromise(vm, nil)
	promise.Resolve(globalObject, vm, argument)
	return &promise.JSNonFinalObject.JSObject
}

// PromiseReject implements Promise.reject.
func PromiseReject(globalObject *JSGlobalObject, constructor *JSObject, argument JSValue) *JSObject {
	vm := globalObject.VM()
	promise := NewJSPromise(vm, nil)
	promise.Reject(vm, argument)
	return &promise.JSNonFinalObject.JSObject
}

// --- Resolving functions ---

// CreateResolvingFunctions creates resolve/reject functions for a promise.
func (p *JSPromise) CreateResolvingFunctions(vm *VM, globalObject *JSGlobalObject) (*JSFunction, *JSFunction) {
	resolve := NewJSFunction(vm, globalObject, "resolve", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		p.resolvePromise(globalObject, globalObject.VM(), argument)
		return JSValueUndefined, nil
	})
	reject := NewJSFunction(vm, globalObject, "reject", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		p.rejectPromise(globalObject.VM(), argument)
		return JSValueUndefined, nil
	})
	resolve.properties["resolvingPromise"] = NewJSValueObject(&p.JSNonFinalObject.JSObject)
	resolve.properties["resolvingOther"] = NewJSValueObject(&reject.JSObject)
	reject.properties["resolvingPromise"] = NewJSValueObject(&p.JSNonFinalObject.JSObject)
	reject.properties["resolvingOther"] = NewJSValueObject(&resolve.JSObject)
	return resolve, reject
}

// CreateFirstResolveFunction creates the first resolve function.
func (p *JSPromise) CreateFirstResolveFunction(vm *VM, globalObject *JSGlobalObject) *JSFunction {
	return NewJSFunction(vm, globalObject, "resolve", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		p.Resolve(globalObject, globalObject.VM(), argument)
		return JSValueUndefined, nil
	})
}

// CreateFirstRejectFunction creates the first reject function.
func (p *JSPromise) CreateFirstRejectFunction(vm *VM, globalObject *JSGlobalObject) *JSFunction {
	return NewJSFunction(vm, globalObject, "reject", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		p.Reject(globalObject.VM(), argument)
		return JSValueUndefined, nil
	})
}

// CreateFirstResolvingFunctions creates the first resolve + reject pair.
func (p *JSPromise) CreateFirstResolvingFunctions(vm *VM, globalObject *JSGlobalObject) (*JSFunction, *JSFunction) {
	return p.CreateFirstResolveFunction(vm, globalObject), p.CreateFirstRejectFunction(vm, globalObject)
}

// CreateResolvingFunctionsWithInternalMicrotask creates resolving functions with an internal microtask.
func (p *JSPromise) CreateResolvingFunctionsWithInternalMicrotask(vm *VM, globalObject *JSGlobalObject, task InternalMicrotask, context JSValue) (*JSFunction, *JSFunction) {
	resolve := NewJSFunction(vm, globalObject, "resolve", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		ResolveWithInternalMicrotask(globalObject, globalObject.VM(), argument, task, context)
		return JSValueUndefined, nil
	})
	reject := NewJSFunction(vm, globalObject, "reject", 1, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		_ = thisValue
		argument := JSValueUndefined
		if len(args) > 0 {
			argument = args[0]
		}
		RejectWithInternalMicrotask(globalObject.VM(), globalObject, argument, task, context)
		return JSValueUndefined, nil
	})
	resolve.properties["resolvingWithInternalMicrotaskOther"] = NewJSValueObject(&reject.JSObject)
	reject.properties["resolvingWithInternalMicrotaskOther"] = NewJSValueObject(&resolve.JSObject)
	return resolve, reject
}

// --- Deferred / Capability ---

// CreateDeferredData creates a deferred data triple (Promise, Resolve, Reject).
func CreateDeferredData(globalObject *JSGlobalObject, promiseConstructor *JSPromiseConstructor) DeferredData {
	pCap, rCap, jCap := NewPromiseCapability(globalObject, NewJSValueObject(&promiseConstructor.InternalFunction.JSNonFinalObject.JSObject))
	if pCap == nil {
		return DeferredData{}
	}
	promise, ok1 := interface{}(pCap).(*JSPromise)
	resolve, ok2 := interface{}(rCap).(*JSFunction)
	reject, ok3 := interface{}(jCap).(*JSFunction)
	if ok1 && ok2 && ok3 {
		return DeferredData{Promise: promise, Resolve: resolve, Reject: reject}
	}
	return DeferredData{}
}

// DeferredData is a helper struct for creating deferred promises.
type DeferredData struct {
	Promise *JSPromise
	Resolve *JSFunction
	Reject  *JSFunction
}

// CreateNewPromiseCapability creates a new PromiseCapability object.
func CreateNewPromiseCapability(globalObject *JSGlobalObject, constructor JSValue) JSValue {
	promise, resolve, reject := NewPromiseCapability(globalObject, constructor)
	if promise == nil {
		return JSValueUndefined
	}
	return CreatePromiseCapabilityObject(globalObject, promise, resolve, reject)
}

// CreatePromiseCapabilityObject creates a capability record as a JSObject.
func CreatePromiseCapabilityObject(globalObject *JSGlobalObject, promise, resolve, reject *JSObject) JSValue {
	_ = globalObject
	capability := &JSObject{
		JSCell:     JSCell{typ: ObjectType, cellState: DefinitelyWhite},
		properties: make(map[string]JSValue),
	}
	capability.properties["resolve"] = NewJSValueObject(resolve)
	capability.properties["reject"] = NewJSValueObject(reject)
	capability.properties["promise"] = NewJSValueObject(promise)
	return NewJSValueObject(capability)
}

// NewPromiseCapability implements promise capability creation (ES spec).
func NewPromiseCapability(globalObject *JSGlobalObject, constructor JSValue) (promise, resolve, reject *JSObject) {
	vm := globalObject.VM()

	// Fast path: default Promise constructor
	_ = constructor
	// Simplified: always use default Promise constructor path
	p := NewJSPromise(vm, nil)
	resolveFn, rejectFn := p.CreateFirstResolvingFunctions(vm, globalObject)
	return &p.JSNonFinalObject.JSObject, &resolveFn.JSObject, &rejectFn.JSObject
}

// --- Static helpers ---

// ResolveWithInternalMicrotaskForAsyncAwait resolves with an internal microtask for async/await.
func ResolveWithInternalMicrotaskForAsyncAwait(globalObject *JSGlobalObject, vm *VM, resolution JSValue, task InternalMicrotask, context JSValue) {
	if resolution.IsCell() {
		if promise, ok := resolution.payload.(*JSPromise); ok {
			if promise.isThenFastAndNonObservable() {
				promise.PerformPromiseThenWithInternalMicrotask(vm, task, nil, context)
				return
			}
		}
	}
	ResolveWithInternalMicrotask(globalObject, vm, resolution, task, context)
}

// ResolveWithInternalMicrotask resolves with an internal microtask.
func ResolveWithInternalMicrotask(globalObject *JSGlobalObject, vm *VM, resolution JSValue, task InternalMicrotask, context JSValue) {
	if !resolution.IsObject() {
		FulfillWithInternalMicrotask(vm, globalObject, resolution, task, context)
		return
	}

	resolutionObj := resolution.ToObject(globalObject)
	if resolutionObj == nil {
		FulfillWithInternalMicrotask(vm, globalObject, resolution, task, context)
		return
	}

	if promise, ok := resolution.payload.(*JSPromise); ok {
		if promise.isThenFastAndNonObservable() {
			// TODO: QueueMicrotask with PromiseResolveThenableJobWithInternalMicrotaskFast
			_ = globalObject
			return
		}
	}

	if isDefinitelyNonThenable(resolutionObj, globalObject) {
		FulfillWithInternalMicrotask(vm, globalObject, resolution, task, context)
		return
	}

	then := resolutionObj.Get(globalObject, NewPropertyName("then"))
	if then.IsCallable() {
		// TODO: QueueMicrotask with PromiseResolveThenableJobWithInternalMicrotask
		_ = then
	} else {
		FulfillWithInternalMicrotask(vm, globalObject, resolution, task, context)
	}
}

// RejectWithInternalMicrotask rejects with an internal microtask.
func RejectWithInternalMicrotask(vm *VM, globalObject *JSGlobalObject, argument JSValue, task InternalMicrotask, context JSValue) {
	// TODO: globalObject.QueueMicrotask(vm, task, Rejected, undefined, argument, context)
	_ = globalObject
	_ = argument
	_ = task
	_ = context
}

// FulfillWithInternalMicrotask fulfills with an internal microtask.
func FulfillWithInternalMicrotask(vm *VM, globalObject *JSGlobalObject, argument JSValue, task InternalMicrotask, context JSValue) {
	// TODO: globalObject.QueueMicrotask(vm, task, Fulfilled, undefined, argument, context)
	_ = globalObject
	_ = argument
	_ = task
	_ = context
}

// --- Utility ---

// isThenFastAndNonObservable checks if .then is the unobservable fast version (simplified).
func (p *JSPromise) isThenFastAndNonObservable() bool {
	return true
}

// isDefinitelyNonThenable checks if an object is definitely not a thenable.
func isDefinitelyNonThenable(obj *JSObject, globalObject *JSGlobalObject) bool {
	_ = globalObject
	thenVal := obj.Get(globalObject, NewPropertyName("then"))
	return thenVal.IsUndefined()
}

// asyncStackTraceContext returns context for async stack traces.
func (p *JSPromise) asyncStackTraceContext() JSValue {
	if p.Status() == PromiseStatusPending {
		return JSValueUndefined
	}
	switch p.inlineReactionKind() {
	case InlineReactionNone:
		return JSValueUndefined
	case InlineReactionInternalMicrotask:
		if promiseReactionPacksGlobalContextAndIndex(p.inlineReactionMicrotask()) {
			if p.packedCell != nil {
				return NewJSValue(p.packedCell)
			}
		}
		return p.slot
	case InlineReactionFulfillHandler, InlineReactionRejectHandler:
		return JSValueUndefined
	}
	return JSValueUndefined
}
