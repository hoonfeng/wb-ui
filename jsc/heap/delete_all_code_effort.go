// Copyright (C) 2013 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type DeleteAllCodeEffort uint8

const (
	DeleteAllCodeEffortDeleteAll DeleteAllCodeEffort = iota
	DeleteAllCodeEffortDeleteAllPreemptively
)
