// Copyright (C) 2012 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/runtime/CodeSpecializationKind.h

package bytecode

import "fmt"

type CodeSpecializationKind uint8

const (
	CodeForCall      CodeSpecializationKind = 0
	CodeForConstruct CodeSpecializationKind = 1
)

func SpecializationFromIsCall(isCall bool) CodeSpecializationKind {
	if isCall {
		return CodeForCall
	}
	return CodeForConstruct
}

func SpecializationFromIsConstruct(isConstruct bool) CodeSpecializationKind {
	if isConstruct {
		return CodeForConstruct
	}
	return CodeForCall
}

func (k CodeSpecializationKind) String() string {
	switch k {
	case CodeForCall:
		return "CodeForCall"
	case CodeForConstruct:
		return "CodeForConstruct"
	default:
		return fmt.Sprintf("CodeSpecializationKind(%d)", k)
	}
}
