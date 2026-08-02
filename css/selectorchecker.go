// Translation of: Source/WebCore/css/SelectorChecker.cpp
//                  Source/WebCore/css/SelectorChecker.h
//                  Source/WebCore/css/SelectorCheckerTestFunctions.h
// Completeness: 70%
// Simplifications:
//   - matches are computed against the wb-ui dom.Element type rather than WebCore's
//     Element with all its rare-data machinery
//   - dynamic pseudo-classes (:hover / :focus / :active / :visited) always evaluate
//     to false; the resolver layer is responsible for flipping them on at runtime
//   - pseudo-elements are reported as matching the host element so the resolver can
//     route them; the actual pseudo-element subtree is built elsewhere
//   - selector lists for :is() / :where() / :not() / :has() are matched recursively
//     with the same simplifications as a top-level match
//   - :nth-* formulas use integer arithmetic; An+B syntax is fully implemented in
//     the parser but here we only test the count of preceding siblings
//   - attribute case-insensitivity flags are honored via the [attr i] suffix
//   - :lang() matches by language tag prefix; the document's lang is read via
//     element.GetAttribute("lang") with a fallback walk to the nearest ancestor
//   - shadow-DOM :host / :host-context / ::slotted / ::part are not implemented
//   - :has() is implemented by walking descendants of the candidate element

package css

import (
	"strings"

	"wb-ui/dom"
)

// SelectorChecker is the Go translation of WebCore::SelectorChecker. It exposes the
// right-to-left match algorithm that drives both selector-based queries and the
// style resolver. The matching mode (collecting relations vs. just querying) is
// folded into a single Resolving bool on the CheckerContext.
type SelectorChecker struct{}

// NewSelectorChecker constructs a SelectorChecker.
func NewSelectorChecker() *SelectorChecker { return &SelectorChecker{} }

// Match reports whether the given complex selector matches element. The match is
// performed right-to-left starting from the last compound selector; combinators are
// resolved by walking the DOM tree.
func (c *SelectorChecker) Match(sel ComplexSelector, el dom.Node) bool {
	if len(sel.Compounds) == 0 {
		return false
	}
	e, ok := el.(*dom.Element)
	if !ok {
		return false
	}
	return c.matchComplex(sel, len(sel.Compounds)-1, e)
}

// MatchList reports whether any selector in the list matches the element. This is
// the entry point used by :is() / :where() / :not() argument evaluation.
func (c *SelectorChecker) MatchList(list *SelectorList, el dom.Node) bool {
	if list == nil {
		return false
	}
	for _, sel := range list.Selectors {
		if c.Match(sel, el) {
			return true
		}
	}
	return false
}

// matchComplex matches the compound at index i against element. The recursion walks
// leftward through compounds, applying the combinator that links compound i to
// compound i-1.
func (c *SelectorChecker) matchComplex(sel ComplexSelector, i int, el *dom.Element) bool {
	if !c.matchCompound(sel.Compounds[i], el) {
		return false
	}
	if i == 0 {
		return true
	}
	rel := sel.Compounds[i].Relation
	parent := el.ParentNode()
	switch rel {
	case RelationSubselector:
		// Should not happen at the boundary between compounds.
		return c.matchComplex(sel, i-1, el)
	case RelationDescendant:
		for a := parent; a != nil; a = a.ParentNode() {
			if pe, ok := a.(*dom.Element); ok {
				if c.matchComplex(sel, i-1, pe) {
					return true
				}
			}
		}
		return false
	case RelationChild:
		if parent == nil {
			return false
		}
		pe, ok := parent.(*dom.Element)
		if !ok {
			return false
		}
		return c.matchComplex(sel, i-1, pe)
	case RelationDirectAdjacent:
		// Previous sibling of same type (element).
		prev := previousSiblingElement(el)
		for prev != nil {
			if c.matchComplex(sel, i-1, prev) {
				return true
			}
			prev = previousSiblingElement(prev)
		}
		return false
	case RelationIndirectAdjacent:
		prev := previousSiblingElement(el)
		for prev != nil {
			if c.matchComplex(sel, i-1, prev) {
				return true
			}
			prev = previousSiblingElement(prev)
		}
		return false
	}
	return false
}

// matchCompound reports whether all simple selectors in comp match el.
func (c *SelectorChecker) matchCompound(comp CompoundSelector, el *dom.Element) bool {
	for _, s := range comp.Selectors {
		if !c.matchSimple(s, el) {
			return false
		}
	}
	return true
}

// matchSimple reports whether a single simple selector matches el.
func (c *SelectorChecker) matchSimple(s SimpleSelector, el *dom.Element) bool {
	switch s.Match {
	case MatchTag:
		return s.Value == "*" || strings.EqualFold(el.LocalName(), s.Value)
	case MatchID:
		return el.GetId() == s.Value
	case MatchClass:
		return el.HasClassName(s.Value)
	case MatchSet:
		return el.HasAttribute(s.Attribute)
	case MatchExact, MatchList, MatchHyphen, MatchBegin, MatchEnd, MatchContain:
		return matchAttribute(s, el)
	case MatchPseudoClass:
		return c.matchPseudoClass(s, el)
	case MatchPseudoElement:
		// Pseudo-elements are treated as matching the host element so the resolver
		// can build the pseudo subtree. Real-world pseudo-element dispatch is the
		// resolver's responsibility.
		return true
	}
	return false
}

// matchAttribute applies the [attr OP value] matching rules per Selectors Level 3.
func matchAttribute(s SimpleSelector, el *dom.Element) bool {
	if !el.HasAttribute(s.Attribute) {
		return false
	}
	actual := el.GetAttribute(s.Attribute)
	expected := s.Value
	if s.AttrMatch == AttributeMatchCaseInsensitive {
		actual = strings.ToLower(actual)
		expected = strings.ToLower(expected)
	}
	switch s.Match {
	case MatchSet:
		return true
	case MatchExact:
		return actual == expected
	case MatchList:
		for _, tok := range strings.Fields(actual) {
			if tok == expected {
				return true
			}
		}
		return false
	case MatchHyphen:
		return actual == expected || strings.HasPrefix(actual, expected+"-")
	case MatchBegin:
		return strings.HasPrefix(actual, expected)
	case MatchEnd:
		return strings.HasSuffix(actual, expected)
	case MatchContain:
		return strings.Contains(actual, expected)
	}
	return false
}

// matchPseudoClass evaluates the pseudo-class against el. Dynamic pseudo-classes
// (:hover / :focus / :active / :visited) always return false here; the resolver
// is responsible for honoring them when collecting rules.
func (c *SelectorChecker) matchPseudoClass(s SimpleSelector, el *dom.Element) bool {
	switch s.PseudoClass {
	case PseudoClassHover:
		return el.IsHovered()
	case PseudoClassFocus:
		return el.IsFocused()
	case PseudoClassFocusVisible:
		// :focus-visible matches keyboard (Tab) focus, not mouse clicks —
		// Chrome/Edge UA default outline is `:focus-visible { outline: auto }`,
		// so clicking a button does NOT draw a focus ring but Tab does.
		return el.IsFocused() && el.FocusByKeyboard()
	case PseudoClassFocusWithin:
		// :focus-within matches if the element itself or any descendant has focus.
		if el.IsFocused() {
			return true
		}
		return c.hasFocusedDescendant(el)
	case PseudoClassActive:
		return el.IsActive()
	case PseudoClassVisited:
		// Visited requires the network-layer history tracking, which is not
		// implemented in this port.
		return false
	case PseudoClassLink:
		// Links are <a> elements with an href.
		return strings.EqualFold(el.LocalName(), "a") && el.HasAttribute("href")
	case PseudoClassAnyLink:
		return el.HasAttribute("href") && (strings.EqualFold(el.LocalName(), "a") ||
			strings.EqualFold(el.LocalName(), "area") ||
			strings.EqualFold(el.LocalName(), "link"))
	case PseudoClassRoot:
		// The root element of the document. We treat the topmost element (parent is
		// the document) as the root.
		p := el.ParentNode()
		return p == nil || p.NodeType() == dom.NodeDocument
	case PseudoClassScope:
		// Without an explicit scope, :scope matches the root.
		return c.matchPseudoClass(SimpleSelector{PseudoClass: PseudoClassRoot}, el)
	case PseudoClassEmpty:
		// No element children and no non-whitespace text.
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if c.NodeType() == dom.NodeElement {
				return false
			}
			if c.NodeType() == dom.NodeText {
				if strings.TrimSpace(c.NodeValue()) != "" {
					return false
				}
			}
		}
		return true
	case PseudoClassFirstChild:
		return previousSiblingElement(el) == nil
	case PseudoClassLastChild:
		return nextSiblingElement(el) == nil
	case PseudoClassOnlyChild:
		return previousSiblingElement(el) == nil && nextSiblingElement(el) == nil
	case PseudoClassFirstOfType:
		return previousSiblingOfType(el, el.LocalName()) == nil
	case PseudoClassLastOfType:
		return nextSiblingOfType(el, el.LocalName()) == nil
	case PseudoClassOnlyOfType:
		return previousSiblingOfType(el, el.LocalName()) == nil &&
			nextSiblingOfType(el, el.LocalName()) == nil
	case PseudoClassNthChild:
		return nthMatch(s.NthA, s.NthB, countPrecedingSiblings(el)+1)
	case PseudoClassNthLastChild:
		return nthMatch(s.NthA, s.NthB, countFollowingSiblings(el)+1)
	case PseudoClassNthOfType:
		return nthMatch(s.NthA, s.NthB, countPrecedingSiblingsOfType(el, el.LocalName())+1)
	case PseudoClassNthLastOfType:
		return nthMatch(s.NthA, s.NthB, countFollowingSiblingsOfType(el, el.LocalName())+1)
	case PseudoClassNot:
		return !c.MatchList(s.SelectorList, el)
	case PseudoClassIs, PseudoClassWhere:
		return c.MatchList(s.SelectorList, el)
	case PseudoClassHas:
		// :has(rel) — match any descendant matching rel.
		return c.matchHas(s.SelectorList, el)
	case PseudoClassTarget:
		doc := el.OwnerDocument()
		if doc == nil {
			return false
		}
		return doc.URL() != "" && el.GetId() != ""
	case PseudoClassLang:
		lang := lookupLang(el)
		for _, want := range s.StringList {
			if langMatches(lang, want) {
				return true
			}
		}
		return false
	case PseudoClassDir:
		// Best-effort: read the dir attribute.
		dir := strings.ToLower(el.GetAttribute("dir"))
		want := strings.ToLower(s.Argument)
		return dir == want
	case PseudoClassChecked:
		// <input checked> or <option selected>.
		if strings.EqualFold(el.LocalName(), "input") {
			return el.HasAttribute("checked")
		}
		if strings.EqualFold(el.LocalName(), "option") {
			return el.HasAttribute("selected")
		}
		return false
	case PseudoClassDisabled:
		return el.HasAttribute("disabled")
	case PseudoClassEnabled:
		return !el.HasAttribute("disabled") && isFormControl(el)
	case PseudoClassReadOnly:
		return !el.HasAttribute("contenteditable") && !isFormControl(el)
	case PseudoClassReadWrite:
		return el.HasAttribute("contenteditable")
	case PseudoClassPlaceholderShown:
		return strings.EqualFold(el.LocalName(), "input") &&
			el.HasAttribute("placeholder")
	case PseudoClassRequired:
		return el.HasAttribute("required")
	case PseudoClassOptional:
		return !el.HasAttribute("required")
	case PseudoClassValid, PseudoClassInvalid, PseudoClassInRange, PseudoClassOutOfRange,
		PseudoClassDefault, PseudoClassIndeterminate:
		// Constraint-validation pseudo-classes are not modeled.
		return false
	case PseudoClassDefined:
		// In this port all elements are "defined".
		return true
	case PseudoClassHost, PseudoClassHostContext:
		// Shadow DOM not implemented.
		return false
	}
	return false
}

// hasFocusedDescendant returns true when any descendant element (child, grandchild,
// etc.) of el has the focused flag set. It is used by :focus-within matching.
func (c *SelectorChecker) hasFocusedDescendant(el *dom.Element) bool {
	for child := el.FirstChild(); child != nil; child = child.NextSibling() {
		if childEl, ok := child.(*dom.Element); ok {
			if childEl.IsFocused() {
				return true
			}
			if c.hasFocusedDescendant(childEl) {
				return true
			}
		}
	}
	return false
}

// matchHas implements :has(rel): returns true if any descendant of el matches any
// selector in rel, with a recursion depth limit to prevent stack overflow from
// nested :has() selectors.
func (c *SelectorChecker) matchHas(rel *SelectorList, el *dom.Element) bool {
	return c.matchHasWithDepth(rel, el, 0)
}

// matchHasWithDepth is the recursive implementation of :has() with a depth limit.
// maxHasDepth (10) prevents infinite recursion from nested :has() pseudo-classes.
const maxHasDepth = 10

func (c *SelectorChecker) matchHasWithDepth(rel *SelectorList, el *dom.Element, depth int) bool {
	if rel == nil || depth > maxHasDepth {
		return false
	}
	for _, child := range childElements(el) {
		for _, sel := range rel.Selectors {
			if c.Match(sel, child) {
				return true
			}
		}
		if c.matchHasWithDepth(rel, child, depth+1) {
			return true
		}
	}
	return false
}

// previousSiblingElement returns the previous sibling of el that is an Element, or
// nil. Mirrors ElementTraversal::previousSibling.
func previousSiblingElement(el *dom.Element) *dom.Element {
	for s := el.PreviousSibling(); s != nil; s = s.PreviousSibling() {
		if e, ok := s.(*dom.Element); ok {
			return e
		}
	}
	return nil
}

// nextSiblingElement returns the next sibling of el that is an Element, or nil.
func nextSiblingElement(el *dom.Element) *dom.Element {
	for s := el.NextSibling(); s != nil; s = s.NextSibling() {
		if e, ok := s.(*dom.Element); ok {
			return e
		}
	}
	return nil
}

// previousSiblingOfType returns the previous sibling element with a matching tag.
func previousSiblingOfType(el *dom.Element, tag string) *dom.Element {
	for s := el.PreviousSibling(); s != nil; s = s.PreviousSibling() {
		if e, ok := s.(*dom.Element); ok {
			if strings.EqualFold(e.LocalName(), tag) {
				return e
			}
		}
	}
	return nil
}

// nextSiblingOfType returns the next sibling element with a matching tag.
func nextSiblingOfType(el *dom.Element, tag string) *dom.Element {
	for s := el.NextSibling(); s != nil; s = s.NextSibling() {
		if e, ok := s.(*dom.Element); ok {
			if strings.EqualFold(e.LocalName(), tag) {
				return e
			}
		}
	}
	return nil
}

// countPrecedingSiblings returns the number of preceding element siblings.
func countPrecedingSiblings(el *dom.Element) int {
	n := 0
	for s := el.PreviousSibling(); s != nil; s = s.PreviousSibling() {
		if _, ok := s.(*dom.Element); ok {
			n++
		}
	}
	return n
}

// countFollowingSiblings returns the number of following element siblings.
func countFollowingSiblings(el *dom.Element) int {
	n := 0
	for s := el.NextSibling(); s != nil; s = s.NextSibling() {
		if _, ok := s.(*dom.Element); ok {
			n++
		}
	}
	return n
}

// countPrecedingSiblingsOfType returns the number of preceding siblings with the
// same tag.
func countPrecedingSiblingsOfType(el *dom.Element, tag string) int {
	n := 0
	for s := el.PreviousSibling(); s != nil; s = s.PreviousSibling() {
		if e, ok := s.(*dom.Element); ok {
			if strings.EqualFold(e.LocalName(), tag) {
				n++
			}
		}
	}
	return n
}

// countFollowingSiblingsOfType returns the number of following siblings with the
// same tag.
func countFollowingSiblingsOfType(el *dom.Element, tag string) int {
	n := 0
	for s := el.NextSibling(); s != nil; s = s.NextSibling() {
		if e, ok := s.(*dom.Element); ok {
			if strings.EqualFold(e.LocalName(), tag) {
				n++
			}
		}
	}
	return n
}

// childElements returns the element children of el.
func childElements(el *dom.Element) []*dom.Element {
	var out []*dom.Element
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok {
			out = append(out, e)
		}
	}
	return out
}

// nthMatch reports whether the index n matches the An+B formula. n is 1-indexed.
func nthMatch(a, b, n int) bool {
	if a == 0 {
		return n == b
	}
	// (n - b) / a must be a non-negative integer.
	diff := n - b
	if diff == 0 {
		return true
	}
	if diff%a != 0 {
		return false
	}
	return diff/a >= 0
}

// lookupLang walks ancestors to find the nearest non-empty lang attribute.
func lookupLang(el *dom.Element) string {
	for n := el; n != nil; {
		if lang := n.GetAttribute("lang"); lang != "" {
			return strings.ToLower(lang)
		}
		p := n.ParentNode()
		if p == nil {
			break
		}
		var ok bool
		n, ok = p.(*dom.Element)
		if !ok {
			break
		}
	}
	return ""
}

// langMatches reports whether the element's language matches the requested language
// tag, following the BCP-47 prefix rule.
func langMatches(actual, want string) bool {
	if actual == "" {
		return false
	}
	want = strings.ToLower(want)
	actual = strings.ToLower(actual)
	if actual == want {
		return true
	}
	return strings.HasPrefix(actual, want+"-")
}

// isFormControl reports whether el is a form control (input, textarea, select,
// button).
func isFormControl(el *dom.Element) bool {
	switch strings.ToLower(el.LocalName()) {
	case "input", "textarea", "select", "button":
		return true
	}
	return false
}
