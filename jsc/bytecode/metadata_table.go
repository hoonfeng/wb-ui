// Copyright (C) 2018-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/MetadataTable.h

package bytecode

// MetadataTable holds metadata for bytecode instructions (profiling data, etc.).
// In the interpreter, this is a simplified version.
//
// Memory layout (C++): [ValueProfile][LinkingData][OffsetTable][MetadataContent]
// Go simplification: use slices for metadata entries.
type MetadataTable struct {
	entries []uint64
}

func NewMetadataTable() *MetadataTable {
	return &MetadataTable{
		entries: make([]uint64, 0, 64),
	}
}

func (m *MetadataTable) Size() int {
	return len(m.entries)
}

func (m *MetadataTable) Get(index int) uint64 {
	if index >= 0 && index < len(m.entries) {
		return m.entries[index]
	}
	return 0
}

func (m *MetadataTable) Set(index int, val uint64) {
	if index >= len(m.entries) {
		newEntries := make([]uint64, index+1)
		copy(newEntries, m.entries)
		m.entries = newEntries
	}
	m.entries[index] = val
}

func (m *MetadataTable) Append(val uint64) int {
	idx := len(m.entries)
	m.entries = append(m.entries, val)
	return idx
}

// UnlinkedMetadataTable is the unlinked version of MetadataTable.
type UnlinkedMetadataTable struct {
	is32Bit        bool
	numValueProfiles uint32
	lastOffset     uint32
	entries        []uint64
}

func NewUnlinkedMetadataTable() *UnlinkedMetadataTable {
	return &UnlinkedMetadataTable{
		entries: make([]uint64, 0, 64),
	}
}

func NewUnlinkedMetadataTableFull(is32Bit bool, numValueProfiles uint32, lastOffset uint32) *UnlinkedMetadataTable {
	return &UnlinkedMetadataTable{
		is32Bit:        is32Bit,
		numValueProfiles: numValueProfiles,
		lastOffset:     lastOffset,
		entries:        make([]uint64, 0, int(lastOffset)),
	}
}

func (m *UnlinkedMetadataTable) NumEntries() int {
	return len(m.entries)
}

func (m *UnlinkedMetadataTable) AddEntry(opcodeID OpcodeID) uint32 {
	idx := uint32(len(m.entries))
	m.entries = append(m.entries, uint64(opcodeID))
	return idx
}

func (m *UnlinkedMetadataTable) AddValueProfile() uint32 {
	idx := m.numValueProfiles
	m.numValueProfiles++
	return idx
}

func (m *UnlinkedMetadataTable) Link() *MetadataTable {
	t := NewMetadataTable()
	for _, v := range m.entries {
		t.Append(v)
	}
	return t
}

// Is32Bit returns whether the metadata table uses 32-bit offsets.
func (m *UnlinkedMetadataTable) Is32Bit() bool { return m.is32Bit }
