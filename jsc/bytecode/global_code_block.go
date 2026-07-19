// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/GlobalCodeBlock.h

package bytecode

// GlobalCodeBlock is a CodeBlock specialized for global/eval code.
type GlobalCodeBlock struct {
	CodeBlock
}

func NewGlobalCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *GlobalCodeBlock {
	cb := NewCodeBlock(GlobalCode, kind, owner)
	return &GlobalCodeBlock{CodeBlock: *cb}
}
