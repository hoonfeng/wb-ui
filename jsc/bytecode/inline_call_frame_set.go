// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InlineCallFrameSet.h

package bytecode

// InlineCallFrameSet is a set of InlineCallFrames for DFG inlining.
type InlineCallFrameSet struct {
	frames []InlineCallFrame
}

func NewInlineCallFrameSet() *InlineCallFrameSet {
	return &InlineCallFrameSet{}
}

func (s *InlineCallFrameSet) Add(frame InlineCallFrame) {
	s.frames = append(s.frames, frame)
}

func (s *InlineCallFrameSet) Size() int                           { return len(s.frames) }
func (s *InlineCallFrameSet) At(i int) InlineCallFrame            { return s.frames[i] }
func (s *InlineCallFrameSet) IsEmpty() bool                       { return len(s.frames) == 0 }
