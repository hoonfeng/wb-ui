// Translation of: Source/JavaScriptCore/runtime/JSArrayBuffer.h/.cpp
package runtime

// JSArrayBuffer corresponds to JSC::JSArrayBuffer.
// Wraps an ArrayBuffer (backed by Go []byte).
type JSArrayBuffer struct {
	JSObject
	buffer   *ArrayBuffer
	data     []byte
	byteLen  uint32
	isShared bool
}

const JSArrayBufferStructureFlags uint32 = JSObjectStructureFlags

func NewJSArrayBuffer(vm *VM, structure *Structure, buffer *ArrayBuffer) *JSArrayBuffer {
	b := &JSArrayBuffer{
		buffer:  buffer,
		data:    buffer.Data(),
		byteLen: uint32(buffer.ByteLength()),
	}
	b.structureID = structure.structureID
	b.typ = ArrayBufferType
	b.cellState = 1 // DefinitelyWhite
	b.properties = make(map[string]JSValue)
	return b
}

func NewJSArrayBufferFromSlice(vm *VM, structure *Structure, data []byte) *JSArrayBuffer {
	buf := NewArrayBufferFromSlice(data)
	return NewJSArrayBuffer(vm, structure, buf)
}

func (b *JSArrayBuffer) ByteLength() uint32 {
	if b.buffer != nil {
		return uint32(b.buffer.ByteLength())
	}
	return b.byteLen
}

func (b *JSArrayBuffer) Data() []byte {
	if b.buffer != nil && !b.buffer.IsDetached() {
		return b.buffer.Data()
	}
	return b.data
}

func (b *JSArrayBuffer) IsDetached() bool {
	return b.buffer == nil || b.buffer.IsDetached()
}

func (b *JSArrayBuffer) IsShared() bool {
	return b.isShared
}

func (b *JSArrayBuffer) Detach() {
	if b.buffer != nil {
		b.buffer.Detach()
	}
	b.data = nil
	b.byteLen = 0
}

func (b *JSArrayBuffer) Transfer(vm *VM) *JSArrayBuffer {
	if b.buffer != nil {
		newBuf := b.buffer.Transfer()
		if newBuf != nil {
			b.Detach()
			result := &JSArrayBuffer{
				buffer:  newBuf,
				data:    newBuf.Data(),
				byteLen: uint32(newBuf.ByteLength()),
			}
			result.structureID = b.structureID
			result.typ = ArrayBufferType
			result.cellState = 1
			result.properties = make(map[string]JSValue)
			return result
		}
	}
	return nil
}

// ArrayBuffer is the low-level backing store (simplified, no SharedArrayBuffer/WebAssembly).
type ArrayBuffer struct {
	data     []byte
	detached bool
}

func NewArrayBuffer(size uint32) *ArrayBuffer {
	return &ArrayBuffer{
		data: make([]byte, size),
	}
}

func NewArrayBufferFromSlice(src []byte) *ArrayBuffer {
	d := make([]byte, len(src))
	copy(d, src)
	return &ArrayBuffer{
		data: d,
	}
}

func NewArrayBufferFromBytes(src []byte) *ArrayBuffer {
	return NewArrayBufferFromSlice(src)
}

func (ab *ArrayBuffer) Data() []byte {
	if ab.detached {
		return nil
	}
	return ab.data
}

func (ab *ArrayBuffer) ByteLength() uint64 {
	if ab.detached {
		return 0
	}
	return uint64(len(ab.data))
}

func (ab *ArrayBuffer) IsDetached() bool {
	return ab.detached
}

func (ab *ArrayBuffer) Detach() {
	ab.data = nil
	ab.detached = true
}

func (ab *ArrayBuffer) Transfer() *ArrayBuffer {
	if ab.detached {
		return nil
	}
	newBuf := &ArrayBuffer{
		data: ab.data,
	}
	ab.data = nil
	ab.detached = true
	return newBuf
}

func (ab *ArrayBuffer) Slice(begin float64, end float64) *ArrayBuffer {
	if ab.detached {
		return nil
	}
	b := int(arrayBufferClampIndex(ab, begin))
	e := int(arrayBufferClampIndex(ab, end))
	if b > e {
		return NewArrayBuffer(0)
	}
	return NewArrayBufferFromSlice(ab.data[b:e])
}

func arrayBufferClampIndex(ab *ArrayBuffer, index float64) float64 {
	length := float64(len(ab.data))
	if index < 0 {
		index = length + index
	}
	if index < 0 {
		index = 0
	}
	if index > length {
		index = length
	}
	return index
}
