// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/IterationModeMetadata.h

package bytecode

// IterationModeMetadata stores metadata for iteration-related bytecodes.
type IterationModeMetadata struct {
	kind uint8 // 0=generic, 1=fast-array, 2=fast-object
}

func NewIterationModeMetadata() *IterationModeMetadata { return &IterationModeMetadata{kind: 0} }
func (m *IterationModeMetadata) Kind() uint8            { return m.kind }
func (m *IterationModeMetadata) SetKind(k uint8)        { m.kind = k }
