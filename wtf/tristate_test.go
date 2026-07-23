package wtf

import "testing"

func TestTriStateFromBool(t *testing.T) {
	if got := TriStateFromBool(true); got != TriStateTrue {
		t.Fatalf("TriStateFromBool(true) = %v, want True", got)
	}
	if got := TriStateFromBool(false); got != TriStateFalse {
		t.Fatalf("TriStateFromBool(false) = %v, want False", got)
	}
}

func TestInvert(t *testing.T) {
	tests := []struct {
		input TriState
		want  TriState
	}{
		{TriStateTrue, TriStateFalse},
		{TriStateFalse, TriStateTrue},
		{TriStateIndeterminate, TriStateIndeterminate},
	}
	for _, tc := range tests {
		if got := Invert(tc.input); got != tc.want {
			t.Errorf("Invert(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestTriStateString(t *testing.T) {
	if got, want := TriStateTrue.String(), "true"; got != want {
		t.Fatalf("String = %q, want %q", got, want)
	}
	if got, want := TriStateFalse.String(), "false"; got != want {
		t.Fatalf("String = %q, want %q", got, want)
	}
	if got, want := TriStateIndeterminate.String(), "indeterminate"; got != want {
		t.Fatalf("String = %q, want %q", got, want)
	}
}
