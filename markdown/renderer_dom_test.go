package markdown

import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/editor"
)

// domText extracts the plain-text content of a DOM subtree (for assertion).
func domText(n dom.Node) string {
	switch v := n.(type) {
	case *dom.Text:
		return v.Data()
	case *dom.Element:
		var sb strings.Builder
		for _, c := range v.ChildNodes() {
			sb.WriteString(domText(c))
		}
		return sb.String()
	case *dom.DocumentFragment:
		var sb strings.Builder
		for _, c := range v.ChildNodes() {
			sb.WriteString(domText(c))
		}
		return sb.String()
	}
	return ""
}

// childTags returns the local names of child elements of n.
func childTags(n dom.Node) []string {
	var out []string
	for _, c := range n.ChildNodes() {
		if el, ok := c.(*dom.Element); ok {
			out = append(out, el.LocalName())
		}
	}
	return out
}

// firstElement returns the first Element child of n, skipping Text nodes.
func firstElement(n dom.Node) *dom.Element {
	for _, c := range n.ChildNodes() {
		if el, ok := c.(*dom.Element); ok {
			return el
		}
	}
	return nil
}

func TestRenderDOM_Paragraph(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("Hello world", doc)
	children := frag.ChildNodes()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	el, ok := children[0].(*dom.Element)
	if !ok {
		t.Fatalf("expected element, got %T", children[0])
	}
	if el.LocalName() != "p" {
		t.Errorf("expected <p>, got <%s>", el.LocalName())
	}
	if domText(el) != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", domText(el))
	}
}

func TestRenderDOM_Heading(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("## Title", doc)
	children := frag.ChildNodes()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	el := children[0].(*dom.Element)
	if el.LocalName() != "h2" {
		t.Errorf("expected <h2>, got <%s>", el.LocalName())
	}
	if domText(el) != "Title" {
		t.Errorf("expected 'Title', got %q", domText(el))
	}
}

func TestRenderDOM_Emphasis(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("*italic*", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	if p.LocalName() != "p" {
		t.Fatalf("expected <p>, got <%s>", p.LocalName())
	}
	em := p.ChildNodes()[0].(*dom.Element)
	if em.LocalName() != "em" {
		t.Errorf("expected <em>, got <%s>", em.LocalName())
	}
	if domText(em) != "italic" {
		t.Errorf("expected 'italic', got %q", domText(em))
	}
}

func TestRenderDOM_Strong(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("**bold**", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	strong := firstElement(p)
	if strong == nil {
		t.Fatalf("no element child in <p>")
	}
	if strong.LocalName() != "strong" {
		t.Errorf("expected <strong>, got <%s>", strong.LocalName())
	}
	if domText(strong) != "bold" {
		t.Errorf("expected 'bold', got %q", domText(strong))
	}
}

func TestRenderDOM_Strikethrough(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("~~deleted~~", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	s := p.ChildNodes()[0].(*dom.Element)
	if s.LocalName() != "s" {
		t.Errorf("expected <s>, got <%s>", s.LocalName())
	}
	if domText(s) != "deleted" {
		t.Errorf("expected 'deleted', got %q", domText(s))
	}
}

func TestRenderDOM_InlineCode(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("`code`", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	code := p.ChildNodes()[0].(*dom.Element)
	if code.LocalName() != "code" {
		t.Errorf("expected <code>, got <%s>", code.LocalName())
	}
	if domText(code) != "code" {
		t.Errorf("expected 'code', got %q", domText(code))
	}
}

func TestRenderDOM_Link(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("[text](http://example.com)", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	a := p.ChildNodes()[0].(*dom.Element)
	if a.LocalName() != "a" {
		t.Fatalf("expected <a>, got <%s>", a.LocalName())
	}
	if a.GetAttribute("href") != "http://example.com" {
		t.Errorf("expected href=http://example.com, got %q", a.GetAttribute("href"))
	}
	if domText(a) != "text" {
		t.Errorf("expected 'text', got %q", domText(a))
	}
}

func TestRenderDOM_Image(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("![alt](http://example.com/img.png)", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	img := p.ChildNodes()[0].(*dom.Element)
	if img.LocalName() != "img" {
		t.Fatalf("expected <img>, got <%s>", img.LocalName())
	}
	if img.GetAttribute("src") != "http://example.com/img.png" {
		t.Errorf("expected src, got %q", img.GetAttribute("src"))
	}
	if img.GetAttribute("alt") != "alt" {
		t.Errorf("expected alt='alt', got %q", img.GetAttribute("alt"))
	}
}

func TestRenderDOM_BulletList(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("- a\n- b\n- c", doc)
	ul := frag.ChildNodes()[0].(*dom.Element)
	if ul.LocalName() != "ul" {
		t.Fatalf("expected <ul>, got <%s>", ul.LocalName())
	}
	items := ul.GetElementsByTagName("li")
	if len(items) != 3 {
		t.Fatalf("expected 3 <li>, got %d", len(items))
	}
	if domText(items[0]) != "a" {
		t.Errorf("expected 'a', got %q", domText(items[0]))
	}
}

func TestRenderDOM_OrderedList(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("1. first\n2. second", doc)
	ol := frag.ChildNodes()[0].(*dom.Element)
	if ol.LocalName() != "ol" {
		t.Fatalf("expected <ol>, got <%s>", ol.LocalName())
	}
	items := ol.GetElementsByTagName("li")
	if len(items) != 2 {
		t.Fatalf("expected 2 <li>, got %d", len(items))
	}
}

func TestRenderDOM_Blockquote(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("> quoted", doc)
	bq := frag.ChildNodes()[0].(*dom.Element)
	if bq.LocalName() != "blockquote" {
		t.Fatalf("expected <blockquote>, got <%s>", bq.LocalName())
	}
	// Inside the blockquote there should be a <p>.
	ps := bq.GetElementsByTagName("p")
	if len(ps) != 1 {
		t.Fatalf("expected 1 <p> in blockquote, got %d", len(ps))
	}
	if domText(ps[0]) != "quoted" {
		t.Errorf("expected 'quoted', got %q", domText(ps[0]))
	}
}

func TestRenderDOM_HR(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("---", doc)
	hr := frag.ChildNodes()[0].(*dom.Element)
	if hr.LocalName() != "hr" {
		t.Errorf("expected <hr>, got <%s>", hr.LocalName())
	}
}

func TestRenderDOM_FencedCode(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("```go\nfmt.Println(\"hi\")\n```", doc)
	pre := frag.ChildNodes()[0].(*dom.Element)
	if pre.LocalName() != "pre" {
		t.Fatalf("expected <pre>, got <%s>", pre.LocalName())
	}
	code := pre.ChildNodes()[0].(*dom.Element)
	if code.LocalName() != "code" {
		t.Fatalf("expected <code>, got <%s>", code.LocalName())
	}
	if code.GetAttribute("class") != "language-go" {
		t.Errorf("expected class=language-go, got %q", code.GetAttribute("class"))
	}
}

func TestRenderDOM_FencedCodeWithHighlighter(t *testing.T) {
	doc := dom.NewDocument()
	h := NewEditorFenceHighlighter().RegisterDefaultLanguages().WithStyle(editor.ThemeDarkPlus())
	frag := ParseToDOMWithHighlighter("```go\nif x {}\n```", doc, h)
	pre := frag.ChildNodes()[0].(*dom.Element)
	code := pre.ChildNodes()[0].(*dom.Element)
	// Should have span children (tokens), not a single text node.
	var spans []*dom.Element
	for _, c := range code.ChildNodes() {
		if el, ok := c.(*dom.Element); ok {
			spans = append(spans, el)
		}
	}
	if len(spans) == 0 {
		t.Fatal("expected at least one <span> from highlighting, got none")
	}
	// The "if" keyword should be in a span with tok-keyword-* class.
	var hasKeyword bool
	for _, s := range spans {
		cls := s.GetAttribute("class")
		if strings.HasPrefix(cls, "tok-keyword") {
			hasKeyword = true
			if s.GetAttribute("style") == "" {
				t.Errorf("expected inline style for keyword token, got empty")
			}
		}
	}
	if !hasKeyword {
		t.Errorf("expected a keyword token, got classes: %v", spans[0].GetAttribute("class"))
	}
}

func TestRenderDOM_MixedContent(t *testing.T) {
	src := `# Title

This is a paragraph with **bold** and *italic*.

- item 1
- item 2

` + "```go" + `
package main
` + "```"
	doc := dom.NewDocument()
	frag := ParseToDOM(src, doc)
	tags := childTags(frag)
	// Expect h1, p, ul, pre (in order).
	if len(tags) < 4 {
		t.Fatalf("expected at least 4 top-level elements, got %d: %v", len(tags), tags)
	}
	if tags[0] != "h1" {
		t.Errorf("expected first element h1, got %s", tags[0])
	}
	if tags[1] != "p" {
		t.Errorf("expected second element p, got %s", tags[1])
	}
	if tags[2] != "ul" {
		t.Errorf("expected third element ul, got %s", tags[2])
	}
	if tags[3] != "pre" {
		t.Errorf("expected fourth element pre, got %s", tags[3])
	}
}

func TestRenderDOM_NestedEmphasis(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("**bold *and italic***", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	strong := firstElement(p)
	if strong == nil {
		t.Fatalf("no element child in <p>")
	}
	if strong.LocalName() != "strong" {
		t.Fatalf("expected <strong>, got <%s>", strong.LocalName())
	}
	// Inside <strong>, there should be text and <em>.
	var hasEm bool
	for _, c := range strong.ChildNodes() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "em" {
			hasEm = true
		}
	}
	if !hasEm {
		t.Errorf("expected <em> inside <strong>, children: %v", childTags(strong))
	}
}

func TestRenderDOM_HardBreak(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("line1  \nline2", doc)
	p := frag.ChildNodes()[0].(*dom.Element)
	var hasBr bool
	for _, c := range p.ChildNodes() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "br" {
			hasBr = true
		}
	}
	if !hasBr {
		t.Errorf("expected <br> for hard break, children tags: %v", childTags(p))
	}
}

func TestRenderDOM_EmptyInput(t *testing.T) {
	doc := dom.NewDocument()
	frag := ParseToDOM("", doc)
	if len(frag.ChildNodes()) != 0 {
		t.Errorf("expected empty fragment, got %d children", len(frag.ChildNodes()))
	}
}

func TestRenderDOM_TightList(t *testing.T) {
	// Tight lists don't have <p> wrappers around list items.
	doc := dom.NewDocument()
	frag := ParseToDOM("- a\n- b", doc)
	ul := frag.ChildNodes()[0].(*dom.Element)
	items := ul.GetElementsByTagName("li")
	if len(items) != 2 {
		t.Fatalf("expected 2 <li>, got %d", len(items))
	}
	// In tight mode, the <li> should contain text directly (not <p>).
	for i, li := range items {
		var hasP bool
		for _, c := range li.ChildNodes() {
			if el, ok := c.(*dom.Element); ok && el.LocalName() == "p" {
				hasP = true
			}
		}
		if hasP {
			t.Errorf("tight list item %d should not have <p>", i)
		}
	}
}

func TestRenderDOM_LooseList(t *testing.T) {
	// Loose lists have <p> wrappers around list items (separated by blank line).
	doc := dom.NewDocument()
	frag := ParseToDOM("- a\n\n- b", doc)
	ul := frag.ChildNodes()[0].(*dom.Element)
	items := ul.GetElementsByTagName("li")
	if len(items) != 2 {
		t.Fatalf("expected 2 <li>, got %d", len(items))
	}
	// In loose mode, each <li> should have a <p>.
	var pCount int
	for _, li := range items {
		pCount += len(li.GetElementsByTagName("p"))
	}
	if pCount != 2 {
		t.Errorf("expected 2 <p> in loose list, got %d", pCount)
	}
}

func TestEditorFenceHighlighter_PlainFallback(t *testing.T) {
	h := NewEditorFenceHighlighter() // no languages registered
	doc := dom.NewDocument()
	nodes := h.Highlight(doc, "code here", "unknown")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node for unknown lang, got %d", len(nodes))
	}
	if _, ok := nodes[0].(*dom.Text); !ok {
		t.Errorf("expected Text node for unknown lang, got %T", nodes[0])
	}
}

// --- GFM extension tests ---

func TestRenderDOM_GFMTable(t *testing.T) {
	doc := dom.NewDocument()
	src := "| H1 | H2 |\n| --- | --- |\n| a | b |"
	frag := ParseToDOMWithGFM(src, doc)
	children := frag.ChildNodes()
	// Expect a single <table> child.
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	table, ok := children[0].(*dom.Element)
	if !ok || table.LocalName() != "table" {
		t.Fatalf("expected <table>, got %T", children[0])
	}
	// <table> should contain <thead> and <tbody>.
	tags := childTags(table)
	if len(tags) != 2 || tags[0] != "thead" || tags[1] != "tbody" {
		t.Errorf("expected [thead, tbody], got %v", tags)
	}
	// <thead> > <tr> > 2 <th>.
	thead := firstElement(table)
	tr := firstElement(thead)
	thTags := childTags(tr)
	if len(thTags) != 2 || thTags[0] != "th" || thTags[1] != "th" {
		t.Errorf("expected 2 <th>, got %v", thTags)
	}
	// First <th> text should be "H1".
	firstTH := firstElement(tr)
	if domText(firstTH) != "H1" {
		t.Errorf("expected 'H1', got '%s'", domText(firstTH))
	}
}

func TestRenderDOM_GFMTableAlignment(t *testing.T) {
	doc := dom.NewDocument()
	src := "| H1 | H2 | H3 |\n| :--- | :---: | ---: |\n| a | b | c |"
	frag := ParseToDOMWithGFM(src, doc)
	table := firstElement(frag)
	if table == nil || table.LocalName() != "table" {
		t.Fatalf("expected <table>, got %+v", table)
	}
	thead := firstElement(table)
	tr := firstElement(thead)
	// Check alignment styles on <th>.
	ths := tr.ChildNodes()
	wantAligns := []string{"text-align:left", "text-align:center", "text-align:right"}
	for i, want := range wantAligns {
		if i >= len(ths) {
			break
		}
		th, ok := ths[i].(*dom.Element)
		if !ok {
			continue
		}
		got := th.GetAttribute("style")
		if got != want {
			t.Errorf("th[%d] style: expected '%s', got '%s'", i, want, got)
		}
	}
}

func TestRenderDOM_GFMTaskListUnchecked(t *testing.T) {
	doc := dom.NewDocument()
	src := "- [ ] todo item"
	frag := ParseToDOMWithGFM(src, doc)
	ul := firstElement(frag)
	if ul == nil || ul.LocalName() != "ul" {
		t.Fatalf("expected <ul>, got %+v", ul)
	}
	li := firstElement(ul)
	if li == nil || li.LocalName() != "li" {
		t.Fatalf("expected <li>, got %+v", li)
	}
	// <li> first child should be <input type="checkbox" disabled>.
	input := firstElement(li)
	if input == nil || input.LocalName() != "input" {
		t.Fatalf("expected <input>, got %+v", input)
	}
	if input.GetAttribute("type") != "checkbox" {
		t.Errorf("expected type=checkbox, got '%s'", input.GetAttribute("type"))
	}
	if input.GetAttribute("disabled") != "disabled" {
		t.Errorf("expected disabled, got '%s'", input.GetAttribute("disabled"))
	}
	if input.GetAttribute("checked") != "" {
		t.Errorf("expected no checked attribute, got '%s'", input.GetAttribute("checked"))
	}
	// Text after checkbox should be "todo item" (with leading space).
	text := domText(li)
	if !strings.Contains(text, "todo item") {
		t.Errorf("expected 'todo item' in text, got '%s'", text)
	}
}

func TestRenderDOM_GFMTaskListChecked(t *testing.T) {
	doc := dom.NewDocument()
	src := "- [x] done item"
	frag := ParseToDOMWithGFM(src, doc)
	ul := firstElement(frag)
	li := firstElement(ul)
	input := firstElement(li)
	if input.GetAttribute("checked") != "checked" {
		t.Errorf("expected checked='checked', got '%s'", input.GetAttribute("checked"))
	}
}

func TestRenderDOM_GFMBareAutolink(t *testing.T) {
	doc := dom.NewDocument()
	src := "Visit https://example.com today"
	frag := ParseToDOMWithGFM(src, doc)
	p := firstElement(frag)
	if p == nil || p.LocalName() != "p" {
		t.Fatalf("expected <p>, got %+v", p)
	}
	// <p> should contain text, <a>, text.
	var foundLink bool
	for _, c := range p.ChildNodes() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "a" {
			foundLink = true
			href := el.GetAttribute("href")
			if href != "https://example.com" {
				t.Errorf("expected href 'https://example.com', got '%s'", href)
			}
			if domText(el) != "https://example.com" {
				t.Errorf("expected link text 'https://example.com', got '%s'", domText(el))
			}
		}
	}
	if !foundLink {
		t.Errorf("expected an <a> element in <p>: %v", p.ChildNodes())
	}
}

func TestRenderDOM_GFMNotTaskList(t *testing.T) {
	// A list item without [ ] or [x] should not have a checkbox.
	doc := dom.NewDocument()
	src := "- regular item"
	frag := ParseToDOMWithGFM(src, doc)
	ul := firstElement(frag)
	li := firstElement(ul)
	for _, c := range li.ChildNodes() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "input" {
			t.Errorf("regular list item should not have an <input>, got: %v", c)
		}
	}
}
