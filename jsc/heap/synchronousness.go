// Copyright (C) 2017 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type Synchronousness uint8

const (
	SynchronousnessAsync Synchronousness = iota
	SynchronousnessSync
)
