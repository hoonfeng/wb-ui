// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PropertyInlineCache.h

package bytecode

// PropertyInlineCache tracks inline cache state for property access (JIT only).
type PropertyInlineCache struct {
	structureID uint64
}

func NewPropertyInlineCache() *PropertyInlineCache     { return &PropertyInlineCache{} }
func (c *PropertyInlineCache) StructureID() uint64       { return c.structureID }
func (c *PropertyInlineCache) SetStructureID(id uint64)  { c.structureID = id }
