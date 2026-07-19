// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CheckPrivateBrandVariant.h

package bytecode

// CheckPrivateBrandVariant represents one variant in a private brand check IC.
type CheckPrivateBrandVariant struct{}

func NewCheckPrivateBrandVariant() *CheckPrivateBrandVariant { return &CheckPrivateBrandVariant{} }
