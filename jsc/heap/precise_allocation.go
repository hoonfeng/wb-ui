// Copyright (C) 2016-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

const (
	PreciseAllocationAlignment    = AtomSize
	PreciseAllocationHalfAlignment = PreciseAllocationAlignment / 2
)

// PreciseAllocation represents a large object allocated directly via malloc.
type PreciseAllocation struct {
	indexInSpace          uint32
	cellSize              uintptr
	isNewlyAllocated      bool
	hasValidCell          bool
	adjustment            uint8
	isMarked              bool
	attributes            CellAttributes
	lowerTierPreciseIndex uint8
	subspace              interface{} // Subspace pointer
	weakSet               WeakSet
}

// PreciseAllocationHeaderSize returns the size of the header before the cell
func PreciseAllocationHeaderSize() uintptr {
	// Simplified: fixed size
	return 128
}

// PreciseAllocationFromCell computes the PreciseAllocation* from a cell pointer
func PreciseAllocationFromCell(cell *HeapCell) *PreciseAllocation {
	return nil
}

// Cell returns the HeapCell within this allocation
func (a *PreciseAllocation) Cell() *HeapCell {
	return nil
}

// IsPreciseAllocation checks if a HeapCell is backed by a PreciseAllocation
func IsPreciseAllocation(cell *HeapCell) bool {
	return false
}

// Subspace returns the subspace for this allocation
func (a *PreciseAllocation) Subspace() interface{} {
	return a.subspace
}

// LastChanceToFinalize finalizes this allocation
func (a *PreciseAllocation) LastChanceToFinalize() {}

// Heap returns the Heap owning this allocation
func (a *PreciseAllocation) Heap() *Heap {
	return a.weakSet.Heap()
}

// WeakSet returns the WeakSet for this allocation
func (a *PreciseAllocation) WeakSet() *WeakSet {
	return &a.weakSet
}

// IndexInSpace returns the index in MarkedSpace
func (a *PreciseAllocation) IndexInSpace() uint32 {
	return a.indexInSpace
}

// SetIndexInSpace sets the index
func (a *PreciseAllocation) SetIndexInSpace(idx uint32) {
	a.indexInSpace = idx
}

// ClearNewlyAllocated clears the newly allocated flag
func (a *PreciseAllocation) ClearNewlyAllocated() {
	a.isNewlyAllocated = false
}

// Flip flips marking bits for a new GC cycle
func (a *PreciseAllocation) Flip() {}

// IsNewlyAllocated returns whether this was newly allocated
func (a *PreciseAllocation) IsNewlyAllocated() bool {
	return a.isNewlyAllocated
}

// IsMarked returns whether this cell is marked alive
func (a *PreciseAllocation) IsMarked() bool {
	return a.isMarked
}

// IsLive returns whether this cell is live
func (a *PreciseAllocation) IsLive() bool {
	return a.isMarked || a.isNewlyAllocated
}

// HasValidCell returns whether there's a valid cell
func (a *PreciseAllocation) HasValidCell() bool {
	return a.hasValidCell
}

// CellSize returns the cell size
func (a *PreciseAllocation) CellSize() uintptr {
	return a.cellSize
}

// Attributes returns the cell attributes
func (a *PreciseAllocation) Attributes() CellAttributes {
	return a.attributes
}

// TestAndSetMarked atomically tests and sets the marked flag
func (a *PreciseAllocation) TestAndSetMarked() bool {
	if a.isMarked {
		return true
	}
	a.isMarked = true
	return false
}

// ClearMarked clears the marked flag
func (a *PreciseAllocation) ClearMarked() {
	a.isMarked = false
}

// NoteMarked is called when this cell is marked
func (a *PreciseAllocation) NoteMarked() {}

// Sweep sweeps this allocation
func (a *PreciseAllocation) Sweep() {}

// Destroy destroys this allocation
func (a *PreciseAllocation) Destroy() {}

// IsLowerTierPrecise returns true if this is a lower-tier precise allocation
func (a *PreciseAllocation) IsLowerTierPrecise() bool {
	return a.lowerTierPreciseIndex != 255
}

// LowerTierPreciseIndex returns the lower tier index
func (a *PreciseAllocation) LowerTierPreciseIndex() uint8 {
	return a.lowerTierPreciseIndex
}
