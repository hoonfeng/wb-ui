// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/GetterSetterAccessCase.h

package bytecode

// GetterSetterAccessCase is an access case for getter/setter accesses (JIT only).
type GetterSetterAccessCase struct {
	AccessCase
}

func NewGetterSetterAccessCase() *GetterSetterAccessCase { return &GetterSetterAccessCase{} }
