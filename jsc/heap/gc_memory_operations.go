// Copyright (C) 2017-2023 Apple Inc. All rights reserved.
// Translated to Go.
package heap

import "unsafe"
type GCMemoryOperations struct{}

func GCMemcpy(dst, src unsafe.Pointer, size uintptr) {
	// Simplified: uses Go memcpy which is GC-safe
	copy((*[1 << 30]byte)(dst)[:size], (*[1 << 30]byte)(src)[:size])
}

func GCMemset(dst unsafe.Pointer, val byte, size uintptr) {
	b := (*[1 << 30]byte)(dst)[:size]
	for i := range b {
		b[i] = val
	}
}
