// Copyright (C) 2016-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type AlignedMemoryAllocator struct{}

func NewAlignedMemoryAllocator() *AlignedMemoryAllocator {
	return &AlignedMemoryAllocator{}
}

type AllocatingScope struct{}

func NewAllocatingScope(heap *Heap) *AllocatingScope {
	return &AllocatingScope{}
}

func (s *AllocatingScope) Close() {}

type Allocator struct{}
type AllocatorForMode struct{}

type CodeBlockSet struct{}
type CollectingScope struct{}

func NewCollectingScope(heap *Heap) *CollectingScope {
	return &CollectingScope{}
}

func (s *CollectingScope) Close() {}

type ConservativeRoots struct{}
type FastMallocAlignedMemoryAllocator struct{}

type GCDeferralContext struct {
	heap *Heap
}

func NewGCDeferralContext(heap *Heap) *GCDeferralContext {
	return &GCDeferralContext{heap: heap}
}

type GCIncomingRefCounted struct{}
type GCIncomingRefCountedSet struct{}
type GCOwnedDataScope struct{}
type GCSegmentedArray struct{}

type Handle struct{}
type HandleBlock struct{}
type HandleSet struct{}

type HeapAnalyzer struct{}
type HeapFinalizerCallback struct{}
type HeapIterationScope struct{}
type HeapProfiler struct{}
type HeapSnapshot struct{}
type HeapSnapshotBuilder struct{}

type IncrementalSweeper struct{}
type IsoCellSet struct{}
type IsoHeapCellType struct{}
type IsoInlinedHeapCellType struct{}
type JITStubRoutineSet struct{}
type LocalAllocator struct{}
type MachineStackMarker struct{}
type MarkStack struct{}
type MarkStackMergingConstraint struct{}
type MarkedBlockSet struct{}
type MarkingConstraint struct{}
type MarkingConstraintExecutorPair struct{}
type MarkingConstraintSet struct{}
type MarkingConstraintSolver struct{}
type MutatorScheduler struct{}
type PreciseSubspace struct{}
type PreventCollectionScope struct{}

func NewPreventCollectionScope(heap *Heap) *PreventCollectionScope {
	return &PreventCollectionScope{}
}

func (s *PreventCollectionScope) Close() {}

type RegisterState struct{}
type ReleaseHeapAccessScope struct{}

func NewReleaseHeapAccessScope(heap *Heap) *ReleaseHeapAccessScope {
	return &ReleaseHeapAccessScope{}
}

func (s *ReleaseHeapAccessScope) Close() {}

type RunningScope struct{}
type SimpleMarkingConstraint struct{}
type SlotVisitor struct{}
type SlotVisitorMacros struct{}
type SpaceTimeMutatorScheduler struct{}
type StochasticSpaceTimeMutatorScheduler struct{}
type StopIfNecessaryTimer struct{}
type Strong struct{}
type StrongForward struct{}
type StructureAlignedMemoryAllocator struct{}
type SweepingScope struct{}
type SynchronousStopTheWorldMutatorScheduler struct{}
type TinyBloomFilter struct{}
type VerifierSlotVisitor struct{}
type VisitCounter struct{}

type AbstractSlotVisitor struct{}
