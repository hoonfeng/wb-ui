// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeLivenessAnalysis.h

package bytecode

// BytecodeLivenessAnalysis determines variable liveness at each bytecode offset.
type BytecodeLivenessAnalysis struct{}

func NewBytecodeLivenessAnalysis() *BytecodeLivenessAnalysis { return &BytecodeLivenessAnalysis{} }
