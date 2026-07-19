// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DataFormat.h

package bytecode

// DataFormat describes the low-level data representation of a JS value in the DFG.
type DataFormat uint8

const (
	DataFormatNone    DataFormat = 0
	DataFormatInt32   DataFormat = 1
	DataFormatDouble  DataFormat = 2
	DataFormatCell    DataFormat = 3
	DataFormatBoolean DataFormat = 4
	DataFormatJS      DataFormat = 5
	DataFormatDead    DataFormat = 6
)
