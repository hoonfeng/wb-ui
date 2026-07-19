// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/Repatch.h

package bytecode

// Repatch provides utilities for re-patching JIT code (JIT only).
type Repatch struct{}

func NewRepatch() *Repatch { return &Repatch{} }
