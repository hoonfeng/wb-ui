// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InlineAccess.h

package bytecode

// InlineAccess provides utilities for generating inline property access code (JIT only).
type InlineAccess struct{}

func NewInlineAccess() *InlineAccess { return &InlineAccess{} }
