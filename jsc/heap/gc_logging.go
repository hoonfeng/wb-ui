// Copyright (C) 2014, 2015 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type GCLoggingLevel uint8

const (
	GCLoggingLevelNone    GCLoggingLevel = 0
	GCLoggingLevelBasic   GCLoggingLevel = iota
	GCLoggingLevelVerbose
)

type GCLogging struct{}

func (GCLogging) DumpObjectGraph(heap *Heap) {
	// Stub: object graph dumping
}
