// Copyright (C) 2012, 2016 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeConventions.h

package bytecode

// Register numbers used in bytecode operations have different meaning according to their ranges:
//
//	0x80000000-0xFFFFFFFF  Negative indices from the CallFrame pointer are entries in the call frame.
//	0x00000000-0x3FFFFFFF  Forwards indices from the CallFrame pointer are local vars and temporaries with the function's callframe.
//	0x40000000-0x7FFFFFFF  Positive indices from 0x40000000 specify entries in the constant pool on the CodeBlock.
const (
	FirstConstantRegisterIndex    = 0x40000000
	FirstConstantRegisterIndex8   = 16
	FirstConstantRegisterIndex16  = 64
	FirstConstantRegisterIndex32  = FirstConstantRegisterIndex
)
