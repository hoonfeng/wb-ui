package editor

import (
	"strings"
	"testing"
)

func TestTextFromString_Empty(t *testing.T) {
	txt := TextFromString("")
	if txt.Length() != 0 {
		t.Errorf("Length() = %d, want 0", txt.Length())
	}
	if txt.Lines() != 1 {
		t.Errorf("Lines() = %d, want 1", txt.Lines())
	}
	if txt.String() != "" {
		t.Errorf("String() = %q, want %q", txt.String(), "")
	}
}

func TestTextFromString_SingleLine(t *testing.T) {
	txt := TextFromString("hello world")
	if txt.Length() != 11 {
		t.Errorf("Length() = %d, want 11", txt.Length())
	}
	if txt.Lines() != 1 {
		t.Errorf("Lines() = %d, want 1", txt.Lines())
	}
	if txt.String() != "hello world" {
		t.Errorf("String() = %q, want %q", txt.String(), "hello world")
	}
}

func TestTextFromString_MultiLine(t *testing.T) {
	txt := TextFromString("line1\nline2\nline3")
	if txt.Length() != 17 {
		t.Errorf("Length() = %d, want 17", txt.Length())
	}
	if txt.Lines() != 3 {
		t.Errorf("Lines() = %d, want 3", txt.Lines())
	}
	if txt.String() != "line1\nline2\nline3" {
		t.Errorf("String() = %q", txt.String())
	}
}

func TestTextFromString_TrailingNewline(t *testing.T) {
	// "a\n" has 1 '\n' → 2 lines (the trailing empty line counts)
	txt := TextFromString("a\n")
	if txt.Length() != 2 {
		t.Errorf("Length() = %d, want 2", txt.Length())
	}
	if txt.Lines() != 2 {
		t.Errorf("Lines() = %d, want 2", txt.Lines())
	}
}

func TestTextFromString_Multibyte(t *testing.T) {
	// "你好\n世界" — 4 runes + 1 newline = 5 runes total
	txt := TextFromString("你好\n世界")
	if txt.Length() != 5 {
		t.Errorf("Length() = %d, want 5", txt.Length())
	}
	if txt.Lines() != 2 {
		t.Errorf("Lines() = %d, want 2", txt.Lines())
	}
}

func TestTextOf(t *testing.T) {
	txt := TextOf([]string{"abc", "def", "ghi"})
	if txt.String() != "abc\ndef\nghi" {
		t.Errorf("String() = %q", txt.String())
	}
	if txt.Length() != 11 {
		t.Errorf("Length() = %d, want 11", txt.Length())
	}
	if txt.Lines() != 3 {
		t.Errorf("Lines() = %d, want 3", txt.Lines())
	}
}

func TestTextOf_Empty(t *testing.T) {
	txt := TextOf(nil)
	if txt.Length() != 0 {
		t.Errorf("Length() = %d, want 0", txt.Length())
	}
	if txt.Lines() != 1 {
		t.Errorf("Lines() = %d, want 1", txt.Lines())
	}
}

func TestTextLeaf_SliceString(t *testing.T) {
	txt := TextFromString("hello world")
	cases := []struct {
		from, to int
		want     string
	}{
		{0, 5, "hello"},
		{6, 11, "world"},
		{0, 11, "hello world"},
		{0, 0, ""},
		{11, 11, ""},
	}
	for _, c := range cases {
		got := txt.SliceString(c.from, c.to)
		if got != c.want {
			t.Errorf("SliceString(%d, %d) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestTextLeaf_SliceString_Multibyte(t *testing.T) {
	txt := TextFromString("你好世界")
	got := txt.SliceString(1, 3)
	if got != "好世" {
		t.Errorf("SliceString(1, 3) = %q, want %q", got, "好世")
	}
}

func TestTextLeaf_Slice(t *testing.T) {
	txt := TextFromString("hello world")
	s := txt.Slice(0, 5)
	if s.String() != "hello" {
		t.Errorf("Slice(0,5).String() = %q, want %q", s.String(), "hello")
	}
}

func TestTextLeaf_Replace(t *testing.T) {
	txt := TextFromString("hello world")
	// Replace "world" with "Go"
	r := txt.Replace(6, 11, TextFromString("Go"))
	if r.String() != "hello Go" {
		t.Errorf("Replace = %q, want %q", r.String(), "hello Go")
	}
	if r.Length() != 8 {
		t.Errorf("Replace Length = %d, want 8", r.Length())
	}
}

func TestTextLeaf_Replace_InsertAtStart(t *testing.T) {
	txt := TextFromString("world")
	r := txt.Replace(0, 0, TextFromString("hello "))
	if r.String() != "hello world" {
		t.Errorf("Replace = %q, want %q", r.String(), "hello world")
	}
}

func TestTextLeaf_Replace_DeleteRange(t *testing.T) {
	txt := TextFromString("hello world")
	r := txt.Replace(5, 11, TextFromString(""))
	if r.String() != "hello" {
		t.Errorf("Replace = %q, want %q", r.String(), "hello")
	}
}

func TestTextLeaf_Append(t *testing.T) {
	a := TextFromString("foo")
	b := TextFromString("bar")
	c := a.Append(b)
	if c.String() != "foobar" {
		t.Errorf("Append = %q, want %q", c.String(), "foobar")
	}
	if c.Length() != 6 {
		t.Errorf("Append Length = %d, want 6", c.Length())
	}
}

func TestTextLeaf_Append_Empty(t *testing.T) {
	a := TextFromString("foo")
	if a.Append(Empty()).String() != "foo" {
		t.Errorf("Append(Empty) failed")
	}
	if Empty().Append(a).String() != "foo" {
		t.Errorf("Empty.Append failed")
	}
}

func TestTextLeaf_LineAt(t *testing.T) {
	txt := TextFromString("line1\nline2\nline3")
	// Position 0 → line 1
	l := txt.LineAt(0)
	if l.Number != 1 || l.Text != "line1" || l.From != 0 || l.To != 5 {
		t.Errorf("LineAt(0) = %+v", l)
	}
	// Position 5 (the '\n') → still line 1
	l = txt.LineAt(5)
	if l.Number != 1 {
		t.Errorf("LineAt(5).Number = %d, want 1", l.Number)
	}
	// Position 6 → line 2
	l = txt.LineAt(6)
	if l.Number != 2 || l.Text != "line2" || l.From != 6 || l.To != 11 {
		t.Errorf("LineAt(6) = %+v", l)
	}
	// Position 12 → line 3
	l = txt.LineAt(12)
	if l.Number != 3 || l.Text != "line3" {
		t.Errorf("LineAt(12) = %+v", l)
	}
}

func TestTextLeaf_LineAt_EmptyLine(t *testing.T) {
	txt := TextFromString("a\n\nb")
	// Position 2 → the empty line (line 2)
	l := txt.LineAt(2)
	if l.Number != 2 || l.Text != "" || l.From != 2 || l.To != 2 {
		t.Errorf("LineAt(2) = %+v", l)
	}
}

func TestTextLeaf_LineN(t *testing.T) {
	txt := TextFromString("line1\nline2\nline3")
	l := txt.LineN(2)
	if l.Number != 2 || l.Text != "line2" || l.From != 6 || l.To != 11 {
		t.Errorf("LineN(2) = %+v", l)
	}
	l = txt.LineN(3)
	if l.Number != 3 || l.Text != "line3" {
		t.Errorf("LineN(3) = %+v", l)
	}
}

func TestTextLeaf_LineN_OutOfRange(t *testing.T) {
	txt := TextFromString("abc\ndef")
	// Line 10 should clamp to last line (2)
	l := txt.LineN(10)
	if l.Number != 2 {
		t.Errorf("LineN(10).Number = %d, want 2 (clamped)", l.Number)
	}
	// Line 0 should clamp to first line (1)
	l = txt.LineN(0)
	if l.Number != 1 {
		t.Errorf("LineN(0).Number = %d, want 1 (clamped)", l.Number)
	}
}

func TestTextLeaf_Iter(t *testing.T) {
	txt := TextFromString("hello")
	it := txt.Iter()
	if !it.Next() {
		t.Fatal("Iter.Next() = false, want true")
	}
	if it.Value() != "hello" {
		t.Errorf("Iter.Value() = %q, want %q", it.Value(), "hello")
	}
	if it.Next() {
		t.Error("Iter.Next() second call should return false")
	}
}

func TestTextLeaf_MultibyteLineAt(t *testing.T) {
	txt := TextFromString("你好\n世界")
	l := txt.LineAt(0)
	if l.Number != 1 || l.Text != "你好" || l.From != 0 || l.To != 2 {
		t.Errorf("LineAt(0) = %+v", l)
	}
	l = txt.LineAt(3)
	if l.Number != 2 || l.Text != "世界" || l.From != 3 || l.To != 5 {
		t.Errorf("LineAt(3) = %+v", l)
	}
}

func TestTextNode_LargeDocument(t *testing.T) {
	// Create a large document that should form a TextNode.
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + itoa(i)
	}
	txt := TextOf(lines)
	if txt.Lines() != 100 {
		t.Errorf("Lines() = %d, want 100", txt.Lines())
	}
	// Verify line lookup works.
	l := txt.LineN(50)
	if l.Number != 50 || l.Text != "line 49" {
		t.Errorf("LineN(50) = %+v", l)
	}
	// Verify String() reconstructs the full document.
	want := strings.Join(lines, "\n")
	if txt.String() != want {
		t.Errorf("String() mismatch")
	}
}

func TestTextNode_SliceString(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line" + itoa(i)
	}
	txt := TextOf(lines)
	// Slice across multiple lines. [0,11) = "line0\nline1" (11 runes).
	s := txt.SliceString(0, 11)
	if s != "line0\nline1" {
		t.Errorf("SliceString(0, 11) = %q, want %q", s, "line0\nline1")
	}
}

func TestTextNode_Replace(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line" + itoa(i)
	}
	txt := TextOf(lines)
	r := txt.Replace(0, 5, TextFromString("XXXX"))
	// "line0" → "XXXX", so first line becomes "XXXX"
	if r.String()[:4] != "XXXX" {
		t.Errorf("Replace result prefix = %q", r.String()[:10])
	}
}

func TestTextNode_Append(t *testing.T) {
	a := TextOf([]string{"a", "b", "c"})
	b := TextOf([]string{"d", "e", "f"})
	c := a.Append(b)
	if c.String() != "a\nb\ncd\ne\nf" {
		// Note: Append doesn't add a separator between a and b.
		// "a\nb\nc" + "d\ne\nf" = "a\nb\ncd\ne\nf"
		t.Errorf("Append = %q", c.String())
	}
}

// itoa is a simple int-to-string helper for tests (avoids strconv import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
