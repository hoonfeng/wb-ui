// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ObjectPropertyCondition.h

package bytecode

// ObjectPropertyCondition is a PropertyCondition bound to a specific object.
type ObjectPropertyCondition struct {
	PropertyCondition
	object uint64
}

func NewObjectPropertyCondition(cond PropertyCondition, object uint64) ObjectPropertyCondition {
	return ObjectPropertyCondition{PropertyCondition: cond, object: object}
}

func (c ObjectPropertyCondition) Object() uint64 { return c.object }
