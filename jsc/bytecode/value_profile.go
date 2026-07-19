// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ValueProfile.h

package bytecode

// ValueProfile stores profiling data for a value-producing instruction.
type ValueProfile struct {
	observedValue uint64
	observedTypes SpeculatedType
}

func NewValueProfile() *ValueProfile { return &ValueProfile{} }
func (p *ValueProfile) ObservedValue() uint64      { return p.observedValue }
func (p *ValueProfile) SetObservedValue(v uint64)  { p.observedValue = v }
func (p *ValueProfile) ObservedTypes() SpeculatedType { return p.observedTypes }
func (p *ValueProfile) SetObservedTypes(t SpeculatedType) { p.observedTypes = t }
