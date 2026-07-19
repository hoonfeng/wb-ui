// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ParseHash.h

package bytecode

// ParseHash computes a hash from source text for cached code identification.
func ParseHash(source string) uint32 {
	var hash uint32 = 0
	for i := 0; i < len(source); i++ {
		hash = hash*31 + uint32(source[i])
	}
	return hash
}
