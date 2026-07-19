// Copyright (C) 2016-2022 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type MutatorState uint8

const (
	MutatorStateRunning    MutatorState = iota
	MutatorStateAllocating
	MutatorStateSweeping
	MutatorStateCollecting
)
