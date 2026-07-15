package widgets

import (
	"testing"

	"wb-ui/dom"
)

func TestProcessMarkdownElements_Basic(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	md := doc.CreateElement("wb-markdown")
	text := dom.NewText(doc, "# Hello\n\nThis is **bold**.")
	md.AppendChild(text)
	body.AppendChild(md)
	html.AppendChild(body)
	doc.AppendChild(html)

	ProcessMarkdownElements(doc)

	// After processing, <wb-markdown> should have <h1> and <p> children.
	children := md.ChildNodes()
	if len(children) < 2 {
		t.Fatalf("expected at least 2 children, got %d", len(children))
	}
	// First child should be <h1>.
	h1, ok := children[0].(*dom.Element)
	if !ok || h1.LocalName() != "h1" {
		t.Errorf("expected <h1> as first child, got %T", children[0])
	}
}

func TestProcessMarkdownElements_GFM(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	md := doc.CreateElement("wb-markdown")
	text := dom.NewText(doc, "| H1 | H2 |\n| --- | --- |\n| a | b |")
	md.AppendChild(text)
	body.AppendChild(md)
	doc.AppendChild(body)

	ProcessMarkdownElements(doc)

	// Should contain a <table>.
	var foundTable bool
	walkElements(md, func(el *dom.Element) {
		if el.LocalName() == "table" {
			foundTable = true
		}
	})
	if !foundTable {
		t.Errorf("expected a <table> in markdown output")
	}
}

func TestProcessMarkdownElements_NoGFM(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	md := doc.CreateElement("wb-markdown")
	md.SetAttribute("data-gfm", "false")
	text := dom.NewText(doc, "| H1 | H2 |\n| --- | --- |\n| a | b |")
	md.AppendChild(text)
	body.AppendChild(md)
	doc.AppendChild(body)

	ProcessMarkdownElements(doc)

	// Without GFM, pipe tables should not be parsed as a table.
	var foundTable bool
	walkElements(md, func(el *dom.Element) {
		if el.LocalName() == "table" {
			foundTable = true
		}
	})
	if foundTable {
		t.Errorf("should not have a <table> without GFM")
	}
}

func TestProcessMarkdownElements_Empty(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	md := doc.CreateElement("wb-markdown")
	body.AppendChild(md)
	doc.AppendChild(body)

	// Empty content: should not crash, children remain empty.
	ProcessMarkdownElements(doc)
	if md.FirstChild() != nil {
		t.Errorf("expected no children for empty markdown")
	}
}

func TestProcessMarkdownElements_NestedElements(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	div := doc.CreateElement("div")
	md := doc.CreateElement("wb-markdown")
	text := dom.NewText(doc, "- item 1\n- item 2")
	md.AppendChild(text)
	div.AppendChild(md)
	body.AppendChild(div)
	html.AppendChild(body)
	doc.AppendChild(html)

	ProcessMarkdownElements(doc)

	// Should find a <ul> inside the <wb-markdown>.
	var foundUL bool
	walkElements(md, func(el *dom.Element) {
		if el.LocalName() == "ul" {
			foundUL = true
		}
	})
	if !foundUL {
		t.Errorf("expected a <ul> in markdown output")
	}
}

func TestEditorRegistry_GetOrCreate(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	text := dom.NewText(doc, "package main\n\nfunc main() {}\n")
	el.AppendChild(text)

	r := NewEditorRegistry()
	v := r.GetOrCreate(el)
	if v == nil {
		t.Fatal("expected non-nil EditorView")
	}

	// Second call should return the same instance.
	v2 := r.GetOrCreate(el)
	if v != v2 {
		t.Errorf("expected same EditorView instance")
	}
}

func TestEditorRegistry_GetNotCreated(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	r := NewEditorRegistry()
	if v := r.Get(el); v != nil {
		t.Errorf("expected nil for un-created editor")
	}
}

func TestEditorRegistry_Remove(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	r := NewEditorRegistry()
	r.GetOrCreate(el)
	r.Remove(el)
	if v := r.Get(el); v != nil {
		t.Errorf("expected nil after remove")
	}
}

func TestEditorRegistry_Language(t *testing.T) {
	doc := dom.NewDocument()

	tests := []struct{ lang string }{
		{"go"},
		{"javascript"},
		{"js"},
		{"markdown"},
		{"md"},
		{""},
	}
	for _, tt := range tests {
		el := doc.CreateElement("wb-editor")
		if tt.lang != "" {
			el.SetAttribute("language", tt.lang)
		}
		r := NewEditorRegistry()
		v := r.GetOrCreate(el)
		if v == nil {
			t.Errorf("lang=%q: expected non-nil view", tt.lang)
		}
	}
}

func TestIsEditorElement(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	if !IsEditorElement(el) {
		t.Errorf("expected wb-editor to be recognized")
	}
	div := doc.CreateElement("div")
	if IsEditorElement(div) {
		t.Errorf("div should not be recognized as editor")
	}
}

func TestIsMarkdownElement(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-markdown")
	if !IsMarkdownElement(el) {
		t.Errorf("expected wb-markdown to be recognized")
	}
	div := doc.CreateElement("div")
	if IsMarkdownElement(div) {
		t.Errorf("div should not be recognized as markdown")
	}
}

func TestEditorRegistry_All(t *testing.T) {
	doc := dom.NewDocument()
	r := NewEditorRegistry()
	el1 := doc.CreateElement("wb-editor")
	el2 := doc.CreateElement("wb-editor")
	r.GetOrCreate(el1)
	r.GetOrCreate(el2)
	all := r.All()
	if len(all) != 2 {
		t.Errorf("expected 2 editors in All(), got %d", len(all))
	}
}

func TestEditorRegistry_AllEmpty(t *testing.T) {
	r := NewEditorRegistry()
	all := r.All()
	if len(all) != 0 {
		t.Errorf("expected empty All(), got %d", len(all))
	}
}

func TestEditorRegistry_GetOrCreateConcurrent(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	r := NewEditorRegistry()
	// Sequential calls from multiple goroutines
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			v := r.GetOrCreate(el)
			if v == nil {
				t.Errorf("expected non-nil view")
			}
			done <- true
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	// Should have exactly one instance
	all := r.All()
	if len(all) != 1 {
		t.Errorf("expected 1 editor after concurrent GetOrCreate, got %d", len(all))
	}
}

func TestEditorRegistry_NilElement(t *testing.T) {
	r := NewEditorRegistry()
	if v := r.Get(nil); v != nil {
		t.Errorf("expected nil for nil element")
	}
}

func TestIsEditorElement_Nil(t *testing.T) {
	if IsEditorElement(nil) {
		t.Errorf("nil should not be an editor element")
	}
}

func TestIsMarkdownElement_Nil(t *testing.T) {
	if IsMarkdownElement(nil) {
		t.Errorf("nil should not be a markdown element")
	}
}

func TestProcessMarkdownElements_NilDoc(t *testing.T) {
	// Should not panic
	ProcessMarkdownElements(nil)
}

func TestProcessMarkdownElements_LanguageAttribute(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	md := doc.CreateElement("wb-markdown")
	md.SetAttribute("lang", "en")
	text := dom.NewText(doc, "# Title")
	md.AppendChild(text)
	body.AppendChild(md)
	doc.AppendChild(body)

	ProcessMarkdownElements(doc)

	// After processing, should have an <h1> child.
	h1 := md.FirstChild()
	if h1 == nil {
		t.Fatalf("expected at least one child after processing")
	}
	if el, ok := h1.(*dom.Element); ok && el.LocalName() != "h1" {
		t.Errorf("expected <h1>, got <%s>", el.LocalName())
	}
}

func TestEditorRegistry_Recreate(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("wb-editor")
	r := NewEditorRegistry()
	v1 := r.GetOrCreate(el)
	if v1 == nil {
		t.Fatal("expected non-nil view")
	}
	r.Remove(el)
	v2 := r.GetOrCreate(el)
	if v2 == nil {
		t.Fatal("expected non-nil view after recreate")
	}
	if v1 == v2 {
		t.Errorf("expected new instance after remove+recreate")
	}
}
