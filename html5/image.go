// Translation of: Source/WebCore/html/HTMLImageElement.h
//                  Source/WebCore/html/HTMLImageElement.cpp
// Completeness: 50%
// Simplifications:
//   - no actual image decoding / bitmap loading; dimensions come from the
//     width/height attributes only
//   - no srcset / sizes / loading=lazy / referrerpolicy / crossorigin support
//   - no onload / onerror event dispatch
//
// HTMLImageElement wraps an <img> element and provides accessors for the
// src, alt, width and height attributes. The rendering package uses the
// width/height attributes to size the replaced-element box.

package html5

import (
	"strconv"

	"wb-ui/dom"
)

// HTMLImageElement wraps an <img> element. Mirrors
// WebCore::HTMLImageElement.
type HTMLImageElement struct {
	El *dom.Element
}

// ToImageElement wraps an element as an HTMLImageElement.
func ToImageElement(el *dom.Element) (HTMLImageElement, bool) {
	if el == nil || el.LocalName() != "img" {
		return HTMLImageElement{}, false
	}
	return HTMLImageElement{El: el}, true
}

// Src returns the src attribute, mirroring HTMLImageElement::src().
func (img HTMLImageElement) Src() string {
	return img.El.GetAttribute("src")
}

// SetSrc sets the src attribute.
func (img HTMLImageElement) SetSrc(s string) {
	img.El.SetAttribute("src", s)
}

// Alt returns the alt attribute, mirroring HTMLImageElement::alt().
func (img HTMLImageElement) Alt() string {
	return img.El.GetAttribute("alt")
}

// Width returns the width attribute (CSS pixels), or 0 if not set.
// Mirrors HTMLImageElement::width().
func (img HTMLImageElement) Width() int {
	if !img.El.HasAttribute("width") {
		return 0
	}
	return attrInt(img.El, "width")
}

// Height returns the height attribute (CSS pixels), or 0 if not set.
// Mirrors HTMLImageElement::height().
func (img HTMLImageElement) Height() int {
	if !img.El.HasAttribute("height") {
		return 0
	}
	return attrInt(img.El, "height")
}

// SetWidth sets the width attribute.
func (img HTMLImageElement) SetWidth(w int) {
	img.El.SetAttribute("width", strconv.Itoa(w))
}

// SetHeight sets the height attribute.
func (img HTMLImageElement) SetHeight(h int) {
	img.El.SetAttribute("height", strconv.Itoa(h))
}

// NaturalWidth returns the intrinsic width of the loaded image. Since this
// port does not decode image bitmaps yet, it returns the width attribute
// value (or 0 if not set). Mirrors HTMLImageElement::naturalWidth().
func (img HTMLImageElement) NaturalWidth() int { return img.Width() }

// NaturalHeight returns the intrinsic height. Mirrors
// HTMLImageElement::naturalHeight().
func (img HTMLImageElement) NaturalHeight() int { return img.Height() }

// Complete reports whether the image has finished loading. Mirrors
// HTMLImageElement::complete(). Returns true when the src attribute is
// empty (a "broken" image is still "complete" per spec).
func (img HTMLImageElement) Complete() bool {
	// In a full browser this checks if the bitmap has been decoded.
	// Here we consider the image complete if it has a data-loaded marker
	// or if the src is empty (trivially complete).
	if !img.El.HasAttribute("src") {
		return true
	}
	return img.El.HasAttribute("data-loaded")
}

// IsMapped reports whether the image has a usemap attribute, mirroring
// HTMLImageElement::isMapped().
func (img HTMLImageElement) IsMapped() bool {
	return img.El.HasAttribute("usemap")
}

// UseMap returns the usemap attribute value, mirroring
// HTMLImageElement::useMap().
func (img HTMLImageElement) UseMap() string {
	return img.El.GetAttribute("usemap")
}
