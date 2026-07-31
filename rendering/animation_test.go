package rendering

import (
	"math"
	"testing"

	"wb-ui/css"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// makeKeyframes builds a *css.KeyframesRule from a compact description.
// entries are alternating [offset, prop, value, offset, prop, value, ...].
func makeKeyframes(name string, entries ...string) *css.KeyframesRule {
	kf := &css.KeyframesRule{Name: name}
	var currentKeys []string
	var currentDecls []css.Declaration
	for i := 0; i+2 < len(entries); {
		offset := entries[i]
		prop := entries[i+1]
		val := entries[i+2]
		// Check if next entry is another offset (starts with digit or "f"/"t").
		i += 3
		nextIsOffset := i < len(entries) && (entries[i][0] >= '0' && entries[i][0] <= '9' ||
			entries[i] == "from" || entries[i] == "to")
		currentKeys = append(currentKeys, offset)
		currentDecls = append(currentDecls, css.Declaration{
			Name:  prop,
			Value: tokenizeValue(val),
		})
		if nextIsOffset || i >= len(entries) {
			kf.Keyframes = append(kf.Keyframes, css.KeyframeRule{
				Keys:         currentKeys,
				Declarations: currentDecls,
			})
			currentKeys = nil
			currentDecls = nil
		}
	}
	return kf
}

// tokenizeValue produces a minimal token slice from a string.
func tokenizeValue(s string) []css.Token {
	t := css.NewTokenizer(s)
	tokens := t.Tokenize()
	if len(tokens) > 0 && tokens[len(tokens)-1].Type == css.TokenEOF {
		return tokens[:len(tokens)-1]
	}
	return tokens
}

// setupLookup registers a keyframes lookup for testing.
func setupLookup(kf *css.KeyframesRule) {
	KeyframesLookup = func(name string) *css.KeyframesRule {
		if kf != nil && name == kf.Name {
			return kf
		}
		return nil
	}
}

// --- Opacity (backward compatibility) ---------------------------------------

func TestAnimateOpacity(t *testing.T) {
	kf := makeKeyframes("fade",
		"from", "opacity", "0",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "fade"
	st.AnimationDuration = 2
	st.AnimationIterationCount = 1

	applyAnimationToStyle(st, 0)
	if st.Opacity != 0 {
		t.Errorf("opacity at t=0: got %v, want 0", st.Opacity)
	}
	applyAnimationToStyle(st, 1)
	if math.Abs(st.Opacity-0.5) > 0.01 {
		t.Errorf("opacity at t=1: got %v, want 0.5", st.Opacity)
	}
	applyAnimationToStyle(st, 2)
	if math.Abs(st.Opacity-1.0) > 0.01 {
		t.Errorf("opacity at t=2: got %v, want 1.0", st.Opacity)
	}
}

// --- Transform: translateX --------------------------------------------------

func TestAnimateTranslateX(t *testing.T) {
	kf := makeKeyframes("slide",
		"from", "transform", "translateX(0px)",
		"to", "transform", "translateX(100px)",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "slide"
	st.AnimationDuration = 2
	st.AnimationIterationCount = 1

	applyAnimationToStyle(st, 0)
	if st.TranslateX != 0 {
		t.Errorf("TranslateX at t=0: got %v, want 0", st.TranslateX)
	}
	applyAnimationToStyle(st, 1)
	if math.Abs(st.TranslateX-50) > 0.01 {
		t.Errorf("TranslateX at t=1: got %v, want 50", st.TranslateX)
	}
	applyAnimationToStyle(st, 2)
	if math.Abs(st.TranslateX-100) > 0.01 {
		t.Errorf("TranslateX at t=2: got %v, want 100", st.TranslateX)
	}
}

// --- Transform: scale -------------------------------------------------------

func TestAnimateScale(t *testing.T) {
	kf := makeKeyframes("grow",
		"from", "transform", "scale(1)",
		"to", "transform", "scale(2)",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "grow"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1

	applyAnimationToStyle(st, 0.5)
	if math.Abs(st.ScaleX-1.5) > 0.01 || math.Abs(st.ScaleY-1.5) > 0.01 {
		t.Errorf("Scale at t=0.5: got (%v,%v), want (1.5,1.5)", st.ScaleX, st.ScaleY)
	}
}

// --- Animation delay --------------------------------------------------------

func TestAnimateDelay(t *testing.T) {
	kf := makeKeyframes("delayed",
		"from", "opacity", "0",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "delayed"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1
	st.AnimationDelay = 2

	// Before delay ends: no animation applied, Opacity stays at default (1.0).
	applyAnimationToStyle(st, 1)
	if st.Opacity != 1.0 {
		t.Errorf("opacity during delay: got %v, want 1.0", st.Opacity)
	}
	// After delay: animation progresses.
	applyAnimationToStyle(st, 2.5)
	if math.Abs(st.Opacity-0.5) > 0.01 {
		t.Errorf("opacity after delay at t=2.5: got %v, want 0.5", st.Opacity)
	}
}

// --- Direction: alternate ---------------------------------------------------

func TestAnimateDirectionAlternate(t *testing.T) {
	kf := makeKeyframes("alt",
		"from", "opacity", "0",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "alt"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 3
	st.AnimationDirection = "alternate"

	// 1st iteration: normal (0→1).
	applyAnimationToStyle(st, 0.5)
	if math.Abs(st.Opacity-0.5) > 0.01 {
		t.Errorf("1st iter t=0.5: got %v, want 0.5", st.Opacity)
	}
	// 2nd iteration: reverse (1→0).
	applyAnimationToStyle(st, 1.5)
	if math.Abs(st.Opacity-0.5) > 0.01 {
		t.Errorf("2nd iter t=1.5: got %v, want 0.5", st.Opacity)
	}
}

// --- Fill-mode: forwards ----------------------------------------------------

func TestAnimateFillModeForwards(t *testing.T) {
	kf := makeKeyframes("stay",
		"from", "opacity", "0.5",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "stay"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1
	st.AnimationFillMode = "forwards"

	// After animation ends, opacity should stay at the last keyframe (1).
	applyAnimationToStyle(st, 3)
	if math.Abs(st.Opacity-1.0) > 0.01 {
		t.Errorf("fill-mode=forwards after end: got %v, want 1.0", st.Opacity)
	}
}

// --- Fill-mode: backwards ---------------------------------------------------

func TestAnimateFillModeBackwards(t *testing.T) {
	kf := makeKeyframes("back",
		"from", "opacity", "0.3",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "back"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1
	st.AnimationDelay = 2
	st.AnimationFillMode = "backwards"

	// During delay: apply first keyframe.
	applyAnimationToStyle(st, 1)
	if math.Abs(st.Opacity-0.3) > 0.01 {
		t.Errorf("fill-mode=backwards during delay: got %v, want 0.3", st.Opacity)
	}
}

// --- Timing function: ease-in -----------------------------------------------

func TestAnimateTimingEaseIn(t *testing.T) {
	kf := makeKeyframes("easeIn",
		"from", "opacity", "0",
		"to", "opacity", "1",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "easeIn"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1
	st.AnimationTimingFunction = "ease-in"

	applyAnimationToStyle(st, 0.5)
	// ease-in: at t=0.5, progress should be < 0.5 (starts slow, accelerates).
	if st.Opacity >= 0.5 {
		t.Errorf("ease-in at t=0.5: opacity=%v, expected <0.5 (decelerating start)", st.Opacity)
	}
}

// --- Color animation --------------------------------------------------------

func TestAnimateColor(t *testing.T) {
	kf := makeKeyframes("colorize",
		"from", "color", "#ff0000",
		"to", "color", "#0000ff",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "colorize"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1

	applyAnimationToStyle(st, 0)
	if st.AnimatedColor.R != 255 || st.AnimatedColor.B != 0 {
		t.Errorf("color at t=0: got %+v, want R=255,B=0", st.AnimatedColor)
	}
	applyAnimationToStyle(st, 1)
	if st.AnimatedColor.R != 0 || st.AnimatedColor.B != 255 {
		t.Errorf("color at t=1: got %+v, want R=0,B=255", st.AnimatedColor)
	}
	applyAnimationToStyle(st, 0.5)
	if st.AnimatedColor.R < 120 || st.AnimatedColor.R > 135 || st.AnimatedColor.B < 120 || st.AnimatedColor.B > 135 {
		t.Errorf("color at t=0.5: got %+v, want R≈127,B≈127", st.AnimatedColor)
	}
}

// --- Background-color animation ---------------------------------------------

func TestAnimateBackgroundColor(t *testing.T) {
	kf := makeKeyframes("bg",
		"from", "background-color", "#000000",
		"to", "background-color", "#ffffff",
	)
	setupLookup(kf)
	st := style.NewComputedStyle()
	st.AnimationName = "bg"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1

	applyAnimationToStyle(st, 0.5)
	if st.AnimatedBackgroundColor.R < 120 || st.AnimatedBackgroundColor.R > 135 {
		t.Errorf("bgcolor at t=0.5: got %+v, want R≈127", st.AnimatedBackgroundColor)
	}
}

// --- Test ApplyAnimations traversal -----------------------------------------

func TestApplyAnimationsWalk(t *testing.T) {
	setupLookup(makeKeyframes("fade", "from", "opacity", "0", "to", "opacity", "1"))
	st := style.NewComputedStyle()
	st.AnimationName = "fade"
	st.AnimationDuration = 1
	st.AnimationIterationCount = 1
	AnimationTime = 0.5

	// Test that ApplyAnimations runs without error on nil.
	ApplyAnimations(nil) // should not panic
}

// --- parseCSSNumber test ----------------------------------------------------

func TestParseCSSNumber(t *testing.T) {
	cases := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"10px", 10, true},
		{"1.5", 1.5, true},
		{"45deg", 45, true},
		{"0", 0, true},
		{"-5em", -5, true},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseCSSNumber(c.input)
		if ok != c.ok || (ok && math.Abs(got-c.want) > 0.001) {
			t.Errorf("parseCSSNumber(%q) = (%v, %v), want (%v, %v)", c.input, got, ok, c.want, c.ok)
		}
	}
}

// --- parseColorSimple test --------------------------------------------------

func TestParseColorSimple(t *testing.T) {
	cases := []struct {
		input string
		want  graphics.Color
		ok    bool
	}{
		{"#ff0000", graphics.Color{R: 255, G: 0, B: 0, A: 255}, true},
		{"red", graphics.Color{R: 255, G: 0, B: 0, A: 255}, true},
		{"#f00", graphics.Color{R: 255, G: 0, B: 0, A: 255}, true},
		{"transparent", graphics.Color{R: 0, G: 0, B: 0, A: 0}, true},
		{"rgb(0, 255, 0)", graphics.Color{R: 0, G: 255, B: 0, A: 255}, true},
	}
	for _, c := range cases {
		got, ok := parseColorSimple(c.input)
		if ok != c.ok || (ok && (got.R != c.want.R || got.G != c.want.G || got.B != c.want.B || got.A != c.want.A)) {
			t.Errorf("parseColorSimple(%q) = (%+v, %v), want (%+v, %v)", c.input, got, ok, c.want, c.ok)
		}
	}
}

// --- cubicBezier test -------------------------------------------------------

func TestCubicBezier(t *testing.T) {
	// At t=0.5, linear gives 0.5. Ease-in gives a smaller value (starts slow).
	linear := cubicBezier(0.5, 0, 0, 1, 1) // linear
	easeIn := cubicBezier(0.5, 0.42, 0, 1, 1)
	if math.Abs(linear-0.5) > 0.01 {
		t.Errorf("linear at t=0.5: got %v, want 0.5", linear)
	}
	if easeIn >= linear {
		t.Errorf("ease-in at t=0.5: got %v, want < 0.5 (starts slow)", easeIn)
	}
}

// --- extractTransformFunc test ----------------------------------------------

func TestExtractTransformFunc(t *testing.T) {
	cases := []struct {
		input    string
		funcName string
		want     float64
		ok       bool
	}{
		{"translateX(10px)", "translateX", 10, true},
		{"translateY(20px)", "translateY", 20, true},
		{"scale(1.5)", "scale", 1.5, true},
		{"rotate(45deg)", "rotate", 45, true},
		{"translateX(10px) scale(2)", "translateX", 10, true},
	}
	for _, c := range cases {
		got, ok := extractTransformFunc(c.input, c.funcName)
		if ok != c.ok || (ok && math.Abs(got-c.want) > 0.001) {
			t.Errorf("extractTransformFunc(%q, %q) = (%v, %v), want (%v, %v)",
				c.input, c.funcName, got, ok, c.want, c.ok)
		}
	}
}
