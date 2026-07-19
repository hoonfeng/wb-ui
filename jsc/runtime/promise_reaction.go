// Translation of: Source/JavaScriptCore/runtime/JSPromiseReaction.h
//
// JSPromiseReaction is the reaction record used by Promise.then/.catch/.finally.
// It forms a linked list of reactions for a pending Promise's settlement.

package runtime

// JSPromiseReaction corresponds to JSC::JSPromiseReaction.
// Base reaction node in a linked list attached to a pending Promise.
type JSPromiseReaction struct {
	JSCell
	promise         JSValue             // the promise this reaction belongs to
	next            *JSPromiseReaction  // next reaction in the linked list
	internalMicrotask InternalMicrotask // the microtask type
}

// NewJSPromiseReaction creates a new base promise reaction.
func NewJSPromiseReaction(vm *VM, promise JSValue, next *JSPromiseReaction, task InternalMicrotask) *JSPromiseReaction {
	r := &JSPromiseReaction{
		promise:          promise,
		next:             next,
		internalMicrotask: task,
	}
	r.structureID = 0
	r.typ = JSSlimPromiseReactionType
	r.cellState = DefinitelyWhite
	_ = vm
	return r
}

// Promise returns the promise this reaction is attached to.
func (r *JSPromiseReaction) Promise() JSValue {
	return r.promise
}

// Next returns the next reaction in the linked list.
func (r *JSPromiseReaction) Next() *JSPromiseReaction {
	return r.next
}

// SetNext sets the next reaction in the linked list.
func (r *JSPromiseReaction) SetNext(next *JSPromiseReaction) {
	r.next = next
}

// InternalTask returns the internal microtask type.
func (r *JSPromiseReaction) InternalTask() InternalMicrotask {
	return r.internalMicrotask
}

// ---

// JSSlimPromiseReaction corresponds to JSC::JSSlimPromiseReaction.
// A compact reaction that stores either a handler or context+microtask.
type JSSlimPromiseReaction struct {
	JSPromiseReaction
	isFulfillHandler   bool
	handlerOrContext   JSValue // handler function or context value
}

// NewJSSlimPromiseReaction creates a slim promise reaction with a handler.
func NewJSSlimPromiseReaction(vm *VM, promise JSValue, handler JSValue, isFulfill bool, next *JSPromiseReaction) *JSSlimPromiseReaction {
	r := &JSSlimPromiseReaction{
		isFulfillHandler: isFulfill,
		handlerOrContext: handler,
	}
	r.promise = promise
	r.next = next
	r.internalMicrotask = InternalMicrotaskNone
	if isFulfill {
		r.internalMicrotask = InternalMicrotaskPromiseResolveThenableJobFast
	}
	r.structureID = 0
	r.typ = JSSlimPromiseReactionType
	r.cellState = DefinitelyWhite
	_ = vm
	return r
}

// NewJSSlimPromiseReactionWithMicrotask creates a slim reaction with an internal microtask.
func NewJSSlimPromiseReactionWithMicrotask(vm *VM, promise JSValue, task InternalMicrotask, context JSValue, next *JSPromiseReaction) *JSSlimPromiseReaction {
	r := &JSSlimPromiseReaction{
		isFulfillHandler: false,
		handlerOrContext: context,
	}
	r.promise = promise
	r.next = next
	r.internalMicrotask = task
	r.structureID = 0
	r.typ = JSSlimPromiseReactionType
	r.cellState = DefinitelyWhite
	_ = vm
	return r
}

// IsFulfillHandler returns true if this reaction is a fulfill handler.
func (r *JSSlimPromiseReaction) IsFulfillHandler() bool {
	return r.isFulfillHandler
}

// HandlerOrContext returns the handler function or context value.
func (r *JSSlimPromiseReaction) HandlerOrContext() JSValue {
	return r.handlerOrContext
}

// ---

// JSFullPromiseReaction corresponds to JSC::JSFullPromiseReaction.
// Full reaction with separate onFulfilled, onRejected, and context.
type JSFullPromiseReaction struct {
	JSPromiseReaction
	onFulfilled JSValue
	onRejected  JSValue
	context     JSValue
}

// NewJSFullPromiseReaction creates a full promise reaction.
func NewJSFullPromiseReaction(vm *VM, promise JSValue, onFulfilled, onRejected, context JSValue, next *JSPromiseReaction) *JSFullPromiseReaction {
	r := &JSFullPromiseReaction{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		context:     context,
	}
	r.promise = promise
	r.next = next
	r.structureID = 0
	r.typ = JSFullPromiseReactionType
	r.cellState = DefinitelyWhite
	_ = vm
	return r
}

// OnFulfilled returns the fulfill handler.
func (r *JSFullPromiseReaction) OnFulfilled() JSValue {
	return r.onFulfilled
}

// OnRejected returns the reject handler.
func (r *JSFullPromiseReaction) OnRejected() JSValue {
	return r.onRejected
}

// Context returns the context value.
func (r *JSFullPromiseReaction) Context() JSValue {
	return r.context
}
