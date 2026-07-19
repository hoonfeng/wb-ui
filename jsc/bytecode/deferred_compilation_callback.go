// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DeferredCompilationCallback.h

package bytecode

// DeferredCompilationCallback is notified when deferred compilation completes.
type DeferredCompilationCallback struct{}

func NewDeferredCompilationCallback() *DeferredCompilationCallback { return &DeferredCompilationCallback{} }
