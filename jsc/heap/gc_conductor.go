// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type GCConductor uint8

const (
	GCConductorMutator  GCConductor = iota
	GCConductorCollector
)

func GCConductorShortName(officer GCConductor) string {
	switch officer {
	case GCConductorMutator:
		return "M"
	case GCConductorCollector:
		return "C"
	default:
		return "?"
	}
}
