package layout

import (
	"testing"

	"wb-ui/style"
)

// setCJKMeasure 设置真实字形测量（每 CJK 字 1em 宽，ASCII 0.6em），
// 模拟渲染层 MeasureTextFunc（测试环境默认 fallback 每字 0.5em 太窄，
// 掩盖真实宽度分配问题）。
func setCJKMeasure(t *testing.T) {
	t.Helper()
	prev := MeasureTextFunc
	MeasureTextFunc = func(family string, fs float64, weight int, fstyle string, text string) float64 {
		w := 0.0
		for _, ch := range text {
			if ch > 0x2E80 { // CJK
				w += fs
			} else {
				w += fs * 0.6
			}
		}
		return w
	}
	t.Cleanup(func() { MeasureTextFunc = prev })
}

// TestFoldedSummaryIntrinsicTitleWidth: 验证 flex 布局对标题 span 的
// intrinsic 宽度（base size）——4 个 CJK 字 @12px 应返回 48px。
// 回归：webkit 级复现显示标题 span box 48px 但 IFC 只排 3 字（36px 换行），
// 怀疑 flex 解析时 span 的 font 未继承 → intrinsic 用默认字体测出 36px。
func TestFoldedSummaryIntrinsicTitleWidth(t *testing.T) {
	setCJKMeasure(t)
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayInline
	cs.FontSize = style.Length{Value: 12, Unit: "px"}
	cs.FontFamily = "sans-serif"
	span := &ElementBox{nodeType: NodeGenericElement, style: cs}
	tb := &InlineTextBox{text: "完成摘要", style: cs}
	span.AddChild(tb)

	w := intrinsicContentWidth(span, true)
	t.Logf("intrinsicContentWidth(完成摘要) = %.1f", w)
	if w < 47 || w > 49 {
		t.Fatalf("intrinsic title width = %.1f, want ~48 (4 CJK chars @12px)", w)
	}
}

// TestFoldedSummaryWideContainer: 真实 folded-summary 宽容器场景回归。
// 结构（RightPanel.vue）：
//   .msg-item (flex row)
//     .msg-bubble (flex:1; min-width:0; word-break:break-word) — 宽容器
//       .folded-summary (flex row; align-items:center; gap:5px)
//         chevron svg + SvgIcon + <span>完成摘要</span> + <span>长 desc</span>
//
// 浏览器语义：宽容器（500px）里「完成摘要」必须在一行（4 字同 Y）。
// 回归：wb-ui 曾把 flex 子项可用宽度算成近 0 → CJK 每字软换行 → 「完成摘要」
// 一行一个（用户反馈「完成摘要这四个字一行一个了」）。
func TestFoldedSummaryWideContainer(t *testing.T) {
	setCJKMeasure(t)

	// folded-summary：flex row 容器
	fs := mkFlex()
	fs.style.FontSize = style.Length{Value: 12, Unit: "px"}
	fs.style.Gap = style.Length{Value: 5, Unit: "px"}
	fs.style.AlignItems = "center"
	fs.style.SetProperty("word-break", "break-word")
	fs.style.SetProperty("overflow-wrap", "break-word")

	inlineSpan := func(text string) *ElementBox {
		cs := style.NewComputedStyle()
		cs.Display = style.DisplayInline
		cs.FontSize = style.Length{Value: 12, Unit: "px"}
		b := &ElementBox{nodeType: NodeGenericElement, style: cs}
		tb := &InlineTextBox{text: text, style: cs}
		b.AddChild(tb)
		return b
	}
	fs.AddChild(inlineSpan("▸"))
	fs.AddChild(inlineSpan("完成摘要"))
	fs.AddChild(inlineSpan("已为你完成全部请求并生成了完整摘要，共修改 12 个文件。"))

	// msg-bubble：flex:1 min-width:0 的 flex item（真实 CSS：flex:1; min-width:0; max-width:85%）
	bubble := mkBlock()
	bubble.style.FlexGrow = 1
	bubble.style.FlexShrink = 1
	bubble.style.FlexBasis = style.Length{Unit: "%"}
	bubble.style.MinWidth = style.Length{Value: 0, Unit: "px"}
	bubble.style.MaxWidth = style.Length{Unit: "%", Value: 85}
	bubble.style.SetProperty("word-break", "break-word")
	bubble.style.SetProperty("overflow-wrap", "break-word")
	bubble.style.FontSize = style.Length{Value: 13, Unit: "px"}
	bubble.AddChild(fs)

	// msg-item：flex row 容器（无明确 width，宽 = 父块自然宽）
	item := mkFlex()
	item.style.FontSize = style.Length{Value: 13, Unit: "px"}
	item.AddChild(bubble)

	// 外层块容器（模拟右侧面板内容区 500px）
	outer := mkBlock()
	outer.style.Width = style.Length{Value: 500, Unit: "px"}
	outer.AddChild(item)

	root := mkBlock()
	root.AddChild(outer)
	Layout(root, 500, 300)

	// 收集「完成摘要」的 segments，按行 Y 分组
	var segs []TextSegment
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, ch := range b.Children() {
			if itb, ok := ch.(*InlineTextBox); ok {
				if itb.Text() == "完成摘要" {
					segs = append(segs, itb.TextSegments...)
				}
				continue
			}
			if eb, ok := ch.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(root)
	if len(segs) == 0 {
		t.Fatal("no segments produced for the CJK title")
	}
	lines := map[int]int{}
	for _, s := range segs {
		lines[int(s.Y/4)]++
	}
	if len(lines) > 1 {
		t.Fatalf("wide container: 完成摘要 split onto %d lines (one char per line bug), want 1 line", len(lines))
	}
}
