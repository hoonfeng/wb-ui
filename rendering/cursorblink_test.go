package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/style"
)

// TestCursorBlinkAnimation 验证 xterm block 光标的闪烁动画（@keyframes
// background-color 0%→50% 插值 + inherit 帧）能被引擎动画系统驱动——
// 用户反馈「光标不闪烁」的回归测试。xterm 注入的动画 CSS：
//
//	@keyframes blink_block_x {
//	  0%  { background-color: #fff; color: #000; }
//	  50% { background-color: inherit; color: #fff; }
//	}
//	.xterm.focus .xterm-cursor.xterm-cursor-blink.xterm-cursor-block {
//	  animation: blink_block_x 1s step-end infinite;
//	}
func TestCursorBlinkAnimation(t *testing.T) {
	// 1. 构造 xterm 光标动画的 @keyframes（模拟 xterm 注入的 CSS）
	p := css.NewParser(`
		@keyframes blink_block_x {
			0%  { background-color: #ffffff; }
			50% { background-color: inherit; }
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

	// 2. 构造元素样式：animation: blink_block_x 1s step-end infinite
	st := &style.ComputedStyle{}
	st.AnimationName = "blink_block_x"
	st.AnimationDuration = 1.0
	st.AnimationIterationCount = 0 // infinite
	st.AnimationTimingFunction = "step-end"
	st.BackgroundColor = style.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	// 3. 关键：inherit 帧必须被解析为 valid（透明等价），否则动画不插值
	//    —— findColorInDecls 的 inherit 处理（0%→50% 可插值）。
	el := &dom.Element{}
	_ = el

	// 4. 模拟动画驱动：AnimationTime 递增，applyAnimationToStyle 直接验证
	orig := KeyframesLookup
	KeyframesLookup = func(name string) *css.KeyframesRule {
		if name == "blink_block_x" {
			return kf
		}
		return nil
	}
	defer func() { KeyframesLookup = orig }()

	// t=0.05（0% 附近）：光标色（白）
	AnimationTime = 0.05
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if st.AnimatedBackgroundColor.A == 0 || st.AnimatedBackgroundColor.R < 200 {
		t.Fatalf("t=0.05: bg=%+v want white cursor", st.AnimatedBackgroundColor)
	}
	t.Logf("t=0.05 bg=%+v (cursor visible)", st.AnimatedBackgroundColor)

	// t=0.75（50% 之后）：透明（inherit → 隐藏）
	AnimationTime = 0.75
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if st.AnimatedBackgroundColor.A != 0 {
		t.Fatalf("t=0.75: bg=%+v want transparent (hidden)", st.AnimatedBackgroundColor)
	}
	t.Logf("t=0.75 bg=%+v (cursor hidden)", st.AnimatedBackgroundColor)

	// t=1.05（下一轮 0% 附近）：再次可见
	AnimationTime = 1.05
	applyAnimationToStyle(st, AnimationTime, KeyframesLookup)
	if st.AnimatedBackgroundColor.A == 0 {
		t.Fatalf("t=1.05: bg=%+v want visible again (blink cycle)", st.AnimatedBackgroundColor)
	}
	t.Logf("t=1.05 bg=%+v (blink cycle OK)", st.AnimatedBackgroundColor)
}
