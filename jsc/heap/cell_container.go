// Copyright (C) 2016-2019 Apple Inc. All rights reserved.
// Translated to Go.
package heap

// CellContainer abstracts over either MarkedBlock or PreciseAllocation.
// Simplified Go version uses an interface-based approach instead of
// the C++ uintptr encoding trick.
type CellContainer struct {
	isMarkedBlockVal bool
	markedBlockVal   *MarkedBlock
	preciseAllocVal  *PreciseAllocation
}

func NewCellContainerFromMarkedBlock(block *MarkedBlock) CellContainer {
	return CellContainer{isMarkedBlockVal: true, markedBlockVal: block}
}

func NewCellContainerFromPreciseAllocation(alloc *PreciseAllocation) CellContainer {
	return CellContainer{preciseAllocVal: alloc}
}

func (c CellContainer) IsValid() bool {
	return c.markedBlockVal != nil || c.preciseAllocVal != nil
}

func (c CellContainer) IsMarkedBlock() bool {
	return c.isMarkedBlockVal
}

func (c CellContainer) IsPreciseAllocation() bool {
	return !c.isMarkedBlockVal && c.preciseAllocVal != nil
}

func (c CellContainer) MarkedBlock() *MarkedBlock {
	if !c.IsMarkedBlock() {
		panic("CellContainer is not a MarkedBlock")
	}
	return c.markedBlockVal
}

func (c CellContainer) PreciseAllocation() *PreciseAllocation {
	if !c.IsPreciseAllocation() {
		panic("CellContainer is not a PreciseAllocation")
	}
	return c.preciseAllocVal
}

func (c CellContainer) AreMarksStale() bool { return false }
func (c CellContainer) IsMarked(cell *HeapCell) bool { return false }
func (c CellContainer) NoteMarked() {}
func (c CellContainer) CellSize() uintptr { return 0 }
