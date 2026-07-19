// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallLinkInfoBase.h

package bytecode

// CallLinkInfoBase is the base type for call link info.
type CallLinkInfoBase struct{}

func NewCallLinkInfoBase() *CallLinkInfoBase { return &CallLinkInfoBase{} }
