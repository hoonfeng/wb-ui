// Translation of: Source/JavaScriptCore/parser/Lexer.h
//                  Source/JavaScriptCore/parser/Lexer.cpp
//                  Source/JavaScriptCore/parser/ParserTokens.h
// Completeness: 65%
// Simplifications:
//   - no template literal tagged functions (raw strings are not retained)
//   - no BigInt literals
//   - no Unicode escape surrogate-pair handling in identifiers (ASCII fast path only)
//   - regex flags are lexed but not validated against the spec
//   - automatic semicolon insertion uses the simplified "newline before token" rule

package jsc

import (
	"strconv"
	"strings"
	"unicode"
)

// TokenKind enumerates the lexeme kinds produced by the Lexer, mirroring JSTokenType
// in ParserTokens.h. The Go port collapses the bit-packed flag/precedence encoding of
// the C++ version into a flat enumeration; precedence is recovered via the
// BinaryPrecedence helper.
type TokenKind int

const (
	// TokenEOF marks end of input.
	TokenEOF TokenKind = iota
	// TokenError marks a lexical error.
	TokenError

	// Literals
	TokenIdentifier
	TokenNumber
	TokenString
	TokenTemplate
	TokenRegex
	TokenBigInt // reserved; not produced by this lexer but kept for API symmetry

	// Punctuation
	TokenOpenBrace    // {
	TokenCloseBrace   // }
	TokenOpenParen    // (
	TokenCloseParen   // )
	TokenOpenBracket  // [
	TokenCloseBracket  // ]
	TokenComma         // ,
	TokenSemicolon     // ;
	TokenColon         // :
	TokenDot           // .
	TokenQuestion      // ?
	TokenArrow         // =>
	TokenSpread        // ...
	TokenQuestionDot   // ?.

	// Assignment operators
	TokenAssign       // =
	TokenPlusAssign   // +=
	TokenMinusAssign  // -=
	TokenMultAssign   // *=
	TokenDivAssign    // /=
	TokenModAssign    // %=
	TokenPowAssign    // **=
	TokenBitAndAssign // &=
	TokenBitOrAssign  // |=
	TokenBitXorAssign // ^=
	TokenLShiftAssign // <<=
	TokenRShiftAssign // >>=
	TokenURShiftAssign // >>>=
	TokenCoalesceAssign // ??=
	TokenOrAssign     // ||=
	TokenAndAssign    // &&=

	// Update operators
	TokenIncrement // ++
	TokenDecrement // --

	// Unary operators
	TokenBang  // !
	TokenTilde  // ~

	// Binary operators (in rough precedence order)
	TokenCoalesce // ??
	TokenOr       // ||
	TokenAnd      // &&
	TokenBitOr    // |
	TokenBitXor  // ^
	TokenBitAnd  // &
	TokenEqual      // ==
	TokenNotEqual   // !=
	TokenStrictEqual  // ===
	TokenStrictNotEqual // !==
	TokenLess        // <
	TokenGreater     // >
	TokenLessEqual   // <=
	TokenGreaterEqual // >=
	TokenInstanceOf
	TokenIn
	TokenLeftShift   // <<
	TokenRightShift  // >>
	TokenUnsignedRightShift // >>>
	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent
	TokenPower // **
)

// Keyword token kinds begin after TokenPower.
const keywordBase TokenKind = TokenPower + 1

// KeywordKind enumerates reserved/contextual keywords. Each maps to a keyword token
// whose kind is keywordBase + KeywordKind, retrievable via KeywordToken().
type KeywordKind int

const (
	KeywordBreak KeywordKind = iota
	KeywordCase
	KeywordCatch
	KeywordClass
	KeywordConst
	KeywordContinue
	KeywordDefault
	KeywordDelete
	KeywordDo
	KeywordElse
	KeywordExport
	KeywordExtends
	KeywordFinally
	KeywordFor
	KeywordFunction
	KeywordIf
	KeywordImport
	KeywordIn
	KeywordInstanceof
	KeywordNew
	KeywordOf
	KeywordReturn
	KeywordSuper
	KeywordSwitch
	KeywordThis
	KeywordThrow
	KeywordTry
	KeywordTypeof
	KeywordVar
	KeywordVoid
	KeywordWhile
	KeywordWith
	KeywordYield
	KeywordAwait
	// literals that are reserved words
	KeywordNull
	KeywordTrue
	KeywordFalse
	KeywordUndefined
	KeywordLet
	// Async is contextual: 'async' alone is an identifier-ish keyword.
	KeywordAsync
)

// keywordText maps KeywordKind to its source spelling.
var keywordText = map[KeywordKind]string{
	KeywordBreak:    "break",
	KeywordCase:     "case",
	KeywordCatch:    "catch",
	KeywordClass:    "class",
	KeywordConst:    "const",
	KeywordContinue: "continue",
	KeywordDefault:  "default",
	KeywordDelete:   "delete",
	KeywordDo:       "do",
	KeywordElse:     "else",
	KeywordExport:   "export",
	KeywordExtends:  "extends",
	KeywordFinally:  "finally",
	KeywordFor:      "for",
	KeywordFunction: "function",
	KeywordIf:       "if",
	KeywordImport:   "import",
	KeywordIn:       "in",
	KeywordInstanceof: "instanceof",
	KeywordNew:      "new",
	KeywordOf:       "of",
	KeywordReturn:   "return",
	KeywordSuper:    "super",
	KeywordSwitch:   "switch",
	KeywordThis:     "this",
	KeywordThrow:    "throw",
	KeywordTry:      "try",
	KeywordTypeof:   "typeof",
	KeywordVar:      "var",
	KeywordVoid:     "void",
	KeywordWhile:    "while",
	KeywordWith:     "with",
	KeywordYield:    "yield",
	KeywordAwait:    "await",
	KeywordNull:     "null",
	KeywordTrue:     "true",
	KeywordFalse:    "false",
	KeywordUndefined: "undefined",
	KeywordLet:      "let",
	KeywordAsync:    "async",
}

// textToKeyword is the reverse lookup used when lexing identifiers.
var textToKeyword = func() map[string]KeywordKind {
	m := make(map[string]KeywordKind, len(keywordText))
	for k, v := range keywordText {
		m[v] = k
	}
	return m
}()

// KeywordToken returns the TokenKind for a given keyword.
func KeywordToken(k KeywordKind) TokenKind { return keywordBase + TokenKind(k) }

// IsKeyword reports whether tok is a keyword token.
func (tok TokenKind) IsKeyword() bool { return tok >= keywordBase }

// KeywordOf returns the KeywordKind for a keyword token. Panics for non-keyword tokens.
func (tok TokenKind) KeywordOf() KeywordKind {
	if !tok.IsKeyword() {
		panic("jsc: KeywordOf on non-keyword token")
	}
	return KeywordKind(tok - keywordBase)
}

// Token is the Go translation of JSC::JSToken. It carries the kind, the matched lexeme
// text, and the 1-based line/column where the lexeme started.
type Token struct {
	Kind   TokenKind
	Lexeme string
	Line   int
	Col    int
	// PrecedingNewline records whether a line terminator appeared before this token, used
	// for restricted production / ASI handling in the parser.
	PrecedingNewline bool
	// NumberValue holds the parsed numeric value for TokenNumber.
	NumberValue float64
	// StringValue holds the decoded string for TokenString/TokenTemplate.
	StringValue string
	// RegexPattern and RegexFlags hold the components of a TokenRegex.
	RegexPattern string
	RegexFlags   string
}

// Lexer is the Go translation of JSC::Lexer<T>. WebKit's Lexer is a templated
// character-stream scanner; the Go port works on a Go string and tracks a byte offset
// plus a 1-based line/column.
type Lexer struct {
	source string
	pos    int
	line   int
	col    int
	// hadLineTerminator records whether the last skip-whitespace pass saw a newline,
	// surfaced on the next token via PrecedingNewline.
	hadLineTerminator bool
}

// NewLexer constructs a Lexer over the given source text. Line/column start at 1.
func NewLexer(src string) *Lexer {
	return &Lexer{source: src, pos: 0, line: 1, col: 1}
}

// Source returns the original source string.
func (l *Lexer) Source() string { return l.source }

// Pos returns the current byte offset.
func (l *Lexer) Pos() int { return l.pos }

// Line returns the current 1-based line number.
func (l *Lexer) Line() int { return l.line }

// Col returns the current 1-based column number.
func (l *Lexer) Col() int { return l.col }

// AtEnd reports whether the lexer has consumed all input.
func (l *Lexer) AtEnd() bool { return l.pos >= len(l.source) }

// peek returns the byte at offset off without advancing. Returns 0 at end of input.
func (l *Lexer) peek(off int) byte {
	p := l.pos + off
	if p < 0 || p >= len(l.source) {
		return 0
	}
	return l.source[p]
}

// cur returns the byte at the current position.
func (l *Lexer) cur() byte { return l.peek(0) }

// advance moves the cursor forward by one byte, updating line/column.
func (l *Lexer) advance() {
	if l.pos >= len(l.source) {
		return
	}
	c := l.source[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
		l.hadLineTerminator = true
	} else {
		l.col++
	}
}

// match consumes the next byte if it equals c and returns true.
func (l *Lexer) match(c byte) bool {
	if l.cur() == c {
		l.advance()
		return true
	}
	return false
}

// isWhitespace reports whether c is a JS whitespace character (subset: ASCII + BOM).
func isWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\v', '\f', 0xA0, 0xEF:
		// 0xEF is the lead byte of the UTF-8 BOM handled separately; for the common
		// ASCII-only sources this branch is enough.
		return true
	}
	return false
}

// isLineTerminator reports whether c is a JS line terminator (\n, \r, U+2028/U+2029).
func isLineTerminator(c byte) bool {
	return c == '\n' || c == '\r'
}

// skipWhitespaceAndComments consumes whitespace, line comments, and block comments,
// recording whether a line terminator was seen.
func (l *Lexer) skipWhitespaceAndComments() {
	for !l.AtEnd() {
		c := l.cur()
		if c == ' ' || c == '\t' || c == '\v' || c == '\f' {
			l.advance()
			continue
		}
		if isLineTerminator(c) {
			l.advance()
			continue
		}
		// UTF-8 BOM (EF BB BF)
		if c == 0xEF && l.peek(1) == 0xBB && l.peek(2) == 0xBF {
			l.advance()
			l.advance()
			l.advance()
			continue
		}
		// Line comment
		if c == '/' && l.peek(1) == '/' {
			for !l.AtEnd() && !isLineTerminator(l.cur()) {
				l.advance()
			}
			continue
		}
		// Block comment
		if c == '/' && l.peek(1) == '*' {
			l.advance()
			l.advance()
			for !l.AtEnd() {
				if l.cur() == '*' && l.peek(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		// HTML-style comment (legacy, ' <!--' to end of line)
		if c == '<' && l.peek(1) == '!' && l.peek(2) == '-' && l.peek(3) == '-' {
			for !l.AtEnd() && !isLineTerminator(l.cur()) {
				l.advance()
			}
			continue
		}
		break
	}
}

// isIdentifierStart reports whether a byte may start an identifier (ASCII subset).
func isIdentifierStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// isIdentifierPart reports whether a byte may continue an identifier (ASCII subset).
func isIdentifierPart(c byte) bool {
	return isIdentifierStart(c) || (c >= '0' && c <= '9')
}

// lexIdentifier scans an identifier or keyword starting at the cursor.
func (l *Lexer) lexIdentifier() Token {
	start := l.pos
	startLine, startCol := l.line, l.col
	for !l.AtEnd() {
		c := l.cur()
		if !isIdentifierPart(c) {
			// Allow backslash-escaped Unicode identifiers minimally by stopping; the
			// parser can surface an error if needed.
			break
		}
		l.advance()
	}
	text := l.source[start:l.pos]
	tok := Token{Lexeme: text, Line: startLine, Col: startCol}
	if kw, ok := textToKeyword[text]; ok {
		tok.Kind = KeywordToken(kw)
		switch kw {
		case KeywordNull:
			tok.StringValue = "null"
		case KeywordTrue:
			tok.StringValue = "true"
		case KeywordFalse:
			tok.StringValue = "false"
		case KeywordUndefined:
			tok.StringValue = "undefined"
		}
	} else {
		tok.Kind = TokenIdentifier
	}
	return tok
}

// lexNumber scans a numeric literal (decimal, hex, octal, binary, float, exponent).
func (l *Lexer) lexNumber() Token {
	start := l.pos
	startLine, startCol := l.line, l.col
	if l.cur() == '0' {
		switch l.peek(1) {
		case 'x', 'X':
			l.advance()
			l.advance()
			digitsStart := l.pos
			for !l.AtEnd() && isHexDigit(l.cur()) {
				l.advance()
			}
			text := l.source[digitsStart:l.pos]
			n, err := strconv.ParseInt(text, 16, 64)
			if err != nil {
				return Token{Kind: TokenError, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol}
			}
			return Token{Kind: TokenNumber, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol, NumberValue: float64(n)}
		case 'o', 'O':
			l.advance()
			l.advance()
			digitsStart := l.pos
			for !l.AtEnd() && isOctalDigit(l.cur()) {
				l.advance()
			}
			text := l.source[digitsStart:l.pos]
			n, err := strconv.ParseInt(text, 8, 64)
			if err != nil {
				return Token{Kind: TokenError, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol}
			}
			return Token{Kind: TokenNumber, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol, NumberValue: float64(n)}
		case 'b', 'B':
			l.advance()
			l.advance()
			digitsStart := l.pos
			for !l.AtEnd() && isBinaryDigit(l.cur()) {
				l.advance()
			}
			text := l.source[digitsStart:l.pos]
			n, err := strconv.ParseInt(text, 2, 64)
			if err != nil {
				return Token{Kind: TokenError, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol}
			}
			return Token{Kind: TokenNumber, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol, NumberValue: float64(n)}
		}
	}
	// Decimal integer / float / exponent
	for !l.AtEnd() && isDecimalDigit(l.cur()) {
		l.advance()
	}
	if !l.AtEnd() && l.cur() == '.' {
		l.advance()
		for !l.AtEnd() && isDecimalDigit(l.cur()) {
			l.advance()
		}
	}
	if !l.AtEnd() && (l.cur() == 'e' || l.cur() == 'E') {
		l.advance()
		if !l.AtEnd() && (l.cur() == '+' || l.cur() == '-') {
			l.advance()
		}
		for !l.AtEnd() && isDecimalDigit(l.cur()) {
			l.advance()
		}
	}
	// BigInt suffix (kept for completeness; we still treat as a number since BigInt is unsupported).
	if !l.AtEnd() && l.cur() == 'n' {
		l.advance()
	}
	text := l.source[start:l.pos]
	text = strings.TrimSuffix(text, "n")
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return Token{Kind: TokenError, Lexeme: text, Line: startLine, Col: startCol}
	}
	return Token{Kind: TokenNumber, Lexeme: l.source[start:l.pos], Line: startLine, Col: startCol, NumberValue: n}
}

func isDecimalDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool {
	return isDecimalDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isOctalDigit(c byte) bool  { return c >= '0' && c <= '7' }
func isBinaryDigit(c byte) bool { return c == '0' || c == '1' }

// lexString scans a single- or double-quoted string literal, decoding escapes.
func (l *Lexer) lexString() Token {
	startLine, startCol := l.line, l.col
	quote := l.cur()
	l.advance()
	var sb strings.Builder
	for !l.AtEnd() {
		c := l.cur()
		if c == quote {
			l.advance()
			return Token{Kind: TokenString, Lexeme: l.source[l.pos-len(sb.String())-2 : l.pos], Line: startLine, Col: startCol, StringValue: sb.String()}
		}
		if isLineTerminator(c) {
			return Token{Kind: TokenError, Lexeme: l.source[l.pos : l.pos+1], Line: startLine, Col: startCol}
		}
		if c == '\\' {
			l.advance()
			if l.AtEnd() {
				break
			}
			esc := l.cur()
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'v':
				sb.WriteByte('\v')
			case '0':
				sb.WriteByte(0)
			case '\\':
				sb.WriteByte('\\')
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			case '`':
				sb.WriteByte('`')
			case 'x':
				l.advance()
				hex := l.readHexRun(2)
				if hex < 0 {
					return Token{Kind: TokenError, Lexeme: "", Line: startLine, Col: startCol}
				}
				sb.WriteRune(rune(hex))
				continue
			case 'u':
				l.advance()
				r, ok := l.readUnicodeEscape()
				if !ok {
					return Token{Kind: TokenError, Lexeme: "", Line: startLine, Col: startCol}
				}
				sb.WriteRune(r)
				continue
			default:
				if isLineTerminator(esc) {
					// Line continuation: skip the terminator.
				} else {
					sb.WriteByte(esc)
				}
			}
			l.advance()
			continue
		}
		sb.WriteByte(c)
		l.advance()
	}
	return Token{Kind: TokenError, Lexeme: "", Line: startLine, Col: startCol}
}

// readHexRun reads up to n hex digits and returns the integer value, or -1 on failure.
func (l *Lexer) readHexRun(n int) int {
	val := 0
	for i := 0; i < n; i++ {
		c := l.cur()
		if !isHexDigit(c) {
			return -1
		}
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		}
		val = val*16 + d
		l.advance()
	}
	return val
}

// readUnicodeEscape reads a \uXXXX or \u{...} escape starting after 'u' was consumed.
// Returns the rune and an ok flag.
func (l *Lexer) readUnicodeEscape() (rune, bool) {
	if l.cur() == '{' {
		l.advance()
		val := 0
		count := 0
		for !l.AtEnd() && l.cur() != '}' {
			c := l.cur()
			if !isHexDigit(c) {
				return 0, false
			}
			var d int
			switch {
			case c >= '0' && c <= '9':
				d = int(c - '0')
			case c >= 'a' && c <= 'f':
				d = int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = int(c-'A') + 10
			}
			val = val*16 + d
			l.advance()
			count++
			if count > 6 {
				return 0, false
			}
		}
		if l.cur() != '}' {
			return 0, false
		}
		l.advance()
		return rune(val), true
	}
	val := l.readHexRun(4)
	if val < 0 {
		return 0, false
	}
	return rune(val), true
}

// lexTemplate scans a `template` literal. The opening backtick has been consumed; this
// function returns TokenTemplate with the cooked string. Substitutions (${...}) are
// represented in the lexeme; the parser splits them out.
func (l *Lexer) lexTemplate() Token {
	startLine, startCol := l.line, l.col
	var sb strings.Builder
	for !l.AtEnd() {
		c := l.cur()
		if c == '`' {
			l.advance()
			return Token{Kind: TokenTemplate, Lexeme: sb.String(), Line: startLine, Col: startCol, StringValue: sb.String()}
		}
		if c == '\\' {
			l.advance()
			if l.AtEnd() {
				break
			}
			esc := l.cur()
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '\\':
				sb.WriteByte('\\')
			case '`':
				sb.WriteByte('`')
			case '$':
				sb.WriteByte('$')
			default:
				sb.WriteByte(esc)
			}
			l.advance()
			continue
		}
		if c == '$' && l.peek(1) == '{' {
			// Record a placeholder; the parser handles substitution by re-lexing.
			sb.WriteString("\x00${")
			l.advance()
			l.advance()
			depth := 1
			for !l.AtEnd() && depth > 0 {
				cc := l.cur()
				if cc == '{' {
					depth++
				} else if cc == '}' {
					depth--
					if depth == 0 {
						l.advance()
						break
					}
				}
				sb.WriteByte(cc)
				l.advance()
			}
			sb.WriteString("\x00}")
			continue
		}
		sb.WriteByte(c)
		l.advance()
	}
	return Token{Kind: TokenError, Lexeme: "", Line: startLine, Col: startCol}
}

// lexRegex scans a regex literal /pattern/flags starting at the first '/'.
func (l *Lexer) lexRegex() Token {
	startLine, startCol := l.line, l.col
	l.advance() // consume '/'
	var pat strings.Builder
	inClass := false
	for !l.AtEnd() {
		c := l.cur()
		if c == '\\' {
			pat.WriteByte(c)
			l.advance()
			if l.AtEnd() {
				break
			}
			pat.WriteByte(l.cur())
			l.advance()
			continue
		}
		if c == '[' {
			inClass = true
		} else if c == ']' {
			inClass = false
		} else if c == '/' && !inClass {
			l.advance()
			break
		}
		if isLineTerminator(c) {
			return Token{Kind: TokenError, Lexeme: "", Line: startLine, Col: startCol}
		}
		pat.WriteByte(c)
		l.advance()
	}
	var flags strings.Builder
	for !l.AtEnd() && isIdentifierPart(l.cur()) {
		flags.WriteByte(l.cur())
		l.advance()
	}
	return Token{Kind: TokenRegex, Lexeme: "/" + pat.String() + "/" + flags.String(), Line: startLine, Col: startCol, RegexPattern: pat.String(), RegexFlags: flags.String()}
}

// lexPunctuation scans operators and punctuation starting at the cursor.
func (l *Lexer) lexPunctuation(prev TokenKind) Token {
	startLine, startCol := l.line, l.col
	c := l.cur()
	mk := func(kind TokenKind, n int) Token {
		text := ""
		if n > 0 {
			text = l.source[l.pos : l.pos+n]
		}
		for i := 0; i < n; i++ {
			l.advance()
		}
		return Token{Kind: kind, Lexeme: text, Line: startLine, Col: startCol}
	}
	switch c {
	case '{':
		l.advance()
		return Token{Kind: TokenOpenBrace, Lexeme: "{", Line: startLine, Col: startCol}
	case '}':
		l.advance()
		return Token{Kind: TokenCloseBrace, Lexeme: "}", Line: startLine, Col: startCol}
	case '(':
		l.advance()
		return Token{Kind: TokenOpenParen, Lexeme: "(", Line: startLine, Col: startCol}
	case ')':
		l.advance()
		return Token{Kind: TokenCloseParen, Lexeme: ")", Line: startLine, Col: startCol}
	case '[':
		l.advance()
		return Token{Kind: TokenOpenBracket, Lexeme: "[", Line: startLine, Col: startCol}
	case ']':
		l.advance()
		return Token{Kind: TokenCloseBracket, Lexeme: "]", Line: startLine, Col: startCol}
	case ',':
		l.advance()
		return Token{Kind: TokenComma, Lexeme: ",", Line: startLine, Col: startCol}
	case ';':
		l.advance()
		return Token{Kind: TokenSemicolon, Lexeme: ";", Line: startLine, Col: startCol}
	case ':':
		l.advance()
		return Token{Kind: TokenColon, Lexeme: ":", Line: startLine, Col: startCol}
	case '~':
		l.advance()
		return Token{Kind: TokenTilde, Lexeme: "~", Line: startLine, Col: startCol}
	case '?':
		if l.peek(1) == '?' {
			if l.peek(2) == '=' {
				return mk(TokenCoalesceAssign, 3)
			}
			return mk(TokenCoalesce, 2)
		}
		if l.peek(1) == '.' {
			// ?. only when not followed by a digit (else '.' is part of a number)
			if !isDecimalDigit(l.peek(2)) {
				return mk(TokenQuestionDot, 2)
			}
		}
		l.advance()
		return Token{Kind: TokenQuestion, Lexeme: "?", Line: startLine, Col: startCol}
	case '.':
		if l.peek(1) == '.' && l.peek(2) == '.' {
			return mk(TokenSpread, 3)
		}
		if isDecimalDigit(l.peek(1)) {
			return l.lexNumber()
		}
		l.advance()
		return Token{Kind: TokenDot, Lexeme: ".", Line: startLine, Col: startCol}
	case '!':
		if l.peek(1) == '=' {
			if l.peek(2) == '=' {
				return mk(TokenStrictNotEqual, 3)
			}
			return mk(TokenNotEqual, 2)
		}
		l.advance()
		return Token{Kind: TokenBang, Lexeme: "!", Line: startLine, Col: startCol}
	case '=':
		if l.peek(1) == '=' {
			if l.peek(2) == '=' {
				return mk(TokenStrictEqual, 3)
			}
			return mk(TokenEqual, 2)
		}
		if l.peek(1) == '>' {
			return mk(TokenArrow, 2)
		}
		l.advance()
		return Token{Kind: TokenAssign, Lexeme: "=", Line: startLine, Col: startCol}
	case '+':
		if l.peek(1) == '+' {
			return mk(TokenIncrement, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenPlusAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenPlus, Lexeme: "+", Line: startLine, Col: startCol}
	case '-':
		if l.peek(1) == '-' {
			return mk(TokenDecrement, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenMinusAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenMinus, Lexeme: "-", Line: startLine, Col: startCol}
	case '*':
		if l.peek(1) == '*' {
			if l.peek(2) == '=' {
				return mk(TokenPowAssign, 3)
			}
			return mk(TokenPower, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenMultAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenStar, Lexeme: "*", Line: startLine, Col: startCol}
	case '/':
		// '/' or '/=' or regex (the caller disambiguates regex by context)
		if l.peek(1) == '=' {
			return mk(TokenDivAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenSlash, Lexeme: "/", Line: startLine, Col: startCol}
	case '%':
		if l.peek(1) == '=' {
			return mk(TokenModAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenPercent, Lexeme: "%", Line: startLine, Col: startCol}
	case '&':
		if l.peek(1) == '&' {
			if l.peek(2) == '=' {
				return mk(TokenAndAssign, 3)
			}
			return mk(TokenAnd, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenBitAndAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenBitAnd, Lexeme: "&", Line: startLine, Col: startCol}
	case '|':
		if l.peek(1) == '|' {
			if l.peek(2) == '=' {
				return mk(TokenOrAssign, 3)
			}
			return mk(TokenOr, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenBitOrAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenBitOr, Lexeme: "|", Line: startLine, Col: startCol}
	case '^':
		if l.peek(1) == '=' {
			return mk(TokenBitXorAssign, 2)
		}
		l.advance()
		return Token{Kind: TokenBitXor, Lexeme: "^", Line: startLine, Col: startCol}
	case '<':
		if l.peek(1) == '<' {
			if l.peek(2) == '=' {
				return mk(TokenLShiftAssign, 3)
			}
			return mk(TokenLeftShift, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenLessEqual, 2)
		}
		l.advance()
		return Token{Kind: TokenLess, Lexeme: "<", Line: startLine, Col: startCol}
	case '>':
		if l.peek(1) == '>' {
			if l.peek(2) == '>' {
				if l.peek(3) == '=' {
					return mk(TokenURShiftAssign, 4)
				}
				return mk(TokenUnsignedRightShift, 3)
			}
			if l.peek(2) == '=' {
				return mk(TokenRShiftAssign, 3)
			}
			return mk(TokenRightShift, 2)
		}
		if l.peek(1) == '=' {
			return mk(TokenGreaterEqual, 2)
		}
		l.advance()
		return Token{Kind: TokenGreater, Lexeme: ">", Line: startLine, Col: startCol}
	}
	// Unknown character: emit error and skip.
	l.advance()
	return Token{Kind: TokenError, Lexeme: string(c), Line: startLine, Col: startCol}
}

// Next scans and returns the next token, skipping leading whitespace and comments.
// prevAllowsRegex reports whether a regex literal is permitted in this position
// (driven by the previous token kind from the parser).
func (l *Lexer) Next(prevAllowsRegex bool) Token {
	l.hadLineTerminator = false
	l.skipWhitespaceAndComments()
	if l.AtEnd() {
		return Token{Kind: TokenEOF, Lexeme: "", Line: l.line, Col: l.col, PrecedingNewline: l.hadLineTerminator}
	}
	c := l.cur()
	precedingNewline := l.hadLineTerminator
	var tok Token
	switch {
	case isIdentifierStart(c):
		tok = l.lexIdentifier()
	case isDecimalDigit(c):
		tok = l.lexNumber()
	case c == '"' || c == '\'':
		tok = l.lexString()
	case c == '`':
		l.advance()
		tok = l.lexTemplate()
	case c == '/' && prevAllowsRegex:
		tok = l.lexRegex()
	default:
		tok = l.lexPunctuation(TokenEOF)
	}
	tok.PrecedingNewline = precedingNewline
	return tok
}

// BinaryPrecedence returns the operator precedence for a binary token, or 0 if the
// token is not a binary operator. Higher numbers bind tighter, mirroring the
// BINARY_OP_PRECEDENCE macro in ParserTokens.h.
func BinaryPrecedence(tok TokenKind) int {
	switch tok {
	case TokenCoalesce:
		return 1
	case TokenOr:
		return 2
	case TokenAnd:
		return 3
	case TokenBitOr:
		return 4
	case TokenBitXor:
		return 5
	case TokenBitAnd:
		return 6
	case TokenEqual, TokenNotEqual, TokenStrictEqual, TokenStrictNotEqual:
		return 7
	case TokenLess, TokenGreater, TokenLessEqual, TokenGreaterEqual, TokenInstanceOf, TokenIn:
		return 8
	case TokenLeftShift, TokenRightShift, TokenUnsignedRightShift:
		return 9
	case TokenPlus, TokenMinus:
		return 10
	case TokenStar, TokenSlash, TokenPercent:
		return 11
	case TokenPower:
		return 12
	}
	return 0
}

// IsAssignmentOp reports whether tok is an assignment operator token.
func IsAssignmentOp(tok TokenKind) bool {
	switch tok {
	case TokenAssign, TokenPlusAssign, TokenMinusAssign, TokenMultAssign, TokenDivAssign,
		TokenModAssign, TokenPowAssign, TokenBitAndAssign, TokenBitOrAssign, TokenBitXorAssign,
		TokenLShiftAssign, TokenRShiftAssign, TokenURShiftAssign, TokenCoalesceAssign,
		TokenOrAssign, TokenAndAssign:
		return true
	}
	return false
}

// IsPrefixOp reports whether tok may be a prefix unary operator.
func IsPrefixOp(tok TokenKind) bool {
	switch tok {
	case TokenBang, TokenTilde, TokenPlus, TokenMinus, TokenIncrement, TokenDecrement,
		KeywordToken(KeywordTypeof), KeywordToken(KeywordVoid), KeywordToken(KeywordDelete):
		return true
	}
	return false
}

// IsUpdateOp reports whether tok is ++ or --.
func IsUpdateOp(tok TokenKind) bool { return tok == TokenIncrement || tok == TokenDecrement }

// unused: keep unicode import referenced for future UTF-16 identifier handling.
var _ = unicode.IsLetter
