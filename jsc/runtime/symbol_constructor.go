// Translation of: Source/JavaScriptCore/runtime/SymbolConstructor.h
//                  Source/JavaScriptCore/runtime/SymbolConstructor.cpp
//
// SymbolConstructor implements the ES Symbol() function and the Symbol constructor.
// Symbol() as a function creates a new Symbol value; new Symbol() throws TypeError.

package runtime

import "fmt"

// SymbolConstructor corresponds to JSC::SymbolConstructor.
type SymbolConstructor struct {
	InternalFunction
}

// NewSymbolConstructor creates a new SymbolConstructor.
func NewSymbolConstructor(vm *VM, structure *Structure, prototype *SymbolPrototype) *SymbolConstructor {
	c := &SymbolConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject:    JSNonFinalObject{},
		functionForCall:     callSymbolFn,
		functionForConstruct: constructSymbolFn,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, prototype)
	return c
}

// FinishCreation completes SymbolConstructor initialization.
func (c *SymbolConstructor) FinishCreation(vm *VM, prototype *SymbolPrototype) {
	c.InternalFunction.FinishCreation(vm, 0, "Symbol")
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&prototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)

	// Static methods: Symbol.for(), Symbol.keyFor()
	c.putDirectWithoutTransition(vm, NewPropertyName("for"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("keyFor"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Well-known symbols — create real Symbol instances as property values
	wellKnown := map[string]string{
		"iterator":           "Symbol.iterator",
		"asyncIterator":      "Symbol.asyncIterator",
		"match":              "Symbol.match",
		"replace":            "Symbol.replace",
		"search":             "Symbol.search",
		"split":              "Symbol.split",
		"hasInstance":        "Symbol.hasInstance",
		"isConcatSpreadable": "Symbol.isConcatSpreadable",
		"unscopables":        "Symbol.unscopables",
		"species":            "Symbol.species",
		"toPrimitive":        "Symbol.toPrimitive",
		"toStringTag":        "Symbol.toStringTag",
		"matchAll":           "Symbol.matchAll",
	}
	for name, desc := range wellKnown {
		sym := NewSymbolWithDescription(vm, desc)
		c.putDirectWithoutTransition(vm, NewPropertyName(name), NewJSValueSymbolCell(sym), PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	}
}

// --- Call / Construct ---

// callSymbolFn implements Symbol() as a function:
//   Symbol()          → new unique Symbol with no description
//   Symbol(desc)      → new unique Symbol with the given description
//   Symbol(Symbol())  → returns the same Symbol value (identity)
func callSymbolFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()

	if callFrame.ArgumentCount() == 0 {
		return NewJSValueSymbolCell(NewSymbol(vm))
	}

	firstArg := callFrame.Argument(0)

	// If the argument is already a Symbol, return it as-is (Symbol identity)
	if firstArg.IsSymbol() {
		return firstArg
	}

	// Convert argument to string description
	desc := firstArg.ToString()
	return NewJSValueSymbolCell(NewSymbolWithDescription(vm, desc))
}

// constructSymbolFn implements new Symbol() — always throws TypeError.
func constructSymbolFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	globalObject.VM().ThrowException(globalObject, fmt.Sprintf("TypeError: Symbol is not a constructor"))
	return JSValueUndefined
}

// symbolConstructorForFn implements Symbol.for(key).
func symbolConstructorForFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	desc := ""
	if callFrame.ArgumentCount() > 0 {
		desc = callFrame.Argument(0).ToString()
	}
	return NewJSValueSymbolCell(NewSymbolWithDescription(globalObject.VM(), desc))
}

// symbolConstructorKeyForFn implements Symbol.keyFor(sym).
func symbolConstructorKeyForFn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	if callFrame.ArgumentCount() == 0 || !callFrame.Argument(0).IsSymbol() {
		return JSValueUndefined
	}
	// Simplified: no global registry yet, always returns undefined
	return JSValueUndefined
}
