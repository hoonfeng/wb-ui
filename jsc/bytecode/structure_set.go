// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/StructureSet.h

package bytecode

// StructureSet is a set of StructureIDs used for polymorphic access predictions.
type StructureSet struct {
	structures []uint64
}

func NewStructureSet() *StructureSet {
	return &StructureSet{structures: make([]uint64, 0)}
}

func (s *StructureSet) Add(id uint64) {
	s.structures = append(s.structures, id)
}

func (s *StructureSet) Size() int                     { return len(s.structures) }
func (s *StructureSet) At(i int) uint64               { return s.structures[i] }
func (s *StructureSet) IsEmpty() bool                  { return len(s.structures) == 0 }
