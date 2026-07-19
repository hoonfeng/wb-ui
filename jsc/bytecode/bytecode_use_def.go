// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeUseDef.h

package bytecode

// BytecodeUseDef provides use/definition analysis for bytecode operands.
type BytecodeUseDef struct{}

func NewBytecodeUseDef() *BytecodeUseDef { return &BytecodeUseDef{} }
