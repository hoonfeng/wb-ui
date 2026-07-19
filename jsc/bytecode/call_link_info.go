// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallLinkInfo.h

package bytecode

// CallLinkInfo holds inline cache state for a call instruction.
type CallLinkInfo struct {
	callType            uint8
	hasBeenSeen         bool
	hasSeenLateRepatch  bool
}

func NewCallLinkInfo() *CallLinkInfo { return &CallLinkInfo{} }

func (i *CallLinkInfo) CallType() uint8            { return i.callType }
func (i *CallLinkInfo) SetCallType(t uint8)         { i.callType = t }
func (i *CallLinkInfo) HasBeenSeen() bool           { return i.hasBeenSeen }
func (i *CallLinkInfo) SetHasBeenSeen(v bool)       { i.hasBeenSeen = v }
func (i *CallLinkInfo) HasSeenLateRepatch() bool    { return i.hasSeenLateRepatch }
func (i *CallLinkInfo) SetHasSeenLateRepatch(v bool) { i.hasSeenLateRepatch = v }
