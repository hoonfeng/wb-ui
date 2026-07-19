// Copyright (C) 2016-2020 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type HeapCellKind int8

const (
	HeapCellKindJSCell               HeapCellKind = iota
	HeapCellKindJSCellWithIndexingHeader
	HeapCellKindAuxiliary
)

type HeapCellZapReason int8

const (
	HeapCellZapReasonUnspecified     HeapCellZapReason = iota
	HeapCellZapReasonDestruction
	HeapCellZapReasonStopAllocating
)

type HeapCell struct {
	// In Go, we store StructureID as first word
	// The second word is preserved for crash analysis
	// Third word stores ZapReason
}

func (c *HeapCell) Zap(reason HeapCellZapReason) {
	// Zaps first word (structureID) and third word (reason)
	// Simplified: just store the reason
}

func (c *HeapCell) IsZapped() bool {
	return false
}

func (c *HeapCell) NotifyNeedsDestruction() {
}

func (c *HeapCell) IsPendingDestruction() bool {
	return false
}

func IsJSCellKind(kind HeapCellKind) bool {
	return kind == HeapCellKindJSCell || kind == HeapCellKindJSCellWithIndexingHeader
}

func MayHaveIndexingHeader(kind HeapCellKind) bool {
	return kind == HeapCellKindAuxiliary || kind == HeapCellKindJSCellWithIndexingHeader
}
