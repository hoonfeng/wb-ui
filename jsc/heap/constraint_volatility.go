// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type ConstraintVolatility uint8

const (
	ConstraintVolatilityGreyedByExecution ConstraintVolatility = iota
	ConstraintVolatilityGreyedByMarking
	ConstraintVolatilitySeldomGreyed
)
