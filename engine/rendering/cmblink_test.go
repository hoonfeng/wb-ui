package rendering

import (
	"math"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/style"
)

// TestCmBlinkOpacityAnimation 验证 CM6 光标闪烁动画（@keyframes cm-blink
// 只有 50% 声明 opacity:0，0%/100% 未声明）能被引擎正确驱动：
//   - 未声明端点回退静态 opacity（StaticOpacity=1.0）→ 插值不再返回 false
//   - steps(1) 时序（step-end 语义）：0~50% 保持 1.0，50%~100% 跳变 0
//   - st.Opacity 随 AnimationTime 在 1 ↔ 0 之间切换 → 光标真正闪烁
// 这是「光标不闪」回归测试：修复前 interpolateFloatAt 对 0%/100% 未声明
// 端点返回 (0,false) → st.Opacity 永不变化。
func TestCmBlinkOpacityAnimation(t *testing.T) {
	// 1. 构造 CM6 的 @keyframes cm-blink（仅 50% 声明 opacity:0）
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
	if kf == nil || len(kf.Keyframes) == 0 {
		t.Fatalf("no keyframes parsed")
	}

	// 2. 元素样式：animation: steps(1) cm-blink 1.2s infinite
	st := &style.ComputedStyle{}
	st.AnimationName = "cm-blink"
	st.AnimationDuration = 1.2
	st.AnimationIterationCount = 0 // infinite
	st.AnimationTimingFunction = "steps(1)"
	st.Opacity = 1.0
	st.StaticOpacity = 1.0 // 解析时保存的静态值

	orig := KeyframesLookup
	KeyframesLookup = func(name string) *css.KeyframesRule {
		if name == "cm-blink" {
			return kf
		}
		return nil
	}
	defer func() { KeyframesLookup = orig }()

	// 3. 关键帧时序（steps(1) step-end）：0.6s 亮（opacity 1），
	//    0.6s 灭（opacity 0），每 1.2s 循环。
	//    t=0.1（0.6s 内，即 0%~50%）：opacity 应保持 1（step-end 不跳变）
	AnimationTime = 0.1
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-1.0) > 0.001 {
		t.Fatalf("t=0.1: opacity=%v want 1.0 (visible phase)", st.Opacity)
	}
	t.Logf("t=0.1 opacity=%.2f (visible)", st.Opacity)

	//    t=0.7（0.6s 后，50%~100%）：opacity 应跳变 0（隐藏相位）
	AnimationTime = 0.7
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-0.0) > 0.001 {
		t.Fatalf("t=0.7: opacity=%v want 0.0 (hidden phase)", st.Opacity)
	}
	t.Logf("t=0.7 opacity=%.2f (hidden)", st.Opacity)

	//    t=1.3（下一轮 0.6s 内）：再次可见（闪烁循环）
	AnimationTime = 1.3
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if math.Abs(st.Opacity-1.0) > 0.001 {
		t.Fatalf("t=1.3: opacity=%v want 1.0 (blink cycle visible)", st.Opacity)
	}
	t.Logf("t=1.3 opacity=%.2f (blink cycle OK)", st.Opacity)
}

// TestCmBlinkOpacityBaseFallback 验证未声明端点回退静态值：opacity
// 关键帧只有 50% 时，0%/100% 用 StaticOpacity 而非返回 false。
func TestCmBlinkOpacityBaseFallback(t *testing.T) {
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
	// 未声明端点用 base=1.0
	op, ok := interpolateKeyframeFloat(kf, 0.0, "opacity", 1.0)
	if !ok {
		t.Fatalf("progress=0: interpolate returned !ok (must fall back to static)")
	}
	if math.Abs(op-1.0) > 0.001 {
		t.Fatalf("progress=0: opacity=%v want 1.0 (static base)", op)
	}
	op, ok = interpolateKeyframeFloat(kf, 0.5, "opacity", 1.0)
	if !ok || math.Abs(op-0.0) > 0.001 {
		t.Fatalf("progress=0.5: opacity=%v ok=%v want 0.0", op, ok)
	}
	op, ok = interpolateKeyframeFloat(kf, 1.0, "opacity", 1.0)
	if !ok || math.Abs(op-1.0) > 0.001 {
		t.Fatalf("progress=1.0: opacity=%v ok=%v want 1.0 (static base)", op, ok)
	}
}
