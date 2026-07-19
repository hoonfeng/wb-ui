// MathObject corresponds to JSC::MathObject (runtime/MathObject.h)
package runtime

import "math"

// MathObject corresponds to JSC::MathObject.
type MathObject struct {
	JSNonFinalObject
}

// NewMathObject creates a new MathObject.
func NewMathObject(vm *VM, globalObject *JSGlobalObject, structure *Structure) *MathObject {
	m := &MathObject{}
	m.structureID = structure.structureID
	m.typ = ObjectType
	m.cellState = DefinitelyWhite
	m.properties = make(map[string]JSValue)
	m.FinishCreation(vm, globalObject)
	return m
}

// FinishCreation completes MathObject initialization with all Math.* properties.
func (m *MathObject) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	// Constants
	m.putDirectWithoutTransition(vm, NewPropertyName("E"), jsNumber(math.E), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("LN10"), jsNumber(math.Ln10), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("LN2"), jsNumber(math.Ln2), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("LOG2E"), jsNumber(math.Log2E), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("LOG10E"), jsNumber(math.Log10E), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("PI"), jsNumber(math.Pi), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("SQRT1_2"), jsNumber(0.7071067811865476), PropertyAttributeDontEnum)
	m.putDirectWithoutTransition(vm, NewPropertyName("SQRT2"), jsNumber(math.Sqrt2), PropertyAttributeDontEnum)
	_ = globalObject
	_ = vm
}
