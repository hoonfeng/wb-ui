package html5

import "wb-ui/engine/dom"

// The modal flag lives on the DOM node itself (dom.Element.SetModalState), not
// in a side map: the CSS selector engine needs it for the :modal pseudo-class,
// and css cannot depend on html5.

// --- HTMLDialogElement ---

// HTMLDialogElement wraps a <dialog> element. Mirrors
// WebCore::HTMLDialogElement.
type HTMLDialogElement struct {
	El *dom.Element
}

// ToDialogElement wraps an element as an HTMLDialogElement.
func ToDialogElement(el *dom.Element) (HTMLDialogElement, bool) {
	if el == nil || el.LocalName() != "dialog" {
		return HTMLDialogElement{}, false
	}
	return HTMLDialogElement{El: el}, true
}

// Open reports whether the dialog is currently open (the "open" attribute
// is present). Mirrors HTMLDialogElement::open().
func (d HTMLDialogElement) Open() bool {
	return d.El.HasAttribute("open")
}

// Show opens the dialog as non-modal. It adds the "open" attribute and
// makes the dialog visible. Mirrors HTMLDialogElement::show().
func (d HTMLDialogElement) Show() {
	d.El.SetAttribute("open", "open")
	d.El.SetModalState(false)
}

// ShowModal opens the dialog as a modal overlay. It adds the "open"
// attribute and marks the dialog as modal (which enables the ::backdrop
// pseudo-element in rendering). Mirrors HTMLDialogElement::showModal().
func (d HTMLDialogElement) ShowModal() {
	d.El.SetAttribute("open", "open")
	d.El.SetModalState(true)
}

// IsModal reports whether this dialog was opened via ShowModal rather than
// Show. Mirrors HTMLDialogElement::isModal().
func (d HTMLDialogElement) IsModal() bool {
	return d.Open() && d.El.ModalState()
}

// Close closes the dialog. If returnValue is provided, it is stored as the
// dialog's return value. The "open" attribute is removed and the modal
// flag is cleared. Mirrors HTMLDialogElement::close().
func (d HTMLDialogElement) Close(returnValue ...string) {
	if len(returnValue) > 0 {
		d.SetReturnValue(returnValue[0])
	}
	d.El.RemoveAttribute("open")
	d.El.SetModalState(false)
	// Dispatch a "close" event on the element
	evt := dom.NewEvent("close", false, false, false)
	_ = d.El.DispatchEvent(evt)
}

// ReturnValue returns the dialog's return value (the value passed to close(),
// or "" if not set). Mirrors HTMLDialogElement::returnValue().
func (d HTMLDialogElement) ReturnValue() string {
	return d.El.GetAttribute("data-returnvalue")
}

// SetReturnValue sets the dialog's return value.
func (d HTMLDialogElement) SetReturnValue(v string) {
	d.El.SetAttribute("data-returnvalue", v)
}
