// Translation of: Source/WebCore/css/parser/CSSTokenizer.cpp
//                  Source/WebCore/css/parser/CSSTokenizer.h
//                  Source/WebCore/css/parser/CSSParserToken.h
//                  Source/WebCore/css/parser/CSSParserToken.cpp
//                  Source/WebCore/css/parser/CSSTokenizerInputStream.cpp
// Completeness: 80%
// Simplifications:
//   - no @container support
//   - no nested at-rules beyond @media
//   - tokens are stored as Go structs rather than the bit-packed C++ representation
//   - escapes resolve eagerly into Go strings; no separate string pool for adoption
//   - the input stream is a Go string with explicit UTF-8 rune decoding rather than
//     WebKit's Latin-1/UChar dual-mode CSSTokenizerInputStream
//   - block tracking is a simple []TokenType stack instead of a TokenType vector
//   - HashToken id/unrestricted distinction is preserved; numeric value is float64

package css

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// TokenType mirrors CSSParserTokenType in CSSParserToken.h. The order matches the
// enumeration order in WebKit so that tests can compare numeric values when needed.
type TokenType int

const (
	TokenIdent TokenType = iota
	TokenFunction
	TokenAtKeyword
	TokenHash
	TokenURL
	TokenBadURL
	TokenDelimiter
	TokenNumber
	TokenPercentage
	TokenDimension
	TokenIncludeMatch    // ~=
	TokenDashMatch       // |=
	TokenPrefixMatch     // ^=
	TokenSuffixMatch     // $=
	TokenSubstringMatch  // *=
	TokenColumn          // ||
	TokenNonNewlineWhitespace
	TokenNewline
	TokenCDO // <!--
	TokenCDC // -->
	TokenColon
	TokenSemicolon
	TokenComma
	TokenLeftParenthesis
	TokenRightParenthesis
	TokenLeftBracket
	TokenRightBracket
	TokenLeftBrace
	TokenRightBrace
	TokenString
	TokenBadString
	TokenEOF
	TokenComment
	tokenLast
)

// NumericSign mirrors NumericSign in CSSParserToken.h.
type NumericSign int

const (
	SignNone NumericSign = iota
	SignPlus
	SignMinus
)

// NumericValueType mirrors NumericValueType in CSSParserToken.h.
type NumericValueType int

const (
	ValueInteger NumericValueType = iota
	ValueNumber
)

// HashTokenType mirrors HashTokenType in CSSParserToken.h.
type HashTokenType int

const (
	HashID HashTokenType = iota
	HashUnrestricted
)

// BlockType mirrors CSSParserToken::BlockType.
type BlockType int

const (
	BlockNotBlock BlockType = iota
	BlockStart
	BlockEnd
)

// Token is the Go translation of CSSParserToken. Fields are stored as plain Go values
// rather than the bit-packed C++ representation; this trades density for clarity. The
// Value field carries the textual payload (identifier, string, unit, etc.) and Numeric
// holds the parsed numeric value when Type is TokenNumber / TokenPercentage /
// TokenDimension.
type Token struct {
	Type         TokenType
	BlockType    BlockType
	Value        string
	Numeric      float64
	NumericSign  NumericSign
	NumericValue NumericValueType
	Unit         string
	HashType     HashTokenType
	Delimiter    rune
}

// Tokenizer is the Go translation of WebCore::CSSTokenizer. It runs the CSS Syntax
// Module Level 3 tokenizer over an input string and produces a slice of Token values
// terminated by an EOF token. The implementation follows the spec's "consume a token"
// algorithm; each helper method corresponds to a "consume an X" algorithm in the spec.
type Tokenizer struct {
	input  string
	pos    int
	tokens []Token
	// blockStack tracks the kind of block currently being consumed (paren/brace/bracket)
	// so that the matching close token can be tagged BlockEnd and mirrored against its
	// opener. Mirrors CSSTokenizer::m_blockStack.
	blockStack []TokenType
}

// NewTokenizer constructs a Tokenizer over input. The CSS Syntax spec mandates
// preprocessing the input stream (replacing CR, FF, and CRLF with LF) before
// tokenizing; Preprocess performs that step.
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{input: Preprocess(input)}
}

// Preprocess performs the CSS input-stream preprocessing specified in
// https://www.w3.org/TR/css-syntax-3/#input-preprocessing: CR, FF, and CRLF are
// replaced with LF, and surrogates are replaced with U+FFFD. Mirrors
// CSSTokenizer::preprocessString.
func Preprocess(s string) string {
	if !strings.ContainsAny(s, "\r\f\x00") && !hasSurrogate(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	runeSlice := []rune(s)
	skip := make([]bool, len(runeSlice))
	for i, r := range runeSlice {
		if skip[i] {
			continue
		}
		switch r {
		case '\r':
			if i+1 < len(runeSlice) && runeSlice[i+1] == '\n' {
				b.WriteByte('\n')
				skip[i+1] = true // consume the LF of the CRLF
				continue
			}
			b.WriteByte('\n')
		case '\f':
			b.WriteByte('\n')
		case 0xD800, 0xDBFF, 0xDC00, 0xDFFF:
			b.WriteRune(0xFFFD)
		case 0:
			b.WriteRune(0xFFFD)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasSurrogate reports whether s contains any UTF-16 surrogate code point.
func hasSurrogate(s string) bool {
	for _, r := range s {
		if r >= 0xD800 && r <= 0xDFFF {
			return true
		}
	}
	return false
}

// Tokenize runs the tokenizer to completion and returns the produced tokens (the
// final token is always EOF). Mirrors CSSTokenizer's behavior of fully materializing
// the token stream up-front so that the parser can operate on a TokenRange view.
func (t *Tokenizer) Tokenize() []Token {
	for {
		tok := t.nextToken()
		t.tokens = append(t.tokens, tok)
		if tok.Type == TokenEOF {
			return t.tokens
		}
	}
}

// Tokens returns the tokens produced so far. If Tokenize has not been called the
// slice is empty.
func (t *Tokenizer) Tokens() []Token { return t.tokens }

// --- Input primitives -------------------------------------------------------

// peek returns the rune at offset off without advancing the position.
func (t *Tokenizer) peek(off int) rune {
	idx := t.pos + off
	if idx < 0 || idx >= len(t.input) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(t.input[idx:])
	return r
}

// peekAt returns the rune at the absolute byte index. Returns -1 if out of range.
func (t *Tokenizer) peekAt(idx int) rune {
	if idx < 0 || idx >= len(t.input) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(t.input[idx:])
	return r
}

// consume returns the next rune and advances the position.
func (t *Tokenizer) consume() rune {
	if t.pos >= len(t.input) {
		return -1
	}
	r, size := utf8.DecodeRuneInString(t.input[t.pos:])
	t.pos += size
	return r
}

// reconsume pushes the position back by one rune. It assumes the previous read was a
// single rune and that there is something to back up over.
func (t *Tokenizer) reconsume(r rune) {
	_ = r
	if t.pos > 0 {
		_, size := utf8.DecodeLastRuneInString(t.input[:t.pos])
		t.pos -= size
	}
}

// consumeIfNext advances past c if it is the next rune and reports whether it did.
// Mirrors CSSTokenizer::consumeIfNext.
func (t *Tokenizer) consumeIfNext(c rune) bool {
	if t.peek(0) == c {
		t.consume()
		return true
	}
	return false
}

// consumeSingleWhitespaceIfNext consumes one whitespace rune if the next rune is
// whitespace. Mirrors CSSTokenizer::consumeSingleWhitespaceIfNext.
func (t *Tokenizer) consumeSingleWhitespaceIfNext() {
	if isCSSWhitespace(t.peek(0)) {
		t.consume()
	}
}

// consumeUntilCommentEndFound consumes runes until "*/" is seen, mirroring
// CSSTokenizer::consumeUntilCommentEndFound. It does not produce a token; the caller
// decides whether to emit a CommentToken.
func (t *Tokenizer) consumeUntilCommentEndFound() {
	for {
		r := t.consume()
		if r == -1 {
			return
		}
		if r == '*' && t.peek(0) == '/' {
			t.consume()
			return
		}
	}
}

// isCSSWhitespace reports whether r is CSS whitespace per the spec (newline, tab,
// space).
func isCSSWhitespace(r rune) bool {
	switch r {
	case '\n', '\t', ' ':
		return true
	}
	return false
}

// isNameStartCodePoint reports whether r may start a CSS identifier per the
// "ident-start code point" rule in the spec.
func isNameStartCodePoint(r rune) bool {
	if r == '_' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	return r >= 0x80
}

// isNameCodePoint reports whether r may appear in the body of a CSS identifier.
func isNameCodePoint(r rune) bool {
	if isNameStartCodePoint(r) {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	return r == '-'
}

// isHexDigit reports whether r is a hexadecimal digit.
func isHexDigit(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'f' {
		return true
	}
	if r >= 'A' && r <= 'F' {
		return true
	}
	return false
}

// hexValue converts a hex rune to its numeric value.
func hexValue(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return 0
}

// --- Token dispatch ---------------------------------------------------------

// nextToken implements the "consume a token" algorithm from CSS Syntax Level 3,
// mirroring CSSTokenizer::nextToken.
func (t *Tokenizer) nextToken() Token {
	r := t.consume()
	if r == -1 {
		return Token{Type: TokenEOF}
	}
	switch {
	case isCSSWhitespace(r):
		return t.consumeWhitespace(r)
	case r == '"':
		return t.consumeStringUntil('"')
	case r == '\'':
		return t.consumeStringUntil('\'')
	case r == '#':
		return t.consumeHash()
	case r == '(':
		return t.blockStartToken(TokenLeftParenthesis)
	case r == ')':
		return t.blockEndToken(TokenRightParenthesis)
	case r == '+':
		if t.nextCharsAreNumber() {
			t.reconsume(r)
			return t.consumeNumericToken()
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == ',':
		return Token{Type: TokenComma}
	case r == '-':
		return t.consumeHyphenMinus(r)
	case r == '.':
		if t.nextCharsAreNumber() {
			t.reconsume(r)
			return t.consumeNumericToken()
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == ':':
		return Token{Type: TokenColon}
	case r == ';':
		return Token{Type: TokenSemicolon}
	case r == '<':
		return t.consumeCDO(r)
	case r == '@':
		return t.consumeAtKeyword()
	case r == '[':
		return t.blockStartToken(TokenLeftBracket)
	case r == ']':
		return t.blockEndToken(TokenRightBracket)
	case r == '{':
		return t.blockStartToken(TokenLeftBrace)
	case r == '}':
		return t.blockEndToken(TokenRightBrace)
	case r == '\\':
		return t.consumeIdentLikeToken(r)
	case r == '*':
		if t.consumeIfNext('=') {
			return Token{Type: TokenSubstringMatch}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == '/':
		if t.peek(0) == '*' {
			t.consume()
			t.consumeUntilCommentEndFound()
			// WebKit's tokenizer drops comment tokens; spec says to emit them. We emit
			// CommentToken so that the parser can drop them by inspecting the type.
			return Token{Type: TokenComment}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == '^':
		if t.consumeIfNext('=') {
			return Token{Type: TokenPrefixMatch}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == '$':
		if t.consumeIfNext('=') {
			return Token{Type: TokenSuffixMatch}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == '|':
		if t.consumeIfNext('=') {
			return Token{Type: TokenDashMatch}
		}
		if t.peek(0) == '|' {
			t.consume()
			return Token{Type: TokenColumn}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r == '~':
		if t.consumeIfNext('=') {
			return Token{Type: TokenIncludeMatch}
		}
		return Token{Type: TokenDelimiter, Delimiter: r}
	case r >= '0' && r <= '9':
		t.reconsume(r)
		return t.consumeNumericToken()
	case r == 'U' || r == 'u':
		if t.peek(0) == '+' && (isHexDigit(t.peek(1)) || t.peek(1) == '?') {
			return t.consumeUnicodeRange()
		}
		t.reconsume(r)
		return t.consumeIdentLikeToken(r)
	case isNameStartCodePoint(r):
		t.reconsume(r)
		return t.consumeIdentLikeToken(r)
	}
	return Token{Type: TokenDelimiter, Delimiter: r}
}

// blockStartToken pushes a block-start token onto the token stream and records the
// opener on the block stack. Mirrors CSSTokenizer::blockStart.
func (t *Tokenizer) blockStartToken(tt TokenType) Token {
	t.blockStack = append(t.blockStack, tt)
	return Token{Type: tt, BlockType: BlockStart}
}

// blockEndToken returns a block-end token and pops the matching opener off the block
// stack. Mirrors CSSTokenizer::blockEnd.
func (t *Tokenizer) blockEndToken(tt TokenType) Token {
	if len(t.blockStack) > 0 {
		t.blockStack = t.blockStack[:len(t.blockStack)-1]
	}
	return Token{Type: tt, BlockType: BlockEnd}
}

// consumeWhitespace handles a run of whitespace, collapsing it into a single
// Whitespace token (or Newline if the run began with a newline). Mirrors
// CSSTokenizer::whitespace / newline.
func (t *Tokenizer) consumeWhitespace(r rune) Token {
	wasNewline := r == '\n'
	for isCSSWhitespace(t.peek(0)) {
		if t.peek(0) == '\n' {
			wasNewline = true
		}
		t.consume()
	}
	if wasNewline {
		return Token{Type: TokenNewline}
	}
	return Token{Type: TokenNonNewlineWhitespace}
}

// consumeStringUntil consumes a quoted string token terminated by quote. Implements
// the "consume a string token" algorithm.
func (t *Tokenizer) consumeStringUntil(quote rune) Token {
	var sb strings.Builder
	for {
		r := t.consume()
		switch {
		case r == -1:
			return Token{Type: TokenBadString}
		case r == quote:
			return Token{Type: TokenString, Value: sb.String()}
		case r == '\n':
			// Bad string: newline in unescaped string.
			return Token{Type: TokenBadString}
		case r == '\\':
			next := t.peek(0)
			if next == -1 {
				// End of input; keep going (spec says do nothing).
				continue
			}
			if next == '\n' {
				t.consume()
				continue
			}
			if isCSSWhitespace(next) {
				// Valid escape of whitespace? No—escaped whitespace means nothing;
				// consume the escape as the next code point.
				sb.WriteRune(t.consumeEscape())
				continue
			}
			sb.WriteRune(t.consumeEscape())
		default:
			sb.WriteRune(r)
		}
	}
}

// consumeHash implements the "consume a hash token" algorithm. If the next runes form
// a valid identifier the hash is classified as ID; otherwise it is unrestricted.
func (t *Tokenizer) consumeHash() Token {
	if t.nextCharsAreIdentifier() {
		hashType := HashID
		// "If the next 3 input code points would start an identifier, and the
		// following input code point is not a name code point" — we already know the
		// start matches an identifier; we still need to ensure that the result is a
		// name. In practice the ID classification holds when nextCharsAreIdentifier
		// returns true; otherwise it's unrestricted.
		name := t.consumeName()
		return Token{Type: TokenHash, Value: name, HashType: hashType}
	}
	// Unrestricted: copy name code points only, with escape handling.
	// This stops at non-name characters (whitespace, ;, :, comma, etc.) so that
	// a trailing semicolon from "color: #333;" is not absorbed into the value.
	var sb strings.Builder
	for {
		r := t.peek(0)
		if r == -1 || !isNameCodePoint(r) {
			break
		}
		t.consume()
		if r == '\\' && t.nextTwoCharsAreValidEscape() {
			sb.WriteRune(t.consumeEscape())
			continue
		}
		sb.WriteRune(r)
	}
	return Token{Type: TokenHash, Value: sb.String(), HashType: HashUnrestricted}
}

// consumeHyphenMinus handles the ambiguous '-' code point. It may begin a number, a
// CDC token, an identifier, or be a delimiter.
func (t *Tokenizer) consumeHyphenMinus(r rune) Token {
	if t.peek(0) == '-' && t.peek(1) == '>' {
		t.consume()
		t.consume()
		return Token{Type: TokenCDC}
	}
	if t.nextCharsAreNumber() {
		t.reconsume(r)
		return t.consumeNumericToken()
	}
	if t.nextCharsAreIdentifier() {
		t.reconsume(r)
		return t.consumeIdentLikeToken(r)
	}
	return Token{Type: TokenDelimiter, Delimiter: r}
}

// consumeCDO handles the '<' code point. If followed by "!--" it is a CDO token;
// otherwise a delimiter.
func (t *Tokenizer) consumeCDO(r rune) Token {
	if t.peek(0) == '!' && t.peek(1) == '-' && t.peek(2) == '-' {
		t.consume()
		t.consume()
		t.consume()
		return Token{Type: TokenCDO}
	}
	return Token{Type: TokenDelimiter, Delimiter: r}
}

// consumeAtKeyword implements the "consume an at-keyword token" algorithm.
func (t *Tokenizer) consumeAtKeyword() Token {
	if !t.nextCharsAreIdentifier() {
		return Token{Type: TokenDelimiter, Delimiter: '@'}
	}
	name := t.consumeName()
	return Token{Type: TokenAtKeyword, Value: name}
}

// consumeIdentLikeToken handles the "consume an ident-like token" algorithm: an
// identifier, a function token (ident followed by "("), or a url token.
func (t *Tokenizer) consumeIdentLikeToken(_ rune) Token {
	name := t.consumeName()
	if t.peek(0) == '(' {
		t.consume()
		lower := strings.ToLower(name)
		if lower == "url" {
			return t.consumeURLToken()
		}
		return Token{Type: TokenFunction, Value: name}
	}
	return Token{Type: TokenIdent, Value: name}
}

// consumeName consumes a name per the "consume a name" algorithm. Escapes are
// resolved eagerly into the returned string. The function peeks before consuming
// so that the backslash of an escape can be detected before it is consumed.
func (t *Tokenizer) consumeName() string {
	var sb strings.Builder
	for {
		r := t.peek(0)
		if r == -1 {
			return sb.String()
		}
		switch {
		case isNameCodePoint(r):
			t.consume()
			sb.WriteRune(r)
		case r == '\\' && t.nextTwoCharsAreValidEscape():
			t.consume() // consume the backslash
			sb.WriteRune(t.consumeEscape())
		default:
			return sb.String()
		}
	}
}

// consumeEscape implements the "consume an escaped code point" algorithm. The caller
// has already consumed the leading backslash.
func (t *Tokenizer) consumeEscape() rune {
	r := t.consume()
	if r == -1 {
		return 0xFFFD
	}
	if isHexDigit(r) {
		// Consume up to 5 hex digits.
		hex := string(r)
		for i := 0; i < 5 && isHexDigit(t.peek(0)); i++ {
			hex += string(t.consume())
		}
		if isCSSWhitespace(t.peek(0)) {
			t.consume()
		}
		v, err := strconv.ParseInt(hex, 16, 32)
		if err != nil || v == 0 || v >= 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
			return 0xFFFD
		}
		return rune(v)
	}
	return r
}

// nextTwoCharsAreValidEscape reports whether the next two runes form a valid escape,
// mirroring CSSTokenizer::nextTwoCharsAreValidEscape.
func (t *Tokenizer) nextTwoCharsAreValidEscape() bool {
	if t.peek(0) != '\\' {
		return false
	}
	if t.peek(1) == '\n' {
		return false
	}
	return true
}

// nextCharsAreIdentifier implements the "would start an identifier" check.
func (t *Tokenizer) nextCharsAreIdentifier() bool {
	return t.nextCharsAreIdentifierFrom(0)
}

// nextCharsAreIdentifierFrom is the underlying check starting at offset off,
// implementing the "would start an identifier" algorithm. The lookahead is at most
// 3 code points (e.g. for "-\X" where X is non-newline).
func (t *Tokenizer) nextCharsAreIdentifierFrom(off int) bool {
	r := t.peek(off)
	if r == -1 {
		return false
	}
	if isNameStartCodePoint(r) {
		return true
	}
	if r == '\\' {
		// '\' followed by non-newline.
		next := t.peek(off + 1)
		return next != -1 && next != '\n'
	}
	if r == '-' {
		next := t.peek(off + 1)
		if next == -1 {
			return false
		}
		if isNameStartCodePoint(next) || next == '-' {
			return true
		}
		if next == '\\' {
			// '\' must be followed by non-newline.
			third := t.peek(off + 2)
			return third != -1 && third != '\n'
		}
		return false
	}
	return false
}

// nextCharsAreNumber implements the "would start a number" check, mirroring
// CSSTokenizer::nextCharsAreNumber.
func (t *Tokenizer) nextCharsAreNumber() bool {
	r := t.peek(0)
	if r == -1 {
		return false
	}
	switch {
	case r == '+' || r == '-':
		next := t.peek(1)
		if next == -1 {
			return false
		}
		if next >= '0' && next <= '9' {
			return true
		}
		if next == '.' {
			third := t.peek(2)
			return third >= '0' && third <= '9'
		}
		return false
	case r == '.':
		next := t.peek(1)
		return next >= '0' && next <= '9'
	default:
		return r >= '0' && r <= '9'
	}
}

// consumeNumericToken implements the "consume a numeric token" algorithm: produces a
// Number, Percentage, or Dimension token depending on what follows the number.
func (t *Tokenizer) consumeNumericToken() Token {
	num, sign, isInteger, original := t.consumeNumber()
	tok := Token{Numeric: num, NumericSign: sign}
	if isInteger {
		tok.NumericValue = ValueInteger
	} else {
		tok.NumericValue = ValueNumber
	}
	_ = original
	if t.peek(0) == '%' {
		t.consume()
		tok.Type = TokenPercentage
		return tok
	}
	if t.nextCharsAreIdentifier() {
		unit := t.consumeName()
		tok.Type = TokenDimension
		tok.Unit = unit
		return tok
	}
	tok.Type = TokenNumber
	return tok
}

// consumeNumber implements the "consume a number" algorithm, returning the parsed
// value, its sign, whether it is an integer, and the original textual form.
func (t *Tokenizer) consumeNumber() (float64, NumericSign, bool, string) {
	sign := SignNone
	var sb strings.Builder
	r := t.peek(0)
	if r == '+' || r == '-' {
		t.consume()
		sb.WriteRune(r)
		if r == '+' {
			sign = SignPlus
		} else {
			sign = SignMinus
		}
	}
	intPart := false
	for {
		d := t.peek(0)
		if d < '0' || d > '9' {
			break
		}
		t.consume()
		sb.WriteRune(d)
		intPart = true
	}
	if t.peek(0) == '.' && t.peek(1) >= '0' && t.peek(1) <= '9' {
		t.consume()
		sb.WriteRune('.')
		for {
			d := t.peek(0)
			if d < '0' || d > '9' {
				break
			}
			t.consume()
			sb.WriteRune(d)
		}
	}
	isInteger := !strings.Contains(sb.String(), ".")
	// Exponent.
	if r := t.peek(0); r == 'e' || r == 'E' {
		next := t.peek(1)
		if (next >= '0' && next <= '9') ||
			((next == '+' || next == '-') && t.peek(2) >= '0' && t.peek(2) <= '9') {
			t.consume()
			sb.WriteRune(r)
			if next == '+' || next == '-' {
				t.consume()
				sb.WriteRune(next)
			}
			for {
				d := t.peek(0)
				if d < '0' || d > '9' {
					break
				}
				t.consume()
				sb.WriteRune(d)
			}
			isInteger = false
		}
	}
	val, err := strconv.ParseFloat(sb.String(), 64)
	if err != nil {
		val = 0
	}
	if !intPart && sb.Len() == 0 {
		// No digits were consumed; spec says the number is zero.
		return 0, sign, true, sb.String()
	}
	return val, sign, isInteger, sb.String()
}

// consumeURLToken implements the "consume a url token" algorithm. The leading "url("
// has already been consumed by the caller.
func (t *Tokenizer) consumeURLToken() Token {
	// Skip leading whitespace.
	for isCSSWhitespace(t.peek(0)) {
		t.consume()
	}
	r := t.peek(0)
	switch {
	case r == -1:
		return Token{Type: TokenURL, Value: ""}
	case r == '"' || r == '\'':
		t.consume()
		tok := t.consumeStringUntil(r)
		if tok.Type == TokenBadString {
			t.consumeBadURLRemnants()
			return Token{Type: TokenBadURL}
		}
		// Skip trailing whitespace then expect ).
		for isCSSWhitespace(t.peek(0)) {
			t.consume()
		}
		if t.peek(0) == ')' {
			t.consume()
		} else if t.peek(0) == -1 {
			// EOF before ); spec says this is still a URL token.
		} else {
			t.consumeBadURLRemnants()
			return Token{Type: TokenBadURL}
		}
		return Token{Type: TokenURL, Value: tok.Value}
	default:
		var sb strings.Builder
		for {
			r := t.consume()
			if r == -1 {
				return Token{Type: TokenURL, Value: sb.String()}
			}
			if r == ')' {
				return Token{Type: TokenURL, Value: sb.String()}
			}
			if isCSSWhitespace(r) {
				for isCSSWhitespace(t.peek(0)) {
					t.consume()
				}
				if t.peek(0) == ')' {
					t.consume()
					return Token{Type: TokenURL, Value: sb.String()}
				}
				if t.peek(0) == -1 {
					return Token{Type: TokenURL, Value: sb.String()}
				}
				t.consumeBadURLRemnants()
				return Token{Type: TokenBadURL}
			}
			if r == '"' || r == '\'' || r == '(' || isCSSNonPrintable(r) {
				t.consumeBadURLRemnants()
				return Token{Type: TokenBadURL}
			}
			if r == '\\' {
				if t.nextTwoCharsAreValidEscape() {
					sb.WriteRune(t.consumeEscape())
					continue
				}
				t.consumeBadURLRemnants()
				return Token{Type: TokenBadURL}
			}
			sb.WriteRune(r)
		}
	}
}

// isCSSNonPrintable reports whether r is a non-printable code point per the spec.
func isCSSNonPrintable(r rune) bool {
	if r >= 0 && r <= 8 {
		return true
	}
	if r == 0xB {
		return true
	}
	if r >= 0xE && r <= 0x1F {
		return true
	}
	if r >= 0x7F && r <= 0x9F {
		return true
	}
	return false
}

// consumeBadURLRemnants implements the "consume the remnants of a bad url" algorithm.
func (t *Tokenizer) consumeBadURLRemnants() {
	for {
		r := t.consume()
		if r == -1 || r == ')' {
			return
		}
		if r == '\\' && t.nextTwoCharsAreValidEscape() {
			_ = t.consumeEscape()
		}
	}
}

// consumeUnicodeRange implements the "consume a unicode-range token" algorithm. The
// leading 'U+' has already been seen.
func (t *Tokenizer) consumeUnicodeRange() Token {
	// Consume the 'U' and '+'.
	t.consume()
	t.consume()
	var hex string
	maxHex := 6
	for maxHex > 0 && isHexDigit(t.peek(0)) {
		hex += string(t.consume())
		maxHex--
	}
	hasQuestion := false
	for t.peek(0) == '?' && len(hex) < 6 {
		hex += "?"
		t.consume()
		hasQuestion = true
	}
	if hasQuestion {
		// Replace '?' with 0 for start, F for end.
		startStr := strings.ReplaceAll(hex, "?", "0")
		endStr := strings.ReplaceAll(hex, "?", "F")
		start, _ := strconv.ParseInt(startStr, 16, 32)
		end, _ := strconv.ParseInt(endStr, 16, 32)
		return Token{Type: TokenIdent, Value: "U+" + hex, Numeric: float64(start), Unit: formatHex(uint32(end))}
	}
	// Range with explicit end.
	if t.peek(0) == '-' && isHexDigit(t.peek(1)) {
		t.consume()
		var endHex string
		maxEnd := 6
		for maxEnd > 0 && isHexDigit(t.peek(0)) {
			endHex += string(t.consume())
			maxEnd--
		}
		start, _ := strconv.ParseInt(hex, 16, 32)
		end, _ := strconv.ParseInt(endHex, 16, 32)
		return Token{Type: TokenIdent, Value: "U+" + hex + "-" + endHex, Numeric: float64(start), Unit: formatHex(uint32(end))}
	}
	start, _ := strconv.ParseInt(hex, 16, 32)
	return Token{Type: TokenIdent, Value: "U+" + hex, Numeric: float64(start), Unit: formatHex(uint32(start))}
}

// formatHex returns the uppercase hex representation of v.
func formatHex(v uint32) string {
	return strings.ToUpper(strconv.FormatUint(uint64(v), 16))
}

// IsWhitespace reports whether a TokenType represents whitespace, mirroring
// CSSTokenizer::isWhitespace.
func IsWhitespace(t TokenType) bool {
	return t == TokenNonNewlineWhitespace || t == TokenNewline
}

// IsLetter reports whether r is an ASCII letter, mirroring
// CSSParserToken helpers.
func IsLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// IsDigit reports whether r is an ASCII digit.
func IsDigit(r rune) bool { return r >= '0' && r <= '9' }
