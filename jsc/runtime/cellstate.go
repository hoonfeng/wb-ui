// Translation of: Source/JavaScriptCore/heap/CellState.h
//
// CellState tracks GC marking state for each JSCell.

package runtime

// CellState corresponds to JSC::CellState. It encodes the GC marking progress of a cell.
type CellState uint8

const (
	// PossiblyBlack: object is either being scanned or finished scanning.
	// During full collection, a white object would have its mark bit clear.
	PossiblyBlack CellState = 0

	// DefinitelyWhite: object is in eden and has not been marked yet.
	DefinitelyWhite CellState = 1

	// PossiblyGrey: object will be scanned (grey). During full collection,
	// it could be white if mark bit is clear.
	PossiblyGrey CellState = 2
)

const (
	BlackThreshold        = 0  // x <= BlackThreshold means x is PossiblyOldOrBlack
	TautologicalThreshold = 100 // x <= TautologicalThreshold is always true
)

// IsWithinThreshold checks if cellState <= threshold.
func IsWithinThreshold(state CellState, threshold uint) bool {
	return uint(state) <= threshold
}
