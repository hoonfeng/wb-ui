package jsc

import "testing"

// tokenNames is a helper that returns the lexeme string of each token in a source.
// It tracks the previous token kind so the lexer's regex/divide disambiguation
// matches real JavaScript (regex is suppressed after value-producing tokens).
func tokenNames(src string) []string {
	l := NewLexer(src)
	var out []string
	allowsRegex := true
	for {
		tok := l.Next(allowsRegex)
		if tok.Kind == TokenEOF {
			break
		}
		out = append(out, tok.Lexeme)
		if tok.Kind == TokenError {
			break
		}
		allowsRegex = regexAllowedAfterToken(tok.Kind)
	}
	return out
}

// regexAllowedAfterToken mirrors the parser's context rule: after value tokens
// (identifiers, literals, closing brackets) a '/' is division, not regex.
func regexAllowedAfterToken(k TokenKind) bool {
	switch k {
	case TokenIdentifier, TokenNumber, TokenString, TokenRegex, TokenTemplate,
		TokenCloseParen, TokenCloseBracket, TokenCloseBrace:
		return false
	}
	if k.IsKeyword() {
		switch k.KeywordOf() {
		case KeywordThis, KeywordTrue, KeywordFalse, KeywordNull, KeywordUndefined:
			return false
		}
	}
	return true
}

// tokenNamesNoRegex lexes a source treating every '/' as division, useful for testing
// punctuation without the regex ambiguity that arises between binary operators.
func tokenNamesNoRegex(src string) []string {
	l := NewLexer(src)
	var out []string
	for {
		tok := l.Next(false)
		if tok.Kind == TokenEOF {
			break
		}
		out = append(out, tok.Lexeme)
		if tok.Kind == TokenError {
			break
		}
	}
	return out
}

func TestLexerIdentifier(t *testing.T) {
	l := NewLexer("foo")
	tok := l.Next(true)
	if tok.Kind != TokenIdentifier || tok.Lexeme != "foo" {
		t.Fatalf("got %v %q, want identifier foo", tok.Kind, tok.Lexeme)
	}
	if tok.Line != 1 || tok.Col != 1 {
		t.Fatalf("position = %d:%d, want 1:1", tok.Line, tok.Col)
	}
}

func TestLexerKeywords(t *testing.T) {
	cases := map[string]KeywordKind{
		"let":     KeywordLet,
		"const":   KeywordConst,
		"var":     KeywordVar,
		"function": KeywordFunction,
		"return":  KeywordReturn,
		"if":      KeywordIf,
		"else":    KeywordElse,
		"for":     KeywordFor,
		"while":   KeywordWhile,
		"break":   KeywordBreak,
		"continue": KeywordContinue,
		"new":     KeywordNew,
		"this":    KeywordThis,
		"typeof":  KeywordTypeof,
		"await":   KeywordAwait,
		"yield":   KeywordYield,
		"try":     KeywordTry,
		"catch":   KeywordCatch,
		"finally": KeywordFinally,
		"throw":   KeywordThrow,
		"async":   KeywordAsync,
		"of":      KeywordOf,
	}
	for src, want := range cases {
		l := NewLexer(src)
		tok := l.Next(true)
		if tok.Kind != KeywordToken(want) {
			t.Fatalf("lex %q: got %v, want keyword %s", src, tok.Kind, keywordText[want])
		}
	}
}

func TestLexerLiteralsTrueFalseNull(t *testing.T) {
	src := "true false null undefined"
	l := NewLexer(src)
	l.Next(true) // true
	tok := l.Next(true)
	if tok.Kind != KeywordToken(KeywordFalse) {
		t.Fatalf("false: got %v", tok.Kind)
	}
	tok = l.Next(true)
	if tok.Kind != KeywordToken(KeywordNull) {
		t.Fatalf("null: got %v", tok.Kind)
	}
	tok = l.Next(true)
	if tok.Kind != KeywordToken(KeywordUndefined) {
		t.Fatalf("undefined: got %v", tok.Kind)
	}
}

func TestLexerDecimalNumber(t *testing.T) {
	l := NewLexer("42")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 42 {
		t.Fatalf("got %v %v, want 42", tok.Kind, tok.NumberValue)
	}
}

func TestLexerFloatNumber(t *testing.T) {
	l := NewLexer("3.14")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 3.14 {
		t.Fatalf("got %v %v, want 3.14", tok.Kind, tok.NumberValue)
	}
}

func TestLexerExponentNumber(t *testing.T) {
	l := NewLexer("1e3")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 1000 {
		t.Fatalf("got %v %v, want 1000", tok.Kind, tok.NumberValue)
	}
}

func TestLexerHexNumber(t *testing.T) {
	l := NewLexer("0xFF")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 255 {
		t.Fatalf("got %v %v, want 255", tok.Kind, tok.NumberValue)
	}
}

func TestLexerBinaryNumber(t *testing.T) {
	l := NewLexer("0b1010")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 10 {
		t.Fatalf("got %v %v, want 10", tok.Kind, tok.NumberValue)
	}
}

func TestLexerOctalNumber(t *testing.T) {
	l := NewLexer("0o17")
	tok := l.Next(true)
	if tok.Kind != TokenNumber || tok.NumberValue != 15 {
		t.Fatalf("got %v %v, want 15", tok.Kind, tok.NumberValue)
	}
}

func TestLexerStringDouble(t *testing.T) {
	l := NewLexer(`"hello"`)
	tok := l.Next(true)
	if tok.Kind != TokenString || tok.StringValue != "hello" {
		t.Fatalf("got %v %q, want hello", tok.Kind, tok.StringValue)
	}
}

func TestLexerStringEscapes(t *testing.T) {
	l := NewLexer(`"a\nb\tc"`)
	tok := l.Next(true)
	want := "a\nb\tc"
	if tok.StringValue != want {
		t.Fatalf("got %q, want %q", tok.StringValue, want)
	}
}

func TestLexerTemplateLiteral(t *testing.T) {
	l := NewLexer("`hello ${1+2} world`")
	tok := l.Next(true)
	if tok.Kind != TokenTemplate {
		t.Fatalf("got %v, want template", tok.Kind)
	}
	// The cooked string should contain the substitution placeholder.
	if tok.StringValue == "" {
		t.Fatalf("template cooked string should be non-empty")
	}
}

func TestLexerRegexLiteral(t *testing.T) {
	l := NewLexer(`/foo[a-z]+/gim`)
	tok := l.Next(true)
	if tok.Kind != TokenRegex {
		t.Fatalf("got %v, want regex", tok.Kind)
	}
	if tok.RegexPattern != "foo[a-z]+" {
		t.Fatalf("pattern = %q", tok.RegexPattern)
	}
	if tok.RegexFlags != "gim" {
		t.Fatalf("flags = %q", tok.RegexFlags)
	}
}

func TestLexerOperators(t *testing.T) {
	src := "+ - * / % ** = == === != !== < > <= >= && || ! & | ^ ~ << >> >>> ++ -- += -= *= /= ? : . ( ) [ ] { } ; ,"
	expected := []string{"+", "-", "*", "/", "%", "**", "=", "==", "===", "!=", "!==", "<", ">", "<=", ">=", "&&", "||", "!", "&", "|", "^", "~", "<<", ">>", ">>>", "++", "--", "+=", "-=", "*=", "/=", "?", ":", ".", "(", ")", "[", "]", "{", "}", ";", ","}
	// The source is a bare sequence of operators with no value tokens between them, so
	// a contextual lexer would treat '/' after '*' as a regex start. We force the
	// no-regex path here to verify each operator's lexeme spelling in isolation.
	got := tokenNamesNoRegex(src)
	if len(got) != len(expected) {
		t.Fatalf("token count = %d, want %d; got %v", len(got), len(expected), got)
	}
	for i, want := range expected {
		if got[i] != want {
			t.Fatalf("token %d = %q, want %q", i, got[i], want)
		}
	}
}

func TestLexerArrowAndSpread(t *testing.T) {
	src := "=> ..."
	got := tokenNames(src)
	if len(got) != 2 || got[0] != "=>" || got[1] != "..." {
		t.Fatalf("got %v, want [=> ...]", got)
	}
}

func TestLexerComments(t *testing.T) {
	src := "// line comment\n/* block */ x"
	got := tokenNames(src)
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got %v, want [x]", got)
	}
}

func TestLexerLineColTracking(t *testing.T) {
	src := "a\n  bc"
	l := NewLexer(src)
	tok := l.Next(true) // a at 1:1
	tok = l.Next(true)   // bc at 2:3
	if tok.Lexeme != "bc" {
		t.Fatalf("got %q, want bc", tok.Lexeme)
	}
	if tok.Line != 2 || tok.Col != 3 {
		t.Fatalf("position = %d:%d, want 2:3", tok.Line, tok.Col)
	}
}

func TestLexerPrecedingNewline(t *testing.T) {
	src := "a\nb"
	l := NewLexer(src)
	l.Next(true) // a
	tok := l.Next(true) // b
	if !tok.PrecedingNewline {
		t.Fatalf("b should have preceding newline")
	}
}

func TestLexerEOF(t *testing.T) {
	l := NewLexer("")
	tok := l.Next(true)
	if tok.Kind != TokenEOF {
		t.Fatalf("got %v, want EOF", tok.Kind)
	}
}

func TestLexerDivideVsRegex(t *testing.T) {
	// After an identifier, '/' is division.
	src := "a / b"
	got := tokenNames(src)
	if len(got) != 3 || got[1] != "/" {
		t.Fatalf("got %v, want [a / b]", got)
	}
	// At the start, '/' is regex.
	l := NewLexer("/re/")
	tok := l.Next(true)
	if tok.Kind != TokenRegex {
		t.Fatalf("got %v, want regex", tok.Kind)
	}
}

func TestLexerUnterminatedString(t *testing.T) {
	l := NewLexer(`"unterminated`)
	tok := l.Next(true)
	if tok.Kind != TokenError {
		t.Fatalf("got %v, want error for unterminated string", tok.Kind)
	}
}
