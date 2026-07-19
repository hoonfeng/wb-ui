// Copyright (C) 2013 Apple Inc. All rights reserved.
// Translated to Go.
//
// TriState corresponds to WTF::TriState.

package runtime

// TriState corresponds to WTF::TriState, a boolean with indeterminate state.
type TriState uint8

const (
	TriStateFalse        TriState = 0
	TriStateTrue         TriState = 1
	TriStateIndeterminate TriState = 2
)

// triState converts a bool to TriState.
func triState(b bool) TriState {
	if b {
		return TriStateTrue
	}
	return TriStateFalse
}
