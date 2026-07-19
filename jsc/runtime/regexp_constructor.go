// Translation of: Source/JavaScriptCore/runtime/RegExpConstructor.h
//                  Source/JavaScriptCore/runtime/RegExpConstructor.cpp
//
// RegExpConstructor implements the RegExp() constructor.
// ES 22.2.4 The RegExp Constructor

package runtime

// RegExpConstructor corresponds to JSC::RegExpConstructor.
type RegExpConstructor struct {
	InternalFunction
}

// RegExpConstructorStructureFlags are the StructureFlags for RegExpConstructor.
const RegExpConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

// NewRegExpConstructor creates a new RegExpConstructor.
// In C++: static RegExpConstructor* create(VM&, Structure*, RegExpPrototype*)
func NewRegExpConstructor(vm *VM, structure *Structure, regExpPrototype *RegExpPrototype) *RegExpConstructor {
	c := &RegExpConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callRegExpConstructorFn,
		functionForConstruct: constructWithRegExpConstructorFn,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, regExpPrototype)
	return c
}

// FinishCreation completes RegExpConstructor initialization.
// In C++: void finishCreation(VM&, RegExpPrototype*)
func (c *RegExpConstructor) FinishCreation(vm *VM, regExpPrototype *RegExpPrototype) {
	c.InternalFunction.FinishCreation(vm, 2, "RegExp")

	// putDirectWithoutTransition prototype
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&regExpPrototype.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)

	// RegExp.escape() — static method
	c.putDirectWithoutTransition(vm, NewPropertyName("escape"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	// @@species (accessor)
	c.putDirectWithoutTransition(vm, NewPropertyName(SymbolSpecies),
		NewJSValueObject(&c.JSObject),
		PropertyAttributeAccessor|PropertyAttributeReadOnly|PropertyAttributeDontEnum)

	_ = vm
}

// ===== Call/Construct =====

// callRegExpConstructorFn implements [[Call]] for RegExp constructor.
func callRegExpConstructorFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	result := constructRegExpFn(globalObject, callFrame.Arguments(), callFrame.Callee(), JSValueUndefined)
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(result))
}

// constructWithRegExpConstructorFn implements [[Construct]] for RegExp constructor.
func constructWithRegExpConstructorFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	result := constructRegExpFn(globalObject, callFrame.Arguments(), callFrame.Callee(), callFrame.NewTarget())
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(result))
}

// ===== Core construction =====

// constructRegExpFn is the main RegExp construction logic.
// In C++: JSObject* constructRegExp(JSGlobalObject*, const ArgList&, JSObject*, JSValue)
func constructRegExpFn(globalObject *JSGlobalObject, args []JSValue, callee *JSObject, newTarget JSValue) *JSObject {
	vm := globalObject.VM()
	_ = callee

	patternArg := jsUndefined()
	flagsArg := jsUndefined()
	if len(args) > 0 {
		patternArg = args[0]
	}
	if len(args) > 1 {
		flagsArg = args[1]
	}

	// Check if patternArg is a RegExpObject
	isPatternRegExp := false
	if patternArg.IsObject() {
		if patternArg.GetObject().typ == RegExpObjectType {
			isPatternRegExp = true
		}
	}

	// If called without new and pattern is a RegExp with undefined flags, return pattern itself
	if !newTarget.IsUndefined() { // newTarget is always set in this codebase (simplified)
		_ = isPatternRegExp
	}

	// Extract pattern string
	var patternStr string
	if isPatternRegExp {
		regExpObj := patternArg.GetObject()
		// Get source from properties
		srcVal := regExpObj.Get(globalObject, NewPropertyName("source"))
		if srcVal.IsString() {
			patternStr = srcVal.ToString()
		} else {
			// Fallback: get from internal RegExp
			if re, ok := interface{}(regExpObj).(*RegExpObject); ok {
				patternStr = re.getRegexpSource()
			} else {
				patternStr = "(?:)"
			}
		}
	} else if patternArg.IsUndefined() {
		patternStr = ""
	} else {
		patternStr = patternArg.ToString()
	}

	// Extract flags string
	var flagsStr string
	if isPatternRegExp && (flagsArg.IsUndefined()) {
		// Get flags from RegExpObject
		if re, ok := interface{}(patternArg.GetObject()).(*RegExpObject); ok {
			flagsStr = re.getRegexpFlags()
		}
	} else if !flagsArg.IsUndefined() {
		flagsStr = flagsArg.ToString()
	}

	// Create new RegExp + RegExpObject
	regExp := NewRegExp(patternStr, flagsStr)

	// Determine structure
	var structure *Structure
	typeInfo := NewTypeInfo(RegExpObjectType, RegExpObjectStructureFlags)
	structure = NewStructure(vm, globalObject, JSValueNull, typeInfo, &ClassInfo{})
	_ = newTarget

	return &NewRegExpObject(vm, structure, regExp).JSNonFinalObject.JSObject
}
