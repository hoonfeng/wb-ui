// Copyright (C) 2018 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/Instruction.h

package bytecode

// Instruction represents a single bytecode instruction in the instruction stream.
// In WebKit, this is a single byte that's reinterpreted based on width prefix.
// In Go, we use a simplified model where an Instruction is a view into a byte slice.
type Instruction struct {
	opcode OpcodeID
	width  OpcodeSize
	data   []byte
}

// JSInstruction is the standard instruction type for JavaScript opcodes.
type JSInstruction = Instruction

// JSOpcodeTraits provides trait information for JavaScript opcodes.
type JSOpcodeTraits struct{}

func (JSOpcodeTraits) MaxOpcodeIDWidth() OpcodeSize { return Narrow }

// DecodeInstruction decodes a single instruction from a byte slice at the given offset.
// Returns the decoded instruction and the byte size consumed.
func DecodeInstruction(code []byte, offset uint32) (Instruction, uint32) {
	if offset >= uint32(len(code)) {
		return Instruction{opcode: OpNop, width: Narrow, data: nil}, 0
	}

	opcode := OpcodeID(code[offset])

	// Check for wide prefix
	var width OpcodeSize = Narrow
	var opcodeOffset uint32 = 0
	var sizeShift uint32 = 0

	if opcode == OpWide16 {
		width = Wide16
		opcodeOffset = 1
		sizeShift = 1
	} else if opcode == OpWide32 {
		width = Wide32
		opcodeOffset = 1
		sizeShift = 2
	}

	// Read the actual opcode
	var actualOpcode OpcodeID
	if width == Narrow {
		actualOpcode = opcode
	} else {
		if offset+opcodeOffset < uint32(len(code)) {
			actualOpcode = OpcodeID(code[offset+opcodeOffset])
		}
	}

	// Calculate total size
	length := uint32(opcodeLengths[actualOpcode])
	if length == 0 {
		length = 2 // minimum
	}
	totalSize := length << sizeShift
	if width != Narrow {
		totalSize += 2 // prefix byte + opcode byte
	}

	end := offset + totalSize
	if end > uint32(len(code)) {
		end = uint32(len(code))
	}

	return Instruction{
		opcode: actualOpcode,
		width:  width,
		data:   code[offset:end],
	}, totalSize
}

func (inst Instruction) OpcodeID() OpcodeID {
	return inst.opcode
}

func (inst Instruction) Name() string {
	return OpcodeName(inst.opcode)
}

func (inst Instruction) Width() OpcodeSize {
	return inst.width
}

func (inst Instruction) Size() int {
	return len(inst.data)
}

func (inst Instruction) HasMetadata() bool {
	return inst.opcode < OpIteratorOpen // approximate: opcodes with metadata
}

func (inst Instruction) HasCheckpoints() bool {
	return inst.opcode < OpCall // approximate: opcodes with checkpoints
}

// ReadOperand reads a single integer operand from the instruction at the given operand index.
func (inst Instruction) ReadOperand(index int) int32 {
	// Skip opcode byte(s)
	var offset int
	if inst.width == Narrow {
		offset = 1 // opcode byte
	} else {
		offset = 2 // prefix + opcode byte
	}

	operandOffset := offset + index*4
	if operandOffset+4 > len(inst.data) {
		return 0
	}

	// Read int32 (little-endian)
	return int32(inst.data[operandOffset]) |
		int32(inst.data[operandOffset+1])<<8 |
		int32(inst.data[operandOffset+2])<<16 |
		int32(inst.data[operandOffset+3])<<24
}

func (inst Instruction) String() string {
	return OpcodeName(inst.opcode)
}
