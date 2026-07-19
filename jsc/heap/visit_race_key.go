// Copyright (C) 2021-2026 Apple Inc. All rights reserved.
// Translated to Go.
package heap

import "fmt"

type VisitRaceKey struct {
	A uintptr
	B uintptr
}

func NewVisitRaceKey(a, b uintptr) VisitRaceKey {
	if a < b {
		return VisitRaceKey{A: a, B: b}
	}
	return VisitRaceKey{A: b, B: a}
}

func (k VisitRaceKey) String() string {
	return fmt.Sprintf("VisitRaceKey(%x, %x)", k.A, k.B)
}
