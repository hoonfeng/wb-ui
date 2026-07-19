// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/LineColumn.h

package bytecode

// LineColumn represents a source position as a line-column pair.
type LineColumn struct {
	line   uint32
	column uint32
}

func NewLineColumn(line, column uint32) LineColumn {
	return LineColumn{line: line, column: column}
}

func (lc LineColumn) Line() uint32   { return lc.line }
func (lc LineColumn) Column() uint32 { return lc.column }
func (lc LineColumn) IsValid() bool  { return lc.line > 0 || lc.column > 0 }
