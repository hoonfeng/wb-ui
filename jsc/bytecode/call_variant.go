// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallVariant.h

package bytecode

// CallVariant represents a specific callee variant for polymorphic call ICs.
type CallVariant struct {
	executable uint64 // ExecutableBase*
	isClosure  bool
}

func NewCallVariant(executable uint64) CallVariant {
	return CallVariant{executable: executable}
}

func (v CallVariant) Executable() uint64 { return v.executable }
func (v CallVariant) IsClosure() bool    { return v.isClosure }
func (v *CallVariant) SetIsClosure(c bool) { v.isClosure = c }
