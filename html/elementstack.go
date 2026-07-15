// Translation of: Source/WebCore/html/parser/HTMLElementStack.cpp
//                  Source/WebCore/html/parser/HTMLElementStack.h
// Completeness: 70%
// Simplifications:
//   - ElementRecord is folded into a plain *dom.Element slice entry
//   - HTMLStackItem is replaced by *dom.Element (attributes are read live)
//   - the root/html/head/body cached pointers are resolved by scanning the stack
//   - the foreign-content scope marker helpers are omitted (foreign content uses a
//     namespace flag on the element instead)
//   - template element count is tracked by scanning rather than a counter

package html

import (
	"strings"

	"wb-ui/dom"
)

// elementStack is the Go translation of WebCore::HTMLElementStack. It holds the
// "stack of open elements" used by the tree builder to track the current
// insertion point and to resolve scope queries.
type elementStack struct {
	items []*dom.Element
}

// push pushes an element onto the top of the stack.
func (s *elementStack) push(e *dom.Element) { s.items = append(s.items, e) }

// pop removes and returns the top element. It panics if the stack is empty; the
// tree builder never pops an empty stack in valid states.
func (s *elementStack) pop() *dom.Element {
	n := len(s.items)
	if n == 0 {
		return nil
	}
	e := s.items[n-1]
	s.items = s.items[:n-1]
	return e
}

// top returns the top element (the current node) or nil if the stack is empty.
func (s *elementStack) top() *dom.Element {
	if len(s.items) == 0 {
		return nil
	}
	return s.items[len(s.items)-1]
}

// oneBelowTop returns the element just below the top, mirroring oneBelowTop().
func (s *elementStack) oneBelowTop() *dom.Element {
	if len(s.items) < 2 {
		return nil
	}
	return s.items[len(s.items)-2]
}

// size returns the stack depth.
func (s *elementStack) size() int { return len(s.items) }

// contains reports whether e is on the stack.
func (s *elementStack) contains(e *dom.Element) bool {
	for _, x := range s.items {
		if x == e {
			return true
		}
	}
	return false
}

// has reports whether an element with the given (lower-case) tag name is on the stack.
func (s *elementStack) has(name string) bool {
	for _, x := range s.items {
		if x.LocalName() == name {
			return true
		}
	}
	return false
}

// topmost returns the topmost element with the given tag name, or nil.
func (s *elementStack) topmost(name string) *dom.Element {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].LocalName() == name {
			return s.items[i]
		}
	}
	return nil
}

// popUntil pops elements until (and including) the first element whose tag name
// matches name. If no such element exists, the stack is unchanged.
func (s *elementStack) popUntil(name string) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].LocalName() == name {
			s.items = s.items[:i]
			return true
		}
	}
	return false
}

// popUntilElement pops until (and including) the given element.
func (s *elementStack) popUntilElement(e *dom.Element) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i] == e {
			s.items = s.items[:i]
			return true
		}
	}
	return false
}

// remove removes e from the stack without touching elements above it.
func (s *elementStack) remove(e *dom.Element) {
	for i, x := range s.items {
		if x == e {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return
		}
	}
}

// insertAbove inserts e immediately above target on the stack, mirroring
// HTMLElementStack::insertAbove.
func (s *elementStack) insertAbove(e, target *dom.Element) {
	for i, x := range s.items {
		if x == target {
			s.items = append(s.items[:i+1], s.items[i:]...)
			s.items[i+1] = e
			return
		}
	}
	// target not found: just push.
	s.push(e)
}

// indexOf returns the index of e on the stack, or -1.
func (s *elementStack) indexOf(e *dom.Element) int {
	for i, x := range s.items {
		if x == e {
			return i
		}
	}
	return -1
}

// at returns the element at stack index i (0 = bottom).
func (s *elementStack) at(i int) *dom.Element {
	if i < 0 || i >= len(s.items) {
		return nil
	}
	return s.items[i]
}

// scopeMarkers lists the tag names that bound the "default" scope. The spec's
// scope is bounded by applet/caption/html/table/td/th/marquee/object/template
// plus the MathML/SVG integration points. The SVG title element is also a
// boundary, but since this port collapses namespaces (HTML <title> and SVG
// <title> share a tag name) we omit it to keep HTML <title> out of scope
// boundaries.
var scopeMarkers = map[string]bool{
	"applet": true, "caption": true, "html": true, "table": true,
	"td": true, "th": true, "marquee": true, "object": true, "template": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true,
	"annotation-xml": true, "foreignobject": true, "desc": true,
}

// inScope reports whether an element with the given tag name is in scope,
// mirroring HTMLElementStack::inScope(ElementName). The default scope is bounded
// by the scope markers.
func (s *elementStack) inScope(name string) bool {
	return s.inScopeWith(name, nil)
}

// inScopeWith reports whether name is in scope. extraMarkers lists additional tag
// names that bound the scope (e.g. "li","ol","ul" for list-item scope).
func (s *elementStack) inScopeWith(name string, extraMarkers map[string]bool) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		cur := s.items[i].LocalName()
		if cur == name {
			return true
		}
		if scopeMarkers[cur] || (extraMarkers != nil && extraMarkers[cur]) {
			return false
		}
	}
	return false
}

// inListItemScope reports whether name is in list-item scope (scope bounded by
// the default markers plus ol/ul).
func (s *elementStack) inListItemScope(name string) bool {
	return s.inScopeWith(name, map[string]bool{"ol": true, "ul": true})
}

// inButtonScope reports whether name is in button scope (default markers + button).
func (s *elementStack) inButtonScope(name string) bool {
	return s.inScopeWith(name, map[string]bool{"button": true})
}

// inTableScope reports whether name is in table scope (bounded by table/template/html).
func (s *elementStack) inTableScope(name string) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		cur := s.items[i].LocalName()
		switch cur {
		case name:
			return true
		case "table", "template", "html":
			return false
		}
	}
	return false
}

// inSelectScope reports whether name is in select scope. Select scope is the
// inverse of the others: it returns true unless it hits an element other than
// optgroup/option.
func (s *elementStack) inSelectScope(name string) bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		cur := s.items[i].LocalName()
		if cur == name {
			return true
		}
		if cur != "optgroup" && cur != "option" {
			return false
		}
	}
	return false
}

// clearBackToTableContext pops elements until a table context (table/template/html)
// is the top, mirroring "clear the stack back to a table context".
func (s *elementStack) clearBackToTableContext() {
	for len(s.items) > 0 {
		cur := s.top().LocalName()
		if cur == "table" || cur == "template" || cur == "html" {
			return
		}
		s.pop()
	}
}

// clearBackToTableBodyContext pops until tbody/tfoot/thead/template/html.
func (s *elementStack) clearBackToTableBodyContext() {
	for len(s.items) > 0 {
		cur := s.top().LocalName()
		if cur == "tbody" || cur == "tfoot" || cur == "thead" ||
			cur == "template" || cur == "html" {
			return
		}
		s.pop()
	}
}

// clearBackToTableRowContext pops until tr/template/html.
func (s *elementStack) clearBackToTableRowContext() {
	for len(s.items) > 0 {
		cur := s.top().LocalName()
		if cur == "tr" || cur == "template" || cur == "html" {
			return
		}
		s.pop()
	}
}

// hasNumberedHeaderInScope reports whether a numbered heading (h1-h6) is in scope.
func (s *elementStack) hasNumberedHeaderInScope() bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		cur := s.items[i].LocalName()
		if isHeader(cur) {
			return true
		}
		if scopeMarkers[cur] {
			return false
		}
	}
	return false
}

// htmlElement / headElement / bodyElement resolve the named elements by scanning.
func (s *elementStack) htmlElement() *dom.Element { return s.topmost("html") }
func (s *elementStack) headElement() *dom.Element { return s.topmost("head") }
func (s *elementStack) bodyElement() *dom.Element { return s.topmost("body") }

// isHeader reports whether name is h1..h6.
func isHeader(name string) bool {
	if len(name) != 2 || name[0] != 'h' {
		return false
	}
	return name[1] >= '1' && name[1] <= '6'
}

// debugString renders the stack bottom-to-top for diagnostics.
func (s *elementStack) debugString() string {
	var b strings.Builder
	b.WriteByte('[')
	for i, e := range s.items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(e.LocalName())
	}
	b.WriteByte(']')
	return b.String()
}
