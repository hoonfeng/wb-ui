// Copyright (C) 2008-2022 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/interpreter/CLoopStack.h

package interpreter

import (
	"wb-ui/jsc/runtime"
)

// CLoopStack implements the C Loop stack for the interpreter.
// This is the stack used by the pure C interpreter (CLoop) in WebKit.
type CLoopStack struct {
	vm      *runtime.VM
	stack   []runtime.JSValue
	maxSize int
}

func NewCLoopStack(vm *runtime.VM) *CLoopStack {
	return &CLoopStack{
		vm:      vm,
		stack:   make([]runtime.JSValue, 0, 1024),
		maxSize: 1024 * 1024, // 1M entries
	}
}

func (s *CLoopStack) Size() int     { return len(s.stack) }
func (s *CLoopStack) Capacity() int { return cap(s.stack) }

func (s *CLoopStack) Push(val runtime.JSValue) {
	s.stack = append(s.stack, val)
}

func (s *CLoopStack) Pop() runtime.JSValue {
	if len(s.stack) == 0 {
		return runtime.JSValueUndefined
	}
	val := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return val
}

func (s *CLoopStack) Top() runtime.JSValue {
	if len(s.stack) == 0 {
		return runtime.JSValueUndefined
	}
	return s.stack[len(s.stack)-1]
}

func (s *CLoopStack) Get(index int) runtime.JSValue {
	if index >= 0 && index < len(s.stack) {
		return s.stack[index]
	}
	return runtime.JSValueUndefined
}

func (s *CLoopStack) Set(index int, val runtime.JSValue) {
	if index >= 0 && index < len(s.stack) {
		s.stack[index] = val
	}
}

func (s *CLoopStack) EnsureCapacity(n int) bool {
	if n > cap(s.stack) {
		newStack := make([]runtime.JSValue, len(s.stack), n+1024)
		copy(newStack, s.stack)
		s.stack = newStack
	}
	return true
}

// EntryFrame represents the boundary between JS and C++ code.
type EntryFrame struct {
	previousEntryFrame *EntryFrame
}

func NewEntryFrame() *EntryFrame {
	return &EntryFrame{}
}

// VMEntryRecord stores state when entering the VM from C++.
type VMEntryRecord struct {
	prevTopCallFrame      *CallFrame
	prevTopEntryFrame     *EntryFrame
	prevLastStackTop      uintptr
}
