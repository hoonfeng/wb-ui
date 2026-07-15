// Fence code block syntax highlighter that bridges the markdown package
// to the editor package's Tokenizer + HighlightStyle.
//
// When a Markdown fenced code block (```go ... ```) is rendered to DOM,
// the EditorFenceHighlighter tokenizes the code using editor.Tokenize
// with the language's MonarchLanguage definition, then wraps each token
// in a <span> with:
//   - a CSS class derived from the token's scope (e.g. "tok-keyword",
//     "tok-string") so a stylesheet can color it, AND
//   - an inline style attribute carrying the HighlightStyle color,
//     so coloring works even without a stylesheet.
//
// This guarantees that fenced code in a Markdown view and code in the
// editor component share the SAME tokenizer and color scheme — there is
// a single source of truth for syntax colors.

package markdown

import (
	"fmt"
	"strings"

	"wb-ui/dom"
	"wb-ui/editor"
)

// EditorFenceHighlighter renders fenced code blocks using the editor
// package's Monarch tokenizer and HighlightStyle. It is the shared
// "single source of truth" bridge between the Markdown DOM renderer and
// the code editor.
type EditorFenceHighlighter struct {
	// languages maps a fence info string (e.g. "go", "js") to its
	// Monarch language definition. If a language is not present, the
	// code is emitted as plain text.
	languages map[string]*editor.MonarchLanguage
	// style is the HighlightStyle used to color tokens. If nil, only
	// CSS classes are emitted (no inline style attributes).
	style *editor.HighlightStyle
}

// NewEditorFenceHighlighter creates a highlighter with no languages
// registered. Call RegisterLanguage to add support for specific
// languages.
func NewEditorFenceHighlighter() *EditorFenceHighlighter {
	return &EditorFenceHighlighter{
		languages: map[string]*editor.MonarchLanguage{},
	}
}

// WithStyle installs a HighlightStyle for inline-style coloring.
// Returns the receiver for chaining.
func (h *EditorFenceHighlighter) WithStyle(style *editor.HighlightStyle) *EditorFenceHighlighter {
	h.style = style
	return h
}

// RegisterLanguage associates a fence info string with a Monarch
// language definition. Multiple aliases can map to the same language
// (e.g. "js", "javascript" → LangJS).
func (h *EditorFenceHighlighter) RegisterLanguage(name string, lang *editor.MonarchLanguage) *EditorFenceHighlighter {
	h.languages[strings.ToLower(name)] = lang
	return h
}

// RegisterDefaultLanguages registers the built-in language definitions
// (Go, JavaScript, Markdown) under their common names and aliases.
func (h *EditorFenceHighlighter) RegisterDefaultLanguages() *EditorFenceHighlighter {
	h.RegisterLanguage("go", editor.LangGo())
	h.RegisterLanguage("golang", editor.LangGo())
	h.RegisterLanguage("js", editor.LangJS())
	h.RegisterLanguage("javascript", editor.LangJS())
	h.RegisterLanguage("ts", editor.LangJS())
	h.RegisterLanguage("typescript", editor.LangJS())
	h.RegisterLanguage("markdown", editor.LangMarkdown())
	h.RegisterLanguage("md", editor.LangMarkdown())
	return h
}

// Highlight implements FenceHighlighter. It tokenizes src with the
// language identified by lang and returns a slice of dom.Node (Text and
// <span> elements) representing the colored code.
func (h *EditorFenceHighlighter) Highlight(doc *dom.Document, src, lang string) []dom.Node {
	lang = strings.ToLower(strings.TrimSpace(lang))
	ml, ok := h.languages[lang]
	if !ok {
		return []dom.Node{dom.NewText(doc, src)}
	}
	tokens := editor.Tokenize(ml, src)
	if len(tokens) == 0 {
		return []dom.Node{dom.NewText(doc, src)}
	}
	// If no style, emit tokens grouped into <span class="tok-..."> per
	// token. Adjacent tokens with the same scope are merged to reduce
	// node count.
	nodes := make([]dom.Node, 0, len(tokens))
	for _, tok := range tokens {
		text := src[tok.StartIndex:tok.EndIndex]
		if text == "" {
			continue
		}
		scope := tok.ScopeString()
		span := doc.CreateElement("span")
		span.SetAttribute("class", scopeClass(scope))
		applyInlineStyle(span, scope, h.style)
		_ = span.AppendChild(dom.NewText(doc, text))
		nodes = append(nodes, span)
	}
	if len(nodes) == 0 {
		return []dom.Node{dom.NewText(doc, src)}
	}
	return nodes
}

// scopeClass converts a dotted scope string (e.g. "keyword.control.go")
// to a CSS class string (e.g. "tok-keyword-control-go"). Tokens with no
// scope get the "tok-text" class.
func scopeClass(scope string) string {
	if scope == "" {
		return "tok-text"
	}
	// Replace dots with dashes and prefix with "tok-".
	return "tok-" + strings.ReplaceAll(scope, ".", "-")
}

// applyInlineStyle sets an inline style attribute on the span based on
// the HighlightStyle's TextStyle for the given scope. If no style is
// set or no match is found, the span is left unstyled (CSS class only).
func applyInlineStyle(span *dom.Element, scope string, style *editor.HighlightStyle) {
	if style == nil {
		return
	}
	ts := style.GetByString(scope)
	var parts []string
	if ts.Color.A != 0 {
		parts = append(parts, fmt.Sprintf("color:#%02x%02x%02x", ts.Color.R, ts.Color.G, ts.Color.B))
	}
	if ts.Bold {
		parts = append(parts, "font-weight:bold")
	}
	if ts.Italic {
		parts = append(parts, "font-style:italic")
	}
	if ts.Underline {
		parts = append(parts, "text-decoration:underline")
	}
	if len(parts) > 0 {
		span.SetAttribute("style", strings.Join(parts, ";"))
	}
}
