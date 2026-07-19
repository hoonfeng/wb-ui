// JSArrayBufferView corresponds to JSC::JSArrayBufferView.
package runtime

// JSArrayBufferView corresponds to JSC::JSArrayBufferView.
// Base class for TypedArray views.
type JSArrayBufferView struct {
	JSObject
	buffer   *JSArrayBuffer
	byteLen  uint32
	byteOffset uint32
}

const ArrayBufferViewStructureFlags uint32 = JSObjectStructureFlags

func NewJSArrayBufferView(vm *VM, structure *Structure, buffer *JSArrayBuffer, byteOffset uint32, byteLen uint32) *JSArrayBufferView {
	view := &JSArrayBufferView{
		buffer:     buffer,
		byteOffset: byteOffset,
		byteLen:    byteLen,
	}
	view.structureID = structure.structureID
	view.typ = ArrayBufferType
	view.cellState = DefinitelyWhite
	view.properties = make(map[string]JSValue)
	return view
}

func (v *JSArrayBufferView) Buffer() *JSArrayBuffer { return v.buffer }
func (v *JSArrayBufferView) ByteLength() uint32     { return v.byteLen }
func (v *JSArrayBufferView) ByteOffset() uint32     { return v.byteOffset }
