package html5

import (
	"strings"

	"wb-ui/dom"
)

// HTMLFormElement wraps a *dom.Element whose tag is "form" and provides
// accessors mirroring WebCore::HTMLFormElement.
type HTMLFormElement struct {
	El *dom.Element
}

// ToFormElement wraps an element as an HTMLFormElement. Returns false if el
// is nil or not a <form> element.
func ToFormElement(el *dom.Element) (HTMLFormElement, bool) {
	if el == nil || el.LocalName() != "form" {
		return HTMLFormElement{}, false
	}
	return HTMLFormElement{El: el}, true
}

// Elements returns the list of form-associated elements (input, select,
// textarea, button, output, fieldset) contained in this form, in document
// order. Mirrors HTMLFormElement::elements().
func (f HTMLFormElement) Elements() []*dom.Element {
	var out []*dom.Element
	walkElements(f.El, func(el *dom.Element) {
		switch el.LocalName() {
		case "input", "select", "textarea", "button", "output", "fieldset":
			if FindFormAncestor(el) == f.El {
				out = append(out, el)
			}
		}
	})
	return out
}

// Length returns the number of form-associated elements.
func (f HTMLFormElement) Length() int {
	return len(f.Elements())
}

// Name returns the form's name attribute.
func (f HTMLFormElement) Name() string {
	return f.El.GetAttribute("name")
}

// SetName sets the form's name attribute.
func (f HTMLFormElement) SetName(n string) {
	f.El.SetAttribute("name", n)
}

// Action returns the form's action attribute (URL).
func (f HTMLFormElement) Action() string {
	return f.El.GetAttribute("action")
}

// Method returns the form's method attribute ("get" or "post"), defaulting to "get".
func (f HTMLFormElement) Method() string {
	m := strings.ToLower(f.El.GetAttribute("method"))
	if m == "post" {
		return "post"
	}
	return "get"
}

// Enctype returns the form's enctype attribute.
func (f HTMLFormElement) Enctype() string {
	e := strings.ToLower(f.El.GetAttribute("enctype"))
	switch e {
	case "multipart/form-data", "text/plain":
		return e
	}
	return "application/x-www-form-urlencoded"
}

// NoValidate reports whether the form has the novalidate attribute.
func (f HTMLFormElement) NoValidate() bool {
	return f.El.HasAttribute("novalidate")
}

// CheckValidity runs constraint validation on all submittable elements.
func (f HTMLFormElement) CheckValidity() bool {
	for _, el := range f.Elements() {
		switch el.LocalName() {
		case "input":
			in, _ := ToInputElement(el)
			if in.WillValidate() && !in.CheckValidity() {
				return false
			}
		case "textarea":
			ta, _ := ToTextAreaElement(el)
			if ta.WillValidate() && !ta.CheckValidity() {
				return false
			}
		case "select":
			sel, _ := ToSelectElement(el)
			if sel.WillValidate() && !sel.CheckValidity() {
				return false
			}
		}
	}
	return true
}

// ReportValidity is the interactive counterpart of CheckValidity.
func (f HTMLFormElement) ReportValidity() bool {
	return f.CheckValidity()
}

// Submit dispatches a "submit" event on the form. If preventDefault is not
// called, the form data is serialized and submission proceeds. Mirrors
// HTMLFormElement::submit().
func (f HTMLFormElement) Submit() bool {
	return f.RequestSubmit(nil)
}

// RequestSubmit dispatches a "submit" event with the given submitter element
// (a submit button/input, or nil for script-triggered submission). If
// preventDefault is not called, the form data is serialized and submission
// proceeds. Mirrors HTMLFormElement::requestSubmit().
func (f HTMLFormElement) RequestSubmit(submitter *dom.Element) bool {
	ev := dom.NewSubmitEvent(submitter)
	if ev == nil {
		return false
	}
	f.El.DispatchEvent(ev)
	if ev.DefaultPrevented() {
		return false
	}
	f.doSubmit(submitter)
	return true
}

// doSubmit performs the actual form submission: collects form data, encodes
// it, and stores the submission info on the form element. In a full browser
// this would navigate to the action URL with GET or POST; here we annotate
// the element with data attributes for the embedder to consume.
func (f HTMLFormElement) doSubmit(submitter *dom.Element) {
	fd := NewDOMFormDataFromForm(f)

	// Include the submit button's name/value if present.
	if submitter != nil {
		name := submitter.GetAttribute("name")
		if name != "" {
			val := submitter.GetAttribute("value")
			fd.Append(name, val)
		}
	}

	encoded := fd.Encode()
	action := f.Action()
	method := f.Method()
	if action == "" {
		doc := f.El.OwnerDocument()
		if doc != nil {
			action = doc.URL()
		}
	}

	// Store submission info on the form element so embedders can read it.
	f.El.SetAttribute("data-submission-url", action)
	f.El.SetAttribute("data-submission-method", method)
	f.El.SetAttribute("data-submission-data", encoded)

	// Dispatch a "formdata" event so listeners can inspect the data.
	f.El.DispatchEvent(dom.NewFormDataEvent(fd))
}

// --- HTMLButtonElement ---

// HTMLButtonElement wraps a <button> element. Mirrors
// WebCore::HTMLButtonElement.
type HTMLButtonElement struct {
	El *dom.Element
}

// ToButtonElement wraps an element as an HTMLButtonElement.
func ToButtonElement(el *dom.Element) (HTMLButtonElement, bool) {
	if el == nil || el.LocalName() != "button" {
		return HTMLButtonElement{}, false
	}
	return HTMLButtonElement{El: el}, true
}

// ButtonType enumerates the button type attribute values.
type ButtonType string

const (
	ButtonSubmit ButtonType = "submit"
	ButtonReset  ButtonType = "reset"
	ButtonButton ButtonType = "button"
)

// Type returns the button's type attribute, defaulting to "submit".
func (b HTMLButtonElement) Type() ButtonType {
	t := strings.ToLower(b.El.GetAttribute("type"))
	switch ButtonType(t) {
	case ButtonSubmit, ButtonReset, ButtonButton:
		return ButtonType(t)
	}
	return ButtonSubmit
}

// SetType sets the button's type attribute.
func (b HTMLButtonElement) SetType(t ButtonType) {
	b.El.SetAttribute("type", string(t))
}

// Value returns the button's value attribute.
func (b HTMLButtonElement) Value() string { return b.El.GetAttribute("value") }

// SetValue sets the button's value attribute.
func (b HTMLButtonElement) SetValue(v string) { b.El.SetAttribute("value", v) }

// Name returns the button's name attribute.
func (b HTMLButtonElement) Name() string { return b.El.GetAttribute("name") }

// Disabled reports whether the button is disabled.
func (b HTMLButtonElement) Disabled() bool { return attrBool(b.El, "disabled") }

// SetDisabled sets the disabled state.
func (b HTMLButtonElement) SetDisabled(d bool) {
	if d {
		b.El.SetAttribute("disabled", "disabled")
	} else {
		b.El.RemoveAttribute("disabled")
	}
}

// Form returns the enclosing form element, or nil.
func (b HTMLButtonElement) Form() *dom.Element { return FindFormAncestor(b.El) }

// Autofocus reports whether the button has the autofocus attribute.
func (b HTMLButtonElement) Autofocus() bool { return attrBool(b.El, "autofocus") }

// Click performs the button's default action. For type=submit this triggers
// form submission; for type=reset it resets the form's controls to their
// default values. Mirrors HTMLButtonElement::click().
func (b HTMLButtonElement) Click() {
	switch b.Type() {
	case ButtonSubmit:
		form := b.Form()
		if form != nil {
			f, ok := ToFormElement(form)
			if ok {
				f.RequestSubmit(b.El)
			}
		}
	case ButtonReset:
		form := b.Form()
		if form != nil {
			resetForm(form)
		}
	}
}

// resetForm resets all form controls inside the given form element to their
// default attribute values (removes value/checked/selected attributes).
func resetForm(form *dom.Element) {
	for _, el := range form.GetElementsByTagName("*") {
		switch el.LocalName() {
		case "input":
			in, ok := ToInputElement(el)
			if !ok {
				continue
			}
			t := in.Type()
			if t == InputCheckbox || t == InputRadio {
				if el.HasAttribute("checked") {
					el.SetAttribute("checked", "checked")
				} else {
					el.RemoveAttribute("checked")
				}
			}
			if el.HasAttribute("value") {
				el.SetAttribute("value", el.GetAttribute("value"))
			} else {
				el.RemoveAttribute("value")
			}
		case "textarea":
			if el.HasAttribute("data-default") {
				_ = el.SetTextContent(el.GetAttribute("data-default"))
			} else {
				_ = el.SetTextContent("")
			}
		case "select":
			for _, opt := range el.GetElementsByTagName("option") {
				if opt.HasAttribute("selected") {
					opt.SetAttribute("selected", "selected")
				} else {
					opt.RemoveAttribute("selected")
				}
			}
		case "output":
			_ = el.SetTextContent("")
		}
	}
}

// --- HTMLLabelElement ---

// HTMLLabelElement wraps a <label> element. Mirrors
// WebCore::HTMLLabelElement.
type HTMLLabelElement struct {
	El *dom.Element
}

// ToLabelElement wraps an element as an HTMLLabelElement.
func ToLabelElement(el *dom.Element) (HTMLLabelElement, bool) {
	if el == nil || el.LocalName() != "label" {
		return HTMLLabelElement{}, false
	}
	return HTMLLabelElement{El: el}, true
}

// HTMLFor returns the for attribute.
func (l HTMLLabelElement) HTMLFor() string { return l.El.GetAttribute("for") }

// SetHTMLFor sets the for attribute.
func (l HTMLLabelElement) SetHTMLFor(id string) { l.El.SetAttribute("for", id) }

// Control returns the element that this label is associated with, either
// by id (for attribute) or by being a descendant labelable element.
func (l HTMLLabelElement) Control() *dom.Element {
	forID := l.HTMLFor()
	if forID != "" {
		doc := l.El.OwnerDocument()
		if doc != nil {
			return doc.GetElementById(forID)
		}
		return nil
	}
	labelable := map[string]bool{
		"button": true, "input": true, "meter": true,
		"output": true, "progress": true, "select": true, "textarea": true,
	}
	var found *dom.Element
	walkElements(l.El, func(el *dom.Element) {
		if found == nil && labelable[el.LocalName()] {
			found = el
		}
	})
	return found
}

// Click simulates a click on the label, which transfers focus to the
// associated control. For checkbox and radio inputs, it also toggles
// the checked state. This mirrors the HTML spec's label activation
// behavior.
func (l HTMLLabelElement) Click() {
	ctrl := l.Control()
	if ctrl == nil {
		return
	}
	// Focus the control.
	ctrl.SetFocused(true)

	// For checkbox/radio inputs, toggle the checked state.
	if ctrl.LocalName() == "input" {
		in, ok := ToInputElement(ctrl)
		if !ok {
			return
		}
		switch in.Type() {
		case InputCheckbox:
			in.SetChecked(!in.Checked())
		case InputRadio:
			in.SetChecked(true)
		}
	}
}

// --- HTMLFieldSetElement ---

// HTMLFieldSetElement wraps a <fieldset> element. Mirrors
// WebCore::HTMLFieldSetElement.
type HTMLFieldSetElement struct {
	El *dom.Element
}

// ToFieldSetElement wraps an element as an HTMLFieldSetElement.
func ToFieldSetElement(el *dom.Element) (HTMLFieldSetElement, bool) {
	if el == nil || el.LocalName() != "fieldset" {
		return HTMLFieldSetElement{}, false
	}
	return HTMLFieldSetElement{El: el}, true
}

// Disabled reports whether the fieldset is disabled.
func (f HTMLFieldSetElement) Disabled() bool { return attrBool(f.El, "disabled") }

// SetDisabled sets the disabled state.
func (f HTMLFieldSetElement) SetDisabled(d bool) {
	if d {
		f.El.SetAttribute("disabled", "disabled")
	} else {
		f.El.RemoveAttribute("disabled")
	}
}

// Name returns the name attribute.
func (f HTMLFieldSetElement) Name() string { return f.El.GetAttribute("name") }

// Form returns the enclosing form, or nil.
func (f HTMLFieldSetElement) Form() *dom.Element { return FindFormAncestor(f.El) }

// Elements returns the form-associated elements within this fieldset.
func (f HTMLFieldSetElement) Elements() []*dom.Element {
	var out []*dom.Element
	form := FindFormAncestor(f.El)
	walkElements(f.El, func(el *dom.Element) {
		switch el.LocalName() {
		case "input", "select", "textarea", "button", "output":
			if FindFormAncestor(el) == form {
				out = append(out, el)
			}
		}
	})
	return out
}

// --- HTMLOutputElement ---

// HTMLOutputElement wraps an <output> element. Mirrors
// WebCore::HTMLOutputElement.
type HTMLOutputElement struct {
	El *dom.Element
}

// ToOutputElement wraps an element as an HTMLOutputElement.
func ToOutputElement(el *dom.Element) (HTMLOutputElement, bool) {
	if el == nil || el.LocalName() != "output" {
		return HTMLOutputElement{}, false
	}
	return HTMLOutputElement{El: el}, true
}

// Value returns the output's text content.
func (o HTMLOutputElement) Value() string { return textContent(o.El) }

// SetValue sets the output's text content.
func (o HTMLOutputElement) SetValue(v string) { _ = o.El.SetTextContent(v) }

// DefaultValue returns the output's default value (text content).
func (o HTMLOutputElement) DefaultValue() string { return textContent(o.El) }

// SetDefaultValue sets the output's default value.
func (o HTMLOutputElement) SetDefaultValue(v string) { _ = o.El.SetTextContent(v) }

// HTMLFor returns the for attribute.
func (o HTMLOutputElement) HTMLFor() string { return o.El.GetAttribute("for") }

// SetHTMLFor sets the for attribute.
func (o HTMLOutputElement) SetHTMLFor(v string) { o.El.SetAttribute("for", v) }

// Name returns the name attribute.
func (o HTMLOutputElement) Name() string { return o.El.GetAttribute("name") }

// Form returns the enclosing form, or nil.
func (o HTMLOutputElement) Form() *dom.Element { return FindFormAncestor(o.El) }

// --- HTMLLegendElement ---

// HTMLLegendElement wraps a <legend> element. Mirrors
// WebCore::HTMLLegendElement.
type HTMLLegendElement struct {
	El *dom.Element
}

// ToLegendElement wraps an element as an HTMLLegendElement.
func ToLegendElement(el *dom.Element) (HTMLLegendElement, bool) {
	if el == nil || el.LocalName() != "legend" {
		return HTMLLegendElement{}, false
	}
	return HTMLLegendElement{El: el}, true
}

// Form returns the form associated with this legend (the form of the
// fieldset that contains this legend).
func (l HTMLLegendElement) Form() *dom.Element {
	for p := l.El.ParentNode(); p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "fieldset" {
			return FindFormAncestor(e)
		}
	}
	return nil
}
