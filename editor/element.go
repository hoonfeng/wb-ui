// <wb-editor> custom element registration for the HTML parser.
//
// When the HTML parser encounters a <wb-editor> tag, it calls the registered
// constructor which creates a host div element with data attributes storing
// the configuration (language, theme, etc.). After the DOM tree is fully
// built, InitEditorElement(s) should be called to create the EditorView
// instances and attach them to their host elements.
//
// Translation of: HTML spec "custom elements" + CM6 EditorView initialization

package editor

import (
	"sync"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/platform/graphics"
)

// editorElementData stores the association between a host DOM element and
// its EditorView instance.
type editorElementData struct {
	view *EditorView
	host *dom.Element
}

// activeEditors maps host DOM elements to their editor data.
var activeEditors sync.Map // *dom.Element → *editorElementData

// RegisterEditorElement registers <wb-editor> with the HTML parser's custom
// element registry. After calling this, parsing a document containing
// <wb-editor> tags will create host elements with data attributes instead
// of generic div elements. Call InitEditorElements after parsing to
// initialize the EditorView instances.
func RegisterEditorElement() {
	html.RegisterCustomElement("wb-editor", wbEditorConstructor)
}

// wbEditorConstructor is the custom element constructor for <wb-editor>.
// It creates a host div element and stores configuration as data attributes.
// The actual EditorView is created lazily when InitEditorElement is called,
// because the element's text content (initial code) is only available after
// the parser has finished inserting child nodes.
func wbEditorConstructor(doc *dom.Document, tagName string, attrs map[string]string) *dom.Element {
	host := doc.CreateElement("div")
	host.SetAttribute("data-wb-component", "editor")

	// Store configuration attributes.
	lang := attrs["language"]
	if lang == "" {
		lang = "text"
	}
	host.SetAttribute("data-wb-language", lang)

	if theme, ok := attrs["theme"]; ok {
		host.SetAttribute("data-wb-theme", theme)
	}
	if sn, ok := attrs["show-line-numbers"]; ok {
		host.SetAttribute("data-wb-show-line-numbers", sn)
	}
	if w, ok := attrs["width"]; ok {
		host.SetAttribute("data-wb-width", w)
	}
	if h, ok := attrs["height"]; ok {
		host.SetAttribute("data-wb-height", h)
	}

	return host
}

// InitEditorElement creates an EditorView for the given host element and
// returns it. The host must have been created by the <wb-editor> custom
// element constructor. If already initialized, returns the existing view.
func InitEditorElement(host *dom.Element) *EditorView {
	if host == nil {
		return nil
	}
	if host.GetAttribute("data-wb-component") != "editor" {
		return nil
	}

	// Check if already initialized.
	if existing, ok := activeEditors.Load(host); ok {
		if ed, ok := existing.(*editorElementData); ok {
			return ed.view
		}
	}

	// Parse configuration from data attributes.
	textContent := host.TextContent()

	// Create the initial state with the text content.
	sel := SelectionCaret(0)
	state := NewState(Config{
		Doc:       TextFromString(textContent),
		Selection: &sel,
	})

	// Parse optional width/height.
	width := 800.0
	height := 600.0
	// parse width/height from data attributes if present

	// Create the EditorView.
	font := graphics.Font{Family: "Consolas", Size: 14}
	view := NewEditorView(EditorViewConfig{
		State:           state,
		Font:            font,
		ShowLineNumbers: true,
		Width:           width,
		Height:          height,
	})

	// Set language if specified.
	lang := host.GetAttribute("data-wb-language")
	if lang != "" && lang != "text" {
		if langFn := lookupLanguage(lang); langFn != nil {
			view.SetLanguage(langFn())
		}
	}

	// Store the association.
	activeEditors.Store(host, &editorElementData{
		view: view,
		host: host,
	})

	return view
}

// InitEditorElements walks the given document and initializes all
// <wb-editor> elements found. It should be called after the HTML parser
// finishes building the DOM tree but before rendering.
func InitEditorElements(doc *dom.Document) {
	if doc == nil {
		return
	}
	walkEditorElements(doc, func(el *dom.Element) {
		InitEditorElement(el)
	})
}

// EditorElementForHost returns the EditorView associated with the given
// host element, or nil if not found.
func EditorElementForHost(host *dom.Element) *EditorView {
	if v, ok := activeEditors.Load(host); ok {
		if ed, ok := v.(*editorElementData); ok {
			return ed.view
		}
	}
	return nil
}

// lookupLanguage returns the Monarch language function for a given language
// name. Returns nil if the language is not recognised.
func lookupLanguage(name string) func() *MonarchLanguage {
	switch name {
	case "go", "golang":
		return LangGo
	case "js", "javascript":
		return LangJS
	case "md", "markdown":
		return LangMarkdown
	}
	return nil
}

// walkEditorElements traverses the DOM tree and calls fn for every Element
// that has data-wb-component="editor".
func walkEditorElements(node dom.Node, fn func(*dom.Element)) {
	if node == nil {
		return
	}
	if el, ok := node.(*dom.Element); ok {
		if el.GetAttribute("data-wb-component") == "editor" {
			fn(el)
		}
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		walkEditorElements(child, fn)
	}
}
