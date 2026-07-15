// Translation of: Source/WebCore/css/parser/CSSTokenizer.cpp (test portion)
// Tests for the CSS tokenizer covering the Level 3 token types.

package css

import (
	"testing"
)

func TestTokenizer_Ident(t *testing.T) {
	tok := tokenizeFirst(t, "abc")
	if tok.Type != TokenIdent || tok.Value != "abc" {
		t.Fatalf("got %+v, want ident=abc", tok)
	}
}

func TestTokenizer_HashID(t *testing.T) {
	tok := tokenizeFirst(t, "#main")
	if tok.Type != TokenHash || tok.Value != "main" || tok.HashType != HashID {
		t.Fatalf("got %+v, want hash id main", tok)
	}
}

func TestTokenizer_HashUnrestricted(t *testing.T) {
	tok := tokenizeFirst(t, "#123")
	if tok.Type != TokenHash || tok.HashType != HashUnrestricted {
		t.Fatalf("got %+v, want unrestricted hash", tok)
	}
	if tok.Value != "123" {
		t.Fatalf("value=%q want 123", tok.Value)
	}
}

func TestTokenizer_AtKeyword(t *testing.T) {
	tok := tokenizeFirst(t, "@media")
	if tok.Type != TokenAtKeyword || tok.Value != "media" {
		t.Fatalf("got %+v, want at-keyword media", tok)
	}
}

func TestTokenizer_String(t *testing.T) {
	tok := tokenizeFirst(t, `"hello"`)
	if tok.Type != TokenString || tok.Value != "hello" {
		t.Fatalf("got %+v, want string hello", tok)
	}
	tok = tokenizeFirst(t, `'a"b'`)
	if tok.Type != TokenString || tok.Value != `a"b` {
		t.Fatalf("got %+v, want string a\"b", tok)
	}
}

func TestTokenizer_BadString(t *testing.T) {
	// A literal newline inside a string makes it a bad string. We use a double-
	// quoted Go literal so the newline is real, and a multi-line raw string for
	// the second case.
	tok := tokenizeFirst(t, "\"hello\nworld\"")
	if tok.Type != TokenBadString {
		t.Fatalf("got %+v, want bad string", tok)
	}
	tok = tokenizeFirst(t, "\"hello\n")
	if tok.Type != TokenBadString {
		t.Fatalf("got %+v, want bad string (no close quote)", tok)
	}
}

func TestTokenizer_Escape(t *testing.T) {
	tok := tokenizeFirst(t, `hell\6F`)
	if tok.Type != TokenIdent || tok.Value != "hello" {
		t.Fatalf("got %+v, want ident hello", tok)
	}
}

func TestTokenizer_Number(t *testing.T) {
	tok := tokenizeFirst(t, "42")
	if tok.Type != TokenNumber || tok.Numeric != 42 {
		t.Fatalf("got %+v, want number 42", tok)
	}
	tok = tokenizeFirst(t, "-3.14")
	if tok.Type != TokenNumber || tok.Numeric != -3.14 {
		t.Fatalf("got %+v, want number -3.14", tok)
	}
}

func TestTokenizer_Percentage(t *testing.T) {
	tok := tokenizeFirst(t, "50%")
	if tok.Type != TokenPercentage || tok.Numeric != 50 {
		t.Fatalf("got %+v, want percentage 50", tok)
	}
}

func TestTokenizer_Dimension(t *testing.T) {
	tok := tokenizeFirst(t, "12px")
	if tok.Type != TokenDimension || tok.Numeric != 12 || tok.Unit != "px" {
		t.Fatalf("got %+v, want dimension 12px", tok)
	}
}

func TestTokenizer_Url(t *testing.T) {
	tok := tokenizeFirst(t, "url(http://example.com/x.png)")
	if tok.Type != TokenURL || tok.Value != "http://example.com/x.png" {
		t.Fatalf("got %+v, want url token", tok)
	}
}

func TestTokenizer_URLWithQuotes(t *testing.T) {
	tok := tokenizeFirst(t, `url("path with spaces.css")`)
	if tok.Type != TokenURL || tok.Value != "path with spaces.css" {
		t.Fatalf("got %+v, want url token with quoted value", tok)
	}
}

func TestTokenizer_Function(t *testing.T) {
	toks := NewTokenizer("calc(1 + 2)").Tokenize()
	if toks[0].Type != TokenFunction || toks[0].Value != "calc" {
		t.Fatalf("got %+v, want function calc", toks[0])
	}
}

func TestTokenizer_Combinators(t *testing.T) {
	// Test for the four combinator characters used in selectors.
	for _, c := range []struct {
		input string
		typ   TokenType
	}{
		{">", TokenDelimiter},
		{"+", TokenDelimiter},
		{"~", TokenDelimiter},
		{"*", TokenDelimiter},
	} {
		tok := tokenizeFirst(t, c.input)
		if tok.Type != c.typ {
			t.Fatalf("%q: got %v want %v", c.input, tok.Type, c.typ)
		}
		if tok.Type == TokenDelimiter && tok.Delimiter != []rune(c.input)[0] {
			t.Fatalf("delimiter=%c want %c", tok.Delimiter, []rune(c.input)[0])
		}
	}
}

func TestTokenizer_MatchOperators(t *testing.T) {
	cases := []struct {
		input string
		typ   TokenType
	}{
		{"~=", TokenIncludeMatch},
		{"|=", TokenDashMatch},
		{"^=", TokenPrefixMatch},
		{"$=", TokenSuffixMatch},
		{"*=", TokenSubstringMatch},
		{"||", TokenColumn},
	}
	for _, c := range cases {
		tok := tokenizeFirst(t, c.input)
		if tok.Type != c.typ {
			t.Fatalf("%q: got %v want %v", c.input, tok.Type, c.typ)
		}
	}
}

func TestTokenizer_Brackets(t *testing.T) {
	cases := []struct {
		input string
		typ   TokenType
	}{
		{"(", TokenLeftParenthesis},
		{")", TokenRightParenthesis},
		{"[", TokenLeftBracket},
		{"]", TokenRightBracket},
		{"{", TokenLeftBrace},
		{"}", TokenRightBrace},
	}
	for _, c := range cases {
		tok := tokenizeFirst(t, c.input)
		if tok.Type != c.typ {
			t.Fatalf("%q: got %v want %v", c.input, tok.Type, c.typ)
		}
	}
}

func TestTokenizer_CDO_CDC(t *testing.T) {
	tok := tokenizeFirst(t, "<!--")
	if tok.Type != TokenCDO {
		t.Fatalf("got %v, want CDO", tok.Type)
	}
	tok = tokenizeFirst(t, "-->")
	if tok.Type != TokenCDC {
		t.Fatalf("got %v, want CDC", tok.Type)
	}
}

func TestTokenizer_Comment(t *testing.T) {
	toks := NewTokenizer("a /* comment */ b").Tokenize()
	if len(toks) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(toks))
	}
	// Expect: ident(a), comment, whitespace, ident(b), EOF.
	if toks[0].Type != TokenIdent || toks[0].Value != "a" {
		t.Fatalf("first token=%+v, want ident a", toks[0])
	}
	foundComment := false
	for _, tok := range toks {
		if tok.Type == TokenComment {
			foundComment = true
		}
	}
	if !foundComment {
		t.Fatalf("expected a comment token in stream")
	}
}

func TestTokenizer_EOF(t *testing.T) {
	toks := NewTokenizer("a").Tokenize()
	last := toks[len(toks)-1]
	if last.Type != TokenEOF {
		t.Fatalf("last token=%v, want EOF", last.Type)
	}
}

func TestTokenizer_Whitespace(t *testing.T) {
	toks := NewTokenizer(" ").Tokenize()
	if toks[0].Type != TokenNonNewlineWhitespace {
		t.Fatalf("got %v, want whitespace", toks[0].Type)
	}
	toks = NewTokenizer("\n").Tokenize()
	if toks[0].Type != TokenNewline {
		t.Fatalf("got %v, want newline", toks[0].Type)
	}
	// A run of mixed whitespace collapses to a single whitespace token.
	toks = NewTokenizer("  \t  \n").Tokenize()
	if toks[0].Type != TokenNewline {
		t.Fatalf("got %v, want newline (run started with whitespace then newline)", toks[0].Type)
	}
}

func TestTokenizer_Preprocess(t *testing.T) {
	// CRLF should be normalized to LF.
	out := Preprocess("a\r\nb")
	if out != "a\nb" {
		t.Fatalf("got %q want a\nb", out)
	}
	// CR alone should be normalized to LF.
	out = Preprocess("a\rb")
	if out != "a\nb" {
		t.Fatalf("got %q want a\nb", out)
	}
	// FF should be normalized to LF.
	out = Preprocess("a\fb")
	if out != "a\nb" {
		t.Fatalf("got %q want a\nb", out)
	}
}

// tokenizeFirst runs the tokenizer over input and returns the first non-whitespace,
// non-comment token.
func tokenizeFirst(t *testing.T, input string) Token {
	t.Helper()
	toks := NewTokenizer(input).Tokenize()
	for _, tok := range toks {
		if tok.Type == TokenEOF || tok.Type == TokenNonNewlineWhitespace ||
			tok.Type == TokenNewline || tok.Type == TokenComment {
			continue
		}
		return tok
	}
	t.Fatalf("no non-whitespace token in %q: %v", input, toks)
	return Token{}
}

func TestTokenizer_NumericSign(t *testing.T) {
	tok := tokenizeFirst(t, "+12")
	if tok.Type != TokenNumber || tok.Numeric != 12 || tok.NumericSign != SignPlus {
		t.Fatalf("got %+v, want +12 sign plus", tok)
	}
	tok = tokenizeFirst(t, "-12")
	if tok.Type != TokenNumber || tok.Numeric != -12 || tok.NumericSign != SignMinus {
		t.Fatalf("got %+v, want -12 sign minus", tok)
	}
}

func TestTokenizer_SemicolonColonComma(t *testing.T) {
	for _, c := range []struct {
		input string
		typ   TokenType
	}{
		{":", TokenColon},
		{";", TokenSemicolon},
		{",", TokenComma},
	} {
		tok := tokenizeFirst(t, c.input)
		if tok.Type != c.typ {
			t.Fatalf("%q: got %v want %v", c.input, tok.Type, c.typ)
		}
	}
}

// ensure reflect is exercised.
