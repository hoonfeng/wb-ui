// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeGeneratorification.h

package bytecode

// BytecodeGeneratorification transforms generator bytecode for resume/return handling.
type BytecodeGeneratorification struct{}

func NewBytecodeGeneratorification() *BytecodeGeneratorification { return &BytecodeGeneratorification{} }
