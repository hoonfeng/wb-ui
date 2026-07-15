package markdown

// parser_test.go — CommonMark test cases for the markdown-it Go translation.
// These tests verify the token stream produced by Parse() matches expected
// CommonMark behavior.

import (
	"strings"
	"testing"
)

// helper: find first token of a given type
func findToken(tokens []Token, typ string) *Token {
	for i := range tokens {
		if tokens[i].Type == typ {
			return &tokens[i]
		}
	}
	return nil
}

// helper: collect all tokens of a given type
func findTokens(tokens []Token, typ string) []*Token {
	var result []*Token
	for i := range tokens {
		if tokens[i].Type == typ {
			result = append(result, &tokens[i])
		}
	}
	return result
}

// helper: collect inline children text content
func childrenText(tokens []Token) string {
	var b strings.Builder
	for i := range tokens {
		if tokens[i].Type == "text" {
			b.WriteString(tokens[i].Content)
		}
	}
	return b.String()
}

func TestParagraph(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("Hello world", nil)
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", len(tokens), tokens)
	}
	if tokens[0].Type != "paragraph_open" || tokens[0].Tag != "p" {
		t.Errorf("expected paragraph_open, got %s", tokens[0].Type)
	}
	if tokens[1].Type != "inline" || tokens[1].Content != "Hello world" {
		t.Errorf("expected inline 'Hello world', got %s '%s'", tokens[1].Type, tokens[1].Content)
	}
	if tokens[2].Type != "paragraph_close" {
		t.Errorf("expected paragraph_close, got %s", tokens[2].Type)
	}
}

func TestATXHeading(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("# Heading 1", nil)
	open := findToken(tokens, "heading_open")
	if open == nil || open.Tag != "h1" {
		t.Errorf("expected h1 heading_open, got %+v", open)
	}
	inline := findToken(tokens, "inline")
	if inline == nil || inline.Content != "Heading 1" {
		t.Errorf("expected inline 'Heading 1', got %+v", inline)
	}
}

func TestATXHeadingLevel(t *testing.T) {
	md := NewMarkdownIt()
	for level := 1; level <= 6; level++ {
		src := strings.Repeat("#", level) + " Test"
		tokens := md.Parse(src, nil)
		open := findToken(tokens, "heading_open")
		if open == nil {
			t.Errorf("level %d: no heading_open", level)
			continue
		}
		wantTag := "h" + intToStr(level)
		if open.Tag != wantTag {
			t.Errorf("level %d: expected tag %s, got %s", level, wantTag, open.Tag)
		}
	}
}

func TestSetextHeading(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("Heading\n===", nil)
	open := findToken(tokens, "heading_open")
	if open == nil || open.Tag != "h1" {
		t.Errorf("expected h1 setext heading, got %+v", open)
	}
}

func TestSetextHeadingLevel2(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("Heading\n---", nil)
	open := findToken(tokens, "heading_open")
	if open == nil || open.Tag != "h2" {
		t.Errorf("expected h2 setext heading, got %+v", open)
	}
}

func TestEmphasis(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("*italic*", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	emOpen := findToken(inline.Children, "em_open")
	if emOpen == nil {
		t.Fatalf("no em_open in children: %+v", inline.Children)
	}
	text := findToken(inline.Children, "text")
	if text == nil || text.Content != "italic" {
		t.Errorf("expected text 'italic', got %+v", text)
	}
}

func TestStrong(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("**bold**", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	strongOpen := findToken(inline.Children, "strong_open")
	if strongOpen == nil {
		t.Fatalf("no strong_open in children: %+v", inline.Children)
	}
	// After the isStrong fix, the outer emphasis markers are cleared to
	// empty text tokens (content=""). Find the non-empty text token.
	var text *Token
	for i := range inline.Children {
		if inline.Children[i].Type == "text" && inline.Children[i].Content != "" {
			text = &inline.Children[i]
			break
		}
	}
	if text == nil || text.Content != "bold" {
		t.Errorf("expected text 'bold', got %+v", text)
	}
}

func TestStrikethrough(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("~~strike~~", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	sOpen := findToken(inline.Children, "s_open")
	if sOpen == nil {
		t.Fatalf("no s_open in children: %+v", inline.Children)
	}
}

func TestInlineCode(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("`code`", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	code := findToken(inline.Children, "code_inline")
	if code == nil || code.Content != "code" {
		t.Errorf("expected code_inline 'code', got %+v", code)
	}
}

func TestFencedCodeBlock(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("```go\nfmt.Println()\n```", nil)
	fence := findToken(tokens, "fence")
	if fence == nil {
		t.Fatal("no fence token")
	}
	if fence.Info != "go" {
		t.Errorf("expected info 'go', got '%s'", fence.Info)
	}
	if !strings.Contains(fence.Content, "fmt.Println()") {
		t.Errorf("expected content to contain 'fmt.Println()', got '%s'", fence.Content)
	}
}

func TestIndentedCodeBlock(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("    code block", nil)
	code := findToken(tokens, "code_block")
	if code == nil {
		t.Fatal("no code_block token")
	}
	if !strings.Contains(code.Content, "code block") {
		t.Errorf("expected 'code block' in content, got '%s'", code.Content)
	}
}

func TestBlockquote(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("> quote", nil)
	open := findToken(tokens, "blockquote_open")
	if open == nil {
		t.Fatal("no blockquote_open token")
	}
	close := findToken(tokens, "blockquote_close")
	if close == nil {
		t.Fatal("no blockquote_close token")
	}
}

func TestUnorderedList(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("- item 1\n- item 2", nil)
	open := findToken(tokens, "bullet_list_open")
	if open == nil || open.Tag != "ul" {
		t.Errorf("expected bullet_list_open (ul), got %+v", open)
	}
	items := findTokens(tokens, "list_item_open")
	if len(items) != 2 {
		t.Errorf("expected 2 list items, got %d", len(items))
	}
}

func TestOrderedList(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("1. first\n2. second", nil)
	open := findToken(tokens, "ordered_list_open")
	if open == nil || open.Tag != "ol" {
		t.Errorf("expected ordered_list_open (ol), got %+v", open)
	}
	items := findTokens(tokens, "list_item_open")
	if len(items) != 2 {
		t.Errorf("expected 2 list items, got %d", len(items))
	}
}

func TestHorizontalRule(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("---", nil)
	// --- could be interpreted as setext heading if preceded by text,
	// but on its own it's an hr
	hr := findToken(tokens, "hr")
	if hr == nil {
		t.Fatalf("no hr token: %+v", tokens)
	}
}

func TestInlineLink(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("[text](http://example.com)", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	linkOpen := findToken(inline.Children, "link_open")
	if linkOpen == nil {
		t.Fatalf("no link_open in children: %+v", inline.Children)
	}
	href := linkOpen.AttrGet("href")
	if href != "http://example.com" {
		t.Errorf("expected href 'http://example.com', got '%s'", href)
	}
	text := findToken(inline.Children, "text")
	if text == nil || text.Content != "text" {
		t.Errorf("expected text 'text', got %+v", text)
	}
}

func TestInlineLinkWithTitle(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse(`[text](http://example.com "Title")`, nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	linkOpen := findToken(inline.Children, "link_open")
	if linkOpen == nil {
		t.Fatal("no link_open")
	}
	title := linkOpen.AttrGet("title")
	if title != "Title" {
		t.Errorf("expected title 'Title', got '%s'", title)
	}
}

func TestReferenceLink(t *testing.T) {
	md := NewMarkdownIt()
	src := "[text][ref]\n\n[ref]: http://example.com"
	tokens := md.Parse(src, nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	linkOpen := findToken(inline.Children, "link_open")
	if linkOpen == nil {
		t.Fatalf("no link_open in children: %+v", inline.Children)
	}
	href := linkOpen.AttrGet("href")
	if href != "http://example.com" {
		t.Errorf("expected href 'http://example.com', got '%s'", href)
	}
}

func TestImage(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("![alt](http://example.com/img.png)", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	img := findToken(inline.Children, "image")
	if img == nil {
		t.Fatalf("no image token in children: %+v", inline.Children)
	}
	src := img.AttrGet("src")
	if src != "http://example.com/img.png" {
		t.Errorf("expected src 'http://example.com/img.png', got '%s'", src)
	}
}

func TestAutolink(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("<http://example.com>", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	linkOpen := findToken(inline.Children, "link_open")
	if linkOpen == nil {
		t.Fatalf("no link_open in children: %+v", inline.Children)
	}
	href := linkOpen.AttrGet("href")
	if href != "http://example.com" {
		t.Errorf("expected href 'http://example.com', got '%s'", href)
	}
}

func TestEscape(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("\\*not emphasis\\*", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	emOpen := findToken(inline.Children, "em_open")
	if emOpen != nil {
		t.Errorf("should not have em_open for escaped asterisks")
	}
	text := childrenText(inline.Children)
	if !strings.Contains(text, "*not emphasis*") {
		t.Errorf("expected '*not emphasis*' in text, got '%s'", text)
	}
}

func TestHardBreak(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("line 1  \nline 2", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	hb := findToken(inline.Children, "hardbreak")
	if hb == nil {
		t.Errorf("expected hardbreak token, got: %+v", inline.Children)
	}
}

func TestSoftBreak(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("line 1\nline 2", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	sb := findToken(inline.Children, "softbreak")
	if sb == nil {
		t.Errorf("expected softbreak token, got: %+v", inline.Children)
	}
}

func TestMultipleParagraphs(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("First paragraph.\n\nSecond paragraph.", nil)
	opens := findTokens(tokens, "paragraph_open")
	if len(opens) != 2 {
		t.Errorf("expected 2 paragraph_open tokens, got %d", len(opens))
	}
}

func TestNestedEmphasis(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("**bold *italic* bold**", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	strongOpen := findToken(inline.Children, "strong_open")
	if strongOpen == nil {
		t.Fatalf("no strong_open: %+v", inline.Children)
	}
	emOpen := findToken(inline.Children, "em_open")
	if emOpen == nil {
		t.Errorf("no em_open inside strong: %+v", inline.Children)
	}
}

func TestRender(t *testing.T) {
	md := NewMarkdownIt()
	result := md.Render("# Hello\n\nThis is a **test**.")
	if !strings.Contains(result, "heading_open") {
		t.Errorf("render should contain heading_open: %s", result)
	}
	if !strings.Contains(result, "strong_open") {
		t.Errorf("render should contain strong_open: %s", result)
	}
}

func TestEmptyInput(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("", nil)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d", len(tokens))
	}
}

func TestCodeBlockInList(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("- item\n\n        code", nil)
	listOpen := findToken(tokens, "bullet_list_open")
	if listOpen == nil {
		t.Errorf("expected bullet_list_open: %+v", tokens)
	}
}

func TestNestedBlockquote(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("> > nested", nil)
	opens := findTokens(tokens, "blockquote_open")
	if len(opens) != 2 {
		t.Errorf("expected 2 blockquote_open tokens, got %d: %+v", len(opens), tokens)
	}
}

func TestEmphasisUnderscore(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("_italic_", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	emOpen := findToken(inline.Children, "em_open")
	if emOpen == nil {
		t.Errorf("expected em_open for _italic_: %+v", inline.Children)
	}
}

func TestStrongUnderscore(t *testing.T) {
	md := NewMarkdownIt()
	tokens := md.Parse("__bold__", nil)
	inline := findToken(tokens, "inline")
	if inline == nil {
		t.Fatal("no inline token")
	}
	strongOpen := findToken(inline.Children, "strong_open")
	if strongOpen == nil {
		t.Errorf("expected strong_open for __bold__: %+v", inline.Children)
	}
}
