// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type AllocationFailureMode uint8

const (
	AllocationFailureModeAssert    AllocationFailureMode = iota
	AllocationFailureModeReturnNull
)
