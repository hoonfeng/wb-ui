// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/Watchpoint.h

package bytecode

// Watchpoint is a JIT optimization primitive: when a condition becomes invalid,
// all code that speculated on it is invalidated.
type Watchpoint struct {
	state uint8
}

const (
	WatchpointStateIsWatched     uint8 = 0
	WatchpointStateIsInvalidated uint8 = 1
)

func NewWatchpoint() *Watchpoint {
	return &Watchpoint{state: WatchpointStateIsWatched}
}

func (w *Watchpoint) IsStillValid() bool     { return w.state == WatchpointStateIsWatched }
func (w *Watchpoint) Invalidate()            { w.state = WatchpointStateIsInvalidated }
func (w *Watchpoint) State() uint8           { return w.state }

// InlineWatchpointSet is a set of watchpoints inlined in objects.
type InlineWatchpointSet struct {
	state uint8
	data  uint64
}

func NewInlineWatchpointSet() *InlineWatchpointSet {
	return &InlineWatchpointSet{state: WatchpointStateIsWatched}
}

func (s *InlineWatchpointSet) IsStillValid() bool { return s.state == WatchpointStateIsWatched }
func (s *InlineWatchpointSet) Invalidate()        { s.state = WatchpointStateIsInvalidated }
func (s *InlineWatchpointSet) State() uint8       { return s.state }
