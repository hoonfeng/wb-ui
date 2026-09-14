package html5

import (
	"strings"

	"wb-ui/dom"
)

// HTMLTextAreaElement wraps a <textarea> element. Mirrors
// WebCore::HTMLTextAreaElement.
type HTMLTextAreaElement struct {
	El *dom.Element
}

// ToTextAreaElement wraps an element as an HTMLTextAreaElement.
func ToTextAreaElement(el *dom.Element) (HTMLTextAreaElement, bool) {
	if el == nil || el.LocalName() != "textarea" {
		return HTMLTextAreaElement{}, false
	}
	return HTMLTextAreaElement{El: el}, true
}

// Value returns the textarea's current value. For textareas, the API-level
// value is stored separately from the text content (defaultValue); here we
// use the text content as the value source.
// Mirrors HTMLTextAreaElement::value().
func (t HTMLTextAreaElement) Value() string {
	return textContent(t.El)
}

// SetValue sets the textarea's value. We store it as text content.
func (t HTMLTextAreaElement) SetValue(v string) {
	_ = t.El.SetTextContent(v)
}

// DefaultValue returns the textarea's default value (its initial text
// content).
func (t HTMLTextAreaElement) DefaultValue() string {
	return textContent(t.El)
}

// SetDefaultValue sets the default value (text content).
func (t HTMLTextAreaElement) SetDefaultValue(v string) {
	_ = t.El.SetTextContent(v)
}

// Name returns the name attribute.
func (t HTMLTextAreaElement) Name() string {
	return t.El.GetAttribute("name")
}

// Placeholder returns the placeholder attribute.
func (t HTMLTextAreaElement) Placeholder() string {
	return t.El.GetAttribute("placeholder")
}

// Disabled reports whether the textarea is disabled.
func (t HTMLTextAreaElement) Disabled() bool {
	return attrBool(t.El, "disabled")
}

// SetDisabled sets the disabled state.
func (t HTMLTextAreaElement) SetDisabled(d bool) {
	if d {
		t.El.SetAttribute("disabled", "disabled")
	} else {
		t.El.RemoveAttribute("disabled")
	}
}

// Required reports whether the textarea is required.
func (t HTMLTextAreaElement) Required() bool {
	return attrBool(t.El, "required")
}

// ReadOnly reports whether the textarea is read-only.
func (t HTMLTextAreaElement) ReadOnly() bool {
	return attrBool(t.El, "readonly")
}

// Rows returns the rows attribute, defaulting to 2.
func (t HTMLTextAreaElement) Rows() int {
	if !t.El.HasAttribute("rows") {
		return 2
	}
	return attrInt(t.El, "rows")
}

// Cols returns the cols attribute, defaulting to 20.
func (t HTMLTextAreaElement) Cols() int {
	if !t.El.HasAttribute("cols") {
		return 20
	}
	return attrInt(t.El, "cols")
}

// MaxLength returns the maxlength attribute, or -1 if unset.
func (t HTMLTextAreaElement) MaxLength() int {
	if !t.El.HasAttribute("maxlength") {
		return -1
	}
	return attrInt(t.El, "maxlength")
}

// MinLength returns the minlength attribute, or -1 if unset.
func (t HTMLTextAreaElement) MinLength() int {
	if !t.El.HasAttribute("minlength") {
		return -1
	}
	return attrInt(t.El, "minlength")
}

// Wrap returns the wrap attribute ("soft", "hard", or "off"), defaulting
// to "soft".
func (t HTMLTextAreaElement) Wrap() string {
	w := strings.ToLower(t.El.GetAttribute("wrap"))
	switch w {
	case "hard", "off":
		return w
	}
	return "soft"
}

// Form returns the enclosing form, or nil.
func (t HTMLTextAreaElement) Form() *dom.Element {
	return FindFormAncestor(t.El)
}

// Autofocus reports whether the textarea should autofocus.
func (t HTMLTextAreaElement) Autofocus() bool {
	return attrBool(t.El, "autofocus")
}

// --- Validation ---

// WillValidate reports whether the textarea participates in validation.
// Disabled and readonly textareas are barred from constraint validation
// (HTML §4.10.7 — see constraint.go's barredTextArea).
func (t HTMLTextAreaElement) WillValidate() bool {
	return !barredTextArea(t)
}

// Validity returns the ValidityState for this textarea.
func (t HTMLTextAreaElement) Validity() ValidityState {
	v := ValidityState{}
	val := t.Value()

	if t.Required() {
		v.ValueMissing = val == ""
	}

	maxLen := t.MaxLength()
	if maxLen >= 0 && len(val) > maxLen {
		v.TooLong = true
	}

	minLen := t.MinLength()
	if minLen >= 0 && len(val) < minLen && val != "" {
		v.TooShort = true
	}

	// Check custom validity
	if msg, ok := customErrorMessages[t.El]; ok && msg != "" {
		v.CustomError = true
	}
	return v
}

// SetCustomValidity sets a custom error message for this textarea.
func (t HTMLTextAreaElement) SetCustomValidity(message string) {
	customErrorMessages[t.El] = message
}

// CheckValidity returns true if the textarea satisfies all constraints.
func (t HTMLTextAreaElement) CheckValidity() bool {
	if !t.WillValidate() {
		return true
	}
	return t.Validity().Valid()
}
