// Translation of: Source/WebCore/html/parser/HTMLDocumentParser.cpp
//                  Source/WebCore/html/parser/HTMLDocumentParser.h
// Completeness: 70%
// Simplifications:
//   - the parser is a simple driver that feeds tokens from the tokenizer into
//     the tree builder until EOF; there is no preload scanner, script execution,
//     or chunked parsing
//   - the XMLErrors/HTMLPreloadScanner/HTMLScriptRunner hooks are omitted
//   - the document is always created fresh; there is no incremental appending
//     to an existing partially-built document

package html

import (
	"errors"

	"wb-ui/dom"
)

// Parse parses an HTML document and returns the resulting Document. It is the
// Go translation of HTMLDocumentParser::parseDocument, combined with the
// document construction pipeline (tokenizer -> tree builder -> DOM).
//
// The input is treated as a complete HTML document: a DOCTYPE followed by html/
// head/body content. For fragment parsing use ParseFragment instead.
func Parse(src string) (*dom.Document, error) {
	if src == "" {
		return nil, errors.New("html: empty input")
	}
	doc := dom.NewDocument()
	tok := NewTokenizer(src)
	tb := NewTreeBuilder(doc, tok)
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
	return doc, nil
}

// ParseDocument is an alias for Parse that matches the naming used by other
// DOM implementations (document parsing entry point).
func ParseDocument(src string) (*dom.Document, error) {
	return Parse(src)
}

// ParseWithOptions parses an HTML document with the given options. Currently the
// only supported option is whether to enable quirks-mode handling (treated as a
// no-op in this port).
type ParseOptions struct {
	// QuirksMode controls whether quirks-mode behaviors are applied. The current
	// port does not implement quirks-mode differences; this field is accepted for
	// API compatibility.
	QuirksMode bool
}

// ParseWithOptions is an alias for Parse accepting parse options. The options are
// accepted but only the source string is used by this port.
func ParseWithOptions(src string, _ ParseOptions) (*dom.Document, error) {
	return Parse(src)
}
