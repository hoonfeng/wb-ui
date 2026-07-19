// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/AccessCaseSnippetParams.h

package bytecode

// AccessCaseSnippetParams holds parameters for access case snippet generation (JIT only).
type AccessCaseSnippetParams struct{}

func NewAccessCaseSnippetParams() *AccessCaseSnippetParams { return &AccessCaseSnippetParams{} }
