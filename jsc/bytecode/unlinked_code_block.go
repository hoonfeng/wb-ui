// Copyright (C) 2012-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/UnlinkedCodeBlock.h
// and UnlinkedCodeBlockGenerator.h

package bytecode

import "wb-ui/jsc/runtime"

// UnlinkedCodeBlock is a CodeBlock before linking (before constant values
// and structure pointers are resolved).
type UnlinkedCodeBlock struct {
	// Bytecode
	instructions  *InstructionStream
	
	// Metadata
	metadata      *UnlinkedMetadataTable
	
	// Constants (unlinked - still as raw JS values)
	constants     []runtime.JSValue
	
	// Counts
	numParameters      int
	numVars            int
	numCalleeLocals    int
	numCapturedVars    int
	
	// Handlers
	exceptionHandlers []UnlinkedHandlerInfo
	
	// Jump tables
	switchJumpTables        map[BytecodeIndex]*UnlinkedSimpleJumpTable
	stringSwitchJumpTables  map[BytecodeIndex]*UnlinkedStringJumpTable
	
	// Executable info
	executableInfo  ExecutableInfo
	
	// Code type
	codeType        CodeType
	codeKind        CodeSpecializationKind
	
	// Source
	sourceID        SourceID
	sourceURL       string
	
	// Features
	features        uint32 // DFG::CapabilityLevel bitfield
	
	// Hashes
	hash            uint32
}

func NewUnlinkedCodeBlock(codeType CodeType, kind CodeSpecializationKind, info ExecutableInfo) *UnlinkedCodeBlock {
	return &UnlinkedCodeBlock{
		codeType:            codeType,
		codeKind:            kind,
		executableInfo:      info,
		exceptionHandlers:   make([]UnlinkedHandlerInfo, 0),
		switchJumpTables:    make(map[BytecodeIndex]*UnlinkedSimpleJumpTable),
		stringSwitchJumpTables: make(map[BytecodeIndex]*UnlinkedStringJumpTable),
		features:            0,
	}
}

func (ucb *UnlinkedCodeBlock) CodeType() CodeType { return ucb.codeType }
func (ucb *UnlinkedCodeBlock) CodeKind() CodeSpecializationKind { return ucb.codeKind }
func (ucb *UnlinkedCodeBlock) ExecutableInfo() *ExecutableInfo { return &ucb.executableInfo }

func (ucb *UnlinkedCodeBlock) Instructions() *InstructionStream { return ucb.instructions }
func (ucb *UnlinkedCodeBlock) SetInstructions(inst *InstructionStream) { ucb.instructions = inst }

func (ucb *UnlinkedCodeBlock) NumberOfParameters() int { return ucb.numParameters }
func (ucb *UnlinkedCodeBlock) SetNumParameters(n int) { ucb.numParameters = n }

func (ucb *UnlinkedCodeBlock) NumberOfVars() int { return ucb.numVars }
func (ucb *UnlinkedCodeBlock) SetNumVars(n int) { ucb.numVars = n }

func (ucb *UnlinkedCodeBlock) NumberOfCalleeLocals() int { return ucb.numCalleeLocals }
func (ucb *UnlinkedCodeBlock) SetNumCalleeLocals(n int) { ucb.numCalleeLocals = n }

func (ucb *UnlinkedCodeBlock) ConstantCount() int { return len(ucb.constants) }
func (ucb *UnlinkedCodeBlock) Constants() []runtime.JSValue { return ucb.constants }

func (ucb *UnlinkedCodeBlock) AddConstant(val runtime.JSValue) int {
	idx := len(ucb.constants)
	ucb.constants = append(ucb.constants, val)
	return idx
}

func (ucb *UnlinkedCodeBlock) AddExceptionHandler(h UnlinkedHandlerInfo) {
	ucb.exceptionHandlers = append(ucb.exceptionHandlers, h)
}

func (ucb *UnlinkedCodeBlock) ExceptionHandlers() []UnlinkedHandlerInfo {
	return ucb.exceptionHandlers
}

func (ucb *UnlinkedCodeBlock) Metadata() *UnlinkedMetadataTable { return ucb.metadata }
func (ucb *UnlinkedCodeBlock) SetMetadata(m *UnlinkedMetadataTable) { ucb.metadata = m }

func (ucb *UnlinkedCodeBlock) FeatureBits() uint32 { return ucb.features }
func (ucb *UnlinkedCodeBlock) SetFeatureBits(bits uint32) { ucb.features = bits }

func (ucb *UnlinkedCodeBlock) SourceID() SourceID { return ucb.sourceID }
func (ucb *UnlinkedCodeBlock) SetSourceID(id SourceID) { ucb.sourceID = id }

func (ucb *UnlinkedCodeBlock) SourceURL() string { return ucb.sourceURL }
func (ucb *UnlinkedCodeBlock) SetSourceURL(url string) { ucb.sourceURL = url }

func (ucb *UnlinkedCodeBlock) Hash() uint32 { return ucb.hash }
