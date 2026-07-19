// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PutByIdFlags.h

package bytecode

// PutByIdFlags encodes flags for put_by_id inline caches.
type PutByIdFlags uint8

const (
	PutByIdFlagsNormal          PutByIdFlags = 0
	PutByIdFlagsStrict          PutByIdFlags = 1
	PutByIdFlagsTransition      PutByIdFlags = 2
	PutByIdFlagsDirect          PutByIdFlags = 3
)

func (f PutByIdFlags) IsStrict() bool     { return f&PutByIdFlagsStrict != 0 }
func (f PutByIdFlags) IsDirect() bool     { return f == PutByIdFlagsDirect }
