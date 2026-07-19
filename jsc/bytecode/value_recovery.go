// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ValueRecovery.h

package bytecode

// ValueRecovery describes how to recover a DFG virtual register's value
// after OSR exit (in the interpreter).
type ValueRecovery struct {
	technique uint8
	data      uint64
}

const (
	RecoveryAlreadyInJSStack                  uint8 = 1
	RecoveryAlreadyInJSStackAsUnboxedInt32    uint8 = 2
	RecoveryAlreadyInJSStackAsUnboxedDouble   uint8 = 3
	RecoveryConstant                           uint8 = 4
	RecoveryDirectArgumentsThatWereNotCreated  uint8 = 5
	RecoveryClonedArgumentsThatWereNotCreated  uint8 = 6
)

func NewValueRecovery(technique uint8, data uint64) ValueRecovery {
	return ValueRecovery{technique: technique, data: data}
}

func (r ValueRecovery) Technique() uint8 { return r.technique }
func (r ValueRecovery) Data() uint64     { return r.data }
func (r ValueRecovery) IsConstant() bool { return r.technique == RecoveryConstant }
