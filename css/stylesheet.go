// Translation of: Source/WebCore/css/StyleSheet.h
//                  Source/WebCore/css/StyleSheet.cpp
//                  Source/WebCore/css/CSSStyleSheet.h
//                  Source/WebCore/css/CSSStyleSheet.cpp
//                  Source/WebCore/css/StyleSheetContents.h
//                  Source/WebCore/css/StyleSheetContents.cpp
// Completeness: 70%
// Simplifications:
//   - the C++ inheritance hierarchy (StyleSheet -> CSSStyleSheet) is collapsed to
//     a single struct with the union of fields
//   - StyleSheetContents is merged into CSSStyleSheet; the "contents" abstraction
//     is preserved only via the RuleSet field
//   - CSSOM wrappers (CSSRuleList / CSSRule) are not modeled; rules are accessed
//     directly via the slice returned by Rules()
//   - owner node and owner rule are tracked via dom.Node and ImportRule pointers;
//     weak pointers are not used (Go GC manages lifetime)
//   - mutation observers, the "was mutated" optimization, and rule mutation scopes
//     are omitted
//   - loading state is always false: this port parses synchronously and never fetches
//     external resources

package css

import (
	"wb-ui/dom"
)

// StyleSheet is the Go translation of the abstract base class WebCore::StyleSheet.
// CSSStyleSheet is the only concrete implementation in this port, so the interface
// is a thin contract exposing the spec-mandated methods.
type StyleSheet interface {
	Disabled() bool
	SetDisabled(bool)
	OwnerNode() dom.Node
	Href() string
	Title() string
	Type() string
	BaseURL() string
	IsLoading() bool
}

// CSSStyleSheet is the Go translation of WebCore::CSSStyleSheet. It holds a parsed
// rule list (mirroring StyleSheetContents's parsed rules) plus the metadata
// (href/title/disabled/owner) that the CSSOM exposes. The parser populates the rule
// list; the resolver reads it back via Rules().
type CSSStyleSheet struct {
	rules    []Rule
	disabled bool
	owner    dom.Node
	href     string
	title    string
	baseURL  string
	media    string
	origin   Origin
}

// NewCSSStyleSheet creates an empty CSSStyleSheet. The origin defaults to author.
func NewCSSStyleSheet() *CSSStyleSheet {
	return &CSSStyleSheet{origin: OriginAuthor}
}

// NewCSSStyleSheetWithOwner creates a CSSStyleSheet owned by node, mirroring the
// (ownerNode, baseURL) constructor in WebKit.
func NewCSSStyleSheetWithOwner(node dom.Node, baseURL string) *CSSStyleSheet {
	return &CSSStyleSheet{
		owner:   node,
		baseURL: baseURL,
		origin:  OriginAuthor,
	}
}

// Disabled reports whether the stylesheet is disabled, mirroring
// CSSStyleSheet::disabled().
func (s *CSSStyleSheet) Disabled() bool { return s.disabled }

// SetDisabled enables or disables the stylesheet, mirroring CSSStyleSheet::setDisabled.
func (s *CSSStyleSheet) SetDisabled(d bool) { s.disabled = d }

// OwnerNode returns the DOM node that owns this stylesheet (typically a <link> or
// <style> element), mirroring CSSStyleSheet::ownerNode().
func (s *CSSStyleSheet) OwnerNode() dom.Node { return s.owner }

// SetOwnerNode sets the owner node. Used by the parser when associating a parsed
// sheet with its owning element.
func (s *CSSStyleSheet) SetOwnerNode(n dom.Node) { s.owner = n }

// Href returns the href of the stylesheet (for inline sheets this is the document
// URL), mirroring CSSStyleSheet::href().
func (s *CSSStyleSheet) Href() string { return s.href }

// SetHref sets the href.
func (s *CSSStyleSheet) SetHref(h string) { s.href = h }

// Title returns the title attribute, mirroring CSSStyleSheet::title().
func (s *CSSStyleSheet) Title() string { return s.title }

// SetTitle sets the title.
func (s *CSSStyleSheet) SetTitle(t string) { s.title = t }

// Type returns the MIME type, always "text/css" for a CSSStyleSheet.
func (s *CSSStyleSheet) Type() string { return "text/css" }

// BaseURL returns the base URL for resolving relative URLs in the sheet.
func (s *CSSStyleSheet) BaseURL() string { return s.baseURL }

// SetBaseURL sets the base URL.
func (s *CSSStyleSheet) SetBaseURL(u string) { s.baseURL = u }

// Media returns the media query string for the sheet.
func (s *CSSStyleSheet) Media() string { return s.media }

// SetMedia sets the media query string.
func (s *CSSStyleSheet) SetMedia(m string) { s.media = m }

// Origin returns the cascade origin of the sheet (user-agent / user / author).
func (s *CSSStyleSheet) Origin() Origin { return s.origin }

// SetOrigin sets the cascade origin.
func (s *CSSStyleSheet) SetOrigin(o Origin) { s.origin = o }

// IsLoading reports whether the sheet is still loading. This port never loads
// external resources, so IsLoading always returns false.
func (s *CSSStyleSheet) IsLoading() bool { return false }

// Rules returns the parsed rule list. The returned slice is the live backing slice;
// callers should not mutate it.
func (s *CSSStyleSheet) Rules() []Rule { return s.rules }

// AppendRule appends a parsed rule.
func (s *CSSStyleSheet) AppendRule(r Rule) { s.rules = append(s.rules, r) }

// InsertRule inserts r at index. Index out of range appends.
func (s *CSSStyleSheet) InsertRule(r Rule, index int) {
	if index < 0 || index > len(s.rules) {
		s.rules = append(s.rules, r)
		return
	}
	s.rules = append(s.rules, nil)
	copy(s.rules[index+1:], s.rules[index:])
	s.rules[index] = r
}

// DeleteRule removes the rule at index. Out-of-range index is a no-op.
func (s *CSSStyleSheet) DeleteRule(index int) {
	if index < 0 || index >= len(s.rules) {
		return
	}
	s.rules = append(s.rules[:index], s.rules[index+1:]...)
}

// StyleSheetList mirrors StyleSheetList. It is a thin slice wrapper used by the
// document.styleSheets accessor.
type StyleSheetList struct {
	sheets []*CSSStyleSheet
}

// NewStyleSheetList constructs an empty StyleSheetList.
func NewStyleSheetList() *StyleSheetList { return &StyleSheetList{} }

// Length returns the number of sheets, mirroring StyleSheetList::length().
func (l *StyleSheetList) Length() int { return len(l.sheets) }

// Item returns the sheet at index or nil, mirroring StyleSheetList::item().
func (l *StyleSheetList) Item(index int) *CSSStyleSheet {
	if index < 0 || index >= len(l.sheets) {
		return nil
	}
	return l.sheets[index]
}

// Append appends a sheet to the list.
func (l *StyleSheetList) Append(s *CSSStyleSheet) { l.sheets = append(l.sheets, s) }

// All returns a slice of all sheets.
func (l *StyleSheetList) All() []*CSSStyleSheet { return l.sheets }
