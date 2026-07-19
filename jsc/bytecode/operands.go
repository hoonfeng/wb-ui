// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/Operands.h

package bytecode

// Operands describes the layout of operands (tmps, locals, args) for a code block.
type Operands struct {
	operands []VirtualRegister
}

func NewOperands(size int) *Operands {
	return &Operands{operands: make([]VirtualRegister, size)}
}

func (o *Operands) Size() int                          { return len(o.operands) }
func (o *Operands) Operand(i int) VirtualRegister      { return o.operands[i] }
func (o *Operands) SetOperand(i int, v VirtualRegister) { o.operands[i] = v }
func (o *Operands) IsEmpty() bool                      { return len(o.operands) == 0 }
