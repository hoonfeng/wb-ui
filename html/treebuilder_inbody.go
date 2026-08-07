// Translation of: Source/WebCore/html/parser/HTMLTreeBuilder.cpp (in-body mode)
//                  Source/WebCore/html/parser/HTMLConstructionSite.cpp (attach/foster)
// Completeness: 90%
// Simplifications:
//   - the in-body mode is the largest insertion mode; this file covers the common
//     cases (block/inline/formatting/table/form elements, headers, lists, paragraphs)
//   - the adoption agency algorithm is implemented as a separate method in this file
//   - foster parenting places the node before the table rather than at the last
//     template/element slot
//   - script/style/template start tags are redirected to the in-head processor
//   - SVG/MathML foreign-content paths are folded into the HTML handling

package html

import (
	"strings"

	"wb-ui/dom"
)

// fosterParent attaches child per the foster parenting rules: when foster
// parenting is enabled (inside a table), the node is inserted as the last child
// of the table's parent, just before the table itself.
func (tb *TreeBuilder) fosterParent(child dom.Node) {
	// Find the last table element on the stack of open elements.
	lastTableIdx := -1
	for i := tb.openElements.size() - 1; i >= 0; i-- {
		if tb.openElements.at(i).LocalName() == "table" {
			lastTableIdx = i
			break
		}
	}

	var parent dom.Node
	if lastTableIdx == -1 {
		// No table on the stack: foster parent to the html element.
		parent = tb.openElements.htmlElement()
	} else {
		// The parent is the element immediately below the table.
		if lastTableIdx == 0 {
			parent = tb.doc
		} else {
			parent = tb.openElements.at(lastTableIdx - 1)
		}
	}
	if parent == nil {
		parent = tb.doc
	}

	// If the table has no previous sibling (table is first child of its parent),
	// append the child to the parent and let the table follow.
	tableEl := tb.openElements.at(lastTableIdx)
	if tableEl == nil {
		_ = parent.AppendChild(child)
		return
	}
	// Find the table's previous sibling; insert child before the table.
	if tableEl.PreviousSibling() == nil {
		// table is the first child: make child the new first child by appending
		// and the table will be reordered by the DOM append semantics (which moves
		// existing nodes). We use InsertBefore with table as the reference.
		_ = parent.InsertBefore(child, tableEl)
		return
	}
	_ = parent.InsertBefore(child, tableEl)
}

// --- In-body insertion mode ---------------------------------------------

// startTagInBody handles a start-tag token in the "in body" insertion mode,
// mirroring HTMLTreeBuilder::processStartTag for InBodyMode.
func (tb *TreeBuilder) startTagInBody(tok *Token) {
	name := tok.Data
	switch {
	case name == "html":
		// Copy attributes onto the existing html element (first wins).
		html := tb.openElements.htmlElement()
		if html != nil {
			for _, a := range tok.Attributes {
				if a.Name == "" || html.HasAttribute(a.Name) {
					continue
				}
				html.SetAttribute(a.Name, a.Value)
			}
		}
		return
	case name == "base" || name == "basefont" || name == "bgsound" ||
		name == "link" || name == "meta" || name == "title" ||
		name == "script" || name == "style" || name == "template":
		// Process as if in head.
		tb.processStartTagInHead(tok)
		return
	case name == "body":
		body := tb.openElements.bodyElement()
		if body != nil {
			for _, a := range tok.Attributes {
				if a.Name == "" || body.HasAttribute(a.Name) {
					continue
				}
				body.SetAttribute(a.Name, a.Value)
			}
		}
		tb.framesetOk = false
		return
	case name == "frameset":
		if !tb.framesetOk {
			return
		}
		body := tb.openElements.bodyElement()
		if body == nil {
			return
		}
		// Replace body with frameset.
		parent := body.ParentNode()
		if parent == nil {
			return
		}
		_ = parent.RemoveChild(body)
		fs := tb.createElementForToken(tok)
		_ = parent.AppendChild(fs)
		tb.openElements.remove(body)
		tb.openElements.push(fs)
		tb.insertionMode = modeInFrameset
		return
	case isSpecialBlock(name):
		// address/article/aside/blockquote/center/details/dialog/dir/div/dl/
		// fieldset/figcaption/figure/footer/header/hgroup/main/menu/nav/ol/p/
		// search/section/summary/ul
		tb.closePInButtonScope()
		tb.insertElement(tok)
		return
	case isHeader(name):
		// h1..h6
		tb.closePInButtonScope()
		if tb.openElements.hasNumberedHeaderInScope() {
			tb.openElements.popUntilElement(tb.openElements.top())
		}
		tb.insertElement(tok)
		return
	case name == "pre" || name == "listing":
		tb.closePInButtonScope()
		tb.insertElement(tok)
		// Spec: skip a following LF.
		tb.skipNextLF = true
		return
	case name == "form":
		if tb.formElement == nil {
			tb.closePInButtonScope()
			form := tb.insertElement(tok)
			tb.formElement = form
		}
		return
	case name == "li":
		tb.stopLi = true
		tb.processListItemStartTag(tok)
		tb.stopLi = false
		return
	case name == "dd" || name == "dt":
		tb.processDdDtStartTag(tok)
		return
	case name == "plaintext":
		tb.closePInButtonScope()
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setPLAINTEXTState()
		}
		return
	case name == "button":
		if tb.openElements.inButtonScope("button") {
			// Parse error: close the button element.
			tb.generateImpliedEndTags("")
			tb.openElements.popUntil("button")
		}
		tb.reconstructActiveFormatting()
		tb.insertElement(tok)
		tb.framesetOk = false
		return
	case name == "a":
		// If there is an active <a>, run the adoption agency for it, then remove.
		if existing := tb.activeFormatting.findByName("a"); existing >= 0 {
			tb.callTheAdoptionAgency(tb.activeFormatting.entryAt(existing))
			if idx := tb.activeFormatting.findByName("a"); idx >= 0 {
				tb.activeFormatting.removeAtIndex(idx)
			}
		}
		tb.reconstructActiveFormatting()
		el := tb.insertElement(tok)
		tb.activeFormatting.append(el)
		return
	case formattingElements[name]:
		tb.reconstructActiveFormatting()
		el := tb.insertElement(tok)
		tb.activeFormatting.append(el)
		return
	case name == "nobr":
		if tb.openElements.inScope("nobr") {
			tb.callTheAdoptionAgency(tb.openElements.topmost("nobr"))
			tb.reconstructActiveFormatting()
		} else {
			tb.reconstructActiveFormatting()
		}
		el := tb.insertElement(tok)
		tb.activeFormatting.append(el)
		return
	case name == "applet" || name == "marquee" || name == "object":
		tb.reconstructActiveFormatting()
		tb.insertElement(tok)
		tb.activeFormatting.appendMarker()
		tb.framesetOk = false
		return
	case name == "table":
		if tb.openElements.inButtonScope("p") {
			tb.closePInButtonScope()
		}
		tb.insertElement(tok)
		tb.insertionMode = modeInTable
		tb.framesetOk = false
		return
	case name == "area" || name == "br" || name == "embed" ||
		name == "img" || name == "keygen" || name == "wbr":
		tb.reconstructActiveFormatting()
		tb.insertSelfClosingElement(tok)
		if name == "input" {
			// handled below
		}
		tb.framesetOk = false
		return
	case name == "input":
		tb.reconstructActiveFormatting()
		tb.insertSelfClosingElement(tok)
		// Spec: if the type is not "hidden", framesetOk = false.
		if strings.ToLower(tok.attrValue("type")) != "hidden" {
			tb.framesetOk = false
		}
		return
	case name == "param" || name == "source" || name == "track":
		tb.insertSelfClosingElement(tok)
		return
	case name == "hr":
		tb.closePInButtonScope()
		tb.insertSelfClosingElement(tok)
		tb.framesetOk = false
		return
	case name == "image":
		// Parse error: treat as <img>.
		tok.Data = "img"
		tb.startTagInBody(tok)
		return
	case name == "textarea":
		tb.closePInButtonScope()
		tb.insertElement(tok)
		tb.skipNextLF = true
		tb.framesetOk = false
		if tb.tokenizer != nil {
			tb.tokenizer.setRCDATAState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
		return
	case name == "xmp":
		tb.closePInButtonScope()
		tb.reconstructActiveFormatting()
		tb.framesetOk = false
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setRAWTEXTState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
		return
	case name == "iframe":
		tb.insertSelfClosingElement(tok)
		tb.framesetOk = false
		if tb.tokenizer != nil {
			tb.tokenizer.setRAWTEXTState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
		return
	case name == "noembed" || name == "noscript":
		tb.reconstructActiveFormatting()
		tb.insertElement(tok)
		if tb.tokenizer != nil {
			tb.tokenizer.setRAWTEXTState()
		}
		tb.originalInsertionMode = tb.insertionMode
		tb.insertionMode = modeText
		return
	case name == "select":
		tb.reconstructActiveFormatting()
		tb.insertElement(tok)
		tb.framesetOk = false
		switch tb.insertionMode {
		case modeInTable, modeInCaption, modeInTableBody, modeInRow, modeInCell:
			tb.insertionMode = modeInSelectInTable
		default:
			tb.insertionMode = modeInSelect
		}
		return
	case name == "optgroup" || name == "option":
		if tb.currentNode() != nil && tb.currentNode().LocalName() == "option" {
			tb.openElements.pop()
		}
		tb.reconstructActiveFormatting()
		tb.insertElement(tok)
		return
	case name == "colgroup" || name == "col" || name == "caption" ||
		name == "tbody" || name == "tfoot" || name == "thead" ||
		name == "tr" || name == "td" || name == "th":
		// Clear back to table context and process as in-table.
		tb.openElements.clearBackToTableContext()
		tb.insertionMode = modeInTable
		tb.processStartTag(tok)
		return
	case name == "math" || name == "svg":
		// Foreign content: handled approximately as regular elements.
		// SVG 里的自闭合标签（<rect/> 等）必须真正自闭合，否则 rect 会
		// 成为开元素把后续 circle/text 全部嵌套进去。
		tb.reconstructActiveFormatting()
		if tok.SelfClosing {
			tb.insertSelfClosingElement(tok)
			return
		}
		tb.insertElement(tok)
		return
	default:
		// Anything else: reconstruct active formatting and insert.
		// SVG 内容（foreign content）里的自闭合标签按自闭合处理。
		tb.reconstructActiveFormatting()
		if tok.SelfClosing && tb.inSVGContent() {
			tb.insertSelfClosingElement(tok)
			return
		}
		tb.insertElement(tok)
		return
	}
}

// inSVGContent reports whether the current insertion context is inside an
// <svg> element (foreign-content approximation). Used to honour self-closing
// tags (<rect/> etc.) only within SVG, keeping regular HTML semantics
// (where a trailing slash on non-void tags is ignored) intact.
func (tb *TreeBuilder) inSVGContent() bool {
	for i := tb.openElements.size() - 1; i >= 0; i-- {
		if strings.EqualFold(tb.openElements.at(i).LocalName(), "svg") {
			return true
		}
	}
	return false
}

// isSpecialBlock reports whether name is a "special" block element handled by
// the same case in the in-body start-tag handler.
func isSpecialBlock(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "center", "details",
		"dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure",
		"footer", "header", "hgroup", "main", "menu", "nav", "ol", "p",
		"search", "section", "summary", "ul":
		return true
	}
	return false
}

// processListItemStartTag handles a <li> start tag per the in-body mode rules.
func (tb *TreeBuilder) processListItemStartTag(tok *Token) {
	// Generate implied end tags except for li.
	tb.generateImpliedEndTagsExceptList()
	if tb.openElements.inListItemScope("li") {
		// Pop until li.
		tb.openElements.popUntil("li")
	}
	tb.reconstructActiveFormatting()
	tb.insertElement(tok)
}

// generateImpliedEndTagsExceptList generates implied end tags but stops at li.
func (tb *TreeBuilder) generateImpliedEndTagsExceptList() {
	for tb.openElements.size() > 0 {
		name := tb.openElements.top().LocalName()
		if name == "li" {
			return
		}
		if !impliedEndTagElements[name] {
			return
		}
		tb.openElements.pop()
	}
}

// processDdDtStartTag handles <dd>/<dt> start tags.
func (tb *TreeBuilder) processDdDtStartTag(tok *Token) {
	name := tok.Data
	tb.generateImpliedEndTagsExceptDdDt()
	if tb.openElements.inListItemScope(name) {
		tb.openElements.popUntil(name)
	}
	tb.reconstructActiveFormatting()
	tb.insertElement(tok)
}

// generateImpliedEndTagsExceptDdDt generates implied end tags but stops at dd/dt.
func (tb *TreeBuilder) generateImpliedEndTagsExceptDdDt() {
	for tb.openElements.size() > 0 {
		name := tb.openElements.top().LocalName()
		if name == "dd" || name == "dt" {
			return
		}
		if !impliedEndTagElements[name] {
			return
		}
		tb.openElements.pop()
	}
}

// findByName returns the index of the first formatting entry with the given tag
// name, scanning from the end. Returns -1 if not found.
func (l *formattingList) findByName(name string) int {
	for i := len(l.entries) - 1; i >= 0; i-- {
		e := l.entries[i]
		if e == nil {
			return -1
		}
		if e.LocalName() == name {
			return i
		}
	}
	return -1
}

// processStartTagInHead processes a start tag using the in-head insertion mode.
// It is a small dispatcher that re-routes head-level elements (base/link/meta/
// script/style/template/title) through the in-head handler.
func (tb *TreeBuilder) processStartTagInHead(tok *Token) {
	saved := tb.insertionMode
	tb.insertionMode = modeInHead
	tb.startTagInHead(tok)
	tb.insertionMode = saved
}

// endTagInBody handles an end-tag token in the "in body" insertion mode.
func (tb *TreeBuilder) endTagInBody(tok *Token) {
	name := tok.Data
	switch {
	case name == "body":
		if tb.openElements.inScope("body") {
			tb.insertionMode = modeAfterBody
		}
		return
	case name == "html":
		if tb.openElements.inScope("body") {
			tb.insertionMode = modeAfterBody
			tb.processEndTag(tok)
		}
		return
	case name == "address" || name == "article" || name == "aside" ||
		name == "blockquote" || name == "button" || name == "center" ||
		name == "details" || name == "dialog" || name == "dir" ||
		name == "div" || name == "dl" || name == "fieldset" ||
		name == "figcaption" || name == "figure" || name == "footer" ||
		name == "header" || name == "hgroup" || name == "listing" ||
		name == "main" || name == "menu" || name == "nav" ||
		name == "ol" || name == "pre" || name == "search" ||
		name == "section" || name == "summary" || name == "ul":
		if tb.openElements.inScope(name) {
			tb.generateImpliedEndTags("")
			tb.openElements.popUntil(name)
		}
		return
	case name == "form":
		if tb.openElements.inScope(name) {
			tb.generateImpliedEndTags("")
			tb.openElements.popUntil(name)
			tb.formElement = nil
		}
		return
	case name == "p":
		if tb.openElements.inButtonScope("p") {
			tb.closePInButtonScope()
		} else {
			// Insert an empty <p> then close it.
			p := tb.doc.CreateElement("p")
			tb.attachNode(p)
			tb.openElements.push(p)
			tb.closePInButtonScope()
		}
		return
	case name == "li":
		if tb.openElements.inListItemScope("li") {
			tb.generateImpliedEndTagsExceptList()
			tb.openElements.popUntil("li")
		}
		return
	case name == "dd" || name == "dt":
		if tb.openElements.inListItemScope(name) {
			tb.generateImpliedEndTagsExceptDdDt()
			tb.openElements.popUntil(name)
		}
		return
	case isHeader(name):
		// </h1>..</h6> closes any header in scope.
		if tb.openElements.hasNumberedHeaderInScope() {
			tb.generateImpliedEndTags("")
			// Pop until a header element is popped.
			for tb.openElements.size() > 0 {
				top := tb.openElements.pop()
				if isHeader(top.LocalName()) {
					break
				}
			}
		}
		return
	case name == "applet" || name == "marquee" || name == "object":
		if tb.openElements.inScope(name) {
			tb.generateImpliedEndTags("")
			tb.openElements.popUntil(name)
			tb.activeFormatting.clearToLastMarker()
		}
		return
	case name == "br":
		// Parse error: treat as <br>.
		tb.startTagInBody(&Token{Type: TokenStartTag, Data: "br"})
		return
	case name == "template":
		// Process as in head.
		saved := tb.insertionMode
		tb.insertionMode = modeInHead
		tb.endTagInHead(tok)
		tb.insertionMode = saved
		return
	case formattingElements[name] || name == "a":
		// Run the adoption agency algorithm for the formatting element.
		target := tb.openElements.topmost(name)
		if target != nil {
			tb.callTheAdoptionAgency(target)
		} else {
			// No element on stack: parse error, ignore.
		}
		return
	case name == "head" || name == "body" || name == "html" ||
		name == "br" || name == "p":
		// Already handled above; fall through to "any other end tag".
		fallthrough
	default:
		// Any other end tag: pop until we find a matching element or run out.
		for i := tb.openElements.size() - 1; i >= 0; i-- {
			el := tb.openElements.at(i)
			if el == nil {
				return
			}
			if el.LocalName() == name {
				tb.generateImpliedEndTags("")
				tb.openElements.popUntilElement(el)
				return
			}
			if isSpecial(el.LocalName()) {
				// Special element: parse error, ignore.
				return
			}
		}
		return
	}
}

// isSpecial reports whether name is a "special" element for the any-other-end-tag
// branch. This is a conservative subset of the spec's special list.
func isSpecial(name string) bool {
	switch name {
	case "address", "applet", "area", "article", "aside", "base", "basefont",
		"bgsound", "blockquote", "body", "br", "button", "caption", "center",
		"col", "colgroup", "dd", "details", "dialog", "dir", "div", "dl", "dt",
		"embed", "fieldset", "figcaption", "figure", "footer", "form",
		"frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head",
		"header", "hgroup", "hr", "html", "iframe", "img", "input", "li",
		"link", "listing", "main", "marquee", "menu", "meta", "nav", "noembed",
		"noframes", "noscript", "object", "ol", "p", "param", "plaintext",
		"pre", "script", "search", "section", "select", "source", "style",
		"table", "tbody", "td", "template", "textarea", "tfoot", "th",
		"thead", "title", "tr", "track", "ul", "wbr", "xmp":
		return true
	}
	return false
}

// processCharacterInBody handles a character token in the "in body" insertion
// mode, mirroring HTMLTreeBuilder::processCharacter for InBodyMode.
func (tb *TreeBuilder) processCharacterInBody(tok *Token) {
	if tok.Data == "" {
		return
	}
	data := tok.Data
	// Skip a leading LF when the flag is set (set by <pre>/<listing>/<textarea>).
	if tb.skipNextLF && len(data) > 0 && data[0] == '\n' {
		data = data[1:]
		tb.skipNextLF = false
	}
	tb.skipNextLF = false
	if data == "" {
		return
	}
	// Reconstruct active formatting for the first character.
	tb.reconstructActiveFormatting()
	// Insert the text.
	tb.insertText(data)
	// If the text contains non-whitespace, framesetOk becomes false.
	for _, r := range data {
		if !isWhitespace(r) {
			tb.framesetOk = false
			break
		}
	}
}

// processEOFInBody handles end of input in the in-body mode.
func (tb *TreeBuilder) processEOFInBody() {
	// Pop any remaining open elements.
	for tb.openElements.size() > 0 {
		tb.openElements.pop()
	}
}

// --- Adoption Agency Algorithm ------------------------------------------

// callTheAdoptionAgency runs the adoption agency algorithm for the given
// formatting element. It is the heart of the HTML5 tree builder's error
// recovery for mis-nested formatting elements like `<p><b>...</p></b>`.
func (tb *TreeBuilder) callTheAdoptionAgency(formattingElement *dom.Element) {
	if formattingElement == nil {
		return
	}

	// Step 1: If the formatting element is not in the active formatting list,
	// nothing to do.
	feIdx := tb.activeFormatting.find(formattingElement)
	if feIdx < 0 {
		// Not in the list: parse error, remove from stack if present.
		tb.openElements.remove(formattingElement)
		return
	}

	// Step 2: If the formatting element is the current node, pop it and return.
	if tb.openElements.top() == formattingElement {
		tb.openElements.pop()
		tb.activeFormatting.remove(formattingElement)
		return
	}

	// Step 3: Find the furthest block: the topmost (lowest-index) element on the
	// open element stack that is below the formatting element and is a "special"
	// element. This is the element that will be re-parented.
	furthestBlock := -1
	feStackIdx := tb.openElements.indexOf(formattingElement)
	if feStackIdx < 0 {
		return
	}
	for i := feStackIdx - 1; i >= 0; i-- {
		if isSpecial(tb.openElements.at(i).LocalName()) {
			furthestBlock = i
			break
		}
	}
	if furthestBlock < 0 {
		// No furthest block: pop until the formatting element is popped.
		for tb.openElements.size() > 0 {
			el := tb.openElements.pop()
			if el == formattingElement {
				break
			}
		}
		tb.activeFormatting.remove(formattingElement)
		return
	}

	// Step 4: commonAncestor is the element immediately above the formatting
	// element on the stack.
	var commonAncestor *dom.Element
	if feStackIdx+1 < tb.openElements.size() {
		commonAncestor = tb.openElements.at(feStackIdx + 1)
	}
	if commonAncestor == nil {
		commonAncestor = tb.openElements.htmlElement()
	}

	// Step 5: bookmark marks the position of the formatting element in the
	// active formatting list; it will be updated as nodes are inserted.
	bookmark := tb.activeFormatting.bookmarkFor(formattingElement)

	// Step 6: Inner loop. Walk from the furthest block towards the formatting
	// element, reconstructing the formatting element's children.
	nodeIdx := furthestBlock
	lastNode := tb.openElements.at(furthestBlock)
	const maxIterations = 8
	iterations := 0
	for {
		iterations++
		if iterations > maxIterations {
			break
		}
		// nodeIdx moves towards the formatting element (upwards in stack).
		nodeIdx--
		if nodeIdx < 0 {
			break
		}
		node := tb.openElements.at(nodeIdx)
		if node == nil {
			break
		}

		// If node is the formatting element, exit the inner loop.
		if node == formattingElement {
			break
		}

		// If node is not in the active formatting list, remove it from the stack
		// and continue.
		afIdx := tb.activeFormatting.find(node)
		if afIdx < 0 {
			tb.openElements.remove(node)
			continue
		}

		// Otherwise: replace node with a clone in the active formatting list.
		clone := tb.cloneElement(node)
		tb.activeFormatting.entries[afIdx] = clone
		// Insert the clone into the common ancestor (or lastNode).
		if lastNode != nil && lastNode.ParentNode() != nil {
			_ = lastNode.ParentNode().RemoveChild(lastNode)
		}
		if commonAncestor != nil {
			_ = commonAncestor.AppendChild(clone)
		}
		// Append lastNode to clone.
		if lastNode != nil && lastNode.ParentNode() != nil {
			_ = lastNode.ParentNode().RemoveChild(lastNode)
		}
		_ = clone.AppendChild(lastNode)
		// Update lastNode to the clone.
		lastNode = clone
		// Replace node on the stack with the clone.
		tb.openElements.items[nodeIdx] = clone
	}

	// Step 7: Insert whatever lastNode ended up being into the common ancestor.
	if lastNode != nil && lastNode.ParentNode() != nil {
		_ = lastNode.ParentNode().RemoveChild(lastNode)
	}
	if commonAncestor != nil {
		_ = commonAncestor.AppendChild(lastNode)
	}

	// Step 8: Create a new clone of the formatting element and adopt the
	// furthest block's children into it.
	clone := tb.cloneElement(formattingElement)
	furthestEl := tb.openElements.at(furthestBlock)
	if furthestEl != nil {
		// Move all children of furthestBlock to clone.
		for furthestEl.HasChildNodes() {
			child := furthestEl.FirstChild()
			_ = furthestEl.RemoveChild(child)
			_ = clone.AppendChild(child)
		}
		_ = furthestEl.AppendChild(clone)
	}

	// Step 9: Insert the clone into the active formatting list at the bookmark.
	tb.activeFormatting.insertAt(bookmark, clone)

	// Step 10: Insert the clone onto the open element stack immediately below
	// the furthest block.
	if furthestEl != nil {
		tb.openElements.insertAbove(clone, furthestEl)
	} else {
		tb.openElements.push(clone)
	}

	// Remove the original formatting element from the active formatting list
	// and the open element stack (it has been replaced by the clone).
	tb.activeFormatting.remove(formattingElement)
	tb.openElements.remove(formattingElement)
}
