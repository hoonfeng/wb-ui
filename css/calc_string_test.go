package css

import "testing"

func TestCalcHasRelativeUnit(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"100% - 40px", true},
		{"50% + 10px", true},
		{"2em + 10px", true},
		{"2rem", true},
		{"50vw + 100px", true},
		{"50vh", true},
		{"50vmin", true},
		{"50vmax", true},
		{"40px + 8px", false},
		{"10px - 5px", false},
		{"100px / 2", false},
		{"2pt + 1cm", false},
	}
	for _, tc := range tests {
		if got := CalcHasRelativeUnit(tc.expr); got != tc.want {
			t.Errorf("CalcHasRelativeUnit(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestEvalCalcString(t *testing.T) {
	ctx := CalcContext{ParentWidth: 200, FontSize: 16, RootFontSize: 16}
	tests := []struct {
		expr string
		want float64
	}{
		{"100% - 40px", 160},
		{"40px + 8px", 48},
		{"100% + 20px", 220},
		{"2em + 10px", 42},
	}
	for _, tc := range tests {
		got, err := EvalCalcString(tc.expr, ctx)
		if err != nil {
			t.Errorf("EvalCalcString(%q) unexpected error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("EvalCalcString(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
