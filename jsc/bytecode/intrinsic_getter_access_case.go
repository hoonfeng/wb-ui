// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/IntrinsicGetterAccessCase.h

package bytecode

// IntrinsicGetterAccessCase is an access case for intrinsic getter accesses (JIT only).
type IntrinsicGetterAccessCase struct {
	AccessCase
}

func NewIntrinsicGetterAccessCase() *IntrinsicGetterAccessCase { return &IntrinsicGetterAccessCase{} }
