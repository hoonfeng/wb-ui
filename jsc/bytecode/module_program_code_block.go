// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ModuleProgramCodeBlock.h

package bytecode

// ModuleProgramCodeBlock is a CodeBlock specialized for module code.
type ModuleProgramCodeBlock struct {
	CodeBlock
}

func NewModuleProgramCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *ModuleProgramCodeBlock {
	cb := NewCodeBlock(ModuleCode, kind, owner)
	return &ModuleProgramCodeBlock{CodeBlock: *cb}
}
