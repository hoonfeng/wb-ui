// Translation of: markdown-it/lib/renderer.mjs
//
// Completeness: 90%
// Differences from markdown-it:
//   - Outputs wb-ui dom.Element tree instead of HTML string. This allows
//     the markdown-rendered content to participate in wb-ui's CSS layout
//     and Skia rendering pipeline directly (no HTML parse round-trip).
//   - Token rules are implemented as methods on DOMRenderer rather than
//     a dynamic rules map, since Go does not have JS's object property
//     model. The rule dispatch is by token Type string via a switch.
//   - Fence code blocks delegate to a FenceHighlighter interface so the
//     editor's Tokenizer + HighlightStyle can be shared.
//
// The DOMRenderer walks the token stream and builds a DocumentFragment.
// Block-level open/close tokens push/pop an element stack; inline tokens
// (the Children of an `inline` token) are rendered into the current
// block element.

package markdown

import (
	"strings"

	"wb-ui/dom"
)

// FenceHighlighter renders a fenced code block's content into syntax-
// highlighted dom.Node children. If nil, the code is emitted as plain
// text inside a <code><pre> pair.
//
// Implementations typically call editor.Tokenize with the fence's info
// string as the language, then wrap each token in a <span> with a class
// derived from the HighlightStyle. See fence_highlight.go for the
// default implementation that bridges to the editor package.
type FenceHighlighter interface {
	// Highlight renders src (the code) with lang (the info string, e.g.
	// "go", "javascript") into a slice of dom.Node. The returned nodes
	// are appended as children of the <code> element. If lang is empty
	// or unsupported, implementations should return a single Text node
	// containing src.
	Highlight(doc *dom.Document, src, lang string) []dom.Node
}

// FenceHighlightFunc is a function adapter for FenceHighlighter.
type FenceHighlightFunc func(doc *dom.Document, src, lang string) []dom.Node

// Highlight calls the wrapped function.
func (f FenceHighlightFunc) Highlight(doc *dom.Document, src, lang string) []dom.Node {
	if f == nil {
		return nil
	}
	return f(doc, src, lang)
}

// DOMRenderer converts a markdown-it token stream into a wb-ui DOM
// DocumentFragment. Mirrors the Renderer class in markdown-it/lib/renderer.mjs.
type DOMRenderer struct {
	// doc is the owning document for created nodes.
	doc *dom.Document
	// md is the source MarkdownIt instance (used for options like breaks).
	md *MarkdownIt
	// env is the parse environment (used for reference lookups).
	env map[string]interface{}
	// fenceHighlighter, if non-nil, renders fenced code blocks with
	// syntax highlighting shared with the editor.
	fenceHighlighter FenceHighlighter
	// stack is the current block element stack. The top of the stack
	// is where newly-rendered block content is appended.
	stack []*dom.Element
}

// NewDOMRenderer creates a renderer bound to the given document and
// MarkdownIt instance.
func NewDOMRenderer(doc *dom.Document, md *MarkdownIt) *DOMRenderer {
	return &DOMRenderer{
		doc: doc,
		md:  md,
		env: nil,
	}
}

// SetFenceHighlighter installs a fence highlighter for syntax-colored
// code blocks.
func (r *DOMRenderer) SetFenceHighlighter(h FenceHighlighter) *DOMRenderer {
	r.fenceHighlighter = h
	return r
}

// Render converts tokens into a DocumentFragment. Mirrors
// Renderer.prototype.render.
func (r *DOMRenderer) Render(tokens []Token) *dom.DocumentFragment {
	frag := dom.NewDocumentFragment(r.doc)
	// The stack holds open block elements so inline tokens know where
	// to append. It starts empty; block open tokens push, close tokens
	// pop. The root fragment is passed directly to renderTokens as the
	// initial parent for top-level block tokens.
	r.stack = nil
	r.renderTokens(tokens, frag)
	return frag
}

// renderTokens renders a slice of tokens as children of parent.
func (r *DOMRenderer) renderTokens(tokens []Token, parent dom.Node) {
	i := 0
	for i < len(tokens) {
		tok := &tokens[i]
		// Hidden tokens are skipped (e.g. paragraph_open in tight lists).
		if tok.Hidden {
			i++
			continue
		}

		// Dispatch on token type.
		handled := r.renderToken(tok, parent)
		if !handled {
			// Unknown token type: try generic handling by nesting.
			target := r.currentParent(parent)
			if tok.Nesting == 1 {
				// Open: create element from tag and push.
				if tok.Tag != "" {
					el := r.doc.CreateElement(tok.Tag)
					r.applyAttrs(el, tok.Attrs)
					_ = target.AppendChild(el)
					r.stack = append(r.stack, el)
				} else {
					// Virtual open (no tag): push a nil sentinel so close
					// knows not to pop.
					r.stack = append(r.stack, nil)
				}
			} else if tok.Nesting == -1 {
				// Close: pop.
				if len(r.stack) > 0 {
					r.stack = r.stack[:len(r.stack)-1]
				}
			} else {
				// Self-closing: create element if tag present.
				if tok.Tag != "" {
					el := r.doc.CreateElement(tok.Tag)
					r.applyAttrs(el, tok.Attrs)
					_ = target.AppendChild(el)
					if len(tok.Children) > 0 {
						r.renderTokens(tok.Children, el)
					}
				} else if tok.Content != "" {
					_ = target.AppendChild(dom.NewText(r.doc, tok.Content))
				}
			}
		}
		i++
	}
}

// currentParent returns the element that new children should be appended
// to. This is the top of the open-element stack if non-empty, otherwise
// the fallback parent passed to renderTokens. This is the core mechanism
// that lets inline tokens like text/code_inline append to the nearest
// enclosing element (e.g. <em> inside <p>) rather than the root.
func (r *DOMRenderer) currentParent(fallback dom.Node) dom.Node {
	if len(r.stack) > 0 {
		top := r.stack[len(r.stack)-1]
		if top != nil {
			return top
		}
	}
	return fallback
}

// renderToken handles a single token by type. Returns true if handled.
func (r *DOMRenderer) renderToken(tok *Token, parent dom.Node) bool {
	// For most tokens, new children should be appended to the current
	// open element (stack top). Compute this once.
	target := r.currentParent(parent)

	switch tok.Type {
	// --- Block container opens ---
	case "paragraph_open":
		return r.openElement(tok, parent, "p")
	case "paragraph_close":
		return r.closeElement(tok)

	case "heading_open":
		return r.openElement(tok, parent, tok.Tag)
	case "heading_close":
		return r.closeElement(tok)

	case "blockquote_open":
		return r.openElement(tok, parent, "blockquote")
	case "blockquote_close":
		return r.closeElement(tok)

	case "bullet_list_open":
		return r.openElement(tok, parent, "ul")
	case "bullet_list_close":
		return r.closeElement(tok)

	case "ordered_list_open":
		return r.openElement(tok, parent, "ol")
	case "ordered_list_close":
		return r.closeElement(tok)

	case "list_item_open":
		// Task list detection is performed in the "inline" case below,
		// where the first text child is inspected for a [ ] or [x]
		// marker. Here we just open the <li> element.
		return r.openElement(tok, parent, "li")
	case "list_item_close":
		return r.closeElement(tok)

	// --- GFM table tokens ---
	case "table_open":
		return r.openElement(tok, parent, "table")
	case "table_close":
		return r.closeElement(tok)
	case "thead_open":
		return r.openElement(tok, parent, "thead")
	case "thead_close":
		return r.closeElement(tok)
	case "tbody_open":
		return r.openElement(tok, parent, "tbody")
	case "tbody_close":
		return r.closeElement(tok)
	case "tr_open":
		return r.openElement(tok, parent, "tr")
	case "tr_close":
		return r.closeElement(tok)
	case "th_open":
		return r.openElement(tok, parent, "th")
	case "th_close":
		return r.closeElement(tok)
	case "td_open":
		return r.openElement(tok, parent, "td")
	case "td_close":
		return r.closeElement(tok)

	// --- Block leaf tokens ---
	case "inline":
		// The inline token carries Children (the inline token stream).
		// Render them into the current open block element.
		// GFM task list: if the target is a <li> (tight list), check
		// the first text token for a task marker [ ] or [x].
		if el, ok := target.(*dom.Element); ok && el.LocalName() == "li" {
			if checkbox, stripped, ok := tryTaskList(tok); ok {
				input := r.doc.CreateElement("input")
				input.SetAttribute("type", "checkbox")
				input.SetAttribute("disabled", "disabled")
				if checkbox {
					input.SetAttribute("checked", "checked")
				}
				_ = el.AppendChild(input)
				_ = el.AppendChild(dom.NewText(r.doc, " "))
				el.SetClassName(el.GetClassName() + " task-list-item")
				r.renderTokens(stripped, target)
				return true
			}
		}
		r.renderTokens(tok.Children, target)
		return true

	case "code_block":
		// Indented code block: <pre><code>content</code></pre>
		pre := r.doc.CreateElement("pre")
		code := r.doc.CreateElement("code")
		_ = code.AppendChild(dom.NewText(r.doc, tok.Content))
		_ = pre.AppendChild(code)
		_ = target.AppendChild(pre)
		return true

	case "fence":
		// Fenced code block. Use highlighter if available.
		pre := r.doc.CreateElement("pre")
		code := r.doc.CreateElement("code")
		if tok.Info != "" {
			// Add language class (e.g. "language-go").
			lang := strings.TrimSpace(tok.Info)
			lang = strings.Fields(lang)[0] // first token is the language
			if lang != "" {
				code.SetAttribute("class", r.md.Options.LangPrefix+lang)
			}
		}
		if r.fenceHighlighter != nil && tok.Info != "" {
			lang := strings.TrimSpace(strings.Fields(tok.Info)[0])
			nodes := r.fenceHighlighter.Highlight(r.doc, tok.Content, lang)
			for _, n := range nodes {
				_ = code.AppendChild(n)
			}
		} else {
			_ = code.AppendChild(dom.NewText(r.doc, tok.Content))
		}
		_ = pre.AppendChild(code)
		_ = target.AppendChild(pre)
		return true

	case "hr":
		_ = target.AppendChild(r.doc.CreateElement("hr"))
		return true

	case "html_block":
		if r.md.Options.Html {
			temp := r.doc.CreateElement("div")
			_ = temp.SetInnerHTML(tok.Content)
			for _, c := range temp.ChildNodes() {
				_ = target.AppendChild(c)
			}
		} else {
			_ = target.AppendChild(dom.NewText(r.doc, tok.Content))
		}
		return true

	// --- Inline tokens ---
	case "text":
		_ = target.AppendChild(dom.NewText(r.doc, tok.Content))
		return true

	case "text_special":
		_ = target.AppendChild(dom.NewText(r.doc, tok.Content))
		return true

	case "code_inline":
		code := r.doc.CreateElement("code")
		_ = code.AppendChild(dom.NewText(r.doc, tok.Content))
		_ = target.AppendChild(code)
		return true

	case "hardbreak":
		_ = target.AppendChild(r.doc.CreateElement("br"))
		return true

	case "softbreak":
		if r.md.Options.Breaks {
			_ = target.AppendChild(r.doc.CreateElement("br"))
		} else {
			_ = target.AppendChild(dom.NewText(r.doc, "\n"))
		}
		return true

	case "em_open":
		return r.openElement(tok, parent, "em")
	case "em_close":
		return r.closeElement(tok)

	case "strong_open":
		return r.openElement(tok, parent, "strong")
	case "strong_close":
		return r.closeElement(tok)

	case "s_open":
		return r.openElement(tok, parent, "s")
	case "s_close":
		return r.closeElement(tok)

	case "link_open":
		return r.openElement(tok, parent, "a")
	case "link_close":
		return r.closeElement(tok)

	case "image":
		img := r.doc.CreateElement("img")
		r.applyAttrs(img, tok.Attrs)
		if len(tok.Children) > 0 {
			altText := inlineText(tok.Children)
			if altText != "" {
				img.SetAttribute("alt", altText)
			}
		}
		_ = target.AppendChild(img)
		return true

	case "html_inline":
		if r.md.Options.Html {
			temp := r.doc.CreateElement("span")
			_ = temp.SetInnerHTML(tok.Content)
			for _, c := range temp.ChildNodes() {
				_ = target.AppendChild(c)
			}
		} else {
			_ = target.AppendChild(dom.NewText(r.doc, tok.Content))
		}
		return true
	}

	return false
}

// openElement creates a new element for an opening token, applies
// attributes, appends it to the current open element (or parent), and
// pushes it on the stack.
func (r *DOMRenderer) openElement(tok *Token, parent dom.Node, tagName string) bool {
	if tagName == "" {
		tagName = tok.Tag
	}
	target := r.currentParent(parent)
	if tagName == "" {
		// Virtual open: push nil marker.
		r.stack = append(r.stack, nil)
		return true
	}
	el := r.doc.CreateElement(tagName)
	r.applyAttrs(el, tok.Attrs)
	_ = target.AppendChild(el)
	r.stack = append(r.stack, el)
	return true
}

// closeElement pops the stack for a closing token.
func (r *DOMRenderer) closeElement(tok *Token) bool {
	if len(r.stack) > 0 {
		r.stack = r.stack[:len(r.stack)-1]
	}
	return true
}

// applyAttrs copies token attributes onto an element.
func (r *DOMRenderer) applyAttrs(el *dom.Element, attrs [][2]string) {
	for _, a := range attrs {
		el.SetAttribute(a[0], a[1])
	}
}

// inlineText extracts the plain-text content of an inline token slice
// (used for image alt text).
func inlineText(tokens []Token) string {
	var sb strings.Builder
	for i := range tokens {
		t := &tokens[i]
		switch t.Type {
		case "text", "text_special":
			sb.WriteString(t.Content)
		case "softbreak", "hardbreak":
			sb.WriteString("\n")
		case "code_inline":
			sb.WriteString(t.Content)
		default:
			if len(t.Children) > 0 {
				sb.WriteString(inlineText(t.Children))
			}
		}
	}
	return sb.String()
}
