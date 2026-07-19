// Copyright (C) 2015 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ComplexGetStatus.h

package bytecode

// ComplexGetStatus describes the result of a complex property get operation.
type ComplexGetStatus struct {
	kind      uint8 // 0=NotExists, 1=Exists, 2=TakesSlowPath
	offset    uint32
	makesCalls bool
}

func (s *ComplexGetStatus) IsNotExists() bool { return s.kind == 0 }
func (s *ComplexGetStatus) Exists() bool { return s.kind == 1 }
func (s *ComplexGetStatus) TakesSlowPath() bool { return s.kind == 2 }
