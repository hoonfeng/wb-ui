// Copyright (C) 2015-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type GCActivityCallback struct{}

func NewGCActivityCallback(heap *Heap) *GCActivityCallback {
	return &GCActivityCallback{}
}

func (c *GCActivityCallback) WillCollect() {}
func (c *GCActivityCallback) DidCollect(scope CollectionScope) {}

type FullGCActivityCallback struct {
	GCActivityCallback
}
type EdenGCActivityCallback struct {
	GCActivityCallback
}
type HeapHelperPool struct{}

type HeapUtil struct{}

type MutatorSchedulerBase struct{}

type PackedCellPtr struct{}
type ParallelSourceAdapter struct{}

type WeakHandleOwnerBase struct{}
type WeakBlockHard struct{}
type WriteBarrierSupport struct{}
