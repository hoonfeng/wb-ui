// Custom element registry for the HTML parser.
// Allows registering constructors for custom tag names
// so that the parser instantiates the corresponding Go component instead of a
// generic dom.Element.
//
// Translation of: HTML spec "custom elements" section (simplified)
// Completeness: 40%
// Simplifications:
//   - no autonomous vs. customized built-in distinction
//   - no observedAttributes / attributeChangedCallback
//   - no connectedCallback / disconnectedCallback / adoptedCallback
//   - only the constructor (which creates the element) is registered;
//     lifecycle callbacks are handled by the component itself

package html

import "wb-ui/dom"

// CustomElementConstructor is a function that creates a custom element
// when the HTML parser encounters a registered tag name.
// doc is the document the element belongs to, tagName is the lowercased tag name,
// and attrs are the parsed HTML attributes (first-wins for duplicates).
type CustomElementConstructor func(doc *dom.Document, tagName string, attrs map[string]string) *dom.Element

// customElementRegistry is the global registry mapping tag names to constructors.
var customElementRegistry = map[string]CustomElementConstructor{}

// RegisterCustomElement registers a constructor for the given tag name.
// When the HTML parser encounters a start tag with this name, it will call
// the constructor instead of doc.CreateElement(tagName).
// If already registered, the old constructor is replaced.
func RegisterCustomElement(tagName string, ctor CustomElementConstructor) {
	customElementRegistry[tagName] = ctor
}

// lookupCustomElement checks if a custom element constructor is registered
// for the given tag name. Returns the constructor and true if found.
func lookupCustomElement(tagName string) (CustomElementConstructor, bool) {
	ctor, ok := customElementRegistry[tagName]
	return ctor, ok
}
