package layout

import (
	"testing"

	"wb-ui/engine/style"
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

// TestParseCSSLengthCalcRelative verifies parseCSSLength defers a calc() with
// relative units (%、em、vw…) to the "calc" Length form, instead of treating it
// as auto (which previously swallowed positioned top/left calc()).
func TestParseCSSLengthCalcRelative(t *testing.T) {
	l := parseCSSLength("calc(100% - 40px)")
	if l.Unit != "calc" {
		t.Fatalf("parseCSSLength(calc(100%% - 40px)).Unit = %q, want %q", l.Unit, "calc")
	}
	if l.CalcExpr != "100% - 40px" {
		t.Fatalf("parseCSSLength(calc(100%% - 40px)).CalcExpr = %q, want %q", l.CalcExpr, "100% - 40px")
	}
}

// TestParseCSSLengthCalcAbsolute verifies a pure-absolute calc() resolves
// immediately to px.
func TestParseCSSLengthCalcAbsolute(t *testing.T) {
	l := parseCSSLength("calc(40px + 8px)")
	if l.Unit != "px" || l.Value != 48 {
		t.Fatalf("parseCSSLength(calc(40px + 8px)) = {%v %q}, want {48 px}", l.Value, l.Unit)
	}
}

// TestResolveOffsetCalc verifies the end-to-end positioned inset path:
// asLength → resolveOffset → resolveLength re-evaluates calc() with the
// containing-block size as the % reference.
func TestResolveOffsetCalc(t *testing.T) {
	// left: calc(100% - 40px) against a 300px containing block → 260px.
	v, auto := resolveOffset(asLength("calc(100% - 40px)"), 300)
	if auto {
		t.Fatalf("resolveOffset(calc left, cb=300) reported auto, want definite")
	}
	if v != 260 {
		t.Fatalf("resolveOffset(calc left, cb=300) = %v, want 260", v)
	}
}

// TestParseCSSLengthMinMaxClamp verifies parseCSSLength treats min()/max()/
// clamp() like calc(): relative units defer, absolute resolve eagerly.
func TestParseCSSLengthMinMaxClamp(t *testing.T) {
	l := parseCSSLength("min(100%, 600px)")
	if l.Unit != "calc" || l.CalcExpr != "min(100%, 600px)" {
		t.Fatalf("parseCSSLength(min(100%%, 600px)) = {%q, %q}, want {calc, min(100%%, 600px)}", l.Unit, l.CalcExpr)
	}
	l = parseCSSLength("max(40px, 10px)")
	if l.Unit != "px" || l.Value != 40 {
		t.Fatalf("parseCSSLength(max(40px, 10px)) = {%v %q}, want {40 px}", l.Value, l.Unit)
	}
}

// TestResolveOffsetClamp verifies end-to-end positioned inset with clamp().
func TestResolveOffsetClamp(t *testing.T) {
	v, auto := resolveOffset(asLength("clamp(0px, 100px, 50px)"), 300)
	if auto {
		t.Fatalf("resolveOffset(clamp top) reported auto, want definite")
	}
	if v != 50 {
		t.Fatalf("resolveOffset(clamp(0px, 100px, 50px)) = %v, want 50", v)
	}
}
