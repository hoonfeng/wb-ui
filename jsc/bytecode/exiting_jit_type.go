// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExitingJITType.h

package bytecode

// ExitingJITType indicates the JIT type that an exit belongs to.
type ExitingJITType uint8

const (
	ExitingJITTypeInterpreter  ExitingJITType = 0
	ExitingJITTypeBaselineJIT  ExitingJITType = 1
	ExitingJITTypeDFGJIT       ExitingJITType = 2
	ExitingJITTypeFTLJIT       ExitingJITType = 3
)

func (t ExitingJITType) IsJIT() bool { return t != ExitingJITTypeInterpreter }
