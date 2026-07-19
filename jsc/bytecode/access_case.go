// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/AccessCase.h

package bytecode

// AccessCase represents an inline cache access case (JIT only).
// In the interpreter, property access is handled directly.
type AccessCase struct{}

func NewAccessCase() *AccessCase { return &AccessCase{} }
