// Copyright (C) 2008-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/JumpTable.h

package bytecode

import "fmt"

// UnlinkedStringJumpTableEntry represents a single entry in an unlinked string jump table.
type UnlinkedStringJumpTableEntry struct {
	BranchOffset int32
	IndexInTable uint32
}

// UnlinkedStringJumpTable maps strings to branch offsets for switch statements.
type UnlinkedStringJumpTable struct {
	OffsetTable map[string]UnlinkedStringJumpTableEntry
}

func NewUnlinkedStringJumpTable() UnlinkedStringJumpTable {
	return UnlinkedStringJumpTable{
		OffsetTable: make(map[string]UnlinkedStringJumpTableEntry),
	}
}

// UnlinkedSimpleJumpTable maps integer values to branch offsets for switch statements.
type UnlinkedSimpleJumpTable struct {
	BranchOffsets []int32
	Min           int32
}

func NewUnlinkedSimpleJumpTable(min int32, size int) UnlinkedSimpleJumpTable {
	return UnlinkedSimpleJumpTable{
		BranchOffsets: make([]int32, size),
		Min:           min,
	}
}

// SimpleJumpTable is the linked version of the simple jump table.
// In the interpreter, it uses branch offsets directly (no JIT code labels).
type SimpleJumpTable struct {
	BranchOffsets []int32
	Min           int32
	DefaultOffset int32
}

func NewSimpleJumpTable(min int32, size int) SimpleJumpTable {
	return SimpleJumpTable{
		BranchOffsets: make([]int32, size),
		Min:           min,
	}
}

func (t *SimpleJumpTable) BranchOffsetForValue(value int32) int32 {
	idx := value - t.Min
	if idx >= 0 && int(idx) < len(t.BranchOffsets) {
		return t.BranchOffsets[idx]
	}
	return t.DefaultOffset
}

// StringJumpTable is the linked version of the string jump table.
type StringJumpTable struct {
	OffsetTable map[string]int32
	DefaultOffset int32
	BranchOffsets []int32 // indexed by IndexInTable from unlinked entries
}

func NewStringJumpTable(unlinked UnlinkedStringJumpTable) StringJumpTable {
	t := StringJumpTable{
		OffsetTable:   make(map[string]int32),
		DefaultOffset: 0,
	}
	size := len(unlinked.OffsetTable) + 1
	t.BranchOffsets = make([]int32, size)
	for key, entry := range unlinked.OffsetTable {
		t.OffsetTable[key] = entry.BranchOffset
		t.BranchOffsets[entry.IndexInTable] = entry.BranchOffset
	}
	return t
}

func (t *StringJumpTable) BranchOffsetForValue(value string) int32 {
	if offset, ok := t.OffsetTable[value]; ok {
		return offset
	}
	return t.DefaultOffset
}

func (t *SimpleJumpTable) String() string {
	return fmt.Sprintf("SimpleJumpTable{min=%d, offsets=%v, default=%d}", t.Min, t.BranchOffsets, t.DefaultOffset)
}

func (t *StringJumpTable) String() string {
	return fmt.Sprintf("StringJumpTable{entries=%d, default=%d}", len(t.OffsetTable), t.DefaultOffset)
}
