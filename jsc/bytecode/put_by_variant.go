// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PutByVariant.h

package bytecode

// PutByVariant represents one variant in a polymorphic PutBy IC.
type PutByVariant struct{}

func NewPutByVariant() *PutByVariant { return &PutByVariant{} }
