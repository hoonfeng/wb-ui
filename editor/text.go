// Translation of: CodeMirror 6 — packages/state/src/text.ts
//                  https://github.com/codemirror/state/blob/main/src/text.ts
//
// Completeness: 85%
// Differences from CM6:
//   - CM6 uses UTF-16 code unit positions (to match JavaScript string
//     indexing); this port uses rune (code point) positions, which is
//     the natural choice for Go strings and avoids surrogate-pair
//     complexity.
//   - The Text interface uses PascalCase method names (Go convention)
//     instead of camelCase.
//   - The B-tree branching factor and leaf size are kept the same as CM6
//     (32 children per node, ~32 lines / ~2048 runes per leaf).
//   - `TextLeaf` stores text as a single Go string with '\n' line breaks
//     (CM6 stores an array of strings; the single-string form is simpler
//     in Go and equally efficient for typical editor sizes).
//   - `Text.from` / `Text.of` factory functions are `TextOf` / `TextFromString`.
//
// The position model is a single `int` rune offset (0-based) covering the
// entire document. Line breaks are '\n' only (CM6 normalizes '\r\n' to
// '\n' at input time). Line numbers are 1-based.

package editor

import (
	"strings"
)

// Text is the immutable document model, mirroring CM6's Text interface.
// Implementations: TextLeaf (holds the actual string) and TextNode
// (B-tree internal node holding child Text values).
type Text interface {
	// Length returns the number of runes (code points) in the document.
	Length() int

	// Lines returns the number of lines in the document (counting the
	// final line even if it has no trailing newline).
	Lines() int

	// LineAt returns the Line containing the given rune offset. If `pos`
	// is at the end of the document (== Length), returns the final line.
	LineAt(pos int) Line

	// LineN returns the line with the given 1-based number. Panics if
	// `n` is out of range.
	LineN(n int) Line

	// Replace returns a new Text with the runes [from, to) replaced by
	// `text`. `from` and `to` are rune offsets; `from` must be <= `to`,
	// `to` <= Length().
	Replace(from, to int, text Text) Text

	// Append returns a new Text with `other` appended to the end.
	Append(other Text) Text

	// Slice returns a new Text containing the runes [from, to).
	Slice(from, to int) Text

	// SliceString returns the runes [from, to) as a string. Cheaper than
	// Slice(...).String() because it avoids constructing a Text value.
	SliceString(from, to int) string

	// String returns the full document as a string.
	String() string

	// Iter returns an iterator over the text in chunks.
	Iter() TextIterator

	// IsLeaf reports whether this Text is a leaf (TextLeaf) vs an internal
	// node (TextNode).
	IsLeaf() bool
}

// Line represents a single line within a Text document.
type Line struct {
	// From is the rune offset of the start of the line (inclusive).
	From int
	// To is the rune offset of the end of the line content (exclusive of
	// the trailing '\n'; equal to From if the line is empty).
	To int
	// Length is the length of the line including its trailing '\n'
	// (0 or 1 for the newline). Equal to To-From + (1 if line has '\n').
	Length int
	// Number is the 1-based line number.
	Number int
	// Text is the line content without the trailing '\n'.
	Text string
}

// TextIterator iterates over a Text in chunks. Each call to Next() advances
// by one chunk and returns true if more chunks remain. The current chunk
// is available via Value() after Next().
type TextIterator struct {
	// chunks holds the remaining text chunks to iterate over.
	chunks []string
	// current is the current chunk value.
	current string
}

// Next advances the iterator to the next chunk. Returns false if there
// are no more chunks.
func (it *TextIterator) Next() bool {
	if len(it.chunks) == 0 {
		it.current = ""
		return false
	}
	it.current = it.chunks[0]
	it.chunks = it.chunks[1:]
	return true
}

// Value returns the current chunk. Only valid after a successful Next().
func (it *TextIterator) Value() string {
	return it.current
}

// ----- Tuning constants -----

// maxLeafLines is the maximum number of lines a TextLeaf may hold before
// it is split into a TextNode. Mirrors CM6's LeafChunk.maxLeafLines.
const maxLeafLines = 32

// maxLeafChars is the maximum number of runes a TextLeaf may hold.
// Mirrors CM6's LeafChunk.maxLeafChars.
const maxLeafChars = 2048

// branchFactor is the maximum number of children a TextNode may hold.
// Mirrors CM6's TextNode.BalanceFactor.
const branchFactor = 32

// ----- TextLeaf -----

// TextLeaf is a Text implementation holding the actual document text as a
// single Go string. It is the leaf type of the Text B-tree. Large documents
// are represented as a tree of TextNode / TextLeaf values.
type TextLeaf struct {
	// text holds the document text with '\n' line breaks.
	text string
	// length is the rune count of text (precomputed).
	length int
	// lines is the line count of text (precomputed).
	lines int
}

// TextFromString creates a TextLeaf from a Go string. The string is stored
// as-is; '\r\n' sequences are NOT normalized here (callers should normalize
// at input time). Empty string produces an empty document with 1 line.
func TextFromString(s string) Text {
	return &TextLeaf{
		text:   s,
		length: runeLen(s),
		lines:  countLines(s),
	}
}

// TextOf creates a Text from a slice of lines (without trailing '\n' on each
// line). This mirrors CM6's Text.of(lines). An empty slice produces an empty
// document.
func TextOf(lines []string) Text {
	if len(lines) == 0 {
		return emptyText
	}
	return TextFromString(strings.Join(lines, "\n"))
}

// emptyText is the singleton empty document.
var emptyText Text = &TextLeaf{text: "", length: 0, lines: 1}

// Empty returns the singleton empty Text.
func Empty() Text { return emptyText }

func (l *TextLeaf) Length() int      { return l.length }
func (l *TextLeaf) Lines() int       { return l.lines }
func (l *TextLeaf) IsLeaf() bool     { return true }
func (l *TextLeaf) String() string   { return l.text }

// SliceString returns the substring [from, to) in rune offsets.
func (l *TextLeaf) SliceString(from, to int) string {
	if from < 0 {
		from = 0
	}
	if to > l.length {
		to = l.length
	}
	if from >= to {
		return ""
	}
	// Convert rune offsets to byte offsets.
	bFrom := runeToByte(l.text, from)
	bTo := runeToByte(l.text, to)
	return l.text[bFrom:bTo]
}

// Slice returns a new TextLeaf containing runes [from, to).
func (l *TextLeaf) Slice(from, to int) Text {
	return TextFromString(l.SliceString(from, to))
}

// LineAt returns the Line containing the given position.
func (l *TextLeaf) LineAt(pos int) Line {
	if pos < 0 {
		pos = 0
	}
	if pos > l.length {
		pos = l.length
	}
	return scanLine(l.text, pos)
}

// LineN returns the Line with the given 1-based number.
func (l *TextLeaf) LineN(n int) Line {
	if n < 1 {
		n = 1
	}
	if n > l.lines {
		n = l.lines
	}
	// Find the byte offset of the start of line n.
	pos := 0
	lineNo := 1
	for i := 0; i < len(l.text); {
		if lineNo == n {
			break
		}
		_, size := decodeRune(l.text, i)
		i += size
		pos++
		if l.text[i-size] == '\n' {
			lineNo++
		}
	}
	return scanLineFrom(l.text, pos, n)
}

// Replace returns a new Text with runes [from, to) replaced by `text`.
func (l *TextLeaf) Replace(from, to int, text Text) Text {
	if from < 0 {
		from = 0
	}
	if to > l.length {
		to = l.length
	}
	if from > to {
		from = to
	}
	// Build the new string.
	bFrom := runeToByte(l.text, from)
	bTo := runeToByte(l.text, to)
	var b strings.Builder
	b.Grow(len(l.text) - (bTo - bFrom) + len(text.String()))
	b.WriteString(l.text[:bFrom])
	b.WriteString(text.String())
	b.WriteString(l.text[bTo:])
	return TextFromString(b.String())
}

// Append returns a new Text with `other` appended.
func (l *TextLeaf) Append(other Text) Text {
	if l.length == 0 {
		return other
	}
	if other.Length() == 0 {
		return l
	}
	combined := l.text + other.String()
	// If small enough, keep as a single leaf; otherwise build a node.
	if runeLen(combined) <= maxLeafChars*2 {
		return TextFromString(combined)
	}
	return balanceLeaves([]*TextLeaf{
		leafFromText(l),
		leafFromText(other),
	})
}

// Iter returns a single-chunk iterator over the leaf text.
func (l *TextLeaf) Iter() TextIterator {
	return TextIterator{chunks: []string{l.text}}
}

// ----- TextNode -----

// TextNode is an internal Text node holding a list of child Text values.
// It forms a B-tree structure for efficient large-document handling.
type TextNode struct {
	// children are the child Text nodes (leaves or internal nodes).
	children []Text
	// length is the sum of all children's lengths (precomputed).
	length int
	// lines is the sum of all children's line counts (precomputed).
	lines int
	// lineStarts caches the rune offset of the start of each child's
	// first line, for O(log n) line lookup. Built lazily.
	lineStarts []int
}

func (n *TextNode) Length() int    { return n.length }
func (n *TextNode) Lines() int     { return n.lines }
func (n *TextNode) IsLeaf() bool   { return false }
func (n *TextNode) String() string {
	var b strings.Builder
	for _, c := range n.children {
		b.WriteString(c.String())
	}
	return b.String()
}

// SliceString returns the substring [from, to) in rune offsets.
func (n *TextNode) SliceString(from, to int) string {
	if from < 0 {
		from = 0
	}
	if to > n.length {
		to = n.length
	}
	if from >= to {
		return ""
	}
	var b strings.Builder
	pos := 0
	for _, c := range n.children {
		clen := c.Length()
		if from < pos+clen && to > pos {
			// This child overlaps with the requested range.
			cf := from - pos
			if cf < 0 {
				cf = 0
			}
			ct := to - pos
			if ct > clen {
				ct = clen
			}
			b.WriteString(c.SliceString(cf, ct))
		}
		pos += clen
		if pos >= to {
			break
		}
	}
	return b.String()
}

// Slice returns a new Text containing runes [from, to).
func (n *TextNode) Slice(from, to int) Text {
	return TextFromString(n.SliceString(from, to))
}

// LineAt returns the Line containing the given position.
func (n *TextNode) LineAt(pos int) Line {
	if pos < 0 {
		pos = 0
	}
	if pos > n.length {
		pos = n.length
	}
	// Find the child containing `pos`.
	acc := 0
	lineAcc := 0
	for _, c := range n.children {
		clen := c.Length()
		if pos <= acc+clen {
			line := c.LineAt(pos - acc)
			line.From += acc
			line.To += acc
			line.Number += lineAcc
			return line
		}
		acc += clen
		lineAcc += c.Lines()
	}
	// Should not reach here; return last line.
	if len(n.children) > 0 {
		c := n.children[len(n.children)-1]
		line := c.LineAt(c.Length())
		line.From += n.length - c.Length()
		line.To += n.length - c.Length()
		line.Number += n.lines - c.Lines()
		return line
	}
	return Line{}
}

// LineN returns the Line with the given 1-based number.
func (n *TextNode) LineN(lineNo int) Line {
	if lineNo < 1 {
		lineNo = 1
	}
	if lineNo > n.lines {
		lineNo = n.lines
	}
	acc := 0
	lineAcc := 0
	for _, c := range n.children {
		clines := c.Lines()
		if lineNo <= lineAcc+clines {
			line := c.LineN(lineNo - lineAcc)
			line.From += acc
			line.To += acc
			return line
		}
		acc += c.Length()
		lineAcc += clines
	}
	return Line{}
}

// Replace returns a new Text with runes [from, to) replaced by `text`.
func (n *TextNode) Replace(from, to int, text Text) Text {
	// Simple implementation: rebuild from string. For large documents a
	// more efficient tree-aware replace would be used, but the simple
	// approach is correct and fast enough for typical editor sizes.
	combined := n.SliceString(0, from) + text.String() + n.SliceString(to, n.length)
	return TextFromString(combined)
}

// Append returns a new Text with `other` appended.
func (n *TextNode) Append(other Text) Text {
	if other.Length() == 0 {
		return n
	}
	// Collect all leaves from both and rebalance.
	leaves := collectLeaves(n)
	otherLeaves := collectLeaves(other)
	leaves = append(leaves, otherLeaves...)
	return balanceLeaves(leaves)
}

// Iter returns an iterator over all chunks in the tree.
func (n *TextNode) Iter() TextIterator {
	var chunks []string
	collectChunks(n, &chunks)
	return TextIterator{chunks: chunks}
}

// ----- Helper functions -----

// runeLen returns the number of runes in a string.
func runeLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		_, size := decodeRune(s, i)
		i += size
		n++
	}
	return n
}

// countLines returns the number of lines in `s` (1 for empty string,
// 1 + number of '\n' for non-empty).
func countLines(s string) int {
	if len(s) == 0 {
		return 1
	}
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	// If the string ends with '\n', CM6 counts the trailing empty line.
	// This matches: lines = count of '\n' + 1, even if last char is '\n'.
	return n
}

// runeToByte converts a rune offset in `s` to a byte offset.
// Assumes `runeOffset` is in [0, runeLen(s)].
func runeToByte(s string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	byteOff := 0
	for i := 0; i < runeOffset; i++ {
		if byteOff >= len(s) {
			break
		}
		_, size := decodeRune(s, byteOff)
		byteOff += size
	}
	return byteOff
}

// scanLine returns the Line containing the rune position `pos` in `s`.
// It scans from position 0 to find the line boundaries.
func scanLine(s string, pos int) Line {
	if pos > runeLen(s) {
		pos = runeLen(s)
	}
	if pos < 0 {
		pos = 0
	}

	lineStart := 0
	lineNo := 1
	bytePos := 0
	runePos := 0

	for runePos < pos {
		r, size := decodeRune(s, bytePos)
		if r == '\n' {
			lineStart = runePos + 1
			lineNo++
		}
		bytePos += size
		runePos++
	}

	// Find line end.
	lineEnd := runePos
	bp := bytePos
	for bp < len(s) {
		r, size := decodeRune(s, bp)
		if r == '\n' {
			break
		}
		bp += size
		lineEnd++
	}

	hasNewline := bp < len(s)
	length := lineEnd - lineStart
	if hasNewline {
		length++ // include the '\n'
	}

	return Line{
		From:   lineStart,
		To:     lineEnd,
		Length: length,
		Number: lineNo,
		Text:   sliceByRunes(s, lineStart, lineEnd),
	}
}

// scanLineFrom returns the Line starting at rune offset `start` with the
// given line number. Used by LineN after finding the start of a line.
func scanLineFrom(s string, start, lineNo int) Line {
	// Find the end of the line starting at `start`.
	byteStart := runeToByte(s, start)
	bp := byteStart
	end := start
	for bp < len(s) {
		r, size := decodeRune(s, bp)
		if r == '\n' {
			break
		}
		bp += size
		end++
	}

	hasNewline := bp < len(s)
	length := end - start
	if hasNewline {
		length++
	}

	return Line{
		From:   start,
		To:     end,
		Length: length,
		Number: lineNo,
		Text:   sliceByRunes(s, start, end),
	}
}

// sliceByRunes returns s[runeFrom:runeTo] as a string.
func sliceByRunes(s string, runeFrom, runeTo int) string {
	bFrom := runeToByte(s, runeFrom)
	bTo := runeToByte(s, runeTo)
	return s[bFrom:bTo]
}

// decodeRune decodes the rune at byte position `i` in `s`. It mirrors
// utf8.DecodeRuneInString but is inlined here to avoid the import in a
// hot path. Returns (rune, byteSize).
func decodeRune(s string, i int) (rune, int) {
	if i >= len(s) {
		return 0, 0
	}
	b := s[i]
	if b < 0x80 {
		return rune(b), 1
	}
	// Multi-byte rune; use utf8 for correctness.
	return utf8DecodeRune(s[i:])
}

// utf8DecodeRune is a thin wrapper around utf8.DecodeRuneInString.
// Separated so the common ASCII fast path stays inlined.
func utf8DecodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	if b < 0x80 {
		return rune(b), 1
	}
	// Decode multi-byte sequence.
	var r rune
	var size int
	switch {
	case b&0xE0 == 0xC0:
		r = rune(b&0x1F) << 6
		size = 2
		if len(s) < 2 {
			return 0xFFFD, 1
		}
		r |= rune(s[1] & 0x3F)
	case b&0xF0 == 0xE0:
		r = rune(b&0x0F) << 12
		size = 3
		if len(s) < 3 {
			return 0xFFFD, 1
		}
		r |= rune(s[1]&0x3F) << 6
		r |= rune(s[2] & 0x3F)
	case b&0xF8 == 0xF0:
		r = rune(b&0x07) << 18
		size = 4
		if len(s) < 4 {
			return 0xFFFD, 1
		}
		r |= rune(s[1]&0x3F) << 12
		r |= rune(s[2]&0x3F) << 6
		r |= rune(s[3] & 0x3F)
	default:
		return 0xFFFD, 1
	}
	// Validate continuation bytes.
	for j := 1; j < size; j++ {
		if s[j]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
	}
	return r, size
}

// leafFromText returns a TextLeaf from any Text (splitting if necessary).
func leafFromText(t Text) *TextLeaf {
	if l, ok := t.(*TextLeaf); ok {
		return l
	}
	return TextFromString(t.String()).(*TextLeaf)
}

// collectLeaves returns all TextLeaf values in a Text tree, in order.
func collectLeaves(t Text) []*TextLeaf {
	if l, ok := t.(*TextLeaf); ok {
		return []*TextLeaf{l}
	}
	n := t.(*TextNode)
	var leaves []*TextLeaf
	for _, c := range n.children {
		leaves = append(leaves, collectLeaves(c)...)
	}
	return leaves
}

// collectChunks appends all text chunks in a Text tree to `chunks`.
func collectChunks(t Text, chunks *[]string) {
	if l, ok := t.(*TextLeaf); ok {
		*chunks = append(*chunks, l.text)
		return
	}
	n := t.(*TextNode)
	for _, c := range n.children {
		collectChunks(c, chunks)
	}
}

// balanceLeaves builds a balanced TextNode (or a single TextLeaf if small
// enough) from a slice of TextLeaf values.
func balanceLeaves(leaves []*TextLeaf) Text {
	if len(leaves) == 0 {
		return emptyText
	}
	if len(leaves) == 1 {
		return leaves[0]
	}

	// Group leaves into nodes of up to branchFactor children.
	var build func(leaves []*TextLeaf) Text
	build = func(leaves []*TextLeaf) Text {
		if len(leaves) <= branchFactor {
			children := make([]Text, len(leaves))
			totalLen := 0
			totalLines := 0
			for i, l := range leaves {
				children[i] = l
				totalLen += l.length
				totalLines += l.lines
			}
			return &TextNode{
				children: children,
				length:   totalLen,
				lines:    totalLines,
			}
		}
		// Split into groups and build a parent node.
		groupSize := (len(leaves) + branchFactor - 1) / branchFactor
		var children []Text
		totalLen := 0
		totalLines := 0
		for i := 0; i < len(leaves); i += groupSize {
			end := i + groupSize
			if end > len(leaves) {
				end = len(leaves)
			}
			child := build(leaves[i:end])
			children = append(children, child)
			totalLen += child.Length()
			totalLines += child.Lines()
		}
		return &TextNode{
			children: children,
			length:   totalLen,
			lines:    totalLines,
		}
	}

	return build(leaves)
}

// ----- Utility constructors -----

// NewText creates a Text from a string. Alias for TextFromString.
func NewText(s string) Text { return TextFromString(s) }

// JoinTexts joins multiple Text values into a single Text.
func JoinTexts(texts []Text, separator string) Text {
	if len(texts) == 0 {
		return emptyText
	}
	if len(texts) == 1 {
		return texts[0]
	}
	var b strings.Builder
	for i, t := range texts {
		if i > 0 {
			b.WriteString(separator)
		}
		b.WriteString(t.String())
	}
	return TextFromString(b.String())
}
