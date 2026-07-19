// Copyright (C) 2019-2025 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeIndex.h

package bytecode

import (
	"fmt"
	"math/bits"
)

// Checkpoint represents a checkpoint index within a bytecode instruction.
type Checkpoint uint8

const NoCheckpoints Checkpoint = 0

// BytecodeIndex represents the position of a bytecode instruction,
// packing both the bytecode offset and checkpoint into a single uint32.
type BytecodeIndex struct {
	packedBits uint32
}

const (
	numberOfCheckpoints = 4
	checkpointMask      = numberOfCheckpoints - 1
	// checkpointShift = log2(4) = 2
	checkpointShift = 2
	invalidOffset   = 0xFFFFFFFF
)

func NewBytecodeIndex(bytecodeOffset uint32, checkpoint Checkpoint) BytecodeIndex {
	Assert(checkpoint < numberOfCheckpoints, "checkpoint out of range")
	packed := pack(bytecodeOffset, checkpoint)
	return BytecodeIndex{packedBits: packed}
}

func (b BytecodeIndex) Offset() uint32 {
	return b.packedBits >> checkpointShift
}

func (b BytecodeIndex) Checkpoint() Checkpoint {
	return Checkpoint(b.packedBits & checkpointMask)
}

func (b BytecodeIndex) AsBits() uint32 {
	return b.packedBits
}

func (b BytecodeIndex) Hash() uint32 {
	return intHash(b.packedBits)
}

func BytecodeIndexDeletedValue() BytecodeIndex {
	return BytecodeIndex{packedBits: invalidOffset - 1}
}

func (b BytecodeIndex) IsHashTableDeletedValue() bool {
	return b == BytecodeIndexDeletedValue()
}

func (b BytecodeIndex) WithCheckpoint(checkpoint Checkpoint) BytecodeIndex {
	return NewBytecodeIndex(b.Offset(), checkpoint)
}

func (b BytecodeIndex) IsValid() bool {
	return b.packedBits != invalidOffset && b.packedBits != BytecodeIndexDeletedValue().packedBits
}

func (b BytecodeIndex) String() string {
	return fmt.Sprintf("BytecodeIndex(%d, checkpoint=%d)", b.Offset(), b.Checkpoint())
}

// Comparison
func (b BytecodeIndex) Less(other BytecodeIndex) bool {
	return b.packedBits < other.packedBits
}

func (b BytecodeIndex) Equal(other BytecodeIndex) bool {
	return b.packedBits == other.packedBits
}

func pack(bytecodeOffset uint32, checkpoint Checkpoint) uint32 {
	Assert(uint32(checkpoint) < numberOfCheckpoints, "checkpoint out of range")
	Assert((bytecodeOffset<<checkpointShift)>>checkpointShift == bytecodeOffset, "bytecodeOffset overflow")
	return bytecodeOffset<<checkpointShift | uint32(checkpoint)
}

func intHash(key uint32) uint32 {
	key = ^key + (key << 15)
	key = key ^ (key >> 12)
	key = key + (key << 2)
	key = key ^ (key >> 4)
	key = key * 2057
	key = key ^ (key >> 16)
	return key
}

// Assert is a simple assertion helper.
func Assert(cond bool, msg string) {
	if !cond {
		panic("bytecode: assertion failed: " + msg)
	}
}

// getMSBSet returns the index of the most significant set bit (0-based).
// Returns 0 if x is 0.
func getMSBSet(x uint32) uint32 {
	if x == 0 {
		return 0
	}
	return uint32(bits.Len32(x) - 1)
}
