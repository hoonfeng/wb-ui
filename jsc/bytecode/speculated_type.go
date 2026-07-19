// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/SpeculatedType.h

package bytecode

// SpeculatedType encodes a set of possible JavaScript types for type speculation.
// (Simplified: JSC has ~50 type flags; this is a minimal port.)
type SpeculatedType uint64

const (
	SpeculatedTypeNone               SpeculatedType = 0
	SpeculatedTypeInt32              SpeculatedType = 1 << 0
	SpeculatedTypeBoolean            SpeculatedType = 1 << 1
	SpeculatedTypeNumber             SpeculatedType = 1 << 2
	SpeculatedTypeString             SpeculatedType = 1 << 3
	SpeculatedTypeObject             SpeculatedType = 1 << 4
	SpeculatedTypeSymbol             SpeculatedType = 1 << 5
	SpeculatedTypeBigInt             SpeculatedType = 1 << 6
	SpeculatedTypeCell               SpeculatedType = 1 << 7
	SpeculatedTypeOther              SpeculatedType = 1 << 8
	SpeculatedTypeArray              SpeculatedType = 1 << 9
	SpeculatedTypeFunction           SpeculatedType = 1 << 10
	SpeculatedTypeFinalObject        SpeculatedType = 1 << 11
	SpeculatedTypeTop                SpeculatedType = (1 << 12) - 1
)

func (t SpeculatedType) Includes(other SpeculatedType) bool { return t&other != 0 }
func (t SpeculatedType) IsNone() bool                       { return t == SpeculatedTypeNone }
func (t SpeculatedType) IsTop() bool                        { return t == SpeculatedTypeTop }

// MergeSpeculatedType merges two speculated types into the union.
func MergeSpeculatedType(a, b SpeculatedType) SpeculatedType { return a | b }
