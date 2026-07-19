// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/VariableWriteFireDetail.h

package bytecode

// VariableWriteFireDetail provides details about a variable write that fires a watchpoint.
type VariableWriteFireDetail struct {
	name string
}

func NewVariableWriteFireDetail(name string) *VariableWriteFireDetail {
	return &VariableWriteFireDetail{name: name}
}

func (d *VariableWriteFireDetail) Name() string { return d.name }
