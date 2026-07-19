// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExitFlag.h

package bytecode

// ExitFlag tracks whether an exit happened in any of a set of code origins.
type ExitFlag struct {
	bits uint8
}

func NewExitFlag() ExitFlag                          { return ExitFlag{} }
func NewExitFlagWithBit(bit uint8) ExitFlag          { return ExitFlag{bits: bit} }
func (f ExitFlag) IsSet() bool                       { return f.bits != 0 }
func (f ExitFlag) Bits() uint8                       { return f.bits }
func (f ExitFlag) Add(other ExitFlag) ExitFlag       { return ExitFlag{bits: f.bits | other.bits} }
