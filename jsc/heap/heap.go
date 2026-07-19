// Copyright (C) 2013-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type Heap struct {
	// Core GC state
	markedSpace      MarkedSpace
	mutatorState     MutatorState
	collectorPhase   CollectorPhase
	collectionScope  *CollectionScope
	isSafeToCollect  bool
}

func NewHeap() *Heap {
	return &Heap{
		mutatorState:    MutatorStateRunning,
		collectorPhase:  CollectorPhaseNotRunning,
		isSafeToCollect: false,
	}
}

func (h *Heap) MutatorState() MutatorState {
	return h.mutatorState
}

func (h *Heap) SetMutatorState(s MutatorState) {
	h.mutatorState = s
}

func (h *Heap) CollectorPhase() CollectorPhase {
	return h.collectorPhase
}

func (h *Heap) SetCollectorPhase(p CollectorPhase) {
	h.collectorPhase = p
}

func (h *Heap) IsSafeToCollect() bool {
	return h.isSafeToCollect
}
