// Translation of: Source/JavaScriptCore/runtime/StringPrototype.h
//                  Source/JavaScriptCore/runtime/StringPrototype.cpp
//
// StringPrototype is the prototype for all JavaScript strings (String.prototype).
// ES 21.1.3 Properties of the String Prototype Object

package runtime

import (
	"math"
	"strings"
)

// StringPrototype corresponds to JSC::StringPrototype.
// In C++ it extends StringObject; in Go we simplify to JSNonFinalObject.
type StringPrototype struct {
	JSNonFinalObject
}

// NewStringPrototype creates a new StringPrototype.
// In C++: static StringPrototype* create(VM&, JSGlobalObject*, Structure*)
func NewStringPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *StringPrototype {
	p := &StringPrototype{}
	p.structureID = structure.structureID
	p.typ = StringObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers all String.prototype methods.
// In C++: void finishCreation(VM&, JSGlobalObject*)
func (p *StringPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.JSNonFinalObject.FinishCreation(vm)

	// === Core ES methods ===
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("charAt"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("charCodeAt"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("codePointAt"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("concat"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("indexOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("lastIndexOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("replace"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("replaceAll"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("repeat"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("padStart"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("padEnd"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("slice"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("substring"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("substr"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("at"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLowerCase"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toUpperCase"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("localeCompare"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleLowerCase"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleUpperCase"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("trim"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("startsWith"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("endsWith"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("includes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("match"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("search"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("matchAll"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("split"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("normalize"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// trimStart / trimLeft / trimEnd / trimRight
	p.putDirectWithoutTransition(vm, NewPropertyName("trimStart"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("trimLeft"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("trimEnd"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("trimRight"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// @@iterator
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolIterator), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// isWellFormed / toWellFormed (ES2024)
	p.putDirectWithoutTransition(vm, NewPropertyName("isWellFormed"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toWellFormed"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// @@toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("String"), PropertyAttributeDontEnum)

	// length
	p.putDirectWithoutTransition(vm, NewPropertyName("length"), jsNumber(0), PropertyAttributeReadOnly|PropertyAttributeDontEnum)

	_ = globalObject
}

// =====================================================================
// Helper: get this string value (ToObject(this) → [[StringData]])
// =====================================================================

// getStringThis extracts the string value from a String.prototype method call.
// In C++: thisValue.toThis(globalObject, ECMAMode::strict()).toString(globalObject)
func getStringThis(globalObject *JSGlobalObject, callFrame *ExecState) (string, JSValue) {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	if thisValue.IsUndefinedOrNull() {
		return "", thisValue
	}
	s := thisValue.ToString()
	return s, thisValue
}

// =====================================================================
// String.prototype method implementations
// =====================================================================

// ---- toString / valueOf ----

// stringProtoFuncToString implements String.prototype.toString() and valueOf()
func stringProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	if !thisValue.IsString() && !thisValue.IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()}, "String.prototype.toString requires a String")
	}
	str := thisValue.ToString()
	return JSValueEncode(NewJSValueString(str))
}

// ---- charAt ----

// stringProtoFuncCharAt implements String.prototype.charAt(pos)
// ES 21.1.3.1
func stringProtoFuncCharAt(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	pos := int(callFrame.Argument(0).ToNumber())
	if pos < 0 || pos >= len(s) {
		return JSValueEncode(NewJSValueString(""))
	}
	// Convert to rune index
	runes := []rune(s)
	if pos >= len(runes) {
		return JSValueEncode(NewJSValueString(""))
	}
	return JSValueEncode(NewJSValueString(string(runes[pos])))
}

// ---- charCodeAt ----

// stringProtoFuncCharCodeAt implements String.prototype.charCodeAt(pos)
// ES 21.1.3.2
func stringProtoFuncCharCodeAt(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	pos := int(callFrame.Argument(0).ToNumber())
	if pos < 0 || pos >= len(s) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	runes := []rune(s)
	if pos >= len(runes) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	cp := runes[pos]
	if cp <= 0xFFFF {
		return JSValueEncode(jsNumber(float64(cp)))
	}
	// Supplementary code point: return lead surrogate
	cp -= 0x10000
	return JSValueEncode(jsNumber(float64(0xD800 + (cp >> 10))))
}

// ---- codePointAt ----

// stringProtoFuncCodePointAt implements String.prototype.codePointAt(pos)
// ES 21.1.3.3
func stringProtoFuncCodePointAt(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	pos := int(callFrame.Argument(0).ToNumber())
	if pos < 0 || pos >= len(s) {
		return JSValueEncode(jsUndefined())
	}
	// Go's range iteration handles surrogate pairs correctly
	runes := []rune(s)
	if pos >= len(runes) {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsNumber(float64(runes[pos])))
}

// ---- concat ----

// stringProtoFuncConcat implements String.prototype.concat(...args)
// ES 21.1.3.4
func stringProtoFuncConcat(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	var parts []string
	parts = append(parts, s)
	for i := 0; i < callFrame.ArgumentCount(); i++ {
		parts = append(parts, callFrame.Argument(i).ToString())
	}
	return JSValueEncode(NewJSValueString(strings.Join(parts, "")))
}

// ---- indexOf ----

// stringProtoFuncIndexOf implements String.prototype.indexOf(searchString, position)
// ES 21.1.3.8
func stringProtoFuncIndexOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchStr := callFrame.Argument(0).ToString()

	var pos int
	if callFrame.ArgumentCount() > 1 {
		numPos := callFrame.Argument(1).ToNumber()
		if numPos < 0 {
			pos = 0
		} else {
			pos = int(numPos)
		}
	}
	if pos > len(s) {
		return JSValueEncode(jsNumber(-1))
	}
	idx := strings.Index(s[pos:], searchStr)
	if idx < 0 {
		return JSValueEncode(jsNumber(-1))
	}
	return JSValueEncode(jsNumber(float64(pos + idx)))
}

// ---- lastIndexOf ----

// stringProtoFuncLastIndexOf implements String.prototype.lastIndexOf(searchString, position)
// ES 21.1.3.9
func stringProtoFuncLastIndexOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchStr := callFrame.Argument(0).ToString()

	var pos int
	if callFrame.ArgumentCount() > 1 && !callFrame.Argument(1).IsUndefined() {
		numPos := callFrame.Argument(1).ToNumber()
		if numPos >= float64(len(s)) {
			pos = len(s)
		} else if numPos < 0 {
			pos = 0
		} else {
			pos = int(numPos) + len(searchStr)
			if pos > len(s) {
				pos = len(s)
			}
		}
	} else {
		pos = len(s)
	}

	if pos > len(s) {
		pos = len(s)
	}
	if pos < 0 {
		pos = 0
	}

	idx := strings.LastIndex(s[:pos], searchStr)
	return JSValueEncode(jsNumber(float64(idx)))
}

// ---- includes ----

// stringProtoFuncIncludes implements String.prototype.includes(searchString, position)
// ES 21.1.3.7
func stringProtoFuncIncludes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchStr := callFrame.Argument(0).ToString()

	var pos int
	if callFrame.ArgumentCount() > 1 {
		numPos := callFrame.Argument(1).ToNumber()
		if numPos < 0 {
			pos = 0
		} else {
			pos = int(numPos)
		}
	}
	if pos > len(s) {
		return JSValueEncode(jsBoolean(false))
	}
	return JSValueEncode(jsBoolean(strings.Contains(s[pos:], searchStr)))
}

// ---- startsWith ----

// stringProtoFuncStartsWith implements String.prototype.startsWith(searchString, position)
// ES 21.1.3.13
func stringProtoFuncStartsWith(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchStr := callFrame.Argument(0).ToString()

	var pos int
	if callFrame.ArgumentCount() > 1 {
		numPos := callFrame.Argument(1).ToNumber()
		if numPos < 0 {
			pos = 0
		} else {
			pos = int(numPos)
		}
	}
	if pos > len(s) {
		return JSValueEncode(jsBoolean(false))
	}
	return JSValueEncode(jsBoolean(strings.HasPrefix(s[pos:], searchStr)))
}

// ---- endsWith ----

// stringProtoFuncEndsWith implements String.prototype.endsWith(searchString, endPosition)
// ES 21.1.3.6
func stringProtoFuncEndsWith(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchStr := callFrame.Argument(0).ToString()

	var endPos int
	if callFrame.ArgumentCount() > 1 && !callFrame.Argument(1).IsUndefined() {
		numPos := callFrame.Argument(1).ToNumber()
		if numPos < 0 {
			endPos = 0
		} else if numPos > float64(len(s)) {
			endPos = len(s)
		} else {
			endPos = int(numPos)
		}
	} else {
		endPos = len(s)
	}
	return JSValueEncode(jsBoolean(strings.HasSuffix(s[:endPos], searchStr)))
}

// ---- slice ----

// stringProtoFuncSlice implements String.prototype.slice(start, end)
// ES 21.1.3.14
func stringProtoFuncSlice(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	length := len(s)

	var start int
	numStart := callFrame.Argument(0).ToNumber()
	if numStart < 0 {
		start = int(numStart) + length
		if start < 0 {
			start = 0
		}
	} else {
		start = int(numStart)
		if start > length {
			start = length
		}
	}

	var end int
	if callFrame.Argument(1).IsUndefined() {
		end = length
	} else {
		numEnd := callFrame.Argument(1).ToNumber()
		if numEnd < 0 {
			end = int(numEnd) + length
			if end < 0 {
				end = 0
			}
		} else {
			end = int(numEnd)
			if end > length {
				end = length
			}
		}
	}

	if start >= end {
		return JSValueEncode(NewJSValueString(""))
	}
	return JSValueEncode(NewJSValueString(s[start:end]))
}

// ---- substring ----

// stringProtoFuncSubstring implements String.prototype.substring(start, end)
// ES 21.1.3.15
func stringProtoFuncSubstring(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	length := len(s)

	var start int
	numStart := callFrame.Argument(0).ToNumber()
	if numStart < 0 || math.IsNaN(numStart) {
		start = 0
	} else if numStart > float64(length) {
		start = length
	} else {
		start = int(numStart)
	}

	var end int
	if callFrame.Argument(1).IsUndefined() {
		end = length
	} else {
		numEnd := callFrame.Argument(1).ToNumber()
		if numEnd < 0 || math.IsNaN(numEnd) {
			end = 0
		} else if numEnd > float64(length) {
			end = length
		} else {
			end = int(numEnd)
		}
	}

	// Swap if start > end (ES 21.1.3.15 step 8)
	if start > end {
		start, end = end, start
	}

	return JSValueEncode(NewJSValueString(s[start:end]))
}

// ---- toLowerCase ----

// stringProtoFuncToLowerCase implements String.prototype.toLowerCase()
// ES 21.1.3.16
func stringProtoFuncToLowerCase(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	return JSValueEncode(NewJSValueString(strings.ToLower(s)))
}

// ---- toUpperCase ----

// stringProtoFuncToUpperCase implements String.prototype.toUpperCase()
// ES 21.1.3.17
func stringProtoFuncToUpperCase(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	return JSValueEncode(NewJSValueString(strings.ToUpper(s)))
}

// ---- trim ----

// stringProtoFuncTrim implements String.prototype.trim()
// ES 21.1.3.18
func stringProtoFuncTrim(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	return JSValueEncode(NewJSValueString(strings.TrimSpace(s)))
}

// ---- trimStart / trimLeft ----

// stringProtoFuncTrimStart implements String.prototype.trimStart() / trimLeft()
// ES 21.1.3.19
func stringProtoFuncTrimStart(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	return JSValueEncode(NewJSValueString(strings.TrimLeft(s, " \t\n\r\f\v")))
}

// ---- trimEnd / trimRight ----

// stringProtoFuncTrimEnd implements String.prototype.trimEnd() / trimRight()
// ES 21.1.3.20
func stringProtoFuncTrimEnd(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	return JSValueEncode(NewJSValueString(strings.TrimRight(s, " \t\n\r\f\v")))
}

// ---- repeat ----

// stringProtoFuncRepeat implements String.prototype.repeat(count)
// ES 21.1.3.13
func stringProtoFuncRepeat(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	count := callFrame.Argument(0).ToNumber()

	if count < 0 || math.IsInf(count, 1) || math.IsNaN(count) {
		vm := globalObject.VM()
		vm.ThrowException(globalObject, "RangeError: Invalid count value for String.prototype.repeat")
		return EncodedJSValue()
	}

	n := int(count)
	if uint64(n)*uint64(len(s)) > maxStringLength {
		vm := globalObject.VM()
		vm.ThrowException(globalObject, "RangeError: String length exceeds maximum")
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueString(strings.Repeat(s, n)))
}

// maxStringLength is the maximum string length allowed (2^28 - 16).
const maxStringLength = 268435440

// ---- padStart ----

// stringProtoFuncPadStart implements String.prototype.padStart(maxLength, fillString)
// ES 21.1.3.10
func stringProtoFuncPadStart(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	targetLength := int(callFrame.Argument(0).ToNumber())
	if targetLength <= len(s) {
		return JSValueEncode(NewJSValueString(s))
	}

	fillStr := " "
	if callFrame.ArgumentCount() > 1 && !callFrame.Argument(1).IsUndefined() {
		fillStr = callFrame.Argument(1).ToString()
		if fillStr == "" {
			return JSValueEncode(NewJSValueString(s))
		}
	}

	fillLen := targetLength - len(s)
	count := fillLen / len(fillStr)
	remainder := fillLen % len(fillStr)
	return JSValueEncode(NewJSValueString(strings.Repeat(fillStr, count) + fillStr[:remainder] + s))
}

// ---- padEnd ----

// stringProtoFuncPadEnd implements String.prototype.padEnd(maxLength, fillString)
// ES 21.1.3.11
func stringProtoFuncPadEnd(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	targetLength := int(callFrame.Argument(0).ToNumber())
	if targetLength <= len(s) {
		return JSValueEncode(NewJSValueString(s))
	}

	fillStr := " "
	if callFrame.ArgumentCount() > 1 && !callFrame.Argument(1).IsUndefined() {
		fillStr = callFrame.Argument(1).ToString()
		if fillStr == "" {
			return JSValueEncode(NewJSValueString(s))
		}
	}

	fillLen := targetLength - len(s)
	count := fillLen / len(fillStr)
	remainder := fillLen % len(fillStr)
	return JSValueEncode(NewJSValueString(s + strings.Repeat(fillStr, count) + fillStr[:remainder]))
}

// ---- replace (non-regex) ----

// stringProtoFuncReplace implements String.prototype.replace(searchValue, replaceValue)
// ES 21.1.3.14 — simplified: non-regexp string replacement
func stringProtoFuncReplace(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchValue := callFrame.Argument(0)
	replaceValue := callFrame.Argument(1)

	// If searchValue is a RegExp (has Symbol.replace), delegate — simplified: non-regex only
	if searchValue.IsObject() {
		// Check for regex-like behavior
		replaceFn := searchValue.GetObject().Get(globalObject, NewPropertyName(SymbolReplace))
		if replaceFn.IsCallable() {
			// Delegate to replace[Symbol.replace](this, replaceValue)
			callData := getCallDataInline(replaceFn)
			result := call(globalObject, replaceFn, callData, searchValue, []JSValue{NewJSValueString(s), replaceValue})
			return JSValueEncode(result)
		}
	}

	searchStr := searchValue.ToString()
	if searchStr == "" {
		if replaceValue.IsCallable() {
			fnResult := callFunctionValue(globalObject, replaceValue, []JSValue{NewJSValueString(s), jsNumber(0), NewJSValueString(s)})
			return JSValueEncode(fnResult)
		}
		repStr := replaceValue.ToString()
		// Replace empty string at position 0 (special case: prepend)
		return JSValueEncode(NewJSValueString(repStr + s))
	}

	idx := strings.Index(s, searchStr)
	if idx < 0 {
		return JSValueEncode(NewJSValueString(s))
	}

	if replaceValue.IsCallable() {
		// Call function match with: match, offset, string
		match := s[idx : idx+len(searchStr)]
		fnResult := callFunctionValue(globalObject, replaceValue, []JSValue{NewJSValueString(match), jsNumber(float64(idx)), NewJSValueString(s)})
		repStr := fnResult.ToString()
		return JSValueEncode(NewJSValueString(s[:idx] + repStr + s[idx+len(searchStr):]))
	}

	repStr := replaceValue.ToString()
	return JSValueEncode(NewJSValueString(s[:idx] + repStr + s[idx+len(searchStr):]))
}

// ---- replaceAll (non-regex) ----

// stringProtoFuncReplaceAll implements String.prototype.replaceAll(searchValue, replaceValue)
// ES 21.1.3.15 — simplified: non-regexp string replacement
func stringProtoFuncReplaceAll(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	searchValue := callFrame.Argument(0)
	replaceValue := callFrame.Argument(1)

	searchStr := searchValue.ToString()
	if searchStr == "" {
		return JSValueEncode(NewJSValueString(s))
	}

	if replaceValue.IsCallable() {
		// Call function for each match
		result := s
		offset := 0
		for {
			idx := strings.Index(result[offset:], searchStr)
			if idx < 0 {
				break
			}
			idx += offset
			match := result[idx : idx+len(searchStr)]
			rep := callFunctionValue(globalObject, replaceValue, []JSValue{NewJSValueString(match), jsNumber(float64(idx)), NewJSValueString(s)})
			result = result[:idx] + rep.ToString() + result[idx+len(searchStr):]
			offset = idx + len(rep.ToString())
		}
		return JSValueEncode(NewJSValueString(result))
	}

	repStr := replaceValue.ToString()
	return JSValueEncode(NewJSValueString(strings.ReplaceAll(s, searchStr, repStr)))
}

// callFunctionValue calls a JavaScript function value with the given arguments.
func callFunctionValue(globalObject *JSGlobalObject, fn JSValue, args []JSValue) JSValue {
	callData := getCallDataInline(fn)
	return call(globalObject, fn, callData, jsUndefined(), args)
}

// ---- split ----

// stringProtoFuncSplit implements String.prototype.split(separator, limit)
// ES 21.1.3.16 — simplified: non-regexp string split
func stringProtoFuncSplit(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	s, _ := getStringThis(globalObject, callFrame)
	separator := callFrame.Argument(0)
	limit := callFrame.Argument(1)

	// If separator is a RegExp (has Symbol.split), delegate — simplified: non-regex
	if separator.IsObject() {
		splitFn := separator.GetObject().Get(globalObject, NewPropertyName(SymbolSplit))
		if splitFn.IsCallable() {
			callData := getCallDataInline(splitFn)
			result := call(globalObject, splitFn, callData, separator, []JSValue{NewJSValueString(s), limit})
			return JSValueEncode(result)
		}
	}

	limitCount := uint64(0)
	hasLimit := !limit.IsUndefined()
	if hasLimit {
		l := limit.ToNumber()
		if l >= 0 {
			limitCount = uint64(l)
		}
	}

	sepStr := separator.ToString()
	var parts []string
	if sepStr == "" {
		// Split each character
		runes := []rune(s)
		for _, r := range runes {
			parts = append(parts, string(r))
			if hasLimit && uint64(len(parts)) >= limitCount {
				break
			}
		}
	} else {
		parts = strings.SplitN(s, sepStr, -1)
		if hasLimit && uint64(len(parts)) > limitCount {
			parts = parts[:limitCount]
		}
	}

	arr := NewJSArray(vm, globalObject)
	for _, p := range parts {
		arr.elements = append(arr.elements, NewJSValueString(p))
	}
	return JSValueEncode(NewJSValueObject(&arr.JSObject))
}

// ---- match ----

// stringProtoFuncMatch implements String.prototype.match(regexp)
// ES 21.1.3.9 — skeleton
func stringProtoFuncMatch(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	s, _ := getStringThis(globalObject, callFrame)
	regexp := callFrame.Argument(0)

	if regexp.IsObject() {
		matchFn := regexp.GetObject().Get(globalObject, NewPropertyName(SymbolMatch))
		if matchFn.IsCallable() {
			callData := getCallDataInline(matchFn)
			result := call(globalObject, matchFn, callData, regexp, []JSValue{NewJSValueString(s)})
			return JSValueEncode(result)
		}
	}
	_ = vm

	// Simplified: if regexp is a string, convert to RegExp and match
	_ = regexp
	return JSValueEncode(NewJSValueString(s)) // placeholder
}

// ---- search ----

// stringProtoFuncSearch implements String.prototype.search(regexp)
// ES 21.1.3.12 — skeleton
func stringProtoFuncSearch(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	regexp := callFrame.Argument(0)

	if regexp.IsObject() {
		searchFn := regexp.GetObject().Get(globalObject, NewPropertyName(SymbolSearch))
		if searchFn.IsCallable() {
			callData := getCallDataInline(searchFn)
			result := call(globalObject, searchFn, callData, regexp, []JSValue{NewJSValueString(s)})
			return JSValueEncode(result)
		}
	}

	// Simplified: return -1 (no match)
	return JSValueEncode(jsNumber(-1))
}

// ---- at ----

// stringProtoFuncAt implements String.prototype.at(index)
// ES 2022
func stringProtoFuncAt(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	length := len(s)

	relIndex := callFrame.Argument(0).ToNumber()
	var index int
	if relIndex < 0 {
		index = int(relIndex) + length
	} else {
		index = int(relIndex)
	}

	if index < 0 || index >= length {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(NewJSValueString(string(s[index])))
}

// ---- iterator ----

// stringProtoFuncIterator implements String.prototype[@@iterator]()
// Returns a new StringIterator that iterates over code points.
func stringProtoFuncIterator(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	_ = s
	// Simplified: return the string itself
	return JSValueEncode(NewJSValueString(s))
}

// ---- localeCompare skeleton ----

// stringProtoFuncLocaleCompare implements String.prototype.localeCompare(that)
func stringProtoFuncLocaleCompare(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	s, _ := getStringThis(globalObject, callFrame)
	that := callFrame.Argument(0).ToString()

	if s < that {
		return JSValueEncode(jsNumber(-1))
	} else if s > that {
		return JSValueEncode(jsNumber(1))
	}
	return JSValueEncode(jsNumber(0))
}

// ---- replace Symbol.replace, Symbol.split, Symbol.match, Symbol.search constants ----
// (defined inline since these skeleton implementations use them)

// (Symbol constants moved to symbol_prototype.go)
