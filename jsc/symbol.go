// Translation of: Source/JavaScriptCore/runtime/SymbolConstructor.cpp
//                  Source/JavaScriptCore/runtime/SymbolPrototype.cpp
//                  Source/JavaScriptCore/runtime/Symbol.cpp
// Completeness: 50%
// Simplifications:
//   - Symbol values are plain Go strings with TagSymbol; no unique identity beyond
//     the description string. Symbol() creates a new unique symbol by appending a
//     counter to the description; Symbol.for() uses a global registry.
//   - No Symbol.prototype.description, no well-known symbol registry beyond
//     Symbol.iterator.
//   - No support for symbols as property keys beyond the well-known ones.

package jsc

import "fmt"

var (
	// symbolCounter ensures each Symbol() call produces a unique description.
	symbolCounter int
	// symbolRegistry holds the global Symbol.for() / Symbol.keyFor() registry.
	symbolRegistry = map[string]string{}
)

// SymbolConstructor builds and returns the Symbol constructor function, including
// static properties (Symbol.iterator, Symbol.for, Symbol.keyFor).
func (in *Interpreter) SymbolConstructor() *JSFunction {
	// Build the prototype.
	symProto := NewObject(in.objectProto)
	symProto.ClassName = "Symbol"
	// Store on interpreter for property lookup on symbol values.
	in.symbolProto = symProto

	// Symbol.prototype.toString()
	symProto.Set("toString", FunctionValue(NewNativeFunction("toString", func(in2 *Interpreter, this JSValue, _ []JSValue) JSValue {
		if this.IsSymbol() {
			return StringValue("Symbol(" + this.symbol + ")")
		}
		return StringValue("Symbol()")
	}, 0)))

	// Symbol.prototype.valueOf() - returns the symbol value itself.
	symProto.Set("valueOf", FunctionValue(NewNativeFunction("valueOf", func(in2 *Interpreter, this JSValue, _ []JSValue) JSValue {
		return this
	}, 0)))

	// Symbol constructor function.
	symCtor := NewNativeFunction("Symbol", func(in2 *Interpreter, this JSValue, args []JSValue) JSValue {
		desc := ""
		if len(args) > 0 && !args[0].IsUndefined() {
			desc = args[0].ToString()
		}
		symbolCounter++
		uniqueDesc := fmt.Sprintf("%s_%d", desc, symbolCounter)
		return SymbolValue(uniqueDesc)
	}, 1)
	symCtor.properties.Prototype = in.functionProto
	symCtor.properties.Set("prototype", ObjectValue(symProto))

	// Symbol.for(key) - global registry lookup.
	symCtor.properties.Set("for", FunctionValue(NewNativeFunction("for", func(in2 *Interpreter, _ JSValue, args []JSValue) JSValue {
		key := ""
		if len(args) > 0 {
			key = args[0].ToString()
		}
		if existing, ok := symbolRegistry[key]; ok {
			return SymbolValue(existing)
		}
		desc := "Symbol(" + key + ")"
		symbolRegistry[key] = desc
		return SymbolValue(desc)
	}, 1)))

	// Symbol.keyFor(sym) - retrieve key from global registry.
	symCtor.properties.Set("keyFor", FunctionValue(NewNativeFunction("keyFor", func(in2 *Interpreter, _ JSValue, args []JSValue) JSValue {
		if len(args) == 0 || !args[0].IsSymbol() {
			return Undefined()
		}
		symDesc := args[0].symbol
		// Look for the description in the registry.
		for k, v := range symbolRegistry {
			if v == symDesc {
				return StringValue(k)
			}
		}
		return Undefined()
	}, 1)))

	// Symbol.iterator - well-known symbol stored as a Symbol value on the constructor.
	symCtor.properties.Set("iterator", SymbolValue("Symbol.iterator"))

	return symCtor
}
