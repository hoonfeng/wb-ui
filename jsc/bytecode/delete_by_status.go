// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/DeleteByStatus.h

package bytecode

// DeleteByStatus describes the state of a property-delete inline cache.
type DeleteByStatus struct {
	state uint8
}

func NewDeleteByStatus() *DeleteByStatus       { return &DeleteByStatus{} }
func (s *DeleteByStatus) IsSet() bool           { return s.state != 0 }
func (s *DeleteByStatus) State() uint8           { return s.state }
func (s *DeleteByStatus) SetState(v uint8)      { s.state = v }
