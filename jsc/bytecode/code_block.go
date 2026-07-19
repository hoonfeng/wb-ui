// Copyright (C) 2008-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeBlock.h

package bytecode

import (
	"fmt"
)

// CodeBlockOwner is an interface for objects that own a CodeBlock.
type CodeBlockOwner interface {
	CodeBlock() *CodeBlock
}

// CodeBlock represents a compiled JavaScript code block.
// This is the core executable unit in JSC's bytecode interpreter.
type CodeBlock struct {
	// Identification
	mode          CodeType
	kind          CodeSpecializationKind
	hash          uint32
	
	// Bytecode
	instructions  *InstructionStream
	bytecodeOffset uint32
	
	// Metadata
	metadata      *MetadataTable
	
	// Constants
	constants     []uint64 // encoded JSValue
	
	// Owner (ScriptExecutable or similar)
	owner         CodeBlockOwner
	
	// Number of locals, parameters, temporaries
	numParameters  int
	numVars        int
	numCalleeLocals int
	numCapturedVars int
	
	// Handler table
	exceptionHandlers []HandlerInfo
	
	// Jump tables
	switchJumpTables   map[BytecodeIndex]*SimpleJumpTable
	stringSwitchJumpTables map[BytecodeIndex]*StringJumpTable
	
	// Name
	inferredName   string
	sourceID       SourceID
	sourceURL      string
	hashIsCalculated bool
	
	// Flags
	isConstructor bool
	isBuiltin     bool
}

func NewCodeBlock(mode CodeType, kind CodeSpecializationKind, owner CodeBlockOwner) *CodeBlock {
	return &CodeBlock{
		mode:           mode,
		kind:           kind,
		owner:          owner,
		exceptionHandlers: make([]HandlerInfo, 0),
		switchJumpTables: make(map[BytecodeIndex]*SimpleJumpTable),
		stringSwitchJumpTables: make(map[BytecodeIndex]*StringJumpTable),
	}
}

func (cb *CodeBlock) Mode() CodeType { return cb.mode }
func (cb *CodeBlock) Kind() CodeSpecializationKind { return cb.kind }
func (cb *CodeBlock) Hash() uint32 { return cb.hash }

func (cb *CodeBlock) Instructions() *InstructionStream { return cb.instructions }
func (cb *CodeBlock) SetInstructions(inst *InstructionStream) { cb.instructions = inst }

func (cb *CodeBlock) NumberOfParameters() int { return cb.numParameters }
func (cb *CodeBlock) SetNumParameters(n int) { cb.numParameters = n }

func (cb *CodeBlock) NumberOfLocals() int { return cb.numVars }
func (cb *CodeBlock) SetNumVars(n int) { cb.numVars = n }

func (cb *CodeBlock) NumberOfCalleeLocals() int { return cb.numCalleeLocals }
func (cb *CodeBlock) SetNumCalleeLocals(n int) { cb.numCalleeLocals = n }

func (cb *CodeBlock) NumberOfCapturedVars() int { return cb.numCapturedVars }

func (cb *CodeBlock) ConstantCount() int { return len(cb.constants) }

func (cb *CodeBlock) ConstantAt(index int) uint64 {
	if index >= 0 && index < len(cb.constants) {
		return cb.constants[index]
	}
	return 0 // JSUndefined encoded
}

func (cb *CodeBlock) SetConstant(index int, val uint64) {
	if index >= len(cb.constants) {
		newConsts := make([]uint64, index+1)
		copy(newConsts, cb.constants)
		cb.constants = newConsts
	}
	cb.constants[index] = val
}

func (cb *CodeBlock) Constants() []uint64 { return cb.constants }

func (cb *CodeBlock) Owner() CodeBlockOwner { return cb.owner }

func (cb *CodeBlock) ExceptionHandlers() []HandlerInfo { return cb.exceptionHandlers }
func (cb *CodeBlock) AddExceptionHandler(h HandlerInfo) {
	cb.exceptionHandlers = append(cb.exceptionHandlers, h)
}

func (cb *CodeBlock) InferredName() string { return cb.inferredName }
func (cb *CodeBlock) SetInferredName(name string) { cb.inferredName = name }

func (cb *CodeBlock) SourceID() SourceID { return cb.sourceID }
func (cb *CodeBlock) SetSourceID(id SourceID) { cb.sourceID = id }

func (cb *CodeBlock) SourceURL() string { return cb.sourceURL }
func (cb *CodeBlock) SetSourceURL(url string) { cb.sourceURL = url }

func (cb *CodeBlock) Metadata() *MetadataTable { return cb.metadata }
func (cb *CodeBlock) SetMetadata(m *MetadataTable) { cb.metadata = m }

func (cb *CodeBlock) IsConstructor() bool { return cb.isConstructor }
func (cb *CodeBlock) SetIsConstructor(v bool) { cb.isConstructor = v }

func (cb *CodeBlock) IsBuiltin() bool { return cb.isBuiltin }
func (cb *CodeBlock) SetIsBuiltin(v bool) { cb.isBuiltin = v }

func (cb *CodeBlock) BytecodeOffset() uint32 { return cb.bytecodeOffset }

func (cb *CodeBlock) String() string {
	return fmt.Sprintf("CodeBlock{%s %s hash=%d params=%d vars=%d}",
		cb.mode.String(), cb.kind.String(), cb.hash, cb.numParameters, cb.numVars)
}

// CodeBlockHash computes a hash for the code block.
type CodeBlockHash struct {
	hash uint32
}

func NewCodeBlockHash(h uint32) CodeBlockHash {
	return CodeBlockHash{hash: h}
}

func (h CodeBlockHash) Hash() uint32 { return h.hash }
func (h CodeBlockHash) String() string { return fmt.Sprintf("%08x", h.hash) }
