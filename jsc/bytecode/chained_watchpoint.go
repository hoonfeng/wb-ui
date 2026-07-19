// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ChainedWatchpoint.h

package bytecode

// ChainedWatchpoint groups multiple watchpoints; invalidating any one invalidates the whole chain.
type ChainedWatchpoint struct{}

func NewChainedWatchpoint() *ChainedWatchpoint { return &ChainedWatchpoint{} }
