// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/SharedJITStubSet.h

package bytecode

// SharedJITStubSet provides shared JIT stub routines (JIT only).
type SharedJITStubSet struct{}

func NewSharedJITStubSet() *SharedJITStubSet { return &SharedJITStubSet{} }
