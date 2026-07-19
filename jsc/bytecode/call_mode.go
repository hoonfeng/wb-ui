// Copyright (C) 2015 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallMode.h

package bytecode

import "fmt"

type CallMode uint8

const (
	CallModeRegular   CallMode = 0
	CallModeTail      CallMode = 1
	CallModeConstruct CallMode = 2
)

type FrameAction uint8

const (
	KeepTheFrame  FrameAction = 0
	ReuseTheFrame FrameAction = 1
)

func SpecializationKindFor(callMode CallMode) CodeSpecializationKind {
	if callMode == CallModeConstruct {
		return CodeForConstruct
	}
	return CodeForCall
}

func (c CallMode) String() string {
	switch c {
	case CallModeRegular:
		return "CallMode::Regular"
	case CallModeTail:
		return "CallMode::Tail"
	case CallModeConstruct:
		return "CallMode::Construct"
	default:
		return fmt.Sprintf("CallMode(%d)", c)
	}
}
