// Copyright (C) 2016 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type DestructionMode uint8

const (
	DestructionModeDoesNotNeedDestruction DestructionMode = iota
	DestructionModeNeedsDestruction
	DestructionModeMayNeedDestruction
)
