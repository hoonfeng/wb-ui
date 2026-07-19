// Translation of: Source/JavaScriptCore/runtime/IteratorOperations.h
//
// IteratorOperations provides helper functions for ES iteration protocol:
//   iteratorNext, iteratorValue, iteratorComplete, iteratorStep, iteratorClose, etc.

package runtime

// IterationRecord stores an iterator and its .next() method.
type IterationRecord struct {
	Iterator   JSValue
	NextMethod JSValue
}

// iteratorNext calls the iterator's .next() method with an optional argument.
func iteratorNext(globalObject *JSGlobalObject, record IterationRecord, argument JSValue) JSValue {
	if record.NextMethod.IsCallable() {
		if fn, ok := record.NextMethod.payload.(*JSFunction); ok {
			args := []JSValue{}
			if argument.Tag() != TagUndefined {
				args = append(args, argument)
			}
			result, err := fn.Call(globalObject, JSValueUndefined, args)
			if err == nil {
				return result
			}
		}
	}
	return JSValueUndefined
}

// iteratorValue extracts the `value` property from an iterator result.
func iteratorValue(globalObject *JSGlobalObject, iterResult JSValue) JSValue {
	if iterResult.IsObject() {
		if obj, ok := iterResult.payload.(*JSObject); ok {
			return obj.Get(globalObject, NewPropertyName("value"))
		}
	}
	return JSValueUndefined
}

// iteratorComplete extracts the `done` property from an iterator result.
func iteratorComplete(globalObject *JSGlobalObject, iterResult JSValue) bool {
	if iterResult.IsObject() {
		if obj, ok := iterResult.payload.(*JSObject); ok {
			done := obj.Get(globalObject, NewPropertyName("done"))
			return done.ToBoolean()
		}
	}
	return false
}

// iteratorStep calls .next() and returns the result if not done, or false if done.
func iteratorStep(globalObject *JSGlobalObject, record IterationRecord) JSValue {
	result := iteratorNext(globalObject, record, JSValueUndefined)
	if globalObject.VM().Exception != nil {
		return JSValueUndefined
	}
	if iteratorComplete(globalObject, result) {
		return JSValueFalse
	}
	return result
}

// iteratorClose closes an iterator (calls .return() if available).
func iteratorClose(globalObject *JSGlobalObject, iterator JSValue) {
	if !iterator.IsObject() {
		return
	}

	var returnFn JSValue
	if it, ok := iterator.payload.(*JSObject); ok {
		returnFn = it.Get(globalObject, NewPropertyName("return"))
	}

	if returnFn.IsCallable() {
		if fn, ok := returnFn.payload.(*JSFunction); ok {
			fn.Call(globalObject, iterator, nil)
		}
	}
}

// createIterationResultObject creates {value, done} iteration result object.
func createIterationResultObject(vm *VM, value JSValue, done bool) *JSObject {
	obj := constructEmptyObject(vm, nil)
	obj.putDirectWithoutTransition(vm, NewPropertyName("value"), value, 0)
	doneVal := JSValueFalse
	if done {
		doneVal = JSValueTrue
	}
	obj.putDirectWithoutTransition(vm, NewPropertyName("done"), doneVal, 0)
	return obj
}

// getIterator retrieves the iterator from an iterable (calls @@iterator method).
func getIterator(globalObject *JSGlobalObject, iterable JSValue) IterationRecord {
	vm := globalObject.VM()

	var iteratorFn JSValue
	if obj, ok := iterable.payload.(*JSObject); ok {
		iteratorFn = obj.Get(globalObject, NewPropertyName(SymbolIterator))
	}

	if !iteratorFn.IsCallable() {
		vm.ThrowException(globalObject, "TypeError: object is not iterable")
		return IterationRecord{}
	}

	var iterator JSValue
	if fn, ok := iteratorFn.payload.(*JSFunction); ok {
		result, err := fn.Call(globalObject, iterable, nil)
		if err != nil {
			vm.ThrowException(globalObject, err.Error())
			return IterationRecord{}
		}
		iterator = result
	}

	var nextMethod JSValue
	if it, ok := iterator.payload.(*JSObject); ok {
		nextMethod = it.Get(globalObject, NewPropertyName("next"))
	}

	return IterationRecord{
		Iterator:   iterator,
		NextMethod: nextMethod,
	}
}
