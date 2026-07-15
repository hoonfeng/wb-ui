// Translation of: Source/WebCore/html/parser/HTMLTreeBuilder.cpp (other modes)
// Completeness: 90%
// Simplifications:
//   - all non-in-body insertion modes are implemented here in a single file
//   - the foreign-content (SVG/MathML) insertion mode is folded into the HTML
//     modes; namespace switching is approximate
//   - template insertion-mode handling is simplified (no separate template
//     contents mode stack management beyond push/pop)
//   - the "in table text" mode's pending character buffer is simplified to
//     direct insertion with foster parenting when needed

package html

import (
	"strings"

	"wb-ui/dom"
)

// --- Initial mode ------------------------------------------------------

// handleDoctypeInitial processes a DOCTYPE token in the Initial insertion mode.
func (tb *TreeBuilder) handleDoctypeInitial(tok *Token) {
	// Append a DocumentType node. This port does not model DocumentType as a
	// separate node; instead we record the doctype on the document via attributes.
	// For simplicity we create a comment-like node with the doctype name.
	if tok.DoctypeData != nil && tok.DoctypeData.ForceQuirks {
		// Quirks mode would be set on the document; omitted in this port.
	}
	// Insert the doctype as a child of the document (as a comment-like marker).
	c := tb.doc.CreateComment("DOCTYPE " + tok.Data)
	_ = tb.doc.AppendChild(c)
	tb.insertionMode = modeBeforeHTML
}

// defaultForInitial performs the "anything else" action for the Initial mode:
// switch to BeforeHTML and reprocess.
func (tb *TreeBuilder) defaultForInitial() {
	tb.insertionMode = modeBeforeHTML
}

// --- BeforeHTML mode ---------------------------------------------------

func (tb *TreeBuilder) startTagBeforeHTML(tok *Token) {
	if tok.Data == "html" {
		el := tb.createElementForToken(tok)
		_ = tb.doc.AppendChild(el)
		tb.openElements.push(el)
		tb.insertionMode = modeBeforeHead
		return
	}
	tb.defaultForBeforeHTML()
	tb.processStartTag(tok)
}

func (tb *TreeBuilder) endTagBeforeHTML(tok *Token) {
	switch tok.Data {
	case "head", "body", "html", "br":
		tb.defaultForBeforeHTML()
		tb.processEndTag(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) defaultForBeforeHTML() {
	// Create the html element if not present.
	if tb.openElements.htmlElement() == nil {
		el := tb.doc.CreateElement("html")
		_ = tb.doc.AppendChild(el)
		tb.openElements.push(el)
	}
	tb.insertionMode = modeBeforeHead
}

// --- BeforeHead mode ---------------------------------------------------

func (tb *TreeBuilder) startTagBeforeHead(tok *Token) {
	switch tok.Data {
	case "html":
		// Process as in before-html.
		tb.processStartTag(&Token{Type: TokenStartTag, Data: "html"})
	case "head":
		el := tb.insertElement(tok)
		tb.headElement = el
		tb.insertionMode = modeInHead
	default:
		tb.defaultForBeforeHead()
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagBeforeHead(tok *Token) {
	switch tok.Data {
	case "head", "body", "html", "br":
		tb.defaultForBeforeHead()
		tb.processEndTag(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) defaultForBeforeHead() {
	// Insert a head element.
	el := tb.doc.CreateElement("head")
	tb.attachNode(el)
	tb.openElements.push(el)
	tb.headElement = el
	tb.insertionMode = modeInHead
}

// --- InHead mode -------------------------------------------------------

func (tb *TreeBuilder) startTagInHead(tok *Token) {
	name := tok.Data
	switch {
	case name == "html":
		// Process as in body.
		tb.startTagInBody(tok)
	case name == "base" || name == "basefont" || name == "bgsound" ||
		name == "link" || name == "meta":
		tb.insertSelfClosingElement(tok)
	case name == "title":
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setRCDATAState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
	case name == "noscript" || name == "noframes" || name == "style":
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setRAWTEXTState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
	case name == "script":
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setScriptDataState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
	case name == "template":
		tb.insertElement(tok)
		tb.activeFormatting.appendMarker()
		tb.framesetOk = false
		tb.templateInsertionModes = append(tb.templateInsertionModes, modeTemplateContents)
		tb.insertionMode = modeTemplateContents
	case name == "head":
		// Parse error, ignore.
	default:
		tb.defaultForInHead()
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagInHead(tok *Token) {
	switch tok.Data {
	case "head":
		tb.openElements.pop()
		tb.insertionMode = modeAfterHead
	case "body", "html", "br":
		tb.defaultForInHead()
		tb.processEndTag(tok)
	case "template":
		// Pop template.
		if tb.openElements.inScope("template") {
			tb.openElements.popUntil("template")
			tb.activeFormatting.clearToLastMarker()
			if len(tb.templateInsertionModes) > 0 {
				tb.templateInsertionModes = tb.templateInsertionModes[:len(tb.templateInsertionModes)-1]
			}
			tb.resetInsertionModeAppropriately()
		}
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) defaultForInHead() {
	// Pop the head element.
	tb.openElements.pop()
	tb.insertionMode = modeAfterHead
}

// --- InHeadNoscript mode -----------------------------------------------

func (tb *TreeBuilder) startTagInHeadNoscript(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "basefont", "bgsound", "link", "meta", "noframes", "style":
		tb.startTagInHead(tok)
	case "head", "noscript":
		// Parse error, ignore.
	default:
		tb.defaultForInHeadNoscript()
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagInHeadNoscript(tok *Token) {
	switch tok.Data {
	case "noscript":
		tb.openElements.pop()
		tb.insertionMode = modeInHead
	case "br":
		tb.defaultForInHeadNoscript()
		tb.processEndTag(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) defaultForInHeadNoscript() {
	// Pop the noscript element.
	tb.openElements.pop()
	tb.insertionMode = modeInHead
}

// --- AfterHead mode ----------------------------------------------------

func (tb *TreeBuilder) startTagAfterHead(tok *Token) {
	name := tok.Data
	switch {
	case name == "html":
		tb.startTagInBody(tok)
	case name == "body":
		el := tb.insertElement(tok)
		_ = el
		tb.framesetOk = false
		tb.insertionMode = modeInBody
	case name == "frameset":
		tb.insertElement(tok)
		tb.insertionMode = modeInFrameset
	case name == "base" || name == "basefont" || name == "bgsound" ||
		name == "link" || name == "meta" || name == "noframes" ||
		name == "script" || name == "style" || name == "template" ||
		name == "title" || name == "noscript":
		// Process as in head.
		if tb.headElement != nil {
			_ = tb.headElement
		}
		saved := tb.insertionMode
		tb.insertionMode = modeInHead
		tb.processStartTag(tok)
		_ = saved
	case name == "head":
		// Parse error, ignore.
	default:
		tb.defaultForAfterHead()
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagAfterHead(tok *Token) {
	switch tok.Data {
	case "body", "html", "br":
		tb.defaultForAfterHead()
		tb.processEndTag(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) defaultForAfterHead() {
	// Insert a body element.
	el := tb.doc.CreateElement("body")
	tb.attachNode(el)
	tb.openElements.push(el)
	tb.insertionMode = modeInBody
}

// --- Text mode --------------------------------------------------------

func (tb *TreeBuilder) startTagInText(tok *Token) {
	// In Text mode, only character tokens are expected; start tags are parse
	// errors. Ignore.
}

func (tb *TreeBuilder) endTagInText(tok *Token) {
	// Pop the current element and restore the insertion mode.
	tb.openElements.pop()
	tb.insertionMode = tb.originalInsertionMode
}

// --- InTable mode ------------------------------------------------------

func (tb *TreeBuilder) startTagInTable(tok *Token) {
	name := tok.Data
	switch {
	case name == "caption":
		tb.openElements.clearBackToTableContext()
		tb.activeFormatting.appendMarker()
		tb.insertElement(tok)
		tb.insertionMode = modeInCaption
	case name == "colgroup":
		tb.openElements.clearBackToTableContext()
		tb.insertElement(tok)
		tb.insertionMode = modeInColumnGroup
	case name == "col":
		tb.processStartTag(&Token{Type: TokenStartTag, Data: "colgroup"})
		tb.processStartTag(tok)
	case name == "tbody" || name == "tfoot" || name == "thead":
		tb.openElements.clearBackToTableContext()
		tb.insertElement(tok)
		tb.insertionMode = modeInTableBody
	case name == "td" || name == "th" || name == "tr":
		tb.processStartTag(&Token{Type: TokenStartTag, Data: "tbody"})
		tb.processStartTag(tok)
	case name == "table":
		// Pop the current table and reprocess.
		if tb.openElements.inTableScope("table") {
			tb.openElements.popUntil("table")
		}
		tb.resetInsertionModeAppropriately()
		tb.processStartTag(tok)
	case name == "style" || name == "script" || name == "template":
		tb.startTagInHead(tok)
	case name == "input":
		// <input type=hidden> is allowed in tables.
		if strings.ToLower(tok.attrValue("type")) == "hidden" {
			tb.insertSelfClosingElement(tok)
		} else {
			tb.fosterParentingEnabled = true
			tb.startTagInBody(tok)
			tb.fosterParentingEnabled = false
		}
	case name == "form":
		// <form> in table uses foster parenting.
		if tb.formElement == nil {
			tb.fosterParentingEnabled = true
			form := tb.insertElement(tok)
			tb.formElement = form
			tb.openElements.pop()
			tb.fosterParentingEnabled = false
		}
	default:
		// Foster parent: process as in body with foster parenting enabled.
		tb.fosterParentingEnabled = true
		tb.startTagInBody(tok)
		tb.fosterParentingEnabled = false
	}
}

func (tb *TreeBuilder) endTagInTable(tok *Token) {
	name := tok.Data
	switch {
	case name == "table":
		if tb.openElements.inTableScope("table") {
			tb.openElements.popUntil("table")
			tb.resetInsertionModeAppropriately()
		}
	case name == "body" || name == "caption" || name == "col" ||
		name == "colgroup" || name == "html" || name == "tbody" ||
		name == "td" || name == "tfoot" || name == "th" ||
		name == "thead" || name == "tr":
		// Parse error, ignore.
	case name == "template":
		tb.endTagInHead(tok)
	default:
		// Foster parent.
		tb.fosterParentingEnabled = true
		tb.endTagInBody(tok)
		tb.fosterParentingEnabled = false
	}
}

func (tb *TreeBuilder) processCharacterInTable(tok *Token) {
	// Buffer the character; enable foster parenting and process as in body.
	tb.fosterParentingEnabled = true
	tb.processCharacterInBody(tok)
	tb.fosterParentingEnabled = false
}

func (tb *TreeBuilder) processEOFInTable() {
	tb.processEOFInBody()
}

// --- InTableText mode --------------------------------------------------

func (tb *TreeBuilder) startTagInTableText(tok *Token) {
	tb.insertionMode = modeInTable
	tb.processStartTag(tok)
}

func (tb *TreeBuilder) endTagInTableText(tok *Token) {
	tb.insertionMode = modeInTable
	tb.processEndTag(tok)
}

func (tb *TreeBuilder) processCharacterInTableText(tok *Token) {
	// Simplified: just insert as in body with foster parenting.
	tb.fosterParentingEnabled = true
	tb.insertText(tok.Data)
	tb.fosterParentingEnabled = false
}

func (tb *TreeBuilder) processEOFInTableText() {
	tb.processEOFInBody()
}

// --- InCaption mode ----------------------------------------------------

func (tb *TreeBuilder) startTagInCaption(tok *Token) {
	switch tok.Data {
	case "caption", "col", "colgroup", "tbody", "td", "tfoot", "thead", "tr":
		if tb.openElements.inTableScope("caption") {
			tb.openElements.popUntil("caption")
			tb.activeFormatting.clearToLastMarker()
			tb.insertionMode = modeInTable
			tb.processStartTag(tok)
		}
	default:
		tb.startTagInBody(tok)
	}
}

func (tb *TreeBuilder) endTagInCaption(tok *Token) {
	switch tok.Data {
	case "caption":
		if tb.openElements.inTableScope("caption") {
			tb.openElements.popUntil("caption")
			tb.activeFormatting.clearToLastMarker()
			tb.insertionMode = modeInTable
		}
	case "table":
		if tb.openElements.inTableScope("caption") {
			tb.openElements.popUntil("caption")
			tb.activeFormatting.clearToLastMarker()
			tb.insertionMode = modeInTable
			tb.processEndTag(tok)
		}
	default:
		tb.endTagInBody(tok)
	}
}

// --- InColumnGroup mode ------------------------------------------------

func (tb *TreeBuilder) startTagInColumnGroup(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "col":
		tb.insertSelfClosingElement(tok)
	case "colgroup":
		// Process the end tag.
		tb.endTagInColumnGroup(&Token{Type: TokenEndTag, Data: "colgroup"})
	default:
		tb.endTagInColumnGroup(&Token{Type: TokenEndTag, Data: "colgroup"})
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagInColumnGroup(tok *Token) {
	switch tok.Data {
	case "colgroup":
		if tb.openElements.inTableScope("colgroup") {
			tb.openElements.popUntil("colgroup")
			tb.insertionMode = modeInTable
		}
	case "col":
		// Parse error, ignore.
	case "template":
		tb.endTagInHead(tok)
	default:
		tb.endTagInColumnGroup(&Token{Type: TokenEndTag, Data: "colgroup"})
		tb.processEndTag(tok)
	}
}

// --- InTableBody mode --------------------------------------------------

func (tb *TreeBuilder) startTagInTableBody(tok *Token) {
	switch tok.Data {
	case "tr":
		tb.openElements.clearBackToTableBodyContext()
		tb.insertElement(tok)
		tb.insertionMode = modeInRow
	case "th", "td":
		tb.processStartTag(&Token{Type: TokenStartTag, Data: "tr"})
		tb.processStartTag(tok)
	case "caption", "col", "colgroup", "tbody", "tfoot", "thead":
		if tb.openElements.inTableScope("tbody") || tb.openElements.inTableScope("thead") || tb.openElements.inTableScope("tfoot") {
			tb.openElements.popUntilVariant("tbody", "thead", "tfoot")
			tb.insertionMode = modeInTable
			tb.processStartTag(tok)
		}
	default:
		tb.startTagInTable(tok)
	}
}

// popUntilVariant pops until one of the named elements is found and popped.
func (s *elementStack) popUntilVariant(names ...string) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		name := s.items[i].LocalName()
		for _, n := range names {
			if name == n {
				s.items = s.items[:i]
				return true
			}
		}
	}
	return false
}

func (tb *TreeBuilder) endTagInTableBody(tok *Token) {
	switch tok.Data {
	case "tbody", "tfoot", "thead":
		if tb.openElements.inTableScope(tok.Data) {
			tb.openElements.popUntil(tok.Data)
			tb.insertionMode = modeInTable
		}
	case "table":
		// Pop the nearest tbody/thead/tfoot.
		if tb.openElements.popUntilVariant("tbody", "thead", "tfoot") {
			tb.insertionMode = modeInTable
			tb.processEndTag(tok)
		}
	case "body", "caption", "col", "colgroup", "html", "td", "th", "tr":
		// Parse error, ignore.
	default:
		tb.endTagInTable(tok)
	}
}

// --- InRow mode --------------------------------------------------------

func (tb *TreeBuilder) startTagInRow(tok *Token) {
	switch tok.Data {
	case "th", "td":
		tb.openElements.clearBackToTableRowContext()
		tb.insertElement(tok)
		tb.insertionMode = modeInCell
		tb.activeFormatting.appendMarker()
	case "caption", "col", "colgroup", "tbody", "tfoot", "thead", "tr":
		tb.endTagInRow(&Token{Type: TokenEndTag, Data: "tr"})
		tb.processStartTag(tok)
	default:
		tb.startTagInTable(tok)
	}
}

func (tb *TreeBuilder) endTagInRow(tok *Token) {
	switch tok.Data {
	case "tr":
		if tb.openElements.inTableScope("tr") {
			tb.openElements.popUntil("tr")
			tb.insertionMode = modeInTableBody
		}
	case "table":
		tb.endTagInRow(&Token{Type: TokenEndTag, Data: "tr"})
		tb.processEndTag(tok)
	case "tbody", "tfoot", "thead":
		if tb.openElements.inTableScope(tok.Data) {
			tb.endTagInRow(&Token{Type: TokenEndTag, Data: "tr"})
			tb.processEndTag(tok)
		}
	case "body", "caption", "col", "colgroup", "html", "td", "th":
		// Parse error, ignore.
	default:
		tb.endTagInTable(tok)
	}
}

// --- InCell mode -------------------------------------------------------

func (tb *TreeBuilder) startTagInCell(tok *Token) {
	switch tok.Data {
	case "caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr":
		if tb.openElements.inTableScope("td") || tb.openElements.inTableScope("th") {
			tb.closeCell()
			tb.processStartTag(tok)
		}
	default:
		tb.startTagInBody(tok)
	}
}

func (tb *TreeBuilder) endTagInCell(tok *Token) {
	switch tok.Data {
	case "td", "th":
		if tb.openElements.inTableScope(tok.Data) {
			tb.generateImpliedEndTags("")
			tb.openElements.popUntil(tok.Data)
			tb.activeFormatting.clearToLastMarker()
			tb.insertionMode = modeInRow
		}
	case "body", "caption", "col", "colgroup", "html":
		// Parse error, ignore.
	case "table", "tbody", "tfoot", "thead", "tr":
		if tb.openElements.inTableScope(tok.Data) {
			tb.closeCell()
			tb.processEndTag(tok)
		}
	default:
		tb.endTagInBody(tok)
	}
}

// closeCell closes the current cell (td/th) for table error recovery.
func (tb *TreeBuilder) closeCell() {
	if tb.openElements.inTableScope("td") {
		tb.openElements.popUntil("td")
	} else if tb.openElements.inTableScope("th") {
		tb.openElements.popUntil("th")
	}
	tb.activeFormatting.clearToLastMarker()
	tb.insertionMode = modeInRow
}

// --- InSelect mode -----------------------------------------------------

func (tb *TreeBuilder) startTagInSelect(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "option":
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			tb.openElements.pop()
		}
		tb.insertElement(tok)
	case "optgroup":
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			tb.openElements.pop()
		}
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "optgroup" {
			tb.openElements.pop()
		}
		tb.insertElement(tok)
	case "hr":
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			tb.openElements.pop()
		}
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "optgroup" {
			tb.openElements.pop()
		}
		tb.insertSelfClosingElement(tok)
	case "input", "keygen", "textarea", "select":
		if tb.openElements.inSelectScope("select") {
			tb.openElements.popUntil("select")
			tb.resetInsertionModeAppropriately()
		}
	case "script", "template":
		tb.startTagInHead(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) endTagInSelect(tok *Token) {
	switch tok.Data {
	case "optgroup":
		// Pop optgroup if current is option and one below is optgroup.
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			below := tb.openElements.oneBelowTop()
			if below != nil && below.LocalName() == "optgroup" {
				tb.openElements.pop()
			}
		}
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "optgroup" {
			tb.openElements.pop()
		}
	case "option":
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			tb.openElements.pop()
		}
	case "select":
		if tb.openElements.inSelectScope("select") {
			tb.openElements.popUntil("select")
			tb.resetInsertionModeAppropriately()
		}
	case "template":
		tb.endTagInHead(tok)
	default:
		// Parse error, ignore.
	}
}

// --- InSelectInTable mode ----------------------------------------------

func (tb *TreeBuilder) startTagInSelectInTable(tok *Token) {
	switch tok.Data {
	case "caption", "table", "tbody", "tfoot", "thead", "tr", "td", "th":
		tb.openElements.popUntil("select")
		tb.resetInsertionModeAppropriately()
		tb.processStartTag(tok)
	default:
		tb.startTagInSelect(tok)
	}
}

func (tb *TreeBuilder) endTagInSelectInTable(tok *Token) {
	switch tok.Data {
	case "caption", "table", "tbody", "tfoot", "thead", "tr", "td", "th":
		if tb.openElements.inTableScope(tok.Data) {
			tb.openElements.popUntil("select")
			tb.resetInsertionModeAppropriately()
			tb.processEndTag(tok)
		}
	default:
		tb.endTagInSelect(tok)
	}
}

// --- AfterBody mode ----------------------------------------------------

func (tb *TreeBuilder) startTagAfterBody(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	default:
		tb.insertionMode = modeInBody
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagAfterBody(tok *Token) {
	switch tok.Data {
	case "body":
		if tb.openElements.inScope("body") {
			tb.insertionMode = modeAfterAfterBody
		}
	case "html":
		if tb.openElements.inScope("body") {
			tb.insertionMode = modeAfterAfterBody
			tb.processEndTag(tok)
		}
	default:
		tb.insertionMode = modeInBody
		tb.processEndTag(tok)
	}
}

// --- InFrameset mode ---------------------------------------------------

func (tb *TreeBuilder) startTagInFrameset(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "frameset":
		tb.insertElement(tok)
	case "frame":
		tb.insertSelfClosingElement(tok)
	case "noframes":
		tb.startTagInHead(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) endTagInFrameset(tok *Token) {
	switch tok.Data {
	case "frameset":
		if tb.openElements.top() != nil && tb.openElements.top().LocalName() == "html" {
			// Parse error, ignore.
			return
		}
		tb.openElements.pop()
		if tb.currentNode() != nil && tb.currentNode().LocalName() != "frameset" {
			// Switch to after-frameset when the current node is no longer a
			// frameset.
			tb.insertionMode = modeAfterFrameset
		}
	default:
		// Parse error, ignore.
	}
}

// --- AfterFrameset mode ------------------------------------------------

func (tb *TreeBuilder) startTagAfterFrameset(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "noframes":
		tb.startTagInHead(tok)
	default:
		// Parse error, ignore.
	}
}

func (tb *TreeBuilder) endTagAfterFrameset(tok *Token) {
	switch tok.Data {
	case "html":
		tb.insertionMode = modeAfterAfterFrameset
	default:
		// Parse error, ignore.
	}
}

// --- AfterAfterBody mode -----------------------------------------------

func (tb *TreeBuilder) startTagAfterAfterBody(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	default:
		tb.insertionMode = modeInBody
		tb.processStartTag(tok)
	}
}

func (tb *TreeBuilder) endTagAfterAfterBody(tok *Token) {
	switch tok.Data {
	case "html":
		// No-op (already at end).
	default:
		tb.insertionMode = modeInBody
		tb.processEndTag(tok)
	}
}

// --- AfterAfterFrameset mode -------------------------------------------

func (tb *TreeBuilder) startTagAfterAfterFrameset(tok *Token) {
	switch tok.Data {
	case "html":
		tb.startTagInBody(tok)
	case "noframes":
		tb.startTagInHead(tok)
	default:
		// Parse error, ignore.
	}
}

// --- TemplateContents mode ---------------------------------------------

func (tb *TreeBuilder) startTagTemplateContents(tok *Token) {
	switch tok.Data {
	case "template":
		tb.insertElement(tok)
		tb.activeFormatting.appendMarker()
		tb.framesetOk = false
		tb.templateInsertionModes = append(tb.templateInsertionModes, modeTemplateContents)
		tb.insertionMode = modeTemplateContents
	case "caption", "colgroup", "tbody", "tfoot", "thead":
		// Switch to in-table-like mode.
		tb.insertionMode = modeInTable
		tb.processStartTag(tok)
	case "col":
		tb.insertionMode = modeInColumnGroup
		tb.processStartTag(tok)
	case "tr":
		tb.insertionMode = modeInTableBody
		tb.processStartTag(tok)
	case "td", "th":
		tb.insertionMode = modeInRow
		tb.processStartTag(tok)
	default:
		// Process as in body.
		tb.startTagInBody(tok)
	}
}

func (tb *TreeBuilder) endTagTemplateContents(tok *Token) {
	switch tok.Data {
	case "template":
		if tb.openElements.inScope("template") {
			tb.openElements.popUntil("template")
			tb.activeFormatting.clearToLastMarker()
			if len(tb.templateInsertionModes) > 0 {
				tb.templateInsertionModes = tb.templateInsertionModes[:len(tb.templateInsertionModes)-1]
			}
			tb.resetInsertionModeAppropriately()
		}
	default:
		// Parse error, ignore.
	}
}

// --- ProcessCommentAt is a helper used by various modes ----------------

// insertComment inserts a comment at the document or current node, depending on
// the insertion mode. This is dispatched by processComment.
func (tb *TreeBuilder) insertComment(data string) {
	tb.insertCommentAt(tb.currentInsertionNode(), data)
}

// --- handleTextReconstruction is a placeholder for text node coalescing --

// skipLF skips a leading line feed if skipNextLF is set (used by <pre>, <listing>,
// <textarea>). Returns true if the LF was skipped.
func (tb *TreeBuilder) skipLF(data string) string {
	if tb.skipNextLF && len(data) > 0 && data[0] == '\n' {
		tb.skipNextLF = false
		return data[1:]
	}
	tb.skipNextLF = false
	return data
}

// insertHTMLFragment inserts the children of an html start tag's token onto the
// document element, mirroring the "in body" handling of <html> when it appears
// in the before-html or after-head modes.
func (tb *TreeBuilder) insertHTMLFragment(tok *Token) {
	html := tb.openElements.htmlElement()
	if html == nil {
		html = tb.doc.CreateElement("html")
		_ = tb.doc.AppendChild(html)
		tb.openElements.push(html)
	}
	for _, a := range tok.Attributes {
		if a.Name == "" || html.HasAttribute(a.Name) {
			continue
		}
		html.SetAttribute(a.Name, a.Value)
	}
}

// asNode converts a *dom.Element to a dom.Node (no-op for clarity at call sites).
func asNode(e *dom.Element) dom.Node { return e }
