// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/GetByStatus.h

package bytecode

// GetByStatus describes the state of a property-get inline cache.
type GetByStatus struct {
	state uint8
}

func NewGetByStatus() *GetByStatus          { return &GetByStatus{} }
func (s *GetByStatus) IsSet() bool          { return s.state != 0 }
func (s *GetByStatus) State() uint8          { return s.state }
func (s *GetByStatus) SetState(v uint8)     { s.state = v }
