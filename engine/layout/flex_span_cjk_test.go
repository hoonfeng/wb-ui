package layout

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

// TestFlexItemSpanBlockifyCJK: a span (inline) inside a flex row is blockified
// into a flex item; its CJK text must wrap at the flex-resolved width, not per
// character (resume-text reproduction).
func TestFlexItemSpanBlockifyCJK(t *testing.T) {
	MeasureTextFunc = func(family string, size float64, weight int, fstyle, text string) float64 {
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
	doc := dom.NewDocument()

	// banner: flex row, width 1000px
	banner := mkFlex()
	banner.style.Width = style.Length{Value: 1000, Unit: "px"}
	banner.style.AlignItems = "center"

	// span text: inline element → blockified flex item
	spanEl := dom.NewElement(doc, "span")
	spanText := dom.NewText(doc, "上次任务未完成，本对话上下文与进度已保留，可直接继续")
	spanEl.AppendChild(spanText)
	spanCs := style.NewComputedStyle()
	spanCs.Display = style.DisplayInline
	spanCs.FlexGrow = 1
	spanCs.FontSize = style.Length{Value: 12, Unit: "px"}
	spanCs.SetProperty("line-height", "1.4")
	span := newBoxForElement(spanEl, spanCs)
	buildChildren(span, spanEl, style.NewResolver())
	banner.AddChild(span)

	// btn
	btnEl := dom.NewElement(doc, "button")
	btnText := dom.NewText(doc, "继续任务")
	btnEl.AppendChild(btnText)
	btnCs := style.NewComputedStyle()
	btnCs.Display = style.DisplayInlineBlock
	btnCs.FlexShrink = 0
	btnCs.WhiteSpace = style.WhiteSpaceNoWrap
	btn := newBoxForElement(btnEl, btnCs)
	buildChildren(btn, btnEl, style.NewResolver())
	banner.AddChild(btn)

	Layout(banner, 1200, 400)

	// 收集 span 子树文本段
	var segs []TextSegment
	var collect func(box Box)
	collect = func(box Box) {
		switch v := box.(type) {
		case *ElementBox:
			for _, c := range v.Children() {
				collect(c)
			}
		case *InlineTextBox:
			segs = append(segs, v.TextSegments...)
		}
	}
	collect(span)
	t.Logf("span text segments: %d", len(segs))
	multiLine := false
	for i := 1; i < len(segs); i++ {
		if !approxEq(segs[i].Y, segs[0].Y) {
			multiLine = true
			t.Logf("  seg %d y=%.1f != seg0 y=%.1f", i, segs[i].Y, segs[0].Y)
		}
	}
	if multiLine {
		t.Errorf("CJK text wrapped into multiple lines in blockified span")
	}
	totalW := 0.0
	for _, s := range segs {
		totalW += s.Width
	}
	t.Logf("total width=%.1f", totalW)
}
