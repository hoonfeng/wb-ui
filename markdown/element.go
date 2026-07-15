// <wb-markdown> custom element registration for the HTML parser.
//
// When the HTML parser encounters a <wb-markdown> tag, it calls the registered
// constructor which creates a host div element with data attributes storing
// the configuration. After the DOM tree is fully built, InitMarkdownElements
// should be called to parse the text content as Markdown and replace the
// element's children with the rendered DOM subtree.
//
// This file follows the same architecture as editor/element.go.

package markdown

import (
	"sync"

	"wb-ui/dom"
	"wb-ui/html"
)

// markdownElementData stores metadata for a <wb-markdown> host element.
type markdownElementData struct {
	host *dom.Element
}

// activeMarkdownElements maps host DOM elements to their metadata.
var activeMarkdownElements sync.Map

// RegisterMarkdownElement registers <wb-markdown> with the HTML parser's
// custom element registry. After calling this, parsing a document containing
// <wb-markdown> tags will create host elements with data attributes instead
// of generic div elements. Call InitMarkdownElements after parsing to
// render the Markdown content.
func RegisterMarkdownElement() {
	html.RegisterCustomElement("wb-markdown", wbMarkdownConstructor)
}

// wbMarkdownConstructor is the custom element constructor for <wb-markdown>.
// It creates a host div element and stores configuration as data attributes.
// The actual Markdown rendering happens lazily in InitMarkdownElement,
// because the element's text content is only available after the parser
// has finished inserting child text nodes.
func wbMarkdownConstructor(doc *dom.Document, tagName string, attrs map[string]string) *dom.Element {
	host := doc.CreateElement("div")
	host.SetAttribute("data-wb-component", "markdown")

	// Store all parsed attributes as data-* attributes for later use.
	for k, v := range attrs {
		host.SetAttribute("data-wb-"+k, v)
	}

	return host
}

// InitMarkdownElement parses the text content of host as Markdown and
// replaces the host's children with the rendered DOM subtree.
// If already initialized, this is a no-op.
func InitMarkdownElement(host *dom.Element) {
	if host == nil {
		return
	}
	if host.GetAttribute("data-wb-component") != "markdown" {
		return
	}

	// Check if already initialized.
	if _, loaded := activeMarkdownElements.LoadOrStore(host, &markdownElementData{host: host}); loaded {
		return
	}

	src := host.TextContent()
	doc := host.OwnerDocument()
	if doc == nil {
		return
	}

	// Check whether GFM mode is enabled.
	gfmAttr := host.GetAttribute("data-wb-gfm")
	var fragment *dom.DocumentFragment
	if gfmAttr == "false" || gfmAttr == "0" {
		fragment = ParseToDOM(src, doc)
	} else {
		fragment = ParseToDOMWithGFM(src, doc)
	}

	// Clear existing children and append rendered content.
	for host.FirstChild() != nil {
		_ = host.RemoveChild(host.FirstChild())
	}
	for child := fragment.FirstChild(); child != nil; child = fragment.FirstChild() {
		_ = host.AppendChild(child)
	}
}

// InitMarkdownElements walks the given document and initializes all
// <wb-markdown> elements found. It should be called after the HTML parser
// finishes building the DOM tree but before rendering.
func InitMarkdownElements(doc *dom.Document) {
	if doc == nil {
		return
	}
	walkMarkdownElements(doc, func(el *dom.Element) {
		InitMarkdownElement(el)
	})
}

// walkMarkdownElements traverses the DOM tree and calls fn for every
// Element that has data-wb-component="markdown".
func walkMarkdownElements(node dom.Node, fn func(*dom.Element)) {
	if node == nil {
		return
	}
	if el, ok := node.(*dom.Element); ok {
		if el.GetAttribute("data-wb-component") == "markdown" {
			fn(el)
		}
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		walkMarkdownElements(child, fn)
	}
}
