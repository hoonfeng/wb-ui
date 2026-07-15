package html5

import (
	"strings"

	"wb-ui/dom"
)

// FormDataEntry is a single name/value pair in a FormData collection.
// Mirrors the entry concept in the FormData WebIDL interface.
type FormDataEntry struct {
	Name  string
	Value string
}

// DOMFormData mirrors the WebIDL FormData interface. It collects name/value
// pairs from form-associated elements and is used for form submission. File
// values are not modeled (this port stores file paths as strings).
type DOMFormData struct {
	entries []FormDataEntry
}

// NewDOMFormData creates an empty FormData.
func NewDOMFormData() *DOMFormData {
	return &DOMFormData{}
}

// NewDOMFormDataFromForm creates a FormData and populates it from the
// submittable elements of the given form, mirroring the
// `new FormData(form)` constructor. Disabled controls and unchecked
// checkboxes/radios are skipped; select controls contribute each selected
// option.
func NewDOMFormDataFromForm(form HTMLFormElement) *DOMFormData {
	fd := NewDOMFormData()
	fd.populateFromForm(form)
	return fd
}

// populateFromForm walks the form's submittable elements and appends their
// name/value pairs following the HTML form submission algorithm.
func (fd *DOMFormData) populateFromForm(form HTMLFormElement) {
	for _, el := range form.Elements() {
		name := el.GetAttribute("name")
		if name == "" {
			continue
		}
		switch el.LocalName() {
		case "input":
			in, _ := ToInputElement(el)
			if in.Disabled() {
				continue
			}
			t := in.Type()
			// Skip submit/reset/button/image (image submits coordinates, not
			// modeled here).
			if t == InputSubmit || t == InputReset || t == InputButton || t == InputImage {
				continue
			}
			if t == InputCheckbox || t == InputRadio {
				if !in.Checked() {
					continue
				}
				v := el.GetAttribute("value")
				if v == "" {
					v = "on"
				}
				fd.Append(name, v)
				continue
			}
			// file/hidden/text/etc.: include value.
			fd.Append(name, in.Value())
		case "textarea":
			ta, _ := ToTextAreaElement(el)
			if ta.Disabled() {
				continue
			}
			fd.Append(name, ta.Value())
		case "select":
			sel, _ := ToSelectElement(el)
			if sel.Disabled() {
				continue
			}
			for _, opt := range sel.Options() {
				if !opt.HasAttribute("selected") {
					continue
				}
				v := opt.GetAttribute("value")
				if v == "" {
					v = strings.TrimSpace(textContent(opt))
				}
				fd.Append(name, v)
			}
		case "output":
			o, _ := ToOutputElement(el)
			fd.Append(name, o.Value())
		}
	}
}

// Append adds a name/value pair to the FormData.
// Mirrors FormData::append().
func (fd *DOMFormData) Append(name, value string) {
	fd.entries = append(fd.entries, FormDataEntry{Name: name, Value: value})
}

// Delete removes all entries with the given name.
// Mirrors FormData::delete().
func (fd *DOMFormData) Delete(name string) {
	out := fd.entries[:0]
	for _, e := range fd.entries {
		if e.Name != name {
			out = append(out, e)
		}
	}
	fd.entries = out
}

// Get returns the first value for the given name, or "" if no entry exists.
// Mirrors FormData::get() (which returns null in JS; here we return "").
func (fd *DOMFormData) Get(name string) string {
	for _, e := range fd.entries {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// Has reports whether any entry with the given name exists.
// Mirrors FormData::has().
func (fd *DOMFormData) Has(name string) bool {
	for _, e := range fd.entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

// Set replaces all entries with the given name with a single new entry.
// Mirrors FormData::set().
func (fd *DOMFormData) Set(name, value string) {
	replaced := false
	out := make([]FormDataEntry, 0, len(fd.entries))
	for _, e := range fd.entries {
		if e.Name == name {
			if !replaced {
				out = append(out, FormDataEntry{Name: name, Value: value})
				replaced = true
			}
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, FormDataEntry{Name: name, Value: value})
	}
	fd.entries = out
}

// Entries returns all name/value pairs in insertion order.
// Mirrors FormData::entries().
func (fd *DOMFormData) Entries() []FormDataEntry {
	out := make([]FormDataEntry, len(fd.entries))
	copy(out, fd.entries)
	return out
}

// All returns all values for the given name in insertion order.
// Mirrors FormData::getAll().
func (fd *DOMFormData) All(name string) []string {
	var out []string
	for _, e := range fd.entries {
		if e.Name == name {
			out = append(out, e.Value)
		}
	}
	return out
}

// Keys returns the list of names in insertion order (with duplicates for
// repeated keys). Mirrors FormData::keys().
func (fd *DOMFormData) Keys() []string {
	out := make([]string, len(fd.entries))
	for i, e := range fd.entries {
		out[i] = e.Name
	}
	return out
}

// Values returns the list of values in insertion order.
// Mirrors FormData::values().
func (fd *DOMFormData) Values() []string {
	out := make([]string, len(fd.entries))
	for i, e := range fd.entries {
		out[i] = e.Value
	}
	return out
}

// Len returns the number of entries.
func (fd *DOMFormData) Len() int {
	return len(fd.entries)
}

// Encode serializes the FormData as application/x-www-form-urlencoded
// (key=value&key=value pairs, URL-encoded). Mirrors the
// application/x-www-form-urlencoded encoder.
func (fd *DOMFormData) Encode() string {
	var sb strings.Builder
	for i, e := range fd.entries {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(urlEncode(e.Name))
		sb.WriteByte('=')
		sb.WriteString(urlEncode(e.Value))
	}
	return sb.String()
}

// urlEncode performs application/x-www-form-urlencoded encoding:
// spaces become '+', and bytes not in [A-Za-z0-9-_.~] are percent-encoded.
func urlEncode(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			sb.WriteByte('+')
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '~':
			sb.WriteRune(r)
		default:
			// Percent-encode the UTF-8 bytes of this rune.
			for _, b := range []byte(string(r)) {
				const hex = "0123456789ABCDEF"
				sb.WriteByte('%')
				sb.WriteByte(hex[b>>4])
				sb.WriteByte(hex[b&0x0F])
			}
		}
	}
	return sb.String()
}

// FormSubmit constructs a DOMFormData from a form element and returns its
// application/x-www-form-urlencoded encoding. Convenience helper mirroring
// form submission.
func FormSubmit(form HTMLFormElement) string {
	return NewDOMFormDataFromForm(form).Encode()
}

// SerializeForm is a convenience that wraps an element as a form (if it is a
// form) and returns the urlencoded submission, or "" if not a form.
func SerializeForm(el *dom.Element) string {
	form, ok := ToFormElement(el)
	if !ok {
		return ""
	}
	return FormSubmit(form)
}
