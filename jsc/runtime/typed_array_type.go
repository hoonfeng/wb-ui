// Translation of: Source/JavaScriptCore/runtime/TypedArrayType.h
//
// TypedArrayType enumerates all TypedArray variants.
package runtime

// TypedArrayType corresponds to JSC::TypedArrayType.
type TypedArrayType uint8

const (
	NotTypedArray TypedArrayType = iota
	TypeInt8
	TypeUint8
	TypeUint8Clamped
	TypeInt16
	TypeUint16
	TypeInt32
	TypeUint32
	TypeFloat16
	TypeFloat32
	TypeFloat64
	TypeBigInt64
	TypeBigUint64
	TypeDataView
)

// TypedArrayContentType corresponds to JSC::TypedArrayContentType.
type TypedArrayContentType uint8

const (
	ContentTypeNone   TypedArrayContentType = iota
	ContentTypeNumber
	ContentTypeBigInt
)

// ToIndex converts TypedArrayType to a 0-based index.
func (t TypedArrayType) ToIndex() uint32 {
	return uint32(t) - 1
}

// IndexToTypedArrayType converts an index back to TypedArrayType.
func IndexToTypedArrayType(index uint32) TypedArrayType {
	return TypedArrayType(index + 1)
}

// TypedArrayTypeFromJSType returns the TypedArrayType for a given JSType.
func TypedArrayTypeFromJSType(typ JSType) TypedArrayType {
	switch typ {
	case Int8ArrayType:
		return TypeInt8
	case Uint8ArrayType:
		return TypeUint8
	case Uint8ClampedArrayType:
		return TypeUint8Clamped
	case Int16ArrayType:
		return TypeInt16
	case Uint16ArrayType:
		return TypeUint16
	case Int32ArrayType:
		return TypeInt32
	case Uint32ArrayType:
		return TypeUint32
	case Float16ArrayType:
		return TypeFloat16
	case Float32ArrayType:
		return TypeFloat32
	case Float64ArrayType:
		return TypeFloat64
	case BigInt64ArrayType:
		return TypeBigInt64
	case BigUint64ArrayType:
		return TypeBigUint64
	case DataViewType:
		return TypeDataView
	default:
		return NotTypedArray
	}
}

// JSTypeForTypedArrayType returns the JSType for a given TypedArrayType.
func JSTypeForTypedArrayType(t TypedArrayType) JSType {
	return FirstTypedArrayType + JSType(t) - JSType(TypeInt8)
}

// IsTypedView returns true if the TypedArrayType is a numeric typed view (not DataView).
func (t TypedArrayType) IsTypedView() bool {
	return t >= TypeInt8 && t <= TypeBigUint64
}

// IsBigIntTypedView returns true if the TypedArrayType is a BigInt typed view.
func (t TypedArrayType) IsBigIntTypedView() bool {
	return t == TypeBigInt64 || t == TypeBigUint64
}

// LogElementSize returns the log2 of element size.
func (t TypedArrayType) LogElementSize() uint32 {
	switch t {
	case TypeInt8, TypeUint8, TypeUint8Clamped, TypeDataView:
		return 0
	case TypeInt16, TypeUint16, TypeFloat16:
		return 1
	case TypeInt32, TypeUint32, TypeFloat32:
		return 2
	case TypeFloat64, TypeBigInt64, TypeBigUint64:
		return 3
	default:
		return 0
	}
}

// ElementSize returns the element size in bytes.
func (t TypedArrayType) ElementSize() uint32 {
	return 1 << t.LogElementSize()
}

// IsInt returns true if the type is an integer type.
func (t TypedArrayType) IsInt() bool {
	switch t {
	case TypeInt8, TypeUint8, TypeUint8Clamped, TypeInt16, TypeUint16, TypeInt32, TypeUint32:
		return true
	default:
		return false
	}
}

// IsFloat returns true if the type is a floating-point type.
func (t TypedArrayType) IsFloat() bool {
	switch t {
	case TypeFloat16, TypeFloat32, TypeFloat64:
		return true
	default:
		return false
	}
}

// IsBigInt returns true if the type is a BigInt type.
func (t TypedArrayType) IsBigInt() bool {
	return t == TypeBigInt64 || t == TypeBigUint64
}

// IsSigned returns true if the type is signed.
func (t TypedArrayType) IsSigned() bool {
	switch t {
	case TypeInt8, TypeInt16, TypeInt32, TypeFloat16, TypeFloat32, TypeFloat64, TypeBigInt64:
		return true
	default:
		return false
	}
}

// IsClamped returns true if the type is clamped (Uint8Clamped).
func (t TypedArrayType) IsClamped() bool {
	return t == TypeUint8Clamped
}

// ContentType returns the content type (Number/BigInt/None) for a given JSType.
func ContentType(typ JSType) TypedArrayContentType {
	if IsTypedArrayType(typ) {
		switch typ {
		case BigInt64ArrayType, BigUint64ArrayType:
			return ContentTypeBigInt
		default:
			return ContentTypeNumber
		}
	}
	return ContentTypeNone
}

// ContentTypeFromTAT returns the content type from TypedArrayType.
func ContentTypeFromTAT(t TypedArrayType) TypedArrayContentType {
	switch t {
	case TypeBigInt64, TypeBigUint64:
		return ContentTypeBigInt
	case TypeInt8, TypeUint8, TypeUint8Clamped, TypeInt16, TypeUint16, TypeInt32, TypeUint32:
		return ContentTypeNumber
	case TypeFloat16, TypeFloat32, TypeFloat64:
		return ContentTypeNumber
	default:
		return ContentTypeNone
	}
}
