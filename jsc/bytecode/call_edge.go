// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallEdge.h

package bytecode

// CallEdge describes a call from one code block to another with a call count.
type CallEdge struct {
	callee uint64 // JSCell*
	count  uint32
}

func NewCallEdge(callee uint64, count uint32) CallEdge {
	return CallEdge{callee: callee, count: count}
}

func (e CallEdge) Callee() uint64      { return e.callee }
func (e CallEdge) Count() uint32       { return e.count }
func (e *CallEdge) SetCount(c uint32)  { e.count = c }
