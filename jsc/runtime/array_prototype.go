// Translation of: Source/JavaScriptCore/runtime/ArrayPrototype.h
//                  Source/JavaScriptCore/runtime/ArrayPrototype.cpp
//
// ArrayPrototype is the prototype for all JavaScript arrays (Array.prototype).

package runtime

import (
	"math"
	"sort"
	"strings"
)

// ArrayPrototype corresponds to JSC::ArrayPrototype.
// Implements all Array.prototype methods (ES 22.1.3).
type ArrayPrototype struct {
	JSNonFinalObject
}

// SpeciesWatchpointStatus corresponds to JSC::ArrayPrototype::SpeciesWatchpointStatus.
type SpeciesWatchpointStatus uint8

const (
	SpeciesWatchpointUninitialized SpeciesWatchpointStatus = iota
	SpeciesWatchpointInitialized
	SpeciesWatchpointFired
)

// NewArrayPrototype creates a new ArrayPrototype.
// In C++: static ArrayPrototype* create(VM&, JSGlobalObject*, Structure*)
func NewArrayPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *ArrayPrototype {
	p := &ArrayPrototype{}
	p.structureID = structure.structureID
	p.typ = ArrayType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.indexingTypeAndMisc = ArrayWithUndecided
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers all Array.prototype methods.
// In C++: void finishCreation(VM&, JSGlobalObject*)
func (p *ArrayPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.JSNonFinalObject.FinishCreation(vm)

	// === Native functions (registered here) ===

	// toString and values are set via globalObject helper functions (simplified — use placeholders here)
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("values"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolIterator), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Standard ES methods (JSC_NATIVE_FUNCTION_WITHOUT_TRANSITION)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("concat"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("fill"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("join"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("pop"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("push"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("reverse"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("shift"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("slice"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("sort"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("splice"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("unshift"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("indexOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("lastIndexOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("includes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("copyWithin"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("keys"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("entries"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// ES2023 immutable methods (registered here)
	p.putDirectWithoutTransition(vm, NewPropertyName("toReversed"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toSorted"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toSpliced"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("with"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// flat (registered here)
	p.putDirectWithoutTransition(vm, NewPropertyName("flat"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Builtin methods (forEach, map, filter, every, some, reduce, reduceRight, find, findIndex, flatMap, at)
	// — registered as placeholders; full implementations require JS builtins.
	p.putDirectWithoutTransition(vm, NewPropertyName("forEach"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("map"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("filter"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("every"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("some"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("reduce"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("reduceRight"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("find"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("findIndex"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("findLast"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("findLastIndex"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("flatMap"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("at"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// === @@unscopables ===
	// In C++: constructEmptyObject(vm, globalObject->nullPrototypeObjectStructure())
	unscopables := &JSObject{properties: make(map[string]JSValue)}
	unscopables.properties["at"] = NewJSValueBool(true)
	unscopables.properties["copyWithin"] = NewJSValueBool(true)
	unscopables.properties["entries"] = NewJSValueBool(true)
	unscopables.properties["fill"] = NewJSValueBool(true)
	unscopables.properties["find"] = NewJSValueBool(true)
	unscopables.properties["findIndex"] = NewJSValueBool(true)
	unscopables.properties["findLast"] = NewJSValueBool(true)
	unscopables.properties["findLastIndex"] = NewJSValueBool(true)
	unscopables.properties["flat"] = NewJSValueBool(true)
	unscopables.properties["flatMap"] = NewJSValueBool(true)
	unscopables.properties["includes"] = NewJSValueBool(true)
	unscopables.properties["keys"] = NewJSValueBool(true)
	unscopables.properties["toReversed"] = NewJSValueBool(true)
	unscopables.properties["toSorted"] = NewJSValueBool(true)
	unscopables.properties["toSpliced"] = NewJSValueBool(true)
	unscopables.properties["values"] = NewJSValueBool(true)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolUnscopables), NewJSValueObject(unscopables),
		PropertyAttributeDontEnum|PropertyAttributeReadOnly)

	_ = globalObject
}

// =====================================================================
// Helper functions
// =====================================================================

// toLength implements ES ToLength (7.1.15).
// Converts a value to an integer usable as an array length.
func toLength(globalObject *JSGlobalObject, thisObj *JSObject) uint64 {
	vm := globalObject.VM()
	lenVal := thisObj.Get(globalObject, NewPropertyName("length"))
	if lenVal.IsUndefined() {
		return 0
	}
	_ = vm

	// ES ToLength: ToInteger(len) clamped to [0, 2^53-1]
	lenNum := lenVal.ToNumber()
	if math.IsNaN(lenNum) || lenNum < 0 {
		return 0
	}
	if math.IsInf(lenNum, 1) {
		return maxArrayLength
	}
	lenInt := uint64(math.Floor(lenNum))
	if lenInt > maxArrayLength {
		return maxArrayLength
	}
	return lenInt
}

// setLength sets the "length" property of an object.
func setLength(globalObject *JSGlobalObject, vm *VM, thisObj *JSObject, length uint64) {
	_ = globalObject
	_ = vm
	thisObj.properties["length"] = NewJSValueNumber(float64(length))
}

// toIntegerOrInfinity implements ES ToIntegerOrInfinity.
func toIntegerOrInfinity(v JSValue) float64 {
	num := v.ToNumber()
	if math.IsNaN(num) {
		return 0
	}
	return math.Trunc(num)
}

// isJSArray checks if a JSValue is a JSArray.
func isJSArray(val JSValue) bool {
	if !val.IsObject() {
		return false
	}
	return val.GetObject().typ == ArrayType || val.GetObject().typ == DerivedArrayType
}

// asArray casts a JSValue to JSArray.
func asArray(val JSValue) *JSArray {
	return uncheckedDowncast[JSArray](val.GetObject())
}

// clampIndex clamps an integer index to [0, length].
func clampIndex(index int64, length uint64) uint64 {
	if index < 0 {
		return 0
	}
	uindex := uint64(index)
	if uindex > length {
		return length
	}
	return uindex
}

// argumentClampedIndexFromStartOrEnd resolves argument to a clamped index.
// If argument is undefined, returns undefinedValue.
// If argument is negative, it's relative to the end (when relativeNegativeIndex is true).
func argumentClampedIndexFromStartOrEnd(globalObject *JSGlobalObject, value JSValue, length uint64, undefinedValue uint64) uint64 {
	if value.IsUndefined() {
		return undefinedValue
	}
	val := value.ToNumber()
	if val < 0 {
		val += float64(length)
		if val < 0 {
			return 0
		}
		return uint64(math.Floor(val))
	}
	uval := uint64(math.Floor(val))
	if uval > length {
		return length
	}
	_ = globalObject
	return uval
}

// checkForHole checks if a value is a "hole" (not present) — simplified: undefined means hole.
func checkForHole(val JSValue) bool {
	return val.IsUndefined()
}

// =====================================================================
// Array.prototype method implementations (Host functions)
// =====================================================================
// Each function follows the pattern: func xxxx(globalObject, callFrame) -> JSValue
// In C++: JSC_DEFINE_HOST_FUNCTION(arrayProtoFuncXxx, ...)
// =====================================================================

// ---- toString ----

// arrayProtoFuncToString implements Array.prototype.toString()
func arrayProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	// 1. Let array = ToObject(this value)
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	// 2. Let func = Get(array, "join")
	joinFunc := thisObj.Get(globalObject, NewPropertyName("join"))
	if joinFunc.IsCallable() {
		// 4. Return Call(func, array)
		callData := getCallDataInline(joinFunc)
		return call(globalObject, joinFunc, callData, NewJSValueObject(thisObj), nil)
	}

	// 3. If IsCallable(func) is false, use Object.prototype.toString
	return objectPrototypeToString(globalObject, NewJSValueObject(thisObj))
}

// ---- Join ----

// arrayProtoFuncJoin implements Array.prototype.join(separator)
func arrayProtoFuncJoin(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)

	separator := ","
	if !callFrame.Argument(0).IsUndefined() {
		separator = callFrame.Argument(0).ToString()
		_ = vm
	}

	if length == 0 {
		return NewJSValueString("")
	}

	var parts []string
	for i := uint64(0); i < length; i++ {
		elem := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
		if !elem.IsUndefinedOrNull() {
			parts = append(parts, elem.ToString())
		} else {
			parts = append(parts, "")
		}
	}
	return NewJSValueString(strings.Join(parts, separator))
}

// ---- Values / Iterator protocol ----

// arrayProtoFuncValues implements Array.prototype.values() (returns iterator)
func arrayProtoFuncValues(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}
	_ = vm
	// Simplified: returns a new array with the same elements
	return NewJSValueObject(thisObj)
}

// arrayProtoFuncEntries implements Array.prototype.entries() (returns iterator)
func arrayProtoFuncEntries(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = globalObject
	_ = callFrame
	// Simplified skeleton: returns this as-is
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	return NewJSValueObject(thisValue.ToObject(globalObject))
}

// arrayProtoFuncKeys implements Array.prototype.keys() (returns iterator)
func arrayProtoFuncKeys(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = globalObject
	_ = callFrame
	// Simplified skeleton
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	return NewJSValueObject(thisValue.ToObject(globalObject))
}

// ---- Pop ----

// arrayProtoFuncPop implements Array.prototype.pop()
func arrayProtoFuncPop(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)

	// Fast path for JSArray
	if val := thisValue.GetObject(); val != nil && (val.typ == ArrayType || val.typ == DerivedArrayType) {
		array := uncheckedDowncast[JSArray](val)
		result := array.Pop(globalObject)
		_ = vm
		return JSValueEncode(result)
	}

	// Generic path for array-like objects
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}
	length := toLength(globalObject, thisObj)
	if length == 0 {
		setLength(globalObject, vm, thisObj, 0)
		return JSValueEncode(jsUndefined())
	}

	index := length - 1
	result := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(index))))
	thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(index))), nil)
	setLength(globalObject, vm, thisObj, index)
	return JSValueEncode(result)
}

// ---- Push ----

// arrayProtoFuncPush implements Array.prototype.push(...items)
func arrayProtoFuncPush(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)

	// Fast path for JSArray
	if val := thisValue.GetObject(); val != nil && (val.typ == ArrayType || val.typ == DerivedArrayType) {
		array := uncheckedDowncast[JSArray](val)
		for _, arg := range callFrame.Arguments() {
			array.Push(globalObject, arg)
		}
		_ = vm
		return JSValueEncode(jsNumber(float64(array.Length())))
	}

	// Generic path
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}
	length := toLength(globalObject, thisObj)
	args := callFrame.Arguments()

	for n := 0; n < len(args); n++ {
		thisObj.PutByIndex(nil, globalObject, uint32(length+uint64(n)), args[n], true)
	}

	newLength := length + uint64(len(args))
	setLength(globalObject, vm, thisObj, newLength)
	return JSValueEncode(jsNumber(float64(newLength)))
}

// ---- Reverse ----

// arrayProtoFuncReverse implements Array.prototype.reverse()
func arrayProtoFuncReverse(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)

	// Fast path for JSArray
	if val := thisValue.GetObject(); val != nil && (val.typ == ArrayType || val.typ == DerivedArrayType) {
		array := uncheckedDowncast[JSArray](val)
		elems := array.elements
		for i, j := 0, len(elems)-1; i < j; i, j = i+1, j-1 {
			elems[i], elems[j] = elems[j], elems[i]
		}
		_ = vm
		return JSValueEncode(NewJSValueObject(val))
	}

	// Generic path
	middle := length / 2
	for lower := uint64(0); lower < middle; lower++ {
		upper := length - 1 - lower
		lowerVal := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(lower))))
		upperVal := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(upper))))

		lowerExists := !lowerVal.IsUndefined()
		upperExists := !upperVal.IsUndefined()

		if lowerExists && upperExists {
			thisObj.PutByIndex(nil, globalObject, uint32(lower), upperVal, true)
			thisObj.PutByIndex(nil, globalObject, uint32(upper), lowerVal, true)
		} else if lowerExists {
			thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(lower))), nil)
			thisObj.PutByIndex(nil, globalObject, uint32(upper), lowerVal, true)
		} else if upperExists {
			thisObj.PutByIndex(nil, globalObject, uint32(lower), upperVal, true)
			thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(upper))), nil)
		}
	}
	_ = vm
	return JSValueEncode(NewJSValueObject(thisObj))
}

// ---- Shift ----

// arrayProtoFuncShift implements Array.prototype.shift()
func arrayProtoFuncShift(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)

	// Fast path for JSArray
	if val := thisValue.GetObject(); val != nil && (val.typ == ArrayType || val.typ == DerivedArrayType) {
		array := uncheckedDowncast[JSArray](val)
		result := array.Shift(globalObject)
		_ = vm
		return JSValueEncode(result)
	}

	// Generic path
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}
	length := toLength(globalObject, thisObj)
	if length == 0 {
		setLength(globalObject, vm, thisObj, 0)
		return JSValueEncode(jsUndefined())
	}

	first := thisObj.Get(globalObject, NewPropertyName("0"))

	for k := uint64(1); k < length; k++ {
		from := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(k))))
		toKey := k - 1
		if !from.IsUndefined() {
			thisObj.PutByIndex(nil, globalObject, uint32(toKey), from, true)
		} else {
			thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(toKey))), nil)
		}
	}

	thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(length-1))), nil)
	setLength(globalObject, vm, thisObj, length-1)
	_ = vm
	return JSValueEncode(first)
}

// ---- Slice ----

// arrayProtoFuncSlice implements Array.prototype.slice(start, end)
func arrayProtoFuncSlice(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)

	relStart := callFrame.Argument(0).ToNumber()
	var start uint64
	if relStart < 0 {
		temp := float64(length) + relStart
		if temp < 0 {
			start = 0
		} else {
			start = uint64(temp)
		}
	} else {
		start = uint64(relStart)
		if start > length {
			start = length
		}
	}

	var end uint64
	if callFrame.Argument(1).IsUndefined() {
		end = length
	} else {
		relEnd := callFrame.Argument(1).ToNumber()
		if relEnd < 0 {
			temp := float64(length) + relEnd
			if temp < 0 {
				end = 0
			} else {
				end = uint64(temp)
			}
		} else {
			end = uint64(relEnd)
			if end > length {
				end = length
			}
		}
	}

	// Fast path for JSArray
	if val := thisValue.GetObject(); val != nil && (val.typ == ArrayType || val.typ == DerivedArrayType) {
		array := uncheckedDowncast[JSArray](val)
		slice := array.Slice(globalObject, int(start), int(end))
		_ = vm
		return JSValueEncode(NewJSValueObject(&slice.JSObject))
	}

	// Generic path
	count := uint64(0)
	if end > start {
		count = end - start
	}
	result := NewJSArray(vm, globalObject)
	for i := uint64(0); i < count; i++ {
		elem := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(start + i))))
		result.elements = append(result.elements, elem)
	}
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- Sort ----

// arrayProtoFuncSort implements Array.prototype.sort(compareFn)
func arrayProtoFuncSort(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	compareFn := callFrame.Argument(0)

	// Collect existing elements
	type element struct {
		value JSValue
		index uint64
	}
	var elements []element
	for i := uint64(0); i < length; i++ {
		key := NewPropertyName(propertyNameFromIndex(uint32(i)))
		val := thisObj.Get(globalObject, key)
		if !val.IsUndefined() {
			elements = append(elements, element{value: val, index: i})
		}
	}

	// Sort
	if compareFn.IsUndefined() {
		// Default sort: convert to string and compare
		sort.SliceStable(elements, func(i, j int) bool {
			s1 := elements[i].value.ToString()
			s2 := elements[j].value.ToString()
			return s1 < s2
		})
	} else if compareFn.IsCallable() {
		sort.SliceStable(elements, func(i, j int) bool {
			callData := getCallDataInline(compareFn)
			args := []JSValue{elements[i].value, elements[j].value}
			result := call(globalObject, compareFn, callData, jsUndefined(), args)
			num := result.ToNumber()
			return num < 0
		})
	}

	// Write back
	for i, elem := range elements {
		thisObj.PutByIndex(nil, globalObject, uint32(i), elem.value, true)
	}
	for i := uint64(len(elements)); i < length; i++ {
		thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))), nil)
	}
	_ = vm
	return JSValueEncode(NewJSValueObject(thisObj))
}

// ---- Splice ----

// arrayProtoFuncSplice implements Array.prototype.splice(start, deleteCount, ...items)
func arrayProtoFuncSplice(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	args := callFrame.Arguments()

	relStart := args[0].ToNumber()
	var start uint64
	if relStart < 0 {
		temp := float64(length) + relStart
		if temp < 0 {
			start = 0
		} else {
			start = uint64(temp)
		}
	} else {
		start = uint64(relStart)
		if start > length {
			start = length
		}
	}

	var deleteCount uint64
	if len(args) == 1 {
		deleteCount = length - start
	} else {
		dc := args[1].ToNumber()
		if dc < 0 {
			deleteCount = 0
		} else {
			deleteCount = uint64(dc)
			if deleteCount > length-start {
				deleteCount = length - start
			}
		}
	}

	// Collect deleted elements
	result := NewJSArray(vm, globalObject)
	for i := uint64(0); i < deleteCount; i++ {
		key := NewPropertyName(propertyNameFromIndex(uint32(start + i)))
		val := thisObj.Get(globalObject, key)
		result.elements = append(result.elements, val)
	}

	itemCount := uint64(0)
	if len(args) > 2 {
		itemCount = uint64(len(args) - 2)
	}

	if itemCount < deleteCount {
		// Move elements down
		for k := start; k < length-deleteCount; k++ {
			from := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(k + deleteCount))))
			if !from.IsUndefined() {
				thisObj.PutByIndex(nil, globalObject, uint32(k+itemCount), from, true)
			} else {
				thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(k+itemCount))), nil)
			}
		}
		for k := length; k > length-deleteCount+itemCount; k-- {
			thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(k-1))), nil)
		}
	} else if itemCount > deleteCount {
		for k := length - deleteCount; k > start; k-- {
			from := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(k + deleteCount - 1))))
			if !from.IsUndefined() {
				thisObj.PutByIndex(nil, globalObject, uint32(k+itemCount-1), from, true)
			} else {
				thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(k+itemCount-1))), nil)
			}
		}
	}

	// Insert new items
	if len(args) > 2 {
		for i := uint64(0); i < itemCount; i++ {
			thisObj.PutByIndex(nil, globalObject, uint32(start+i), args[2+i], true)
		}
	}

	newLength := length - deleteCount + itemCount
	setLength(globalObject, vm, thisObj, newLength)
	_ = vm
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- Unshift ----

// arrayProtoFuncUnShift implements Array.prototype.unshift(...items)
func arrayProtoFuncUnShift(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	args := callFrame.Arguments()
	argCount := uint64(len(args))

	// Move existing elements up
	for k := length; k > 0; k-- {
		from := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(k - 1))))
		toKey := k - 1 + argCount
		if !from.IsUndefined() {
			thisObj.PutByIndex(nil, globalObject, uint32(toKey), from, true)
		} else {
			thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(toKey))), nil)
		}
	}

	// Insert new items at start
	for i := uint64(0); i < argCount; i++ {
		thisObj.PutByIndex(nil, globalObject, uint32(i), args[i], true)
	}

	newLength := length + argCount
	setLength(globalObject, vm, thisObj, newLength)
	_ = vm
	return JSValueEncode(jsNumber(float64(newLength)))
}

// ---- indexOf ----

// arrayProtoFuncIndexOf implements Array.prototype.indexOf(searchElement, fromIndex)
func arrayProtoFuncIndexOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	if length == 0 {
		return JSValueEncode(jsNumber(-1))
	}

	searchElement := callFrame.Argument(0)

	var fromIndex uint64
	if callFrame.Argument(1).IsUndefined() {
		fromIndex = 0
	} else {
		n := callFrame.Argument(1).ToNumber()
		if n >= float64(length) {
			return JSValueEncode(jsNumber(-1))
		}
		if n < 0 {
			temp := float64(length) + n
			if temp < 0 {
				fromIndex = 0
			} else {
				fromIndex = uint64(temp)
			}
		} else {
			fromIndex = uint64(n)
		}
	}

	for i := fromIndex; i < length; i++ {
		key := NewPropertyName(propertyNameFromIndex(uint32(i)))
		val := thisObj.Get(globalObject, key)
		// Check for strict equality (sameValueZero for includes)
		if sameValue(globalObject, val, searchElement) {
			return JSValueEncode(jsNumber(float64(i)))
		}
	}

	return JSValueEncode(jsNumber(-1))
}

// ---- lastIndexOf ----

// arrayProtoFuncLastIndexOf implements Array.prototype.lastIndexOf(searchElement, fromIndex)
func arrayProtoFuncLastIndexOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	if length == 0 {
		return JSValueEncode(jsNumber(-1))
	}

	searchElement := callFrame.Argument(0)

	var fromIndex int64
	if callFrame.Argument(1).IsUndefined() {
		fromIndex = int64(length) - 1
	} else {
		n := callFrame.Argument(1).ToNumber()
		if n < 0 {
			temp := float64(length) + n
			if temp < 0 {
				fromIndex = -1
			} else {
				fromIndex = int64(temp)
			}
		} else {
			fromIndex = int64(n)
			if fromIndex >= int64(length) {
				fromIndex = int64(length) - 1
			}
		}
	}

	for i := fromIndex; i >= 0; i-- {
		key := NewPropertyName(propertyNameFromIndex(uint32(i)))
		val := thisObj.Get(globalObject, key)
		if sameValue(globalObject, val, searchElement) {
			return JSValueEncode(jsNumber(float64(i)))
		}
	}

	return JSValueEncode(jsNumber(-1))
}

// ---- includes ----

// arrayProtoFuncIncludes implements Array.prototype.includes(searchElement, fromIndex)
func arrayProtoFuncIncludes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	if length == 0 {
		return JSValueEncode(jsBoolean(false))
	}

	searchElement := callFrame.Argument(0)

	var fromIndex uint64
	if callFrame.Argument(1).IsUndefined() {
		fromIndex = 0
	} else {
		n := callFrame.Argument(1).ToNumber()
		if n >= float64(length) {
			return JSValueEncode(jsBoolean(false))
		}
		n = math.Trunc(n)
		if n < 0 {
			temp := float64(length) + n
			if temp < 0 {
				fromIndex = 0
			} else {
				fromIndex = uint64(temp)
			}
		} else {
			fromIndex = uint64(n)
		}
	}

	for i := fromIndex; i < length; i++ {
		key := NewPropertyName(propertyNameFromIndex(uint32(i)))
		val := thisObj.Get(globalObject, key)
		if sameValueZero(globalObject, val, searchElement) {
			return JSValueEncode(jsBoolean(true))
		}
	}

	return JSValueEncode(jsBoolean(false))
}

// sameValueZero is like sameValue but treats -0 === +0.
func sameValueZero(globalObject *JSGlobalObject, a, b JSValue) bool {
	_ = globalObject
	if a.IsNumber() && b.IsNumber() {
		if a.IsNaN() && b.IsNaN() {
			return true
		}
		return a.ToNumber() == b.ToNumber()
	}
	if a.Tag() != b.Tag() {
		return false
	}
	return a == b
}

// ---- Concat ----

// arrayProtoFuncConcat implements Array.prototype.concat(...items)
func arrayProtoFuncConcat(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)

	// Create result array
	result := NewJSArray(vm, globalObject)

	// Add this object's elements
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	// Check if this is an array (spreadable)
	addToResult := func(obj *JSObject) {
		length := toLength(globalObject, obj)
		for i := uint64(0); i < length; i++ {
			key := NewPropertyName(propertyNameFromIndex(uint32(i)))
			val := obj.Get(globalObject, key)
			result.elements = append(result.elements, val)
		}
	}

	if thisObj.typ == ArrayType || thisObj.typ == DerivedArrayType {
		addToResult(thisObj)
	} else {
		result.elements = append(result.elements, NewJSValueObject(thisObj))
	}

	// Add arguments
	for _, arg := range callFrame.Arguments() {
		if arg.IsObject() {
			argObj := arg.GetObject()
			if argObj.typ == ArrayType || argObj.typ == DerivedArrayType {
				addToResult(argObj)
			} else {
				result.elements = append(result.elements, arg)
			}
		} else {
			result.elements = append(result.elements, arg)
		}
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- Fill ----

// arrayProtoFuncFill implements Array.prototype.fill(value, start, end)
func arrayProtoFuncFill(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	value := callFrame.Argument(0)

	start := argumentClampedIndexFromStartOrEnd(globalObject, callFrame.Argument(1), length, 0)

	var end uint64 = length
	if !callFrame.Argument(2).IsUndefined() {
		end = argumentClampedIndexFromStartOrEnd(globalObject, callFrame.Argument(2), length, length)
	}

	for i := start; i < end; i++ {
		thisObj.PutByIndex(nil, globalObject, uint32(i), value, true)
	}

	return JSValueEncode(NewJSValueObject(thisObj))
}

// ---- CopyWithin ----

// arrayProtoFuncCopyWithin implements Array.prototype.copyWithin(target, start, end)
func arrayProtoFuncCopyWithin(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)

	target := argumentClampedIndexFromStartOrEnd(globalObject, callFrame.Argument(0), length, 0)
	start := argumentClampedIndexFromStartOrEnd(globalObject, callFrame.Argument(1), length, 0)

	var end uint64 = length
	if !callFrame.Argument(2).IsUndefined() {
		end = argumentClampedIndexFromStartOrEnd(globalObject, callFrame.Argument(2), length, length)
	}

	count := uint64(0)
	if end > start {
		count = end - start
		if count > length-target {
			count = length - target
		}
	}

	// Copy in reverse order if target > start to avoid overwriting source
	if uint64(target) > start {
		for i := uint64(count); i > 0; i-- {
			fromKey := start + i - 1
			toKey := uint64(target) + i - 1
			fromVal := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(fromKey))))
			if !fromVal.IsUndefined() {
				thisObj.PutByIndex(nil, globalObject, uint32(toKey), fromVal, true)
			} else {
				thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(toKey))), nil)
			}
		}
	} else {
		for i := uint64(0); i < count; i++ {
			fromKey := start + i
			toKey := uint64(target) + i
			fromVal := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(fromKey))))
			if !fromVal.IsUndefined() {
				thisObj.PutByIndex(nil, globalObject, uint32(toKey), fromVal, true)
			} else {
				thisObj.DeleteProperty(nil, globalObject, NewPropertyName(propertyNameFromIndex(uint32(toKey))), nil)
			}
		}
	}

	return JSValueEncode(NewJSValueObject(thisObj))
}

// ---- ToReversed ----

// arrayProtoFuncToReversed implements Array.prototype.toReversed()
func arrayProtoFuncToReversed(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	result := NewJSArray(vm, globalObject)
	result.elements = make([]JSValue, length)

	for i := uint64(0); i < length; i++ {
		fromKey := length - 1 - i
		val := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(fromKey))))
		result.elements[i] = val
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- ToSorted ----

// arrayProtoFuncToSorted implements Array.prototype.toSorted(compareFn)
func arrayProtoFuncToSorted(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	compareFn := callFrame.Argument(0)

	// Collect elements
	type elem struct {
		value JSValue
		index int
		exists bool
	}
	elements := make([]elem, length)
	for i := uint64(0); i < length; i++ {
		val := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
		elements[i] = elem{value: val, index: int(i), exists: !val.IsUndefined()}
	}

	// Sort existing elements
	if compareFn.IsUndefined() {
		sort.SliceStable(elements, func(i, j int) bool {
			if !elements[i].exists && !elements[j].exists {
				return false
			}
			if !elements[i].exists {
				return false
			}
			if !elements[j].exists {
				return true
			}
			s1 := elements[i].value.ToString()
			s2 := elements[j].value.ToString()
			return s1 < s2
		})
	} else if compareFn.IsCallable() {
		sort.SliceStable(elements, func(i, j int) bool {
			if !elements[i].exists && !elements[j].exists {
				return false
			}
			if !elements[i].exists {
				return false
			}
			if !elements[j].exists {
				return true
			}
			callData := getCallDataInline(compareFn)
			args := []JSValue{elements[i].value, elements[j].value}
			result := call(globalObject, compareFn, callData, jsUndefined(), args)
			return result.ToNumber() < 0
		})
	}

	result := NewJSArray(vm, globalObject)
	result.elements = make([]JSValue, length)
	for i, e := range elements {
		result.elements[i] = e.value
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- ToSpliced ----

// arrayProtoFuncToSpliced implements Array.prototype.toSpliced(start, deleteCount, ...items)
func arrayProtoFuncToSpliced(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	args := callFrame.Arguments()

	var start uint64
	if len(args) == 0 {
		start = 0
	} else {
		relStart := args[0].ToNumber()
		if relStart < 0 {
			temp := float64(length) + relStart
			if temp < 0 {
				start = 0
			} else {
				start = uint64(temp)
			}
		} else {
			start = uint64(relStart)
			if start > length {
				start = length
			}
		}
	}

	var deleteCount uint64
	if len(args) <= 1 {
		deleteCount = length - start
	} else {
		dc := args[1].ToNumber()
		if dc < 0 {
			deleteCount = 0
		} else {
			deleteCount = uint64(dc)
			if deleteCount > length-start {
				deleteCount = length - start
			}
		}
	}

	itemCount := uint64(0)
	var items []JSValue
	if len(args) > 2 {
		itemCount = uint64(len(args) - 2)
		items = args[2:]
	}

	newLen := length - deleteCount + itemCount
	result := NewJSArray(vm, globalObject)
	result.elements = make([]JSValue, 0, newLen)

	// Elements before start
	for i := uint64(0); i < start; i++ {
		val := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
		result.elements = append(result.elements, val)
	}

	// Inserted items
	for _, item := range items {
		result.elements = append(result.elements, item)
	}

	// Elements after deleteCount
	for i := start + deleteCount; i < length; i++ {
		val := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
		result.elements = append(result.elements, val)
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- With ----

// arrayProtoFuncWith implements Array.prototype.with(index, value)
func arrayProtoFuncWith(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)

	// Resolve index
	relIndex := callFrame.Argument(0).ToNumber()
	var actualIndex int64
	if relIndex < 0 {
		actualIndex = int64(relIndex + float64(length))
	} else {
		actualIndex = int64(relIndex)
	}

	if actualIndex < 0 || uint64(actualIndex) >= length {
		globalObject.VM().ThrowException(globalObject, "RangeError: Invalid index on Array.prototype.with")
		return EncodedJSValue()
	}

	value := callFrame.Argument(1)

	result := NewJSArray(vm, globalObject)
	result.elements = make([]JSValue, length)
	for i := uint64(0); i < length; i++ {
		if i == uint64(actualIndex) {
			result.elements[i] = value
		} else {
			val := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
			result.elements[i] = val
		}
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- Flat ----

// arrayProtoFuncFlat implements Array.prototype.flat(depth)
func arrayProtoFuncFlat(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	_ = toLength(globalObject, thisObj)
	_ = vm

	depth := uint64(1)
	if !callFrame.Argument(0).IsUndefined() {
		d := callFrame.Argument(0).ToNumber()
		if !math.IsInf(d, 1) {
			depth = uint64(math.Floor(d))
		} else {
			depth = math.MaxUint64
		}
	}

	var flatten func(obj *JSObject, currentDepth uint64) []JSValue
	flatten = func(obj *JSObject, currentDepth uint64) []JSValue {
		var result []JSValue
		len := toLength(globalObject, obj)
		for i := uint64(0); i < len; i++ {
			val := obj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
			if currentDepth > 0 && val.IsObject() {
				argObj := val.GetObject()
				// Check if it's "array-like" (simplified: just arrays)
				if argObj.typ == ArrayType || argObj.typ == DerivedArrayType {
					result = append(result, flatten(argObj, currentDepth-1)...)
				} else {
					result = append(result, val)
				}
			} else {
				result = append(result, val)
			}
		}
		return result
	}

	flattened := flatten(thisObj, depth)
	result := NewJSArray(vm, globalObject)
	result.elements = flattened
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ---- ToLocaleString ----

// arrayProtoFuncToLocaleString implements Array.prototype.toLocaleString(locales, options)
func arrayProtoFuncToLocaleString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	length := toLength(globalObject, thisObj)
	locales := callFrame.Argument(0)
	_ = locales
	options := callFrame.Argument(1)
	_ = options

	if length == 0 {
		return JSValueEncode(NewJSValueString(""))
	}

	var parts []string
	for i := uint64(0); i < length; i++ {
		if i > 0 {
			parts = append(parts, ",")
		}
		elem := thisObj.Get(globalObject, NewPropertyName(propertyNameFromIndex(uint32(i))))
		if !elem.IsUndefinedOrNull() {
			// Call element.toLocaleString()
			toLocaleStr := elem.GetObject().Get(globalObject, NewPropertyName("toLocaleString"))
			if toLocaleStr.IsCallable() {
				callData := getCallDataInline(toLocaleStr)
				result := call(globalObject, toLocaleStr, callData, elem, nil)
				parts = append(parts, result.ToString())
			} else {
				parts = append(parts, elem.ToString())
			}
		}
	}

	return JSValueEncode(NewJSValueString(strings.Join(parts, ",")))
}
