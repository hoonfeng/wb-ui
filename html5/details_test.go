package html5

import (
	"testing"

	"wb-ui/dom"
)

// newDetails creates a <details> element in a fresh document and returns it
// wrapped as an HTMLDetailsElement.
func newDetails(t *testing.T) HTMLDetailsElement {
	t.Helper()
	doc := dom.NewDocument()
	el := doc.CreateElement("details")
	d, ok := ToDetailsElement(el)
	if !ok {
		t.Fatal("ToDetailsElement returned false for <details>")
	}
	return d
}

func TestDetails_DefaultClosed(t *testing.T) {
	d := newDetails(t)
	if d.Open() {
		t.Error("Open() = true, want false (default closed)")
	}
}

func TestDetails_SetOpen(t *testing.T) {
	d := newDetails(t)
	d.SetOpen(true)
	if !d.Open() {
		t.Error("Open() = false after SetOpen(true)")
	}
	// Verify the attribute was set
	if !d.El.HasAttribute("open") {
		t.Error("open attribute not set after SetOpen(true)")
	}
	// Now close it
	d.SetOpen(false)
	if d.Open() {
		t.Error("Open() = true after SetOpen(false)")
	}
	if d.El.HasAttribute("open") {
		t.Error("open attribute still present after SetOpen(false)")
	}
}

func TestDetails_Toggle(t *testing.T) {
	d := newDetails(t)
	if d.Toggle() != true {
		t.Error("Toggle() should return true when toggling from closed")
	}
	if !d.Open() {
		t.Error("Open() = false after Toggle()")
	}
	if d.Toggle() != false {
		t.Error("Toggle() should return false when toggling from open")
	}
	if d.Open() {
		t.Error("Open() = true after second Toggle()")
	}
}

func TestDetails_Summary(t *testing.T) {
	doc := dom.NewDocument()
	details := doc.CreateElement("details")
	summary := doc.CreateElement("summary")
	summary.SetAttribute("class", "title")
	details.AppendChild(summary)

	// Add a non-summary element
	p := doc.CreateElement("p")
	details.AppendChild(p)

	d, ok := ToDetailsElement(details)
	if !ok {
		t.Fatal("ToDetailsElement failed")
	}

	got := d.Summary()
	if got == nil {
		t.Fatal("Summary() = nil, want non-nil")
	}
	if got.GetAttribute("class") != "title" {
		t.Errorf("Summary class = %q, want 'title'", got.GetAttribute("class"))
	}
}

func TestDetails_SummaryNone(t *testing.T) {
	doc := dom.NewDocument()
	details := doc.CreateElement("details")
	p := doc.CreateElement("p")
	details.AppendChild(p)

	d, ok := ToDetailsElement(details)
	if !ok {
		t.Fatal("ToDetailsElement failed")
	}
	if s := d.Summary(); s != nil {
		t.Fatal("Summary() = non-nil, want nil when no <summary> present")
	}
}

func TestToDetailsElement_RejectsNonDetails(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	if _, ok := ToDetailsElement(el); ok {
		t.Fatal("ToDetailsElement should return false for <div>")
	}
}

func TestToSummaryElement(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("summary")
	s, ok := ToSummaryElement(el)
	if !ok {
		t.Fatal("ToSummaryElement returned false for <summary>")
	}
	if s.El != el {
		t.Fatal("ToSummaryElement wrapped wrong element")
	}
}

func TestToSummaryElement_RejectsNonSummary(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	if _, ok := ToSummaryElement(el); ok {
		t.Fatal("ToSummaryElement should return false for <div>")
	}
}
