package editor

import "testing"

func TestTokenize_GoKeywords(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "func main()")

	// "func" should be a keyword
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}

	// Find the "func" token
	funcToken := findTokenByText(tokens, "func", "func main()")
	if funcToken == nil {
		t.Fatalf("func keyword not found in tokens: %+v", tokens)
	}
	if !funcToken.HasScope("keyword") {
		t.Errorf("func token scopes = %v, want keyword", funcToken.Scopes)
	}
}

func TestTokenize_GoString(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, `"hello"`)

	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	// The string token should have "string" scope
	strToken := tokens[0]
	if !strToken.HasScope("string") {
		t.Errorf("string token scopes = %v, want string", strToken.Scopes)
	}
}

func TestTokenize_GoComment(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "// comment")

	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	commentToken := tokens[0]
	if !commentToken.HasScope("comment") {
		t.Errorf("comment token scopes = %v, want comment", commentToken.Scopes)
	}
}

func TestTokenize_GoBlockComment(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "/* block\ncomment */")

	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	// All tokens should be comments
	for _, tok := range tokens {
		if !tok.HasScope("comment") {
			t.Errorf("block comment token scopes = %v, want comment", tok.Scopes)
		}
	}
}

func TestTokenize_GoNumber(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "42")

	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	if !tokens[0].HasScope("constant.numeric") {
		t.Errorf("number token scopes = %v, want constant.numeric", tokens[0].Scopes)
	}
}

func TestTokenize_GoHexNumber(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "0xFF")
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	if !tokens[0].HasScope("constant.numeric") {
		t.Errorf("hex number scopes = %v, want constant.numeric", tokens[0].Scopes)
	}
}

func TestTokenize_GoRawString(t *testing.T) {
	lang := LangGo()
	tokens := Tokenize(lang, "`raw string`")
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	if !tokens[0].HasScope("string") {
		t.Errorf("raw string scopes = %v, want string", tokens[0].Scopes)
	}
}

func TestTokenize_GoFullSnippet(t *testing.T) {
	lang := LangGo()
	src := `package main

import "fmt"

func main() {
	// comment
	fmt.Println("hello")
}`
	tokens := Tokenize(lang, src)
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}

	// Verify we got keyword tokens
	hasPackage := false
	hasFunc := false
	hasImport := false
	for _, tok := range tokens {
		text := src[tok.StartIndex:tok.EndIndex]
		if text == "package" && tok.HasScope("keyword") {
			hasPackage = true
		}
		if text == "func" && tok.HasScope("keyword") {
			hasFunc = true
		}
		if text == "import" && tok.HasScope("keyword") {
			hasImport = true
		}
	}
	if !hasPackage {
		t.Error("package keyword not found")
	}
	if !hasFunc {
		t.Error("func keyword not found")
	}
	if !hasImport {
		t.Error("import keyword not found")
	}
}

func TestTokenize_MarkdownHeading(t *testing.T) {
	lang := LangMarkdown()
	tokens := Tokenize(lang, "# Hello World")
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	// Should have heading tokens
	hasHeading := false
	for _, tok := range tokens {
		if tok.HasScope("markup.heading") {
			hasHeading = true
			break
		}
	}
	if !hasHeading {
		t.Error("heading token not found")
	}
}

func TestTokenize_MarkdownCodeFence(t *testing.T) {
	lang := LangMarkdown()
	src := "```go\nfmt.Println()\n```"
	tokens := Tokenize(lang, src)
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	// Should have code fence tokens
	hasCodeFence := false
	for _, tok := range tokens {
		if tok.HasScope("string") {
			hasCodeFence = true
			break
		}
	}
	if !hasCodeFence {
		t.Error("code fence token not found")
	}
}

func TestTokenize_MarkdownBold(t *testing.T) {
	lang := LangMarkdown()
	tokens := Tokenize(lang, "**bold text**")
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	hasBold := false
	for _, tok := range tokens {
		if tok.HasScope("markup.bold") {
			hasBold = true
			break
		}
	}
	if !hasBold {
		t.Error("bold token not found")
	}
}

func TestTokenize_MarkdownLink(t *testing.T) {
	lang := LangMarkdown()
	tokens := Tokenize(lang, "[text](http://example.com)")
	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}
	hasLink := false
	for _, tok := range tokens {
		if tok.HasScope("string.link") {
			hasLink = true
			break
		}
	}
	if !hasLink {
		t.Error("link token not found")
	}
}

func TestHighlightStyle_Get(t *testing.T) {
	style := ThemeDarkPlus()

	// Test exact match
	s := style.Get([]string{"keyword", "control", "go"})
	if s.Color.R != 197 { // #C586C0
		t.Errorf("keyword.control color R = %d, want 197", s.Color.R)
	}

	// Test prefix match (keyword.control should match for "keyword.control.js")
	s = style.Get([]string{"keyword", "control", "js"})
	if s.Color.R != 197 {
		t.Errorf("keyword.control.js color R = %d, want 197", s.Color.R)
	}

	// Test shorter prefix match (keyword should match for "keyword.other")
	s = style.Get([]string{"keyword", "other"})
	if s.Color.R != 86 { // #569CD6
		t.Errorf("keyword.other color R = %d, want 86", s.Color.R)
	}
}

func TestHighlightStyle_GetString(t *testing.T) {
	style := ThemeDarkPlus()
	s := style.GetByString("string.quoted.double.go")
	if s.Color.R != 206 { // #CE9178
		t.Errorf("string color R = %d, want 206", s.Color.R)
	}
}

func TestHighlightStyle_Comment(t *testing.T) {
	style := ThemeDarkPlus()
	s := style.GetByString("comment.line.double-slash.go")
	if !s.Italic {
		t.Error("comment should be italic")
	}
	if s.Color.R != 106 { // #6A9955
		t.Errorf("comment color R = %d, want 106", s.Color.R)
	}
}

func TestToken_Eq(t *testing.T) {
	t1 := NewToken(0, 5, "keyword")
	t2 := NewToken(0, 5, "keyword")
	if t1.Length() != 5 {
		t.Errorf("Length = %d, want 5", t1.Length())
	}
	if t1.ScopeString() != "keyword" {
		t.Errorf("ScopeString = %q", t1.ScopeString())
	}
	_ = t2
}

// findTokenByText finds a token whose text (in src) matches the given string.
func findTokenByText(tokens []Token, text string, src string) *Token {
	for i := range tokens {
		tok := &tokens[i]
		if tok.StartIndex < len(src) && tok.EndIndex <= len(src) {
			if src[tok.StartIndex:tok.EndIndex] == text {
				return tok
			}
		}
	}
	return nil
}
