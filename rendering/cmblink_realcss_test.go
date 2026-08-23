package rendering

import (
	"math"
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/style"
)

// TestCmBlinkRealCSS 用真实 CSS 解析器解析 CM6 的 @keyframes cm-blink
// （0% 和 100% 是空帧，解析器可能丢弃 → 只有 50% 单点），验证：
//   - 单点中部（offset=0.5）时 0%/100% 用 base 补端点
//   - progress=0 → base(1.0)；progress=0.5 → 0；progress=1 → base(1.0)
func TestCmBlinkRealCSS(t *testing.T) {
	p := css.NewParser(`
		@keyframes cm-blink {
			0%  {}
			50% { opacity: 0; }
			100% {}
		}
	`)
	rules := p.ParseStyleSheet()
	var kf *css.KeyframesRule
	for _, r := range rules {
		if kfr, ok := r.(*css.KeyframesRule); ok {
			kf = kfr
		}
	}
	if kf == nil {
		t.Fatalf("no keyframes")
	}
	t.Logf("keyframes=%d", len(kf.Keyframes))
	for i, rule := range kf.Keyframes {
		var ds []string
		for _, d := range rule.Declarations {
			ds = append(ds, d.Name+"="+d.ValueString())
		}
		t.Logf("  kf[%d] keys=%v decls=[%s]", i, rule.Keys, strings.Join(ds, ", "))
	}

	// progress=0 → 0% 帧（隐式 base=1.0）
	op, ok := interpolateKeyframeFloat(kf, 0.0, "opacity", 1.0)
	t.Logf("progress=0: op=%v ok=%v", op, ok)
	if !ok || math.Abs(op-1.0) > 0.001 {
		t.Fatalf("progress=0: got op=%v ok=%v want 1.0", op, ok)
	}

	// progress=0.5 → 50% 帧 opacity:0
	op, ok = interpolateKeyframeFloat(kf, 0.5, "opacity", 1.0)
	t.Logf("progress=0.5: op=%v ok=%v", op, ok)
	if !ok || math.Abs(op-0.0) > 0.001 {
		t.Fatalf("progress=0.5: got op=%v ok=%v want 0.0", op, ok)
	}

	// progress=1 → 100% 帧（隐式 base=1.0）
	op, ok = interpolateKeyframeFloat(kf, 1.0, "opacity", 1.0)
	t.Logf("progress=1: op=%v ok=%v", op, ok)
	if !ok || math.Abs(op-1.0) > 0.001 {
		t.Fatalf("progress=1: got op=%v ok=%v want 1.0", op, ok)
	}

	// steps(1) 时序下的完整闪烁：t=0.1 可见(1)、t=0.7 隐藏(0)、t=1.3 可见(1)
	st := &style.ComputedStyle{}
	st.AnimationName = "cm-blink"
	st.AnimationDuration = 1.2
	st.AnimationIterationCount = 0
	st.AnimationTimingFunction = "steps(1)"
	st.Opacity = 1.0
	st.StaticOpacity = 1.0
	orig := KeyframesLookup
	KeyframesLookup = func(name string) *css.KeyframesRule { return kf }
	defer func() { KeyframesLookup = orig }()
	AnimationTime = 0.1
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-1.0) > 0.001 {
		t.Fatalf("t=0.1: opacity=%v want 1.0 (visible)", st.Opacity)
	}
	AnimationTime = 0.7
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-0.0) > 0.001 {
		t.Fatalf("t=0.7: opacity=%v want 0.0 (hidden)", st.Opacity)
	}
	AnimationTime = 1.3
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-1.0) > 0.001 {
		t.Fatalf("t=1.3: opacity=%v want 1.0 (blink cycle)", st.Opacity)
	}
	t.Log("blink cycle OK: 1 → 0 → 1")
}

