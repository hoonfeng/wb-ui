// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeBlockHash.h

package bytecode

// CodeBlockHash is a hash value computed from source code for identifying CodeBlocks.
type CodeBlockHash struct {
	hash uint32
}

func NewCodeBlockHash(hash uint32) CodeBlockHash {
	return CodeBlockHash{hash: hash}
}

func (h CodeBlockHash) Hash() uint32 { return h.hash }
func (h CodeBlockHash) IsValid() bool { return h.hash != 0 }
