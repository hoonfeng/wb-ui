// Copyright (C) 2012-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeBasicBlock.h

package bytecode

// BytecodeBasicBlock represents a basic block in the bytecode control flow graph.
type BytecodeBasicBlock struct {
	leaderBytecodeOffset uint32
	bytecodeOffsets      []uint32
}

func NewBytecodeBasicBlock(leaderOffset uint32) *BytecodeBasicBlock {
	return &BytecodeBasicBlock{
		leaderBytecodeOffset: leaderOffset,
		bytecodeOffsets:      make([]uint32, 0),
	}
}

func (b *BytecodeBasicBlock) LeaderBytecodeOffset() uint32 { return b.leaderBytecodeOffset }
func (b *BytecodeBasicBlock) BytecodeOffsets() []uint32 { return b.bytecodeOffsets }
func (b *BytecodeBasicBlock) AddOffset(offset uint32) {
	b.bytecodeOffsets = append(b.bytecodeOffsets, offset)
}
