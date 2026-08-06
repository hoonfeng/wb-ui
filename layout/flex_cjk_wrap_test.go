package layout

import (
	"testing"

	"wb-ui/style"
)

// TestFlexItemCJKTextWrap: a flex item (flex:1) with a short CJK text line
// must stay on ONE line (container is wide enough), not wrap per character.
// Reproduces .resume-text ballooning to 437px (26 lines × 16.8) which ate the
// chat area and shrank .chat-messages to 110px (thinking scrollbar clipped).
func TestFlexItemCJKTextWrap(t *testing.T) {
	MeasureTextFunc = func(family string, size float64, weight int, fstyle, text string) float64 {
		// CJK char ≈ 1em wide, ASCII ≈ 0.5em
		w := 0.0
		for _, r := range text {
			if r > 0x2E80 {
				w += size
			} else {
				w += size * 0.55
			}
		}
		return w
	}
	FontMetricsFunc = func(family string, size float64, weight int, fstyle string) (float64, float64, float64) {
		return size * 0.85, size * 0.2, 0
	}
	// banner: display:flex; width:1000px → text flex:1 (~900px), btn 80px.
	banner := mkFlex()
	banner.style.Width = style.Length{Value: 1000, Unit: "px"}
	banner.style.AlignItems = "center"

	text := mkBlock()
	text.style.FlexGrow = 1
	text.style.FontSize = style.Length{Value: 12, Unit: "px"}
	text.style.Properties["line-height"] = "1.4"
	text.AddChild(mkTextRun("上次任务未完成，本对话上下文与进度已保留，可直接继续"))
	banner.AddChild(text)

	btn := mkBlockWH(80, 0)
	btn.style.FlexShrink = 0
	banner.AddChild(btn)

	state := Layout(banner, 1200, 400)

	tw := state.GeometryForBox(text).ContentWidth()
	t.Logf("text box content width=%.1f", tw)
	if tw < 800 {
		t.Errorf("text flex item only %.1f px wide (want ≥ 800)", tw)
	}

	// 收集 text 子树的文本段
	var segs []TextSegment
	var walk func(box Box)
	walk = func(box Box) {
		switch v := box.(type) {
		case *ElementBox:
			for _, c := range v.Children() {
				walk(c)
			}
		case *InlineTextBox:
			segs = append(segs, v.TextSegments...)
		}
	}
	walk(text)
	t.Logf("text segments: %d", len(segs))
	// 先检查是否多行（y 不同），再判断行数
	multiLine := false
	for i := 1; i < len(segs); i++ {
		if !approxEq(segs[i].Y, segs[0].Y) {
			multiLine = true
			t.Logf("  seg %d y=%.1f != seg0 y=%.1f", i, segs[i].Y, segs[0].Y)
		}
	}
	if multiLine {
		t.Errorf("CJK text wrapped into multiple lines")
	}
	// 总宽 = 所有段宽之和，应 ≈ 26 字 × 12px = 312
	totalW := 0.0
	for _, s := range segs {
		totalW += s.Width
	}
	t.Logf("text total width=%.1f (want ~312 for 26 CJK chars)", totalW)
	if totalW < 280 || totalW > 350 {
		t.Errorf("text total width %.1f outside expected 280..350", totalW)
	}
}
