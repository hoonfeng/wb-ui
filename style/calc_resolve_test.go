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
