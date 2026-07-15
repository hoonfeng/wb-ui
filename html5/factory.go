// Package html5 — element factory registration.
//
// This file registers specialized element constructors with the dom package's
// element factory so that Document.CreateElement returns elements that carry
// the correct tag name semantics for form-associated HTML elements.
//
// Without these registrations, document.CreateElement("input") returns a
// generic *dom.Element, and ToInputElement(el) always succeeds only if
// el.LocalName() == "input". With registration, the factory creates elements
// in a consistent way that future enhancements can extend (e.g. embedding
// default values, validity state, or other per-type metadata).
//
// Registration happens in init(), so it is active as soon as the html5 package
// is imported anywhere in the binary (typically via the page or rendering
// packages).
package html5

import (
	"wb-ui/dom"
)

func init() {
	dom.RegisterElement("input", newHTMLInputElement)
	dom.RegisterElement("form", newHTMLFormElement)
	dom.RegisterElement("button", newHTMLButtonElement)
	dom.RegisterElement("textarea", newHTMLTextAreaElement)
	dom.RegisterElement("select", newHTMLSelectElement)
	dom.RegisterElement("option", newHTMLOptionElement)
	dom.RegisterElement("optgroup", newHTMLOptGroupElement)
	dom.RegisterElement("label", newHTMLLabelElement)
	dom.RegisterElement("fieldset", newHTMLFieldSetElement)
	dom.RegisterElement("legend", newHTMLLegendElement)
	dom.RegisterElement("datalist", newHTMLDataListElement)
	dom.RegisterElement("output", newHTMLOutputElement)
	dom.RegisterElement("progress", newHTMLProgressElement)
	dom.RegisterElement("meter", newHTMLMeterElement)
	dom.RegisterElement("script", newHTMLScriptElement)
	dom.RegisterElement("link", newHTMLLinkElement)
	dom.RegisterElement("a", newHTMLAnchorElement)
	dom.RegisterElement("img", newHTMLImageElement)
}

// Each constructor creates a *dom.Element via dom.NewElement. In the current
// architecture, the html5 wrapper types (HTMLInputElement, etc.) work by
// wrapping a *dom.Element whose tag matches. The factory guarantees that
// elements created through Document.CreateElement have the correct tag and
// document owner. Future improvements can extend these constructors to embed
// per-element metadata (e.g. default attribute values, validity state) without
// changing the public API.

func newHTMLInputElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLFormElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLButtonElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLTextAreaElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLSelectElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLOptionElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLOptGroupElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLLabelElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLFieldSetElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLLegendElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLDataListElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLOutputElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLProgressElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLMeterElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLScriptElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLLinkElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLAnchorElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}

func newHTMLImageElement(doc *dom.Document, tagName string) *dom.Element {
	return dom.NewElement(doc, tagName)
}
