// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InlineCacheHandler.h

package bytecode

// InlineCacheHandler handles inline cache miss and patching (JIT only).
type InlineCacheHandler struct{}

func NewInlineCacheHandler() *InlineCacheHandler { return &InlineCacheHandler{} }
