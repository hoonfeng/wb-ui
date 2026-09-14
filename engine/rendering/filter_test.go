// Tests for engine/rendering/filter.go — CSS filter function parsing and Skia chain building.
// Completeness: 80%
//   - parseCSSFilters is unit-tested for all 9 filter functions, including edge cases
//     (empty, none, invalid, multiple combined, percentage/decimal/deg/px units)
//   - buildCSSFilterChain is smoke-tested (returns non-nil chain for known filters)

package rendering

import (
	"testing"
)

// --- parseCSSFilters tests --------------------------------------------------

func TestParseCSSFilters_Empty(t *testing.T) {
	if got := parseCSSFilters(""); got != nil {
		t.Fatalf("empty string: got %v, want nil", got)
	}
	if got := parseCSSFilters("none"); got != nil {
		t.Fatalf("\"none\": got %v, want nil", got)
	}
}

func TestParseCSSFilters_Blur(t *testing.T) {
	f := parseCSSFilters("blur(5px)")
	if len(f) != 1 || !f[0].Valid {
		t.Fatal("expected 1 valid blur filter")
	}
	if f[0].Name != "blur" || f[0].Value != 5 {
		t.Fatalf("blur = {Name:%q Value:%v}, want blur/5", f[0].Name, f[0].Value)
	}
}

func TestParseCSSFilters_Grayscale(t *testing.T) {
	f := parseCSSFilters("grayscale(100%)")
	if len(f) != 1 || !f[0].Valid {
		t.Fatal("expected 1 valid grayscale filter")
	}
	if f[0].Name != "grayscale" || f[0].Value != 1.0 {
		t.Fatalf("grayscale = {Name:%q Value:%v}, want grayscale/1.0", f[0].Name, f[0].Value)
	}
}

func TestParseCSSFilters_Sepia(t *testing.T) {
	f := parseCSSFilters("sepia(50%)")
	if len(f) != 1 || !f[0].Valid {
		t.Fatal("expected 1 valid sepia filter")
	}
	if f[0].Name != "sepia" || f[0].Value != 0.5 {
		t.Fatalf("sepia = %v, want 0.5", f[0].Value)
	}
}

func TestParseCSSFilters_Brightness(t *testing.T) {
	f := parseCSSFilters("brightness(1.5)")
	if len(f) != 1 || f[0].Value != 1.5 {
		t.Fatalf("brightness = %v, want 1.5", f[0].Value)
	}
}

func TestParseCSSFilters_Contrast(t *testing.T) {
	f := parseCSSFilters("contrast(2)")
	if len(f) != 1 || f[0].Value != 2.0 {
		t.Fatalf("contrast = %v, want 2.0", f[0].Value)
	}
}

func TestParseCSSFilters_Invert(t *testing.T) {
	f := parseCSSFilters("invert(100%)")
	if len(f) != 1 || f[0].Value != 1.0 {
		t.Fatalf("invert = %v, want 1.0", f[0].Value)
	}
}

func TestParseCSSFilters_Saturate(t *testing.T) {
	f := parseCSSFilters("saturate(2)")
	if len(f) != 1 || f[0].Value != 2.0 {
		t.Fatalf("saturate = %v, want 2.0", f[0].Value)
	}
}

func TestParseCSSFilters_HueRotate(t *testing.T) {
	f := parseCSSFilters("hue-rotate(90deg)")
	if len(f) != 1 || f[0].Name != "hue-rotate" || f[0].Value != 90 {
		t.Fatalf("hue-rotate = {Name:%q Value:%v}, want hue-rotate/90", f[0].Name, f[0].Value)
	}
}

func TestParseCSSFilters_Opacity(t *testing.T) {
	f := parseCSSFilters("opacity(50%)")
	if len(f) != 1 || f[0].Value != 0.5 {
		t.Fatalf("opacity = %v, want 0.5", f[0].Value)
	}
}

func TestParseCSSFilters_Multiple(t *testing.T) {
	f := parseCSSFilters("blur(2px) grayscale(100%)")
	if len(f) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(f))
	}
	if f[0].Name != "blur" || f[1].Name != "grayscale" {
		t.Fatalf("filter order wrong: %+v", f)
	}
}

func TestParseCSSFilters_InvalidName(t *testing.T) {
	f := parseCSSFilters("unknown(10px)")
	if len(f) != 0 {
		t.Fatalf("expected 0 filters for unknown function, got %d", len(f))
	}
}

func TestParseCSSFilters_EmptyParens(t *testing.T) {
	f := parseCSSFilters("blur()")
	if len(f) != 1 {
		t.Fatalf("expected 1 filter (blur with empty arg), got %d", len(f))
	}
	// Empty arg => Value=0, still Valid for blur (blur(0) is no-op but valid).
	if !f[0].Valid {
		t.Fatal("blur() should be valid")
	}
}

// --- parseFilterArg tests ---------------------------------------------------

func TestParseFilterArg_Percent(t *testing.T) {
	if v := parseFilterArg("50%"); v != 0.5 {
		t.Fatalf("50%% = %v, want 0.5", v)
	}
	if v := parseFilterArg("100%"); v != 1.0 {
		t.Fatalf("100%% = %v, want 1.0", v)
	}
}

func TestParseFilterArg_Px(t *testing.T) {
	if v := parseFilterArg("5px"); v != 5 {
		t.Fatalf("5px = %v, want 5", v)
	}
}

func TestParseFilterArg_Deg(t *testing.T) {
	if v := parseFilterArg("90deg"); v != 90 {
		t.Fatalf("90deg = %v, want 90", v)
	}
}

func TestParseFilterArg_Decimal(t *testing.T) {
	if v := parseFilterArg("1.5"); v != 1.5 {
		t.Fatalf("1.5 = %v, want 1.5", v)
	}
}

func TestParseFilterArg_Empty(t *testing.T) {
	if v := parseFilterArg(""); v != 0 {
		t.Fatalf("empty arg = %v, want 0", v)
	}
}

func TestParseFilterArg_Invalid(t *testing.T) {
	if v := parseFilterArg("abc"); v != 0 {
		t.Fatalf("invalid arg = %v, want 0", v)
	}
}

// --- tokenizeFilterFuncs tests ----------------------------------------------

func TestTokenizeFilterFuncs_Single(t *testing.T) {
	toks := tokenizeFilterFuncs("blur(5px)")
	if len(toks) != 1 || toks[0] != "blur(5px)" {
		t.Fatalf("single func = %v, want [blur(5px)]", toks)
	}
}

func TestTokenizeFilterFuncs_Multiple(t *testing.T) {
	toks := tokenizeFilterFuncs("blur(2px) grayscale(100%)")
	if len(toks) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(toks), toks)
	}
}

func TestTokenizeFilterFuncs_Nested(t *testing.T) {
	// Nested parentheses should not split.
	toks := tokenizeFilterFuncs("drop-shadow(10px 5px 5px rgba(0,0,0,0.5))")
	// drop-shadow is not in the supported list, but tokenizer should handle it.
	if len(toks) != 1 {
		t.Fatalf("expected 1 token for drop-shadow, got %d: %v", len(toks), toks)
	}
}

// --- buildCSSFilterChain tests (require CGO for Skia) -----------------------

func TestBuildCSSFilterChain_None(t *testing.T) {
	chain := buildCSSFilterChain(nil)
	if chain != nil {
		t.Fatal("nil input should return nil chain")
	}
	chain = buildCSSFilterChain([]CSSFilter{})
	if chain != nil {
		t.Fatal("empty slice should return nil chain")
	}
}

func TestBuildCSSFilterChain_Blur(t *testing.T) {
	filters := []CSSFilter{{Name: "blur", Value: 5, Valid: true}}
	chain := buildCSSFilterChain(filters)
	if chain == nil {
		t.Fatal("blur(5px) should produce a non-nil chain")
	}
}

func TestBuildCSSFilterChain_Grayscale(t *testing.T) {
	filters := []CSSFilter{{Name: "grayscale", Value: 1.0, Valid: true}}
	chain := buildCSSFilterChain(filters)
	if chain == nil {
		t.Fatal("grayscale(100%) should produce a non-nil chain")
	}
}

func TestBuildCSSFilterChain_Multiple(t *testing.T) {
	filters := []CSSFilter{
		{Name: "blur", Value: 2, Valid: true},
		{Name: "grayscale", Value: 1.0, Valid: true},
	}
	chain := buildCSSFilterChain(filters)
	if chain == nil {
		t.Fatal("multiple filters should produce a non-nil chain")
	}
}

func TestBuildCSSFilterChain_InvalidFilter(t *testing.T) {
	// Invalid filter (Valid=false) should be skipped; results in nil chain.
	filters := []CSSFilter{{Name: "blur", Value: 5, Valid: false}}
	chain := buildCSSFilterChain(filters)
	if chain != nil {
		t.Fatal("all filters invalid => nil chain")
	}
}
