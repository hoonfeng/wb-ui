// Translation of: Source/JavaScriptCore/runtime/JSGenericTypedArrayView.h/.cpp
//
// Generic TypedArray implementation using Go generics for all 12 numeric types.
package runtime

import "math"

// TypedArrayAdaptor defines the interface for typed array element type adaptors.
type TypedArrayAdaptor[T any] interface {
	Type() JSType
	TypedArrayType() TypedArrayType
	ElementSize() uint32
	Load(data []byte, index uint32) T
	Store(data []byte, index uint32, value T)
	ToJSValue(v T) JSValue
	Canonicalize(v T) T
}

// --- Adaptor implementations ---

type Int8Adaptor struct{}

func (Int8Adaptor) Type() JSType                             { return Int8ArrayType }
func (Int8Adaptor) TypedArrayType() TypedArrayType            { return TypeInt8 }
func (Int8Adaptor) ElementSize() uint32                       { return 1 }
func (Int8Adaptor) Load(data []byte, index uint32) int8       { return int8(data[index]) }
func (Int8Adaptor) Store(data []byte, index uint32, v int8)   { data[index] = byte(v) }
func (Int8Adaptor) ToJSValue(v int8) JSValue                  { return jsNumber(float64(v)) }
func (Int8Adaptor) Canonicalize(v int8) int8                  { return v }

type Uint8Adaptor struct{}

func (Uint8Adaptor) Type() JSType                              { return Uint8ArrayType }
func (Uint8Adaptor) TypedArrayType() TypedArrayType             { return TypeUint8 }
func (Uint8Adaptor) ElementSize() uint32                        { return 1 }
func (Uint8Adaptor) Load(data []byte, index uint32) uint8       { return data[index] }
func (Uint8Adaptor) Store(data []byte, index uint32, v uint8)   { data[index] = v }
func (Uint8Adaptor) ToJSValue(v uint8) JSValue                  { return jsNumber(float64(v)) }
func (Uint8Adaptor) Canonicalize(v uint8) uint8                 { return v }

type Uint8ClampedAdaptor struct{}

func (Uint8ClampedAdaptor) Type() JSType                               { return Uint8ClampedArrayType }
func (Uint8ClampedAdaptor) TypedArrayType() TypedArrayType              { return TypeUint8Clamped }
func (Uint8ClampedAdaptor) ElementSize() uint32                         { return 1 }
func (Uint8ClampedAdaptor) Load(data []byte, index uint32) uint8        { return data[index] }
func (Uint8ClampedAdaptor) Store(data []byte, index uint32, v uint8)    { data[index] = v }
func (Uint8ClampedAdaptor) ToJSValue(v uint8) JSValue                   { return jsNumber(float64(v)) }
func (Uint8ClampedAdaptor) Canonicalize(v uint8) uint8                  { return v }

type Int16Adaptor struct{}

func (Int16Adaptor) Type() JSType                              { return Int16ArrayType }
func (Int16Adaptor) TypedArrayType() TypedArrayType             { return TypeInt16 }
func (Int16Adaptor) ElementSize() uint32                        { return 2 }
func (Int16Adaptor) Load(data []byte, index uint32) int16       { return int16(binaryLE.Uint16(data[index*2:])) }
func (Int16Adaptor) Store(data []byte, index uint32, v int16)   { binaryLE.PutUint16(data[index*2:], uint16(v)) }
func (Int16Adaptor) ToJSValue(v int16) JSValue                  { return jsNumber(float64(v)) }
func (Int16Adaptor) Canonicalize(v int16) int16                 { return v }

type Uint16Adaptor struct{}

func (Uint16Adaptor) Type() JSType                                 { return Uint16ArrayType }
func (Uint16Adaptor) TypedArrayType() TypedArrayType                { return TypeUint16 }
func (Uint16Adaptor) ElementSize() uint32                           { return 2 }
func (Uint16Adaptor) Load(data []byte, index uint32) uint16         { return binaryLE.Uint16(data[index*2:]) }
func (Uint16Adaptor) Store(data []byte, index uint32, v uint16)     { binaryLE.PutUint16(data[index*2:], v) }
func (Uint16Adaptor) ToJSValue(v uint16) JSValue                    { return jsNumber(float64(v)) }
func (Uint16Adaptor) Canonicalize(v uint16) uint16                  { return v }

type Int32Adaptor struct{}

func (Int32Adaptor) Type() JSType                              { return Int32ArrayType }
func (Int32Adaptor) TypedArrayType() TypedArrayType             { return TypeInt32 }
func (Int32Adaptor) ElementSize() uint32                        { return 4 }
func (Int32Adaptor) Load(data []byte, index uint32) int32       { return int32(binaryLE.Uint32(data[index*4:])) }
func (Int32Adaptor) Store(data []byte, index uint32, v int32)   { binaryLE.PutUint32(data[index*4:], uint32(v)) }
func (Int32Adaptor) ToJSValue(v int32) JSValue                  { return jsNumber(float64(v)) }
func (Int32Adaptor) Canonicalize(v int32) int32                 { return v }

type Uint32Adaptor struct{}

func (Uint32Adaptor) Type() JSType                                  { return Uint32ArrayType }
func (Uint32Adaptor) TypedArrayType() TypedArrayType                 { return TypeUint32 }
func (Uint32Adaptor) ElementSize() uint32                            { return 4 }
func (Uint32Adaptor) Load(data []byte, index uint32) uint32          { return binaryLE.Uint32(data[index*4:]) }
func (Uint32Adaptor) Store(data []byte, index uint32, v uint32)      { binaryLE.PutUint32(data[index*4:], v) }
func (Uint32Adaptor) ToJSValue(v uint32) JSValue                     { return jsNumber(float64(v)) }
func (Uint32Adaptor) Canonicalize(v uint32) uint32                   { return v }

type Float32Adaptor struct{}

func (Float32Adaptor) Type() JSType                              { return Float32ArrayType }
func (Float32Adaptor) TypedArrayType() TypedArrayType             { return TypeFloat32 }
func (Float32Adaptor) ElementSize() uint32                        { return 4 }
func (Float32Adaptor) Load(data []byte, index uint32) float32     { return math.Float32frombits(binaryLE.Uint32(data[index*4:])) }
func (Float32Adaptor) Store(data []byte, index uint32, v float32) { binaryLE.PutUint32(data[index*4:], math.Float32bits(v)) }
func (Float32Adaptor) ToJSValue(v float32) JSValue                { return jsNumber(float64(v)) }
func (Float32Adaptor) Canonicalize(v float32) float32             { return v }

type Float64Adaptor struct{}

func (Float64Adaptor) Type() JSType                              { return Float64ArrayType }
func (Float64Adaptor) TypedArrayType() TypedArrayType             { return TypeFloat64 }
func (Float64Adaptor) ElementSize() uint32                        { return 8 }
func (Float64Adaptor) Load(data []byte, index uint32) float64     { return math.Float64frombits(binaryLE.Uint64(data[index*8:])) }
func (Float64Adaptor) Store(data []byte, index uint32, v float64) { binaryLE.PutUint64(data[index*8:], math.Float64bits(v)) }
func (Float64Adaptor) ToJSValue(v float64) JSValue                { return jsNumber(v) }
func (Float64Adaptor) Canonicalize(v float64) float64             { return v }

type BigInt64Adaptor struct{}

func (BigInt64Adaptor) Type() JSType                              { return BigInt64ArrayType }
func (BigInt64Adaptor) TypedArrayType() TypedArrayType             { return TypeBigInt64 }
func (BigInt64Adaptor) ElementSize() uint32                        { return 8 }
func (BigInt64Adaptor) Load(data []byte, index uint32) int64       { return int64(binaryLE.Uint64(data[index*8:])) }
func (BigInt64Adaptor) Store(data []byte, index uint32, v int64)   { binaryLE.PutUint64(data[index*8:], uint64(v)) }
func (BigInt64Adaptor) ToJSValue(v int64) JSValue                  { return jsNumber(float64(v)) }
func (BigInt64Adaptor) Canonicalize(v int64) int64                 { return v }

type BigUint64Adaptor struct{}

func (BigUint64Adaptor) Type() JSType                                  { return BigUint64ArrayType }
func (BigUint64Adaptor) TypedArrayType() TypedArrayType                 { return TypeBigUint64 }
func (BigUint64Adaptor) ElementSize() uint32                            { return 8 }
func (BigUint64Adaptor) Load(data []byte, index uint32) uint64          { return binaryLE.Uint64(data[index*8:]) }
func (BigUint64Adaptor) Store(data []byte, index uint32, v uint64)      { binaryLE.PutUint64(data[index*8:], v) }
func (BigUint64Adaptor) ToJSValue(v uint64) JSValue                     { return jsNumber(float64(v)) }
func (BigUint64Adaptor) Canonicalize(v uint64) uint64                   { return v }

// binaryLE provides little-endian byte encoding/decoding.
var binaryLE = struct {
	Uint16    func([]byte) uint16
	PutUint16 func([]byte, uint16)
	Uint32    func([]byte) uint32
	PutUint32 func([]byte, uint32)
	Uint64    func([]byte) uint64
	PutUint64 func([]byte, uint64)
}{
	Uint16:    func(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 },
	PutUint16: func(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) },
	Uint32:    func(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24 },
	PutUint32: func(b []byte, v uint32) { b[0] = byte(v); b[1] = byte(v >> 8); b[2] = byte(v >> 16); b[3] = byte(v >> 24) },
	Uint64:    func(b []byte) uint64 { return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 | uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56 },
	PutUint64: func(b []byte, v uint64) { b[0] = byte(v); b[1] = byte(v >> 8); b[2] = byte(v >> 16); b[3] = byte(v >> 24); b[4] = byte(v >> 32); b[5] = byte(v >> 40); b[6] = byte(v >> 48); b[7] = byte(v >> 56) },
}

// JSTypedArrayView is the generic typed array view.
type JSTypedArrayView[T any, A TypedArrayAdaptor[T]] struct {
	JSArrayBufferView
	adaptor A
	length  uint32
}

const JSTypedArrayViewStructureFlags uint32 = JSArrayBufferViewStructureFlags

func NewJSTypedArrayView[T any, A TypedArrayAdaptor[T]](vm *VM, structure *Structure, buffer *JSArrayBuffer, byteOffset uint32, length uint32) *JSTypedArrayView[T, A] {
	var adaptor A
	elemSize := adaptor.ElementSize()
	byteLen := length * elemSize
	v := &JSTypedArrayView[T, A]{
		adaptor: adaptor,
		length:  length,
	}
	v.structureID = structure.structureID
	v.typ = adaptor.Type()
	v.cellState = 1 // DefinitelyWhite
	v.properties = make(map[string]JSValue)
	v.AttachBuffer(buffer, byteOffset, byteLen)
	return v
}

func (v *JSTypedArrayView[T, A]) Length() uint32 {
	if v.IsDetached() {
		return 0
	}
	return v.length
}

func (v *JSTypedArrayView[T, A]) Get(index uint32) T {
	if v.IsDetached() || index >= v.length {
		var zero T
		return zero
	}
	return v.adaptor.Load(v.vector, index)
}

func (v *JSTypedArrayView[T, A]) Set(index uint32, value T) bool {
	if v.IsDetached() || index >= v.length {
		return false
	}
	v.adaptor.Store(v.vector, index, v.adaptor.Canonicalize(value))
	return true
}

func (v *JSTypedArrayView[T, A]) GetJSValue(index uint32) JSValue {
	return v.adaptor.ToJSValue(v.Get(index))
}

func (v *JSTypedArrayView[T, A]) TypedArrayType() TypedArrayType {
	return v.adaptor.TypedArrayType()
}
