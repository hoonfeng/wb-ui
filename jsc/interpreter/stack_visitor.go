// Copyright (C) 2013-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/interpreter/StackVisitor.h

package interpreter

import (
	"wb-ui/jsc/bytecode"
	"wb-ui/jsc/runtime"
)

// StackVisitor traverses the call stack for debugging/inspection.
type StackVisitor struct {
	currentFrame *CallFrame
	frameCount   int
}

func NewStackVisitor(startFrame *CallFrame) *StackVisitor {
	return &StackVisitor{
		currentFrame: startFrame,
		frameCount:   0,
	}
}

func (v *StackVisitor) HasMoreFrames() bool {
	return v.currentFrame != nil
}

func (v *StackVisitor) Next() {
	v.currentFrame = v.currentFrame.GetCallerFrame()
	v.frameCount++
}

func (v *StackVisitor) Frame() *CallFrame {
	return v.currentFrame
}

func (v *StackVisitor) FrameCount() int { return v.frameCount }

func (v *StackVisitor) SourceURL() string {
	if v.currentFrame != nil && v.currentFrame.GetCodeBlock() != nil {
		return v.currentFrame.GetCodeBlock().SourceURL()
	}
	return ""
}

func (v *StackVisitor) Line() int {
	// Simplified - no source mapping yet
	return 0
}

func (v *StackVisitor) Column() int {
	return 0
}

// CalleeBits stores the callee function reference.
type CalleeBits struct {
	bits uint64
}

func CalleeBitsFromCell(cell *runtime.JSCell) CalleeBits {
	return CalleeBits{bits: uint64(uintptr(interface{}(cell).(uint64)))}
}

// ShadowChicken is a debugging tool for tracking function calls.
type ShadowChicken struct {
	log []ShadowChickenLogEntry
}

type ShadowChickenLogEntry struct {
	Frame  *CallFrame
	Callee *runtime.JSCell
}

func NewShadowChicken() *ShadowChicken {
	return &ShadowChicken{log: make([]ShadowChickenLogEntry, 0)}
}

func (s *ShadowChicken) Append(entry ShadowChickenLogEntry) {
	s.log = append(s.log, entry)
}

func (s *ShadowChicken) Clear() { s.log = s.log[:0] }
func (s *ShadowChicken) Size() int { return len(s.log) }

// FrameTracers are for profiling (stub in interpreter mode).
type FrameTracer struct{}
func NewFrameTracer() *FrameTracer { return &FrameTracer{} }
func (t *FrameTracer) Begin() {}
func (t *FrameTracer) End() {}

// CheckpointOSRExitSideState holds state for OSR exit from checkpoints.
type CheckpointOSRExitSideState struct {
	bytecodeIndex bytecode.BytecodeIndex
	values        []runtime.JSValue
}

// CachedCall caches a function + arguments for fast calls.
type CachedCall struct {
	codeBlock *bytecode.CodeBlock
	scope     *runtime.JSScope
	callFrame *CallFrame
}

func NewCachedCall() *CachedCall {
	return &CachedCall{callFrame: NewCallFrame()}
}

// ProtoCallFrame is a simplified call frame used for initial entry.
type ProtoCallFrame struct {
	codeBlock  *bytecode.CodeBlock
	thisValue  runtime.JSValue
	argc       int32
	argv       []runtime.JSValue
}

func NewProtoCallFrame() *ProtoCallFrame {
	return &ProtoCallFrame{argv: make([]runtime.JSValue, 0)}
}
