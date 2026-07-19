// Copyright (C) 2008-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/interpreter/Interpreter.h

package interpreter

import (
	"wb-ui/jsc/bytecode"
	"wb-ui/jsc/runtime"
)

// Interpreter is the bytecode interpreter for JSC.
// It executes JavaScript bytecode by dispatching opcodes.
type Interpreter struct {
	vm      *runtime.VM
	stack   *CLoopStack
}

func NewInterpreter(vm *runtime.VM) *Interpreter {
	return &Interpreter{
		vm:    vm,
		stack: NewCLoopStack(vm),
	}
}

func (interp *Interpreter) VM() *runtime.VM { return interp.vm }

// ExecuteProgram executes a program CodeBlock.
func (interp *Interpreter) ExecuteProgram(exec *runtime.ExecState, program *bytecode.CodeBlock) (runtime.JSValue, error) {
	// Create call frame
	callFrame := NewCallFrame()
	callFrame.SetCodeBlock(program)
	callFrame.SetBytecodeOffset(0)

	// Execute
	return interp.execute(callFrame)
}

// ExecuteCall executes a function call.
func (interp *Interpreter) ExecuteCall(callFrame *CallFrame, thisVal runtime.JSValue, args []runtime.JSValue) (runtime.JSValue, error) {
	callFrame.SetThisValue(thisVal)
	callFrame.SetArgumentCount(int32(len(args) + 1)) // including this
	for i, arg := range args {
		callFrame.SetArgument(int32(i+1), arg)
	}
	return interp.execute(callFrame)
}

// execute is the main interpreter loop.
func (interp *Interpreter) execute(callFrame *CallFrame) (runtime.JSValue, error) {
	codeBlock := callFrame.GetCodeBlock()
	if codeBlock == nil {
		return runtime.JSValueUndefined, nil
	}

	instructions := codeBlock.Instructions()
	if instructions == nil {
		return runtime.JSValueUndefined, nil
	}

	offset := callFrame.GetBytecodeOffset()

	for offset < uint32(instructions.Size()) {
		inst, size := bytecode.DecodeInstruction(instructions.RawPointer(), offset)

		switch inst.OpcodeID() {
		case bytecode.OpRet:
			// Return the value in the return register
			retReg := inst.ReadOperand(0)
			val := callFrame.Local(retReg)
			return val, nil

		case bytecode.OpNop:
			// No-op

		case bytecode.OpJmp:
			target := inst.ReadOperand(0)
			offset = uint32(target)
			continue

		case bytecode.OpMov:
			dst := inst.ReadOperand(0)
			src := inst.ReadOperand(1)
			callFrame.SetLocal(dst, callFrame.Local(src))

		default:
			// Unknown/incomplete opcode - skip for now
		}

		offset += size
	}

	return runtime.JSValueUndefined, nil
}

// Stack snaphot/debug helpers
func (interp *Interpreter) GetStackTrace() []runtime.JSValue {
	// Simplified - returns empty
	return nil
}
