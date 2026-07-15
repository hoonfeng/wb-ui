package html5

import "wb-ui/dom"

// --- HTMLDetailsElement ---

// HTMLDetailsElement wraps a <details> element. Mirrors
// WebCore::HTMLDetailsElement.
type HTMLDetailsElement struct {
	El *dom.Element
}

// ToDetailsElement wraps an element as an HTMLDetailsElement.
func ToDetailsElement(el *dom.Element) (HTMLDetailsElement, bool) {
	if el == nil || el.LocalName() != "details" {
		return HTMLDetailsElement{}, false
	}
	return HTMLDetailsElement{El: el}, true
}

// Open reports whether the details are visible (the "open" attribute is
// present). Mirrors HTMLDetailsElement::open().
func (d HTMLDetailsElement) Open() bool {
	return d.El.HasAttribute("open")
}

// SetOpen toggles the visibility of the details content. When set to true,
// the "open" attribute is added and the content becomes visible. When set
// to false, the "open" attribute is removed and the content is hidden
// (via the UA stylesheet rule details:not([open]) > :not(summary)).
// Mirrors HTMLDetailsElement::setOpen().
func (d HTMLDetailsElement) SetOpen(o bool) {
	if o {
		d.El.SetAttribute("open", "open")
	} else {
		d.El.RemoveAttribute("open")
	}
}

// Toggle switches the open state and returns the new state.
func (d HTMLDetailsElement) Toggle() bool {
	d.SetOpen(!d.Open())
	return d.Open()
}

// Summary returns the first <summary> child element, or nil if none exists.
func (d HTMLDetailsElement) Summary() *dom.Element {
	for c := d.El.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "summary" {
			return el
		}
	}
	return nil
}

// --- HTMLSummaryElement ---

// HTMLSummaryElement wraps a <summary> element. Mirrors
// WebCore::HTMLSummaryElement.
type HTMLSummaryElement struct {
	El *dom.Element
}

// ToSummaryElement wraps an element as an HTMLSummaryElement.
func ToSummaryElement(el *dom.Element) (HTMLSummaryElement, bool) {
	if el == nil || el.LocalName() != "summary" {
		return HTMLSummaryElement{}, false
	}
	return HTMLSummaryElement{El: el}, true
}
