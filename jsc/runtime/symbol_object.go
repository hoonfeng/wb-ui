// Translation of: Source/JavaScriptCore/runtime/SymbolObject.h
//
// SymbolObject is the wrapper object form of a Symbol value (created by Object(sym)).

package runtime

// SymbolObject corresponds to JSC::SymbolObject.
// It wraps a Symbol value in an object form, allowing property access.
type SymbolObject struct {
	JSObject
	wrappedValue JSValue // the wrapped Symbol value
}

// NewSymbolObject creates a new SymbolObject wrapping the given Symbol.
func NewSymbolObject(vm *VM, structure *Structure, symbol *Symbol) *SymbolObject {
	obj := &SymbolObject{
		wrappedValue: NewJSValueSymbolCell(symbol),
	}
	obj.structureID = structure.structureID
	obj.typ = ObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	_ = vm
	return obj
}

// InternalValue returns the wrapped Symbol value.
func (so *SymbolObject) InternalValue() JSValue {
	return so.wrappedValue
}
