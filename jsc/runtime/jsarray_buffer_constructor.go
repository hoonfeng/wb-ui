// Translation of: Source/JavaScriptCore/runtime/JSArrayBufferConstructor.h/.cpp
package runtime

// JSArrayBufferConstructor corresponds to JSC::JSArrayBufferConstructor.
type JSArrayBufferConstructor struct {
	InternalFunction
}

const JSArrayBufferConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

func NewJSArrayBufferConstructor(vm *VM, structure *Structure, arrayBufferPrototype *JSArrayBufferPrototype) *JSArrayBufferConstructor {
	c := &JSArrayBufferConstructor{}
	c.structureID = structure.structureID
	c.typ = structure.Type()
	c.cellState = 1
	c.properties = make(map[string]JSValue)
	if arrayBufferPrototype != nil {
		c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
			NewJSValueObject(&arrayBufferPrototype.JSNonFinalObject.JSObject),
			PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
	}
	return c
}

func (c *JSArrayBufferConstructor) FinishCreation(vm *VM) {
	c.InternalFunction.FinishCreation(vm, 1, "ArrayBuffer")
	c.putDirectWithoutTransition(vm, NewPropertyName("length"), jsNumber(1), PropertyAttributeDontEnum|PropertyAttributeReadOnly)
	c.putDirectWithoutTransition(vm, NewPropertyName("name"), NewJSValueString("ArrayBuffer"), PropertyAttributeDontEnum|PropertyAttributeReadOnly)
}

// CallArrayBufferConstructor implements ArrayBuffer() call.
func CallArrayBufferConstructor(globalObject *JSGlobalObject, exec *ExecState) (JSValue, error) {
	// ArrayBuffer must be called with new
	exec.VM().ThrowException(globalObject, "Constructor ArrayBuffer requires 'new'")
	return JSValueUndefined, nil
}

// ConstructArrayBufferConstructor implements new ArrayBuffer(length).
func ConstructArrayBufferConstructor(globalObject *JSGlobalObject, exec *ExecState) (JSValue, error) {
	var byteLen uint32
	if exec.ArgumentCount() > 0 {
		n := exec.arguments[0].ToNumber()
		if n < 0 || n > 4294967296 || n != n {
			exec.VM().ThrowException(globalObject, "Invalid ArrayBuffer length")
			return JSValueUndefined, nil
		}
		byteLen = uint32(n)
	}
	buf := NewArrayBuffer(byteLen)
	jsBuf := NewJSArrayBuffer(globalObject.VM(), nil, buf)
	return NewJSValueObject(&jsBuf.JSObject), nil
}
