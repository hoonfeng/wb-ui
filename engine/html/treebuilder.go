// Translation of: Source/WebCore/html/parser/HTMLTreeBuilder.cpp
//                  Source/WebCore/html/parser/HTMLTreeBuilder.h
//                  Source/WebCore/html/parser/HTMLConstructionSite.cpp
//                  Source/WebCore/html/parser/HTMLConstructionSite.h
// Completeness: 65%
// Simplifications:
//   - insertion modes dispatched via a switch; the foreign-content rules are
//     folded into the HTML modes (SVG/MathML namespace handling is approximate)
//   - HTMLConstructionSite is folded into the TreeBuilder (insert/attach helpers)
//   - script execution / custom element construction / preload scanning are omitted
//   - the template insertion mode stack is a plain slice
//   - setInnerHTML / DOMContentLoaded hooks are omitted

package html

import (
	"strings"

	"wb-ui/engine/dom"
)

// insertionMode mirrors HTMLTreeBuilder::InsertionMode.
type insertionMode uint8

// Insertion mode constants, mirroring HTMLTreeBuilder.h.
const (
	modeInitial insertionMode = iota
	modeBeforeHTML
	modeBeforeHead
	modeInHead
	modeInHeadNoscript
	modeAfterHead
	modeTemplateContents
	modeInBody
	modeText
	modeInTable
	modeInTableText
	modeInCaption
	modeInColumnGroup
	modeInTableBody
	modeInRow
	modeInCell
	modeInSelect
	modeInSelectInTable
	modeAfterBody
	modeInFrameset
	modeAfterFrameset
	modeAfterAfterBody
	modeAfterAfterFrameset
)

// voidElements lists the HTML void elements that cannot have content, mirroring
// the spec's "void elements" list. These are never pushed onto the open
// element stack and have no end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// formattingElements lists the HTML formatting elements tracked by the
// adoption agency algorithm: a, b, big, code, em, font, i, nobr, s, small,
// strike, strong, tt, u. The spec includes these in the active formatting list.
var formattingElements = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true,
	"font": true, "i": true, "nobr": true, "s": true, "small": true,
	"strike": true, "strong": true, "tt": true, "u": true,
}

// rawTextElements switch the tokenizer to RAWTEXT (textarea/title use RCDATA).
var rawTextElements = map[string]bool{
	"style": true, "xmp": true, "iframe": true, "noembed": true,
	"noframes": true, "noscript": true,
}

// rcdataElements switch the tokenizer to RCDATA.
var rcdataElements = map[string]bool{
	"textarea": true, "title": true,
}

// impliedEndTagElements are the elements for which "generate implied end tags"
// pops them from the open element stack.
var impliedEndTagElements = map[string]bool{
	"dd": true, "dt": true, "li": true, "optgroup": true, "option": true,
	"p": true, "rb": true, "rp": true, "rt": true, "rtc": true,
}

// TreeBuilder is the Go translation of WebCore::HTMLTreeBuilder. It consumes
// tokens from the tokenizer and builds a DOM tree, applying the HTML5 spec's
// insertion modes, adoption agency algorithm and foster parenting.
type TreeBuilder struct {
	doc *dom.Document

	// openElements is the stack of open elements.
	openElements elementStack

	// activeFormatting is the list of active formatting elements.
	activeFormatting formattingList

	// insertionMode is the current insertion mode.
	insertionMode insertionMode
	// originalInsertionMode is saved when entering the Text insertion mode.
	originalInsertionMode insertionMode
	// templateInsertionModes is the stack of template insertion modes.
	templateInsertionModes []insertionMode

	// headElement points at the head element, set when it is inserted.
	headElement *dom.Element
	// formElement points at the currently open form element (or nil).
	formElement *dom.Element
	// contextElement is the fragment parsing context (or nil for document parsing).
	contextElement *dom.Element

	// framesetOk is the "frameset-ok" flag from the spec.
	framesetOk bool

	// fosterParentingEnabled mirrors the foster parenting flag.
	fosterParentingEnabled bool

	// tokenizer references the tokenizer so the tree builder can switch its state
	// for RAWTEXT/RCDATA/script parsing.
	tokenizer *Tokenizer

	// fragmentParsing reports whether we are parsing a fragment.
	fragmentParsing bool

	// pendingText collects character data to insert (coalesced per the spec's
	// character token buffering in InBody/InTableText).
	pendingText     strings.Builder
	hasPendingText  bool

	// skipNextLF records that the next LF character should be ignored (set by
	// <pre>/<listing>/<textarea> start tags per the spec).
	skipNextLF bool

	// stopLi is a transient flag used by the <li> start-tag handler.
	stopLi bool
}

// NewTreeBuilder returns a TreeBuilder that builds into doc.
func NewTreeBuilder(doc *dom.Document, tok *Tokenizer) *TreeBuilder {
	tb := &TreeBuilder{doc: doc, tokenizer: tok, framesetOk: true}
	tb.insertionMode = modeInitial
	return tb
}

// ConstructTree consumes a token, mirroring HTMLTreeBuilder::constructTree.
func (tb *TreeBuilder) ConstructTree(tok *Token) {
	switch tok.Type {
	case TokenCharacter:
		tb.processCharacter(tok)
	case TokenComment:
		tb.processComment(tok)
	case TokenDoctype:
		tb.processDoctype(tok)
	case TokenStartTag:
		tb.processStartTag(tok)
	case TokenEndTag:
		tb.processEndTag(tok)
	case TokenEOF:
		tb.processEOF()
	}
}

// currentNode returns the top of the open element stack (the adjusted current
// node), or nil when the stack is empty.
func (tb *TreeBuilder) currentNode() *dom.Element {
	return tb.openElements.top()
}

// currentInsertionNode returns the node to which new content is appended. For
// document parsing this is the top of the open element stack; when the stack is
// empty it is the document.
func (tb *TreeBuilder) currentInsertionNode() dom.Node {
	if e := tb.openElements.top(); e != nil {
		return e
	}
	return tb.doc
}

// createElementForToken creates a dom.Element from a start-tag token, copying the
// token's attributes. The tag name is lower-cased by the tokenizer already.
// If a custom element constructor is registered for the tag name, it is called
// instead of the default doc.CreateElement.
func (tb *TreeBuilder) createElementForToken(tok *Token) *dom.Element {
	// Check custom element registry first.
	if ctor, ok := lookupCustomElement(tok.Data); ok {
		attrs := map[string]string{}
		for _, a := range tok.Attributes {
			if a.Name != "" {
				if _, exists := attrs[a.Name]; !exists {
					attrs[a.Name] = a.Value
				}
			}
		}
		return ctor(tb.doc, tok.Data, attrs)
	}
	// Fallback to default creation.
	e := tb.doc.CreateElement(tok.Data)
	for _, a := range tok.Attributes {
		if a.Name == "" {
			continue
		}
		// Per the spec, duplicate attribute names are dropped (first wins).
		if !e.HasAttribute(a.Name) {
			e.SetAttribute(a.Name, a.Value)
		}
	}
	return e
}

// attachNode attaches child to the current insertion location, applying foster
// parenting when enabled.
func (tb *TreeBuilder) attachNode(child dom.Node) {
	if tb.fosterParentingEnabled {
		tb.fosterParent(child)
		return
	}
	parent := tb.currentInsertionNode()
	_ = parent.AppendChild(child)
}

// insertElement creates an element for tok, attaches it and pushes it onto the
// open element stack. It returns the inserted element.
func (tb *TreeBuilder) insertElement(tok *Token) *dom.Element {
	e := tb.createElementForToken(tok)
	tb.attachNode(e)
	tb.openElements.push(e)
	return e
}

// insertSelfClosingElement inserts an element for a self-closing start tag and
// does not push it (void or self-closing handling).
func (tb *TreeBuilder) insertSelfClosingElement(tok *Token) {
	e := tb.createElementForToken(tok)
	tb.attachNode(e)
}

// insertText appends text data at the current insertion location, merging with
// an adjacent existing text node where possible.
func (tb *TreeBuilder) insertText(data string) {
	tb.insertTextAt(tb.currentInsertionNode(), data)
}

// insertTextAt appends data as a Text node under parent, merging with the last
// child when it is already a text node.
func (tb *TreeBuilder) insertTextAt(parent dom.Node, data string) {
	if data == "" || parent == nil {
		return
	}
	if last := parent.LastChild(); last != nil {
		if t, ok := last.(*dom.Text); ok {
			t.AppendData(data)
			return
		}
	}
	parent.AppendChild(tb.doc.CreateTextNode(data))
}

// insertCommentAt inserts a comment as the last child of target.
func (tb *TreeBuilder) insertCommentAt(target dom.Node, data string) {
	if target == nil {
		return
	}
	target.AppendChild(tb.doc.CreateComment(data))
}

// generateImpliedEndTags pops elements from the open element stack whose tag
// names are in the "generate implied end tags" list, except the named one.
// Mirrors the spec's "generate implied end tags" algorithm.
func (tb *TreeBuilder) generateImpliedEndTags(except string) {
	for tb.openElements.size() > 0 {
		name := tb.openElements.top().LocalName()
		if name == except || !impliedEndTagElements[name] {
			return
		}
		tb.openElements.pop()
	}
}

// closePInButtonScope pops a <p> element if one is in button scope, mirroring the
// spec's "if the stack of open elements has a p element in button scope, close
// the p element" step.
func (tb *TreeBuilder) closePInButtonScope() {
	if tb.openElements.inButtonScope("p") {
		tb.generateImpliedEndTags("p")
		tb.openElements.popUntil("p")
	}
}

// reconstructActiveFormatting reconstructs the active formatting elements,
// mirroring the spec's "reconstruct the active formatting elements" algorithm. It
// is called before inserting content in the in-body insertion mode.
func (tb *TreeBuilder) reconstructActiveFormatting() {
	if tb.activeFormatting.isEmpty() {
		return
	}
	last := tb.activeFormatting.size() - 1
	entry := tb.activeFormatting.entryAt(last)
	if entry == nil {
		// last entry is a marker: nothing to do.
		return
	}
	if tb.openElements.contains(entry) {
		return
	}
	// Walk back to the first entry that is either a marker, on the stack, or the
	// first entry.
	idx := last
	for idx > 0 {
		idx--
		e := tb.activeFormatting.entryAt(idx)
		if e == nil || tb.openElements.contains(e) {
			idx++ // start reconstruction at the next entry
			break
		}
	}
	for idx <= last {
		e := tb.activeFormatting.entryAt(idx)
		if e == nil {
			idx++
			continue
		}
		// Re-create the formatting element and insert at the current node.
		clone := tb.cloneElement(e)
		tb.attachNode(clone)
		tb.openElements.push(clone)
		// Replace the entry with the new clone.
		tb.activeFormatting.entries[idx] = clone
		idx++
	}
}

// cloneElement creates a copy of e (same tag + attributes) owned by the document.
func (tb *TreeBuilder) cloneElement(e *dom.Element) *dom.Element {
	c := tb.doc.CreateElement(e.TagName())
	for _, name := range e.AttributeNames() {
		c.SetAttribute(name, e.GetAttribute(name))
	}
	return c
}

// isFragmentParsing reports whether we are parsing a fragment.
func (tb *TreeBuilder) isFragmentParsing() bool { return tb.fragmentParsing }

// resetInsertionModeAppropriately mirrors the spec algorithm that picks the
// insertion mode based on the open element stack.
func (tb *TreeBuilder) resetInsertionModeAppropriately() {
	last := false
	for i := tb.openElements.size() - 1; i >= 0; i-- {
		node := tb.openElements.at(i)
		if i == 0 {
			last = true
			if tb.fragmentParsing {
				node = tb.contextElement
			}
		}
		name := node.LocalName()
		switch {
		case name == "select":
			tb.insertionMode = modeInSelect
			return
		case name == "td" || name == "th":
			if !last {
				tb.insertionMode = modeInCell
				return
			}
		case name == "tr":
			tb.insertionMode = modeInRow
			return
		case name == "tbody" || name == "thead" || name == "tfoot":
			tb.insertionMode = modeInTableBody
			return
		case name == "caption":
			tb.insertionMode = modeInCaption
			return
		case name == "colgroup":
			tb.insertionMode = modeInColumnGroup
			return
		case name == "table":
			tb.insertionMode = modeInTable
			return
		case name == "template":
			tb.insertionMode = tb.currentTemplateInsertionMode()
			return
		case name == "head":
			if !last {
				tb.insertionMode = modeInHead
				return
			}
		case name == "body":
			tb.insertionMode = modeInBody
			return
		case name == "frameset":
			tb.insertionMode = modeInFrameset
			return
		case name == "html":
			tb.insertionMode = modeBeforeHead
			return
		}
		if last {
			break
		}
	}
	tb.insertionMode = modeInBody
}

// currentTemplateInsertionMode returns the top of the template insertion mode
// stack, or modeInitial if empty.
func (tb *TreeBuilder) currentTemplateInsertionMode() insertionMode {
	if len(tb.templateInsertionModes) == 0 {
		return modeInitial
	}
	return tb.templateInsertionModes[len(tb.templateInsertionModes)-1]
}

// --- Dispatch to insertion modes ---------------------------------------

// processDoctype dispatches a DOCTYPE token based on the current insertion mode.
func (tb *TreeBuilder) processDoctype(tok *Token) {
	switch tb.insertionMode {
	case modeInitial:
		tb.handleDoctypeInitial(tok)
	default:
		// In other modes a DOCTYPE is a parse error and ignored.
	}
}

// processStartTag dispatches a start tag based on the current insertion mode.
func (tb *TreeBuilder) processStartTag(tok *Token) {
	switch tb.insertionMode {
	case modeInitial:
		tb.defaultForInitial()
		tb.processStartTag(tok)
	case modeBeforeHTML:
		tb.startTagBeforeHTML(tok)
	case modeBeforeHead:
		tb.startTagBeforeHead(tok)
	case modeInHead:
		tb.startTagInHead(tok)
	case modeInHeadNoscript:
		tb.startTagInHeadNoscript(tok)
	case modeAfterHead:
		tb.startTagAfterHead(tok)
	case modeInBody:
		tb.startTagInBody(tok)
	case modeText:
		tb.startTagInText(tok)
	case modeInTable:
		tb.startTagInTable(tok)
	case modeInTableText:
		tb.startTagInTableText(tok)
	case modeInCaption:
		tb.startTagInCaption(tok)
	case modeInColumnGroup:
		tb.startTagInColumnGroup(tok)
	case modeInTableBody:
		tb.startTagInTableBody(tok)
	case modeInRow:
		tb.startTagInRow(tok)
	case modeInCell:
		tb.startTagInCell(tok)
	case modeInSelect:
		tb.startTagInSelect(tok)
	case modeInSelectInTable:
		tb.startTagInSelectInTable(tok)
	case modeAfterBody:
		tb.startTagAfterBody(tok)
	case modeInFrameset:
		tb.startTagInFrameset(tok)
	case modeAfterFrameset:
		tb.startTagAfterFrameset(tok)
	case modeAfterAfterBody:
		tb.startTagAfterAfterBody(tok)
	case modeAfterAfterFrameset:
		tb.startTagAfterAfterFrameset(tok)
	case modeTemplateContents:
		tb.startTagTemplateContents(tok)
	}
}

// processEndTag dispatches an end tag based on the current insertion mode.
func (tb *TreeBuilder) processEndTag(tok *Token) {
	switch tb.insertionMode {
	case modeInitial:
		tb.defaultForInitial()
		tb.processEndTag(tok)
	case modeBeforeHTML:
		tb.endTagBeforeHTML(tok)
	case modeBeforeHead:
		tb.endTagBeforeHead(tok)
	case modeInHead:
		tb.endTagInHead(tok)
	case modeInHeadNoscript:
		tb.endTagInHeadNoscript(tok)
	case modeAfterHead:
		tb.endTagAfterHead(tok)
	case modeInBody:
		tb.endTagInBody(tok)
	case modeText:
		tb.endTagInText(tok)
	case modeInTable:
		tb.endTagInTable(tok)
	case modeInTableText:
		tb.endTagInTableText(tok)
	case modeInCaption:
		tb.endTagInCaption(tok)
	case modeInColumnGroup:
		tb.endTagInColumnGroup(tok)
	case modeInTableBody:
		tb.endTagInTableBody(tok)
	case modeInRow:
		tb.endTagInRow(tok)
	case modeInCell:
		tb.endTagInCell(tok)
	case modeInSelect:
		tb.endTagInSelect(tok)
	case modeInSelectInTable:
		tb.endTagInSelectInTable(tok)
	case modeAfterBody:
		tb.endTagAfterBody(tok)
	case modeInFrameset:
		tb.endTagInFrameset(tok)
	case modeAfterFrameset:
		tb.endTagAfterFrameset(tok)
	case modeAfterAfterBody:
		tb.endTagAfterAfterBody(tok)
	case modeTemplateContents:
		tb.endTagTemplateContents(tok)
	}
}

// processComment inserts a comment. The insertion location depends on the mode.
func (tb *TreeBuilder) processComment(tok *Token) {
	switch tb.insertionMode {
	case modeInitial, modeBeforeHTML, modeBeforeHead, modeAfterHead,
		modeAfterAfterBody, modeAfterAfterFrameset:
		tb.insertCommentAt(tb.doc, tok.Data)
	case modeInBody, modeInTable, modeInCaption, modeInTableBody, modeInRow,
		modeInCell, modeInColumnGroup, modeInFrameset, modeAfterFrameset,
		modeInSelect, modeInSelectInTable, modeInHeadNoscript, modeInHead,
		modeTemplateContents:
		tb.insertCommentAt(tb.currentInsertionNode(), tok.Data)
	case modeAfterBody:
		tb.insertCommentAt(tb.doc.DocumentElement(), tok.Data)
	case modeText, modeInTableText:
		// ignore
	}
}

// processCharacter handles character tokens. Most modes route through the in-body
// character handler (which reconstructs formatting and inserts text); the table
// modes buffer the characters for foster-parenting.
func (tb *TreeBuilder) processCharacter(tok *Token) {
	switch tb.insertionMode {
	case modeInTable, modeInCaption, modeInTableBody, modeInRow, modeInCell,
		modeInColumnGroup, modeInSelect, modeInSelectInTable:
		tb.processCharacterInTable(tok)
	case modeText:
		tb.insertText(tok.Data)
	case modeInTableText:
		tb.processCharacterInTableText(tok)
	case modeAfterBody:
		tb.insertionMode = modeInBody
		tb.processCharacter(tok)
	default:
		tb.processCharacterInBody(tok)
	}
}

// processEOF handles end of input by closing open elements and finishing.
func (tb *TreeBuilder) processEOF() {
	switch tb.insertionMode {
	case modeInTableText:
		tb.processEOFInTableText()
	case modeInitial:
		tb.defaultForInitial()
		tb.processEOF()
	case modeBeforeHTML:
		tb.defaultForBeforeHTML()
		tb.processEOF()
	case modeBeforeHead:
		tb.defaultForBeforeHead()
		tb.processEOF()
	case modeInHead:
		tb.defaultForInHead()
		tb.processEOF()
	case modeAfterHead:
		tb.defaultForAfterHead()
		tb.processEOF()
	case modeInHeadNoscript:
		tb.defaultForInHeadNoscript()
		tb.processEOF()
	case modeInTable, modeInCaption, modeInTableBody, modeInRow, modeInCell,
		modeInColumnGroup:
		tb.processEOFInTable()
	case modeInFrameset, modeAfterFrameset, modeAfterAfterFrameset:
		// parse error, ignore
	default:
		tb.processEOFInBody()
	}
}

// --- Tokenizer state switching -----------------------------------------

// setTokenizerStateFor switches the tokenizer state based on a tag name,
// mirroring the tree builder's update of the tokenizer for RAWTEXT/RCDATA/script.
func (tb *TreeBuilder) setTokenizerStateFor(tagName string) {
	if tb.tokenizer == nil {
		return
	}
	tb.tokenizer.appropriateEndTagName = tagName
	switch {
	case rcdataElements[tagName]:
		tb.tokenizer.setRCDATAState()
	case rawTextElements[tagName]:
		tb.tokenizer.setRAWTEXTState()
	case tagName == "script":
		tb.tokenizer.setScriptDataState()
	case tagName == "plaintext":
		tb.tokenizer.setPLAINTEXTState()
	default:
		tb.tokenizer.setDataState()
	}
}

// Finished signals end of parsing, mirroring HTMLTreeBuilder::finished.
func (tb *TreeBuilder) Finished() {
	// No-op: the DOM tree is already built; document finalization hooks are omitted.
}
