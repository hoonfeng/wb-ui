// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DeferredSourceDump.h

package bytecode

// DeferredSourceDump holds source information needed for deferred compilation dumps.
type DeferredSourceDump struct{}

func NewDeferredSourceDump() *DeferredSourceDump { return &DeferredSourceDump{} }
