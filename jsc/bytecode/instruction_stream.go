// Copyright (C) 2018 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InstructionStream.h

package bytecode

// InstructionStream represents a stream of bytecode instructions, stored as a byte buffer.
// This is a simplified Go version of WebKit's InstructionStream.
type InstructionStream struct {
	code []byte
}

// Ref is a reference to a specific instruction in the stream.
type Ref struct {
	stream *InstructionStream
	offset uint32
}

func NewInstructionStream(code []byte) *InstructionStream {
	return &InstructionStream{code: code}
}

func (s *InstructionStream) SizeInBytes() int {
	return len(s.code)
}

func (s *InstructionStream) Size() int {
	return len(s.code)
}

func (s *InstructionStream) At(offset uint32) Ref {
	return Ref{stream: s, offset: offset}
}

func (s *InstructionStream) Begin() Ref {
	return Ref{stream: s, offset: 0}
}

func (s *InstructionStream) End() Ref {
	return Ref{stream: s, offset: uint32(len(s.code))}
}

func (s *InstructionStream) RawPointer() []byte {
	return s.code
}

func (r Ref) Instruction() (Instruction, uint32) {
	return DecodeInstruction(r.stream.code, r.offset)
}

func (r Ref) Next() Ref {
	_, size := DecodeInstruction(r.stream.code, r.offset)
	return Ref{stream: r.stream, offset: r.offset + size}
}

func (r Ref) Offset() uint32 {
	return r.offset
}

func (r Ref) Index() BytecodeIndex {
	return NewBytecodeIndex(r.offset, 0)
}

func (r Ref) IsValid() bool {
	return r.offset < uint32(len(r.stream.code))
}

func (r Ref) Equals(other Ref) bool {
	return r.stream == other.stream && r.offset == other.offset
}

// MutableRef is a mutable reference to an instruction in the stream (used for writing).
type MutableRef struct {
	stream *InstructionStream
	offset uint32
}

func NewMutableRef(stream *InstructionStream, offset uint32) MutableRef {
	return MutableRef{stream: stream, offset: offset}
}

func (r MutableRef) Freeze() Ref {
	return Ref{stream: r.stream, offset: r.offset}
}

func (r MutableRef) Offset() uint32 {
	return r.offset
}

func (r MutableRef) Instruction() (Instruction, uint32) {
	return DecodeInstruction(r.stream.code, r.offset)
}

// InstructionStreamWriter is used to write bytecode instructions.
type InstructionStreamWriter struct {
	code       []byte
	position   uint32
	finalized  bool
}

func NewInstructionStreamWriter() *InstructionStreamWriter {
	return &InstructionStreamWriter{
		code: make([]byte, 0, 1024),
	}
}

func (w *InstructionStreamWriter) Write(bytes ...byte) {
	if w.finalized {
		panic("InstructionStreamWriter: already finalized")
	}
	w.code = append(w.code, bytes...)
	w.position = uint32(len(w.code))
}

func (w *InstructionStreamWriter) WriteUint32(val uint32) {
	w.Write(byte(val), byte(val>>8), byte(val>>16), byte(val>>24))
}

func (w *InstructionStreamWriter) Position() uint32 {
	return w.position
}

func (w *InstructionStreamWriter) Seek(pos uint32) {
	if pos > uint32(len(w.code)) {
		// Extend buffer if needed
		needed := make([]byte, pos-uint32(len(w.code)))
		w.code = append(w.code, needed...)
	}
	w.position = pos
}

func (w *InstructionStreamWriter) Ref() MutableRef {
	return MutableRef{stream: &InstructionStream{code: w.code}, offset: w.position}
}

func (w *InstructionStreamWriter) Finalize() *InstructionStream {
	w.finalized = true
	result := make([]byte, len(w.code))
	copy(result, w.code)
	return &InstructionStream{code: result}
}

// JSInstructionStream is the standard instruction stream for JavaScript.
var JSInstructionStream = &InstructionStream{}
