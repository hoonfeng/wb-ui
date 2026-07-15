package rendering

import (
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

// TestIntegration_WBMarkdownInHTML loads an HTML document containing
// <wb-markdown> and verifies the render tree builder transforms it.
func TestIntegration_WBMarkdownInHTML(t *testing.T) {
	htmlSrc := `<!DOCTYPE html>
<html>
<head><style>wb-markdown { display: block; }</style></head>
<body>
<wb-markdown>
# Title

Some **bold** text and *italic*.

- item 1
- item 2
</wb-markdown>
</body>
</html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	// Build the render tree — this triggers ProcessMarkdownElements.
	resolver := style.NewResolver()
	css := css.NewCSSStyleSheet()
	resolver.AddStyleSheet(css)
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("expected non-nil RenderView")
	}

	// Find the <wb-markdown> element and verify it has been transformed.
	var mdEl *dom.Element
	walkElementsForTest(doc, func(el *dom.Element) {
		if el.LocalName() == "wb-markdown" {
			mdEl = el
		}
	})
	if mdEl == nil {
		t.Fatal("no <wb-markdown> element found")
	}

	// After processing, it should contain <h1>, <p>, <ul>.
	tags := childTagsForTest(mdEl)
	var foundH1, foundP, foundUL bool
	for _, tag := range tags {
		switch tag {
		case "h1":
			foundH1 = true
		case "p":
			foundP = true
		case "ul":
			foundUL = true
		}
	}
	if !foundH1 {
		t.Errorf("expected <h1> in markdown output, got tags: %v", tags)
	}
	if !foundP {
		t.Errorf("expected <p> in markdown output, got tags: %v", tags)
	}
	if !foundUL {
		t.Errorf("expected <ul> in markdown output, got tags: %v", tags)
	}
}

// TestIntegration_WBEditorInHTML loads an HTML document containing
// <wb-editor> and verifies the editor view is created.
func TestIntegration_WBEditorInHTML(t *testing.T) {
	htmlSrc := `<!DOCTYPE html>
<html>
<body>
<wb-editor language="go" show-line-numbers="true">
package main

func main() {
    println("hello")
}
</wb-editor>
</body>
</html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	resolver := style.NewResolver()
	css := css.NewCSSStyleSheet()
	resolver.AddStyleSheet(css)
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("expected non-nil RenderView")
	}

	// Find the <wb-editor> element.
	var editorEl *dom.Element
	walkElementsForTest(doc, func(el *dom.Element) {
		if el.LocalName() == "wb-editor" {
			editorEl = el
		}
	})
	if editorEl == nil {
		t.Fatal("no <wb-editor> element found")
	}

	// The editor registry should create a view on first access.
	registry := view.EditorRegistry()
	v := registry.GetOrCreate(editorEl)
	if v == nil {
		t.Fatal("expected non-nil EditorView")
	}

	// Verify the editor state contains the code text.
	editorDoc := v.State().Doc
	if editorDoc.Length() == 0 {
		t.Errorf("expected non-empty document in editor")
	}
	if !strings.Contains(editorDoc.String(), "package main") {
		t.Errorf("expected 'package main' in editor doc, got: %s", editorDoc.String())
	}
}

// TestIntegration_BothElements loads HTML with both <wb-editor> and
// <wb-markdown> and verifies they coexist.
func TestIntegration_BothElements(t *testing.T) {
	htmlSrc := `<!DOCTYPE html>
<html>
<body>
<wb-markdown>
# Code Example

Here's a Go snippet:
</wb-markdown>
<wb-editor language="go">
package main
func main() {}
</wb-editor>
</body>
</html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	resolver := style.NewResolver()
	css := css.NewCSSStyleSheet()
	resolver.AddStyleSheet(css)
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("expected non-nil RenderView")
	}

	// Find both elements.
	var mdCount, editorCount int
	walkElementsForTest(doc, func(el *dom.Element) {
		switch el.LocalName() {
		case "wb-markdown":
			mdCount++
		case "wb-editor":
			editorCount++
		}
	})
	if mdCount != 1 {
		t.Errorf("expected 1 <wb-markdown>, got %d", mdCount)
	}
	if editorCount != 1 {
		t.Errorf("expected 1 <wb-editor>, got %d", editorCount)
	}

	// Verify <wb-markdown> was transformed.
	var mdEl *dom.Element
	walkElementsForTest(doc, func(el *dom.Element) {
		if el.LocalName() == "wb-markdown" {
			mdEl = el
		}
	})
	if mdEl != nil {
		tags := childTagsForTest(mdEl)
		var foundH1 bool
		for _, tag := range tags {
			if tag == "h1" {
				foundH1 = true
			}
		}
		if !foundH1 {
			t.Errorf("expected <h1> in markdown output, got: %v", tags)
		}
	}
}

// TestIntegration_MultipleEditors verifies multiple <wb-editor> elements
// each get independent editor views.
func TestIntegration_MultipleEditors(t *testing.T) {
	htmlSrc := `<!DOCTYPE html>
<html>
<body>
<wb-editor language="go">package a</wb-editor>
<wb-editor language="javascript">var x = 1;</wb-editor>
<wb-editor>plain text</wb-editor>
</body>
</html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("expected non-nil RenderView")
	}

	// Collect all <wb-editor> elements.
	var editors []*dom.Element
	walkElementsForTest(doc, func(el *dom.Element) {
		if el.LocalName() == "wb-editor" {
			editors = append(editors, el)
		}
	})
	if len(editors) != 3 {
		t.Fatalf("expected 3 <wb-editor> elements, got %d", len(editors))
	}

	registry := view.EditorRegistry()
	for i, el := range editors {
		v := registry.GetOrCreate(el)
		if v == nil {
			t.Errorf("editor[%d]: expected non-nil view", i)
			continue
		}
	}
}

// TestIntegration_WBMarkdownWithGFM verifies GFM features work inside
// <wb-markdown>.
func TestIntegration_WBMarkdownWithGFM(t *testing.T) {
	htmlSrc := `<!DOCTYPE html>
<html>
<body>
<wb-markdown>
| Name | Age |
| --- | --- |
| Alice | 30 |

- [x] done task
- [ ] todo task
</wb-markdown>
</body>
</html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	builder.Build(doc)

	// Find the <wb-markdown> element.
	var mdEl *dom.Element
	walkElementsForTest(doc, func(el *dom.Element) {
		if el.LocalName() == "wb-markdown" {
			mdEl = el
		}
	})
	if mdEl == nil {
		t.Fatal("no <wb-markdown> element found")
	}

	// Should contain <table> and <input> (task list checkbox).
	var foundTable, foundCheckbox bool
	walkElementsForTest(mdEl, func(el *dom.Element) {
		if el.LocalName() == "table" {
			foundTable = true
		}
		if el.LocalName() == "input" {
			foundCheckbox = true
		}
	})
	if !foundTable {
		t.Errorf("expected a <table> in GFM markdown output")
	}
	if !foundCheckbox {
		t.Errorf("expected a checkbox <input> in GFM task list")
	}
}

// walkElementsForTest traverses the DOM tree and calls fn for every Element.
func walkElementsForTest(node dom.Node, fn func(*dom.Element)) {
	if el, ok := node.(*dom.Element); ok {
		fn(el)
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		walkElementsForTest(c, fn)
	}
}

// childTagsForTest returns the local names of child elements of n.
func childTagsForTest(n dom.Node) []string {
	var out []string
	if n == nil {
		return out
	}
	for _, c := range n.ChildNodes() {
		if el, ok := c.(*dom.Element); ok {
			out = append(out, el.LocalName())
		}
	}
	return out
}
