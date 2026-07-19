// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InternalFunctionAllocationProfile.h

package bytecode

// InternalFunctionAllocationProfile tracks allocation of InternalFunction instances.
type InternalFunctionAllocationProfile struct {
	structureID uint64
	allocated   bool
}

func NewInternalFunctionAllocationProfile() *InternalFunctionAllocationProfile {
	return &InternalFunctionAllocationProfile{}
}

func (p *InternalFunctionAllocationProfile) Allocated() bool        { return p.allocated }
func (p *InternalFunctionAllocationProfile) SetAllocated(v bool)   { p.allocated = v }
func (p *InternalFunctionAllocationProfile) StructureID() uint64   { return p.structureID }
func (p *InternalFunctionAllocationProfile) SetStructureID(id uint64) { p.structureID = id }
