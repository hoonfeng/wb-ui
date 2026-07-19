// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CallLinkStatus.h

package bytecode

// CallLinkStatus describes the state of monomorphic / polymorphic / megamorphic inline caches for calls.
type CallLinkStatus struct {
	variants []CallVariant
	isProved bool
}

func NewCallLinkStatus() *CallLinkStatus { return &CallLinkStatus{} }

func (s *CallLinkStatus) Variants() []CallVariant { return s.variants }
func (s *CallLinkStatus) IsProved() bool           { return s.isProved }
func (s *CallLinkStatus) SetIsProved(v bool)       { s.isProved = v }
func (s *CallLinkStatus) IsSet() bool              { return len(s.variants) > 0 }
func (s *CallLinkStatus) Size() int                { return len(s.variants) }
