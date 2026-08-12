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

func TestEvalMinMaxClamp(t *testing.T) {
	ctx := CalcContext{
		ParentWidth:    1000,
		FontSize:       16,
		RootFontSize:   16,
		ViewportWidth:  800,
		ViewportHeight: 1000,
	}
	tests := []struct {
		expr string
		want float64
	}{
		{"min(600px, 100%)", 600},          // min(600, 1000) = 600
		{"max(10px, 5em)", 80},             // max(10, 80) = 80
		{"clamp(16px, 4vw, 40px)", 32},     // clamp(16, 32, 40) = 32
		{"min(100% - 40px, 600px)", 600},   // min(960, 600) = 600
		{"max(-10px, 5px)", 5},
		{"min(max(10px, 20px), 15px)", 15}, // nested min/max
		{"clamp(0px, 10vh, 50px)", 50},     // clamp(0, 100, 50) = 50
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

func TestEvalNestedCalc(t *testing.T) {
	ctx := CalcContext{ParentWidth: 200, FontSize: 16, RootFontSize: 16}
	got, err := EvalCalcString("calc(100% - 40px)", ctx)
	if err != nil {
		t.Fatalf("EvalCalcString(nested calc) unexpected error: %v", err)
	}
	if got != 160 {
		t.Fatalf("EvalCalcString(calc(100%% - 40px)) = %v, want 160", got)
	}
	// Deeper nesting.
	got, err = EvalCalcString("calc(calc(50% + 10px))", ctx)
	if err != nil {
		t.Fatalf("EvalCalcString(deep nested calc) unexpected error: %v", err)
	}
	if got != 110 {
		t.Fatalf("EvalCalcString(calc(calc(50%% + 10px))) = %v, want 110", got)
	}
}
