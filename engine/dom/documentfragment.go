// Translation of: Source/WebCore/dom/DocumentFragment.h
//                  Source/WebCore/dom/DocumentFragment.cpp
// Completeness: 70%
// Simplifications:
//   - parseHTML is a small stack-based parser sufficient for well-formed fragments
//     (text, comments, open/close tags, self-closing tags, quoted attributes); it does
//     not implement the full HTML5 tree construction / adoption agency / foster parenting
//     (that belongs to Phase 4)
//   - ParserContentPolicy and template content are omitted

package dom

import (
	"strings"
)

// DocumentFragment is the Go translation of WebCore::DocumentFragment. It is a container
// node used to hold a detached subtree, e.g. as the result of Range.ExtractContents or
// as the parse target of Element.SetInnerHTML.
type DocumentFragment struct {
	nodeBase
}

// NewDocumentFragment creates a DocumentFragment owned by doc.
func NewDocumentFragment(doc *Document) *DocumentFragment {
	f := &DocumentFragment{}
	f.initNodeBase(f, doc, NodeDocumentFragment)
	return f
}

// NodeName returns "#document-fragment", mirroring DocumentFragment::nodeName().
func (f *DocumentFragment) NodeName() string { return "#document-fragment" }

// NodeValue returns the empty string, mirroring Node::nodeValue().
func (f *DocumentFragment) NodeValue() string { return "" }

// SetNodeValue has no effect, mirroring Node::setNodeValue().
func (f *DocumentFragment) SetNodeValue(string) error { return nil }

// cloneShallow returns an empty fragment, mirroring cloneNodeInternal.
func (f *DocumentFragment) cloneShallow(doc *Document) Node { return NewDocumentFragment(doc) }

// GetElementById returns the first descendant element whose id matches, mirroring
// NonElementParentNode::getElementById.
func (f *DocumentFragment) GetElementById(id string) *Element {
	var found *Element
	walkDescendantsNode(f, func(n Node) bool {
		if el, ok := n.(*Element); ok && el.GetId() == id {
			found = el
			return false
		}
		return true
	})
	return found
}

// parseFragment parses a small subset of HTML into a DocumentFragment owned by doc. It is
// the backend of Element.SetInnerHTML. The parser is stack-based and recursive: it
// handles text runs, comments, opening tags with attributes, closing tags and
// self-closing tags. Malformed input is tolerated by closing still-open elements at EOF.
func parseFragment(html string, doc *Document) *DocumentFragment {
	frag := NewDocumentFragment(doc)
	if strings.TrimSpace(html) == "" {
		// Even whitespace-only input still produces text children, so only short-circuit
		// the truly empty case.
		if html == "" {
			return frag
		}
	}
	p := &htmlParser{src: html, doc: doc}
	p.parse(frag)
	return frag
}

// htmlParser is a minimal recursive-descent HTML fragment parser.
type htmlParser struct {
	src string
	pos int
	doc *Document
}

// parse appends the parsed children to parent.
func (p *htmlParser) parse(parent Node) {
	for p.pos < len(p.src) {
		if p.startsWith("<!--") {
			if end := strings.Index(p.src[p.pos:], "-->"); end >= 0 {
				data := p.src[p.pos+4 : p.pos+end]
				parent.AppendChild(NewComment(p.doc, data))
				p.pos += end + 3
				continue
			}
		}
		if p.startsWith("<") {
			if p.startsWith("</") {
				// Closing tag: stop parsing children for the current element.
				return
			}
			if el, selfClose, ok := p.parseOpenTag(); ok {
				parent.AppendChild(el)
				if !selfClose && !isVoidElement(el) {
					p.parse(el)
					p.expectCloseTag(el.tag)
				}
				continue
			}
			// Not a recognisable tag: treat '<' as text.
		}
		// Text run up to the next '<'.
		next := strings.IndexByte(p.src[p.pos:], '<')
		if next < 0 {
			next = len(p.src) - p.pos
		}
		text := p.src[p.pos : p.pos+next]
		if text != "" {
			parent.AppendChild(NewText(p.doc, unescapeEntities(text)))
		}
		p.pos += next
	}
}

func (p *htmlParser) startsWith(s string) bool {
	return strings.HasPrefix(p.src[p.pos:], s)
}

// voidElements 是 HTML5 空元素集合（无内容、无闭合标签）。
// 解析时遇到 void 元素应视为立即自闭合：不递归解析子节点、不期待
// 闭合标签。否则 <input type="text"> 这类无 "/>" 的标签会把后续所有
// HTML 错误地当作其子节点嵌套（replaced 元素子节点被忽略 → 内容丢失、
// 兄弟节点结构错乱 → innerHTML 注入的布局整体崩坏）。
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

func isVoidElement(el *Element) bool {
	return voidElements[strings.ToLower(el.LocalName())]
}

// parseOpenTag parses a leading "<tag ...>" starting at p.pos (which points at '<').
// On success it returns the constructed element, whether it was self-closing, and true.
func (p *htmlParser) parseOpenTag() (*Element, bool, bool) {
	// p.src[p.pos] == '<'
	end := strings.IndexByte(p.src[p.pos:], '>')
	if end < 0 {
		return nil, false, false
	}
	inner := p.src[p.pos+1 : p.pos+end]
	p.pos += end + 1
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return nil, false, false
	}
	selfClose := strings.HasSuffix(inner, "/")
	if selfClose {
		inner = strings.TrimSpace(inner[:len(inner)-1])
	}
	// Split tag name and attributes.
	sp := strings.IndexAny(inner, " \t\n\r\f")
	var name, attrs string
	if sp < 0 {
		name = inner
	} else {
		name = inner[:sp]
		attrs = strings.TrimSpace(inner[sp:])
	}
	if name == "" {
		return nil, false, false
	}
	el := NewElement(p.doc, name)
	p.parseAttributes(attrs, el)
	return el, selfClose, true
}

// parseAttributes fills el's attributes from the attribute portion of a tag.
func (p *htmlParser) parseAttributes(s string, el *Element) {
	for s != "" {
		// Skip whitespace.
		for len(s) > 0 && isSpace(s[0]) {
			s = s[1:]
		}
		if s == "" {
			return
		}
		// Attribute name up to '=' or whitespace.
		i := 0
		for i < len(s) && s[i] != '=' && !isSpace(s[i]) {
			i++
		}
		name := s[:i]
		s = s[i:]
		// Skip whitespace.
		for len(s) > 0 && isSpace(s[0]) {
			s = s[1:]
		}
		value := ""
		if len(s) > 0 && s[0] == '=' {
			s = s[1:]
			for len(s) > 0 && isSpace(s[0]) {
				s = s[1:]
			}
			if len(s) > 0 && (s[0] == '"' || s[0] == '\'') {
				q := s[0]
				s = s[1:]
				j := strings.IndexByte(s, q)
				if j < 0 {
					value = s
					s = ""
				} else {
					value = s[:j]
					s = s[j+1:]
				}
			} else {
				j := 0
				for j < len(s) && !isSpace(s[j]) {
					j++
				}
				value = s[:j]
				s = s[j:]
			}
		}
		if name != "" {
			el.SetAttribute(name, unescapeEntities(value))
		}
	}
}

// expectCloseTag consumes a matching "</tag>" if present, mirroring the close-tag step
// of recursive parsing. A missing close tag is tolerated (the element was already
// appended), matching the EOF-recovery behaviour.
func (p *htmlParser) expectCloseTag(tag string) {
	if !p.startsWith("</") {
		return
	}
	end := strings.IndexByte(p.src[p.pos:], '>')
	if end < 0 {
		return
	}
	p.pos += end + 1
}

// isSpace reports whether b is HTML whitespace.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

// unescapeEntities expands the common HTML entities. Only the named entities needed for
// round-tripping text are supported; numeric references are decoded.
func unescapeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&apos;", "'",
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(s)
}
