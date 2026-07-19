// Translation of: Source/JavaScriptCore/runtime/JSDataView.h/.cpp
//
// DataView provides a low-level interface for reading/writing
// multiple number types from an ArrayBuffer.
package runtime

import "math"

// JSDataView corresponds to JSC::JSDataView.
type JSDataView struct {
	JSArrayBufferView
}

const JSDataViewStructureFlags uint32 = JSArrayBufferViewStructureFlags

func NewJSDataView(vm *VM, structure *Structure, buffer *JSArrayBuffer, byteOffset uint32, byteLen uint32) *JSDataView {
	v := &JSDataView{}
	v.structureID = structure.structureID
	v.typ = DataViewType
	v.cellState = 1
	v.properties = make(map[string]JSValue)
	v.AttachBuffer(buffer, byteOffset, byteLen)
	return v
}

func (v *JSDataView) GetInt8(byteIndex uint32) int8 {
	if v.IsDetached() || byteIndex >= v.byteLen {
		return 0
	}
	return int8(v.vector[byteIndex])
}

func (v *JSDataView) GetUint8(byteIndex uint32) uint8 {
	if v.IsDetached() || byteIndex >= v.byteLen {
		return 0
	}
	return v.vector[byteIndex]
}

func (v *JSDataView) GetInt16(byteIndex uint32, littleEndian bool) int16 {
	if v.IsDetached() || byteIndex+2 > v.byteLen {
		return 0
	}
	if littleEndian {
		return int16(binaryLE.Uint16(v.vector[byteIndex:]))
	}
	return int16(uint16(v.vector[byteIndex])<<8 | uint16(v.vector[byteIndex+1]))
}

func (v *JSDataView) GetUint16(byteIndex uint32, littleEndian bool) uint16 {
	if v.IsDetached() || byteIndex+2 > v.byteLen {
		return 0
	}
	if littleEndian {
		return binaryLE.Uint16(v.vector[byteIndex:])
	}
	return uint16(v.vector[byteIndex])<<8 | uint16(v.vector[byteIndex+1])
}

func (v *JSDataView) GetInt32(byteIndex uint32, littleEndian bool) int32 {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return 0
	}
	if littleEndian {
		return int32(binaryLE.Uint32(v.vector[byteIndex:]))
	}
	return int32(uint32(v.vector[byteIndex])<<24 | uint32(v.vector[byteIndex+1])<<16 | uint32(v.vector[byteIndex+2])<<8 | uint32(v.vector[byteIndex+3]))
}

func (v *JSDataView) GetUint32(byteIndex uint32, littleEndian bool) uint32 {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return 0
	}
	if littleEndian {
		return binaryLE.Uint32(v.vector[byteIndex:])
	}
	return uint32(v.vector[byteIndex])<<24 | uint32(v.vector[byteIndex+1])<<16 | uint32(v.vector[byteIndex+2])<<8 | uint32(v.vector[byteIndex+3])
}

func (v *JSDataView) GetFloat32(byteIndex uint32, littleEndian bool) float32 {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return 0
	}
	var bits uint32
	if littleEndian {
		bits = binaryLE.Uint32(v.vector[byteIndex:])
	} else {
		bits = uint32(v.vector[byteIndex])<<24 | uint32(v.vector[byteIndex+1])<<16 | uint32(v.vector[byteIndex+2])<<8 | uint32(v.vector[byteIndex+3])
	}
	return math.Float32frombits(bits)
}

func (v *JSDataView) GetFloat64(byteIndex uint32, littleEndian bool) float64 {
	if v.IsDetached() || byteIndex+8 > v.byteLen {
		return 0
	}
	var bits uint64
	if littleEndian {
		bits = binaryLE.Uint64(v.vector[byteIndex:])
	} else {
		bits = uint64(v.vector[byteIndex])<<56 | uint64(v.vector[byteIndex+1])<<48 | uint64(v.vector[byteIndex+2])<<40 | uint64(v.vector[byteIndex+3])<<32 |
			uint64(v.vector[byteIndex+4])<<24 | uint64(v.vector[byteIndex+5])<<16 | uint64(v.vector[byteIndex+6])<<8 | uint64(v.vector[byteIndex+7])
	}
	return math.Float64frombits(bits)
}

func (v *JSDataView) SetInt8(byteIndex uint32, value int8) {
	if v.IsDetached() || byteIndex >= v.byteLen {
		return
	}
	v.vector[byteIndex] = byte(value)
}

func (v *JSDataView) SetUint8(byteIndex uint32, value uint8) {
	if v.IsDetached() || byteIndex >= v.byteLen {
		return
	}
	v.vector[byteIndex] = value
}

func (v *JSDataView) SetInt16(byteIndex uint32, value int16, littleEndian bool) {
	if v.IsDetached() || byteIndex+2 > v.byteLen {
		return
	}
	if littleEndian {
		binaryLE.PutUint16(v.vector[byteIndex:], uint16(value))
	} else {
		v.vector[byteIndex] = byte(uint16(value) >> 8)
		v.vector[byteIndex+1] = byte(value)
	}
}

func (v *JSDataView) SetUint16(byteIndex uint32, value uint16, littleEndian bool) {
	if v.IsDetached() || byteIndex+2 > v.byteLen {
		return
	}
	if littleEndian {
		binaryLE.PutUint16(v.vector[byteIndex:], value)
	} else {
		v.vector[byteIndex] = byte(value >> 8)
		v.vector[byteIndex+1] = byte(value)
	}
}

func (v *JSDataView) SetInt32(byteIndex uint32, value int32, littleEndian bool) {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return
	}
	if littleEndian {
		binaryLE.PutUint32(v.vector[byteIndex:], uint32(value))
	} else {
		v.vector[byteIndex] = byte(uint32(value) >> 24)
		v.vector[byteIndex+1] = byte(uint32(value) >> 16)
		v.vector[byteIndex+2] = byte(uint32(value) >> 8)
		v.vector[byteIndex+3] = byte(value)
	}
}

func (v *JSDataView) SetUint32(byteIndex uint32, value uint32, littleEndian bool) {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return
	}
	if littleEndian {
		binaryLE.PutUint32(v.vector[byteIndex:], value)
	} else {
		v.vector[byteIndex] = byte(value >> 24)
		v.vector[byteIndex+1] = byte(value >> 16)
		v.vector[byteIndex+2] = byte(value >> 8)
		v.vector[byteIndex+3] = byte(value)
	}
}

func (v *JSDataView) SetFloat32(byteIndex uint32, value float32, littleEndian bool) {
	if v.IsDetached() || byteIndex+4 > v.byteLen {
		return
	}
	bits := math.Float32bits(value)
	if littleEndian {
		binaryLE.PutUint32(v.vector[byteIndex:], bits)
	} else {
		v.vector[byteIndex] = byte(bits >> 24)
		v.vector[byteIndex+1] = byte(bits >> 16)
		v.vector[byteIndex+2] = byte(bits >> 8)
		v.vector[byteIndex+3] = byte(bits)
	}
}

func (v *JSDataView) SetFloat64(byteIndex uint32, value float64, littleEndian bool) {
	if v.IsDetached() || byteIndex+8 > v.byteLen {
		return
	}
	bits := math.Float64bits(value)
	if littleEndian {
		binaryLE.PutUint64(v.vector[byteIndex:], bits)
	} else {
		v.vector[byteIndex] = byte(bits >> 56)
		v.vector[byteIndex+1] = byte(bits >> 48)
		v.vector[byteIndex+2] = byte(bits >> 40)
		v.vector[byteIndex+3] = byte(bits >> 32)
		v.vector[byteIndex+4] = byte(bits >> 24)
		v.vector[byteIndex+5] = byte(bits >> 16)
		v.vector[byteIndex+6] = byte(bits >> 8)
		v.vector[byteIndex+7] = byte(bits)
	}
}
