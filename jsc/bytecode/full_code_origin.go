// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/FullCodeOrigin.h

package bytecode

// FullCodeOrigin extends CodeOrigin with source position info (line/column).
type FullCodeOrigin struct {
	CodeOrigin
	line   uint32
	column uint32
}

func NewFullCodeOrigin(origin CodeOrigin, line, column uint32) FullCodeOrigin {
	return FullCodeOrigin{CodeOrigin: origin, line: line, column: column}
}

func (f FullCodeOrigin) Line() uint32   { return f.line }
func (f FullCodeOrigin) Column() uint32 { return f.column }
