// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type ConstraintParallelism uint8

const (
	ConstraintParallelismSequential ConstraintParallelism = iota
	ConstraintParallelismParallel
)
