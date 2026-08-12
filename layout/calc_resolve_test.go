package layout

import (
	"testing"

	"wb-ui/style"
)

// TestResolveLengthCalc verifies the layout-time re-evaluation of a deferred
// calc() Length: resolveLength must resolve the raw expression against the
// containing-block reference (reference → %, fontSize → em).
func TestResolveLengthCalc(t *testing.T) {
	l := style.Length{Unit: "calc", CalcExpr: "100% - 40px"}
	r := resolveLength(l, 500, 16)
	if !r.Definite {
		t.Fatalf("resolveLength(calc 100%% - 40px, ref=500) not definite")
	}
	if r.Value != 460 {
		t.Fatalf("resolveLength(calc 100%% - 40px, ref=500) = %v, want 460", r.Value)
	}
}

// TestResolveLengthCalcEm verifies em units inside calc resolve against fontSize.
func TestResolveLengthCalcEm(t *testing.T) {
	l := style.Length{Unit: "calc", CalcExpr: "2em + 10px"}
	r := resolveLength(l, 0, 16)
	if !r.Definite || r.Value != 42 {
		t.Fatalf("resolveLength(calc 2em + 10px, fs=16) = %v (definite=%v), want 42", r.Value, r.Definite)
	}
}

// TestResolveLengthCalcEmpty verifies a calc Length without expression is
// treated as indefinite (not silently 0).
func TestResolveLengthCalcEmpty(t *testing.T) {
	l := style.Length{Unit: "calc"}
	r := resolveLength(l, 500, 16)
	if r.Definite {
		t.Fatalf("resolveLength(calc, no expr) = definite %v, want indefinite", r.Value)
	}
}
