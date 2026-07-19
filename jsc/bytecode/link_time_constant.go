// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/LinkTimeConstant.h

package bytecode

// LinkTimeConstant represents a well-known constant resolved at code link time.
type LinkTimeConstant struct {
	index uint32
}

func NewLinkTimeConstant(index uint32) LinkTimeConstant {
	return LinkTimeConstant{index: index}
}

func (c LinkTimeConstant) Index() uint32 { return c.index }
func (c LinkTimeConstant) IsValid() bool { return c.index > 0 }
