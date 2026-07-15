// Package bmalloc provides WebKit's bmalloc (Source/bmalloc) ported to Go.
//
// bmalloc is WebKit's malloc implementation featuring IsoHeap, a per-type
// segregation allocator that prevents use-after-free type confusion attacks.
// The Go port wraps sync.Pool and per-type slab allocators.
package bmalloc
