// Translation of: Source/bmalloc/IsoHeap.h
//                  Source/bmalloc/bmalloc.h
// Completeness: 70%
// Simplifications:
//   - Go GC replaces most of bmalloc's malloc/free/scavenge machinery
//   - sync.Pool provides per-type object recycling
//   - No page-level decommit or mmap; Go runtime handles memory

package bmalloc

import (
	"reflect"
	"sync"
	"unsafe"
)

// isoHeapRegistry holds the global per-type IsoHeap instances so that the
// package-level Alloc[T]/Free[T]/Scavenge[T] helpers can find the right pool
// without the caller holding an explicit *IsoHeap reference. The key is
// reflect.Type so that distinct named types with the same layout (e.g. two
// structs both containing a single int) do not collide.
var (
	registryMu sync.RWMutex
	registry   = map[reflect.Type]any{}
)

// IsoHeap is a type-isolated allocator backed by sync.Pool, mirroring
// WebKit's bmalloc::IsoHeap<T>. Each IsoHeap serves objects of a single type
// T, ensuring that pointers of different types never share the same memory.
// Alloc returns a zeroed *T; Free returns the object to the pool for reuse.
type IsoHeap[T any] struct {
	pool sync.Pool
	heap *Heap
}

// NewIsoHeap creates a new IsoHeap for type T.
func NewIsoHeap[T any]() *IsoHeap[T] {
	h := &IsoHeap[T]{
		heap: NewHeap(),
	}
	h.pool.New = func() any {
		return new(T)
	}
	return h
}

// Alloc returns a zeroed *T from the pool. The returned pointer is safe to
// use until Free is called.
func (h *IsoHeap[T]) Alloc() *T {
	p := h.pool.Get().(*T)
	*p = zeroValue[T]()
	h.heap.Record(int(unsafe.Sizeof(*p)))
	return p
}

// TryAlloc is an alias for Alloc. In WebKit, TryAlloc can fail (return
// nullptr) under memory pressure; in Go we always succeed.
func (h *IsoHeap[T]) TryAlloc() *T {
	return h.Alloc()
}

// Free returns ptr to the pool. Passing nil is a no-op.
func (h *IsoHeap[T]) Free(ptr *T) {
	if ptr == nil {
		return
	}
	*ptr = zeroValue[T]()
	h.pool.Put(ptr)
	h.heap.Release(int(unsafe.Sizeof(*ptr)))
}

// zeroValue returns the zero value of T.
func zeroValue[T any]() T {
	var z T
	return z
}

// getGlobalIsoHeap returns (creating if necessary) the global IsoHeap for T.
func getGlobalIsoHeap[T any]() *IsoHeap[T] {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	registryMu.RLock()
	if h, ok := registry[rt]; ok {
		registryMu.RUnlock()
		return h.(*IsoHeap[T])
	}
	registryMu.RUnlock()

	registryMu.Lock()
	defer registryMu.Unlock()
	if h, ok := registry[rt]; ok {
		return h.(*IsoHeap[T])
	}
	h := &IsoHeap[T]{heap: NewHeap()}
	h.pool.New = func() any { return new(T) }
	registry[rt] = h
	return h
}

// Alloc allocates a zeroed *T using the global per-type IsoHeap. This mirrors
// the WTF_MAKE_TZONE_ALLOCATED macro expansion that provides a process-wide
// IsoHeap per type.
func Alloc[T any]() *T {
	return getGlobalIsoHeap[T]().Alloc()
}

// Free returns ptr to the global per-type IsoHeap. nil is a no-op.
func Free[T any](ptr *T) {
	if ptr == nil {
		return
	}
	getGlobalIsoHeap[T]().Free(ptr)
}

// Scavenge releases all pooled objects for type T, mimicking
// bmalloc::Scavenge. After Scavenge, subsequent Alloc calls will allocate
// fresh objects from a new pool.
func Scavenge[T any]() {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	registryMu.Lock()
	defer registryMu.Unlock()
	h := &IsoHeap[T]{heap: NewHeap()}
	h.pool.New = func() any { return new(T) }
	registry[rt] = h
}
