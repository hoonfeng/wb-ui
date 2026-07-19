// Heap corresponds to JSC::Heap (runtime/Heap.h - simplified)
package runtime

// Heap manages GC memory (corresponds to JSC::Heap, simplified).
// In Go translation, most GC is handled by the Go runtime.
type Heap struct{}

// ClassInfo holds metadata about a JS class (corresponds to JSC::ClassInfo).
type ClassInfo struct{}
