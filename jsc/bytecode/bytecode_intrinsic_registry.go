// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/BytecodeIntrinsicRegistry.h

package bytecode

// BytecodeIntrinsicRegistry maps intrinsic function names to their bytecode opcodes.
type BytecodeIntrinsicRegistry struct {
	intrinsics map[string]uint8
}

func NewBytecodeIntrinsicRegistry() *BytecodeIntrinsicRegistry {
	return &BytecodeIntrinsicRegistry{intrinsics: make(map[string]uint8)}
}

func (r *BytecodeIntrinsicRegistry) Get(name string) (uint8, bool) {
	v, ok := r.intrinsics[name]
	return v, ok
}

func (r *BytecodeIntrinsicRegistry) Set(name string, opcode uint8) {
	r.intrinsics[name] = opcode
}
