// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExitingInlineKind.h

package bytecode

// ExitingInlineKind indicates whether an exit originated from an inlined call.
type ExitingInlineKind uint8

const (
	ExitingInlineKindNotInlined   ExitingInlineKind = 0
	ExitingInlineKindInlined      ExitingInlineKind = 1
	ExitingInlineKindVariable     ExitingInlineKind = 2
)

func (k ExitingInlineKind) IsInlined() bool { return k == ExitingInlineKindInlined }
