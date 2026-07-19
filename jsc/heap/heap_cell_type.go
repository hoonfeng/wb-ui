// Copyright (C) 2017-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type HeapCellType struct {
	attributes CellAttributes
}

func NewHeapCellType(attrs CellAttributes) *HeapCellType {
	return &HeapCellType{attributes: attrs}
}

func (t *HeapCellType) Attributes() CellAttributes {
	return t.attributes
}

func (t *HeapCellType) FinishSweep(handle *MarkedBlockHandle, freeList *FreeList) {
	// Default implementation: calls MarkedBlock::finishSweepKnowingSubspace
	// Subclasses may override
}

func (t *HeapCellType) Destroy(vm interface{}, cell interface{}) {
	// Default: no-op. Subclasses may override for custom destruction.
}
