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

// BytecodeGraph represents the control flow graph of bytecode basic blocks.
type BytecodeGraph struct {
	blocks []*BytecodeBasicBlock
}

func NewBytecodeGraph() *BytecodeGraph {
	return &BytecodeGraph{blocks: make([]*BytecodeBasicBlock, 0)}
}

func (g *BytecodeGraph) Blocks() []*BytecodeBasicBlock { return g.blocks }
func (g *BytecodeGraph) AddBlock(b *BytecodeBasicBlock) { g.blocks = append(g.blocks, b) }
func (g *BytecodeGraph) Size() int { return len(g.blocks) }
