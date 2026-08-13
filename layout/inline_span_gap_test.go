package layout

// 回归测试：相邻 span/文本段之间必须无缝衔接（CM6 编辑器的 token span 场景）。
// 根因历史：pre 模式下空格 segment 之后，下一个 word 仍叠加 spaceWidth →
// " = " 渲染为 "  ="（span 间异常空隙，每个空隙 = 一个空格宽）。
import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// spanSegments lays out the given root element and collects all inline text
// segments in document order with their owner node name.
type spanSegment struct {
	text  string
	x, w  float64
	owner string
}

func collectSpanSegments(t *testing.T, rootEl *dom.Element, viewportW int) []spanSegment {
	t.Helper()
	resolver := style.NewResolver()
	root := BuildLayoutTree(rootEl, resolver)
	if root == nil {
		t.Fatal("root is nil")
	}
	state := Layout(root.(*ElementBox), viewportW, 600)
	if state == nil {
		t.Fatal("state is nil")
	}
	var segs []spanSegment
	var walk func(b Box)
	walk = func(b Box) {
		if tb, ok := b.(*InlineTextBox); ok && len(tb.TextSegments) > 0 {
			owner := "?"
			if n := tb.Node(); n != nil {
				owner = n.NodeName()
			}
			for _, sg := range tb.TextSegments {
				runes := []rune(tb.text)
				sub := ""
				if sg.Start < len(runes) {
					e := sg.Start + sg.Len
					if e > len(runes) {
						e = len(runes)
					}
					sub = string(runes[sg.Start:e])
				}
				segs = append(segs, spanSegment{sub, sg.X, sg.Width, owner})
			}
		}
		if eb, ok := b.(*ElementBox); ok {
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(root)
	return segs
}

func setupSkiaMeasure() func() {
	prevM, prevF := MeasureTextFunc, FontMetricsFunc
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	MeasureTextFunc = func(family string, size float64, weight int, fstyle, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: fstyle}, text)
	}
	FontMetricsFunc = func(family string, size float64, weight int, fstyle string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: fstyle}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	return func() { MeasureTextFunc, FontMetricsFunc = prevM, prevF }
}

// assertContiguous fails if any segment does not start exactly where the
// previous segment ended (within float epsilon).
func assertContiguous(t *testing.T, segs []spanSegment) {
	t.Helper()
	if len(segs) == 0 {
		t.Fatal("no segments")
	}
	prevRight := segs[0].x + segs[0].w
	for i := 1; i < len(segs); i++ {
		delta := segs[i].x - prevRight
		if delta > 0.02 {
			t.Fatalf("seg[%d] %q: gap of %.3fpx after %q (x=%.3f prevRight=%.3f)",
				i, segs[i].text, delta, segs[i-1].text, segs[i].x, prevRight)
		}
		if delta < -0.02 {
			t.Fatalf("seg[%d] %q: overlap of %.3fpx after %q (x=%.3f prevRight=%.3f)",
				i, segs[i].text, -delta, segs[i-1].text, segs[i].x, prevRight)
		}
		prevRight = segs[i].x + segs[i].w
	}
}

// TestInlineSpanAdjacency_ConsecutiveWhitespaceNodes normal 模式下两个相邻
// 空白文本节点只折叠成一个空格（"foo" + " " + " bar" → "foo bar"）。
func TestInlineSpanAdjacency_ConsecutiveWhitespaceNodes(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()
	doc := dom.NewDocument()
	p := doc.CreateElement("p")
	p.SetAttribute("style", "font-family:monospace;font-size:14px")
	p.AppendChild(doc.CreateTextNode("foo"))
	p.AppendChild(doc.CreateTextNode(" "))
	p.AppendChild(doc.CreateTextNode(" bar"))
	doc.AppendChild(p)

	segs := collectSpanSegments(t, p, 800)
	if len(segs) == 0 {
		t.Fatal("no segments")
	}
	want := MeasureTextFunc("monospace", 14, 400, "normal", "foo bar")
	got := segs[len(segs)-1].x + segs[len(segs)-1].w - segs[0].x
	if diff := got - want; diff > 0.02 || diff < -0.02 {
		t.Fatalf("total width %.3f, want %.3f (diff %.3f) — double space?", got, want, diff)
	}
}

// TestInlineSpanAdjacency_PreSpaces 复现 CM6 结构：.cm-line（white-space:pre）
// 下 token span 与含空格的裸文本节点（" = "、"; // "）交错。空格 segment 后
// 的单词曾多叠加一个 spaceWidth，产生 1 空格宽的空隙。
func TestInlineSpanAdjacency_PreSpaces(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()
	doc := dom.NewDocument()
	line := doc.CreateElement("div")
	line.SetAttribute("style", "font-family:monospace;font-size:14px;white-space:pre")
	mkSpan := func(text string) *dom.Element {
		s := doc.CreateElement("span")
		s.AppendChild(doc.CreateTextNode(text))
		return s
	}
	line.AppendChild(mkSpan("const"))
	line.AppendChild(doc.CreateTextNode(" "))
	line.AppendChild(mkSpan("foo"))
	line.AppendChild(doc.CreateTextNode(" = "))
	line.AppendChild(mkSpan("42"))
	line.AppendChild(doc.CreateTextNode("; // "))
	line.AppendChild(mkSpan("注释"))
	doc.AppendChild(line)

	segs := collectSpanSegments(t, line, 800)
	if len(segs) == 0 {
		t.Fatal("no segments")
	}
	assertContiguous(t, segs)

	// 结构完整性：文本顺序拼接 = "const foo = 42; // 注释"
	var sb []rune
	for _, s := range segs {
		sb = append(sb, []rune(s.text)...)
	}
	if got := string(sb); got != "const foo = 42; // 注释" {
		t.Fatalf("concatenated segments = %q, want %q", got, "const foo = 42; // 注释")
	}
}

// TestInlineSpanAdjacency_NormalSpaces normal（折叠）模式下 span 间空格节点
// 只贡献一个空格：<span>foo</span> <span>bar</span> → "foo bar"。
func TestInlineSpanAdjacency_NormalSpaces(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()
	doc := dom.NewDocument()
	p := doc.CreateElement("p")
	p.SetAttribute("style", "font-family:monospace;font-size:14px")
	mkSpan := func(text string) *dom.Element {
		s := doc.CreateElement("span")
		s.AppendChild(doc.CreateTextNode(text))
		return s
	}
	p.AppendChild(mkSpan("foo"))
	p.AppendChild(doc.CreateTextNode(" "))
	p.AppendChild(mkSpan("bar"))
	p.AppendChild(doc.CreateTextNode("  baz"))
	doc.AppendChild(p)

	segs := collectSpanSegments(t, p, 800)
	if len(segs) == 0 {
		t.Fatal("no segments")
	}
	// normal 模式：空格被折叠，段与段之间允许恰好一个空格宽的间隙。
	spaceW := MeasureTextFunc("monospace", 14, 400, "normal", " ")
	prevRight := segs[0].x + segs[0].w
	for i := 1; i < len(segs); i++ {
		delta := segs[i].x - prevRight
		if delta > 0.02 {
			if delta < spaceW-0.02 || delta > spaceW+0.02 {
				t.Fatalf("seg[%d] %q: unexpected gap %.3fpx (want 0 or one space %.3f)",
					i, segs[i].text, delta, spaceW)
			}
		} else if delta < -0.02 {
			t.Fatalf("seg[%d] %q: overlap %.3fpx", i, segs[i].text, -delta)
		}
		prevRight = segs[i].x + segs[i].w
	}
	// normal 模式空格被折叠：无空格 segment，只有单词段（"foobarbaz"），
	// 空格体现为段间恰好一个 spaceWidth 的间隙。
	if len(segs) != 3 {
		t.Fatalf("expected 3 word segments (spaces collapsed), got %d", len(segs))
	}
	if got := segs[0].text + segs[1].text + segs[2].text; got != "foobarbaz" {
		t.Fatalf("concatenated segments = %q, want %q", got, "foobarbaz")
	}
	// 校验整体宽度 = "foo bar baz" 的 Skia 测量宽（不允许双空格）。
	want := MeasureTextFunc("monospace", 14, 400, "normal", "foo bar baz")
	got := segs[len(segs)-1].x + segs[len(segs)-1].w - segs[0].x
	if diff := got - want; diff > 0.02 || diff < -0.02 {
		t.Fatalf("total width %.3f, want %.3f (diff %.3f)", got, want, diff)
	}
}
