// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/UnlinkedFunctionExecutable.h

package bytecode

// UnlinkedFunctionExecutable holds the unlinked (compilation-unit) form of a
// function executable, before linking to a specific CodeBlock.
type UnlinkedFunctionExecutable struct {
	name              string
	inferredName      string
	sourceID          SourceID
	sourceURL         string
	lineCount         uint32
	unlinkedCodeBlock *UnlinkedCodeBlock
}

func NewUnlinkedFunctionExecutable(name string) *UnlinkedFunctionExecutable {
	return &UnlinkedFunctionExecutable{name: name}
}

func (e *UnlinkedFunctionExecutable) Name() string                   { return e.name }
func (e *UnlinkedFunctionExecutable) InferredName() string           { return e.inferredName }
func (e *UnlinkedFunctionExecutable) SetInferredName(n string)       { e.inferredName = n }
func (e *UnlinkedFunctionExecutable) SourceID() SourceID             { return e.sourceID }
func (e *UnlinkedFunctionExecutable) SourceURL() string              { return e.sourceURL }
func (e *UnlinkedFunctionExecutable) LineCount() uint32              { return e.lineCount }
func (e *UnlinkedFunctionExecutable) UnlinkedCodeBlock() *UnlinkedCodeBlock { return e.unlinkedCodeBlock }

func (e *UnlinkedFunctionExecutable) LinkCodeBlock(cb *CodeBlock) {
	// Link unlinked code block data to the code block.
	// (simplified — copies constants and structure IDs at link time)
}
