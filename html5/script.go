// Translation of: Source/WebCore/html/HTMLScriptElement.h
//                  Source/WebCore/html/HTMLScriptElement.cpp
// Completeness: 70%
// Simplifications:
//   - no external script loading / network fetch
//   - no script preload scanning / async / defer scheduling
//   - no module script type support
//   - no script text mutation observer / dynamic injection hooks
//
// HTMLScriptElement wraps a <script> element and provides accessors for
// script content, src, type, and boolean attributes (async, defer). It is
// used by the Frame script-execution pipeline to find and run inline scripts.

package html5

import (
	"strings"

	"wb-ui/dom"
)

// HTMLScriptElement wraps a <script> element. Mirrors
// WebCore::HTMLScriptElement.
type HTMLScriptElement struct {
	El *dom.Element
}

// ToScriptElement wraps an element as an HTMLScriptElement.
func ToScriptElement(el *dom.Element) (HTMLScriptElement, bool) {
	if el == nil || el.LocalName() != "script" {
		return HTMLScriptElement{}, false
	}
	return HTMLScriptElement{El: el}, true
}

// Text returns the script content (text content of the element), mirroring
// HTMLScriptElement::text().
func (s HTMLScriptElement) Text() string {
	return s.El.TextContent()
}

// Src returns the src attribute (external script URL), or "" for inline
// scripts that have no src attribute. Mirrors HTMLScriptElement::src().
func (s HTMLScriptElement) Src() string {
	return s.El.GetAttribute("src")
}

// Type returns the script's type attribute, defaulting to "text/javascript"
// when the attribute is empty or a standard JS MIME type. Mirrors
// HTMLScriptElement::type().
func (s HTMLScriptElement) Type() string {
	t := strings.ToLower(s.El.GetAttribute("type"))
	if t == "" || t == "text/javascript" || t == "application/javascript" ||
		t == "application/ecmascript" {
		return "text/javascript"
	}
	return t
}

// IsAsync returns true if the async attribute is set. Mirrors
// HTMLScriptElement::async().
func (s HTMLScriptElement) IsAsync() bool {
	return s.El.HasAttribute("async")
}

// IsDefer returns true if the defer attribute is set. Mirrors
// HTMLScriptElement::defer().
func (s HTMLScriptElement) IsDefer() bool {
	return s.El.HasAttribute("defer")
}
