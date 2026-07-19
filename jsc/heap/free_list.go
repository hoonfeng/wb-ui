// Copyright (C) 2016-2019 Apple Inc. All rights reserved.
// Translated to Go.
package heap

import "math"

type FreeCell struct {
	PreservedBitsForCrashAnalysis uint64
	ScrambledBits                 uint64
}

func Scramble(offsetToNext int32, lengthInBytes uint32, secret uint64) uint64 {
	return (uint64(lengthInBytes)<<32 | uint64(offsetToNext)) ^ secret
}

func Descramble(scrambledBits, secret uint64) (int32, uint32) {
	descrambled := scrambledBits ^ secret
	return int32(descrambled & math.MaxUint32), uint32(descrambled >> 32)
}

func (c *FreeCell) MakeLast(lengthInBytes uint32, secret uint64) {
	c.ScrambledBits = Scramble(1, lengthInBytes, secret)
}

func (c *FreeCell) SetNext(next *FreeCell, lengthInBytes uint32, secret uint64) {
	// Simplified: no pointer arithmetic in Go version
	c.ScrambledBits = Scramble(1, lengthInBytes, secret)
}

func (c *FreeCell) Decode(secret uint64) (int32, uint32) {
	return Descramble(c.ScrambledBits, secret)
}

func AdvanceFreeList(secret uint64, interval **FreeCell, intervalStart, intervalEnd *string) {
	// Simplified: no-op in Go
}

type FreeList struct {
	intervalStart string
	intervalEnd   string
	nextInterval  *FreeCell
	secret        uint64
	originalSize  uint32
	cellSize      uint32
}

func NewFreeList(cellSize uint32) *FreeList {
	return &FreeList{
		cellSize: cellSize,
	}
}

func (fl *FreeList) Clear() {
	fl.intervalStart = ""
	fl.intervalEnd = ""
}

func (fl *FreeList) Initialize(head *FreeCell, secret uint64, bytes uint32) {
	fl.nextInterval = head
	fl.secret = secret
	fl.originalSize = bytes
}

func (fl *FreeList) AllocationWillFail() bool {
	return fl.intervalStart >= fl.intervalEnd && IsSentinel(fl.nextInterval)
}

func (fl *FreeList) AllocationWillSucceed() bool {
	return !fl.AllocationWillFail()
}

func IsSentinel(cell *FreeCell) bool {
	return cell == nil
}

func (fl *FreeList) OriginalSize() uint32 {
	return fl.originalSize
}

func (fl *FreeList) CellSize() uint32 {
	return fl.cellSize
}
