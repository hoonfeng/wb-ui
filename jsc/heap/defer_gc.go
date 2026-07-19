// Copyright (C) 2013-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type DeferGC struct {
	heap *Heap
}

func NewDeferGC(heap *Heap) *DeferGC {
	return &DeferGC{heap: heap}
}

func (d *DeferGC) Close() {}
