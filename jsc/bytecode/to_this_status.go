// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ToThisStatus.h

package bytecode

// ToThisStatus tracks the result of ToThis conversion for optimization.
type ToThisStatus uint8

const (
	ToThisStatusUninitialized ToThisStatus = 0
	ToThisStatusOK            ToThisStatus = 1
	ToThisStatusGood          ToThisStatus = 2
)

func (s ToThisStatus) IsOK() bool   { return s >= ToThisStatusOK }
func (s ToThisStatus) IsGood() bool { return s == ToThisStatusGood }
