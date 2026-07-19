// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ICStatusMap.h

package bytecode

// ICStatusMap maps code origins to their IC status structures.
type ICStatusMap struct{}

func NewICStatusMap() *ICStatusMap { return &ICStatusMap{} }
