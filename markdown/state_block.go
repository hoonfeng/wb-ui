package markdown

// Translation of: markdown-it/lib/rules_block/state_block.mjs
// Completeness: 100%
//
// StateBlock is the state object for the block rule chain. It pre-computes
// per-line metadata (begin/end byte offsets, indent, tab-expanded indent) into
// parallel slices so that block rules can jump to line boundaries in O(1).
//
// Faithfulness notes:
//   - markdown-it uses charCodeAt on the JS string (UTF-16 code units). We use
//     byte offsets into the Go UTF-8 string. This is correct because block
//     rules only test for ASCII characters (0x0A, 0x09, 0x20, #, >, -, *, `),
//     and in UTF-8 those byte values never appear as continuation bytes of a
//     multi-byte character. All offset arithmetic is done consistently in
//     bytes, so line slicing via Src[start:end] yields valid UTF-8.

// StateBlock is the state passed to block chain rules. Each rule receives
// (state, startLine, endLine, silent).
type StateBlock struct {
	Src string // source Markdown text (UTF-8)
	Md  *MarkdownIt
	Env map[string]interface{}

	Tokens []Token

	// Per-line byte offsets and metadata (indexed by line number).
	BMarks []int // line begin byte offset
	EMarks []int // line end byte offset (exclusive of newline)
	TShift []int // byte offset of first non-space char from line start
	SCount []int // expanded indent (tabs → spaces) for the line
	BsCount []int // virtual spaces for tab expansion offset

	BlkIndent  int    // required block content indent (e.g. inside list)
	Line       int    // current line index being processed
	LineMax    int    // total line count (excluding fake tail entry)
	Tight      bool   // loose/tight mode for lists
	DdIndent   int    // indent of current dd block (-1 = none)
	ListIndent int    // indent of current list block (-1 = none)
	ParentType string // 'root', 'blockquote', 'list', 'paragraph', 'reference'
	Level      int    // nesting level for open/close balance
}

// NewStateBlock creates a StateBlock, pre-computing the line metadata caches.
// Mirrors the StateBlock constructor.
func NewStateBlock(src string, md *MarkdownIt, env map[string]interface{}, tokens []Token) *StateBlock {
	st := &StateBlock{
		Src:        src,
		Md:         md,
		Env:        env,
		Tokens:     tokens,
		BMarks:     make([]int, 0),
		EMarks:     make([]int, 0),
		TShift:     make([]int, 0),
		SCount:     make([]int, 0),
		BsCount:    make([]int, 0),
		BlkIndent:  0,
		Tight:      false,
		DdIndent:   -1,
		ListIndent: -1,
		ParentType: "root",
	}

	s := st.Src
	length := len(s)

	start := 0
	pos := 0
	indent := 0
	offset := 0
	indentFound := false

	for pos < length {
		ch := rune(s[pos])
		if !indentFound {
			if IsSpace(ch) {
				indent++
				if ch == 0x09 /* \t */ {
					offset += 4 - offset%4
				} else {
					offset++
				}
				pos++
				continue
			} else {
				indentFound = true
			}
		}
		if ch == 0x0A || pos == length-1 {
			if ch != 0x0A {
				pos++
			}
			st.BMarks = append(st.BMarks, start)
			st.EMarks = append(st.EMarks, pos)
			st.TShift = append(st.TShift, indent)
			st.SCount = append(st.SCount, offset)
			st.BsCount = append(st.BsCount, 0)
			indentFound = false
			indent = 0
			offset = 0
			start = pos + 1
		}
		pos++
	}

	// Push fake entry to simplify cache bounds checks
	st.BMarks = append(st.BMarks, length)
	st.EMarks = append(st.EMarks, length)
	st.TShift = append(st.TShift, 0)
	st.SCount = append(st.SCount, 0)
	st.BsCount = append(st.BsCount, 0)
	st.LineMax = len(st.BMarks) - 1 // don't count last fake line

	return st
}

// Push creates a new block-level token and appends it to the token stream.
// Mirrors StateBlock.prototype.push. The returned pointer references the
// element inside st.Tokens (not a heap copy), so callers can set fields like
// Content/Map/Children on it and have the changes reflected in the stream.
func (st *StateBlock) Push(typ, tag string, nesting int) *Token {
	token := NewToken(typ, tag, nesting)
	token.Block = true
	if nesting < 0 {
		st.Level-- // closing tag
	}
	token.Level = st.Level
	if nesting > 0 {
		st.Level++ // opening tag
	}
	st.Tokens = append(st.Tokens, *token)
	return &st.Tokens[len(st.Tokens)-1]
}

// IsEmpty reports whether line `line` is blank (contains only whitespace).
// Mirrors StateBlock.prototype.isEmpty.
func (st *StateBlock) IsEmpty(line int) bool {
	return st.BMarks[line]+st.TShift[line] >= st.EMarks[line]
}

// SkipEmptyLines returns the first non-empty line at or after `from`.
// Mirrors StateBlock.prototype.skipEmptyLines.
func (st *StateBlock) SkipEmptyLines(from int) int {
	for max := st.LineMax; from < max; from++ {
		if st.BMarks[from]+st.TShift[from] < st.EMarks[from] {
			break
		}
	}
	return from
}

// SkipSpaces returns the position of the first non-space character at or after
// `pos`. Mirrors StateBlock.prototype.skipSpaces.
func (st *StateBlock) SkipSpaces(pos int) int {
	for max := len(st.Src); pos < max; pos++ {
		if !IsSpace(rune(st.Src[pos])) {
			break
		}
	}
	return pos
}

// SkipSpacesBack returns the position of the last non-space character at or
// before `pos`, but not before `min`. Mirrors StateBlock.prototype.skipSpacesBack.
func (st *StateBlock) SkipSpacesBack(pos, min int) int {
	if pos <= min {
		return pos
	}
	for pos > min {
		pos--
		if !IsSpace(rune(st.Src[pos])) {
			return pos + 1
		}
	}
	return pos
}

// SkipChars returns the position of the first character at or after `pos` that
// is not equal to `code`. Mirrors StateBlock.prototype.skipChars.
func (st *StateBlock) SkipChars(pos int, code rune) int {
	for max := len(st.Src); pos < max; pos++ {
		if rune(st.Src[pos]) != code {
			break
		}
	}
	return pos
}

// SkipCharsBack returns the position after the last character equal to `code`
// at or before `pos`, but not before `min`. Mirrors StateBlock.prototype.skipCharsBack.
func (st *StateBlock) SkipCharsBack(pos int, code rune, min int) int {
	if pos <= min {
		return pos
	}
	for pos > min {
		pos--
		if code != rune(st.Src[pos]) {
			return pos + 1
		}
	}
	return pos
}

// GetLines extracts the text of lines [begin, end) with leading indentation
// stripped up to `indent` columns (tabs expanded). If keepLastLF is true, the
// last line includes its trailing newline.
// Mirrors StateBlock.prototype.getLines.
func (st *StateBlock) GetLines(begin, end, indent int, keepLastLF bool) string {
	if begin >= end {
		return ""
	}

	queue := make([]string, end-begin)
	i := 0
	for line := begin; line < end; line, i = line+1, i+1 {
		lineIndent := 0
		lineStart := st.BMarks[line]
		first := lineStart
		var last int

		if line+1 < end || keepLastLF {
			// No need for bounds check because we have fake entry on tail.
			last = st.EMarks[line] + 1
		} else {
			last = st.EMarks[line]
		}

		for first < last && lineIndent < indent {
			ch := rune(st.Src[first])
			if IsSpace(ch) {
				if ch == 0x09 /* \t */ {
					lineIndent += 4 - (lineIndent+st.BsCount[line])%4
				} else {
					lineIndent++
				}
			} else if first-lineStart < st.TShift[line] {
				// patched tShift masked characters to look like spaces
				// (blockquotes, list markers)
				lineIndent++
			} else {
				break
			}
			first++
		}

		if lineIndent > indent {
			// partially expanding tabs in code blocks, e.g '\t\tfoobar'
			// with indent=2 becomes '  \tfoobar'
			spaces := ""
			for j := 0; j < lineIndent-indent; j++ {
				spaces += " "
			}
			queue[i] = spaces + st.Src[first:last]
		} else {
			queue[i] = st.Src[first:last]
		}
	}

	result := ""
	for _, s := range queue {
		result += s
	}
	return result
}
