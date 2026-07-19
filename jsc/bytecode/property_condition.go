// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PropertyCondition.h

package bytecode

// PropertyCondition describes a property condition for speculation.
// kind: 0=Equivalence, 1=HasInstance, 2=Presence, 3=Absence, 4=AbsenceOfSetter
type PropertyCondition struct {
	kind   uint8
	offset uint32
	object uint64 // JSCell*
}

func NewPropertyCondition(kind uint8) PropertyCondition {
	return PropertyCondition{kind: kind}
}

func (c PropertyCondition) Kind() uint8            { return c.kind }
func (c PropertyCondition) Offset() uint32          { return c.offset }
func (c PropertyCondition) Object() uint64          { return c.object }
func (c *PropertyCondition) SetObject(o uint64)     { c.object = o }
func (c *PropertyCondition) SetOffset(o uint32)     { c.offset = o }
