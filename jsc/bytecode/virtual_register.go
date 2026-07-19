// Copyright (C) 2011-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/VirtualRegister.h

package bytecode

import "fmt"

// VirtualRegister represents a register number used in bytecode operations.
// Negative indices from the CallFrame pointer are entries in the call frame.
// Forward indices from the CallFrame pointer are local vars and temporaries.
// Positive indices from FirstConstantRegisterIndex specify entries in the constant pool.
type VirtualRegister struct {
	virtualRegister int32
}

const InvalidVirtualRegister int32 = 0x3fffffff

func NewVirtualRegister(vreg int32) VirtualRegister {
	return VirtualRegister{virtualRegister: vreg}
}

func NewVirtualRegisterFromSlot(slot CallFrameSlot) VirtualRegister {
	return VirtualRegister{virtualRegister: int32(slot)}
}

func (v VirtualRegister) IsValid() bool {
	return v.virtualRegister != InvalidVirtualRegister
}

func (v VirtualRegister) IsLocal() bool {
	return virtualRegisterIsLocal(v.virtualRegister)
}

func (v VirtualRegister) IsArgument() bool {
	return virtualRegisterIsArgument(v.virtualRegister)
}

func (v VirtualRegister) IsHeader() bool {
	return v.virtualRegister >= 0 && v.virtualRegister < int32(CallFrameSlotThisArgument)
}

func (v VirtualRegister) IsConstant() bool {
	return v.virtualRegister >= FirstConstantRegisterIndex
}

func (v VirtualRegister) ToLocal() int {
	if !v.IsLocal() {
		panic("VirtualRegister is not local")
	}
	return operandToLocal(v.virtualRegister)
}

func (v VirtualRegister) ToArgument() int {
	if !v.IsArgument() {
		panic("VirtualRegister is not argument")
	}
	return operandToArgument(v.virtualRegister)
}

func (v VirtualRegister) ToConstantIndex() int {
	if !v.IsConstant() {
		panic("VirtualRegister is not constant")
	}
	return int(v.virtualRegister - FirstConstantRegisterIndex)
}

func (v VirtualRegister) Offset() int32 {
	return v.virtualRegister
}

func (v VirtualRegister) OffsetInBytes() int32 {
	return v.virtualRegister * 8 // sizeof(Register) is 8 bytes (JSValue)
}

func (v VirtualRegister) Add(val int) VirtualRegister {
	return VirtualRegister{virtualRegister: v.virtualRegister + int32(val)}
}

func (v VirtualRegister) Sub(val int) VirtualRegister {
	return VirtualRegister{virtualRegister: v.virtualRegister - int32(val)}
}

func (v VirtualRegister) String() string {
	return fmt.Sprintf("VirtualRegister(%d)", v.virtualRegister)
}

// CallFrameSlot represents slots in the call frame header.
type CallFrameSlot int32

const (
	CallFrameSlotCodeBlock      CallFrameSlot = 0
	CallFrameSlotCallee         CallFrameSlot = 1
	CallFrameSlotArgumentCount  CallFrameSlot = 2
	CallFrameSlotThisArgument   CallFrameSlot = 3
	CallFrameSlotFirstArgument  CallFrameSlot = 4
)

// Helper functions
func virtualRegisterIsLocal(operand int32) bool {
	return operand < 0
}

func virtualRegisterIsArgument(operand int32) bool {
	return operand >= 0
}

func localToOperand(local int) int32 {
	return int32(-1 - local)
}

func operandToLocal(operand int32) int {
	return int(-1 - operand)
}

func operandToArgument(operand int32) int {
	return int(operand - int32(CallFrameSlotThisArgument))
}

func argumentToOperand(argument int) int32 {
	return int32(argument) + int32(CallFrameSlotThisArgument)
}

func VirtualRegisterForLocal(local int) VirtualRegister {
	return VirtualRegister{virtualRegister: localToOperand(local)}
}

func VirtualRegisterForArgumentIncludingThis(argument int, offset int) VirtualRegister {
	return VirtualRegister{virtualRegister: argumentToOperand(argument) + int32(offset)}
}
