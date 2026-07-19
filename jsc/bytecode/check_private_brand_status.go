// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CheckPrivateBrandStatus.h

package bytecode

// CheckPrivateBrandStatus describes the state of a private brand check IC.
type CheckPrivateBrandStatus struct {
	state uint8 // 0=unset, 1=monomorphic, 2=polymorphic, 3=megamorphic
}

func NewCheckPrivateBrandStatus() *CheckPrivateBrandStatus { return &CheckPrivateBrandStatus{} }
func (s *CheckPrivateBrandStatus) IsSet() bool              { return s.state != 0 }
func (s *CheckPrivateBrandStatus) State() uint8             { return s.state }
func (s *CheckPrivateBrandStatus) SetState(v uint8)         { s.state = v }
