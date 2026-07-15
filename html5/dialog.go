package html5

import "wb-ui/dom"

// dialogState tracks per-element modal state. Since <dialog> elements are
// plain *dom.Element instances (wrapped by HTMLDialogElement), we keep the
// modal flag in a side map rather than on the DOM node itself.
var dialogState = map[*dom.Element]struct{}{}

// markDialogModal records that the dialog was opened via showModal().
func markDialogModal(el *dom.Element) {
	dialogState[el] = struct{}{}
}

// clearDialogModal removes the modal flag.
func clearDialogModal(el *dom.Element) {
	delete(dialogState, el)
}

// isDialogModal reports whether the dialog was opened via showModal().
func isDialogModal(el *dom.Element) bool {
	_, ok := dialogState[el]
	return ok
}

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
	clearDialogModal(d.El)
}

// ShowModal opens the dialog as a modal overlay. It adds the "open"
// attribute and marks the dialog as modal (which enables the ::backdrop
// pseudo-element in rendering). Mirrors HTMLDialogElement::showModal().
func (d HTMLDialogElement) ShowModal() {
	d.El.SetAttribute("open", "open")
	markDialogModal(d.El)
}

// IsModal reports whether this dialog was opened via ShowModal rather than
// Show. Mirrors HTMLDialogElement::isModal().
func (d HTMLDialogElement) IsModal() bool {
	return d.Open() && isDialogModal(d.El)
}

// Close closes the dialog. If returnValue is provided, it is stored as the
// dialog's return value. The "open" attribute is removed and the modal
// flag is cleared. Mirrors HTMLDialogElement::close().
func (d HTMLDialogElement) Close(returnValue ...string) {
	if len(returnValue) > 0 {
		d.SetReturnValue(returnValue[0])
	}
	d.El.RemoveAttribute("open")
	clearDialogModal(d.El)
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
