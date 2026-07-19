// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExecutionCounter.h

package bytecode

// ExecutionCounter tracks execution counts for JIT tier-up decisions.
type ExecutionCounter struct {
	count     uint32
	threshold uint32
}

func (c *ExecutionCounter) Count() uint32                       { return c.count }
func (c *ExecutionCounter) SetCount(n uint32)                   { c.count = n }
func (c *ExecutionCounter) Threshold() uint32                   { return c.threshold }
func (c *ExecutionCounter) SetThreshold(t uint32)               { c.threshold = t }
func (c *ExecutionCounter) Increment()                          { c.count++ }
func (c *ExecutionCounter) HasCrossedThreshold() bool           { return c.count >= c.threshold }
func (c *ExecutionCounter) Reset()                              { c.count = 0 }
