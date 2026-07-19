// JSArrayBuffer corresponds to JSC::JSArrayBuffer.
package runtime

// JSArrayBuffer corresponds to JSC::JSArrayBuffer.
// Backed by a Go byte slice.
type JSArrayBuffer struct {
	JSObject
	data     []byte
	byteLen  uint32
	isShared bool
}

const ArrayBufferStructureFlags uint32 = JSObjectStructureFlags

func NewJSArrayBuffer(vm *VM, structure *Structure, size uint32) *JSArrayBuffer {
	buf := &JSArrayBuffer{
		data:     make([]byte, size),
		byteLen:  size,
	}
	buf.structureID = structure.structureID
	buf.typ = ArrayBufferType
	buf.cellState = DefinitelyWhite
	buf.properties = make(map[string]JSValue)
	return buf
}

func NewJSArrayBufferFromBytes(vm *VM, structure *Structure, data []byte) *JSArrayBuffer {
	buf := &JSArrayBuffer{
		data:    data,
		byteLen: uint32(len(data)),
	}
	buf.structureID = structure.structureID
	buf.typ = ArrayBufferType
	buf.cellState = DefinitelyWhite
	buf.properties = make(map[string]JSValue)
	return buf
}

func (b *JSArrayBuffer) ByteLength() uint32 { return b.byteLen }
func (b *JSArrayBuffer) Data() []byte       { return b.data }
