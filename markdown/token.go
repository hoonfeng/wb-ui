package markdown

// Translation of: markdown-it/lib/token.mjs
// Completeness: 100%
//
// Token represents a single node in the markdown-it token stream. The stream is
// a flat ordered list of tokens; block tokens nest via open/close nesting
// markers (Nesting = +1 / -1), and inline tokens may carry Children for things
// like emphasis containing text.
//
// Notes on fidelity to the JS source:
//   - markdown-it stores attrs as `[ [name, value], ... ]` or null. In Go we
//     use a nilable slice [][2]string, so a null attrs maps to nil.
//   - markdown-it stores map as `[start, end]` or null. Because line 0 is a
//     valid source line, we cannot use a value [2]int (its zero value would be
//     indistinguishable from "lines 0..0"). We use *[2]int so nil means "no
//     map", matching the JS null check `if (token.map)`.

// Token is the unit of output produced by the block and inline parsers.
type Token struct {
	Type     string      // e.g. "paragraph_open", "text", "heading_open"
	Tag      string      // HTML tag name, e.g. "p", "h1", "ul" (empty for virtual)
	Attrs    [][2]string // HTML attributes as [name, value] pairs (nil = none)
	Map      *[2]int     // source map [startLine, endLine] (0-based); nil when unset
	Nesting  int        // 1 = open, -1 = close, 0 = self-closing
	Level     int        // nesting level, used to keep open/close balanced
	Children []Token    // child tokens (inline only)
	Content  string      // text content
	Markup   string      // markup string, e.g. "##", "**", "```"
	Info     string      // fence info string, e.g. "go", "javascript"
	Meta     interface{} // arbitrary metadata
	Block    bool        // is this a block-level token
	Hidden   bool        // should this token be hidden from output
}

// NewToken creates a token with the given type/tag/nesting, mirroring the JS
// constructor `new Token(type, tag, nesting)`.
func NewToken(typ, tag string, nesting int) *Token {
	return &Token{
		Type:    typ,
		Tag:     tag,
		Nesting: nesting,
	}
}

// Open reports whether this is an opening token (nesting === 1). Mirrors the
// `token.open` getter in markdown-it.
func (t *Token) Open() bool { return t.Nesting == 1 }

// Close reports whether this is a closing token (nesting === -1). Mirrors the
// `token.close` getter in markdown-it.
func (t *Token) Close() bool { return t.Nesting == -1 }

// AttrIndex returns the index of the attribute with the given name, or -1 if
// not present. Mirrors Token.prototype.attrIndex.
func (t *Token) AttrIndex(name string) int {
	attrs := t.Attrs
	for i, a := range attrs {
		if a[0] == name {
			return i
		}
	}
	return -1
}

// AddAttr pushes a new [name, value] attribute onto the token, mirroring
// Token.prototype.attrPush. Unlike AttrSet it never overwrites an existing
// attribute of the same name.
func (t *Token) AddAttr(name, val string) {
	t.Attrs = append(t.Attrs, [2]string{name, val})
}

// AttrSet sets the attribute name to value, creating it if it does not exist,
// mirroring Token.prototype.attrSet.
func (t *Token) AttrSet(name, val string) {
	idx := t.AttrIndex(name)
	if idx < 0 {
		t.AddAttr(name, val)
	} else {
		t.Attrs[idx][1] = val
	}
}

// AttrGet returns the value of the attribute name, or "" if not present.
// Mirrors Token.prototype.attrGet (which returns null; "" is the Go analogue
// for a missing string).
func (t *Token) AttrGet(name string) string {
	idx := t.AttrIndex(name)
	if idx >= 0 {
		return t.Attrs[idx][1]
	}
	return ""
}

// AttrJoin appends value to the existing attribute name joined by a space, or
// creates the attribute if absent. Mirrors Token.prototype.attrJoin.
func (t *Token) AttrJoin(name, val string) {
	idx := t.AttrIndex(name)
	if idx < 0 {
		t.AddAttr(name, val)
	} else {
		t.Attrs[idx][1] = t.Attrs[idx][1] + " " + val
	}
}
