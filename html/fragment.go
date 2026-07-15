// Translation of: Source/WebCore/html/parser/HTMLDocumentParser.cpp (parseFragment)
//                  Source/WebCore/html/parser/HTMLTreeBuilder.cpp (setFragmentContext)
// Completeness: 90%
// Simplifications:
//   - fragment parsing uses a synthetic document as the owner; the context
//     element is recorded on the tree builder so resetInsertionModeAppropriately
//     uses it as the stack root
//   - the fragment is parsed as if it were the content of the context element;
//     no script execution or preload scanning is performed
//   - the returned nodes are the children of a temporary fragment, detached
//     from the synthetic document so they can be adopted by the caller's document

package html

import (
	"errors"

	"wb-ui/dom"
)

// ParseFragment parses an HTML fragment in the context of parent and returns
// the resulting nodes. It is the Go translation of
// HTMLDocumentParser::parseFragment. The returned nodes are detached from their
// creation document and ready to be adopted into the caller's document.
//
// If parent is nil the fragment is parsed as if the parent were a div, matching
// the behavior of Element::setInnerHTML.
func ParseFragment(src string, parent *dom.Element) ([]dom.Node, error) {
	if src == "" {
		return nil, nil
	}
	if parent == nil {
		// Default context element is a div when none is provided.
		synthetic := dom.NewDocument()
		parent = synthetic.CreateElement("div")
	}

	// Create a synthetic document to own the parsing process.
	doc := dom.NewDocument()
	// The context element must be owned by the synthetic document so nodes
	// created during parsing are correctly owned.
	_ = doc.AppendChild(parent)

	tok := NewTokenizer(src)
	tb := NewTreeBuilder(doc, tok)
	tb.fragmentParsing = true
	tb.contextElement = parent

	// Push the context element as the root of the open element stack.
	tb.openElements.push(parent)
	tb.resetInsertionModeAppropriately()

	for {
		t := tok.NextToken()
		if t == nil {
			break
		}
		tb.ConstructTree(t)
		if t.Type == TokenEOF {
			break
		}
	}
	tb.Finished()

	// Collect the children of the context element; these are the parsed nodes.
	var out []dom.Node
	for c := parent.FirstChild(); c != nil; {
		next := c.NextSibling()
		_ = parent.RemoveChild(c)
		out = append(out, c)
		c = next
	}
	if len(out) == 0 {
		return nil, errors.New("html: no nodes parsed")
	}
	return out, nil
}

// ParseFragmentIntoDocument parses an HTML fragment using the provided document
// as the owner, returning the resulting nodes. This matches the signature used
// by Element::setInnerHTML when the owning document is already known.
func ParseFragmentIntoDocument(src string, doc *dom.Document, parent *dom.Element) ([]dom.Node, error) {
	if src == "" {
		return nil, nil
	}
	if doc == nil {
		return nil, errors.New("html: nil document")
	}
	if parent == nil {
		parent = doc.CreateElement("div")
	}

	_ = doc.AppendChild(parent)
	tok := NewTokenizer(src)
	tb := NewTreeBuilder(doc, tok)
	tb.fragmentParsing = true
	tb.contextElement = parent
	tb.openElements.push(parent)
	tb.resetInsertionModeAppropriately()

	for {
		t := tok.NextToken()
		if t == nil {
			break
		}
		tb.ConstructTree(t)
		if t.Type == TokenEOF {
			break
		}
	}
	tb.Finished()

	var out []dom.Node
	for c := parent.FirstChild(); c != nil; {
		next := c.NextSibling()
		_ = parent.RemoveChild(c)
		out = append(out, c)
		c = next
	}
	_ = doc.RemoveChild(parent)
	return out, nil
}
