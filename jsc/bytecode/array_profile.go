// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ArrayProfile.h

package bytecode

// ArrayProfile captures profiling information for array operations.
type ArrayProfile struct {
	lastArrayStructure uint64 // StructureID
	observedArrayModes uint32
	expectedStructure  uint64
}

func NewArrayProfile() *ArrayProfile {
	return &ArrayProfile{}
}

func (p *ArrayProfile) ObservedArrayModes() uint32 { return p.observedArrayModes }
func (p *ArrayProfile) SetObservedArrayModes(m uint32) { p.observedArrayModes = m }
