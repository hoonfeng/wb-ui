// Copyright (C) 1999-2026 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/interpreter/CallFrame.h

package interpreter

import (
	"wb-ui/jsc/bytecode"
	"wb-ui/jsc/runtime"
)

// CallSiteIndex identifies a call site by bytecode offset.
type CallSiteIndex struct {
	bits uint32
}

const invalidCallSiteIndex uint32 = 0xFFFFFFFF

func NewCallSiteIndex(bytecodeOffset uint32) CallSiteIndex {
	return CallSiteIndex{bits: bytecodeOffset}
}

func CallSiteIndexFromBits(bits uint32) CallSiteIndex {
	return CallSiteIndex{bits: bits}
}

func (c CallSiteIndex) Bits() uint32 { return c.bits }
func (c CallSiteIndex) BytecodeIndex() bytecode.BytecodeIndex {
	return bytecode.NewBytecodeIndex(c.bits, 0)
}
func (c CallSiteIndex) IsValid() bool { return c.bits != invalidCallSiteIndex }

// DisposableCallSiteIndex is a temporary call site index.
type DisposableCallSiteIndex struct {
	CallSiteIndex
}

// CallerFrameAndPC represents the caller frame and return PC on the stack.
type CallerFrameAndPC struct {
	CallerFrame *CallFrame
	ReturnPC    uintptr
}

const SizeInRegisters = 2

// CallFrame represents a JavaScript call frame on the virtual stack.
type CallFrame struct {
	// Call frame header
	thisArgument runtime.JSValue
	argCount     int32
	callee       *runtime.JSCell
	codeBlock    *bytecode.CodeBlock
	callerFrame  *CallFrame
	returnPC     uintptr

	// Execution state
	bytecodeOffset uint32
	scope          *runtime.JSScope

	// Variable storage
	arguments []runtime.JSValue
	locals    []runtime.JSValue
}

func NewCallFrame() *CallFrame {
	return &CallFrame{
		arguments: make([]runtime.JSValue, 0),
		locals:    make([]runtime.JSValue, 0),
	}
}

// Header field accessors
func (cf *CallFrame) GetThisValue() runtime.JSValue          { return cf.thisArgument }
func (cf *CallFrame) SetThisValue(v runtime.JSValue)         { cf.thisArgument = v }
func (cf *CallFrame) GetArgumentCount() int32                { return cf.argCount }
func (cf *CallFrame) SetArgumentCount(n int32)               { cf.argCount = n }
func (cf *CallFrame) GetCallee() *runtime.JSCell             { return cf.callee }
func (cf *CallFrame) SetCallee(c *runtime.JSCell)            { cf.callee = c }
func (cf *CallFrame) GetCodeBlock() *bytecode.CodeBlock      { return cf.codeBlock }
func (cf *CallFrame) SetCodeBlock(cb *bytecode.CodeBlock)   { cf.codeBlock = cb }
func (cf *CallFrame) GetCallerFrame() *CallFrame             { return cf.callerFrame }
func (cf *CallFrame) SetCallerFrame(f *CallFrame)            { cf.callerFrame = f }
func (cf *CallFrame) GetReturnPC() uintptr                   { return cf.returnPC }
func (cf *CallFrame) SetReturnPC(pc uintptr)                 { cf.returnPC = pc }

// Execution state
func (cf *CallFrame) GetScope() *runtime.JSScope             { return cf.scope }
func (cf *CallFrame) SetScope(s *runtime.JSScope)            { cf.scope = s }
func (cf *CallFrame) GetBytecodeOffset() uint32              { return cf.bytecodeOffset }
func (cf *CallFrame) SetBytecodeOffset(offset uint32)        { cf.bytecodeOffset = offset }

// Argument access
func (cf *CallFrame) Argument(index int32) runtime.JSValue {
	if index >= 0 && int(index) < len(cf.arguments) {
		return cf.arguments[index]
	}
	return runtime.JSValueUndefined
}

func (cf *CallFrame) SetArgument(index int32, val runtime.JSValue) {
	i := int(index)
	if i >= len(cf.arguments) {
		newArgs := make([]runtime.JSValue, i+1)
		copy(newArgs, cf.arguments)
		cf.arguments = newArgs
	}
	cf.arguments[i] = val
}

func (cf *CallFrame) NumArguments() int {
	if len(cf.arguments) > 0 {
		return len(cf.arguments) - 1 // excluding this
	}
	return 0
}

func (cf *CallFrame) ArgumentCountIncludingThis() int32 {
	return cf.argCount
}

// Local variable access
func (cf *CallFrame) Local(index int32) runtime.JSValue {
	if index >= 0 && int(index) < len(cf.locals) {
		return cf.locals[index]
	}
	return runtime.JSValueUndefined
}

func (cf *CallFrame) SetLocal(index int32, val runtime.JSValue) {
	i := int(index)
	if i >= len(cf.locals) {
		newLocals := make([]runtime.JSValue, i+1)
		copy(newLocals, cf.locals)
		cf.locals = newLocals
	}
	cf.locals[i] = val
}

func (cf *CallFrame) NumLocals() int { return len(cf.locals) }

// CalleeAsObject returns the callee as a JSObject.
func (cf *CallFrame) CalleeAsObject() *runtime.JSObject {
	if cell := cf.callee; cell != nil {
		if obj := runtime.JSObjectFromCell(cell); obj != nil {
			return obj
		}
	}
	return nil
}
