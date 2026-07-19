// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/InByStatus.h

package bytecode

// InByStatus describes the state of a property-"in" inline cache.
type InByStatus struct {
	state uint8
}

func NewInByStatus() *InByStatus            { return &InByStatus{} }
func (s *InByStatus) IsSet() bool           { return s.state != 0 }
func (s *InByStatus) State() uint8           { return s.state }
func (s *InByStatus) SetState(v uint8)      { s.state = v }
