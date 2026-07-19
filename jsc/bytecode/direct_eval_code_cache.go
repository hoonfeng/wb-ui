// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DirectEvalCodeCache.h

package bytecode

// DirectEvalCodeCache caches the result of compiling direct eval() calls.
type DirectEvalCodeCache struct {
	cache map[string]*UnlinkedCodeBlock
}

func NewDirectEvalCodeCache() *DirectEvalCodeCache {
	return &DirectEvalCodeCache{cache: make(map[string]*UnlinkedCodeBlock)}
}

func (c *DirectEvalCodeCache) Get(source string) *UnlinkedCodeBlock {
	return c.cache[source]
}

func (c *DirectEvalCodeCache) Put(source string, codeBlock *UnlinkedCodeBlock) {
	c.cache[source] = codeBlock
}

func (c *DirectEvalCodeCache) IsEmpty() bool    { return len(c.cache) == 0 }
func (c *DirectEvalCodeCache) Clear()           { c.cache = make(map[string]*UnlinkedCodeBlock) }
