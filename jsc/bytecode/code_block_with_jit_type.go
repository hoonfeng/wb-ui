// Copyright (C) 2013 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeBlockWithJITType.h

package bytecode

// CodeBlockWithJITType pairs a CodeBlock with its JIT type.
// In the interpreter, the JIT type is always NotJIT.
type JITType uint8

const (
	JITTypeNone     JITType = 0
	JITTypeHostCall  JITType = 1
	JITTypeInterpreter JITType = 2
	JITTypeBaselineJIT  JITType = 3
	JITTypeDFGJIT    JITType = 4
	JITTypeFTLJIT    JITType = 5
)

type CodeBlockWithJITType struct {
	CodeBlock *CodeBlock
	JITType   JITType
}

func NewCodeBlockWithJITType(cb *CodeBlock) CodeBlockWithJITType {
	return CodeBlockWithJITType{CodeBlock: cb, JITType: JITTypeInterpreter}
}
