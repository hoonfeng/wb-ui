// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/RecordedStatuses.h

package bytecode

// RecordedStatuses collects all IC status changes made during DFG compilation.
type RecordedStatuses struct{}

func NewRecordedStatuses() *RecordedStatuses { return &RecordedStatuses{} }
