// Copyright (C) 2016-2024 Apple Inc. All rights reserved.
// Translated to Go.
package heap

import "fmt"

type CellAttributes struct {
	Destruction DestructionMode
	CellKind    HeapCellKind
}

func NewCellAttributes(destruction DestructionMode, cellKind HeapCellKind) CellAttributes {
	return CellAttributes{Destruction: destruction, CellKind: cellKind}
}

func (a CellAttributes) String() string {
	return fmt.Sprintf("CellAttributes(%v, %v)", a.Destruction, a.CellKind)
}
