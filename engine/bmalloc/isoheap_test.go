package bmalloc

import (
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

// node is a representative IsoHeap-managed type, mirroring a WTF_MAKE_TZONE_ALLOCATED
// class. It carries a pointer and a couple of scalar fields so we can verify
// that Free properly clears retained state.
type node struct {
	next *node
	id   int
	tag  string
}

func TestIsoHeapAllocReturnsZeroed(t *testing.T) {
	h := NewIsoHeap[node]()
	p := h.Alloc()
	if p == nil {
		t.Fatalf("Alloc returned nil")
	}
	if p.next != nil || p.id != 0 || p.tag != "" {
		t.Fatalf("Alloc did not return a zeroed value: %+v", *p)
	}
}

func TestIsoHeapAllocFreeReuses(t *testing.T) {
	h := NewIsoHeap[node]()
	first := h.Alloc()
	first.id = 42
	first.tag = "first"
	h.Free(first)

	// Spin until the pool hands back the same pointer to prove reuse, or give
	// up after enough attempts (sync.Pool is not guaranteed to return the same
	// object, but within a single goroutine it usually does).
	var got *node
	for i := 0; i < 64; i++ {
		got = h.Alloc()
		if got == first {
			break
		}
		h.Free(got)
	}
	if got != first {
		t.Logf("pool did not reuse same pointer (acceptable): %p != %p", got, first)
	}
	// Reused or not, the returned object must be zeroed.
	if got.id != 0 || got.tag != "" || got.next != nil {
		t.Fatalf("Alloc after Free did not return a zeroed value: %+v", *got)
	}
}

func TestIsoHeapFreeNilIsNoOp(t *testing.T) {
	h := NewIsoHeap[node]()
	h.Free(nil) // must not panic
}

func TestIsoHeapTryAllocEqualsAlloc(t *testing.T) {
	h := NewIsoHeap[node]()
	p := h.TryAlloc()
	if p == nil {
		t.Fatalf("TryAlloc returned nil")
	}
	h.Free(p)
}

func TestIsoHeapTypeIsolation(t *testing.T) {
	hN := NewIsoHeap[node]()
	other := struct {
		x int
		y float64
	}{}
	hO := NewIsoHeap[struct {
		x int
		y float64
	}]()

	pn := hN.Alloc()
	po := hO.Alloc()

	// Pointers must not alias (different backing allocations / types).
	if uintptr(unsafe.Pointer(pn)) == uintptr(unsafe.Pointer(po)) {
		t.Fatalf("pointers of different types alias: %p", pn)
	}
	// Sizes must match their respective types.
	if size := unsafe.Sizeof(*pn); size != unsafe.Sizeof(node{}) {
		t.Fatalf("node size = %d, want %d", size, unsafe.Sizeof(node{}))
	}
	if size := unsafe.Sizeof(*po); size != unsafe.Sizeof(other) {
		t.Fatalf("other size = %d, want %d", size, unsafe.Sizeof(other))
	}

	// Returning each pointer to its own pool must not panic (type mismatch
	// would surface as a panic in a stricter design).
	hN.Free(pn)
	hO.Free(po)
}

func TestGlobalAllocFree(t *testing.T) {
	Scavenge[node]() // start clean
	p := Alloc[node]()
	if p == nil {
		t.Fatalf("Alloc returned nil")
	}
	p.id = 7
	p.tag = "global"
	Free(p)

	p2 := Alloc[node]()
	if p2.id != 0 || p2.tag != "" {
		t.Fatalf("global Alloc after Free not zeroed: %+v", *p2)
	}
	Free(p2)
}

func TestGlobalTypeIsolation(t *testing.T) {
	type alpha struct{ n int }
	type beta struct{ n int }

	pa := Alloc[alpha]()
	pb := Alloc[beta]()
	*pa = alpha{n: 1}
	*pb = beta{n: 2}
	Free(pa)
	Free(pb)

	// The two types share only the global registry, not a pool, so freeing one
	// never feeds the other.
	ra := Alloc[alpha]()
	rb := Alloc[beta]()
	if ra.n != 0 {
		t.Fatalf("alpha cross-contaminated: %+v", *ra)
	}
	if rb.n != 0 {
		t.Fatalf("beta cross-contaminated: %+v", *rb)
	}
	Free(ra)
	Free(rb)
}

func TestScavengeReleasesPool(t *testing.T) {
	Scavenge[node]()
	p := Alloc[node]()
	*p = node{id: 99}
	Free(p)
	Scavenge[node]()
	// After scavenge a new allocation is still valid and zeroed.
	p2 := Alloc[node]()
	if p2.id != 0 {
		t.Fatalf("Alloc after Scavenge not zeroed: %+v", *p2)
	}
	Free(p2)
}

func TestConcurrentAllocFree(t *testing.T) {
	Scavenge[node]()
	h := NewIsoHeap[node]()

	const goroutines = 32
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	var errs int64
	for g := 0; g < goroutines; g++ {
		go func(seed int) {
			defer wg.Done()
			hold := make([]*node, 0, 8)
			for i := 0; i < iterations; i++ {
				p := h.Alloc()
				// Validate zero-initialisation invariant under concurrency.
				if p.next != nil || p.id != 0 || p.tag != "" {
					atomic.AddInt64(&errs, 1)
				}
				p.id = seed*1000 + i
				hold = append(hold, p)
				if len(hold) >= 8 {
					for _, q := range hold {
						h.Free(q)
					}
					hold = hold[:0]
				}
			}
			for _, q := range hold {
				h.Free(q)
			}
		}(g)
	}
	wg.Wait()
	if errs != 0 {
		t.Fatalf("non-zeroed allocations observed: %d", errs)
	}
}

func TestConcurrentGlobalAPI(t *testing.T) {
	type item struct{ v int }
	Scavenge[item]()

	const goroutines = 16
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				p := Alloc[item]()
				if p.v != 0 {
					t.Errorf("non-zeroed global alloc: %d", p.v)
				}
				p.v = i
				Free(p)
			}
		}()
	}
	wg.Wait()
}
