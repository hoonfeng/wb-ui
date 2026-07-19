// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type AtomIndices struct {
	Block      *MarkedBlock
	BlockIndex uint32
	AtomNumber uint32
}

func NewAtomIndices(cell *HeapCell) AtomIndices {
	// Simplified
	return AtomIndices{
		BlockIndex: 0,
		AtomNumber: 0,
	}
}
