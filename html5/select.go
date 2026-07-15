package html5

import (
	"strings"

	"wb-ui/dom"
)

// --- HTMLSelectElement ---

// HTMLSelectElement wraps a <select> element. Mirrors
// WebCore::HTMLSelectElement.
type HTMLSelectElement struct {
	El *dom.Element
}

// ToSelectElement wraps an element as an HTMLSelectElement.
func ToSelectElement(el *dom.Element) (HTMLSelectElement, bool) {
	if el == nil || el.LocalName() != "select" {
		return HTMLSelectElement{}, false
	}
	return HTMLSelectElement{El: el}, true
}

// Options returns the <option> elements within this select (including those
// inside <optgroup>), in document order. Mirrors
// HTMLSelectElement::options().
func (s HTMLSelectElement) Options() []*dom.Element {
	var out []*dom.Element
	walkElements(s.El, func(el *dom.Element) {
		if el.LocalName() == "option" {
			out = append(out, el)
		}
	})
	return out
}

// Length returns the number of <option> elements.
// Mirrors HTMLSelectElement::length().
func (s HTMLSelectElement) Length() int {
	return len(s.Options())
}

// Multiple reports whether the select allows multiple selections.
func (s HTMLSelectElement) Multiple() bool {
	return attrBool(s.El, "multiple")
}

// SetMultiple sets the multiple attribute.
func (s HTMLSelectElement) SetMultiple(m bool) {
	if m {
		s.El.SetAttribute("multiple", "multiple")
	} else {
		s.El.RemoveAttribute("multiple")
	}
}

// Size returns the number of visible options, defaulting to 1 (or 4 if
// multiple).
func (s HTMLSelectElement) Size() int {
	if !s.El.HasAttribute("size") {
		if s.Multiple() {
			return 4
		}
		return 1
	}
	return attrInt(s.El, "size")
}

// Disabled reports whether the select is disabled.
func (s HTMLSelectElement) Disabled() bool {
	return attrBool(s.El, "disabled")
}

// SetDisabled sets the disabled state.
func (s HTMLSelectElement) SetDisabled(d bool) {
	if d {
		s.El.SetAttribute("disabled", "disabled")
	} else {
		s.El.RemoveAttribute("disabled")
	}
}

// Required reports whether the select is required.
func (s HTMLSelectElement) Required() bool {
	return attrBool(s.El, "required")
}

// Name returns the name attribute.
func (s HTMLSelectElement) Name() string {
	return s.El.GetAttribute("name")
}

// Form returns the enclosing form, or nil.
func (s HTMLSelectElement) Form() *dom.Element {
	return FindFormAncestor(s.El)
}

// SelectedIndex returns the index of the first selected option, or -1 if
// none is selected. Mirrors HTMLSelectElement::selectedIndex().
func (s HTMLSelectElement) SelectedIndex() int {
	opts := s.Options()
	for i, o := range opts {
		if o.HasAttribute("selected") {
			return i
		}
	}
	return -1
}

// SetSelectedIndex selects the option at the given index and clears any
// other selection. Mirrors HTMLSelectElement::setSelectedIndex().
func (s HTMLSelectElement) SetSelectedIndex(idx int) {
	opts := s.Options()
	for i, o := range opts {
		if i == idx {
			o.SetAttribute("selected", "selected")
		} else if !s.Multiple() {
			o.RemoveAttribute("selected")
		}
	}
}

// Value returns the value of the first selected option, or "" if none is
// selected. For a selected option without a value attribute, the option's
// text content is returned. Mirrors HTMLSelectElement::value().
func (s HTMLSelectElement) Value() string {
	for _, o := range s.Options() {
		if !o.HasAttribute("selected") {
			continue
		}
		v := o.GetAttribute("value")
		if v == "" {
			return strings.TrimSpace(textContent(o))
		}
		return v
	}
	return ""
}

// SetValue selects the option with the matching value and clears any other
// selection. If no option matches, all options are deselected.
func (s HTMLSelectElement) SetValue(v string) {
	for _, o := range s.Options() {
		optVal := o.GetAttribute("value")
		if optVal == "" {
			optVal = strings.TrimSpace(textContent(o))
		}
		if optVal == v {
			o.SetAttribute("selected", "selected")
		} else if !s.Multiple() {
			o.RemoveAttribute("selected")
		}
	}
}

// WillValidate reports whether the select participates in validation.
func (s HTMLSelectElement) WillValidate() bool {
	return !s.Disabled()
}

// Validity returns the ValidityState for this select. A required select is
// valid only if at least one option is selected (with non-empty value).
func (s HTMLSelectElement) Validity() ValidityState {
	v := ValidityState{}
	if s.Required() {
		hasValid := false
		for _, o := range s.Options() {
			if !o.HasAttribute("selected") {
				continue
			}
			val := o.GetAttribute("value")
			if val == "" {
				val = strings.TrimSpace(textContent(o))
			}
			if val != "" {
				hasValid = true
				break
			}
		}
		v.ValueMissing = !hasValid
	}
	// Check custom validity
	if msg, ok := customErrorMessages[s.El]; ok && msg != "" {
		v.CustomError = true
	}
	return v
}

// SetCustomValidity sets a custom error message for this select element.
func (s HTMLSelectElement) SetCustomValidity(message string) {
	customErrorMessages[s.El] = message
}

// CheckValidity returns true if the select's value satisfies all constraints.
func (s HTMLSelectElement) CheckValidity() bool {
	if !s.WillValidate() {
		return true
	}
	return s.Validity().Valid()
}

// --- HTMLOptionElement ---

// HTMLOptionElement wraps an <option> element. Mirrors
// WebCore::HTMLOptionElement.
type HTMLOptionElement struct {
	El *dom.Element
}

// ToOptionElement wraps an element as an HTMLOptionElement.
func ToOptionElement(el *dom.Element) (HTMLOptionElement, bool) {
	if el == nil || el.LocalName() != "option" {
		return HTMLOptionElement{}, false
	}
	return HTMLOptionElement{El: el}, true
}

// Value returns the value attribute, or the text content if no value
// attribute is set. Mirrors HTMLOptionElement::value().
func (o HTMLOptionElement) Value() string {
	if v := o.El.GetAttribute("value"); v != "" {
		return v
	}
	return strings.TrimSpace(textContent(o.El))
}

// SetValue sets the value attribute.
func (o HTMLOptionElement) SetValue(v string) {
	o.El.SetAttribute("value", v)
}

// Text returns the option's text content (trimmed).
// Mirrors HTMLOptionElement::text().
func (o HTMLOptionElement) Text() string {
	return strings.TrimSpace(textContent(o.El))
}

// SetText replaces the option's text content.
func (o HTMLOptionElement) SetText(t string) {
	_ = o.El.SetTextContent(t)
}

// Selected reports whether the option is currently selected.
func (o HTMLOptionElement) Selected() bool {
	return o.El.HasAttribute("selected")
}

// SetSelected sets the selected state of this option. Note: this does not
// automatically clear other options in the parent select unless the caller
// uses HTMLSelectElement.SetSelectedIndex / SetValue.
func (o HTMLOptionElement) SetSelected(s bool) {
	if s {
		o.El.SetAttribute("selected", "selected")
	} else {
		o.El.RemoveAttribute("selected")
	}
}

// DefaultSelected reports the selected attribute as set in HTML.
func (o HTMLOptionElement) DefaultSelected() bool {
	return o.El.HasAttribute("selected")
}

// Disabled reports whether the option is disabled.
func (o HTMLOptionElement) Disabled() bool {
	return attrBool(o.El, "disabled")
}

// SetDisabled sets the disabled state.
func (o HTMLOptionElement) SetDisabled(d bool) {
	if d {
		o.El.SetAttribute("disabled", "disabled")
	} else {
		o.El.RemoveAttribute("disabled")
	}
}

// Index returns the option's index within its parent select's options
// list, or -1 if not inside a select.
func (o HTMLOptionElement) Index() int {
	parent := o.El.ParentNode()
	// Skip <optgroup> and look for <select>.
	for p := parent; p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "select" {
			sel, _ := ToSelectElement(e)
			opts := sel.Options()
			for i, opt := range opts {
				if opt == o.El {
					return i
				}
			}
		}
	}
	return -1
}

// Form returns the enclosing form, or nil.
func (o HTMLOptionElement) Form() *dom.Element {
	// An option's form is the form of its enclosing select.
	for p := o.El.ParentNode(); p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "select" {
			return FindFormAncestor(e)
		}
	}
	return nil
}

// --- HTMLOptGroupElement ---

// HTMLOptGroupElement wraps an <optgroup> element.
type HTMLOptGroupElement struct {
	El *dom.Element
}

// ToOptGroupElement wraps an element as an HTMLOptGroupElement.
func ToOptGroupElement(el *dom.Element) (HTMLOptGroupElement, bool) {
	if el == nil || el.LocalName() != "optgroup" {
		return HTMLOptGroupElement{}, false
	}
	return HTMLOptGroupElement{El: el}, true
}

// Label returns the optgroup's label attribute.
func (g HTMLOptGroupElement) Label() string {
	return g.El.GetAttribute("label")
}

// SetLabel sets the optgroup's label attribute.
func (g HTMLOptGroupElement) SetLabel(l string) {
	g.El.SetAttribute("label", l)
}

// Disabled reports whether the optgroup is disabled.
func (g HTMLOptGroupElement) Disabled() bool {
	return attrBool(g.El, "disabled")
}

// SetDisabled sets the disabled state.
func (g HTMLOptGroupElement) SetDisabled(d bool) {
	if d {
		g.El.SetAttribute("disabled", "disabled")
	} else {
		g.El.RemoveAttribute("disabled")
	}
}

// --- HTMLDataListElement ---

// HTMLDataListElement wraps a <datalist> element. It provides a list of
// <option> suggestions for an associated <input list="..."> control.
type HTMLDataListElement struct {
	El *dom.Element
}

// ToDataListElement wraps an element as an HTMLDataListElement.
func ToDataListElement(el *dom.Element) (HTMLDataListElement, bool) {
	if el == nil || el.LocalName() != "datalist" {
		return HTMLDataListElement{}, false
	}
	return HTMLDataListElement{El: el}, true
}

// Options returns the <option> elements contained in this datalist.
func (d HTMLDataListElement) Options() []*dom.Element {
	var out []*dom.Element
	walkElements(d.El, func(el *dom.Element) {
		if el.LocalName() == "option" {
			out = append(out, el)
		}
	})
	return out
}

// Length returns the number of options.
func (d HTMLDataListElement) Length() int {
	return len(d.Options())
}

// SuggestionsFor returns the label strings of options whose value starts with
// the given prefix (case-insensitive). The label is the option's "label"
// attribute if present, otherwise the option's "value" attribute, and finally
// the option's text content.
// This is the primary API for autocomplete dropdown rendering.
func (d HTMLDataListElement) SuggestionsFor(prefix string) []string {
	if prefix == "" {
		return nil
	}
	lower := strings.ToLower(prefix)
	var out []string
	for _, opt := range d.Options() {
		v := strings.ToLower(opt.GetAttribute("value"))
		if strings.HasPrefix(v, lower) {
			// label priority: <label> attribute > <value> attribute > text content
			label := opt.GetAttribute("label")
			if label == "" {
				label = opt.GetAttribute("value")
			}
			if label == "" {
				label = strings.TrimSpace(textContent(opt))
			}
			if label != "" {
				out = append(out, label)
			}
		}
	}
	return out
}
