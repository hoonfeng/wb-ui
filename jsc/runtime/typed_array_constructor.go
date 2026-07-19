// Translation of: Source/JavaScriptCore/runtime/JSGenericTypedArrayViewConstructor.h/.cpp
//
// Generic TypedArray constructor using Go generics.
package runtime

import "math"

// JSTypedArrayConstructor corresponds to JSC::JSGenericTypedArrayViewConstructor.
type JSTypedArrayConstructor struct {
	InternalFunction
}

func NewJSTypedArrayConstructor(vm *VM, structure *Structure) *JSTypedArrayConstructor {
	c := &JSTypedArrayConstructor{}
	c.structureID = structure.structureID
	c.typ = structure.Type()
	c.cellState = 1
	c.properties = make(map[string]JSValue)
	return c
}

// FinishCreation sets up the constructor.
func (c *JSTypedArrayConstructor) FinishCreation(vm *VM, length int, name string, prototype JSValue) {
	c.InternalFunction.FinishCreation(vm, length, name)
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"), prototype,
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
}

// ConstructTypedArray creates a typed array with the given element size from the call frame.
// Simplified: creates a zero-initialized buffer.
func (c *JSTypedArrayConstructor) ConstructTypedArray(globalObject *JSGlobalObject, exec *ExecState, elemSize uint32) *JSArrayBufferView {
	var length uint32
	if exec.ArgumentCount() > 0 {
		n := exec.arguments[0].ToNumber()
		if !math.IsNaN(n) && n >= 0 && !math.IsInf(n, 1) && n <= 4294967296 {
			length = uint32(n)
		}
	}
	totalBytes := length * elemSize
	buf := NewArrayBuffer(totalBytes)
	jsBuf := NewJSArrayBuffer(exec.VM(), nil, buf)
	view := NewJSArrayBufferView(exec.VM(), nil)
	view.AttachBuffer(jsBuf, 0, totalBytes)
	return view
}
