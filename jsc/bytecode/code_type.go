// Copyright (C) 2012 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeType.h

package bytecode

import "fmt"

type CodeType uint8

const (
	GlobalCode  CodeType = 0
	EvalCode    CodeType = 1
	FunctionCode CodeType = 2
	ModuleCode  CodeType = 3
)

func (c CodeType) String() string {
	switch c {
	case GlobalCode:
		return "GlobalCode"
	case EvalCode:
		return "EvalCode"
	case FunctionCode:
		return "FunctionCode"
	case ModuleCode:
		return "ModuleCode"
	default:
		return fmt.Sprintf("CodeType(%d)", c)
	}
}
