// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeDumper.h

package bytecode

// BytecodeDumper is a utility for dumping bytecode disassembly.
type BytecodeDumper struct{}

func NewBytecodeDumper() *BytecodeDumper { return &BytecodeDumper{} }
func DumpBytecode(cb *CodeBlock) string { return "bytecode dump not implemented" }
