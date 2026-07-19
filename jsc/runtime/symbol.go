// Translation of: Source/JavaScriptCore/runtime/Symbol.h
//                  Source/JavaScriptCore/runtime/Symbol.cpp
//
// Symbol is the JSCell-based representation of an ES2015 Symbol value.
// In our Go architecture, well-known symbols are still represented as
// string constants for property key usage (PropertyName = string).
// This file implements the Symbol value type itself (uid + description).

package runtime

import (
	"fmt"
	"sync/atomic"
)

// Symbol corresponds to JSC::Symbol. It represents a unique Symbol value.
type Symbol struct {
	JSCell
	uid         uint64 // unique identifier (auto-incremented)
	description string // optional description string
}

var (
	symbolNextUID uint64 = 1
)

// newSymbolUID generates a new unique Symbol identifier.
func newSymbolUID() uint64 {
	return atomic.AddUint64(&symbolNextUID, 1)
}

// --- Factory methods ---

// NewSymbol creates a new Symbol without description.
func NewSymbol(vm *VM) *Symbol {
	s := &Symbol{
		uid: newSymbolUID(),
	}
	s.structureID = vm.symbolStructure.structureID
	s.typ = SymbolType
	s.cellState = DefinitelyWhite
	return s
}

// NewSymbolWithDescription creates a new Symbol with the given description.
func NewSymbolWithDescription(vm *VM, desc string) *Symbol {
	s := &Symbol{
		uid:         newSymbolUID(),
		description: desc,
	}
	s.structureID = vm.symbolStructure.structureID
	s.typ = SymbolType
	s.cellState = DefinitelyWhite
	return s
}

// NewWellKnownSymbol creates a Symbol with a specific uid (used for well-known symbols).
// The uid is a small number assigned by the well-known symbol table.
func NewWellKnownSymbol(vm *VM, uid uint64, description string) *Symbol {
	s := &Symbol{
		uid:         uid,
		description: description,
	}
	s.structureID = vm.symbolStructure.structureID
	s.typ = SymbolType
	s.cellState = DefinitelyWhite
	return s
}

// --- Methods ---

// UID returns the unique identifier of this Symbol.
func (s *Symbol) UID() uint64 {
	return s.uid
}

// Description returns the description string (may be empty).
func (s *Symbol) Description() string {
	return s.description
}

// PrivateName returns a string that uniquely identifies this Symbol as a property key.
// The format is "Symbol(<uid>)" to guarantee uniqueness.
func (s *Symbol) PrivateName() string {
	if s.description != "" {
		return fmt.Sprintf("Symbol(%s)", s.description)
	}
	return fmt.Sprintf("Symbol(<%d>)", s.uid)
}

// ToPrimitive implements Symbol.[Symbol.toPrimitive]() — returns the Symbol itself.
func (s *Symbol) ToPrimitive(globalObject *JSGlobalObject, hint PreferredPrimitiveType) JSValue {
	_ = globalObject
	_ = hint
	return NewJSValueSymbolCell(s)
}

// ToObject wraps this Symbol in a SymbolObject.
func (s *Symbol) ToObject(globalObject *JSGlobalObject) *JSObject {
	obj := &SymbolObject{
		wrappedValue: NewJSValueSymbolCell(s),
	}
	obj.structureID = globalObject.symbolObjectStructure.structureID
	obj.typ = ObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	return &obj.JSObject
}

// ToNumber always throws a TypeError — Symbols cannot be converted to numbers.
func (s *Symbol) ToNumber(globalObject *JSGlobalObject) (float64, error) {
	_ = s
	globalObject.VM().ThrowException(globalObject, "TypeError: Cannot convert a Symbol value to a number")
	return 0, nil
}

// ToString returns "Symbol(<description>)" or "Symbol()".
func (s *Symbol) ToString(globalObject *JSGlobalObject) string {
	_ = globalObject
	if s.description != "" {
		return "Symbol(" + s.description + ")"
	}
	return "Symbol()"
}

// DescriptiveString returns the descriptive string used by Symbol.prototype.toString().
func (s *Symbol) DescriptiveString() string {
	if s.description != "" {
		return "Symbol(" + s.description + ")"
	}
	return "Symbol()"
}

// --- Helper functions ---

// NewJSValueSymbolCell creates a JSValue wrapping a Symbol pointer.
func NewJSValueSymbolCell(s *Symbol) JSValue {
	return JSValue{
		tag:     TagSymbol,
		payload: s,
	}
}
