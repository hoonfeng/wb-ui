// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type ConstraintConcurrency uint8

const (
	ConstraintConcurrencySequential ConstraintConcurrency = iota
	ConstraintConcurrencyConcurrent
)
