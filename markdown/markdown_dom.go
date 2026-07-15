// High-level Markdown → wb-ui DOM conversion entry points.
//
// This file provides convenience functions that combine parsing (Phase 5)
// and DOM rendering (Phase 6) into a single call, with optional fence
// syntax highlighting via the editor package.

package markdown

import (
	"wb-ui/dom"
	"wb-ui/editor"
)

// ParseToDOM parses src as Markdown and returns a DocumentFragment
// containing the rendered DOM tree. It uses a fresh MarkdownIt with
// default options and no fence highlighting.
//
// For fenced code block syntax coloring, use ParseToDOMWithHighlighter.
func ParseToDOM(src string, doc *dom.Document) *dom.DocumentFragment {
	md := NewMarkdownIt()
	r := NewDOMRenderer(doc, md)
	return r.Render(md.Parse(src, nil))
}

// ParseToDOMWithGFM parses src as GFM (GitHub Flavored Markdown) and
// returns a DocumentFragment. GFM enables tables, task lists, and bare
// URL autolinking. Strikethrough is always on.
func ParseToDOMWithGFM(src string, doc *dom.Document) *dom.DocumentFragment {
	md := NewMarkdownItWithGFM()
	r := NewDOMRenderer(doc, md)
	return r.Render(md.Parse(src, nil))
}

// ParseToDOMWithHighlighter parses src as Markdown and renders it to a
// DocumentFragment, using the provided FenceHighlighter for fenced
// code blocks. Use NewEditorFenceHighlighter to bridge to the editor
// package's tokenizer + HighlightStyle.
func ParseToDOMWithHighlighter(src string, doc *dom.Document, h FenceHighlighter) *dom.DocumentFragment {
	md := NewMarkdownIt()
	r := NewDOMRenderer(doc, md).SetFenceHighlighter(h)
	return r.Render(md.Parse(src, nil))
}

// ParseToDOMWithEditor is a convenience wrapper that installs an
// EditorFenceHighlighter backed by the given HighlightStyle and the
// built-in language definitions (Go, JS, Markdown). This is the
// "single source of truth" path: fenced code in Markdown and code in
// the editor share the same tokenizer and colors.
//
// If style is nil, only CSS classes (tok-keyword, tok-string, ...) are
// emitted; an external stylesheet must provide colors. If style is
// non-nil, inline style attributes are also set.
func ParseToDOMWithEditor(src string, doc *dom.Document, style *editor.HighlightStyle) *dom.DocumentFragment {
	h := NewEditorFenceHighlighter().
		RegisterDefaultLanguages().
		WithStyle(style)
	return ParseToDOMWithHighlighter(src, doc, h)
}
