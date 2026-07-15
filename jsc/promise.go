// Translation of: Source/JavaScriptCore/runtime/JSPromise.cpp
//                  Source/JavaScriptCore/runtime/JSPromise.h
// Completeness: 50%
// Simplifications:
//   - No microtask queue: callbacks are invoked synchronously.
//   - No PromiseReaction / deferred chaining.
//   - Promise.all / Promise.race accept only plain JS arrays.
//   - No unhandled rejection tracking.

package jsc

// promiseState mirrors [[PromiseState]].
type promiseState int

const (
	promisePending   promiseState = iota
	promiseFulfilled
	promiseRejected
)

// promiseCallback records a pending reaction registered via then().
type promiseCallback struct {
	onFulfilled JSValue
	onRejected  JSValue
	child       *promiseData
}

// promiseData holds the internal state of a Promise instance.
type promiseData struct {
	state     promiseState
	value     JSValue
	callbacks []*promiseCallback
}

func newPromiseData() *promiseData {
	return &promiseData{state: promisePending}
}

// settle transitions the promise and runs pending callbacks synchronously.
func (pd *promiseData) settle(state promiseState, value JSValue, in *Interpreter) {
	if pd.state != promisePending {
		return
	}
	pd.state = state
	pd.value = value
	for _, cb := range pd.callbacks {
		in.runPromiseCallback(cb, value, state)
	}
	pd.callbacks = nil
}

// runPromiseCallback invokes the appropriate handler and settles the child.
func (in *Interpreter) runPromiseCallback(cb *promiseCallback, value JSValue, state promiseState) {
	var handler JSValue
	if state == promiseFulfilled {
		handler = cb.onFulfilled
	} else {
		handler = cb.onRejected
	}
	if handler.IsFunction() {
		result, exc := in.callValue(handler, Undefined(), []JSValue{value})
		if exc != nil {
			cb.child.settle(promiseRejected, exc.value, in)
			return
		}
		cb.child.settle(promiseFulfilled, result, in)
	} else {
		if state == promiseFulfilled {
			cb.child.settle(promiseFulfilled, value, in)
		} else {
			cb.child.settle(promiseRejected, value, in)
		}
	}
}

// promiseDataOf extracts the promiseData from a JSValue.
func promiseDataOf(v JSValue) *promiseData {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	if o.Internal == nil {
		return nil
	}
	pd, ok := o.Internal.(*promiseData)
	if !ok {
		return nil
	}
	return pd
}

// newPromiseObject creates a Promise JSObject with the given internal data.
func newPromiseObject(pd *promiseData, in *Interpreter) *JSObject {
	obj := NewObject(in.PromisePrototype())
	obj.ClassName = "Promise"
	obj.Internal = pd
	return obj
}

// PromisePrototype returns the Promise.prototype object.
func (in *Interpreter) PromisePrototype() *JSObject {
	if in.promiseProto != nil {
		return in.promiseProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "Promise"

	// then(onFulfilled, onRejected)
	proto.Set("then", FunctionValue(NewNativeFunction("then", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		pd := promiseDataOf(this)
		if pd == nil {
			return Undefined()
		}
		onFulfilled := Undefined()
		onRejected := Undefined()
		if len(args) > 0 {
			onFulfilled = args[0]
		}
		if len(args) > 1 {
			onRejected = args[1]
		}
		child := newPromiseData()
		cb := &promiseCallback{
			onFulfilled: onFulfilled,
			onRejected:  onRejected,
			child:       child,
		}
		if pd.state == promisePending {
			pd.callbacks = append(pd.callbacks, cb)
		} else {
			in.runPromiseCallback(cb, pd.value, pd.state)
		}
		return ObjectValue(newPromiseObject(child, in))
	}, 2)))

	// catch(onRejected) — then(undefined, onRejected)
	proto.Set("catch", FunctionValue(NewNativeFunction("catch", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		onRejected := Undefined()
		if len(args) > 0 {
			onRejected = args[0]
		}
		pd := promiseDataOf(this)
		if pd == nil {
			return Undefined()
		}
		child := newPromiseData()
		cb := &promiseCallback{
			onFulfilled: Undefined(),
			onRejected:  onRejected,
			child:       child,
		}
		if pd.state == promisePending {
			pd.callbacks = append(pd.callbacks, cb)
		} else {
			in.runPromiseCallback(cb, pd.value, pd.state)
		}
		return ObjectValue(newPromiseObject(child, in))
	}, 1)))

	// finally(onFinally) — calls onFinally and passes through original value/reason.
	proto.Set("finally", FunctionValue(NewNativeFunction("finally", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		pd := promiseDataOf(this)
		if pd == nil {
			return Undefined()
		}
		onFinally := Undefined()
		if len(args) > 0 {
			onFinally = args[0]
		}
		child := newPromiseData()

		if pd.state == promisePending {
			var wrapFulfilled, wrapRejected JSValue
			if onFinally.IsCallable() {
				// Wrapper for fulfilled: call onFinally, then fulfill child with original value.
				wrapFulfilled = FunctionValue(NewNativeFunction("", func(in2 *Interpreter, _ JSValue, args2 []JSValue) JSValue {
					origVal := Undefined()
					if len(args2) > 0 {
						origVal = args2[0]
					}
					_, exc := in2.callValue(onFinally, Undefined(), nil)
					if exc != nil {
						child.settle(promiseRejected, exc.value, in2)
					} else {
						child.settle(promiseFulfilled, origVal, in2)
					}
					return Undefined()
				}, 1))
				// Wrapper for rejected: call onFinally, then reject child with original reason.
				wrapRejected = FunctionValue(NewNativeFunction("", func(in2 *Interpreter, _ JSValue, args2 []JSValue) JSValue {
					origReason := Undefined()
					if len(args2) > 0 {
						origReason = args2[0]
					}
					_, exc := in2.callValue(onFinally, Undefined(), nil)
					if exc != nil {
						child.settle(promiseRejected, exc.value, in2)
					} else {
						child.settle(promiseRejected, origReason, in2)
					}
					return Undefined()
				}, 1))
			}
			cb := &promiseCallback{
				onFulfilled: wrapFulfilled,
				onRejected:  wrapRejected,
				child:       child,
			}
			pd.callbacks = append(pd.callbacks, cb)
		} else {
			if onFinally.IsCallable() {
				_, exc := in.callValue(onFinally, Undefined(), nil)
				if exc != nil {
					child.settle(promiseRejected, exc.value, in)
					return ObjectValue(newPromiseObject(child, in))
				}
			}
			child.settle(pd.state, pd.value, in)
		}
		return ObjectValue(newPromiseObject(child, in))
	}, 1)))

	in.promiseProto = proto
	return proto
}

// PromiseConstructor returns the Promise constructor function.
func (in *Interpreter) PromiseConstructor() *JSFunction {
	return NewNativeFunction("Promise", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		pd := newPromiseData()

		resolve := NewNativeFunction("resolve", func(in2 *Interpreter, _ JSValue, args2 []JSValue) JSValue {
			val := Undefined()
			if len(args2) > 0 {
				val = args2[0]
			}
			pd.settle(promiseFulfilled, val, in2)
			return Undefined()
		}, 1)

		reject := NewNativeFunction("reject", func(in2 *Interpreter, _ JSValue, args2 []JSValue) JSValue {
			reason := Undefined()
			if len(args2) > 0 {
				reason = args2[0]
			}
			pd.settle(promiseRejected, reason, in2)
			return Undefined()
		}, 1)

		if len(args) > 0 && args[0].IsCallable() {
			_, exc := in.callValue(args[0], Undefined(), []JSValue{
				FunctionValue(resolve),
				FunctionValue(reject),
			})
			if exc != nil {
				pd.settle(promiseRejected, exc.value, in)
			}
		}
		return ObjectValue(newPromiseObject(pd, in))
	}, 1)
}

// staticResolve implements Promise.resolve(value).
func (in *Interpreter) staticResolve() *JSFunction {
	return NewNativeFunction("resolve", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		val := Undefined()
		if len(args) > 0 {
			val = args[0]
		}
		if promiseDataOf(val) != nil {
			return val
		}
		pd := newPromiseData()
		pd.settle(promiseFulfilled, val, in)
		return ObjectValue(newPromiseObject(pd, in))
	}, 1)
}

// staticReject implements Promise.reject(reason).
func (in *Interpreter) staticReject() *JSFunction {
	return NewNativeFunction("reject", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		reason := Undefined()
		if len(args) > 0 {
			reason = args[0]
		}
		pd := newPromiseData()
		pd.settle(promiseRejected, reason, in)
		return ObjectValue(newPromiseObject(pd, in))
	}, 1)
}

// staticAll implements Promise.all(iterable).
func (in *Interpreter) staticAll() *JSFunction {
	return NewNativeFunction("all", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		result := newPromiseData()
		if len(args) == 0 || !args[0].IsObject() {
			result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, nil)), in)
			return ObjectValue(newPromiseObject(result, in))
		}
		arr := args[0].AsObject()
		if !arr.IsArray {
			result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, nil)), in)
			return ObjectValue(newPromiseObject(result, in))
		}
		n := len(arr.Elements)
		if n == 0 {
			result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, nil)), in)
			return ObjectValue(newPromiseObject(result, in))
		}
		results := make([]JSValue, n)
		remaining := n
		for i, elem := range arr.Elements {
			idx, val := i, elem
			pd := promiseDataOf(val)
			if pd == nil {
				results[idx] = val
				remaining--
				if remaining == 0 {
					result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, results)), in)
				}
				continue
			}
			if pd.state == promiseRejected {
				result.settle(promiseRejected, pd.value, in)
				return ObjectValue(newPromiseObject(result, in))
			}
			if pd.state == promiseFulfilled {
				results[idx] = pd.value
				remaining--
				if remaining == 0 {
					result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, results)), in)
				}
				continue
			}
			onFulfilled := FunctionValue(NewNativeFunction("", func(_ *Interpreter, _ JSValue, args2 []JSValue) JSValue {
				results[idx] = args2[0]
				remaining--
				if remaining == 0 {
					result.settle(promiseFulfilled, ObjectValue(NewArray(in.arrayProto, results)), in)
				}
				return Undefined()
			}, 1))
			onRejected := FunctionValue(NewNativeFunction("", func(_ *Interpreter, _ JSValue, args2 []JSValue) JSValue {
				result.settle(promiseRejected, args2[0], in)
				return Undefined()
			}, 1))
			pd.callbacks = append(pd.callbacks, &promiseCallback{
				onFulfilled: onFulfilled,
				onRejected:  onRejected,
				child:       newPromiseData(),
			})
		}
		return ObjectValue(newPromiseObject(result, in))
	}, 1)
}

// staticRace implements Promise.race(iterable).
func (in *Interpreter) staticRace() *JSFunction {
	return NewNativeFunction("race", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		result := newPromiseData()
		if len(args) == 0 || !args[0].IsObject() {
			return ObjectValue(newPromiseObject(result, in))
		}
		arr := args[0].AsObject()
		if !arr.IsArray || len(arr.Elements) == 0 {
			return ObjectValue(newPromiseObject(result, in))
		}
		for _, elem := range arr.Elements {
			val := elem
			pd := promiseDataOf(val)
			if pd == nil {
				result.settle(promiseFulfilled, val, in)
				return ObjectValue(newPromiseObject(result, in))
			}
			if pd.state != promisePending {
				result.settle(pd.state, pd.value, in)
				return ObjectValue(newPromiseObject(result, in))
			}
			onFulfilled := FunctionValue(NewNativeFunction("", func(_ *Interpreter, _ JSValue, args2 []JSValue) JSValue {
				result.settle(promiseFulfilled, args2[0], in)
				return Undefined()
			}, 1))
			onRejected := FunctionValue(NewNativeFunction("", func(_ *Interpreter, _ JSValue, args2 []JSValue) JSValue {
				result.settle(promiseRejected, args2[0], in)
				return Undefined()
			}, 1))
			pd.callbacks = append(pd.callbacks, &promiseCallback{
				onFulfilled: onFulfilled,
				onRejected:  onRejected,
				child:       newPromiseData(),
			})
		}
		return ObjectValue(newPromiseObject(result, in))
	}, 1)
}
