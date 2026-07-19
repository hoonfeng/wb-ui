// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ProxyableAccessCase.h

package bytecode

// ProxyableAccessCase is an access case that may involve a Proxy object (JIT only).
type ProxyableAccessCase struct {
	AccessCase
}

func NewProxyableAccessCase() *ProxyableAccessCase { return &ProxyableAccessCase{} }
