package layout

import (
	"strings"
	"testing"

	"wb-ui/style"
)

// layoutWordBreakText lays out a single long word inside a narrow container
// and returns the per-line segment texts (grouped by line Y).
func layoutWordBreakText(t *testing.T, css string, text string) []string {
	t.Helper()
	box := mkBlock()
	if strings.Contains(css, "width:60px") {
		box.style.Width = style.Length{Value: 60, Unit: "px"}
	}
	box.style.FontSize = style.Length{Value: 10, Unit: "px"}
	if strings.Contains(css, "word-break:break-all") {
		box.style.SetProperty("word-break", "break-all")
	}
	if strings.Contains(css, "overflow-wrap:break-word") {
		box.style.SetProperty("overflow-wrap", "break-word")
	}
	tb := &InlineTextBox{text: text, style: box.style}
	box.AddChild(tb)

	root := mkBlock()
	root.AddChild(box)
	Layout(root, 120, 200)

	// Collect text segments, grouped by line Y.
	var segs []TextSegment
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, ch := range b.Children() {
			if childTB, ok := ch.(*InlineTextBox); ok {
				segs = append(segs, childTB.TextSegments...)
				continue
			}
			if eb, ok := ch.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)

	lines := map[int][]string{}
	for _, s := range segs {
		key := int(s.Y / 4) // bucket by ~line height
		lines[key] = append(lines[key], text[s.Start:s.Start+s.Len])
	}
	var keys []int
	for k := range lines {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var out []string
	for _, k := range keys {
		out = append(out, strings.Join(lines[k], ""))
	}
	return out
}

// TestWordBreakAll: a 20-char word in a 60px container with
// word-break:break-all must be split across multiple lines (not overflow).
func TestWordBreakAll(t *testing.T) {
	word := "AAAAAAAAAAAAAAAAAAAA"
	lines := layoutWordBreakText(t, "width:60px; word-break:break-all; font-size:10px;", word)
	if len(lines) < 2 {
		t.Fatalf("word-break:break-all produced %d line(s), want >=2 (word split)", len(lines))
	}
	joined := strings.Join(lines, "")
	if joined != word {
		t.Fatalf("joined=%q want %q", joined, word)
	}
}

// TestOverflowWrapBreakWord: same long word with overflow-wrap:break-word
// also splits across lines.
func TestOverflowWrapBreakWord(t *testing.T) {
	word := "AAAAAAAAAAAAAAAAAAAA"
	withBreak := layoutWordBreakText(t, "width:60px; overflow-wrap:break-word; font-size:10px;", word)
	if len(withBreak) < 2 {
		t.Fatalf("overflow-wrap:break-word produced %d line(s), want >=2", len(withBreak))
	}
}

// TestCJKDefaultLineBreak: a space-less CJK phrase ("完成摘要") in a narrow
// container must wrap per character BY DEFAULT — in browsers every CJK
// ideograph is a soft-wrap opportunity even without word-break:break-word.
// Regression: wb-ui treated the whole space-less CJK run as one unbreakable
// word, so a 4-char title in a cramped flex header stayed on one line (and
// overlapped its siblings) while the browser split it across two lines.
func TestCJKDefaultLineBreak(t *testing.T) {
	box := mkBlock()
	box.style.Width = style.Length{Value: 20, Unit: "px"}
	box.style.FontSize = style.Length{Value: 12, Unit: "px"}
	word := "完成摘要"
	tb := &InlineTextBox{text: word, style: box.style}
	box.AddChild(tb)

	root := mkBlock()
	root.AddChild(box)
	Layout(root, 120, 200)

	var segs []TextSegment
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, ch := range b.Children() {
			if childTB, ok := ch.(*InlineTextBox); ok {
				segs = append(segs, childTB.TextSegments...)
				continue
			}
			if eb, ok := ch.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)

	lines := map[int]string{}
	wordRunes := []rune(word)
	for _, s := range segs {
		key := int(s.Y / 4)
		lines[key] += string(wordRunes[s.Start : s.Start+s.Len])
	}
	if len(lines) < 2 {
		t.Fatalf("CJK text in 40px container produced %d line(s), want >=2 (default per-char wrapping)", len(lines))
	}
}

// TestCJKBreakInFlexItem: the real "folded-summary" structure — a flex row
// holding a chevron span, a CJK title span ("完成摘要") and a long summary
// span, laid out in a narrow container — must wrap the CJK title per
// character (browser default), not keep it as one unbreakable word that
// overflows the flex row and overlaps its siblings.
func TestCJKBreakInFlexItem(t *testing.T) {
	inlineSpan := func(text string) *ElementBox {
		cs := style.NewComputedStyle()
		cs.Display = style.DisplayInline
		cs.FontSize = style.Length{Value: 12, Unit: "px"}
		b := &ElementBox{nodeType: NodeGenericElement, style: cs}
		tb := &InlineTextBox{text: text, style: cs}
		b.AddChild(tb)
		return b
	}

	container := mkFlex()
	container.style.Width = style.Length{Value: 130, Unit: "px"}
	container.style.FontSize = style.Length{Value: 12, Unit: "px"}

	container.AddChild(inlineSpan("▸"))
	title := inlineSpan("完成摘要")
	container.AddChild(title)
	container.AddChild(inlineSpan("已为你完成全部请求并生成了完整摘要，共修改 12 个文件。"))

	root := mkBlock()
	root.AddChild(container)
	Layout(root, 200, 200)

	// Collect segments belonging to the CJK title span.
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
	walk(container)
	if len(segs) == 0 {
		t.Fatal("no segments produced for the CJK title")
	}
	// Group segments by line Y: two or more distinct Y buckets mean the
	// title wrapped onto multiple lines (per-char CJK breaking).
	lines := map[int]int{}
	wordRunes := []rune("完成摘要")
	for _, s := range segs {
		lines[int(s.Y/4)] += len(wordRunes[s.Start : s.Start+s.Len])
	}
	if len(lines) < 2 {
		t.Fatalf("CJK title in narrow flex row produced %d line(s), want >=2 (per-char wrap)", len(lines))
	}
}

// TestWordBreakNoneOverflow: without break rules the word stays on one line
// (overflowing the 60px box) — preserves existing behavior.
func TestWordBreakNoneOverflow(t *testing.T) {
	word := "AAAAAAAAAAAAAAAAAAAA"
	lines := layoutWordBreakText(t, "width:60px; font-size:10px;", word)
	if len(lines) != 1 {
		t.Fatalf("no break rule: %d line(s), want 1 (overflow)", len(lines))
	}
}

// TestWordBreakInheritedCJK: a space-less CJK paragraph (no whitespace
// between characters, so the whole text is one "word") must wrap per
// character when word-break:break-word is INHERITED from an ancestor.
// Regression: word-break is a CSS inherited property but was only stored in
// the raw Properties map (not InheritedData), so descendants lost it and the
// CJK text overflowed the container.
func TestWordBreakInheritedCJK(t *testing.T) {
	parent := mkBlock()
	parent.style.WordBreak = "break-word" // as the resolver would set it
	child := mkBlock()
	child.style.InheritFrom(parent.style)
	child.style.FontSize = style.Length{Value: 10, Unit: "px"}
	child.style.Width = style.Length{Value: 60, Unit: "px"}
	// 20 CJK chars, no spaces: a single "word" wider than the container.
	word := "这是一段没有空格的长中文文本用来验证逐字断行"
	tb := &InlineTextBox{text: word, style: child.style}
	child.AddChild(tb)

	root := mkBlock()
	root.AddChild(child)
	Layout(root, 120, 200)

	var segs []TextSegment
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, ch := range b.Children() {
			if childTB, ok := ch.(*InlineTextBox); ok {
				segs = append(segs, childTB.TextSegments...)
				continue
			}
			if eb, ok := ch.(*ElementBox); ok {
				walk(eb)
			}		}
	}
	walk(child)
	if len(segs) == 0 {
		t.Fatal("no segments produced")
	}
	lines := map[int]string{}
	for _, s := range segs {
		lines[int(s.Y/4)] += string([]rune(word)[s.Start : s.Start+s.Len])
	}
	if len(lines) < 2 {
		t.Fatalf("inherited word-break:break-word produced %d line(s), want >=2 (CJK text must wrap)", len(lines))
	}
	var keys []int
	for k := range lines {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var joined string
	for _, k := range keys {
		joined += lines[k]
	}
	if joined != word {
		t.Fatalf("joined=%q want %q", joined, word)
	}
}

// layoutPreText lays out text inside a white-space:pre box and returns the
// per-line rendered strings (spaces preserved, explicit \n breaks).
func layoutPreText(t *testing.T, text string) []string {
	t.Helper()
	box := mkBlock()
	box.style.WhiteSpace = style.WhiteSpacePre
	box.style.FontSize = style.Length{Value: 10, Unit: "px"}
	box.style.Width = style.Length{Value: 60, Unit: "px"}
	tb := &InlineTextBox{text: text, style: box.style}
	box.AddChild(tb)

	root := mkBlock()
	root.AddChild(box)
	Layout(root, 120, 200)

	var segs []TextSegment
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, ch := range b.Children() {
			if childTB, ok := ch.(*InlineTextBox); ok {
				segs = append(segs, childTB.TextSegments...)
				continue
			}
			if eb, ok := ch.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)

	// Group segments by line Y (spaces are separate 1-char segments).
	lines := map[int][]string{}
	for _, s := range segs {
		key := int(s.Y / 4)
		lines[key] = append(lines[key], text[s.Start:s.Start+s.Len])
	}
	var keys []int
	for k := range lines {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var out []string
	for _, k := range keys {
		out = append(out, strings.Join(lines[k], ""))
	}
	return out
}

// TestWhiteSpacePre: white-space:pre preserves consecutive spaces (no
// collapsing) and breaks lines ONLY at explicit \n — never soft-wraps.
// Regression: pre was treated as normal → spaces collapsed + soft-wrap at
// any width overflow → terminal rows lost spacing / split mid-row.
func TestWhiteSpacePre(t *testing.T) {
	lines := layoutPreText(t, "aa  bb\ncc   dd")
	if len(lines) != 2 {
		t.Fatalf("white-space:pre produced %d line(s), want 2 (only \\n breaks)", len(lines))
	}
	if lines[0] != "aa  bb" {
		t.Fatalf("line0=%q want %q (spaces preserved)", lines[0], "aa  bb")
	}
	if lines[1] != "cc   dd" {
		t.Fatalf("line1=%q want %q (spaces preserved)", lines[1], "cc   dd")
	}
}

// TestWhiteSpacePreNoSoftWrap: pre must NOT soft-wrap when content exceeds
// the container width (unlike normal/pre-wrap). A long single word stays on
// one line and overflows.
func TestWhiteSpacePreNoSoftWrap(t *testing.T) {
	lines := layoutPreText(t, "AAAAAAAAAAAAAAAAAAAA")
	if len(lines) != 1 {
		t.Fatalf("white-space:pre soft-wrapped %d line(s), want 1 (no soft wrap)", len(lines))
	}
}

// TestMinContentBreakAll: min-content of a long English word under
// word-break:break-all is per-character (one char width), NOT the whole word
// — otherwise flex cross-axis fit-content (max(min-content, avail)) is pinned
// to the word width and the item overflows its container instead of wrapping.
// Regression: 弹幕气泡 .text word-break:break-all 的 52 字符英文词把气泡撑
// 到 817px（容器 420px），文字横着溢出被裁。
func TestMinContentBreakAll(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayInline
	cs.FontSize = style.Length{Value: 10, Unit: "px"}
	cs.SetProperty("word-break", "break-all")
	box := &ElementBox{nodeType: NodeGenericElement, style: cs}
	word := "SUPERCALIFRAGILISTICEXPIALIDOCIOUS"
	tb := &InlineTextBox{text: word, style: cs}
	box.AddChild(tb)

	mc := minContentWidth(box)
	full := measureText(box, word)
	if mc >= full {
		t.Fatalf("minContentWidth(break-all)=%.1f should be much smaller than full word %.1f", mc, full)
	}
	if mc > full/float64(len([]rune(word)))+2 {
		t.Fatalf("minContentWidth(break-all)=%.1f: want ≈ single-char width, got whole-word-ish", mc)
	}
}

// TestMinContentBreakAllNoWrap: word-break:break-all under white-space:nowrap
// must NOT make min-content per-character (nowrap forbids all soft wrap —
// same rule as InlineFormattingContext), so min-content stays the word width.
func TestMinContentBreakAllNoWrap(t *testing.T) {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayInline
	cs.FontSize = style.Length{Value: 10, Unit: "px"}
	cs.SetProperty("word-break", "break-all")
	cs.WhiteSpace = style.WhiteSpaceNoWrap
	box := &ElementBox{nodeType: NodeGenericElement, style: cs}
	word := "SUPERCALIFRAGILISTICEXPIALIDOCIOUS"
	tb := &InlineTextBox{text: word, style: cs}
	box.AddChild(tb)

	mc := minContentWidth(box)
	full := measureText(box, word)
	if mc < full-0.5 {
		t.Fatalf("minContentWidth(nowrap+break-all)=%.1f want full word %.1f (nowrap forbids char breaks)", mc, full)
	}
}

