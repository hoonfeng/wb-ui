// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InByVariant.h

package bytecode

// InByVariant represents one variant in a polymorphic InBy IC.
type InByVariant struct{}

func NewInByVariant() *InByVariant { return &InByVariant{} }
