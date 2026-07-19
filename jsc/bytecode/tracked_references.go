// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/TrackedReferences.h

package bytecode

// TrackedReferences tracks JSCell references for GC conservative scanning.
type TrackedReferences struct {
	references []uint64
}

func NewTrackedReferences() *TrackedReferences {
	return &TrackedReferences{references: make([]uint64, 0)}
}

func (t *TrackedReferences) Add(ref uint64) {
	t.references = append(t.references, ref)
}

func (t *TrackedReferences) Contains(ref uint64) bool {
	for _, r := range t.references {
		if r == ref {
			return true
		}
	}
	return false
}

func (t *TrackedReferences) Size() int                     { return len(t.references) }
func (t *TrackedReferences) IsEmpty() bool                 { return len(t.references) == 0 }
func (t *TrackedReferences) At(i int) uint64               { return t.references[i] }
