// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeRewriter.h

package bytecode

// BytecodeRewriter rewrites bytecode sequences for optimizations.
type BytecodeRewriter struct{}

func NewBytecodeRewriter() *BytecodeRewriter { return &BytecodeRewriter{} }
