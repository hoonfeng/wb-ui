// Translation of: Source/JavaScriptCore/runtime/SymbolPrototype.h
//                  Source/JavaScriptCore/runtime/SymbolPrototype.cpp
//
// SymbolPrototype implements Symbol.prototype — the prototype of all Symbol values.

package runtime

// SymbolPrototype corresponds to JSC::SymbolPrototype.
type SymbolPrototype struct {
	JSNonFinalObject
}

// NewSymbolPrototype creates a new SymbolPrototype.
func NewSymbolPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *SymbolPrototype {
	p := &SymbolPrototype{}
	p.structureID = structure.structureID
	p.typ = ObjectType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	_ = globalObject
	p.FinishCreation(vm)
	return p
}

// FinishCreation completes SymbolPrototype initialization.
func (p *SymbolPrototype) FinishCreation(vm *VM) {
	// Symbol.prototype.toString()
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	// Symbol.prototype.valueOf()
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	// Symbol.prototype.description getter
	p.putDirectWithoutTransition(vm, NewPropertyName("description"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	// Symbol.toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Symbol"), PropertyAttributeDontEnum)
	_ = vm
}

// --- Symbol.prototype methods ---

// symbolProtoToString implements Symbol.prototype.toString():
//   Returns "Symbol(<description>)" or "Symbol()".
func symbolProtoToString(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	thisVal := callFrame.ThisValue()
	sym, err := thisSymbolValue(globalObject, thisVal)
	if err != nil {
		return JSValueUndefined, err
	}
	return NewJSValueString(sym.DescriptiveString()), nil
}

// symbolProtoValueOf implements Symbol.prototype.valueOf():
//   Returns the Symbol value itself.
func symbolProtoValueOf(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	thisVal := callFrame.ThisValue()
	sym, err := thisSymbolValue(globalObject, thisVal)
	if err != nil {
		return JSValueUndefined, err
	}
	return NewJSValueSymbolCell(sym), nil
}

// symbolProtoDescription implements Symbol.prototype.description getter:
//   Returns the description string or undefined.
func symbolProtoDescription(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	thisVal := callFrame.ThisValue()
	sym, err := thisSymbolValue(globalObject, thisVal)
	if err != nil {
		return JSValueUndefined, err
	}
	desc := sym.Description()
	if desc == "" {
		return JSValueUndefined, nil
	}
	return NewJSValueString(desc), nil
}

// symbolProtoToPrimitive implements Symbol.prototype[@@toPrimitive]:
//   Returns the Symbol value.
func symbolProtoToPrimitive(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	_ = globalObject
	thisVal := callFrame.ThisValue()
	if sym, ok := thisVal.payload.(*Symbol); ok {
		return NewJSValueSymbolCell(sym), nil
	}
	return JSValueUndefined, nil
}

// --- Helper ---

// thisSymbolValue extracts a Symbol from a value (unwraps SymbolObject if needed).
func thisSymbolValue(globalObject *JSGlobalObject, val JSValue) (*Symbol, error) {
	if val.IsSymbol() {
		if sym, ok := val.payload.(*Symbol); ok {
			return sym, nil
		}
	}
	// If it's a SymbolObject, unwrap it
	if symObj, ok := val.payload.(*SymbolObject); ok {
		if sym, ok := symObj.wrappedValue.payload.(*Symbol); ok {
			return sym, nil
		}
	}
	globalObject.VM().ThrowException(globalObject, "TypeError: Symbol.prototype method called on non-Symbol value")
	return nil, nil
}

// --- Well-known Symbol constants ---
// These are used as property name keys throughout the runtime.
// The actual Symbol instance values are created in SymbolConstructor.FinishCreation.

// SymbolToStringTag is the well-known Symbol.toStringTag property name.
const SymbolToStringTag = "Symbol.toStringTag"

// SymbolSpecies is the well-known Symbol.species property name used for
// @@species accessor in ArrayConstructor and other built-in constructors.
const SymbolSpecies = "Symbol.species"

// SymbolIterator is the well-known Symbol.iterator property name.
const SymbolIterator = "Symbol.iterator"

// SymbolUnscopables is the well-known Symbol.unscopables property name.
const SymbolUnscopables = "Symbol.unscopables"

// SymbolHasInstance is the well-known Symbol.hasInstance property name.
const SymbolHasInstance = "Symbol.hasInstance"

// SymbolToPrimitive is the well-known Symbol.toPrimitive property name.
const SymbolToPrimitive = "Symbol.toPrimitive"

// SymbolMatch is the well-known Symbol.match property name.
const SymbolMatch = "Symbol.match"

// SymbolMatchAll is the well-known Symbol.matchAll property name.
const SymbolMatchAll = "Symbol.matchAll"

// SymbolReplace is the well-known Symbol.replace property name.
const SymbolReplace = "Symbol.replace"

// SymbolSplit is the well-known Symbol.split property name.
const SymbolSplit = "Symbol.split"

// SymbolSearch is the well-known Symbol.search property name.
const SymbolSearch = "Symbol.search"
