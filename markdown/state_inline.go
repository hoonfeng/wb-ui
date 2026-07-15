package markdown

// Translation of: markdown-it/lib/rules_inline/state_inline.mjs
// Completeness: 100%
//
// StateInline is the state object for the inline rule chain. It scans a single
// line (or paragraph) of text, producing a flat token list. Emphasis-like
// delimiters are accumulated in per-scope delimiter lists so that the
// postProcess rules (balance_pairs, emphasis.postProcess) can match opening
// and closing markers.
//
// Faithfulness notes:
//   - markdown-it uses charCodeAt (UTF-16 code units) for character access.
//     We use []rune (full Unicode code points), so surrogate pair handling
//     in scanDelims is not needed.
//   - JS arrays are reference types; Go slices are value headers. To share
//     delimiter storage between state.delimiters and tokens_meta[i].delimiters
//     (which both must see pushes), we use *[]Delimiter (pointer to slice).
//     Appending via *ptr = append(*ptr, item) is visible through all pointers
//     to the same slice variable.

// Delimiter represents an emphasis-like marker in the delimiter stack. It is
// created by the emphasis/strikethrough tokenize rules and consumed by their
// postProcess rules.
type Delimiter struct {
	Marker rune // the marker character (e.g. '*', '_', '~')
	Length int  // number of consecutive markers
	Token  int  // index into state.tokens of the text token holding the markers
	End    int  // index into the delimiter list of the matching close (-1 = none)
	Open   bool // can this be an opening delimiter?
	Close  bool // can this be a closing delimiter?
	Weak   bool // weak opening/closing (for emphasis parsing edge cases)
}

// TokenMeta holds per-token metadata. Currently only used to store the
// delimiter list for opening tags, so that postProcess rules can access
// delimiters grouped by their containing inline element.
type TokenMeta struct {
	Delimiters *[]Delimiter
}

// ScanDelimsResult is the return value of StateInline.ScanDelims.
type ScanDelimsResult struct {
	CanOpen  bool
	CanClose bool
	Length   int
}

// StateInline is the state passed to inline chain rules. Each rule receives
// (state, silent) (with startLine/endLine ignored per the unified RuleFn type).
type StateInline struct {
	Src     string // original source string
	SrcRunes []rune // rune slice for O(1) indexing; pos/posMax index into this
	Env     map[string]interface{}
	Md      *MarkdownIt
	Tokens  []Token       // output token stream (shared with caller via outTokens)
	TokensMeta []*TokenMeta // per-token metadata (nil for tokens without meta)
	Pos     int // current scan position (rune index)
	PosMax  int // length of SrcRunes (one past last valid index)
	Level   int // nesting level for open/close balance

	Pending      string // accumulated text not yet flushed to a token
	PendingLevel int    // level at which pending text was accumulated

	Cache         map[string]int // backtracking cache (key: "start:end" → end pos)
	Delimiters    *[]Delimiter   // pointer to current scope's delimiter list
	PrevDelimiters []*[]Delimiter // stack of saved delimiter lists (for nesting)

	Backticks        map[int]int // backtick length → last seen position
	BackticksScanned bool        // whether backticks have been pre-scanned

	LinkLevel int // counter to disable linkify inside <a>/markdown links
}

// NewStateInline creates a StateInline, mirroring the JS constructor
// `new StateInline(src, md, env, outTokens)`.
func NewStateInline(src string, md *MarkdownIt, env map[string]interface{}, outTokens []Token) *StateInline {
	runes := []rune(src)
	delims := []Delimiter{}
	return &StateInline{
		Src:              src,
		SrcRunes:         runes,
		Env:              env,
		Md:               md,
		Tokens:           outTokens,
		TokensMeta:       make([]*TokenMeta, len(outTokens)),
		Pos:              0,
		PosMax:           len(runes),
		Level:            0,
		Pending:          "",
		PendingLevel:     0,
		Cache:            make(map[string]int),
		Delimiters:       &delims,
		PrevDelimiters:   nil,
		Backticks:        make(map[int]int),
		BackticksScanned: false,
		LinkLevel:        0,
	}
}

// PushPending flushes the pending text buffer as a 'text' token, mirroring
// StateInline.prototype.pushPending. The returned pointer references the
// element inside st.Tokens so callers can mutate it in place.
func (st *StateInline) PushPending() *Token {
	token := NewToken("text", "", 0)
	token.Content = st.Pending
	token.Level = st.PendingLevel
	st.Tokens = append(st.Tokens, *token)
	st.TokensMeta = append(st.TokensMeta, nil)
	st.Pending = ""
	return &st.Tokens[len(st.Tokens)-1]
}

// Push creates a new inline token. If pending text exists, it is flushed first.
// For opening tags (nesting > 0), the current delimiter list is saved and a
// new empty list is started; for closing tags (nesting < 0), the previous
// delimiter list is restored.
// Mirrors StateInline.prototype.push.
func (st *StateInline) Push(typ, tag string, nesting int) *Token {
	if st.Pending != "" {
		st.PushPending()
	}

	token := NewToken(typ, tag, nesting)
	var tokenMeta *TokenMeta

	if nesting < 0 {
		// closing tag
		st.Level--
		if len(st.PrevDelimiters) > 0 {
			st.Delimiters = st.PrevDelimiters[len(st.PrevDelimiters)-1]
			st.PrevDelimiters = st.PrevDelimiters[:len(st.PrevDelimiters)-1]
		}
	}

	token.Level = st.Level

	if nesting > 0 {
		// opening tag
		st.Level++
		// Save current delimiter list and start a new one.
		st.PrevDelimiters = append(st.PrevDelimiters, st.Delimiters)
		newDelims := []Delimiter{}
		st.Delimiters = &newDelims
		tokenMeta = &TokenMeta{Delimiters: &newDelims}
	}

	st.PendingLevel = st.Level
	st.Tokens = append(st.Tokens, *token)
	st.TokensMeta = append(st.TokensMeta, tokenMeta)
	return &st.Tokens[len(st.Tokens)-1]
}

// ScanDelims scans a sequence of emphasis-like markers starting at position
// `start` and determines whether they can open or close an emphasis sequence.
// `canSplitWord` indicates whether markers inside a word (e.g. `foo*bar`) are
// allowed.
// Mirrors StateInline.prototype.scanDelims.
//
// Note: markdown-it's JS version handles UTF-16 surrogate pairs here. Since we
// use []rune (full code points), this complexity is unnecessary.
func (st *StateInline) ScanDelims(start int, canSplitWord bool) ScanDelimsResult {
	max := st.PosMax
	marker := st.SrcRunes[start]

	// Determine the character before the marker sequence.
	var lastChar rune
	if start == 0 {
		// treat beginning of the line as a whitespace
		lastChar = 0x20
	} else {
		lastChar = st.SrcRunes[start-1]
	}

	// Count consecutive markers.
	pos := start
	for pos < max && st.SrcRunes[pos] == marker {
		pos++
	}
	count := pos - start

	// Determine the character after the marker sequence.
	var nextChar rune
	if pos < max {
		nextChar = st.SrcRunes[pos]
	} else {
		// treat end of the line as a whitespace
		nextChar = 0x20
	}

	isLastPunctChar := IsMdAsciiPunct(lastChar) || IsPunctCharCode(lastChar)
	isNextPunctChar := IsMdAsciiPunct(nextChar) || IsPunctCharCode(nextChar)
	isLastWhiteSpace := IsWhiteSpace(lastChar)
	isNextWhiteSpace := IsWhiteSpace(nextChar)

	leftFlanking := !isNextWhiteSpace && (!isNextPunctChar || isLastWhiteSpace || isLastPunctChar)
	rightFlanking := !isLastWhiteSpace && (!isLastPunctChar || isNextWhiteSpace || isNextPunctChar)

	canOpen := leftFlanking && (canSplitWord || !rightFlanking || isLastPunctChar)
	canClose := rightFlanking && (canSplitWord || !leftFlanking || isNextPunctChar)

	return ScanDelimsResult{
		CanOpen:  canOpen,
		CanClose: canClose,
		Length:   count,
	}
}
