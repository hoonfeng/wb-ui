// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/EvalCodeBlock.h

package bytecode

// EvalCodeBlock is a CodeBlock specialized for eval code.
type EvalCodeBlock struct {
	CodeBlock
}

func NewEvalCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *EvalCodeBlock {
	cb := NewCodeBlock(EvalCode, kind, owner)
	return &EvalCodeBlock{CodeBlock: *cb}
}
