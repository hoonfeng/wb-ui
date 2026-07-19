// Translation of: Source/JavaScriptCore/runtime/RegExpPrototype.h
//                  Source/JavaScriptCore/runtime/RegExpPrototype.cpp
//
// RegExpPrototype is the prototype for all JavaScript RegExp objects (RegExp.prototype).
// ES 22.2.5 Properties of the RegExp Prototype Object

package runtime

// RegExpPrototype corresponds to JSC::RegExpPrototype.
type RegExpPrototype struct {
	JSNonFinalObject
}

// NewRegExpPrototype creates a new RegExpPrototype.
// In C++: static RegExpPrototype* create(VM&, JSGlobalObject*, Structure*)
func NewRegExpPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *RegExpPrototype {
	p := &RegExpPrototype{}
	p.structureID = structure.structureID
	p.typ = RegExpObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers all RegExp.prototype methods and getters.
// In C++: void finishCreation(VM&, JSGlobalObject*)
func (p *RegExpPrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.JSNonFinalObject.FinishCreation(vm)

	// Standard methods
	p.putDirectWithoutTransition(vm, NewPropertyName("exec"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("test"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("compile"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Getter properties (Accessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("global"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("ignoreCase"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("multiline"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("dotAll"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("sticky"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("unicode"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("hasIndices"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("source"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("flags"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)
	p.putDirectWithoutTransition(vm, NewPropertyName("unicodeSets"), NewJSValueObject(nil), PropertyAttributeDontEnum|PropertyAttributeAccessor)

	// Well-known symbol methods
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolMatch), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolMatchAll), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolReplace), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolSearch), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolSplit), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// @@toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("RegExp"), PropertyAttributeDontEnum)

	_ = globalObject
}

// =====================================================================
// Method implementations
// =====================================================================

// regExpProtoFuncExec implements RegExp.prototype.exec(string)
// In C++: regExpProtoFuncExec
func regExpProtoFuncExec(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue()

	re := dynamicDowncast[RegExpObject](thisValue.GetObject())
	if re == nil {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"Builtin RegExp exec can only be called on a RegExp object")
	}

	str := callFrame.Argument(0).ToString()
	return re.exec(globalObject, str)
}

// regExpProtoFuncTest implements RegExp.prototype.test(string)
// In C++: regExpProtoFuncTest
func regExpProtoFuncTest(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue()
	if !thisValue.IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"RegExp.prototype.test requires that |this| be an Object")
	}

	str := callFrame.Argument(0).ToString()

	// Fast path for RegExpObject
	re := dynamicDowncast[RegExpObject](thisValue.GetObject())
	if re != nil {
		result := re.test(globalObject, str)
		return JSValueEncode(jsBoolean(result))
	}

	// Generic path: call exec and check result
	match := regExpExecGeneric(globalObject, thisValue, str)
	return JSValueEncode(jsBoolean(!match.IsNull()))
}

// regExpProtoFuncToString implements RegExp.prototype.toString()
// Returns "/pattern/flags"
// In C++: regExpProtoFuncToString
func regExpProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue()
	if !thisValue.IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"RegExp.prototype.toString requires that |this| be an Object")
	}

	thisObj := thisValue.GetObject()

	// Get source
	source := thisObj.Get(globalObject, NewPropertyName("source"))
	sourceStr := source.ToString()

	// Get flags
	flags := thisObj.Get(globalObject, NewPropertyName("flags"))
	flagsStr := flags.ToString()

	return JSValueEncode(NewJSValueString("/" + sourceStr + "/" + flagsStr))
}

// =====================================================================
// Getter implementations
// =====================================================================

// regExpProtoGetterGlobal implements the "global" getter
func regExpProtoGetterGlobal(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpGlobal()))
}

// regExpProtoGetterIgnoreCase implements the "ignoreCase" getter
func regExpProtoGetterIgnoreCase(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpIgnoreCase()))
}

// regExpProtoGetterMultiline implements the "multiline" getter
func regExpProtoGetterMultiline(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpMultiline()))
}

// regExpProtoGetterDotAll implements the "dotAll" getter
func regExpProtoGetterDotAll(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpDotAll()))
}

// regExpProtoGetterSticky implements the "sticky" getter
func regExpProtoGetterSticky(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpSticky()))
}

// regExpProtoGetterUnicode implements the "unicode" getter
func regExpProtoGetterUnicode(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(jsBoolean(re.getRegexpUnicode()))
}

// regExpProtoGetterSource implements the "source" getter
func regExpProtoGetterSource(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(NewJSValueString(re.getRegexpSource()))
}

// regExpProtoGetterFlags implements the "flags" getter
func regExpProtoGetterFlags(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re == nil {
		return JSValueEncode(jsUndefined())
	}
	return JSValueEncode(NewJSValueString(re.getRegexpFlags()))
}

// =====================================================================
// Well-known Symbol methods (@@match, @@matchAll, @@replace, @@search, @@split)
// =====================================================================

// regExpProtoFuncMatch implements RegExp.prototype[@@match](string)
func regExpProtoFuncMatch(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()
	if !thisValue.IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm},
			"RegExp.prototype.@@match requires that |this| be an Object")
	}
	thisObj := thisValue.GetObject()
	str := callFrame.Argument(0).ToString()

	// Fast path for RegExpObject
	re := dynamicDowncast[RegExpObject](thisObj)
	if re != nil {
		if re.regExp != nil && re.regExp.global {
			return re.matchGlobal(globalObject, str)
		}
		return re.exec(globalObject, str)
	}

	// Generic path
	return regExpExecGeneric(globalObject, thisValue, str)
}

// regExpProtoFuncMatchAll implements RegExp.prototype[@@matchAll](string)
// Simplified skeleton
func regExpProtoFuncMatchAll(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	_ = callFrame
	// Simplified: returns the exec result
	return regExpProtoFuncExec(globalObject, callFrame)
}

// regExpProtoFuncReplace implements RegExp.prototype[@@replace](string, replaceValue)
// Simplified: string-based replacement
func regExpProtoFuncReplace(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	thisValue := callFrame.ThisValue()
	str := callFrame.Argument(0).ToString()
	replaceValue := callFrame.Argument(1)

	// Fast path for RegExpObject
	re := dynamicDowncast[RegExpObject](thisValue.GetObject())
	if re != nil && re.regExp != nil && re.regExp.goRegexp != nil {
		if replaceValue.IsCallable() {
			// Function replacement — simplified
			result := re.regExp.goRegexp.ReplaceAllStringFunc(str,
				func(match string) string {
					args := []JSValue{NewJSValueString(match)}
					result := callFunctionValue(globalObject, replaceValue, args)
					return result.ToString()
				})
			return JSValueEncode(NewJSValueString(result))
		}
		// String replacement
		repStr := replaceValue.ToString()
		result := re.regExp.goRegexp.ReplaceAllString(str, repStr)
		return JSValueEncode(NewJSValueString(result))
	}

	_ = vm
	// Generic path — simplified
	return JSValueEncode(NewJSValueString(str))
}

// regExpProtoFuncSearch implements RegExp.prototype[@@search](string)
func regExpProtoFuncSearch(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue()
	str := callFrame.Argument(0).ToString()

	re := dynamicDowncast[RegExpObject](thisValue.GetObject())
	if re != nil && re.regExp != nil && re.regExp.goRegexp != nil {
		loc := re.regExp.goRegexp.FindStringIndex(str)
		if loc == nil {
			return JSValueEncode(jsNumber(-1))
		}
		return JSValueEncode(jsNumber(float64(loc[0])))
	}

	return JSValueEncode(jsNumber(-1))
}

// regExpProtoFuncSplit implements RegExp.prototype[@@split](string, limit)
func regExpProtoFuncSplit(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	str := callFrame.Argument(0).ToString()

	var limit uint64 = 0
	hasLimit := !callFrame.Argument(1).IsUndefined()
	if hasLimit {
		l := callFrame.Argument(1).ToNumber()
		if l > 0 {
			limit = uint64(l)
		}
	}

	re := dynamicDowncast[RegExpObject](callFrame.ThisValue().GetObject())
	if re != nil && re.regExp != nil && re.regExp.goRegexp != nil {
		// Use Go regexp split
		parts := re.regExp.goRegexp.Split(str, -1)
		result := NewJSArray(vm, globalObject)
		for i, p := range parts {
			if hasLimit && uint64(i) >= limit {
				break
			}
			result.elements = append(result.elements, NewJSValueString(p))
		}
		return JSValueEncode(NewJSValueObject(&result.JSObject))
	}

	// No regexp — treat as non-regex split
	result := NewJSArray(vm, globalObject)
	result.elements = append(result.elements, NewJSValueString(str))
	return JSValueEncode(NewJSValueObject(&result.JSObject))
}

// =====================================================================
// Generic helpers
// =====================================================================

// regExpExecGeneric performs RegExpExec on a generic (non-RegExpObject) this value.
func regExpExecGeneric(globalObject *JSGlobalObject, thisValue JSValue, str string) JSValue {
	// Get the exec method
	thisObj := thisValue.GetObject()
	execMethod := thisObj.Get(globalObject, NewPropertyName("exec"))

	if !execMethod.IsCallable() {
		return JSValueEncode(jsNull())
	}

	callData := getCallDataInline(execMethod)
	result := call(globalObject, execMethod, callData, thisValue, []JSValue{NewJSValueString(str)})
	return result
}
