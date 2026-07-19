// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InstanceOfAccessCase.h

package bytecode

// InstanceOfAccessCase is an access case for instanceof checks (JIT only).
type InstanceOfAccessCase struct {
	AccessCase
}

func NewInstanceOfAccessCase() *InstanceOfAccessCase { return &InstanceOfAccessCase{} }
