package html5

import (
	"testing"

	"wb-ui/dom"
)

// newDialog creates a <dialog> element in a fresh document and returns it
// wrapped as an HTMLDialogElement.
func newDialog(t *testing.T) HTMLDialogElement {
	t.Helper()
	doc := dom.NewDocument()
	el := doc.CreateElement("dialog")
	d, ok := ToDialogElement(el)
	if !ok {
		t.Fatal("ToDialogElement returned false for <dialog>")
	}
	return d
}

func TestDialog_DefaultClosed(t *testing.T) {
	d := newDialog(t)
	if d.Open() {
		t.Error("Open() = true, want false")
	}
}

func TestDialog_Show(t *testing.T) {
	d := newDialog(t)
	d.Show()
	if !d.Open() {
		t.Error("Open() = false after Show()")
	}
	if !d.El.HasAttribute("open") {
		t.Error("open attribute not set after Show()")
	}
	if d.IsModal() {
		t.Error("IsModal() = true after Show(), want false")
	}
}

func TestDialog_ShowModal(t *testing.T) {
	d := newDialog(t)
	d.ShowModal()
	if !d.Open() {
		t.Error("Open() = false after ShowModal()")
	}
	if !d.El.HasAttribute("open") {
		t.Error("open attribute not set after ShowModal()")
	}
	if !d.IsModal() {
		t.Error("IsModal() = false after ShowModal(), want true")
	}
}

func TestDialog_Close(t *testing.T) {
	d := newDialog(t)
	d.Show()
	d.Close()
	if d.Open() {
		t.Error("Open() = true after Close()")
	}
	if d.El.HasAttribute("open") {
		t.Error("open attribute still present after Close()")
	}
}

func TestDialog_CloseModalClearsFlag(t *testing.T) {
	d := newDialog(t)
	d.ShowModal()
	if !d.IsModal() {
		t.Fatal("IsModal() = false, expected true")
	}
	d.Close()
	if d.IsModal() {
		t.Error("IsModal() = true after Close(), want false")
	}
}

func TestDialog_CloseWithReturnValue(t *testing.T) {
	d := newDialog(t)
	d.Show()
	d.Close("ok")
	if d.ReturnValue() != "ok" {
		t.Errorf("ReturnValue() = %q, want 'ok'", d.ReturnValue())
	}
}

func TestDialog_ReturnValueDefault(t *testing.T) {
	d := newDialog(t)
	if d.ReturnValue() != "" {
		t.Errorf("ReturnValue() = %q, want ''", d.ReturnValue())
	}
}

func TestDialog_SetReturnValue(t *testing.T) {
	d := newDialog(t)
	d.SetReturnValue("cancel")
	if d.ReturnValue() != "cancel" {
		t.Errorf("ReturnValue() = %q, want 'cancel'", d.ReturnValue())
	}
}

func TestToDialogElement_RejectsNonDialog(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	if _, ok := ToDialogElement(el); ok {
		t.Fatal("ToDialogElement should return false for <div>")
	}
}

func TestToDialogElement_AcceptsDialog(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("dialog")
	if _, ok := ToDialogElement(el); !ok {
		t.Fatal("ToDialogElement should return true for <dialog>")
	}
}
