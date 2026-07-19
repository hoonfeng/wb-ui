// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ReduceWhitespace.h

package bytecode

// ReduceWhitespace reduces consecutive whitespace to a single space.
func ReduceWhitespace(s string) string {
	if len(s) == 0 {
		return s
	}
	result := make([]byte, 0, len(s))
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !prevSpace {
				result = append(result, ' ')
			}
			prevSpace = true
		} else {
			result = append(result, c)
			prevSpace = false
		}
	}
	return string(result)
}
