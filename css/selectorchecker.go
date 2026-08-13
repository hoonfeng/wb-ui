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
//   - :host / :host-context match against the shadow host; ::slotted matches a
//     slot-assigned light-DOM node; ::part matches a shadow element by part name;
//     a host-selector prefix before ::part / ::slotted (e.g. x-widget::part(btn))
//     is forward-matched against the shadow host, including across a combinator
//     (.outer x-widget::part(btn) walks the host's composed ancestors); ::part also
//     follows the exportparts re-export chain across nested shadow trees (CSS Scoping L1)
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

	// Forward matching (CSS Scoping Level 1): when this compound ends in ::part or
	// ::slotted, its "composed position" is the shadow host, so the combinator to the
	// left crosses the shadow boundary and walks from the host — e.g. in
	// `.outer x-widget::part(btn)`, `.outer` matches an ancestor of the host, not of
	// the part element (which lives inside the shadow tree).
	base := el
	forward := false
	if pe, ok := c.forwardPseudoOf(sel.Compounds[i]); ok {
		if h := c.shadowHostFor(el, pe); h != nil {
			base = h
			forward = true
		}
	}
	parentOf := func(n dom.Node) dom.Node {
		if forward {
			return dom.ComposedParent(n)
		}
		return n.ParentNode()
	}

	parent := parentOf(base)
	switch rel {
	case RelationSubselector:
		// Should not happen at the boundary between compounds.
		return c.matchComplex(sel, i-1, el)
	case RelationDescendant:
		for a := parent; a != nil; a = parentOf(a) {
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
		prev := previousSiblingElement(base)
		for prev != nil {
			if c.matchComplex(sel, i-1, prev) {
				return true
			}
			prev = previousSiblingElement(prev)
		}
		return false
	case RelationIndirectAdjacent:
		prev := previousSiblingElement(base)
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

// matchCompound reports whether all simple selectors in comp match el. For a
// compound ending in ::part or ::slotted, the simple selectors BEFORE that
// pseudo-element match the shadow host instead of el (CSS Scoping Level 1 forward
// matching) — e.g. x-widget::part(btn): ::part(btn) matches the part element el,
// while x-widget matches el's shadow host.
func (c *SelectorChecker) matchCompound(comp CompoundSelector, el *dom.Element) bool {
	if n := len(comp.Selectors); n > 1 {
		last := comp.Selectors[n-1]
		if last.Match == MatchPseudoElement &&
			(last.PseudoElem == PseudoElementPart || last.PseudoElem == PseudoElementSlotted) {
			host := c.shadowHostFor(el, last.PseudoElem)
			if host == nil {
				return false
			}
			for _, s := range comp.Selectors[:n-1] {
				if !c.matchSimple(s, host) {
					return false
				}
			}
			return c.matchSimple(last, el)
		}
	}
	for _, s := range comp.Selectors {
		if !c.matchSimple(s, el) {
			return false
		}
	}
	return true
}

// shadowHostFor returns the shadow host that the host-selector prefix of a
// ::part / ::slotted compound must match against. For ::part it is the host of the
// shadow tree containing el; for ::slotted it is the host of the shadow tree
// containing the slot that assigns el.
func (c *SelectorChecker) shadowHostFor(el *dom.Element, pe PseudoElement) *dom.Element {
	switch pe {
	case PseudoElementPart:
		if sr := dom.ContainingShadowRoot(el); sr != nil {
			return sr.Host()
		}
	case PseudoElementSlotted:
		if slot := el.AssignedSlot(); slot != nil {
			if sr := dom.ContainingShadowRoot(slot); sr != nil {
				return sr.Host()
			}
		}
	}
	return nil
}

// forwardPseudoOf reports whether a compound selector ends in ::part or ::slotted
// (a forward-matching pseudo-element per CSS Scoping Level 1), returning the
// pseudo-element type. Such a compound matches the element, but the combinators to
// its left must cross the shadow boundary and walk from the element's shadow host.
func (c *SelectorChecker) forwardPseudoOf(comp CompoundSelector) (PseudoElement, bool) {
	if len(comp.Selectors) == 0 {
		return PseudoElementUnknown, false
	}
	last := comp.Selectors[len(comp.Selectors)-1]
	if last.Match != MatchPseudoElement {
		return PseudoElementUnknown, false
	}
	if last.PseudoElem == PseudoElementPart || last.PseudoElem == PseudoElementSlotted {
		return last.PseudoElem, true
	}
	return PseudoElementUnknown, false
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
		// ::slotted and ::part have real matching predicates; other pseudo-elements
		// are treated as matching the host element so the resolver can build the
		// pseudo subtree.
		switch s.PseudoElem {
		case PseudoElementSlotted:
			return c.matchSlotted(s, el)
		case PseudoElementPart:
			return c.matchPart(s, el)
		default:
			return true
		}
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
		// CSS :hover matches the element itself OR any descendant currently
		// under the cursor — hovering an <svg> icon inside a <button> must
		// light up the whole button (the hover state "bubbles" up the
		// ancestor chain in the browser). Without this, moving onto the icon
		// kills the button's hover background.
		if el.IsHovered() {
			return true
		}
		return c.hasHoveredDescendant(el)
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
	case PseudoClassHost:
		// :host matches the shadow host itself; :host(sel) additionally requires
		// the host to match the selector list.
		if !el.HasShadowRoot() {
			return false
		}
		if s.SelectorList != nil {
			return c.MatchList(s.SelectorList, el)
		}
		return true
	case PseudoClassHostContext:
		// :host-context(sel) matches the shadow host if the host itself or any
		// ancestor in the outer (document) tree matches sel.
		if !el.HasShadowRoot() {
			return false
		}
		if s.SelectorList == nil {
			return true
		}
		if c.MatchList(s.SelectorList, el) {
			return true
		}
		for a := el.ParentNode(); a != nil; a = a.ParentNode() {
			if pe, ok := a.(*dom.Element); ok {
				if c.MatchList(s.SelectorList, pe) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// hasFocusedDescendant returns true when any descendant element (child, grandchild,
// etc.) of el has the focused flag set. It is used by :focus-within matching.
// hasHoveredDescendant reports whether any descendant element is currently
// hovered — the counterpart to hasFocusedDescendant for :hover bubbling.
func (c *SelectorChecker) hasHoveredDescendant(el *dom.Element) bool {
	for child := el.FirstChild(); child != nil; child = child.NextSibling() {
		if childEl, ok := child.(*dom.Element); ok {
			if childEl.IsHovered() {
				return true
			}
			if c.hasHoveredDescendant(childEl) {
				return true
			}
		}
	}
	return false
}

func (c *SelectorChecker) hasFocusedDescendant(el *dom.Element) bool {	for child := el.FirstChild(); child != nil; child = child.NextSibling() {
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

// matchSlotted reports whether ::slotted matches el: a light-DOM node assigned to a
// slot in its shadow host's shadow tree. The optional argument (::slotted(sel)) must
// additionally match el. Mirrors SelectorCheckerTestFunctions for ::slotted.
func (c *SelectorChecker) matchSlotted(s SimpleSelector, el *dom.Element) bool {
	if el.AssignedSlot() == nil {
		return false
	}
	if s.SelectorList != nil {
		return c.MatchList(s.SelectorList, el)
	}
	return true
}

// matchPart reports whether ::part(name) matches el: a shadow-tree element whose
// `part` attribute lists one of the requested names, or whose part name is re-exported
// through one or more ancestor shadow hosts via their `exportparts` attribute
// (CSS Scoping Level 1 §4.5). exportparts maps a shadow-internal part name to one or
// more outer names (e.g. host exportparts="x: y, x: z" exposes the shadow part "x" as
// both "y" and "z"), and the re-export chain is followed layer by layer.
func (c *SelectorChecker) matchPart(s SimpleSelector, el *dom.Element) bool {
	if len(s.StringList) == 0 {
		return false
	}
	wants := s.StringList
	visible := el.PartNames()
	cur := el
	for {
		for _, p := range visible {
			for _, w := range wants {
				if p == w {
					return true
				}
			}
		}
		// Walk the exportparts chain: only part names the host re-exports remain
		// visible to the next outer tree, under their new outer name.
		sr := dom.ContainingShadowRoot(cur)
		if sr == nil {
			break
		}
		host := sr.Host()
		if host == nil {
			break
		}
		exp := parseExportparts(host.GetAttribute("exportparts"))
		if len(exp) == 0 {
			break // no re-export: part names do not cross this boundary
		}
		var next []string
		for _, p := range visible {
			if outers, ok := exp[p]; ok {
				next = append(next, outers...)
			}
		}
		if len(next) == 0 {
			break // none of the visible part names are re-exported
		}
		visible = next
		cur = host
	}
	return false
}

// parseExportparts parses an `exportparts` attribute value into a map of
// shadow-internal part name -> outer part names. The value is a comma-separated list of
// `ident : ident` pairs (CSS Scoping Level 1 part-mapping-list); a single inner name
// may be exported to multiple outer names (`exportparts="x: a, x: b"`).
func parseExportparts(attr string) map[string][]string {
	if attr == "" {
		return nil
	}
	m := make(map[string][]string)
	for _, pair := range strings.Split(attr, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		if i := strings.Index(pair, ":"); i >= 0 {
			inner := strings.TrimSpace(pair[:i])
			outer := strings.TrimSpace(pair[i+1:])
			if inner != "" && outer != "" {
				m[inner] = append(m[inner], outer)
			}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
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
