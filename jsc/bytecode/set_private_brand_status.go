// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/SetPrivateBrandStatus.h

package bytecode

// SetPrivateBrandStatus describes the state of a private brand set IC.
type SetPrivateBrandStatus struct {
	state uint8
}

func NewSetPrivateBrandStatus() *SetPrivateBrandStatus { return &SetPrivateBrandStatus{} }
func (s *SetPrivateBrandStatus) IsSet() bool            { return s.state != 0 }
func (s *SetPrivateBrandStatus) State() uint8           { return s.state }
func (s *SetPrivateBrandStatus) SetState(v uint8)       { s.state = v }
