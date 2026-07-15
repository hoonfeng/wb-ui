// Package widgets provides custom HTML element integrations for the
// wb-editor and wb-markdown custom elements.
//
// <wb-markdown> is a block element whose text content is parsed as
// GitHub Flavored Markdown and replaced with the resulting DOM subtree
// during render tree construction. This lets markdown content
// participate in CSS layout and Skia painting like native HTML.
//
// <wb-editor> is a block element that hosts a CM6-style code editor
// (see the editor package). The editor view is lazily created on first
// paint and cached in an EditorRegistry keyed by the DOM element. The
// language attribute selects syntax highlighting (go, javascript,
// markdown); the show-line-numbers attribute toggles the gutter.
package widgets

import (
	"strings"
	"sync"

	"wb-ui/dom"
	"wb-ui/editor"
	"wb-ui/markdown"
	"wb-ui/platform/graphics"
)

// --- <wb-markdown> ---

// ProcessMarkdownElements walks the document and transforms every
// <wb-markdown> element: its text content is parsed as GFM and the
// resulting DOM nodes replace the original text children. This should
// be called after the HTML parser finishes building the DOM but before
// the render tree is constructed.
//
// If a <wb-markdown> element has a data-gfm="false" attribute, plain
// CommonMark is used instead of GFM.
func ProcessMarkdownElements(doc *dom.Document) {
	if doc == nil {
		return
	}
	walkElements(doc, func(el *dom.Element) {
		if el.LocalName() != "wb-markdown" {
			return
		}
		transformMarkdownElement(el)
	})
}

// walkElements traverses the DOM tree and calls fn for every Element.
func walkElements(node dom.Node, fn func(*dom.Element)) {
	if el, ok := node.(*dom.Element); ok {
		fn(el)
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		walkElements(c, fn)
	}
}

// transformMarkdownElement parses the text content of a <wb-markdown>
// element and replaces it with the rendered DOM.
func transformMarkdownElement(el *dom.Element) {
	// Collect text content.
	src := elementTextContent(el)
	if strings.TrimSpace(src) == "" {
		return
	}

	// Determine whether to use GFM (default) or plain CommonMark.
	useGFM := el.GetAttribute("data-gfm") != "false"

	// Parse markdown to DOM.
	doc := el.OwnerDocument()
	var frag *dom.DocumentFragment
	if useGFM {
		frag = markdown.ParseToDOMWithGFM(src, doc)
	} else {
		frag = markdown.ParseToDOM(src, doc)
	}

	// Remove existing children.
	for c := el.FirstChild(); c != nil; {
		next := c.NextSibling()
		el.RemoveChild(c)
		c = next
	}

	// Append the rendered DOM nodes.
	for _, c := range frag.ChildNodes() {
		el.AppendChild(c)
	}
}

// elementTextContent returns the concatenated text content of all
// descendant Text nodes.
func elementTextContent(el *dom.Element) string {
	var sb strings.Builder
	var walk func(dom.Node)
	walk = func(n dom.Node) {
		if t, ok := n.(*dom.Text); ok {
			sb.WriteString(t.Data())
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(el)
	return sb.String()
}

// --- <wb-editor> ---

// EditorRegistry manages EditorView instances for <wb-editor> elements.
// Each <wb-editor> element gets a lazily-created EditorView on first
// access. The view is configured from the element's attributes and text
// content.
type EditorRegistry struct {
	mu     sync.Mutex
	editors map[*dom.Element]*editor.EditorView
}

// NewEditorRegistry creates an empty registry.
func NewEditorRegistry() *EditorRegistry {
	return &EditorRegistry{editors: map[*dom.Element]*editor.EditorView{}}
}

// GetOrCreate returns the EditorView for the given <wb-editor> element,
// creating it on first access. The element's language attribute and text
// content are read at creation time.
func (r *EditorRegistry) GetOrCreate(el *dom.Element) *editor.EditorView {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.editors[el]; ok {
		return v
	}
	v := createEditorView(el)
	r.editors[el] = v
	return v
}

// Get returns the EditorView for the element, or nil if not yet created.
func (r *EditorRegistry) Get(el *dom.Element) *editor.EditorView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.editors[el]
}

// Remove deletes the editor view for the given element.
func (r *EditorRegistry) Remove(el *dom.Element) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.editors, el)
}

// All returns all (element, view) pairs in the registry.
func (r *EditorRegistry) All() map[*dom.Element]*editor.EditorView {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[*dom.Element]*editor.EditorView, len(r.editors))
	for k, v := range r.editors {
		out[k] = v
	}
	return out
}

// createEditorView builds an EditorView from a <wb-editor> DOM element.
// Attributes:
//   - language: "go", "javascript", "js", "markdown", "md" (default: plain)
//   - show-line-numbers: "true"/"false" (default: true)
//   - theme: "dark"/"light" (default: dark)
//   - width, height: CSS pixels (default: 400x300)
//
// The element's text content is used as the initial document text.
func createEditorView(el *dom.Element) *editor.EditorView {
	src := elementTextContent(el)
	// Strip a leading newline that comes from <wb-editor>\n  code\n</wb-editor>.
	src = strings.TrimPrefix(src, "\n")

	lang := strings.ToLower(el.GetAttribute("language"))
	if lang == "" {
		lang = strings.ToLower(el.GetAttribute("lang"))
	}

	var monarch *editor.MonarchLanguage
	var theme *editor.HighlightStyle = editor.ThemeDarkPlus()
	switch lang {
	case "go", "golang":
		monarch = editor.LangGo()
	case "javascript", "js":
		monarch = editor.LangJS()
	case "markdown", "md":
		monarch = editor.LangMarkdown()
	}

	showLineNumbers := el.GetAttribute("show-line-numbers") != "false"

	font := graphics.Font{
		Family: "Consolas",
		Size:   14,
		Weight: 400,
		Style:  "normal",
	}

	config := editor.EditorViewConfig{
		State: editor.NewState(editor.Config{
			Doc: editor.TextFromString(src),
		}),
		Font:            font,
		Theme:           theme,
		Lang:            monarch,
		ShowLineNumbers:  showLineNumbers,
		Width:           400,
		Height:          300,
	}
	return editor.NewEditorView(config)
}

// IsEditorElement reports whether the given element is a <wb-editor>.
func IsEditorElement(el *dom.Element) bool {
	return el != nil && el.LocalName() == "wb-editor"
}

// IsMarkdownElement reports whether the given element is a <wb-markdown>.
func IsMarkdownElement(el *dom.Element) bool {
	return el != nil && el.LocalName() == "wb-markdown"
}

// PaintEditor paints the editor view for a <wb-editor> element at the
// given position and size. If no editor view has been created yet, one
// is created lazily.
func (r *EditorRegistry) PaintEditor(el *dom.Element, canvas *graphics.Canvas, x, y, w, h float64) {
	if !IsEditorElement(el) {
		return
	}
	v := r.GetOrCreate(el)
	if v == nil {
		return
	}
	// Update the view dimensions to match the laid-out box.
	v.SetSize(w, h)
	editor.PaintEditor(canvas, v, x, y, w, h)
}
