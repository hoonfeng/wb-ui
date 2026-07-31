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

// TestWordBreakNoneOverflow: without break rules the word stays on one line
// (overflowing the 60px box) — preserves existing behavior.
func TestWordBreakNoneOverflow(t *testing.T) {
	word := "AAAAAAAAAAAAAAAAAAAA"
	lines := layoutWordBreakText(t, "width:60px; font-size:10px;", word)
	if len(lines) != 1 {
		t.Fatalf("no break rule: %d line(s), want 1 (overflow)", len(lines))
	}
}
