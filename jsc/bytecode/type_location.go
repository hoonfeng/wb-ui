// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/TypeLocation.h

package bytecode

// TypeLocation stores type inference location info.
type TypeLocation struct {
	globalVariable    uint32
	identifier        uint32
	sourceID          SourceID
	sourceOffset      uint32
	globalTypeSet     uint64 // simplified: bitset of observed types
}

func NewTypeLocation(gv, ident uint32, sid SourceID, offset uint32) *TypeLocation {
	return &TypeLocation{
		globalVariable: gv,
		identifier:     ident,
		sourceID:       sid,
		sourceOffset:   offset,
	}
}

func (l *TypeLocation) GlobalVariable() uint32   { return l.globalVariable }
func (l *TypeLocation) Identifier() uint32        { return l.identifier }
func (l *TypeLocation) SourceID() SourceID        { return l.sourceID }
func (l *TypeLocation) SourceOffset() uint32      { return l.sourceOffset }
func (l *TypeLocation) GlobalTypeSet() uint64     { return l.globalTypeSet }
func (l *TypeLocation) SetGlobalTypeSet(s uint64) { l.globalTypeSet = s }
