// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/PutByStatus.h

package bytecode

// PutByStatus describes the state of a property-put inline cache.
type PutByStatus struct {
	state uint8
}

func NewPutByStatus() *PutByStatus          { return &PutByStatus{} }
func (s *PutByStatus) IsSet() bool          { return s.state != 0 }
func (s *PutByStatus) State() uint8          { return s.state }
func (s *PutByStatus) SetState(v uint8)     { s.state = v }
