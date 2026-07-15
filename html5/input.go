package html5

import (
	"regexp"
	"strconv"
	"strings"

	"wb-ui/dom"
)

// HTMLInputElement wraps a *dom.Element whose tag is "input" and
// provides type-safe accessors for form control properties.
// Mirrors WebCore::HTMLInputElement.
type HTMLInputElement struct {
	El *dom.Element
}

// ToInputElement wraps an element as an HTMLInputElement. Returns
// false if el is nil or not an <input> element.
func ToInputElement(el *dom.Element) (HTMLInputElement, bool) {
	if el == nil || el.LocalName() != "input" {
		return HTMLInputElement{}, false
	}
	return HTMLInputElement{El: el}, true
}

// Type returns the input's type attribute, defaulting to "text".
// Mirrors HTMLInputElement::type().
func (i HTMLInputElement) Type() InputType {
	t := strings.ToLower(i.El.GetAttribute("type"))
	if t == "" {
		return InputText
	}
	if IsKnownInputType(t) {
		return InputType(t)
	}
	return InputText
}

// SetType sets the type attribute.
func (i HTMLInputElement) SetType(t InputType) {
	i.El.SetAttribute("type", string(t))
}

// Value returns the current value. For most types this is the value
// attribute; for checkbox/radio it's "on" when checked, "" when unchecked.
// Mirrors HTMLInputElement::value().
func (i HTMLInputElement) Value() string {
	switch i.Type() {
	case InputCheckbox, InputRadio:
		if i.Checked() {
			v := i.El.GetAttribute("value")
			if v == "" {
				return "on"
			}
			return v
		}
		return ""
	case InputFile:
		// File input value is read-only (the selected file path).
		return i.El.GetAttribute("value")
	default:
		v := i.El.GetAttribute("value")
		return v
	}
}

// SetValue sets the value attribute. Mirrors HTMLInputElement::setValue().
func (i HTMLInputElement) SetValue(v string) {
	i.El.SetAttribute("value", v)
}

// DefaultValue returns the value attribute as set in HTML.
func (i HTMLInputElement) DefaultValue() string {
	return i.El.GetAttribute("value")
}

// SetDefaultValue sets the value attribute.
func (i HTMLInputElement) SetDefaultValue(v string) {
	i.El.SetAttribute("value", v)
}

// Checked reports whether a checkbox/radio is checked.
// Mirrors HTMLInputElement::checked().
func (i HTMLInputElement) Checked() bool {
	return i.El.HasAttribute("checked")
}

// SetChecked sets the checked state.
func (i HTMLInputElement) SetChecked(c bool) {
	if c {
		i.El.SetAttribute("checked", "checked")
	} else {
		i.El.RemoveAttribute("checked")
	}
}

// DefaultChecked returns the default checked state from HTML.
func (i HTMLInputElement) DefaultChecked() bool {
	return i.El.HasAttribute("checked")
}

// Disabled reports whether the input is disabled.
func (i HTMLInputElement) Disabled() bool {
	return attrBool(i.El, "disabled")
}

// SetDisabled sets the disabled state.
func (i HTMLInputElement) SetDisabled(d bool) {
	if d {
		i.El.SetAttribute("disabled", "disabled")
	} else {
		i.El.RemoveAttribute("disabled")
	}
}

// Required reports whether the input is required.
func (i HTMLInputElement) Required() bool {
	return attrBool(i.El, "required")
}

// ReadOnly reports whether the input is read-only.
func (i HTMLInputElement) ReadOnly() bool {
	return attrBool(i.El, "readonly")
}

// Name returns the name attribute.
func (i HTMLInputElement) Name() string {
	return i.El.GetAttribute("name")
}

// Placeholder returns the placeholder attribute.
func (i HTMLInputElement) Placeholder() string {
	return i.El.GetAttribute("placeholder")
}

// Min returns the min attribute.
func (i HTMLInputElement) Min() string {
	return i.El.GetAttribute("min")
}

// Max returns the max attribute.
func (i HTMLInputElement) Max() string {
	return i.El.GetAttribute("max")
}

// Step returns the step attribute.
func (i HTMLInputElement) Step() string {
	return i.El.GetAttribute("step")
}

// Pattern returns the pattern attribute.
func (i HTMLInputElement) Pattern() string {
	return i.El.GetAttribute("pattern")
}

// MaxLength returns the maxlength attribute, or -1 if unset.
func (i HTMLInputElement) MaxLength() int {
	if !i.El.HasAttribute("maxlength") {
		return -1
	}
	return attrInt(i.El, "maxlength")
}

// MinLength returns the minlength attribute, or -1 if unset.
func (i HTMLInputElement) MinLength() int {
	if !i.El.HasAttribute("minlength") {
		return -1
	}
	return attrInt(i.El, "minlength")
}

// Form returns the enclosing form element, or nil if not in a form.
func (i HTMLInputElement) Form() *dom.Element {
	return FindFormAncestor(i.El)
}

// Autofocus reports whether the input should autofocus.
func (i HTMLInputElement) Autofocus() bool {
	return attrBool(i.El, "autofocus")
}

// Multiple reports whether multiple values are allowed (file/email).
func (i HTMLInputElement) Multiple() bool {
	return attrBool(i.El, "multiple")
}

// List returns the associated <datalist> element, or nil if the input's
// "list" attribute references a non-existent datalist. Mirrors
// HTMLInputElement::list().
func (i HTMLInputElement) List() *HTMLDataListElement {
	listID := i.El.GetAttribute("list")
	if listID == "" {
		return nil
	}
	doc := i.El.OwnerDocument()
	if doc == nil {
		return nil
	}
	dl := doc.GetElementById(listID)
	if dl == nil || dl.LocalName() != "datalist" {
		return nil
	}
	dle, ok := ToDataListElement(dl)
	if !ok {
		return nil
	}
	return &dle
}

// AcceptedLabels returns the list of label strings from the datalist options
// whose value starts with the given prefix. If the input has no associated
// datalist, returns nil. This is the primary API for autocomplete UI.
func (i HTMLInputElement) AcceptedLabels(prefix string) []string {
	dl := i.List()
	if dl == nil {
		return nil
	}
	return dl.SuggestionsFor(prefix)
}

// --- Validation ---

// Validity returns the ValidityState for this input.
// Mirrors HTMLInputElement::validity().
func (i HTMLInputElement) Validity() ValidityState {
	v := ValidityState{}
	t := i.Type()
	val := i.Value()

	// required check (valueMissing)
	if i.Required() {
		switch t {
		case InputCheckbox:
			v.ValueMissing = !i.Checked()
		case InputRadio:
			// For radio, check if any radio in the same group is checked.
			v.ValueMissing = !radioGroupChecked(i)
		default:
			v.ValueMissing = val == ""
		}
	}

	// type-specific validation (typeMismatch / badInput)
	switch t {
	case InputEmail:
		if val != "" {
			emails := []string{val}
			if i.Multiple() {
				emails = strings.Split(val, ",")
				for j := range emails {
					emails[j] = strings.TrimSpace(emails[j])
				}
			}
			for _, e := range emails {
				if !emailRe.MatchString(e) {
					v.TypeMismatch = true
					break
				}
			}
		}
	case InputURL:
		if val != "" {
			if _, ok := parseURL(val); !ok {
				v.TypeMismatch = true
			}
		}
	case InputNumber, InputRange:
		if val != "" {
			if _, err := parseFloat(val); err != nil {
				v.BadInput = true
			}
		}
	case InputDate, InputMonth, InputWeek, InputTime, InputDateTimeLocal:
		if val != "" {
			if !validateDateTimeInput(t, val) {
				v.BadInput = true
			}
		}
	case InputColor:
		if val != "" {
			if !validateColor(val) {
				v.BadInput = true
			}
		}
	}

	// pattern check
	if i.Pattern() != "" && val != "" {
		pat := i.Pattern()
		if !strings.HasPrefix(pat, "^") {
			pat = "^" + pat
		}
		if !strings.HasSuffix(pat, "$") {
			pat = pat + "$"
		}
		re, err := compilePattern(pat)
		if err == nil && !re.MatchString(val) {
			v.PatternMismatch = true
		}
	}

	// maxlength / minlength
	maxLen := i.MaxLength()
	if maxLen >= 0 && len(val) > maxLen {
		v.TooLong = true
	}
	minLen := i.MinLength()
	if minLen >= 0 && len(val) < minLen && val != "" {
		v.TooShort = true
	}

	// range checks (min/max/step) for number/range/date/time
	if t == InputNumber || t == InputRange {
		if val != "" {
			num, err := parseFloat(val)
			if err == nil {
				if i.Min() != "" {
					if min, err := parseFloat(i.Min()); err == nil && num < min {
						v.RangeUnderflow = true
					}
				}
				if i.Max() != "" {
					if max, err := parseFloat(i.Max()); err == nil && num > max {
						v.RangeOverflow = true
					}
				}
				if i.Step() != "" && i.Step() != "any" {
					if step, err := parseFloat(i.Step()); err == nil && step > 0 {
						var base float64
						if i.Min() != "" {
							base, _ = parseFloat(i.Min())
						}
						remainder := num - base
						if remainder < 0 {
							remainder = -remainder
						}
						if remainder/step != float64(int64(remainder/step)) {
							v.StepMismatch = true
						}
					}
				}
			}
		}
	}

	return v
}

// WillValidate reports whether this input participates in constraint
// validation. Disabled/hidden inputs do not.
func (i HTMLInputElement) WillValidate() bool {
	t := i.Type()
	if i.Disabled() || t == InputHidden || t == InputReset || t == InputButton {
		return false
	}
	return true
}

// CheckValidity returns true if the input's value satisfies all
// constraints. Mirrors HTMLInputElement::checkValidity().
func (i HTMLInputElement) CheckValidity() bool {
	if !i.WillValidate() {
		return true
	}
	return i.Validity().Valid()
}

// SetCustomValidity sets a custom error message. If message is non-empty,
// the element is invalid with a customError.
func (i HTMLInputElement) SetCustomValidity(message string) {
	customErrorMessages[i.El] = message
}

// radioGroupChecked reports whether any radio input with the same name
// in the same form is checked.
func radioGroupChecked(input HTMLInputElement) bool {
	name := input.Name()
	if name == "" {
		return input.Checked()
	}
	form := input.Form()
	if form == nil {
		return input.Checked()
	}
	// Walk form descendants looking for radio inputs with same name.
	var found bool
	walkElements(form, func(el *dom.Element) {
		if el.LocalName() != "input" {
			return
		}
		t := strings.ToLower(el.GetAttribute("type"))
		if t != "radio" {
			return
		}
		if el.GetAttribute("name") != name {
			return
		}
		if el.HasAttribute("checked") {
			found = true
		}
	})
	return found
}

// walkElements traverses the DOM tree and calls fn for every Element.
func walkElements(node dom.Node, fn func(*dom.Element)) {
	if el, ok := node.(*dom.Element); ok {
		fn(el)
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		walkElements(c, fn)
	}
}

// parseFloat wraps strconv.ParseFloat for cleaner error handling.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// validateDateTimeInput checks if a date/time string is valid for the type.
func validateDateTimeInput(t InputType, val string) bool {
	switch t {
	case InputDate:
		_, ok := parseDate(val)
		return ok
	case InputTime:
		_, ok := parseTimeStr(val)
		return ok
	case InputMonth:
		_, _, ok := parseMonth(val)
		return ok
	case InputWeek:
		_, _, ok := parseWeek(val)
		return ok
	case InputDateTimeLocal:
		_, ok := parseDateTimeLocal(val)
		return ok
	}
	return true
}

// validateColor checks if a color value is a valid #rrggbb hex.
func validateColor(val string) bool {
	if len(val) != 7 || val[0] != '#' {
		return false
	}
	for _, c := range val[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// compilePattern wraps regexp.Compile for the pattern validation.
func compilePattern(pat string) (*regexp.Regexp, error) {
	return regexp.Compile(pat)
}

// FindFormAncestor returns the nearest <form> ancestor of el, or nil.
func FindFormAncestor(el *dom.Element) *dom.Element {
	for p := el.ParentNode(); p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "form" {
			return e
		}
	}
	return nil
}
