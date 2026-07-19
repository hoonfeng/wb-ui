// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/SuperSampler.h

package bytecode

// SuperSampler provides a coarse-grained profiling mechanism (JIT only).
type SuperSampler struct{}

func NewSuperSampler() *SuperSampler { return &SuperSampler{} }
