// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ModuleNamespaceAccessCase.h

package bytecode

// ModuleNamespaceAccessCase is an access case for module namespace accesses (JIT only).
type ModuleNamespaceAccessCase struct {
	AccessCase
}

func NewModuleNamespaceAccessCase() *ModuleNamespaceAccessCase { return &ModuleNamespaceAccessCase{} }
