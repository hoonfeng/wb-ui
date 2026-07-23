// Translation of: Source/WTF/wtf/TriState.h
// Completeness: 100%
// Simplifications: None; the enum is as simple as it looks.

package wtf

// TriState mirrors WTF::TriState: a three-state boolean used extensively by
// CSS/Style resolution and DOM query operations where the answer may be
// "true", "false", or "unknown" (Indeterminate).
//
// Construct helper values with TriStateTrue / TriStateFalse / TriStateIndeterminate.
// Use TriStateFromBool to lift a Go bool, and Invert to flip True↔False.
type TriState uint8

const (
	TriStateFalse        TriState = 0
	TriStateTrue         TriState = 1
	TriStateIndeterminate TriState = 2
)

// TriStateFromBool converts a Go bool to TriState (true→True, false→False).
// This mirrors WTF::triState(bool).
func TriStateFromBool(b bool) TriState {
	if b {
		return TriStateTrue
	}
	return TriStateFalse
}

// Invert returns the logical negation of ts:
//   True→False, False→True, Indeterminate→Indeterminate.
// This mirrors WTF::invert(TriState).
func Invert(ts TriState) TriState {
	switch ts {
	case TriStateTrue:
		return TriStateFalse
	case TriStateFalse:
		return TriStateTrue
	default:
		return TriStateIndeterminate
	}
}

// String returns "true", "false", or "indeterminate".
func (ts TriState) String() string {
	switch ts {
	case TriStateTrue:
		return "true"
	case TriStateFalse:
		return "false"
	default:
		return "indeterminate"
	}
}
