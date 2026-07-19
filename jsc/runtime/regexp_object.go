// Translation of: Source/JavaScriptCore/runtime/RegExpObject.h
//                  Source/JavaScriptCore/runtime/RegExpObject.cpp
//
// RegExpObject implements the JavaScript RegExp (regular expression) type.
// ES 22.2.3 Properties of the RegExp Prototype Object
//
// NOTE: Uses Go's built-in regexp package as the backend (simplified, not JSC's Yarr engine).
// Go's regexp syntax differs from JavaScript's in some cases (no lookahead, no Unicode property escapes, etc.)

package runtime

import (
	"regexp"
)

// RegExp corresponds to JSC::RegExp — holds the compiled regular expression data.
type RegExp struct {
	pattern    string
	flags      string
	goRegexp   *regexp.Regexp // Go regexp backend
	global     bool
	ignoreCase bool
	multiline  bool
	dotAll     bool
	unicode    bool
	sticky     bool
}

// NewRegExp creates a new RegExp from a pattern and flags string.
func NewRegExp(pattern string, flags string) *RegExp {
	r := &RegExp{
		pattern: pattern,
		flags:   flags,
	}
	// Parse flags
	for _, f := range flags {
		switch f {
		case 'g':
			r.global = true
		case 'i':
			r.ignoreCase = true
		case 'm':
			r.multiline = true
		case 's':
			r.dotAll = true
		case 'u':
			r.unicode = true
		case 'y':
			r.sticky = true
		}
	}

	// Convert JS regexp pattern to Go regexp syntax
	goPattern := convertJSRegexpToGo(pattern, r)
	if compiled, err := regexp.Compile(goPattern); err == nil {
		r.goRegexp = compiled
	}
	return r
}

// convertJSRegexpToGo converts a JavaScript regex pattern to Go regexp syntax.
// This is a simplified conversion — many JS-specific features are not supported.
func convertJSRegexpToGo(pattern string, r *RegExp) string {
	goPattern := pattern

	// Escape sequences that differ between JS and Go
	// JS \d, \w, \s etc are mostly compatible with Go
	// The main differences:
	// 1. JS lookahead/lookbehind assertion — Go does NOT support these
	// 2. JS \b word boundary — Go supports \b in POSIX but not in default
	// 3. JS backreferences like \1 — Go does NOT support
	// 4. JS ^ and $ with /m flag — Go's ^ and $ work the same
	// 5. JS . with /s flag — Go . does NOT match \n (need (?s) flag)
	// 6. JS /i flag — Go uses (?i) at the start

	if r.ignoreCase {
		goPattern = "(?i)" + goPattern
	}
	if r.dotAll {
		// Go . doesn't match \n; (?s) flag enables this (same as JS /s)
		goPattern = "(?s)" + goPattern
	}
	if r.multiline {
		// Go doesn't have ^/$ multi-line by default; (?m) enables it
		goPattern = "(?m)" + goPattern
	}

	return goPattern
}

// MatchResult corresponds to JSC::MatchResult — holds match position info.
type MatchResult struct {
	start int
	end   int
}

// NewMatchResult creates a new MatchResult.
func NewMatchResult(start, end int) MatchResult {
	return MatchResult{start: start, end: end}
}

// RegExpObject corresponds to JSC::RegExpObject.
type RegExpObject struct {
	JSNonFinalObject
	regExp       *RegExp
	lastIndex    JSValue
	isNotWritable bool // if true, lastIndex is read-only
}

// RegExpObjectStructureFlags are the StructureFlags for RegExpObject.
const RegExpObjectStructureFlags uint32 = JSNonFinalObjectStructureFlags | OverridesGetOwnPropertySlot | OverridesGetOwnSpecialPropertyNames | OverridesPut

// NewRegExpObject creates a new RegExpObject.
// In C++: static RegExpObject* create(VM&, Structure*, RegExp*, bool)
func NewRegExpObject(vm *VM, structure *Structure, regExp *RegExp) *RegExpObject {
	obj := &RegExpObject{
		regExp:    regExp,
		lastIndex: jsNumber(0),
	}
	obj.structureID = structure.structureID
	obj.typ = RegExpObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	_ = vm
	obj.finishCreationVM(vm)
	return obj
}

// finishCreationVM completes initialization (simplified version of C++ finishCreation).
func (o *RegExpObject) finishCreationVM(vm *VM) {
	o.JSNonFinalObject.FinishCreation(vm)
}

// ===== lastIndex accessors =====

// getLastIndex returns the current lastIndex value.
// In C++: JSValue getLastIndex() const
func (o *RegExpObject) getLastIndex() JSValue {
	return o.lastIndex
}

// setLastIndex sets lastIndex to a numeric value.
// In C++: bool setLastIndex(JSGlobalObject*, size_t lastIndex)
func (o *RegExpObject) setLastIndex(globalObject *JSGlobalObject, newLastIndex uint64) bool {
	if o.isNotWritable {
		vm := globalObject.VM()
		vm.ThrowException(globalObject, "TypeError: Cannot assign to read only property 'lastIndex'")
		return false
	}
	o.lastIndex = jsNumber(float64(newLastIndex))
	return true
}

// setLastIndexValue sets lastIndex from a JSValue.
// In C++: bool setLastIndex(JSGlobalObject*, JSValue, bool shouldThrow)
func (o *RegExpObject) setLastIndexValue(globalObject *JSGlobalObject, value JSValue, shouldThrow bool) bool {
	if o.isNotWritable {
		if shouldThrow {
			vm := globalObject.VM()
			vm.ThrowException(globalObject, "TypeError: Cannot assign to read only property 'lastIndex'")
		}
		return false
	}
	o.lastIndex = value
	return true
}

// lastIndexIsWritable checks if lastIndex is writable.
// In C++: bool lastIndexIsWritable() const
func (o *RegExpObject) lastIndexIsWritable() bool {
	return !o.isNotWritable
}

// ===== Core match/exec methods =====

// match performs a regular expression match against the given string.
// In C++: MatchResult match(JSGlobalObject*, JSString*)
// Returns MatchResult where start/end = -1 if no match.
func (o *RegExpObject) match(globalObject *JSGlobalObject, str string) MatchResult {
	regExp := o.regExp
	if regExp == nil || regExp.goRegexp == nil {
		return NewMatchResult(-1, -1)
	}

	// Handle lastIndex for global/sticky matches
	lastIdx := uint64(0)
	if regExp.global || regExp.sticky {
		li := o.getLastIndex().ToNumber()
		if li > 0 {
			lastIdx = uint64(li)
		}
	}

	// Perform match
	var loc []int
	if lastIdx > 0 {
		substr := str
		if int(lastIdx) < len(str) {
			substr = str[lastIdx:]
		}
		loc = regExp.goRegexp.FindStringSubmatchIndex(substr)
		if loc != nil && lastIdx > 0 {
			loc[0] += int(lastIdx)
			loc[1] += int(lastIdx)
		}
	} else {
		loc = regExp.goRegexp.FindStringSubmatchIndex(str)
	}

	if loc == nil || len(loc) < 2 {
		// No match — reset lastIndex if global/sticky
		if regExp.global || regExp.sticky {
			o.setLastIndex(globalObject, 0)
		}
		return NewMatchResult(-1, -1)
	}

	// Update lastIndex for global/sticky
	if regExp.global || regExp.sticky {
		o.setLastIndex(globalObject, uint64(loc[1]))
	}

	_ = globalObject
	return NewMatchResult(loc[0], loc[1])
}

// exec performs exec() and returns a JSArray result or null.
// In C++: JSValue exec(JSGlobalObject*, JSString*)
func (o *RegExpObject) exec(globalObject *JSGlobalObject, str string) JSValue {
	regExp := o.regExp
	if regExp == nil || regExp.goRegexp == nil {
		return JSValueEncode(jsNull())
	}
	vm := globalObject.VM()

	// Get lastIndex for global/sticky
	lastIdx := 0
	if regExp.global || regExp.sticky {
		li := o.getLastIndex().ToNumber()
		if li > 0 {
			lastIdx = int(li)
		}
	}

	// Find match
	var matchStr string
	var loc []int
	searchStr := str
	if lastIdx > 0 {
		if lastIdx >= len(str) {
			// past end
			if regExp.global || regExp.sticky {
				o.setLastIndex(globalObject, 0)
			}
			// sticky mode returns null when lastIndex >= length
			if regExp.sticky {
				return JSValueEncode(jsNull())
			}
			// global mode tries from start
			lastIdx = 0
		} else {
			searchStr = str[lastIdx:]
		}
	}

	loc = regExp.goRegexp.FindStringSubmatchIndex(searchStr)
	if loc == nil || len(loc) < 2 {
		if regExp.global || regExp.sticky {
			o.setLastIndex(globalObject, 0)
		}
		return JSValueEncode(jsNull())
	}

	// Adjust positions
	if lastIdx > 0 {
		for i := range loc {
			if loc[i] >= 0 {
				loc[i] += lastIdx
			}
		}
	}

	matchStr = str[loc[0]:loc[1]]

	// Build result array
	result := NewJSArray(vm, globalObject)
	result.elements = append(result.elements, NewJSValueString(matchStr))

	// Add captured groups
	numPairs := len(loc) / 2
	for i := 1; i < numPairs; i++ {
		start := loc[2*i]
		end := loc[2*i+1]
		if start >= 0 && end >= 0 {
			result.elements = append(result.elements, NewJSValueString(str[start:end]))
		} else {
			result.elements = append(result.elements, jsUndefined())
		}
	}

	// Set index property
	result.properties["index"] = jsNumber(float64(loc[0]))
	// Set input property
	result.properties["input"] = NewJSValueString(str)
	// Set groups property
	result.properties["groups"] = jsUndefined()
	// Set indices property (ES2022)
	result.properties["indices"] = jsUndefined()

	// Update lastIndex
	if regExp.global || regExp.sticky {
		o.setLastIndex(globalObject, uint64(loc[1]))
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// test checks if the regex matches the string.
// In C++: bool test(JSGlobalObject*, JSString*)
func (o *RegExpObject) test(globalObject *JSGlobalObject, str string) bool {
	result := o.match(globalObject, str)
	return result.start >= 0
}

// matchGlobal performs a global match and returns an array of all matches.
// In C++: JSValue matchGlobal(JSGlobalObject*, JSString*)
func (o *RegExpObject) matchGlobal(globalObject *JSGlobalObject, str string) JSValue {
	vm := globalObject.VM()
	regExp := o.regExp
	if regExp == nil || regExp.goRegexp == nil || !regExp.global {
		return JSValueEncode(jsNull())
	}

	o.setLastIndex(globalObject, 0)

	result := NewJSArray(vm, globalObject)
	allLoc := regExp.goRegexp.FindAllStringSubmatchIndex(str, -1)
	if allLoc == nil {
		return JSValueEncode(NewJSValueObject(&result.JSObject))
	}

	for _, loc := range allLoc {
		if loc[0] >= 0 && loc[1] >= 0 {
			matchStr := str[loc[0]:loc[1]]
			result.elements = append(result.elements, NewJSValueString(matchStr))
		}
	}

	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// ===== Symbol method helpers =====

// isSymbolMatchFastAndNonObservable checks if Symbol.match is fast and non-observable.
// In C++: bool isSymbolMatchFastAndNonObservable()
func (o *RegExpObject) isSymbolMatchFastAndNonObservable() bool {
	return true // simplified
}

// isSymbolSearchFastAndNonObservable checks if Symbol.search is fast and non-observable.
func (o *RegExpObject) isSymbolSearchFastAndNonObservable() bool {
	return true // simplified
}

// isSymbolMatchAllFastAndNonObservable checks if Symbol.matchAll is fast and non-observable.
func (o *RegExpObject) isSymbolMatchAllFastAndNonObservable() bool {
	return true // simplified
}

// isSymbolReplaceFastAndNonObservable checks if Symbol.replace is fast and non-observable.
func (o *RegExpObject) isSymbolReplaceFastAndNonObservable() bool {
	return true // simplified
}

// isSymbolSplitFastAndNonObservable checks if Symbol.split is fast and non-observable.
func (o *RegExpObject) isSymbolSplitFastAndNonObservable() bool {
	return true // simplified
}

// ===== Overridden property methods =====

// getOwnPropertySlot handles the custom "lastIndex" property.
// In C++: static bool getOwnPropertySlot(...)
func regexpObjectGetOwnPropertySlot(regExp *RegExpObject, globalObject *JSGlobalObject, propertyName PropertyName, slot *PropertySlot) bool {
	if propertyName.String() == "lastIndex" {
		attributes := PropertyAttributeDontDelete | PropertyAttributeDontEnum
		if !regExp.lastIndexIsWritable() {
			attributes |= PropertyAttributeReadOnly
		}
		slot.Value = regExp.getLastIndex()
		return true
	}
	return regExp.GetOwnPropertySlot(globalObject, propertyName, slot)
}

// put handles assignment to lastIndex.
// In C++: static bool put(...)
func regexpObjectPut(regExp *RegExpObject, globalObject *JSGlobalObject, propertyName PropertyName, value JSValue, slot *PutPropertySlot) bool {
	if propertyName.String() == "lastIndex" {
		if !regExp.lastIndexIsWritable() {
			return false
		}
		regExp.setLastIndexValue(globalObject, value, false)
		return true
	}
	return regExp.Put(nil, globalObject, propertyName, value, slot)
}

// ===== Accessor helpers =====

// areLegacyFeaturesEnabled checks if legacy RegExp features are enabled.
func (o *RegExpObject) areLegacyFeaturesEnabled() bool {
	return true // simplified
}

// regExp returns the internal RegExp object.
func (o *RegExpObject) regExpPtr() *RegExp {
	return o.regExp
}

// getRegexpSource returns the source pattern string.
func (o *RegExpObject) getRegexpSource() string {
	if o.regExp == nil {
		return "(?:)"
	}
	src := o.regExp.pattern
	if src == "" {
		return "(?:)"
	}
	return src
}

// getRegexpFlags returns the flags string.
func (o *RegExpObject) getRegexpFlags() string {
	if o.regExp == nil {
		return ""
	}
	return o.regExp.flags
}

// getRegexpGlobal returns the global flag.
func (o *RegExpObject) getRegexpGlobal() bool {
	return o.regExp != nil && o.regExp.global
}

// getRegexpIgnoreCase returns the ignoreCase flag.
func (o *RegExpObject) getRegexpIgnoreCase() bool {
	return o.regExp != nil && o.regExp.ignoreCase
}

// getRegexpMultiline returns the multiline flag.
func (o *RegExpObject) getRegexpMultiline() bool {
	return o.regExp != nil && o.regExp.multiline
}

// getRegexpDotAll returns the dotAll flag.
func (o *RegExpObject) getRegexpDotAll() bool {
	return o.regExp != nil && o.regExp.dotAll
}

// getRegexpUnicode returns the unicode flag.
func (o *RegExpObject) getRegexpUnicode() bool {
	return o.regExp != nil && o.regExp.unicode
}

// getRegexpSticky returns the sticky flag.
func (o *RegExpObject) getRegexpSticky() bool {
	return o.regExp != nil && o.regExp.sticky
}
