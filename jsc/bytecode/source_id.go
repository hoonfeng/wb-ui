// Copyright (C) 2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/SourceID.h

package bytecode

// SourceID uniquely identifies a source file.
type SourceID uint64

const NoSourceID SourceID = 0

// ParseHash represents the hash of parsed source code.
type ParseHash struct {
	hash uint32
}

func NewParseHash(h uint32) ParseHash {
	return ParseHash{hash: h}
}

func (h ParseHash) Hash() uint32 { return h.hash }
