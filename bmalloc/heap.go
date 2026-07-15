// Translation of: Source/bmalloc/Heap.cpp
//                  Source/bmalloc/Heap.h
// Completeness: 40%
// Simplifications:
//   - Go GC replaces the scavenger, page decommit and mmap machinery
//   - Only allocation statistics are tracked (alloc count, bytes, active)

package bmalloc

import "sync/atomic"

// Stats is a snapshot of bmalloc allocation counters, mirroring the
// bookkeeping done by WebKit's bmalloc::Heap. All fields are point-in-time
// values read atomically.
type Stats struct {
	// Allocs is the total number of allocations served since the heap was
	// created (or last reset). It is a lifetime counter and is not decremented
	// on Free.
	Allocs int64
	// Bytes is the cumulative number of bytes handed out across all
	// allocations (lifetime total).
	Bytes int64
	// Active is the number of currently outstanding objects that have been
	// allocated but not yet freed.
	Active int64
}

// Heap tracks allocation statistics for bmalloc. This is a simplified Go port
// of WebKit's bmalloc::Heap. In C++ Heap owns pages, line caches, per-size-class
// freelists and drives the scavenger that decommits idle pages. Go's runtime
// garbage collector replaces all of that page/scavenge machinery, so Heap is
// reduced to atomic counters that mirror Heap's accounting role.
//
// All methods are safe for concurrent use.
type Heap struct {
	allocs int64
	bytes  int64
	active int64
}

// NewHeap returns an empty Heap ready to record allocations.
func NewHeap() *Heap { return &Heap{} }

// Record notes an allocation of size bytes, mirroring the bookkeeping side
// effect of Heap::allocate (without the actual page allocation). size is
// expected to be the type's sizeof.
func (h *Heap) Record(size int) {
	atomic.AddInt64(&h.allocs, 1)
	atomic.AddInt64(&h.bytes, int64(size))
	atomic.AddInt64(&h.active, 1)
}

// Release notes that an object of size bytes has been freed, mirroring the
// bookkeeping side effect of Heap::deallocate. The Active count is decremented;
// the lifetime Allocs and Bytes counters are intentionally retained. A stray
// Release without a matching Record is clamped so Active never goes negative.
func (h *Heap) Release(size int) {
	if n := atomic.AddInt64(&h.active, -1); n < 0 {
		// Defensive: a Free without a matching Alloc must not drive the
		// active counter into a runaway negative range. Reset to zero.
		atomic.StoreInt64(&h.active, 0)
	}
}

// Stats returns a point-in-time snapshot of the heap counters.
func (h *Heap) Stats() Stats {
	return Stats{
		Allocs: atomic.LoadInt64(&h.allocs),
		Bytes:  atomic.LoadInt64(&h.bytes),
		Active: atomic.LoadInt64(&h.active),
	}
}

// Reset zeroes all counters. Intended for tests that need a clean baseline.
func (h *Heap) Reset() {
	atomic.StoreInt64(&h.allocs, 0)
	atomic.StoreInt64(&h.bytes, 0)
	atomic.StoreInt64(&h.active, 0)
}
