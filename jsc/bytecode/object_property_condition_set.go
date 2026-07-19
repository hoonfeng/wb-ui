// Copyright (C) 2012-2019 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ObjectPropertyConditionSet.h

package bytecode

// ObjectPropertyConditionSet is a set of ObjectPropertyConditions for a speculation.
type ObjectPropertyConditionSet struct {
	conditions []ObjectPropertyCondition
}

func NewObjectPropertyConditionSet() *ObjectPropertyConditionSet {
	return &ObjectPropertyConditionSet{}
}

func (s *ObjectPropertyConditionSet) IsEmpty() bool       { return len(s.conditions) == 0 }
func (s *ObjectPropertyConditionSet) Size() int           { return len(s.conditions) }
func (s *ObjectPropertyConditionSet) Add(c ObjectPropertyCondition) {
	s.conditions = append(s.conditions, c)
}
func (s *ObjectPropertyConditionSet) At(i int) ObjectPropertyCondition { return s.conditions[i] }
