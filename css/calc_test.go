package css

import (
	"testing"
)

func tokenizeExpr(expr string) []Token {
	t := NewTokenizer(expr)
	tokens := t.Tokenize()
	// Strip EOF token.
	return tokens[:len(tokens)-1]
}

func TestIsCalcValue(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"calc(10px + 20px)", true},
		{"calc(100% - 40px)", true},
		{"calc((10px + 20px) * 2)", true},
		{"10px", false},
		{"none", false},
		{"auto", false},
		{"calc(", true}, // malformed but still starts with calc
	}
	for _, tc := range tests {
		tokens := tokenizeExpr(tc.input)
		got := IsCalcValue(tokens)
		if got != tc.want {
			t.Errorf("IsCalcValue(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEvalCalc(t *testing.T) {
	ctx := CalcContext{
		ParentWidth:   200,
		FontSize:      16,
		RootFontSize:  16,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	tests := []struct {
		expr string
		ctx  CalcContext
		want float64
	}{
	{"calc(10px + 20px)", ctx, 30},
	{"calc(100% - 40px)", CalcContext{ParentWidth: 200}, 160},
	{"calc(2em + 10px)", CalcContext{FontSize: 16}, 42},
	{"calc((10px + 20px) * 2)", ctx, 60},
	{"calc(100vw / 2)", CalcContext{ViewportWidth: 800}, 400},
	{"calc(100vh / 2)", CalcContext{ViewportHeight: 600}, 300},
	{"calc(20px * 3)", ctx, 60},
	{"calc(100px - 30px)", ctx, 70},
	{"calc(10px + 5px * 2)", ctx, 20},
	{"calc((100%) / 2)", CalcContext{ParentWidth: 200}, 100},
	{"calc(100% + 20px)", CalcContext{ParentWidth: 200}, 220},
	{"calc(1em)", CalcContext{FontSize: 16}, 16},
	{"calc(2rem)", CalcContext{RootFontSize: 16}, 32},
	{"calc(50vw + 100px)", CalcContext{ViewportWidth: 800}, 500},
	{"calc(10px + -5px)", ctx, 5},
	{"calc(-10px + 20px)", ctx, 10},
	}

	for _, tc := range tests {
		tokens := tokenizeExpr(tc.expr)
		got, err := EvalCalc(tokens, tc.ctx)
		if err != nil {
			t.Errorf("EvalCalc(%q) unexpected error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("EvalCalc(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestEvalCalcErrors(t *testing.T) {
	ctx := CalcContext{}
	tests := []string{
		"10px",                            // not a calc()
		"calc()",                          // empty
		"calc(/ 10px)",                    // invalid syntax
		"calc(10px / 0px)",                // division by zero
	}
	for _, expr := range tests {
		tokens := tokenizeExpr(expr)
		_, err := EvalCalc(tokens, ctx)
		if err == nil {
			t.Errorf("EvalCalc(%q) expected error, got nil", expr)
		}
	}
}

func TestEvalCalcMixedUnits(t *testing.T) {
	ctx := CalcContext{
		ParentWidth:   400,
		FontSize:      16,
		ViewportWidth:  1024,
	}
	tests := []struct {
		expr string
		want float64
	}{
		{"calc(50% - 20px)", 180},   // 200 - 20 = 180 (50% of 400 = 200)
		{"calc(25vw + 1em)", 272},   // 256 + 16 = 272 (25% of 1024 = 256)
		{"calc(10px + 2em - 6px)", 36}, // 10 + 32 - 6 = 36
	}
	for _, tc := range tests {
		tokens := tokenizeExpr(tc.expr)
		got, err := EvalCalc(tokens, ctx)
		if err != nil {
			t.Errorf("EvalCalc(%q) unexpected error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("EvalCalc(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
