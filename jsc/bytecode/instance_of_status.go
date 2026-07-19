// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InstanceOfStatus.h

package bytecode

// InstanceOfStatus describes the state of an instanceof inline cache.
type InstanceOfStatus struct {
	state uint8
}

func NewInstanceOfStatus() *InstanceOfStatus     { return &InstanceOfStatus{} }
func (s *InstanceOfStatus) IsSet() bool           { return s.state != 0 }
func (s *InstanceOfStatus) State() uint8           { return s.state }
func (s *InstanceOfStatus) SetState(v uint8)      { s.state = v }
