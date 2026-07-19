// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ArithProfile.h

package bytecode

// ArithProfile captures profiling information for arithmetic operations.
type ArithProfile struct {
	bits uint64
}

func NewArithProfile() ArithProfile { return ArithProfile{} }

func (p *ArithProfile) ObservedInt() bool { return p.bits&1 != 0 }
func (p *ArithProfile) ObservedInt52() bool { return p.bits&2 != 0 }
func (p *ArithProfile) ObservedDouble() bool { return p.bits&4 != 0 }
func (p *ArithProfile) ObservedNegZero() bool { return p.bits&8 != 0 }
func (p *ArithProfile) ObservedOverflow() bool { return p.bits&16 != 0 }

func (p *ArithProfile) ObserveInt() { p.bits |= 1 }
func (p *ArithProfile) ObserveInt52() { p.bits |= 2 }
func (p *ArithProfile) ObserveDouble() { p.bits |= 4 }
func (p *ArithProfile) ObserveNegZero() { p.bits |= 8 }
func (p *ArithProfile) ObserveOverflow() { p.bits |= 16 }
