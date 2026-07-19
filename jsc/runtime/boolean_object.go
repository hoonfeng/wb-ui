// BooleanObject corresponds to JSC::BooleanObject (runtime/BooleanObject.h)
package runtime

// BooleanObject corresponds to JSC::BooleanObject — a wrapper for a boolean value.
type BooleanObject struct {
	JSObject
	internalValue JSValue // wrapped boolean value
}

// NewBooleanObject creates a new BooleanObject.
func NewBooleanObject(vm *VM, structure *Structure) *BooleanObject {
	b := &BooleanObject{}
	b.structureID = structure.structureID
	b.typ = ObjectType
	b.cellState = DefinitelyWhite
	b.properties = make(map[string]JSValue)
	_ = vm
	return b
}

// InternalValue returns the wrapped boolean value.
func (b *BooleanObject) InternalValue() JSValue { return b.internalValue }

// SetInternalValue sets the wrapped boolean value.
func (b *BooleanObject) SetInternalValue(v JSValue) { b.internalValue = v }
