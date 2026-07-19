// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/UnlinkedCodeBlockGenerator.h

package bytecode

// UnlinkedCodeBlockGenerator generates an UnlinkedCodeBlock during compilation.
type UnlinkedCodeBlockGenerator struct {
	codeBlock *UnlinkedCodeBlock
}

func NewUnlinkedCodeBlockGenerator(codeType CodeType, kind CodeSpecializationKind, info ExecutableInfo) *UnlinkedCodeBlockGenerator {
	return &UnlinkedCodeBlockGenerator{
		codeBlock: NewUnlinkedCodeBlock(codeType, kind, info),
	}
}

func (g *UnlinkedCodeBlockGenerator) CodeBlock() *UnlinkedCodeBlock { return g.codeBlock }
func (g *UnlinkedCodeBlockGenerator) SetInstructions(inst *InstructionStream) {
	g.codeBlock.SetInstructions(inst)
}
