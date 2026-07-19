// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DFGExitProfile.h

package bytecode

// DFGExitProfile collects exit frequency data for DFG JIT.
type DFGExitProfile struct{}

func NewDFGExitProfile() *DFGExitProfile { return &DFGExitProfile{} }
