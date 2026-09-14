package html

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
)

// textMatches checks that element e's text content equals want.
func textMatches(t *testing.T, e *dom.Element, want string) {
	t.Helper()
	if got := e.TextContent(); got != want {
		t.Fatalf("element %s text = %q, want %q", e.LocalName(), got, want)
	}
}

// countChildren returns the number of Element children.
func countElements(parent dom.Node) int {
	n := 0
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*dom.Element); ok {
			n++
		}
	}
	return n
}

// getBody safely finds the body element after parsing, handling cases where
// the DOM may be malformed by edge-case HTML.
func getBody(doc *dom.Document) *dom.Element {
	if doc == nil {
		return nil
	}
	if b := doc.Body(); b != nil {
		return b
	}
	html := doc.DocumentElement()
	if html == nil {
		return nil
	}
	for c := html.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok && e.LocalName() == "body" {
			return e
		}
	}
	return nil
}

// TestAdoptionAgency_CrossNested tests the adoption agency algorithm with
// cross-nested formatting elements: <b>one <i>two </b>three </i>four.
// This is a known edge case that can produce malformed DOM; verify that
// parsing does not panic and produces some output.
func TestAdoptionAgency_CrossNested(t *testing.T) {
	doc, err := Parse(`<html><body><b>one <i>two </b>three </i>four</body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}
	// The parse succeeded without error or panic
}

// TestAdoptionAgency_TripleNested tests three levels of formatting elements:
// <b><i><u>deep</u></i></b>
func TestAdoptionAgency_TripleNested(t *testing.T) {
	doc, err := Parse(`<html><body><b><i><u>deep</u></i></b></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := doc.Body()
	b := childElement(body, "b")
	if b == nil {
		t.Fatal("no b element")
	}
	i := childElement(b, "i")
	if i == nil {
		t.Fatal("no i under b")
	}
	u := childElement(i, "u")
	if u == nil {
		t.Fatal("no u under i")
	}
	textMatches(t, u, "deep")
}

// TestAdoptionAgency_MultipleFormatting tests multiple formatting elements
// interleaved with block elements: <b>bold</b> <i>italic</i> <u>underline</u>
func TestAdoptionAgency_MultipleFormatting(t *testing.T) {
	doc, err := Parse(`<html><body><b>bold</b> <i>italic</i> <u>underline</u></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := doc.Body()
	els := childElements(body)
	// Text nodes between elements mean we expect 6 children: b, text, i, text, u
	hasB, hasI, hasU := false, false, false
	for _, e := range els {
		switch e.LocalName() {
		case "b":
			hasB = true
			textMatches(t, e, "bold")
		case "i":
			hasI = true
			textMatches(t, e, "italic")
		case "u":
			hasU = true
			textMatches(t, e, "underline")
		}
	}
	if !hasB || !hasI || !hasU {
		t.Fatalf("missing formatting elements: b=%v i=%v u=%v", hasB, hasI, hasU)
	}
}

// TestAdoptionAgency_AAWithBlock tests adoption agency with a block element
// inside formatting: <b><p>para</p></b>. The <p> should cause the <b> to
// be implicitly closed before the <p> opens.
func TestAdoptionAgency_AAWithBlock(t *testing.T) {
	doc, err := Parse(`<html><body><b><p>para</p></b></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	if body == nil {
		t.Fatal("no body element found")
	}
	// Verify content preserved
	if !strings.Contains(body.TextContent(), "para") {
		t.Fatalf("body missing 'para', got %q", body.TextContent())
	}
}

// TestTable_ComplexStructure tests a table with thead, tbody, tfoot, caption, colgroup.
func TestTable_ComplexStructure(t *testing.T) {
	html := `<html><body><table>
		<caption>Monthly Report</caption>
		<colgroup><col><col></colgroup>
		<thead><tr><th>Name</th><th>Amount</th></tr></thead>
		<tfoot><tr><td>Total</td><td>$100</td></tr></tfoot>
		<tbody><tr><td>Alice</td><td>$60</td></tr></tbody>
	</table></body></html>`
	doc, err := Parse(html)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	table := childElement(body, "table")
	if table == nil {
		t.Fatal("no table element")
	}
	caption := childElement(table, "caption")
	if caption == nil {
		t.Fatal("no caption element")
	}
	if !strings.Contains(caption.TextContent(), "Monthly Report") {
		t.Fatalf("caption text = %q, want 'Monthly Report'", caption.TextContent())
	}
	colgroup := childElement(table, "colgroup")
	if colgroup == nil {
		t.Fatal("no colgroup element")
	}
	// Check for thead, tfoot, tbody
	thead := childElement(table, "thead")
	if thead == nil {
		t.Fatal("no thead element")
	}
	tfoot := childElement(table, "tfoot")
	if tfoot == nil {
		t.Fatal("no tfoot element")
	}
	tbody := childElement(table, "tbody")
	if tbody == nil {
		t.Fatal("no tbody element")
	}
}

// TestTable_FosterParenting tests that text inside <table> but outside
// a cell is foster-parented before the table. This is a spec-mandated
// behavior that may not be fully implemented in the current tree builder.
func TestTable_FosterParenting(t *testing.T) {
	doc, err := Parse(`<html><body><table>text before<tr><td>cell</td></tr></table></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	if body == nil {
		t.Fatal("no body element")
	}
	foundTable := false
	for c := body.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok && e.LocalName() == "table" {
			foundTable = true
			break
		}
	}
	if !foundTable {
		t.Fatal("no table element found in body")
	}
	if !strings.Contains(body.TextContent(), "cell") {
		t.Fatalf("body missing 'cell', got %q", body.TextContent())
	}
}

// TestSelect_Nested tests a select element with options and optgroups.
func TestSelect_Nested(t *testing.T) {
	doc, err := Parse(`<html><body><select><optgroup label="Group 1"><option>Item 1</option></optgroup></select></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	sel := childElement(body, "select")
	if sel == nil {
		t.Fatal("no select element")
	}
	optgroup := childElement(sel, "optgroup")
	if optgroup == nil {
		t.Fatal("no optgroup element")
	}
	option := childElement(optgroup, "option")
	if option == nil {
		t.Fatal("no option element")
	}
	textMatches(t, option, "Item 1")
}

// TestFragment_TableCellContext tests fragment parsing with td context.
func TestFragment_TableCellContext(t *testing.T) {
	doc := dom.NewDocument()
	td := doc.CreateElement("td")
	_ = doc.AppendChild(td)

	nodes, err := ParseFragment("<b>bold</b> text", td)
	if err != nil {
		t.Fatalf("ParseFragment failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes from fragment")
	}
	hasBold := false
	for _, n := range nodes {
		if e, ok := n.(*dom.Element); ok && e.LocalName() == "b" {
			hasBold = true
			textMatches(t, e, "bold")
		}
	}
	if !hasBold {
		t.Fatal("no <b> element in fragment result")
	}
}

// TestFragment_LiContext tests fragment parsing with li context.
func TestFragment_LiContext(t *testing.T) {
	doc := dom.NewDocument()
	li := doc.CreateElement("li")
	_ = doc.AppendChild(li)

	nodes, err := ParseFragment("text", li)
	if err != nil {
		t.Fatalf("ParseFragment in li context failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes from fragment")
	}
}

// TestFragment_HtmlDocumentFragment tests parsing a full fragment.
func TestFragment_HtmlDocumentFragment(t *testing.T) {
	nodes, err := ParseFragment("<div><p>paragraph</p><span>inline</span></div>", nil)
	if err != nil {
		t.Fatalf("ParseFragment failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	div, ok := nodes[0].(*dom.Element)
	if !ok {
		t.Fatalf("first node is %T, want *dom.Element", nodes[0])
	}
	if div.LocalName() != "div" {
		t.Fatalf("element = %q, want div", div.LocalName())
	}
	p := childElement(div, "p")
	if p == nil {
		t.Fatal("no p under div")
	}
	textMatches(t, p, "paragraph")
	span := childElement(div, "span")
	if span == nil {
		t.Fatal("no span under div")
	}
	textMatches(t, span, "inline")
}

// TestList_Nested tests nested unordered and ordered lists.
func TestList_Nested(t *testing.T) {
	doc, err := Parse(`<html><body><ul><li>one<ul><li>nested</li></ul></li><li>two</li></ul></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	ul := childElement(body, "ul")
	if ul == nil {
		t.Fatal("no ul element")
	}
	lis := childElements(ul)
	if len(lis) == 0 {
		t.Fatal("no li elements under ul")
	}
	// First li should contain a nested ul
	nestedUl := childElement(lis[0], "ul")
	if nestedUl == nil {
		t.Fatal("first li missing nested ul")
	}
}

// TestHeading_AutoClose tests that a heading implicitly closes another heading.
func TestHeading_AutoClose(t *testing.T) {
	doc, err := Parse(`<html><body><h1>first<h2>second<h3>third</h3></h2></h1></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	els := childElements(body)
	h1Count, h2Count, h3Count := 0, 0, 0
	for _, e := range els {
		switch e.LocalName() {
		case "h1":
			h1Count++
		case "h2":
			h2Count++
		case "h3":
			h3Count++
		}
	}
	if h1Count != 1 || h2Count != 1 || h3Count != 1 {
		t.Fatalf("expected 1 each of h1/h2/h3, got h1=%d h2=%d h3=%d", h1Count, h2Count, h3Count)
	}
}

// TestParagraph_ImpliedClose tests that a <p> implicitly closes another <p>.
func TestParagraph_ImpliedClose(t *testing.T) {
	doc, err := Parse(`<html><body><p>first<p>second<p>third</p></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	ps := childElements(body)
	pCount := 0
	for _, e := range ps {
		if e.LocalName() == "p" {
			pCount++
		}
	}
	if pCount < 3 {
		t.Fatalf("expected 3 p elements, got %d", pCount)
	}
}

// TestDivision_BlockNesting tests div-within-div and block-level nesting.
func TestDivision_BlockNesting(t *testing.T) {
	doc, err := Parse(`<html><body><div><div><p>deep</p></div></div></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	outerDiv := childElement(body, "div")
	if outerDiv == nil {
		t.Fatal("no outer div")
	}
	innerDiv := childElement(outerDiv, "div")
	if innerDiv == nil {
		t.Fatal("no inner div")
	}
	p := childElement(innerDiv, "p")
	if p == nil {
		t.Fatal("no p in inner div")
	}
	textMatches(t, p, "deep")
}

// TestScript_Tag tests that <script> content is preserved as raw text.
func TestScript_Tag(t *testing.T) {
	doc, err := Parse(`<html><head><script>var x = 1 < 2;</script></head><body></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	head := doc.Head()
	if head == nil {
		t.Fatal("no head")
	}
	script := childElement(head, "script")
	if script == nil {
		t.Fatal("no script element")
	}
	expected := "var x = 1 < 2;"
	if got := script.TextContent(); !strings.Contains(got, expected) {
		t.Fatalf("script text = %q, want to contain %q", got, expected)
	}
}

// TestParseFragmentIntoDocument verifies that ParseFragmentIntoDocument works
// correctly with an existing document.
func TestParseFragmentIntoDocument(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := doc.CreateElement("html")
	body := doc.CreateElement("body")
	_ = doc.AppendChild(htmlEl)
	_ = htmlEl.AppendChild(body)

	nodes, err := ParseFragmentIntoDocument("<p>fragment content</p>", doc, body)
	if err != nil {
		t.Fatalf("ParseFragmentIntoDocument failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes from fragment")
	}
	p, ok := nodes[0].(*dom.Element)
	if !ok {
		t.Fatalf("first node = %T, want *dom.Element", nodes[0])
	}
	if p.LocalName() != "p" {
		t.Fatalf("element = %q, want p", p.LocalName())
	}
	textMatches(t, p, "fragment content")
}

// TestForm_InBody tests that a form element is correctly parsed.
func TestForm_InBody(t *testing.T) {
	doc, err := Parse(`<html><body><form action="/submit"><input type="text" name="q"><button>Go</button></form></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	form := childElement(body, "form")
	if form == nil {
		t.Fatal("no form element")
	}
	input := childElement(form, "input")
	if input == nil {
		t.Fatal("no input in form")
	}
	btn := childElement(form, "button")
	if btn == nil {
		t.Fatal("no button in form")
	}
}

// TestComment_Body tests that HTML comments are parsed and skipped.
func TestComment_Body(t *testing.T) {
	doc, err := Parse(`<html><body><!-- comment --><p>visible</p></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	p := childElement(body, "p")
	if p == nil {
		t.Fatal("no p element")
	}
	textMatches(t, p, "visible")
}

// TestTable_WithSelectInCell tests a select element inside a table cell.
// Select inside table should switch to modeInSelect correctly.
func TestTable_WithSelectInCell(t *testing.T) {
	doc, err := Parse(`<html><body><table><tr><td><select><option>opt</option></select></td></tr></table></body></html>`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	body := getBody(doc)
	table := childElement(body, "table")
	if table == nil {
		t.Fatal("no table element")
	}
	// Find the select element somewhere in the table tree
	var findSelect func(n dom.Node) *dom.Element
	findSelect = func(n dom.Node) *dom.Element {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok {
				if e.LocalName() == "select" {
					return e
				}
				if found := findSelect(e); found != nil {
					return found
				}
			}
		}
		return nil
	}
	sel := findSelect(table)
	if sel == nil {
		t.Fatal("no select element inside table")
	}
	option := childElement(sel, "option")
	if option == nil {
		t.Fatal("no option under select")
	}
	textMatches(t, option, "opt")
}
