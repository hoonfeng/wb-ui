package markdown

// Translation of: markdown-it/lib/index.mjs (partial — facade only)
// Completeness: 70%
//
// MarkdownIt is the main facade. It owns the three parser chains (core, block,
// inline), holds the options, and provides Parse/Render entry points.
//
// Simplifications for P0:
//   - ValidateLink: basic protocol check (javascript:, vbscript:, file:, data:)
//   - NormalizeLink: identity (no mdurl/punycode encoding)
//   - NormalizeLinkText: identity
//   - No linkify, typographer, smartquotes, replacements
//   - No HTML rendering (Phase 6); Render produces a debug tree dump

import (
	"fmt"
	"strings"
)

// Options holds parser configuration. Mirrors the options object in the JS
// MarkdownIt constructor.
type Options struct {
	Html       bool   // allow raw HTML in source (default false)
	XhtmlOut   bool   // add '/' to self-closing tags (default false)
	Breaks     bool   // convert \n in paragraphs to <br> (default false)
	LangPrefix string // CSS class prefix for fenced code (default "language-")
	Linkify    bool   // autoconvert URL-like text to links (default false)
	Typographer bool   // typographic replacements (default false)
	Quotes     string // smart quotes replacement pairs
	MaxNesting  int    // maximum nesting depth (default 100)
	Highlight   func(str, lang string) string // code highlighter
}

// DefaultOptions returns the default option values.
func DefaultOptions() Options {
	return Options{
		Html:       false,
		XhtmlOut:   false,
		Breaks:     false,
		LangPrefix: "language-",
		Linkify:    false,
		Typographer: false,
		Quotes:     "\"\"''",
		MaxNesting:  100,
		Highlight:   nil,
	}
}

// MarkdownIt is the main parser instance, mirroring the JS MarkdownIt class.
type MarkdownIt struct {
	Inline          *ParserInline
	Block           *ParserBlock
	Core            *ParserCore
	Options         Options
	ValidateLink    func(url string) bool
	NormalizeLink   func(url string) string
	NormalizeLinkText func(url string) string
}

// NewMarkdownIt creates a MarkdownIt with default options and rules registered.
func NewMarkdownIt() *MarkdownIt {
	md := &MarkdownIt{
		Options: DefaultOptions(),
	}
	md.ValidateLink = defaultValidateLink
	md.NormalizeLink = func(url string) string { return url }
	md.NormalizeLinkText = func(url string) string { return url }

	md.Inline = NewParserInline()
	md.Block = NewParserBlock()
	md.Core = NewParserCore()

	return md
}

// Set updates options (chainable). Mirrors MarkdownIt.prototype.set.
func (md *MarkdownIt) Set(opts Options) *MarkdownIt {
	md.Options = opts
	return md
}

// Parse converts source Markdown into a token stream. Mirrors
// MarkdownIt.prototype.parse.
func (md *MarkdownIt) Parse(src string, env map[string]interface{}) []Token {
	if env == nil {
		env = make(map[string]interface{})
	}
	state := NewStateCore(src, md, env)
	md.Core.Process(state)
	return state.Tokens
}

// RenderInline parses a single inline string (no block parsing). Mirrors
// MarkdownIt.prototype.renderInline (partially — returns tokens, not HTML).
func (md *MarkdownIt) ParseInline(src string, env map[string]interface{}) []Token {
	if env == nil {
		env = make(map[string]interface{})
	}
	state := NewStateCore(src, md, env)
	state.InlineMode = true
	md.Core.Process(state)
	return state.Tokens
}

// Render produces a simple string representation of the token tree for
// debugging purposes. This is NOT HTML rendering (Phase 6).
func (md *MarkdownIt) Render(src string) string {
	env := make(map[string]interface{})
	tokens := md.Parse(src, env)
	var b strings.Builder
	renderTokens(&b, tokens, 0)
	return b.String()
}

// renderTokens recursively renders tokens for debugging.
func renderTokens(b *strings.Builder, tokens []Token, indent int) {
	pad := strings.Repeat("  ", indent)
	for i := range tokens {
		t := &tokens[i]
		b.WriteString(pad)
		b.WriteString(fmt.Sprintf("[%d] %s", i, t.Type))
		if t.Tag != "" {
			b.WriteString(" <" + t.Tag + ">")
		}
		if t.Nesting != 0 {
			b.WriteString(fmt.Sprintf(" nesting=%d", t.Nesting))
		}
		if t.Level != 0 {
			b.WriteString(fmt.Sprintf(" level=%d", t.Level))
		}
		if t.Content != "" {
			content := t.Content
			if len(content) > 60 {
				content = content[:60] + "..."
			}
			content = strings.ReplaceAll(content, "\n", "\\n")
			b.WriteString(" content=" + quote(content))
		}
		if t.Markup != "" {
			b.WriteString(" markup=" + t.Markup)
		}
		if t.Info != "" {
			b.WriteString(" info=" + t.Info)
		}
		if t.Map != nil {
			b.WriteString(fmt.Sprintf(" map=[%d,%d]", t.Map[0], t.Map[1]))
		}
		if len(t.Attrs) > 0 {
			b.WriteString(" attrs={")
			for i, a := range t.Attrs {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(a[0] + "=" + quote(a[1]))
			}
			b.WriteString("}")
		}
		b.WriteString("\n")
		if len(t.Children) > 0 {
			renderTokens(b, t.Children, indent+1)
		}
	}
}

func quote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

// defaultValidateLink prohibits javascript:, vbscript:, file:, and most data:
// URLs. Mirrors the validateLink function in markdown-it/lib/index.mjs.
func defaultValidateLink(url string) bool {
	str := strings.TrimSpace(strings.ToLower(url))
	if strings.HasPrefix(str, "vbscript:") ||
		strings.HasPrefix(str, "javascript:") ||
		strings.HasPrefix(str, "file:") ||
		strings.HasPrefix(str, "data:") {
		// Only allow data:image/(gif|png|jpeg|webp);
		return strings.HasPrefix(str, "data:image/gif;") ||
			strings.HasPrefix(str, "data:image/png;") ||
			strings.HasPrefix(str, "data:image/jpeg;") ||
			strings.HasPrefix(str, "data:image/webp;")
	}
	return true
}
