/*
 * Copyright (C) 2008 Apple Inc. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. ``AS IS'' AND ANY
 * EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED.  IN NO EVENT SHALL APPLE INC. OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
 * EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
 * PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
 * PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
 * OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

// ResultType.h — Type inference result for parse-time expression types

package parser

// ResultType corresponds to JSC::ResultType
type ResultType struct {
	bits uint8
}

// Result type bits
const (
	resultTypeInt32       uint8 = 0x1 << 0
	resultTypeMaybeNumber uint8 = 0x1 << 1
	resultTypeMaybeString uint8 = 0x1 << 2
	resultTypeMaybeBigInt uint8 = 0x1 << 3
	resultTypeMaybeNull   uint8 = 0x1 << 4
	resultTypeMaybeBool   uint8 = 0x1 << 5
	resultTypeMaybeOther  uint8 = 0x1 << 6
	resultTypeBits        uint8 = resultTypeMaybeNumber | resultTypeMaybeString | resultTypeMaybeBigInt | resultTypeMaybeNull | resultTypeMaybeBool | resultTypeMaybeOther
)

const ResultTypeNumBitsNeeded = 7

func NewResultType() ResultType                   { return ResultType{bits: resultTypeBits} }
func resultTypeFromBits(bits uint8) ResultType     { return ResultType{bits: bits} }

func (r ResultType) IsInt32() bool                                    { return r.bits&resultTypeInt32 != 0 }
func (r ResultType) DefinitelyIsNumber() bool                         { return (r.bits & resultTypeBits) == resultTypeMaybeNumber }
func (r ResultType) DefinitelyIsString() bool                         { return (r.bits & resultTypeBits) == resultTypeMaybeString }
func (r ResultType) DefinitelyIsBoolean() bool                        { return (r.bits & resultTypeBits) == resultTypeMaybeBool }
func (r ResultType) DefinitelyIsBigInt() bool                         { return (r.bits & resultTypeBits) == resultTypeMaybeBigInt }
func (r ResultType) DefinitelyIsNull() bool                           { return (r.bits & resultTypeBits) == resultTypeMaybeNull }
func (r ResultType) MightBeUndefinedOrNull() bool                     { return r.bits&(resultTypeMaybeNull|resultTypeMaybeOther) != 0 }
func (r ResultType) MightBeNumber() bool                              { return r.bits&resultTypeMaybeNumber != 0 }
func (r ResultType) IsNotNumber() bool                                { return !r.MightBeNumber() }
func (r ResultType) MightBeBigInt() bool                              { return r.bits&resultTypeMaybeBigInt != 0 }
func (r ResultType) IsNotBigInt() bool                                { return !r.MightBeBigInt() }
func (r ResultType) Bits() uint8                                      { return r.bits }

func ResultTypeNullType() ResultType         { return resultTypeFromBits(resultTypeMaybeNull) }
func ResultTypeBooleanType() ResultType      { return resultTypeFromBits(resultTypeMaybeBool) }
func ResultTypeNumberType() ResultType       { return resultTypeFromBits(resultTypeMaybeNumber) }
func ResultTypeNumberTypeIsInt32() ResultType { return resultTypeFromBits(resultTypeInt32 | resultTypeMaybeNumber) }
func ResultTypeStringOrNumberType() ResultType { return resultTypeFromBits(resultTypeMaybeNumber | resultTypeMaybeString) }
func ResultTypeAddResultType() ResultType     { return resultTypeFromBits(resultTypeMaybeNumber | resultTypeMaybeString | resultTypeMaybeBigInt) }
func ResultTypeStringType() ResultType        { return resultTypeFromBits(resultTypeMaybeString) }
func ResultTypeBigIntType() ResultType        { return resultTypeFromBits(resultTypeMaybeBigInt) }
func ResultTypeBigIntOrInt32Type() ResultType { return resultTypeFromBits(resultTypeMaybeBigInt | resultTypeInt32 | resultTypeMaybeNumber) }
func ResultTypeBigIntOrNumberType() ResultType { return resultTypeFromBits(resultTypeMaybeBigInt | resultTypeMaybeNumber) }
func ResultTypeUnknownType() ResultType       { return resultTypeFromBits(resultTypeBits) }

func ResultTypeForAdd(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return ResultTypeNumberType()
	}
	if op1.DefinitelyIsString() || op2.DefinitelyIsString() {
		return ResultTypeStringType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return ResultTypeBigIntType()
	}
	return ResultTypeAddResultType()
}

func ResultTypeForNonAddArith(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return ResultTypeNumberType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return ResultTypeBigIntType()
	}
	return ResultTypeBigIntOrNumberType()
}

func ResultTypeForUnaryArith(op ResultType) ResultType {
	if op.DefinitelyIsNumber() {
		return ResultTypeNumberType()
	}
	if op.DefinitelyIsBigInt() {
		return ResultTypeBigIntType()
	}
	return ResultTypeBigIntOrNumberType()
}

func ResultTypeForLogicalOp(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsBoolean() && op2.DefinitelyIsBoolean() {
		return ResultTypeBooleanType()
	}
	if op1.DefinitelyIsNumber() && op2.DefinitelyIsNumber() {
		return ResultTypeNumberType()
	}
	if op1.DefinitelyIsString() && op2.DefinitelyIsString() {
		return ResultTypeStringType()
	}
	if op1.DefinitelyIsBigInt() && op2.DefinitelyIsBigInt() {
		return ResultTypeBigIntType()
	}
	return ResultTypeUnknownType()
}

func ResultTypeForCoalesce(op1, op2 ResultType) ResultType {
	if op1.DefinitelyIsNull() {
		return op2
	}
	if !op1.MightBeUndefinedOrNull() {
		return op1
	}
	return ResultTypeUnknownType()
}

func ResultTypeForBitOp() ResultType {
	return ResultTypeBigIntOrInt32Type()
}

// OperandTypes corresponds to JSC::OperandTypes
type OperandTypes struct {
	first  uint8
	second uint8
}

func NewOperandTypes(first, second ResultType) OperandTypes {
	return OperandTypes{first: first.bits, second: second.bits}
}

func (o OperandTypes) First() ResultType  { return resultTypeFromBits(o.first) }
func (o OperandTypes) Second() ResultType { return resultTypeFromBits(o.second) }
