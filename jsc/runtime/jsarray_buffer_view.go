// Translation of: Source/JavaScriptCore/runtime/JSArrayBufferView.h/.cpp
package runtime

// JSArrayBufferView corresponds to JSC::JSArrayBufferView.
// Base class for all typed array views and DataView.
type JSArrayBufferView struct {
	JSObject
	buffer     *JSArrayBuffer
	byteLen    uint32
	byteOffset uint32
	vector     []byte
	isDetached bool
}

const JSArrayBufferViewStructureFlags uint32 = JSObjectStructureFlags

func NewJSArrayBufferView(vm *VM, structure *Structure) *JSArrayBufferView {
	v := &JSArrayBufferView{}
	v.structureID = structure.structureID
	v.typ = structure.Type()
	v.cellState = 1 // DefinitelyWhite
	v.properties = make(map[string]JSValue)
	return v
}

func (v *JSArrayBufferView) AttachBuffer(buffer *JSArrayBuffer, byteOffset uint32, byteLen uint32) {
	v.buffer = buffer
	v.byteOffset = byteOffset
	v.byteLen = byteLen
	if buffer != nil && !buffer.IsDetached() {
		data := buffer.Data()
		if uint32(len(data)) >= byteOffset+byteLen {
			v.vector = data[byteOffset : byteOffset+byteLen]
		} else {
			v.vector = nil
		}
	} else {
		v.vector = nil
	}
	v.isDetached = false
}

func (v *JSArrayBufferView) Buffer() *JSArrayBuffer { return v.buffer }

func (v *JSArrayBufferView) ByteLength() uint32 {
	if v.isDetached {
		return 0
	}
	return v.byteLen
}

func (v *JSArrayBufferView) ByteOffset() uint32 { return v.byteOffset }

func (v *JSArrayBufferView) Vector() []byte {
	if v.isDetached {
		return nil
	}
	return v.vector
}

func (v *JSArrayBufferView) IsDetached() bool {
	if v.isDetached {
		return true
	}
	if v.buffer != nil && v.buffer.IsDetached() {
		v.isDetached = true
		return true
	}
	return false
}

func (v *JSArrayBufferView) DetachFromArrayBuffer() {
	v.isDetached = true
	v.vector = nil
	v.byteLen = 0
}
