// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/LLIntPrototypeLoadAdaptiveStructureWatchpoint.h

package bytecode

// LLIntPrototypeLoadAdaptiveStructureWatchpoint watches a prototype's structure
// for LLInt prototype load caching.
type LLIntPrototypeLoadAdaptiveStructureWatchpoint struct{}

func NewLLIntPrototypeLoadAdaptiveStructureWatchpoint() *LLIntPrototypeLoadAdaptiveStructureWatchpoint {
	return &LLIntPrototypeLoadAdaptiveStructureWatchpoint{}
}
