// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/FunctionCodeBlock.h

package bytecode

// FunctionCodeBlock is a CodeBlock specialized for function code.
type FunctionCodeBlock struct {
	CodeBlock
}

func NewFunctionCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *FunctionCodeBlock {
	cb := NewCodeBlock(FunctionCode, kind, owner)
	return &FunctionCodeBlock{CodeBlock: *cb}
}
