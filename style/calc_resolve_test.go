package style

import "testing"

// TestParseLengthCalcRelative verifies the calc() regression fix: a calc()
// with relative units (%, em, ...) must NOT be resolved with a zero context
// (which turned calc(100% - 40px) into -40px). It must be deferred as a
// "calc" Length carrying the raw expression for later layout-time evaluation.
func TestParseLengthCalcRelative(t *testing.T) {
	l, ok := parseLength("calc(100% - 40px)")
	if !ok {
		t.Fatalf("parseLength(calc(100%% - 40px)) not ok")
	}
	if l.Unit != "calc" {
		t.Fatalf("parseLength(calc(100%% - 40px)).Unit = %q, want %q (value=%v)", l.Unit, "calc", l.Value)
	}
	if l.CalcExpr != "100% - 40px" {
		t.Fatalf("parseLength(calc(100%% - 40px)).CalcExpr = %q, want %q", l.CalcExpr, "100% - 40px")
	}
}

// TestParseLengthCalcAbsolute verifies pure-absolute-unit calc() still resolves
// eagerly to a px Length.
func TestParseLengthCalcAbsolute(t *testing.T) {
	l, ok := parseLength("calc(40px + 8px)")
	if !ok {
		t.Fatalf("parseLength(calc(40px + 8px)) not ok")
	}
	if l.Unit != "px" || l.Value != 48 {
		t.Fatalf("parseLength(calc(40px + 8px)) = %v%s, want 48px", l.Value, l.Unit)
	}
}

// TestParseLengthMinMaxClamp verifies min()/max()/clamp() are recognized as
// math functions: relative-unit ones defer as "calc" Length, absolute ones
// resolve eagerly.
func TestParseLengthMinMaxClamp(t *testing.T) {
	// Relative: min(100%, 600px) → deferred, CalcExpr keeps full expression.
	l, ok := parseLength("min(100%, 600px)")
	if !ok {
		t.Fatalf("parseLength(min(100%%, 600px)) not ok")
	}
	if l.Unit != "calc" || l.CalcExpr != "min(100%, 600px)" {
		t.Fatalf("parseLength(min(100%%, 600px)) = {%q, %q}, want {calc, min(100%%, 600px)}", l.Unit, l.CalcExpr)
	}
	// Absolute: max(40px, 10px) → eager px.
	l, ok = parseLength("max(40px, 10px)")
	if !ok || l.Unit != "px" || l.Value != 40 {
		t.Fatalf("parseLength(max(40px, 10px)) = %v%s ok=%v, want 40px", l.Value, l.Unit, ok)
	}
	// clamp absolute.
	l, ok = parseLength("clamp(10px, 30px, 20px)")
	if !ok || l.Value != 20 {
		t.Fatalf("parseLength(clamp(10px, 30px, 20px)) = %v%s ok=%v, want 20px", l.Value, l.Unit, ok)
	}
}
