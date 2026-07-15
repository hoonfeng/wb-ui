// Translation of: Source/WebCore/html/HTMLLinkElement.h
//                  Source/WebCore/html/HTMLLinkElement.cpp
// Completeness: 60%
// Simplifications:
//   - no preload scanning / resource loader integration
//   - no cross-origin / integrity / nonce / CSP checks
//   - only rel=stylesheet is handled; other rel values (icon, preload, dns-prefetch, etc.) are ignored
//
// HTMLLinkElement wraps a <link> element and provides accessors for the
// rel, href and type attributes. The Frame's extractAndAddStyles method
// uses this to discover and load external stylesheets.

package html5

import (
	"strings"

	"wb-ui/dom"
)

// HTMLLinkElement wraps a <link> element. Mirrors
// WebCore::HTMLLinkElement.
type HTMLLinkElement struct {
	El *dom.Element
}

// ToLinkElement wraps an element as an HTMLLinkElement.
func ToLinkElement(el *dom.Element) (HTMLLinkElement, bool) {
	if el == nil || el.LocalName() != "link" {
		return HTMLLinkElement{}, false
	}
	return HTMLLinkElement{El: el}, true
}

// Rel returns the rel attribute (e.g., "stylesheet"), lowercased.
// Mirrors HTMLLinkElement::rel().
func (l HTMLLinkElement) Rel() string {
	return strings.ToLower(l.El.GetAttribute("rel"))
}

// Href returns the href attribute (URL of the linked resource).
// Mirrors HTMLLinkElement::href().
func (l HTMLLinkElement) Href() string {
	return l.El.GetAttribute("href")
}

// Type returns the type attribute (e.g., "text/css"), lowercased.
// Mirrors HTMLLinkElement::type().
func (l HTMLLinkElement) Type() string {
	return strings.ToLower(l.El.GetAttribute("type"))
}

// IsStyleSheet reports whether this link is a stylesheet link
// (rel="stylesheet" or rel="alternate stylesheet").
func (l HTMLLinkElement) IsStyleSheet() bool {
	rel := l.Rel()
	return rel == "stylesheet" || rel == "alternate stylesheet"
}
