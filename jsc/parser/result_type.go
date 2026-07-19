// 版权所有 (C) 2008 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ResultType.h 翻译为 Go

package parser

// ResultType 表示表达式结果类型，用于类型推断优化
type ResultType struct {
	m_bits uint8
}

// 位掩码常量
const (
	resultTypeInt32       uint8 = 0x1 << 0
	resultTypeMaybeNumber uint8 = 0x1 << 1
	resultTypeMaybeString uint8 = 0x1 << 2
	resultTypeMaybeBigInt uint8 = 0x1 << 3
	resultTypeMaybeNull   uint8 = 0x1 << 4
	resultTypeMaybeBool   uint8 = 0x1 << 5
	resultTypeMaybeOther  uint8 = 0x1 << 6
)

const resultTypeBits uint8 = resultTypeMaybeNumber | resultTypeMaybeString | resultTypeMaybeBigInt | resultTypeMaybeNull | resultTypeMaybeBool | resultTypeMaybeOther

// NewResultType 创建指定类型的 ResultType
func NewResultType(typ uint8) ResultType {
	return ResultType{m_bits: typ}
}

// NewResultTypeFromBits 从位掩码创建 ResultType
func NewResultTypeFromBits(bits uint8) ResultType {
	return ResultType{m_bits: bits}
}

// Bits 返回原始位掩码
func (r ResultType) Bits() uint8 {
	return r.m_bits
}

// IsInt32 判断是否是 Int32 类型
func (r ResultType) IsInt32() bool {
	return r.m_bits&resultTypeInt32 != 0
}

// DefinitelyIsNumber 判断是否确定是数值类型
func (r ResultType) DefinitelyIsNumber() bool {
	return (r.m_bits & resultTypeBits) == resultTypeMaybeNumber
}

// DefinitelyIsString 判断是否确定是字符串类型
func (r ResultType) DefinitelyIsString() bool {
	return (r.m_bits & resultTypeBits) == resultTypeMaybeString
}

// DefinitelyIsBoolean 判断是否确定是布尔类型
func (r ResultType) DefinitelyIsBoolean() bool {
	return (r.m_bits & resultTypeBits) == resultTypeMaybeBool
}

// DefinitelyIsBigInt 判断是否确定是 BigInt 类型
func (r ResultType) DefinitelyIsBigInt() bool {
	return (r.m_bits & resultTypeBits) == resultTypeMaybeBigInt
}

// DefinitelyIsNull 判断是否确定是 null 类型
func (r ResultType) DefinitelyIsNull() bool {
	return (r.m_bits & resultTypeBits) == resultTypeMaybeNull
}

// MightBeUndefinedOrNull 判断可能是 undefined 或 null
func (r ResultType) MightBeUndefinedOrNull() bool {
	return r.m_bits&(resultTypeMaybeNull|resultTypeMaybeOther) != 0
}

// MightBeNumber 判断可能是数值类型
func (r ResultType) MightBeNumber() bool {
	return r.m_bits&resultTypeMaybeNumber != 0
}

// IsNotNumber 判断不是数值类型
func (r ResultType) IsNotNumber() bool {
	return !r.MightBeNumber()
}

// MightBeBigInt 判断可能是 BigInt 类型
func (r ResultType) MightBeBigInt() bool {
	return r.m_bits&resultTypeMaybeBigInt != 0
}

// IsNotBigInt 判断不是 BigInt 类型
func (r ResultType) IsNotBigInt() bool {
	return !r.MightBeBigInt()
}

// IsKnown 判断是否是已知类型
func (r ResultType) IsKnown() bool {
	return r.m_bits != 0
}

// NullType 返回 null 结果类型
func NullType() ResultType {
	return NewResultType(resultTypeMaybeNull)
}

// BooleanType 返回布尔结果类型
func BooleanType() ResultType {
	return NewResultType(resultTypeMaybeBool)
}

// NumberType 返回数值结果类型
func NumberType() ResultType {
	return NewResultType(resultTypeMaybeNumber)
}

// NumberTypeIsInt32 返回 Int32 数值结果类型
func NumberTypeIsInt32() ResultType {
	return NewResultType(resultTypeInt32 | resultTypeMaybeNumber)
}

// StringOrNumberType 返回字符串或数值结果类型
func StringOrNumberType() ResultType {
	return NewResultType(resultTypeMaybeNumber | resultTypeMaybeString)
}

// AddResultType 返回加法结果类型
func AddResultType() ResultType {
	return NewResultType(resultTypeMaybeNumber | resultTypeMaybeString | resultTypeMaybeBigInt)
}

// StringType 返回字符串结果类型
func StringType() ResultType {
	return NewResultType(resultTypeMaybeString)
}

// BigIntType 返回 BigInt 结果类型
func BigIntType() ResultType {
	return NewResultType(resultTypeMaybeBigInt)
}

// BigIntOrInt32Type 返回 BigInt 或 Int32 结果类型
func BigIntOrInt32Type() ResultType {
	return NewResultType(resultTypeMaybeBigInt | resultTypeInt32 | resultTypeMaybeNumber)
}

// BigIntOrNumberType 返回 BigInt 或数值结果类型
func BigIntOrNumberType() ResultType {
	return NewResultType(resultTypeMaybeBigInt | resultTypeMaybeNumber)
}

// UnknownType 返回未知结果类型
func UnknownType() ResultType {
	return NewResultType(resultTypeBits)
}

// ForAdd 根据两个操作数推断加法结果类型
func ForAdd(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return NumberType()
	}
	if op1.DefinitelyIsString() || op2.DefinitelyIsString() {
		return StringType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return BigIntType()
	}
	return AddResultType()
}

// ForNonAddArith 根据两个操作数推断非加法算术结果类型
func ForNonAddArith(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return NumberType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return BigIntType()
	}
	return BigIntOrNumberType()
}

// ForUnaryArith 根据操作数推断一元算术结果类型
func ForUnaryArith(op ResultType) ResultType {
	if op.DefinitelyIsNumber() {
		return NumberType()
	}
	if op.DefinitelyIsBigInt() {
		return BigIntType()
	}
	return BigIntOrNumberType()
}

// ForLogicalOp 根据两个操作数推断逻辑运算结果类型
func ForLogicalOp(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsBoolean() && op2.DefinitelyIsBoolean() {
		return BooleanType()
	}
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return NumberType()
	}
	if op1.DefinitelyIsString() && op2.DefinitelyIsString() {
		return StringType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return BigIntType()
	}
	return UnknownType()
}

// ForCoalesce 推断空值合并运算结果类型
func ForCoalesce(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNull() {
		return op2
	}
	if !op1.MightBeUndefinedOrNull() {
		return op1
	}
	return UnknownType()
}

// ForBitOp 返回位运算结果类型
func ForBitOp() ResultType {
	return BigIntOrInt32Type()
}

// NumBitsNeeded 表示 ResultType 需要的位数
const NumBitsNeeded = 7

// OperandTypes 表示两个操作数的类型
type OperandTypes struct {
	m_first  uint8
	m_second uint8
}

// NewOperandTypes 创建 OperandTypes
func NewOperandTypes(first, second ResultType) OperandTypes {
	return OperandTypes{m_first: first.m_bits, m_second: second.m_bits}
}

// First 返回第一个操作数的类型
func (o OperandTypes) First() ResultType {
	return NewResultType(o.m_first)
}

// Second 返回第二个操作数的类型
func (o OperandTypes) Second() ResultType {
	return NewResultType(o.m_second)
}

// Bits 返回紧凑的位表示
func (o OperandTypes) Bits() uint16 {
	return uint16(o.m_first) | (uint16(o.m_second) << 8)
}

// OperandTypesFromBits 从紧凑的位表示创建 OperandTypes
func OperandTypesFromBits(bits uint16) OperandTypes {
	return OperandTypes{
		m_first:  uint8(bits & 0xFF),
		m_second: uint8((bits >> 8) & 0xFF),
	}
}
