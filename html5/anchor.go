// Translation of: Source/WebCore/html/HTMLAnchorElement.h
//                  Source/WebCore/html/HTMLAnchorElement.cpp
// Completeness: 60%
// Simplifications:
//   - no ping / download / hreflang / type attribute accessors
//   - no link-click activation / navigation hook (the Host handles it)
//
// HTMLAnchorElement wraps an <a> element and provides accessors for the
// href, target, rel attributes and the link text. The app host uses this
// to implement click-to-navigate behaviour.

package html5

import (
	"strings"

	"wb-ui/dom"
)

// HTMLAnchorElement wraps an <a> element. Mirrors
// WebCore::HTMLAnchorElement.
type HTMLAnchorElement struct {
	El *dom.Element
}

// ToAnchorElement wraps an element as an HTMLAnchorElement.
func ToAnchorElement(el *dom.Element) (HTMLAnchorElement, bool) {
	if el == nil || el.LocalName() != "a" {
		return HTMLAnchorElement{}, false
	}
	return HTMLAnchorElement{El: el}, true
}

// Href returns the href attribute, mirroring HTMLAnchorElement::href().
func (a HTMLAnchorElement) Href() string {
	return a.El.GetAttribute("href")
}

// SetHref sets the href attribute.
func (a HTMLAnchorElement) SetHref(h string) {
	a.El.SetAttribute("href", h)
}

// Target returns the target attribute (_self, _blank, etc.), lowercased.
// Mirrors HTMLAnchorElement::target().
func (a HTMLAnchorElement) Target() string {
	return strings.ToLower(a.El.GetAttribute("target"))
}

// Text returns the anchor's text content, mirroring HTMLAnchorElement::text().
func (a HTMLAnchorElement) Text() string {
	return a.El.TextContent()
}

// Rel returns the rel attribute, lowercased.
// Mirrors HTMLAnchorElement::rel().
func (a HTMLAnchorElement) Rel() string {
	return strings.ToLower(a.El.GetAttribute("rel"))
}

// IsExternalLink reports whether the link points to an external URL
// (starts with http:// or https://).
func (a HTMLAnchorElement) IsExternalLink() bool {
	href := a.Href()
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}
