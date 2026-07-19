// NumberObject corresponds to JSC::NumberObject (runtime/NumberObject.h)
package runtime

// NumberObject corresponds to JSC::NumberObject — a wrapper for a number value.
type NumberObject struct {
	JSObject
	internalValue JSValue // wrapped number value
}

// NewNumberObject creates a new NumberObject.
func NewNumberObject(vm *VM, structure *Structure) *NumberObject {
	n := &NumberObject{}
	n.structureID = structure.structureID
	n.typ = ObjectType
	n.cellState = DefinitelyWhite
	n.properties = make(map[string]JSValue)
	_ = vm
	return n
}

// InternalValue returns the wrapped number value.
func (n *NumberObject) InternalValue() JSValue { return n.internalValue }

// SetInternalValue sets the wrapped number value.
func (n *NumberObject) SetInternalValue(v JSValue) { n.internalValue = v }
