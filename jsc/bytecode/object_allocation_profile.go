// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ObjectAllocationProfile.h

package bytecode

// ObjectAllocationProfile tracks observed object allocations for allocation-sinking.
type ObjectAllocationProfile struct {
	structureID  uint64
	allocated    bool
}

func NewObjectAllocationProfile() *ObjectAllocationProfile {
	return &ObjectAllocationProfile{}
}

func (p *ObjectAllocationProfile) StructureID() uint64        { return p.structureID }
func (p *ObjectAllocationProfile) SetStructureID(id uint64)   { p.structureID = id }
func (p *ObjectAllocationProfile) Allocated() bool            { return p.allocated }
func (p *ObjectAllocationProfile) SetAllocated(v bool)        { p.allocated = v }
