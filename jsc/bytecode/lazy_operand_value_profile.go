// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/LazyOperandValueProfile.h

package bytecode

// LazyOperandValueProfile is a value profile for operands, computed lazily.
type LazyOperandValueProfile struct {
	operand VirtualRegister
	value   uint64
}

func NewLazyOperandValueProfile(operand VirtualRegister) *LazyOperandValueProfile {
	return &LazyOperandValueProfile{operand: operand}
}

func (p *LazyOperandValueProfile) Operand() VirtualRegister { return p.operand }
func (p *LazyOperandValueProfile) Value() uint64            { return p.value }
func (p *LazyOperandValueProfile) SetValue(v uint64)        { p.value = v }
