// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExitKind.h

package bytecode

// ExitKind classifies the reason for an OSR exit or bailout.
type ExitKind uint8

const (
	ExitKindNormal             ExitKind = 0
	ExitKindOSRExit            ExitKind = 1
	ExitKindInvalidation       ExitKind = 2
	ExitKindWatchpointFire     ExitKind = 3
	ExitKindUncountable        ExitKind = 4
	ExitKindUnclassified       ExitKind = 5
	ExitKindNotConstant        ExitKind = 6
	ExitKindArgumentsEscaped   ExitKind = 7
	ExitKindHoistingFailed     ExitKind = 8
	ExitKindTypeCheck          ExitKind = 9
	ExitKindBadType            ExitKind = 10
	ExitKindBadCell            ExitKind = 11
	ExitKindBadIdentifier      ExitKind = 12
	ExitKindBadExecutable      ExitKind = 13
	ExitKindBadCache           ExitKind = 14
	ExitKindBadConstantCache   ExitKind = 15
	ExitKindBadIndexingType    ExitKind = 16
	ExitKindBadValue           ExitKind = 17
	ExitKindStoreToFrozenObject ExitKind = 18
	ExitKindStoreToGlobalObject ExitKind = 19
	ExitKindOutOfBounds        ExitKind = 20
	ExitKindInadequateCoverage ExitKind = 21
	ExitKindMaximum            ExitKind = 22
)

func (k ExitKind) IsCountable() bool {
	return k != ExitKindUncountable && k != ExitKindUnclassified
}
