// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeOperandsForCheckpoint.h

package bytecode

// BytecodeOperandsForCheckpoint provides operand mapping for bytecode checkpoints.
type BytecodeOperandsForCheckpoint struct{}

func NewBytecodeOperandsForCheckpoint() *BytecodeOperandsForCheckpoint {
	return &BytecodeOperandsForCheckpoint{}
}
