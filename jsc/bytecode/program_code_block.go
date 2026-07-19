// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ProgramCodeBlock.h

package bytecode

// ProgramCodeBlock is a CodeBlock specialized for top-level program code.
type ProgramCodeBlock struct {
	CodeBlock
}

func NewProgramCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *ProgramCodeBlock {
	cb := NewCodeBlock(GlobalCode, kind, owner)
	return &ProgramCodeBlock{CodeBlock: *cb}
}
