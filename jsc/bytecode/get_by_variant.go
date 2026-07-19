// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/GetByVariant.h

package bytecode

// GetByVariant represents one variant in a polymorphic GetBy IC.
type GetByVariant struct{}

func NewGetByVariant() *GetByVariant { return &GetByVariant{} }
