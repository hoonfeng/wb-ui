// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PropertyInlineCacheClearingWatchpoint.h

package bytecode

// PropertyInlineCacheClearingWatchpoint fires to clear a property inline cache.
type PropertyInlineCacheClearingWatchpoint struct{}

func NewPropertyInlineCacheClearingWatchpoint() *PropertyInlineCacheClearingWatchpoint {
	return &PropertyInlineCacheClearingWatchpoint{}
}
