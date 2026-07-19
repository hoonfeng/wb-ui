// Copyright (C) 2012 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeOrigin.h

package bytecode

import "fmt"

// CodeOrigin identifies the bytecode offset where a value or operation originated.
type CodeOrigin struct {
	bytecodeIndex BytecodeIndex
}

func NewCodeOrigin(index BytecodeIndex) CodeOrigin {
	return CodeOrigin{bytecodeIndex: index}
}

func (c CodeOrigin) BytecodeIndex() BytecodeIndex { return c.bytecodeIndex }
func (c CodeOrigin) IsSet() bool { return c.bytecodeIndex.IsValid() }

func (c CodeOrigin) String() string {
	return fmt.Sprintf("CodeOrigin(%d)", c.bytecodeIndex.Offset())
}

// FullCodeOrigin extends CodeOrigin with source position info.
type FullCodeOrigin struct {
	CodeOrigin
	line   uint32
	column uint32
}

func NewFullCodeOrigin(origin CodeOrigin, line, column uint32) FullCodeOrigin {
	return FullCodeOrigin{CodeOrigin: origin, line: line, column: column}
}

func (f FullCodeOrigin) Line() uint32 { return f.line }
func (f FullCodeOrigin) Column() uint32 { return f.column }

// TerminatedCodeOrigin pairs a CodeOrigin with a termination watchpoint.
type TerminatedCodeOrigin struct {
	CodeOrigin
	watchpoint InlineWatchpointSet
}
