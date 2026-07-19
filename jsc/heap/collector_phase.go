// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type CollectorPhase uint8

const (
	CollectorPhaseNotRunning CollectorPhase = iota
	CollectorPhaseBegin
	CollectorPhaseFixpoint
	CollectorPhaseConcurrent
	CollectorPhaseReloop
	CollectorPhaseEnd
)

func WorldShouldBeSuspended(phase CollectorPhase) bool {
	switch phase {
	case CollectorPhaseBegin, CollectorPhaseFixpoint, CollectorPhaseReloop, CollectorPhaseEnd:
		return true
	default:
		return false
	}
}
