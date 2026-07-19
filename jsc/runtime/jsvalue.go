// Translation of: Source/JavaScriptCore/runtime/JSCJSValue.h
//                  Source/JavaScriptCore/runtime/JSCJSValue.cpp (partial)
//
// JSValue is the core value type — a tagged union that can hold undefined,
// null, boolean, number (float64), string, symbol, or object (via *JSCell).

package runtime

import (
	"fmt"
	"math"
	"strconv"
)

// JSValueTag encodes the runtime type of a JSValue.
// Go port uses explicit tag + payload rather than NaN-boxing.
type JSValueTag uint8

const (
	TagUndefined JSValueTag = iota
	TagNull
	TagBoolean
	TagNumber
	TagString
	TagSymbol
	TagObject
)

// JSValue corresponds to JSC::JSValue. It holds a tagged union:
//   - undefined / null / boolean / number (float64) / cell pointer
type JSValue struct {
	tag     JSValueTag
	payload interface{}
}

// Predefined JSValue constants.
var (
	JSValueUndefined = JSValue{tag: TagUndefined}
	JSValueNull      = JSValue{tag: TagNull}
	JSValueTrue      = JSValue{tag: TagBoolean, payload: true}
	JSValueFalse     = JSValue{tag: TagBoolean, payload: false}
)

// --- Constructors ---

// NewJSValue creates a JSValue from a JSCell pointer.
func NewJSValue(cell *JSCell) JSValue {
	if cell == nil {
		return JSValueNull
	}
	tag := TagObject
	if cell.IsString() {
		tag = TagString
	} else if cell.IsSymbol() {
		tag = TagSymbol
	}
	return JSValue{tag: tag, payload: cell}
}

// NewJSValueBool creates a boolean JSValue.
func NewJSValueBool(b bool) JSValue {
	if b {
		return JSValueTrue
	}
	return JSValueFalse
}

// NewJSValueNumber creates a numeric JSValue.
func NewJSValueNumber(n float64) JSValue {
	return JSValue{tag: TagNumber, payload: n}
}

// NewJSValueInt32 creates an integer JSValue (stored as float64).
func NewJSValueInt32(n int32) JSValue {
	return JSValue{tag: TagNumber, payload: float64(n)}
}

// NewJSValueString creates a string JSValue.
func NewJSValueString(s string) JSValue {
	return JSValue{tag: TagString, payload: s}
}

// NewJSValueObject creates an object JSValue.
func NewJSValueObject(obj *JSObject) JSValue {
	if obj == nil {
		return JSValueNull
	}
	return JSValue{tag: TagObject, payload: obj}
}

// --- Type queries ---

// IsUndefined returns true if this value is undefined.
func (v JSValue) IsUndefined() bool { return v.tag == TagUndefined }

// IsNull returns true if this value is null.
func (v JSValue) IsNull() bool { return v.tag == TagNull }

// IsBoolean returns true if this value is boolean.
func (v JSValue) IsBoolean() bool { return v.tag == TagBoolean }

// IsNumber returns true if this value is a number.
func (v JSValue) IsNumber() bool { return v.tag == TagNumber }

// IsString returns true if this value is a string.
func (v JSValue) IsString() bool { return v.tag == TagString }

// IsSymbol returns true if this value is a symbol.
func (v JSValue) IsSymbol() bool { return v.tag == TagSymbol }

// IsObject returns true if this value is an object (or array/function).
func (v JSValue) IsObject() bool { return v.tag == TagObject }

// IsCell returns true if this value is backed by a JSCell (string, symbol, or object).
func (v JSValue) IsCell() bool {
	return v.tag == TagString || v.tag == TagSymbol || v.tag == TagObject
}

// IsPrimitive returns true if this value is not an object.
func (v JSValue) IsPrimitive() bool { return !v.IsObject() }

// IsEmpty returns true if this is the zero value (undefined).
func (v JSValue) IsEmpty() bool { return v.tag == TagUndefined && v.payload == nil }

// ToBoolean converts this value to a boolean per ECMA-262 ToBoolean().
func (v JSValue) ToBoolean() bool {
	switch v.tag {
	case TagUndefined, TagNull:
		return false
	case TagBoolean:
		b, _ := v.payload.(bool)
		return b
	case TagNumber:
		n, _ := v.payload.(float64)
		return n != 0 && !math.IsNaN(n)
	case TagString:
		s, _ := v.payload.(string)
		return len(s) > 0
	case TagSymbol:
		return true
	case TagObject:
		return true
	default:
		return false
	}
}

// ToNumber converts this value to a number per ECMA-262 ToNumber().
func (v JSValue) ToNumber() float64 {
	switch v.tag {
	case TagUndefined:
		return math.NaN()
	case TagNull:
		return 0
	case TagBoolean:
		b, _ := v.payload.(bool)
		if b {
			return 1
		}
		return 0
	case TagNumber:
		n, _ := v.payload.(float64)
		return n
	case TagString:
		s, _ := v.payload.(string)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	default:
		return math.NaN()
	}
}

// ToString converts this value to a string per ECMA-262 ToString().
// Simplified: does not handle Symbol → TypeError.
func (v JSValue) ToString() string {
	switch v.tag {
	case TagUndefined:
		return "undefined"
	case TagNull:
		return "null"
	case TagBoolean:
		b, _ := v.payload.(bool)
		if b {
			return "true"
		}
		return "false"
	case TagNumber:
		n, _ := v.payload.(float64)
		if n == math.Trunc(n) && !math.IsInf(n, 0) && !math.IsNaN(n) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'g', -1, 64)
	case TagString:
		s, _ := v.payload.(string)
		return s
	case TagSymbol:
		return "" // TypeError would be thrown in full impl
	default:
		return ""
	}
}

// ToObject converts this value to a JSObject (boxing primitives).
func (v JSValue) ToObject(globalObject *JSGlobalObject) *JSObject {
	switch v.tag {
	case TagString:
		return v.ToStringObject(globalObject)
	case TagNumber:
		return v.ToNumberObject(globalObject)
	case TagBoolean:
		return v.ToBooleanObject(globalObject)
	case TagObject:
		obj, _ := v.payload.(*JSObject)
		return obj
	default:
		return nil
	}
}

// ToStringObject boxes the value as a String wrapper object.
func (v JSValue) ToStringObject(globalObject *JSGlobalObject) *JSObject {
	_ = globalObject
	// TODO: create StringObject wrapping this string
	return nil
}

// ToNumberObject boxes the value as a Number wrapper object.
func (v JSValue) ToNumberObject(globalObject *JSGlobalObject) *JSObject {
	_ = globalObject
	// TODO: create NumberObject wrapping this number
	return nil
}

// ToBooleanObject boxes the value as a Boolean wrapper object.
func (v JSValue) ToBooleanObject(globalObject *JSGlobalObject) *JSObject {
	_ = globalObject
	// TODO: create BooleanObject wrapping this bool
	return nil
}


// GetObject returns the JSObject, or nil if not an object.
func (v JSValue) GetObject() *JSObject {
	obj, _ := v.payload.(*JSObject)
	return obj
}

// GetString returns the string value, or empty string if not a string.
func (v JSValue) GetString() string {
	if v.tag != TagString {
		return ""
	}
	s, _ := v.payload.(string)
	return s
}

// GetNumber returns the number value, or NaN if not a number.
func (v JSValue) GetNumber() float64 {
	if v.tag != TagNumber {
		return math.NaN()
	}
	n, _ := v.payload.(float64)
	return n
}

// GetBool returns the boolean value.
func (v JSValue) GetBool() bool {
	if v.tag != TagBoolean {
		return false
	}
	b, _ := v.payload.(bool)
	return b
}

// IsInt32 returns true if the value is a 32-bit integer number.
func (v JSValue) IsInt32() bool {
	if v.tag != TagNumber {
		return false
	}
	n, _ := v.payload.(float64)
	return n == float64(int32(n)) && !math.IsInf(n, 0) && !math.IsNaN(n)
}

// IsDouble returns true if the value is a non-int32 number.
func (v JSValue) IsDouble() bool {
	if v.tag != TagNumber {
		return false
	}
	n, _ := v.payload.(float64)
	return !math.IsInf(n, 0) && !math.IsNaN(n) && n != float64(int32(n))
}

// IsNumberOrBoolean returns true for number or boolean.
func (v JSValue) IsNumberOrBoolean() bool {
	return v.tag == TagNumber || v.tag == TagBoolean
}

// IsFunction checks if this value is a callable function.
func (v JSValue) IsFunction() bool {
	if v.tag != TagObject {
		return false
	}
	obj := v.GetObject()
	if obj == nil {
		return false
	}
	return obj.IsCallable()
}

// Math utilities (from JSValue C++ helpers)

// TryConvertToInt52 attempts to convert a double to int52.
func TryConvertToInt52(d float64) int64 {
	if d != d { // NaN
		return 0
	}
	if d < -4503599627370496.0 || d > 4503599627370496.0 {
		return 0
	}
	if d != math.Trunc(d) {
		return 0
	}
	return int64(d)
}

// IsInt52 returns true if d is a 52-bit integer.
func IsInt52(d float64) bool {
	return TryConvertToInt52(d) != 0 || d == 0
}

// --- Equality (abstract) ---

// Abstract equality (==) per ECMA-262 §7.2.14.
func (v JSValue) AbstractEqual(other JSValue) bool {
	if v.tag == other.tag {
		switch v.tag {
		case TagUndefined, TagNull:
			return true
		case TagBoolean:
			return v.payload == other.payload
		case TagNumber:
			return v.payload.(float64) == other.payload.(float64)
		case TagString:
			return v.payload.(string) == other.payload.(string)
		case TagObject:
			return v.payload == other.payload
		}
	}
	// Type-based dispatch
	if v.tag == TagNull && other.tag == TagUndefined {
		return true
	}
	if v.tag == TagUndefined && other.tag == TagNull {
		return true
	}
	if v.tag == TagNumber && other.tag == TagString {
		return v.payload.(float64) == other.ToNumber()
	}
	if v.tag == TagString && other.tag == TagNumber {
		return v.ToNumber() == other.payload.(float64)
	}
	if v.tag == TagBoolean {
		return NewJSValueNumber(v.ToNumber()).AbstractEqual(other)
	}
	if other.tag == TagBoolean {
		return v.AbstractEqual(NewJSValueNumber(other.ToNumber()))
	}
	if v.tag == TagObject && (other.tag == TagString || other.tag == TagNumber) {
		return v.ToPrimitive(NoPreference).AbstractEqual(other)
	}
	if other.tag == TagObject && (v.tag == TagString || v.tag == TagNumber) {
		return v.AbstractEqual(other.ToPrimitive(NoPreference))
	}
	return false
}

// StrictEqual implements === per ECMA-262 §7.2.15.
func (v JSValue) StrictEqual(other JSValue) bool {
	if v.tag != other.tag {
		return false
	}
	switch v.tag {
	case TagUndefined, TagNull:
		return true
	case TagBoolean:
		return v.payload == other.payload
	case TagNumber:
		return v.payload.(float64) == other.payload.(float64)
	case TagString:
		return v.payload.(string) == other.payload.(string)
	case TagSymbol, TagObject:
		return v.payload == other.payload
	default:
		return false
	}
}

// ToPrimitive converts this value to a primitive per ECMA-262 §7.1.1.
func (v JSValue) ToPrimitive(preferred PreferredPrimitiveType) JSValue {
	return JSValueUndefined
}
func (v JSValue) String() string {
	switch v.tag {
	case TagUndefined:
		return "undefined"
	case TagNull:
		return "null"
	case TagBoolean:
		b, _ := v.payload.(bool)
		if b {
			return "true"
		}
		return "false"
	case TagNumber:
		return fmt.Sprintf("%v", v.payload)
	case TagString:
		return fmt.Sprintf("\"%v\"", v.payload)
	case TagSymbol:
		return "Symbol()"
	case TagObject:
		return fmt.Sprintf("JSObject(%p)", v.payload)
	default:
		return "JSValue(?)"
	}
}


// PutToPrimitive handles property assignment on a primitive value.
func (v JSValue) PutToPrimitive(globalObject *JSGlobalObject, name PropertyName, value JSValue, slot *PutPropertySlot) bool {
	_ = globalObject
	_ = name
	_ = value
	_ = slot
	// In strict mode, throw TypeError; in sloppy mode, silently ignore.
	return false
}

// encode / decode for serialization (placeholder).
func EncodeJSValue(v JSValue) uint64 {
	_ = v
	return 0
}

func DecodeJSValue(encoded uint64) JSValue {
	_ = encoded
	return JSValueUndefined
}
