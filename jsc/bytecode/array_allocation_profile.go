// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ArrayAllocationProfile.h

package bytecode

// ArrayAllocationProfile captures allocation behavior for arrays.
type ArrayAllocationProfile struct {
	allocatedSomething bool
	lastAllocation     uint8 // IndexingType
}

func NewArrayAllocationProfile() *ArrayAllocationProfile {
	return &ArrayAllocationProfile{}
}

func (p *ArrayAllocationProfile) AllocatedSomething() bool { return p.allocatedSomething }
func (p *ArrayAllocationProfile) SetAllocatedSomething(v bool) { p.allocatedSomething = v }
func (p *ArrayAllocationProfile) LastAllocation() uint8 { return p.lastAllocation }
func (p *ArrayAllocationProfile) SetLastAllocation(v uint8) { p.lastAllocation = v }

func (p *ArrayAllocationProfile) SelectIndexingType() uint8 {
	return p.lastAllocation
}

func (p *ArrayAllocationProfile) SelectStructure() uint64 {
	return 0 // simplified: returns 0 (no structure)
}
