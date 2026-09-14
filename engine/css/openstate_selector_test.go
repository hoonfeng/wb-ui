// Tests for the :open / :closed pseudo-classes (HTML §4.16.4): they only match
// the elements that have an "open state" (details / dialog / select); every
// other element matches neither.

package css

import (
	"testing"

	"wb-ui/engine/dom"
)

func TestSelector_OpenClosedPseudoClasses(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)

	detailsOpen := dom.NewElement(doc, "details")
	detailsOpen.SetAttribute("open", "")
	body.AppendChild(detailsOpen)
	detailsClosed := dom.NewElement(doc, "details")
	body.AppendChild(detailsClosed)
	dialogOpen := dom.NewElement(doc, "dialog")
	dialogOpen.SetAttribute("open", "")
	body.AppendChild(dialogOpen)
	selectOpen := dom.NewElement(doc, "select")
	selectOpen.SetAttribute("open", "")
	body.AppendChild(selectOpen)
	divOpen := dom.NewElement(doc, "div")
	divOpen.SetAttribute("open", "")
	body.AppendChild(divOpen)
	divPlain := dom.NewElement(doc, "div")
	body.AppendChild(divPlain)

	c := NewSelectorChecker()
	open := mustParseOneSelector(t, ":open")
	closed := mustParseOneSelector(t, ":closed")

	if open.String() != ":open" {
		t.Fatalf("ComplexSelector.String() = %q，want \":open\"", open.String())
	}
	if closed.String() != ":closed" {
		t.Fatalf("ComplexSelector.String() = %q，want \":closed\"", closed.String())
	}

	// 有 open 状态的元素：按 open 属性区分。
	if !c.Match(open, detailsOpen) {
		t.Fatal("<details open> 应匹配 :open")
	}
	if c.Match(closed, detailsOpen) {
		t.Fatal("<details open> 不应匹配 :closed")
	}
	if c.Match(open, detailsClosed) {
		t.Fatal("<details> 不应匹配 :open")
	}
	if !c.Match(closed, detailsClosed) {
		t.Fatal("<details> 应匹配 :closed")
	}
	if !c.Match(open, dialogOpen) {
		t.Fatal("<dialog open> 应匹配 :open")
	}
	if !c.Match(open, selectOpen) {
		t.Fatal("<select open> 应匹配 :open")
	}

	// 没有 open 状态的元素：两个伪类都不匹配（即使带 open 属性）。
	if c.Match(open, divOpen) {
		t.Fatal("<div open> 不应匹配 :open（div 没有 open 状态）")
	}
	if c.Match(closed, divOpen) {
		t.Fatal("<div open> 不应匹配 :closed（div 没有 open 状态）")
	}
	if c.Match(closed, divPlain) {
		t.Fatal("<div> 不应匹配 :closed")
	}
	if c.Match(open, body) || c.Match(closed, body) {
		t.Fatal("<body> 不应匹配 :open / :closed")
	}
}
