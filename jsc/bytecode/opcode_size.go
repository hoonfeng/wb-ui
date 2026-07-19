// Copyright (C) 2018 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/OpcodeSize.h

package bytecode

import "math"

type OpcodeSize uint8

const (
	Narrow OpcodeSize = 1
	Wide16 OpcodeSize = 2
	Wide32 OpcodeSize = 4
)

// TypeBySize maps OpcodeSize to signed/unsigned types.
// In Go, we use fixed-size integer types.

func SignedTypeForSize(s OpcodeSize) interface{} {
	switch s {
	case Narrow:
		return int8(0)
	case Wide16:
		return int16(0)
	case Wide32:
		return int32(0)
	default:
		panic("unknown OpcodeSize")
	}
}

func UnsignedTypeForSize(s OpcodeSize) interface{} {
	switch s {
	case Narrow:
		return uint8(0)
	case Wide16:
		return uint16(0)
	case Wide32:
		return uint32(0)
	default:
		panic("unknown OpcodeSize")
	}
}

// PaddingBySize provides padding bytes for each opcode size.
func PaddingForSize(s OpcodeSize) uint8 {
	switch s {
	case Narrow:
		return 0
	case Wide16:
		return 1
	case Wide32:
		return 1
	default:
		panic("unknown OpcodeSize")
	}
}

// OpcodeIDWidthBySize determines the opcode type width for a given size.
type OpcodeIDWidthBySize struct {
	OpcodeTypeSize int
	OpcodeIDSize   OpcodeSize
}

func GetOpcodeIDWidthBySize(traits interface{}, s OpcodeSize) OpcodeIDWidthBySize {
	// maxOpcodeIDWidth defaults to Narrow for the interpreter
	// In Go, we simplify this - for narrow use uint8, for wide use uint16
	var useNarrow bool
	if t, ok := traits.(interface{ MaxOpcodeIDWidth() OpcodeSize }); ok {
		useNarrow = t.MaxOpcodeIDWidth() == Narrow
	} else {
		useNarrow = true
	}

	opcodeTypeSize := 1
	if s == Wide16 || s == Wide32 {
		if useNarrow {
			opcodeTypeSize = 1 // uint8
		} else {
			opcodeTypeSize = 2 // uint16
		}
	}

	return OpcodeIDWidthBySize{
		OpcodeTypeSize: opcodeTypeSize,
		OpcodeIDSize:   OpcodeSize(opcodeTypeSize),
	}
}

// BitWidthForMaxBytecodeStructLength computes the bit width for the max structure length.
func BitWidthForMaxBytecodeStructLength(maxLength uint32) uint {
	if maxLength == 0 {
		return 0
	}
	return uint(math.Log2(float64(maxLength))) + 1
}
